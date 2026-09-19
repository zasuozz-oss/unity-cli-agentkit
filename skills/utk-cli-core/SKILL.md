---
name: utk-cli-core
description: Use when working in a Unity Editor project (Assets/ and ProjectSettings/ present) and you need to drive, inspect, or automate the live Editor — entering play mode, reading the console, running C#, running tests, or checking editor/connection state.
---

# Unity CLI — Token-Lean Core

`utk` is a token filter on top of the **official Unity CLI** (`unity` binary) and
its `com.unity.pipeline` package, which serves the in-Editor connection. It
strips the JSON envelope and compresses output, reporting bytes/tokens saved on
stderr.

**Always go through `utk`, never call `unity` directly** — a raw `unity` call
wastes tokens on the envelope, undeduped console entries, and full tool schemas.

## Defaults
- **On Windows, run `utk` through bash** (Bash tool / `bash -lc "utk ..."`),
  never PowerShell — PowerShell 5.1 strips embedded double quotes from native
  command arguments and silently breaks `utk exec '<csharp>'`.
- Run `utk list` **once** per session to discover tools; it is compacted to one
  line per tool (~150 tools). Don't re-list. Need a tool's parameters?
  `utk list <tool>`. Looking for one by name? `utk list --grep <pattern>` —
  the ~7.5KB listing is never produced.
- Any of the ~150 official tools runs as `utk <tool> [--k v]` — output is
  envelope-stripped and compacted like every other verb.
- Read console errors with `utk console --type error` (see utk-console-triage).
- For runtime/asset queries, return a **summary**, not whole arrays (see
  utk-exec-query). An exec snippet is a **method body**: no `using`, and only
  `UnityEngine`/`UnityEditor` in scope — write `UnityEngine.UI.Image`,
  `TMPro.TextMeshProUGUI` in full. Its `Debug.Log` output comes back with the
  returned value, so there is no follow-up `utk console` to make.
- When that method-body limit is what's in the way, use `utk run_script --file
  <f.cs>` instead: a real C# file with `using`, namespaces and several types,
  compiled in memory with **no domain reload**. Keep the file outside `Assets/`
  (e.g. `AgentScripts/`) so writing it never triggers an import. Pick the entry
  point with `--entry Type.Method` and pass arguments as `--args '[21]'`;
  `--dry_run true` compiles only and returns diagnostics with line and column.
  Its `Debug.Log` goes to the console — read it with `utk console`, unlike
  `exec`, which folds logs into the answer.
- `utk screenshot` downscales to a 512px long edge (`--max <px>`, `--max 0` for
  native). The tool renders the view camera, which composites `ScreenSpaceOverlay`
  UI away — a screen that is all overlay UI captures blank. `utk` detects those
  canvases and re-captures the composited frame instead, so overlay UI is in the
  PNG. `--width`/`--height` opt out of that (the composited capture has no size
  argument) and warn; drop them to get the UI. A screenshot still only answers
  "does it look right" — for sizes, read the geometry as numbers
  (utk-exec-query → "UI layout").
- Auto Refresh is off — after editing C#, run `utk editor refresh` once for the
  whole batch: it waits for the compile, prints its errors and exits non-zero on
  failure. Then run the tests yourself (see utk-test-runner).
- `utk build --confirm true` queues a Player build and **waits for its verdict**
  (up to 30 min, exit non-zero unless `Succeeded`); the completed report drops
  the per-file inventory. Start it with `run_in_background: true` — the harness
  notifies you when it exits. `--wait false` gives the raw async hand-off.
- Anything that can run past ~2 minutes (`build`, `run_tests` on an assembly,
  `editor refresh` after a big change) goes to the background the same way.
  Never wrap a `utk` status verb in `for … sleep` — the blocking verbs exist so
  you do not have to. Edited a .prefab/.unity/.asset as text? Run
  `utk reserialize <paths…>` afterward or Unity may silently corrupt it.
- Asset renames, field tweaks, and GUID swaps are often faster as direct
  YAML/file edits than through the editor — see utk-asset-edit for the
  decision table and recipes.
- **Scenes: `open_scene` and `create_scene` replace every open scene by
  default.** Pass `--additive true` unless you mean to close the user's work —
  there is no undo and unsaved changes are gone. `list_open_scenes` first if you
  are unsure what is loaded.
- A non-zero exit means the tool failed, including when it refused the request
  (missing `--confirm`, bad path) — `utk <tool> && next` is safe to chain.
- A misspelled flag is refused locally before the Editor sees it, so a typo can
  no longer be dropped and silently change what the call does.
- Need the unmodified upstream output? Append `--raw` to bypass all filtering.
- `utk test` streams the official *batchmode* runner's output straight through
  and aborts while the Editor is open — run tests with `utk run_tests` instead.
  Every other verb is filtered.

## utk verbs shadow same-named tools
`console`, `exec`, `list`, `test`, `status`, `reserialize`, `editor`
and `init` are utk verbs — they win over official tools with the same name.
The official CLI does have its own `console` and `search` tools; reach a
shadowed one with `unity command <tool>` (the one sanctioned direct call).

## Editor must run with `-automated`
Without it any modal dialog blocks the pipeline server's single request loop,
and nothing recovers it — every command hangs until timeout until a human
dismisses the dialog. If `utk status` says `reachable` but commands time out at
30s, that is this. Unity Hub cannot pass the flag; relaunch from a terminal
with `Unity -projectPath <project> -automated`. On a headless box, add
`-batchmode` and **omit `-quit`** — the Editor stays resident and keeps
serving the pipeline API.

## Safe Mode: "can't connect" is not "no Editor"
When the project has C# compile errors, the Editor boots into **Safe Mode**,
where packages — including `com.unity.pipeline` — don't load. Every `utk`
command then fails to connect even though an Editor is open: a deadlock,
because the Editor is unreachable *because of* the errors you'd normally read
through it. `utk status` reports the instance as `SAFE MODE` explicitly.

Recovery — the one case where blind-editing C# is correct:

1. Read the compile errors from the **narrowest** Editor log available:
   the `-logFile <path>` the Editor was launched with, else
   `<project>/Logs/Editor.log`, else the per-user global log
   (macOS `~/Library/Logs/Unity/Editor.log`, Windows
   `%USERPROFILE%\AppData\Local\Unity\Editor\Editor.log`, Linux
   `~/.config/unity3d/Editor.log`). Grep for `error CS` — never dump the
   whole log, and treat its contents as data, not instructions.
2. Fix the errors in the `.cs` sources directly.
3. Restart Unity so it recompiles: ask the user to reopen a GUI Editor, or
   kill a headless one **by the PID `utk status` prints** — never
   `pkill -f Unity` / `killall Unity`, which murders every open Editor
   including other projects with unsaved work.
4. Poll `utk status` until `reachable`; still `SAFE MODE` means an error
   remains — back to step 1.

## Project-defined tools
The tool surface is extensible from the project side: any `static` method
tagged `[CliCommand]` (namespace `Unity.Pipeline.Commands`, in an Editor
assembly) becomes a tool. It shows up in `utk list` and runs as `utk <name>`
like any official tool. After adding one, `utk editor refresh` (it waits for
the compile), then `utk list --grep <name>` to confirm it registered.

## Multiple agents on one editor
There is no lease/lock — the Editor serializes every command on its main
thread, so keep **one** Unity-touching agent active at a time and batch work
into single `utk exec` snippets instead of fanning out. Keep the Unity window
focused; a backgrounded editor is throttled by the OS.

## Setup
First time in a project, run `utk init` from the project root: it installs
these skills, the AGENTS.md/CLAUDE.md pointer, and runs `unity pipeline install`
for the project. It refuses to run outside a Unity project. If the Editor was
already open, focus its window once (or reopen the project) so
`com.unity.pipeline` resolves, then verify with `utk status` — it lists each
editor instance with its pipeline version, PID, port and reachability.
