# unity-cli-agentkit (`utk`)

`utk` is a **token filter** for the official [Unity CLI](https://unity.com/blog/meet-the-unity-cli),
optimized for AI agents. The official `unity` binary is the transport; `utk`
maps its own verbs onto it, strips the JSON envelope, and compresses the
payload — console entries deduped, stacktraces trimmed, the ~150-tool listing
folded to one line per tool. `utk init` also installs the bundled **skills** and
a guidance block in `CLAUDE.md`/`AGENTS.md` per Unity project.

- **Loss-safe** filtering: any payload that isn't the expected shape is passed
  through verbatim — it never silently drops data.
- `--raw` on any command bypasses filtering and streams `unity` unchanged.
- Exit codes and stderr are preserved; bytes/tokens saved are printed to **stderr**.

## Requirements

Install these **before** `utk init`, in this order.

| # | Requirement | Verify with |
|---|---|---|
| 1 | **Unity Hub** ≥ 3.19.5 | — |
| 2 | **Unity Editor** 6.0 or later | `unity editors list` |
| 3 | **Official Unity CLI** (beta) on `PATH` | `unity --version` → `1.0.0-beta.10` |
| 4 | **`com.unity.pipeline`** ≥ `0.6.0-exp.1` in the project | `utk status` |
| 5 | **Go ≥ 1.26** — only to build `utk` from source | `go version` |

### 3. Official Unity CLI

The `unity` binary is `utk`'s transport; without it every verb fails. It is
self-contained — it does not need Unity Hub or a full Editor install to run.

**Windows** (PowerShell):

```powershell
$env:UNITY_CLI_CHANNEL='beta'; irm https://public-cdn.cloud.unity3d.com/hub/prod/cli/install.ps1 | iex
```

**macOS / Linux**:

```sh
curl -fsSL https://public-cdn.cloud.unity3d.com/hub/prod/cli/install.sh | UNITY_CLI_CHANNEL=beta bash
```

The two installers are **not** interchangeable: `install.sh` aborts on
MinGW/MSYS/Cygwin and redirects you to `install.ps1`, so the bash one-liner
fails in Git Bash on Windows. Both add the install dir to your user `PATH` —
**reopen the terminal** afterwards, or `unity` still reads as "not recognized".

If the binary lives somewhere off `PATH`, point `utk` at it directly with
`UTK_UNITY_BIN=/path/to/unity` instead.

### 4. `com.unity.pipeline`

`utk init` runs `unity pipeline install` for you; run it by hand if init
reported it as skipped. Controlling an Editor instance through the pipeline
requires a signed-in CLI — run `unity auth login` (browser flow) if `utk status`
stays `UNREACHABLE` with the Editor open.

**CLI `1.0.0-beta.9` and newer require pipeline `0.6.0-exp.1` — the pairing is
not optional.** They send the command line for the package to bind server-side,
and 0.5.0 cannot parse it, so every Editor-driving verb (`utk exec`, `utk
console`, `utk editor …`) fails at once with:

```
COMMAND_FAILED: This Editor's Unity Pipeline package is too old to parse
command lines. Update it to 0.6.0-exp.1 or newer with unity pipeline upgrade.
```

`utk list` keeps working — it reads the CLI's own tool catalogue and never
reaches the Editor — which makes the break look narrower than it is. Fix it
with `unity pipeline upgrade` in the project, then **focus the Unity window**
so the Package Manager resolves the new version: `utk status` keeps reporting
the old one until it does.

From CLI `1.0.0-beta.10` a *bare, argument-free* command against such an Editor
runs instead of refusing — there is nothing to bind, so the CLI retries once
against the older request shape. Anything carrying arguments still refuses with
the message above, which is the point: binding arguments without the Editor's
schema is the silent-wrong-parameter bug the refusal exists to prevent.

**`unity pipeline upgrade` was broken on CLI `1.0.0-beta.9`** — it reports
`alreadyLatest: true` while the project sits on an older version, even though
`unity pipeline list-versions` marks the newer one `Latest`. beta.10's release
notes do not claim a fix, so keep installing the version explicitly until you
see `upgrade` actually move a project:

```sh
unity pipeline list-versions                              # find the Latest row
unity pipeline install --package-version <that version>
```

`0.7.0-exp.1` is supported too. It removed `get_console_logs`, so `utk console`
drives the `console` tool instead — that one exists on `0.6.0` as well, which is
why both versions work. Two notes if you take 0.7.0: an assembly definition that
referenced `Unity.Pipeline` only to reach the code-reload attributes must now
also reference `Unity.Pipeline.Attributes` (scripts outside an asmdef need no
change), and the runtime server no longer ships in a non-development Player
build unless you define `ENABLE_RUNTIME_PIPELINE`.

From CLI `1.0.0-beta.10`, a command that needs an OAuth token starts a resident
auth broker on demand, so a `unity --internal-auth-broker-serve` process can
outlive the verb that spawned it by up to two minutes of inactivity before
exiting on its own. It holds no more than the credentials the CLI already read
directly. `UNITY_NO_AUTH_BROKER=1` restores the previous keyring-direct
behavior.

### Launch the Editor with `-automated`

The server handles requests one at a time and dispatches each to the main
thread, so a modal dialog — nothing else needed, just a dialog waiting for a
click — blocks the request in flight and every one behind it. `-automated` is
what stops those dialogs appearing.

Pipeline `0.6.0-exp.1` changed how this surfaces. The startup warning it used
to print (*"Editor is not in automated mode. Modal Pop up might break
continuous command workflow."*) is gone from the console — it is reported as
`info` on the instance descriptor instead — and a dialog no longer hangs you
blind: a main-thread command is refused with a busy status while one is open,
and status polling names the dialog as the blocker (that endpoint needs a
recent enough Editor build). Before 0.6.0 the port stayed open, the package's
own watchdog saw a healthy listener, and commands simply hung until someone
dismissed the dialog.

Unity Hub cannot pass the flag, so launch from a terminal instead:

```sh
"/Applications/Unity/Hub/Editor/<version>/Unity.app/Contents/MacOS/Unity" \
  -projectPath /path/to/project -automated       # macOS
```

### Disable focus throttling (one-time, per machine)

In Unity open **Edit → Preferences → General → Interaction Mode → No
Throttling** (on macOS this is **Unity → Settings…**). By default, when the
Unity window loses focus the Editor slows its update loop, so a `utk` command
may **hang until timeout** — and an agent can mistake that for Unity being
broken. This is a machine setting, not agent behavior.

### Telemetry in the transport

`utk` shells out to `unity` for every verb, so both layers below it report:

- **CLI `1.0.0-beta.9` and newer** send one `cli` event per run, so total CLI usage can
  be counted including runs that never see a consent prompt. It carries which
  Unity tool sent it, the CLI version, whether this looks like CI, and whether
  a consent prompt was possible — no user, machine, or session identifier (a
  fresh random UUID per invocation, never persisted). Opt out with
  `UNITY_NO_CLI_INVOKED_TELEMETRY=1`.
- **`com.unity.pipeline` `0.6.0-exp.1`** logs every `eval`/`eval_file` call —
  which is every `utk exec` — to `<project>/Library/Pipeline/eval-usage.jsonl`:
  timing, success, statement shape, and an API fingerprint of top-level member
  accesses. Editor-only and local-first, and **the snippet source is not
  stored** unless *Store Eval Source* is on. Toggle with *Eval Telemetry
  Enabled*. The package also emits `Pipeline_SessionStarted` /
  `Pipeline_CommandExecuted` / `Pipeline_SessionStopped` editor analytics.

## Installation

`utk` itself is a single Go binary. The fastest way is the auto-build script
(requires Go ≥ 1.26):

```sh
./setup-cli.sh
export PATH="$HOME/.unity-cli-agentkit/bin:$PATH"   # if not already on PATH
```

**On Windows**, run the script from Git Bash. It builds `utk.exe` (PowerShell
will not run an extension-less binary) and prints the one-time PowerShell
command that puts it on your user PATH — shell profiles like `.bashrc` are
invisible to PowerShell, so the script does not touch them there.

The script builds the binary and installs the exact layout `utk init` expects:

```
<home>/bin/utk[.exe]      # binary
<home>/skills/            # bundled skills
```

`<home>` defaults to `~/.unity-cli-agentkit`; override it with the
`UNITY_CLI_AGENTKIT_HOME` variable. `utk init` locates the directories above via
that same variable (defaulting to the binary's parent dir), so keep the layout
intact.

Manual build (if you'd rather not use the script):

```sh
go build -o ~/.unity-cli-agentkit/bin/utk ./cmd/utk
cp -r skills ~/.unity-cli-agentkit/
```

## Quick start

| Task | Command |
|---|---|
| Read console errors (grouped by root cause, stacktraces trimmed) | `utk console --type error [--limit N]` |
| Discover available tools (run once) | `utk list` — one line per tool |
| Search for a tool by name/description | `utk list --grep <pattern>` |
| Inspect one tool's parameters | `utk list <tool>` |
| Run C# in the Editor, get compact JSON + its `Debug.Log` | `utk exec 'return ...;'` |
| Run a longer snippet from a file | `utk exec --file <snippet.cs>` |
| Screenshot the game view (downscaled to 512px) | `utk screenshot [--max <px>]` |
| Recompile after editing C# (Auto Refresh OFF) | `utk editor refresh` — waits for the compile and reports its errors; exits non-zero when it failed |
| Run tests (Editor open) | `utk run_tests --mode editor\|playmode --filter <name>` — exits non-zero if any test fails |
| Run tests (Editor closed) | `utk test` — batchmode runner, unfiltered passthrough; aborts if the project is open |
| Build the Player | `utk build --confirm true [--outputPath P]` — waits for the verdict (≤30 min), exits non-zero unless `Succeeded`; run it in the background. `--wait false` keeps the async hand-off |
| Import an external file (image, audio, model…) into `Assets/` | `utk import <file> [dest]` — copies + imports in one call; never base64 through `exec` |
| Reserialize assets the editor has already imported | `utk reserialize <paths…>` (after `utk editor refresh`, never before — see below) |
| List editor instances / check the connection | `utk status` |
| Run a whole C# file (`using`, namespaces, several types) | `utk run_script --file <f.cs> [--entry Type.Method] [--args '[…]']` — compiles in memory, no domain reload; `--dry_run true` compiles only |
| Any other official tool | `utk <tool> [--k v]` (any of the ~150) |
| Get raw, unfiltered output | add `--raw` to any command |

**Go through `utk`, not `unity` directly** — a raw `unity` call returns the full
JSON envelope with no dedup or trimming. `utk` verbs (`console`, `exec`, `list`,
`test`, `status`, `import`, `reserialize`, `editor`, `init`) shadow same-named
official tools; reach a shadowed one with `unity command <tool>`.

Everything except `utk test` (a passthrough to the official runner) and `--raw`
goes through the filter, and `utk` prints the bytes/tokens saved to **stderr**
(never mixed into stdout).

A few tips:
- An **`exec` snippet is a method body**: `using` is invalid there and only
  `UnityEngine`/`UnityEditor` are in scope — write anything else in full
  (`UnityEngine.UI.Image`, `TMPro.TextMeshProUGUI`). A failed compile repeats
  this hint.
- `utk exec` folds the snippet's own `Debug.Log` into its answer, so a snippet
  that reports through the console does not need a follow-up `utk console`.
  `UTK_NO_EXEC_LOGS=1` skips the extra local call.
- `utk exec` collapses JSON whitespace (loss-safe); use `--truncate N` to cut
  long arrays (marked `"…+K more"`) when you deliberately accept some loss.
- Return a **count + a few samples** instead of whole arrays to save tokens at
  the source.
- `utk console --type` takes one value: `all`, `log`, `warning` or `error`.
- Compile errors in `utk console` are grouped by root cause (severity + CS code
  + text, with the locations rolled up) and reported **first error first** — the
  tool itself answers newest-first, so a cascade otherwise buries its own cause.
  Warnings are labelled as warnings and never counted as errors.
- `utk screenshot` downscales the PNG to a 512px long edge (`--max 0` keeps the
  native size). A scene with active `ScreenSpaceOverlay` canvases is re-captured
  through `ScreenCapture.CaptureScreenshot` so the overlay UI is in the frame —
  the tool's own camera render composites it away and returns a blank PNG. That
  path has no size argument, so `--width`/`--height` still capture without it.
- A non-zero exit means the call failed, including when the tool refused it
  (missing `--confirm`, bad path) — the official CLI reports those as exit 0.
- A flag the tool does not accept is refused locally (exit 2) instead of being
  dropped silently upstream; the schema is cached under the kit home and
  refreshed when the `unity` binary changes. `UTK_NO_FLAG_CHECK=1` skips it.
- Commands issued during a domain reload are retried for up to 8s instead of
  failing, so you do not have to poll `editor_status` after a recompile.
- `utk editor refresh` blocks until the recompile it triggered finishes (up to
  5 minutes) and answers with `recompile_status`'s final report — the errors
  included — instead of the "started" hand-off. A failed compile exits non-zero,
  so `utk editor refresh && utk run_tests …` stops at a broken build.
- After editing an asset as text, run `utk editor refresh` **before**
  `utk reserialize`. Reserialize writes the editor's in-memory copy back to
  disk, so on a file the editor has not re-imported yet it silently reverts
  your edit — no error, and `utk console` stays clean.
- `utk build` polls `build_status` for up to 30 minutes and answers with the
  BuildReport's verdict — minus its per-file inventory, which for a 1 GB build
  is hundreds of KB (`--raw` for all of it).
- `utk run_tests` runs the suite asynchronously and polls `test_status` for up
  to 10 minutes: `unity command` itself waits only 30s by default, and a
  synchronous run needs the suite's duration guessed up front — a wrong guess
  reports failure while the Editor keeps running it.
- `utk exec --timeout <ms>` (default 60000) is the snippet's whole budget: utk
  routes it to both the CLI transport (seconds) and the eval tool itself
  (milliseconds, after `--`). com.unity.pipeline 0.5.0 started enforcing the
  tool's own 5000ms default, which the CLI's `--timeout` alone cannot raise.
- `open_scene`/`create_scene` **replace every open scene** unless you pass
  `--additive true`. There is no undo.

## Install skills into a project (`utk init`)

Run it from the root of a Unity project (needs `Assets/` and `ProjectSettings/`):

```sh
utk init              # install skills + CLAUDE.md/AGENTS.md guidance + pipeline package
utk init --uninstall  # remove them
```

`init` will:
- Copy each kit skill **individually** into `.claude/skills/` (for Claude
  Code) and `.agents/skills/` (for Codex/Antigravity). Each project gets its own
  self-contained copy, so it never points back into the central store. Each
  install writes a `.utk-skills.json` manifest next to the copies and only ever
  creates or removes the entries listed in it, so skills already in those
  folders are left untouched — even one whose name looks like a kit skill.
  (Updating a kit skill means re-running `utk init` to refresh the copies.)
- Run `unity pipeline install` for the project (best-effort, bounded timeout) —
  a failure is reported and init continues.
- Upsert a **sentinel-delimited** guidance block into both `CLAUDE.md` (for
  Claude Code) and `AGENTS.md` (for Codex/Antigravity) — idempotent, never
  touching content outside the block.

> **If the Editor is already open**, a freshly added package does not resolve on
> its own: focus the Unity window once (or reopen the project), then verify with
> `utk status`.

`utk init --uninstall` reverses exactly this: it removes the skills listed in
the manifest and the managed block, preserving your other skills and any
content outside the block. It does **not** uninstall `com.unity.pipeline` —
remove that yourself with the official CLI if you want it gone.

Bundled skills — the `utk-*` ones cover **driving** the Editor:

| Skill | When to use |
|---|---|
| `utk-cli-core` | Driving/inspecting the Editor with `utk` (start here) |
| `utk-console-triage` | Reading, triaging, and fixing console errors |
| `utk-exec-query` | Inspecting/changing runtime state via `utk exec` |
| `utk-test-runner` | Verifying compilation + running EditMode/PlayMode tests |
| `utk-asset-edit` | Editing assets as text (renames, field tweaks, GUID swaps) vs going through the editor |
| `utk-asset-import` | Bringing external files (images, audio, models) into the project by path — never base64 |

…and 29 advisory skills, vendored from
[agentic-unity-skills](https://github.com/zasuozz-oss/antigravity-unity-skills),
cover **what to write**. They carry their own activation descriptions, so an
agent picks them up by topic:

| Group | Skills |
|---|---|
| `unity-*` | `csharp-standards`, `ugui-layout`, `ui-performance`, `async-patterns`, `dotween-safety`, `event-safety`, `addressables`, `asset-audit`, `editor-tools`, `editmode-tests`, `android-build`, `social-auth`, `telemetry-analytics`, `spine-ui`, `panel-navigation`, `scrollview-recycling`, `startup-loading`, `bug-regression-workflow`, `texture-pipeline` |
| `unity-urp-*` / `unity-3d-*` | `urp-setup`, `urp-renderer-feature`, `shader-authoring`, `3d-lighting`, `3d-rendering-performance`, `3d-model-pipeline` |
| `unity-qa-*` | `parser`, `generator`, `scorer`, `verifier` |

Installing them through `utk init` replaces `ag-unity init` — running both
installs the same skill names twice over, so pick one.

## Migration from utk v1

v1 vendored its own Go engine plus a C# connector package. Both are gone — the
official CLI and `com.unity.pipeline` replace them. In each project that used
v1, re-run `utk init` to install the pipeline package, then focus/reopen the
Editor and check `utk status`.

`utk` no longer removes the old connector for you. Delete
`Packages/com.youngwoocho02.unity-cli-connector` by hand if it is still there —
leaving it alongside `com.unity.pipeline` runs two HTTP servers inside the
Editor.
