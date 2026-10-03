---
name: utk-playmode-driving
description: Use when an agent has to drive Play mode unattended through `utk` — entering play, waiting for a scene to be ready, stepping frames on an unfocused Editor, screenshotting a runtime state, setting the Game view resolution, or when `editor refresh`/`run_tests`/`exec` hang or fail around play mode ("This cannot be used during play mode", "please use EditorSceneManager.OpenScene", a screenshot stuck on the loading screen), or when the Editor hangs / stops answering / only progresses while its window is focused, or runtime state looks impossible (errors that appear only mid-Play, values surviving Stop → Play, every tween frozen).
---

# Driving Play Mode Unattended

Play mode is where most agent runs stall. The causes are few and they repeat:
the Editor is not focused, the agent waits on a wall clock instead of a state,
or it calls an API that belongs to the other mode. Each one below cost real
sessions dozens of retries.

## An unfocused Editor barely runs frames

The OS throttles a backgrounded Unity, so in play mode the player loop crawls:
`sleep 5`/`10`/`20` before a screenshot still captures the loading screen, and
no amount of sleeping fixes it. **Stop trusting wall-clock time — keep the
loop ticking and wait on a state.** `com.unity.pipeline` 0.7+ ships both halves
(`utk list --grep wait_for` to check; older packages → the Step fallback below).

```sh
utk set_autotick --enable true             # full-rate ticks while unfocused; survives reloads this session
utk editor play
utk wait_for --condition '{"findType":"MyGame.GameController","member":"IsReady","op":"equals","value":true}' \
  --tolerate_missing true --timeout_s 60
utk screenshot --output Temp/shots/game.png --max 1024
```

- `wait_for` polls **server-side, every frame** — no client loop, no missed
  2-second screens.
- Shoot with `utk screenshot --output <path>`: it lands where you say.
  `wait_for --on_met '{"capture":…}'` and `capture_game_view` take a
  `save_path` that resolves **under `Assets/`** — `Temp/x.png` becomes
  `Assets/Temp/x.png`, imported with a `.meta` (`..` is refused). Used one by
  mistake? `rm -rf Assets/Temp Assets/Temp.meta`. For a frame-exact shot,
  pause and step (below) instead of `on_met`.
- `condition.op` accepts only `equals`, `notEquals`, `greaterThan`,
  `lessThan`, `contains`, `changed`. There is no `greaterOrEqual` — write
  "N or more" as `{"op":"greaterThan","value":N-1}`.
- `tolerate_missing true` is what lets you wait for an object that does not
  exist yet (the controller spawns after the scene loads).
- A **sync** wait holds the command queue: every other `utk` call waits behind
  it. If the condition depends on a command you still have to send, pass
  `--async true` and poll `utk wait_status --wait_id <id>` — otherwise it
  deadlocks until its timeout.
- `set_autotick` costs a CPU core like a focused Editor. `--enable false` at the
  end of the run.

### Still frozen with autotick on → check Run In Background

`set_autotick` keeps the **Editor** ticking, but in play mode the **player
loop** also stops while unfocused when Player Settings → *Run In Background*
is off (`runInBackground: 0` in `ProjectSettings/ProjectSettings.asset`, the
default). Symptom: `utk` answers fine, but the game sits on the loading screen
and `Time.frameCount` does not move until someone clicks the Unity window.

```sh
grep -n runInBackground ProjectSettings/ProjectSettings.asset   # 0 = the cause
utk exec 'UnityEditor.PlayerSettings.runInBackground = true; UnityEditor.AssetDatabase.SaveAssets(); return UnityEditor.PlayerSettings.runInBackground;'
```

- Set it through the Editor as above, not by editing the `.asset` while the
  Editor is open — Unity rewrites the file from memory and reverts the edit.
- Re-enter play mode afterwards; the running session keeps the old value.
- Simulated clicks/touches (Input System) are also dropped while unfocused.
  Before driving input, set the runtime-only (not saved) setting:
  `UnityEngine.InputSystem.InputSystem.settings.editorInputBehaviorInPlayMode =
  UnityEngine.InputSystem.InputSettings.EditorInputBehaviorInPlayMode.AllDeviceInputAlwaysGoesToGameView;`
- Safe for mobile builds: Android/iOS ignore it. On a desktop build it changes
  real behavior (the game keeps running when alt-tabbed), so tell the user.

### Editor hangs or stops answering — triage in this order

Each step is one bounded command; stop at the first that explains it.

1. `utk status` — not reachable / pipeline too old (< 0.5 has no autotick by
   default). Old package → `unity pipeline install --package-version <v>`, then
   focus the Editor once so it resolves.
2. `utk editor_status` — compiling / importing / updating? Wait on that state,
   do not retry the command (in-flight requests are dropped on reload).
3. Modal dialog or progress bar open → no command can run; hand it to the user.
4. Play mode frozen while `utk` still answers → Run In Background (above).
5. Editor answers only when focused → `utk set_autotick --enable true`, and
   once per machine Preferences → General → Interaction Mode → **No Throttling**.

**Fallback — and for frame-exact captures:** pause and step frames yourself.
This also makes tween/VFX captures deterministic (same frame every run):

```csharp
// AgentScripts/step.cs — run with: utk exec --file AgentScripts/step.cs
EditorApplication.isPaused = true;
Time.captureDeltaTime = 1f / 60f;          // each Step = one fixed 1/60 s frame
for (int i = 0; i < 30; i++) EditorApplication.Step();
return $"frame={Time.frameCount} scene={UnityEngine.SceneManagement.SceneManager.GetActiveScene().name}";
```

```sh
utk editor play
for i in $(seq 1 15); do                   # bounded — never an open-ended loop
  r=$(utk exec --file AgentScripts/step.cs)
  echo "$r" | grep -q "scene=Game" && break
done
utk screenshot --max 1024
```

- Return a **readiness signal**, not just the scene name — the scene can be
  active while its controller is still loading. `FindAnyObjectByType<T>() != null`
  for the component that owns the screen is a good one.
- For animation frames: step N, screenshot, repeat — each capture lands on the
  same frame every run.
- `isPaused` persists. Unpause (or `utk editor stop`) when you're done, or the
  next person to press Play sees a frozen game.

## A/B a fix at runtime (repro without reverting code)

When the fix is already compiled in, get the "before" run by removing the fix
**in the running game**, not by reverting and recompiling. Typical case: the
fix subscribes a handler to an event — strip that delegate from the event's
backing field:

```csharp
// AgentScripts/ab_disable_fix.cs — placeholders: <OWNER_TYPE>, <EVENT_FIELD>, <FIX_TYPE>
var inst = UnityEngine.Object.FindAnyObjectByType<<OWNER_TYPE>>();
var f = typeof(<OWNER_TYPE>).GetField("<EVENT_FIELD>",
    System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic);
var d = (System.Action)f.GetValue(inst);
int removed = 0;
if (d != null)
    foreach (var x in d.GetInvocationList())
        if (x.Target is <FIX_TYPE>) { d -= (System.Action)x; removed++; }
f.SetValue(inst, d);
return $"removed={removed}";                // 0 = wrong field/type, not a repro
```

Then drive the STR **step by step with `utk exec`**, calling the same public
handlers the buttons call (`OnClick…`, `Open…`, `Close…`) — find them from the
button's `onClick` or the prefab, not by guessing. After each step print **one
compact state line** from a reusable probe file:

```
step=3 tab=Profile widget.active=False topPopup=DetailsPanel navStack=[Home,Profile,Details]
```

Keep the probe as `AgentScripts/probe.cs` (`utk exec --file`) so both runs
print identical lines. Re-enter Play (the fix comes back) and run the
**identical** step script: the diff between the two logs is the evidence.

A full-screen panel can cover the widget in a screenshot, so it "looks hidden"
in both runs. Trust the state line (`activeSelf`, alpha, parent active); use
the screenshot only as a supplement.

## Driving input

- **Tap a uGUI element**: compute its screen point in one `exec`
  (`RectTransformUtility.WorldToScreenPoint(cam, rt.TransformPoint(rt.rect.center))`,
  `cam` = null for an Overlay canvas), then `utk simulate_pointer --x <x> --y <y>`.
  Keys: `utk simulate_key`. Both go through the real input path, so
  raycast blocking and `interactable` are honoured — `onClick.Invoke()` skips
  both and proves less.
- Fallback when the screen point is uncertain (custom Game view size): raycast
  with `EventSystem.current.RaycastAll` and send
  `ExecuteEvents.ExecuteHierarchy(hit, pointerData, ExecuteEvents.pointerClickHandler)`.
- `exec --file` takes no arguments; a click/probe helper reused every step is
  one file per action, or a tiny file you rewrite per call.
- **Waiting N frames**: step them (`EditorApplication.Step()` loop above) or
  `wait_for` on a member that counts. Never `until [ $(utk exec 'return
  Time.frameCount;') -gt … ]` — unbounded, and hundreds of round trips.

## Pick the scene API by mode

Two opposite failures, both seen repeatedly:

| Mode | Use | Wrong call → error |
|---|---|---|
| edit mode | `EditorSceneManager.OpenScene(path)` | `SceneManager.LoadScene` → "…please use EditorSceneManager.OpenScene() instead" |
| play mode | `SceneManager.LoadScene(name)` | `EditorSceneManager.OpenScene/NewScene` → "This cannot be used during play mode" |

Branch on `Application.isPlaying` in any snippet that may run in either mode.
Loading a scene in play mode also needs it in Build Settings.

## Stop play mode before compiling or testing

A compile queued while play mode runs waits for play mode to end — so
`utk editor refresh` and `utk run_tests` just hang to their timeout. Always:

```sh
utk editor stop; utk editor refresh && utk run_tests --mode editor
```

Entering play, stopping and refreshing reload the domain. A call landing in
that window ("Network error: An error occurred while sending the request",
"Cannot connect …") is retried by `utk` for up to 30s, so `utk editor play;
utk set_autotick --enable true` needs no sleep or retry loop around it. If it
still fails, walk the hang triage above rather than looping.

## State that lies to you in Play mode

Check these before debugging "impossible" runtime state:

| Symptom | Cause | Check / fix |
|---|---|---|
| Errors from nowhere mid-Play: a singleton `Instance` is null, `Awake` state missing, a SmartFormat/`LocalizeStringEvent` "`null` is not a valid choice" | Scripts **recompiled while playing** (Preferences → Script Changes While Playing = Recompile And Continue). Statics and non-serialized fields are wiped | `Editor.log` has `Reloading assemblies after finishing script compilation` during the Play session. Re-enter Play; don't patch code for it |
| Values your own earlier test set come back next Play (pinned timestamp, flag, cache); negative/stuck counters | **Domain reload on Enter Play is off**, so a static you set via `utk exec` survives Stop → Play | `EditorSettings.enterPlayModeOptionsEnabled`. Restore every static you set; or `utk editor refresh` to force a reload. Suspect your own hack first |
| Every DOTween/animation frozen, "scale still 0 after 1s" | `Time.timeScale == 0` (an ad SDK, a pause menu) and the tween isn't `SetUpdate(true)` | `return Time.timeScale;`. Set it to 1 for the test run; not a popup bug |
| Timing measurement is wildly off | Editor is paused — an `exec` timeout or Error Pause left `isPaused = true` | `return EditorApplication.isPaused;` before measuring; unpause and re-measure |

## Game view resolution has no verb

Screenshots in play mode are the Game view's size. There is no `utk` command to
set it; it is internal API, reached by reflection. Keep this as a file and
substitute the size:

```csharp
// AgentScripts/gameview_size.cs — replace W/H, run with utk exec --file
int w = 1080, h = 1920;
var asm = typeof(UnityEditor.EditorWindow).Assembly;
var sizesT = asm.GetType("UnityEditor.GameViewSizes");
var inst = typeof(UnityEditor.ScriptableSingleton<>).MakeGenericType(sizesT).GetProperty("instance").GetValue(null);
var groupType = sizesT.GetProperty("currentGroupType").GetValue(inst);
var group = sizesT.GetMethod("GetGroup").Invoke(inst, new object[] { (int)groupType });
var gT = group.GetType();
int total = (int)gT.GetMethod("GetTotalCount").Invoke(group, null), idx = -1;
for (int i = 0; i < total && idx < 0; i++) {
  var s = gT.GetMethod("GetGameViewSize").Invoke(group, new object[] { i }); var sT = s.GetType();
  if ((int)sT.GetProperty("width").GetValue(s) == w && (int)sT.GetProperty("height").GetValue(s) == h
      && sT.GetProperty("sizeType").GetValue(s).ToString() == "FixedResolution") idx = i;
}
if (idx < 0) {
  var size = System.Activator.CreateInstance(asm.GetType("UnityEditor.GameViewSize"), new object[] {
    System.Enum.Parse(asm.GetType("UnityEditor.GameViewSizeType"), "FixedResolution"), w, h, w + "x" + h });
  gT.GetMethod("AddCustomSize").Invoke(group, new object[] { size }); idx = total;
}
var gvT = asm.GetType("UnityEditor.GameView");
gvT.GetMethod("SizeSelectionCallback", System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Instance)
   .Invoke(UnityEditor.EditorWindow.GetWindow(gvT), new object[] { idx, null });
return $"gameview {w}x{h} idx={idx}";
```

It rides Editor internals — if a Unity upgrade renames a member, the snippet
fails loudly; fix the reflection, don't fall back to guessing the size.

## The Editor may be someone's game in progress

Agents often run against the Editor the user is playing in. Before anything
that rewrites runtime state:

- **Check `EditorApplication.isPlaying` first.** If the user is playing, wait
  (poll on a bounded budget) or ask — don't stop play mode under them.
- **Never delete save data you didn't create in this run.** Seeding a level via
  PlayerPrefs/save files for a screenshot is fine; snapshot the old values and
  restore them after:

  ```sh
  D=$(UTK_NO_EXEC_LOGS=1 utk exec 'return Application.persistentDataPath;' | tr -d '"')
  cp -R "$D" "$SCRATCH/save.bak"                          # before seeding / Play
  # … play, capture, then:
  utk editor stop && rm -rf "${D:?}"/* && cp -R "$SCRATCH/save.bak/." "$D/"
  ```

  Reset any debug static you set too (a forced hour, an "unlock all" flag). A test that called `PlayerPrefs.DeleteKey` in `SetUp`
  without restoring wiped a user's real progress on every `run_tests`
  (utk-test-runner → "Tests share the Editor").

## Rules

- ✅ `set_autotick` + `wait_for` on a readiness signal (or `Step()` + a bounded
  poll); screenshot only once it reports ready.
- ✅ `utk editor stop` before `editor refresh` / `run_tests`.
- ✅ Keep step/prep/gameview snippets as files under `AgentScripts/` and run
  them with `utk exec --file` — they get reused every capture round.
- ❌ **NEVER** `sleep N` and hope the frame advanced.
- ❌ **NEVER** leave the Editor paused, in play mode, or auto-ticking at the end
  of a run.

## Related Skills

- `@utk-cli-core` — `utk` defaults, `-automated`, screenshots and overlay canvases
- `@utk-exec-query` — the snippet contract, timeouts, async stalls
- `@utk-test-runner` — compile check and tests, and why tests must restore global state
- `@unity-ui-motion-tuning` — scrubbing a tween by frame instead of recompiling
