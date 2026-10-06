---
name: utk-editor-queue
description: Use when two or more agents share one Unity Editor and you need to compile, run tests, run a scene/asset builder, or take screenshots — or when you are about to call `utk editor refresh`, `utk run_tests`, `utk editor play`/`screenshot`, `utk open_scene`/`save_scene` and another agent may be on the same Editor. Routes the work through `utk queue submit` so compile/reload/Play cost is paid once per cycle for everyone.
---

# Shared Editor queue

One Editor serves one job at a time. The queue (`utk queue`) does not add Editors; it
removes repeated work: per cycle it runs ONE `dotnet build` gate, ONE `refresh`, ONE
the `run_tests` runs back to back (the pipeline's --filter is a substring, so they cannot merge), the scene jobs shortest-first, and ONE Play session for every
screenshot script. You get back only your own results.

## What goes through the queue — and what does not

| You want to | Do |
|---|---|
| Edit a prefab you own | Call `utk exec` directly: `LoadPrefabContents → edit → SaveAsPrefabAsset → UnloadPrefabContents → ImportAsset(ForceSynchronousImport)` in ONE snippet (utk-asset-edit). No queue. |
| Query, console, layout check on a prefab | `utk exec` / `utk batch` directly. No queue. |
| Check that C# compiles | `utk queue submit compile` |
| Run your tests | `utk queue submit test --filter <YourTestClass> [--mode playmode]` |
| Edit a scene, open another scene, run a builder that writes many assets | `utk queue submit scene --cmd 'exec --file build.cs' --est-seconds 30` or `--script run.sh` |
| Screenshot / runtime UI check | write `shots.sh` with your usual `utk exec`/`wait_for`/`simulate_pointer`/`utk screenshot` steps, print `ARTIFACT: <png>` per file, then `utk queue submit shot --script shots.sh --scene Assets/Scenes/MainMenu.unity` |

Rules:
- `submit` blocks until your result and exits with its exit code. Run it with the
  Bash tool's `run_in_background` and keep working.
- Never `utk editor refresh` / `run_tests` / `editor play` directly while others
  share the Editor — that is the queue's job. Direct `exec` is fine.
- `GATE_FAILED` = your tree does not build on plain .NET; nobody else was blocked.
  Fix, resubmit. `NO_TESTS_MATCHED` = your filter selects nothing.
- In a `shot` script: Play is already on and your scene freshly loaded
  (`UNITY_IN_PLAY=1`); do not `editor_play`/`editor_stop`. Static state from another
  agent's script may be present — need a clean domain? add `--isolated`. Static UI
  without Play: `--edit`.
- File ownership is yours to keep: two agents editing one prefab or one scene is a
  merge conflict the queue cannot prevent. Name your files on the task board.

- Submitted the wrong thing, or a job is stuck? `utk queue cancel <id>` (id from
  `utk queue status`): a queued request is withdrawn, a running job is stopped
  and reported `CANCELLED`. A job whose submitter died is stopped and reported
  `ORPHANED`; one past its `--timeout` (tests too, default 600 s) `TIMEOUT`. In both
  cases the Editor-side run is stopped as well: `cancel_tests`, and if the main
  thread is still silent 60 s later, `restart --force` → `editor wait` → autotick on.
- Hung Editor: between cycles the coordinator asks the **main thread** (a one-line
  `exec`, 20 s) — `utk status` answers from the HTTP thread even when the Editor is
  frozen. Silent for 5 min while no live job holds the lock → one forced restart.
  Silent again within 30 min of it → no restart: `utk queue status` prints `HUNG:`,
  queued jobs get `EDITOR_BLOCKED`, a human finds the cause. `UTK_NO_WATCHDOG=1` turns
  the watchdog off. Autotick stays on while the coordinator serves.
- The coordinator keeps the Editor healthy between cycles: it destroys the fonts
  Unity leaks at each reload (`utk editor gc`), and when domain reloads have
  become several times slower than at startup and nobody is queued it restarts
  the Editor (`utk editor restart`, 15-20 s on a warm project, never while playing or with
  unsaved scenes). `UTK_NO_AUTO_RESTART=1` turns the restart off.

## Inspect
`utk queue status` (pending, the running job and its deadline, the Editor lock holder
and pid, a `HUNG:` line), `utk queue stats --since 24h` (wait/hold per kind),
`~/.unity-cli-agentkit/queue/<editor>/serve.log`. The coordinator starts on the first
`submit`; `UNITY_LOCK_NAME` picks the Editor, like unity-lock.sh.
