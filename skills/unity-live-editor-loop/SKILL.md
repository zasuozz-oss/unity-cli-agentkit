---
name: unity-live-editor-loop
description: Use when building, changing or verifying a Unity game through `utk` against the OPEN Editor (any game) — the build-script → compile → test → Play → screenshot loop, reproducing a UI bug, capturing screens, or when Play mode is frozen, a screenshot shows the wrong scene/transition, `utk` loses its connection, or the scene lost a manual change after a rebuild.
---

# The live-Editor loop without redo rounds

Most Editor pitfalls are already documented — the failure in real sessions was
**not loading those skills**. Before driving the Editor, load the ones the
table points at (fallback path `~/.unity-cli-agentkit/skills/<name>/SKILL.md`
when the project has no copy).

| Symptom | Read | Fix in one line |
|---|---|---|
| Play mode frozen, `frameCount` stuck, fades never finish | `utk-playmode-driving` | `utk set_autotick --enable true` (or bounded `EditorApplication.Step()`), off at the end |
| "This cannot be used during play mode" / "use EditorSceneManager" | `utk-playmode-driving` | branch on `Application.isPlaying` |
| `refresh` / `run_tests` hang | `utk-playmode-driving` | `utk editor stop` first |
| "Cannot connect … pipeline server" right after a compile or sprite import | `utk-cli-core`, `utk-exec-query` | domain reload; retry ≤3 with a pause |
| `utk test`: "already open in a running Editor" | `utk-test-runner` | batch runner needs the Editor closed; use `run_tests` against the open one |
| Screenshot size wrong | `utk-playmode-driving` | set the Game view size first (below) |
| Test run wiped the user's progress | `utk-test-runner` | snapshot/restore globals |
| Game pieces/FX drawn over a popup, taps reach the board behind it | `unity-layer-audit` | run `scripts/layer_audit.cs` with the popup open |
| Popup text spills out, buttons too small/close/off the panel, ✕ at the edge | `unity-popup-layout` | run `scripts/popup_layout_check.cs` with the popup open |

## 1. The build script owns the scene — backport every hand edit

If a scene/prefab is produced by an idempotent `Tools/build_<scene>.cs`
(`unity-scene-script`), a change made any other way (a quick `utk exec`, an
Inspector tweak, a prefab edit) is **deleted by the next rebuild**. Port it
into the build script in the same pass, then rebuild once to prove it
survives. Seen: an input field added to a scene by `exec` had to be re-authored
when the agent noticed the scene is rebuilt from scratch.

Build-script hygiene that failed compiles:
- One eval scope — give each block distinct local names (`panelBody`,
  `introBody`), not a reused `body`.
- Guard every `Find(...)` of a child you did not create
  (`?? throw new Exception("missing path X")`) — a missing Figma child
  otherwise surfaces as a bare `NullReferenceException`.

## 2. After tests, the active scene is not yours

`run_tests` can leave a different (often empty) scene open, and a test-runner
error can leave the Editor half-restored. Before entering Play for a capture:
`EditorSceneManager.OpenScene(<the scene>)`, then Play. Seen: a capture showed
an empty skybox because Play started in the scene the test run left behind.

## 3. Capture recipe (one file, reused every round)

1. `utk editor stop` if playing (ask first if the **user** is playing).
2. Snapshot the save/PlayerPrefs you will change; seed the state you need.
3. Open the scene; set the Game view size. Unity 6 has a public call:
   `UnityEditor.PlayModeWindow.SetCustomRenderingResolution(w, h, "name")`
   (the reflection snippet in `utk-playmode-driving` is the fallback).
4. `utk editor play`; `utk set_autotick --enable true`; wait on a readiness signal
   (`wait_for`, or poll `Time.frameCount`/a game flag) — never a bare sleep.
5. Wait out transitions/fades (poll the transition's state), then screenshot.
6. Stop Play, restore the snapshot, autotick off.

To reproduce resize/orientation bugs: enter Play at one size (e.g. landscape
1920×1080), then change the size while playing and read the layout values
back (`utk-exec-query`) before and after.

## 4. Verify the real asset, not a stand-in

EditMode tests that build a mock popup/scene can stay green while the real
prefab is wrong. For "the popup has one button / uses the shared frame /
text fits", open the real scene or prefab in the test and assert on it
(`ui-scene-authoring` → verification).

## 5. Budget and hand-back

Every `utk` call gets a timeout; retries cap at 3; after the cap, report what
failed with the last output. A step that needs a human (Figma not running,
Editor crashed, a modal dialog) is handed back, not looped on.

## Rules

- ✅ Load the table's skill before fighting a symptom.
- ✅ Hand edit → backport into the build script → rebuild once.
- ✅ Reopen the target scene after `run_tests`, before Play.
- ✅ Snapshot and restore any save data you touch; leave the Editor stopped,
  unpaused, autotick off.
- ❌ **NEVER** `sleep` and hope frames advanced; poll a signal.
- ❌ **NEVER** assert UI facts on a mock when the authored asset exists.

## Related Skills

- `utk-cli-core`, `utk-exec-query`, `utk-test-runner`, `utk-playmode-driving`
- `unity-scene-script` (studio) — the idempotent build script pattern
- `figma-unity-workflow`, `unity-runtime-ui-rules`
