---
name: utk-playmode-driving
description: Use when an agent has to drive Play mode unattended through `utk` — entering play, waiting for a scene to be ready, stepping frames on an unfocused Editor, screenshotting a runtime state, setting the Game view resolution, or when `editor refresh`/`run_tests`/`exec` hang or fail around play mode ("This cannot be used during play mode", "please use EditorSceneManager.OpenScene", a screenshot stuck on the loading screen).
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
  --tolerate_missing true --timeout_s 60 \
  --on_met '{"capture":{"view":"game","source":"screen","save_path":"Temp/shots/game.png"}}'
```

- `wait_for` polls **server-side, every frame**, and `on_met.capture` shoots in
  the same frame the condition holds — no client loop, no missed 2-second
  screens. `source:"screen"` includes Screen Space-Overlay UI; the default
  `camera` source misses it.
- `tolerate_missing true` is what lets you wait for an object that does not
  exist yet (the controller spawns after the scene loads).
- A **sync** wait holds the command queue: every other `utk` call waits behind
  it. If the condition depends on a command you still have to send, pass
  `--async true` and poll `utk wait_status --wait_id <id>` — otherwise it
  deadlocks until its timeout.
- `set_autotick` costs a CPU core like a focused Editor. `--enable false` at the
  end of the run.

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

An `exec` sent in the instant play mode starts can come back as a network
error: entering play reloads the domain and drops the in-flight request. Retry
once after it settles; don't treat it as a dead connection.

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
  restore them after. A test that called `PlayerPrefs.DeleteKey` in `SetUp`
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
