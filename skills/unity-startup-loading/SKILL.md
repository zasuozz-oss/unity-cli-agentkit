---
name: unity-startup-loading
description: "Use when working on app startup/loading screens, SDK initialization order, game data download, or bugs mentioning stuck loading, slow first load, blocked startup, or crashes on game scene entry on low-end devices."
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
