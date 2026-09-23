---
name: unity-ui-spam-click
description: "Use when a Unity bug report says clicking/tapping fast, spam, double tap, click nhanh or bấm liên tục breaks the UI — a popup or reward frame stuck on screen (bị dính), the game frozen (kẹt game) while input still works, a tutorial skipped or stuck, a hand shown over a popup, panels stacked, animations fighting, a list rebuilt mid-flow. Covers the mechanisms behind spam bugs, the repro ladder (region spam → directed taps → same-frame logic repro in the player loop), and guards that fix them without new deadlocks."
---

# UI Spam-Click Bugs — Reproduce the Race, Then Guard the Right Place

A spam bug is never "the player clicked too fast". A second input landed
**between two steps that assumed nothing could happen in between**, and a
piece of state (a flag, a tween, a recycled cell, a tutorial step) was left
half-done. The fix is a guard at the place where the state goes wrong, found
by reproducing the interleaving, not by disabling the button named in the
report.

This skill runs inside `@unity-bug-regression-workflow`: recall prior fixes
first, and reproduce before writing the fix. The same report text coming back
after a fix is often **a new path to the same symptom**. Check whether the old
guard even sits on the path you reproduce.

## 1. Know the mechanisms

Each row comes from a real shipped bug. The report rarely names the mechanism,
so match the symptom.

| Symptom | Mechanism |
|---|---|
| Popup/frame waits on screen, input still live, no dialog or hand for the current step (a dim overlay may remain) | A coroutine/task is stuck in `WaitUntil(flag)`. The step that releases the flag never ran. |
| Same report "fixed" before, came back | The guard covered **one entry point** of a shared rebuild or state change, and a sibling (another bar, tab, cell or close button) stayed open. Or the old guard is on a different path entirely. |
| Tutorial step skipped, later freeze | A **skip/shortcut branch jumped over the release step**. Flag set by step A, released only by step C, and a `Skip…` path goes from A straight past C. |
| Tutorial advances or breaks after tapping something the step did not point at | **Input reached an unintended target**: a click-through focus hole, or a panel moving under the finger, exposes other items. Their async callbacks land in a *later* step and act on it. |
| Off-sequence handler ran on the wrong step | **Polling state machine window**: after `Fire()`, the machine only advances on its next poll (e.g. 16 ms), so for one tick the old step is still "current" and a second handler sees stale state. |
| Hand and popup shown together, or a step that can never fire | Off-sequence input: step N+1's button was hit while the machine waited for step N. The N+1 flag can then never fire, because its button is already gone. |
| A list/grid is rebuilt mid-flow, and the tutorial target disappears | Spam triggered a data reload that recycled the cell the flow was holding (`@unity-scrollview-recycling`). |
| A window the tutorial closed is open again | A global `UnblockUI()`/`EventSystem.enabled = true` in some unrelated `finally` (a network call, for example) re-enabled input mid-flow. |
| Panels stacked, or a panel animates twice | Open is not idempotent (`@unity-panel-navigation`). |
| Animations fight, or the end state is wrong | A new tween started without killing the in-flight one (`@unity-dotween-safety`). |
| The fix itself freezes the game | A new wait was added between two systems that already wait on each other's flags (cross-wait deadlock). |

## 2. Pre-flight — or the repro lies

Check these before the first tap. Each one has produced false observations in
a real session.

- **Enter Play Mode Options with domain reload off.** Check
  `EditorSettings.enterPlayModeOptionsEnabled` and `enterPlayModeOptions`. With
  domain reload off, statics (busy flags, skip flags, "blocked by tutorial")
  survive between plays, and one run's leftovers look like the bug. Reset the
  suspect statics in edit mode before every play.
  - A bug the user "spammed" in the Editor after several plays may be
    **Editor-only leaked state**, seeded by earlier plays, including the agent's
    own test runs. Real case: a leftover skip flag from the previous play sent
    the next Buy into a step that sets no blocking flag, so a dialog stacked on
    another panel. Before accepting such a report, prove or rule out the leak:
    inject the suspected stale state on a clean play and see whether it
    reproduces. After the fix, confirm that a second play starts clean. That
    much was proven in the real case. Recommended but not done there: run a
    Player build, which starts every launch with fresh statics, and confirm it
    does not show the bug. Without that build, "Editor-only" is an inference
    from the code.
  - Look for singletons with `if (IsInitialized) return;` in `Init()`. With
    domain reload off, they carry their whole context across plays.
  - The fix is `[RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.SubsystemRegistration)]`
    static resets for those singletons and handshake flags. This makes the Editor
    behave like a build, so it is a legitimate fix, not a workaround.
- **Fresh account per attempt when the flow mutates server state.** A purchase
  or level-up changes server-side data, so a replay on the same account takes a
  different path. Back up PlayerPrefs first (macOS:
  `defaults export unity.<company>.<product> backup.plist`), use
  `PlayerPrefs.DeleteAll()` for a new guest, and `defaults import` at the end.
  **Never lose the user's real progress.**
- **Fixed Game view size before recording any coordinate.** Use the
  `gameview_size` snippet in `@utk-playmode-driving`. Tap points only replay at
  the resolution they were recorded at.
- Drive play mode unattended (`@utk-playmode-driving`): `utk set_autotick
  --enable true`, `utk editor play`, and `utk wait_for` on a real state instead
  of `sleep`. When there is no state to wait on, a `wait_for` on an impossible
  condition with `timeout_s` works as a bounded "let the game run N s".

## 3. The repro ladder

Climb only as far as needed, and record what each rung showed.

### Rung 1 — real taps through the EventSystem, from the player loop

`button.onClick.Invoke()` skips what makes spam dangerous: focus masks,
blockers, panels moving under the point, frame spacing. Send taps the way a
finger does: raycast at a screen point, then run pointer down, up and click on
what is hit.

- **Yield to the player loop first.** Inside the `utk exec` call itself,
  `Screen.width/height` are the calling editor window's size (e.g. 746×783), not
  the Game view's. Tap 0 then misses overlay canvases.
- **Log to a file, not `Debug.Log`.** The console collapses identical lines and
  lost spam taps. Use an absolute path and one line per tap: timestamp,
  `Time.frameCount`, whether `EventSystem` is enabled, what was hit, and the
  suspect flags or current step.
- **Optional step gate.** Wait until the state machine reaches step X before
  the loop, to aim the spam at one step's window.

Core of the helper. Keep it outside `Assets/` (e.g. `AgentScripts/tap.cs`) and
run it with `utk exec --file`:

```csharp
Cysharp.Threading.Tasks.UniTask.Void(async () => {
  await Cysharp.Threading.Tasks.UniTask.Yield();          // leave the exec context
  // optional gate: await UniTask.WaitUntil(() => CurrentStep() == TARGET_STEP);
  var rng = new System.Random(SEED);
  for (int i = 0; i < COUNT; i++) {
    var es = UnityEngine.EventSystems.EventSystem.current;
    // Modes: random point in a box (below), a fixed x,y, a GameObject's rect
    // centre, or a directed sequence of points.
    var pos = new Vector2(X0 + (float)rng.NextDouble() * (X1 - X0),
                          Y0 + (float)rng.NextDouble() * (Y1 - Y0));
    string res;
    if (es == null || !es.enabled) res = "BLOCKED(es off)"; // input blocked: taps cannot reach this window
    else {
      var ped = new UnityEngine.EventSystems.PointerEventData(es) { position = pos };
      var hits = new System.Collections.Generic.List<UnityEngine.EventSystems.RaycastResult>();
      es.RaycastAll(ped, hits);
      res = "no hit";
      if (hits.Count > 0) {
        var go = hits[0].gameObject;
        ped.pointerCurrentRaycast = ped.pointerPressRaycast = hits[0];
        var down = UnityEngine.EventSystems.ExecuteEvents.ExecuteHierarchy(go, ped,
                     UnityEngine.EventSystems.ExecuteEvents.pointerDownHandler);
        var click = UnityEngine.EventSystems.ExecuteEvents.GetEventHandler<
                      UnityEngine.EventSystems.IPointerClickHandler>(go);
        ped.pointerPress = down ?? click; ped.eligibleForClick = true;
        UnityEngine.EventSystems.ExecuteEvents.Execute(ped.pointerPress ?? go, ped,
          UnityEngine.EventSystems.ExecuteEvents.pointerUpHandler);
        if (click != null) UnityEngine.EventSystems.ExecuteEvents.Execute(click, ped,
          UnityEngine.EventSystems.ExecuteEvents.pointerClickHandler);
        res = "hit=" + go.name + " click=" + (click ? click.name : "none");
      }
    }
    System.IO.File.AppendAllText(LOG_ABS_PATH, System.DateTime.Now.ToString("HH:mm:ss.fff")
      + " #" + i + " f=" + Time.frameCount + " pos=" + pos + " " + res
      + " | step=" + CurrentStep() + " flag=" + SuspectFlag + "\n");
    await Cysharp.Threading.Tasks.UniTask.DelayFrame(FRAMES_BETWEEN);
  }
});
return "posted";
```

Without UniTask, run the same loop as a coroutine on any active
`MonoBehaviour`; only the player-loop requirement matters. Screen-space-camera
canvases need `canvas.worldCamera` in `RectTransformUtility.WorldToScreenPoint`.

**Region spam first.** Use several seeds over boxes: around the reported
control, around what the flow shows next, and the full screen. It is cheap,
and it finds candidates. It is **not** proof of absence: a race window of one
frame can survive hundreds of random taps (measured: ~200 taps × 4 seeds never
hit a real ≤16 ms window). Treat "random spam did not break it" as "no
evidence", never as "not reproducible".

### Rung 2 — directed tap sequences for a known candidate

When the mechanism table or the log suggests an order ("tap an item *other*
than the tutorial target, then Buy, then Confirm, during step X"), replay that
exact point sequence with real taps. This is what reproduces the
"wrong target" and "skip branch" classes deterministically. The log line that
proves it looks like `click=<unintended item> | step=<earlier step>`.

### Rung 3 — same-frame logic repro in the player loop

When the tap log shows `BLOCKED(es off)` around the window (the flow disables
input for a few hundred ms), or the window is a single frame, taps cannot hit
it. Then, from the player loop, **call the real entry points in the racing
order within one frame**: the handlers the taps would have reached, the async
callback bodies, then the follow-up action after the delay the flow uses. Log
the state afterwards and wait long enough (e.g. 20 s) to prove it stays stuck.
The repro is valid only if the end state and screenshot match the tester's.

An EditMode test (`@unity-editmode-tests`) can pin the same order afterwards.
It is not a substitute: it skips the player loop, the polling state machine
and the real callbacks.

### Record before fixing

Write down the rung, and the seeds, boxes or sequence, or the logic call order.
Add the log lines at the freeze (which step it waits on, which flag stayed
set) and a screenshot that matches the report. If nothing reproduces, report
exactly what ran and do not fix blind.

## 4. Fix where the state goes wrong

Find the **single place** that produces the bad state (the branch that skips
the release step, the handler that acts for the wrong target), and fix it
there.

- **A step advances only for its intended target.** Check identity ("is this
  the item the tutorial pointed at?"), not "some item was clicked". Unintended
  items reached through a click-through hole must not fire the step.
- **Skip branches must not jump over a release step.** If a shortcut can skip
  the step that clears a blocking flag, either clear the flag in the shortcut,
  or allow the shortcut only when that step is truly moot.
- **Off-sequence handlers must be safe during the poll window.** After
  `Fire()`, the old step may still read as current for one tick. Handlers must
  not branch on "current step" alone.
- **Absorbing off-sequence input needs care.** Advancing through the missed
  step instead of ignoring the click is right, **but** absorbing by *setting a
  blocking flag* is only safe if no other path can skip that flag's release
  step. In a real case, the absorb branch was half of the deadlock.
- **Safety valves must be precise.** `WaitUntil(() => !flag ||
  !flowStillRunning)` does not help when the flow survives past the releasing
  step. Valve on "the release step is no longer ahead", or clear the flag
  wherever that step is skipped.
- **Guard shared mutators, not the reported caller.** Grep every entry point
  that rebuilds, reloads or advances the same state, and put one predicate in
  the shared function. Deliberately exclude the step that legitimately needs
  that input.
- **One owner per busy/motion flag.** The code that sets it clears it, through
  one `End(owner)` path. Others only check it. Grep for global unblocks in
  `finally` blocks, which re-open input behind the flow's back.
- **Make open/show idempotent, and cancel before restarting.** Kill the
  in-flight tween or cancel its CTS, and restore interactivity in `finally`.
- **Disabling input is a complement, not the fix.** It narrows the window, and
  another entry point still reaches the same state.

**Never:**
- Guard only the button named in the report.
- Add a new `await`/`WaitUntil` between two systems that already wait on each
  other (for example, a tutorial step waiting for popups to be idle while the
  popup waits on the tutorial flag).
- Rely on a timer debounce as the whole fix.

## 5. Verify

1. Compile clean (`utk editor refresh`), then re-run **every repro that
   reproduced**, at the same rung. The flow must now reach the release step,
   and the flag must clear (e.g. dialog shown → Continue → flag false → next
   step).
2. Re-run region spam with new seeds, as a smoke test only.
3. Re-run the previous fix's STR. Spam fixes on the same flow regress each
   other.
4. Walk the normal, slow path once. The guard must not block the click the
   flow legitimately waits for, including when the intended target is already
   owned or complete.
5. Save the mechanism, the racing order and the fixed site as a lesson.
