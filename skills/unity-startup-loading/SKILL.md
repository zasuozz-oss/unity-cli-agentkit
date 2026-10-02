---
name: unity-startup-loading
description: "Use when working on app startup/loading screens, SDK initialization order, game data download, or bugs mentioning stuck loading, slow first load, blocked startup, crashes on game scene entry on low-end devices, consent/UMP/GDPR delaying boot, or a loading/transition cover that opens before the content behind it is ready (old skin/model visible, then swaps)."
---

# Unity Startup & Loading Flow

## Overview
Guidelines for structuring the startup sequence of a mobile game: SDK initialization, game data download, and scene-entry task scheduling. Core principle: **no single SDK or API call may block the game from loading.**

## When to Use
- Modifying the loading controller / bootstrap sequence.
- Adding a new SDK init or a new API call during loading.
- Debugging stuck loading, long first load, or low-end device crashes when entering the game scene.
- Instrumenting the login → loaded funnel.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Init/Data separation** | SDK initialization (Firebase, ads, attribution) runs in parallel with — never ahead of — game data download. |
| **Single point of hang** | Any one request in the boot chain (remote config, profile, entitlement…) with no timeout can hold the whole game on the loading screen. |
| **Bounded parallelism** | Scene-entry task fan-out (`Task.WhenAll`-style) must be capped; unbounded parallel loads crash low-end devices. |
| **Funnel checkpoints** | Analytics events between each loading phase to locate silent user drop-off. |
| **Stale prefetch failure** | A boot-time request cached as a task and consumed later can serve a *failure* recorded when the network was still down. |

## Best Practices
- ✅ **Always** wrap every loading-phase API call with an explicit timeout AND a defined fallback value — decide up front what the game does when that call never answers.
- ✅ Keep SDK init failures non-fatal: the game must reach the menu even if an SDK fails to initialize.
- ✅ Cap concurrent downloads/instantiations on game-scene entry; queue the rest (low-end devices die on spikes, not totals).
- ✅ Fire funnel events at every phase boundary (login success → config ready → data ready → scene loaded) so drop-off is measurable.
- ✅ Keep loading status text order in sync with the actual phases — mismatched text hides where users are stuck.
- ❌ **NEVER** `await` an SDK init before starting the game data download.
- ❌ **NEVER** let a pre-game popup or analytics call block the loading chain.
- ❌ **NEVER** show an error popup whose "retry" button is wired to nothing — a transient network blip becomes a permanent dead-end.
- ❌ **NEVER** re-run login or version checks from a boot popup's `Close()`. A dismissible popup shown during boot (soft update, notice) must clear the flag that gates scene-advance and call the advance step directly. Re-running the checks shows the same popup again, forever (`@unity-popup-queue`).

## A cached prefetch can serve a stale failure forever

Prefetching at boot (`_configTask = FetchAsync()`) and awaiting the stored task
later is fine — until the app resumes from background with the network still
down. The task is already *completed, faulted*; every later consumer awaits the
same recorded failure, and the retry button re-awaits it too. The user is stuck
until they force-quit.

```csharp
// ✅ consume once, discard on failure, retry fresh
public async UniTask<Config> GetConfigAsync(CancellationToken ct)
{
    if (_configTask == null) _configTask = FetchAsync(ct);
    try { return await _configTask; }
    catch { _configTask = null; return await FetchAsync(ct); }  // fresh attempt, not the cached corpse
}
```

Rule: cache the *task* only while it can still succeed. Anything holding a
faulted/cancelled result must be dropped before the next attempt — and a retry
button must call the fresh path, not re-await the cache.

## Consent (UMP / GDPR) is a boot-chain request too

The consent update and form are the slowest, least reliable step in many
boots — `ConsentInformation.Update` measured ~35-40 s on some devices even
outside the EEA, and the form's WebView can hang outright.

- ✅ Start it first (in `Awake` of the boot scene), in parallel with data
  download, not after the loading bar.
- ✅ Give it a hard cap, and anchor that cap to **app start**, not to when the
  wait began: a "wait up to N s" measured from a later point silently adds the
  earlier seconds. Clarify with the user which baseline a number means.
- ✅ Ads init waits on consent; the *game* doesn't — on timeout, continue and
  let ads initialise when consent lands.
- ✅ Its callbacks may be off the main thread (`@unity-async-patterns` → "SDK
  callbacks").
- ✅ Measure on a device (`@unity-device-testing`): `adb shell am force-stop`,
  `logcat -c`, then cold-launch and timestamp each phase. Confirm the
  installed build is the one with your change before reading the numbers.
- ❌ A fixed "fake" loading duration (`timeLoading = 5f`) on top of real work
  — it is pure added wait.

## A transition cover must wait for the work it hides

A fade/cover/loading overlay that opens on a timer reveals whatever isn't
ready yet — on a slow device the player sees the old outfit/skin/model swap
in after the cover lifts. Gate the reveal on a **completion predicate with a
max wait**, not on a delay:

```csharp
// hold the cover until the async apply finished, never longer than maxWait
async UniTask HoldCoverUntil(Func<bool> done, float maxWait, CancellationToken ct)
{
    float t = 0f;
    while (!done() && t < maxWait) { t += Time.unscaledDeltaTime; await UniTask.Yield(ct); }
}
await HoldCoverUntil(() => !model.IsApplyingSkin, 2f, ct);
cover.Open();
```

Expose the predicate (`IsApplyingSkin`, `IsLoading`) on the component doing the
work. If the cover is an Animator clip and a fade runs beside it, drive the
fade's alpha from the clip's normalized time each frame — two independent
timers drift apart on a hitch.

## Few-Shot Examples

### Example 1: Any boot-chain request needs a timeout + fallback
**User**: "check whether a hung API in the loading chain blocks the game, and fix it so it never blocks"

**Agent**: wrap the call once and reuse it for every boot request — the fallback
is the design decision, the timeout is just plumbing.

```csharp
// Hard cap: a hanging boot request must never hold the loading screen.
async UniTask<T> BootRequestAsync<T>(
    Func<CancellationToken, UniTask<T>> call, T fallback, CancellationToken ct)
{
    try
    {
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(5));
        return await call(cts.Token);
    }
    catch (Exception)
    {
        IsFullySynced = false;   // degraded, but the game still boots
        return fallback;
    }
}

// caller decides what "no answer" means for its own data
var config = await BootRequestAsync(_api.FetchConfigAsync, CachedConfig, ct);
```

## Related Skills
- `@unity-async-patterns` - Cancellation and UniTask usage in loading chains.
- `@unity-telemetry-analytics` - Pre-init event buffering and funnel instrumentation.
- `@unity-popup-queue` - Popups shown during boot and the gates they hold.
- `@unity-remote-image-flicker` - Background prefetch of images the first screens show.
- `@unity-device-testing` - Cold-start timing on a real device.
