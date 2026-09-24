## Unity CLI Agent Kit

This project runs a Unity Editor you can control from the command line with
`utk`, a thin wrapper around the official Unity CLI (`unity`) and its
`com.unity.pipeline` in-Editor connection. Reach for `utk <cmd>` whenever a
task touches the Editor — play mode, console errors, running C# inside the
Editor, running tests. Output is filtered to save tokens; exit codes and
stderr stay intact.

Run all Unity operations through `utk`, never call `unity` directly — `utk`
strips the JSON envelope and compresses output (console dedup, list
one-liner); calling `unity` raw wastes tokens. Exception: a tool shadowed by a
utk verb (`console`, `list`, `test`, `status`) can be reached via
`unity command <tool>` if ever needed.

Run `utk` commands **directly in your own context** — do NOT spawn a subagent
just to run `utk`. A subagent starts cold and re-derives project context
(minutes), while a `utk` call takes ~1–2s. Delegate to a **single** subagent
only when the Unity workload is genuinely bulky AND independent — never for a
handful of commands. Codex runs `utk` directly; for commands that contact the
live Editor (e.g. `utk console`, `utk editor refresh`, `utk run_tests`), pass
`sandbox_permissions: "require_escalated"` with a finite timeout, otherwise
they fail to reach the pipeline server.

Hard rules (full details live in the skills listed below):

- **On Windows, run `utk` only through bash** (Bash tool / `bash -lc`), never
  PowerShell — it silently corrupts quotes in `utk exec '<csharp>'`. Wrap the
  snippet in single quotes, use double quotes inside the C#. Past a line or two,
  write the snippet to a file and use `utk exec --file <f.cs>` instead of
  `utk exec "$(cat f.cs)"`.
- **An exec snippet is a method body:** `using` is invalid there, and only
  `UnityEngine`/`UnityEditor` are in scope — write everything else in full
  (`UnityEngine.UI.Image`, `TMPro.TextMeshProUGUI`). Its `Debug.Log` output is
  returned with the value, so no follow-up `utk console` is needed.
- **Auto Refresh is off:** after editing C#, run `utk editor refresh` once for
  the whole batch. It waits for the compile and answers with its errors (exit
  non-zero when it failed) — no `recompile_status` poll, no follow-up
  `utk console`. Fix until it exits clean before reporting the work done.
- **Batch, don't fan out:** the editor runs every command serialized on one
  main thread, and each `utk` call costs a model round-trip. One `utk exec`
  snippet that loops beats N per-object calls; use at most ONE Unity-touching
  (sub)agent at a time; keep the Unity window focused (a backgrounded editor
  is throttled).
- **Round-trips are the cost, not tool time.** Measured over two weeks of
  sessions: tools ran for 2.7h, the model spent 23h between tool calls (~22s
  each), and a quarter of all tool turns were lone `grep`/`sed`/`cat`/Read
  calls in runs of 4–30. So: "where is X / who calls X / how does X work" →
  `codegraph_explore` first when the project has a `.codegraph/` (it returns
  the source AND the callers AND the prefabs/scenes that reference the script
  in one call); several greps → ONE Bash; more than 3 files to survey → one
  `Explore` subagent, not 10 turns; never re-Read a file you just edited
  (Edit fails loudly when it misses); 3+ edits to one file → one Write or one
  patch.
- **Long runs go to the background, never into a poll loop.** `utk run_tests`
  on a whole assembly, `utk build`, `utk editor refresh` after a large change
  can outlive the Bash tool's default 120s and get backgrounded anyway — so
  start them with `run_in_background: true` and wait for the harness to notify
  you. Never write `for … sleep … utk <status>` loops: `refresh`, `run_tests`
  and `build` already block until the result and exit non-zero on failure.
- **Deterministic asset edits are faster as direct YAML/file edits:** rename =
  `mv` the asset AND its `.meta` together (references point at the GUID inside
  the `.meta`); `m_Name:` renames; field value tweaks; `guid:` swaps to
  existing assets. Structural changes (add/remove components/GameObjects, new
  fileID/GUID) must go through `utk exec`. After every batch of direct edits,
  validate ONCE: `utk editor refresh`, then `utk console --type error`. Never
  run `utk reserialize` on a file the editor has not imported yet — it writes
  the editor's stale in-memory copy back over your edit, silently. See the
  `utk-asset-edit` skill.

Common tasks:

- Read / triage Editor errors → `utk console [--type error] [--limit N]` (grouped by root cause, first error first)
- Discover available tools (run once) → `utk list`; inspect one tool's schema → `utk list <tool>`; search → `utk list --grep <pattern>`
- Inspect or change live state, run C# → `utk exec '<csharp>'` / `utk exec --file <f.cs>` (return a count/summary, not whole arrays)
- C# too big for a method body (needs `using`, namespaces, several types) → `utk run_script --file <f.cs> [--entry Type.Method] [--args '[…]']` — compiles in memory, no domain reload. Put the file OUTSIDE `Assets/` (e.g. `AgentScripts/`) so writing it never triggers an import. `--dry_run true` compiles only and returns diagnostics with line/column. Unlike `exec`, its `Debug.Log` lands in the console, not the returned value
- Look at the game view → `utk screenshot` (512px; re-captures composited when the scene has ScreenSpaceOverlay UI)
- Verify edited C# compiles → `utk editor refresh` (blocks until the compile ends, prints its errors, exits non-zero on failure)
- Run tests → `utk run_tests --mode editor|playmode --filter <name>` (exits non-zero on failures). `utk test` is the batchmode runner and aborts while the Editor is open — only use it with the Editor closed
- Control/inspect play mode → `utk editor refresh|play|pause|stop|status`
- Build the Player → `utk build --confirm true [--outputPath P]` in the background — it waits for the verdict (≤30 min) and exits non-zero unless `Succeeded`; the report comes back without the per-file inventory (`--raw` for all of it)
- Import an external file (image/texture/audio/model) into `Assets/` → `utk import <file> [dest]` — copies + imports in one call and returns the GUID. NEVER push file bytes as base64 through `utk exec`; get the file on disk first (download/decode outside Unity), then import by path. Tune importer settings afterwards with `utk set_import_settings`
- Validate an asset edited as text (.prefab/.unity/.asset) → `utk editor refresh`, then `utk console --type error`. `utk reserialize <path>` only for a file the editor has already imported — on an unimported one it overwrites your edit with the stale in-memory copy
- Check the editor connection → `utk status`
- Any other official tool not covered above → `utk <tool> [--k v]` (any of the ~150 official Unity CLI tools)
- Full, unfiltered output for any command → add `--raw`

For deeper, task-specific guidance, see these skills:

- `utk-cli-core` — driving and inspecting the Editor with `utk` (start here)
- `utk-console-triage` — reading and triaging console errors and warnings
- `utk-exec-query` — inspecting or changing live state via `utk exec`
- `utk-test-runner` — verifying C# compiles and running EditMode/PlayMode tests
- `utk-playmode-driving` — driving Play mode unattended (unfocused Editor, waiting on a state, screenshots)
- `utk-asset-edit` — editing assets as text (renames, field tweaks, GUID swaps) vs going through the editor
- `utk-asset-import` — bringing external files (images, audio, models) into the project by path, never base64

The seven above cover *driving* the Editor. `utk init` also installs advisory
Unity skills covering *what to write* — they activate from their own
descriptions, so consult them by topic rather than listing them here:

- `unity-*` — C# standards, uGUI layout, UI performance, async/UniTask, DOTween safety, UI motion tuning, event safety, Addressables, asset audit, editor tooling, EditMode tests, Android builds, social auth, telemetry, Spine UI, panel navigation, scrollview recycling, startup/loading, texture pipeline, narrow-aspect fit & row overlap on tall phones, popup queue & one-time popups, first-open image flicker & loading indicators, IAP purchase & ownership, server-driven time-based feature testing, bug-regression workflow (reproduce before fixing), spam-click bugs, popup layout, layer/draw-order audit, sprite distortion, UI build workflow & numeric done gate
- `unity-urp-*` / `unity-3d-*` — URP asset & rendering path, Render Graph renderer
  features, shader authoring & SRP Batcher, 3D lighting/lightmaps/APV, 3D
  rendering performance, model import pipeline
- `unity-qa-*` — QA parser, generator, scorer, verifier

**Building or changing any UI → load `unity-ui-build` first.** It sets the
order of work and the done gate: `scripts/ui_audit.cs` clean at the design,
narrowest and one wide aspect, plus a `scripts/ui_boxes.cs` screenshot.

**UI bug → load the skill BEFORE the first edit.** These classes were fixed
by eye, reported done, and came back — each skill ships a script that finds
the whole class in one call. The user often writes in Vietnamese:

| The report says | Load | Run before "done" |
|---|---|---|
| layer, sorting, "nằm dưới", "bị che", "bị đè", "đè lên", FX/pieces over a popup, tooltip/toast under the board, taps through a popup | `unity-layer-audit` (+ `unity-runtime-ui-rules`) | `scripts/layer_audit.cs` with the popup/overlay showing |
| "méo", "bẹp", "giãn", "bị kéo", "bị scale", stretched icon/button/chip, fill/progress/loading bar ends warped | `unity-ui-sprite-distortion` | `scripts/sprite_distortion_audit.cs`, whole project |
| buttons/texts inside a popup: size, spacing, text spilling, ✕ at the edge | `unity-popup-layout` | `scripts/popup_layout_check.cs` |
| "không căn giữa", "lệch", text off-centre on a button/label, "scale sai", "to quá / nhỏ quá", "tràn", "bị cắt" | `unity-ui-build` | `scripts/ui_audit.cs`, each aspect of the sweep |
| overlap only on tall/narrow phones (9:20+) | `unity-ui-narrow-aspect-fit` / `unity-ugui-aspect-overlap` | the aspect sweep |

Report a fix done only with the script's output and a screenshot of the
state the user described; a change deferred because the Editor is in Play
is reported as not done.

### Unity Verification

Verify every C# edit through the **utk CLI** — compile-check and Test Runner as
in the Hard rules above. If `utk` is unavailable or no Editor is connected
(`utk status` fails / reports `not responding`), SKIP verification entirely and
say so. Exception: `utk status` reporting `SAFE MODE` means the Editor booted
with compile errors — fix them in the `.cs` sources (read them from
`<project>/Logs/Editor.log`) and have the user restart Unity; see the
utk-cli-core skill's Safe Mode loop.

**Never** fallback to Unity batchmode commands, `dotnet build`, or launching
another Unity Editor instance.

### UI Button Wiring (NEVER wire onClick in script code)

**NEVER** wire UI Button handlers at runtime in C# scripts. Patterns like the following are FORBIDDEN:

```csharp
// ❌ FORBIDDEN — do not write this
if (closeButton != null)
{
    closeButton.onClick.RemoveAllListeners();
    closeButton.onClick.AddListener(Close);
}
```

This also applies to any `onClick.AddListener(...)`, `onClick.RemoveAllListeners()`, `onClick.RemoveListener(...)`, or equivalent runtime wiring in `Awake`/`OnEnable`/`Start` for UI Buttons.

**Instead, bind the handler as a persistent listener on the Button itself**, using one of:

1. **Edit the scene/prefab YAML directly** — add the target method to the Button's `m_OnClick.m_PersistentCalls.m_Calls` (set `m_Target` to the component's `fileID`, `m_MethodName` to the handler, `m_Mode` to match the signature, and `m_CallState: 2`).
2. **Use editor tooling** (e.g. `utk exec` running `UnityEventTools.AddPersistentListener` / `SerializedObject` edits) to register the handler on the Button's `onClick`, then save the asset.

After wiring, verify the binding exists in the serialized asset (the YAML shows the persistent call), not just that the code compiles.
