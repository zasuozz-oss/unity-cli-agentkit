---
name: unity-parallel-branch
description: Use when a Unity task is heavy — a second agent session wants the same project or Editor, the work holds the Editor for a long stretch, touches many scenes/prefabs/build scripts, is risky/experimental, or runs for hours — to ASK the user whether to split it onto a git branch in a separate worktree instead of hand-copying the project folder and merging by hand. Covers setup, working, merge, cleanup and pitfalls. Never creates a branch, commit or worktree without the user's yes.
---

> **Decision Protocol (CLAUDE.md):** every "May I write", approval or
> AskUserQuestion checkpoint below goes to the decision owner, not the user.
> The decision owner is the agent with the right specialty, one model tier up
> (`.claude/docs/coordination-rules.md` → Decision Owners). Log each call to
> `production/decision-log.md`. Ask the user only when they asked to be asked,
> or when a global rule requires it.

# Split heavy Unity work onto a git branch + worktree

Hand-copying the project (`Game-fog/`, no git) forces per-file "freeze lists",
hunk-by-hunk merging, a backup folder and a locked merge window. A git
worktree gives the same isolated folder, but git does the merge.

**This skill is a question, not an action.** Agents never create branches,
commits or worktrees on their own — only after the user says yes.

## 1. When to ask

Ask when ANY holds; otherwise just work in the main project (no question).

| Trigger | Why |
|---|---|
| Another agent session already uses the project/Editor | Two writers on one Editor corrupt scenes |
| Needs the Editor for a long stretch (builds, many Play captures) | Blocks everyone else |
| Touches many scenes/prefabs/build scripts | Big diff, hard to undo by hand |
| Risky or experimental (may be thrown away) | Branch delete = clean rollback |
| Multi-hour work | Checkpoints worth having |
| 3+ agents (or parallel subagents) need the Editor at once | One Editor serializes them: a lock queue and one agent's compile error stalled 11 builders for hours, while the agents with their own worktree finished in minutes |

Ask once (AskUserQuestion, or a plain question), stating the cost:
~4 GB `Library/` per worktree plus the heavy gitignored folders copied
alongside (§2.3), a second Editor, and every committed binary stays in `.git`
forever (no Git LFS). Options: split onto a branch+worktree
/ work in place / wait for the other session. Respect "work in place".

## 2. Setup (only after a yes)

1. `git status` / `git log -1` in the project. No commits yet → a baseline
   commit is needed first; ask for that yes separately (what it will contain:
   `Assets/`, `Packages/`, `ProjectSettings/`; `Library/Temp/Obj/Logs/UserSettings`
   stay ignored — check `.gitignore` before committing).
2. `git worktree add ../<Project>-<task> -b <task-branch>`
3. **Copy what git does not carry.** Folders heavier than ~100 MB (third-party
   SDKs such as `Assets/Firebase/`, `Plugins/`, `FacebookSDK/`,
   `ExternalDependencyManager/`, `TextMesh Pro/`, plus their `.meta`) are
   gitignored on purpose, so a worktree comes up without them and fails to
   compile. List them with
   `git ls-files --others --ignored --exclude-standard --directory`, then
   `cp -R` each one (with its `.meta`) into the same path in the worktree.
   Skip regenerated output (`Temp/`, `Logs/`, `obj/`, `*.csproj`, `*.sln`).
   Copy `Library/` too (Editor on the source closed or idle) — avoids a full
   reimport. Then delete `Library/Pipeline/.unity-pipeline-port` in the copy:
   it still names the source Editor's port, so `utk` in the worktree drives
   the *other* Editor until the new one writes its own.
4. Open a second Editor on the worktree folder.
5. **Verify the target before every drive**: `utk exec` →
   `Application.dataPath`. After a domain reload the two Editors' utk ports
   (7800/7801) can swap.
6. Use a per-project lock owner for the shared Editor lock
   (`unity-lock.sh acquire|touch|release|want <owner>`, if the studio has it).

## 3. Working

- Commit on the branch at checkpoints (the user said yes to the branch, so
  checkpoint commits there are covered; say so if unsure).
- Periodically `git merge main` into the branch so the final merge is small.
- Scenes are produced by idempotent `Tools/build_*.cs` (`unity-scene-script`);
  keep the **run order** in a file in the repo (e.g. `Tools/BUILD_ORDER.md`,
  last entry = the "wire" script, which must run LAST), not in chat.

## 4. Merge back

1. In the MAIN worktree, take the editor lock, make sure the tree is clean.
2. `git merge <branch>`. Code, JSON and build scripts merge 3-way.
3. `.unity`/`.prefab` conflicts that build scripts generate: take either side
   (`git checkout --ours/--theirs <file>`), then re-run the build scripts in
   the documented order. Never hand-merge scene YAML.
4. `utk editor refresh`, compile check (`utk-console-triage`), full EditMode suite
   (`utk-test-runner`). Green → release the lock.

## 5. Cleanup

Close the second Editor → `git worktree remove ../<Project>-<task>` →
`git branch -d <branch>` (ask before any `-D`). Free the ~4 GB Library.

## Pitfalls

- Port swap with two Editors — check `Application.dataPath` every time.
- Shared data OUTSIDE the repo (a workspace folder) is not isolated by the
  branch; coordinate it by message/lock as before. That includes the save
  data: both copies have the same company/product name, so they share one
  `Application.persistentDataPath` — a Play run in either one overwrites the
  other's save.
- A `Packages/manifest.json` change from the merge keeps the Editor resolving
  packages for minutes (`editor_status` times out, "Cannot connect"). Wait on
  `utk status` with a bounded loop; don't retry other commands meanwhile.
  Settings objects registered in `EditorBuildSettings` (Localization,
  Addressables) are not carried by copying files — re-assign them through
  their API (`LocalizationEditorSettings.ActiveLocalizationSettings`,
  `AddressableAssetSettingsDefaultObject.Settings`).
- Two Editors on the same folder: never.
- `Library/` is never committed or merged.
- Gitignored heavy folders are not merged either. If the branch changed one
  (an SDK update), copy it back to main by hand during the merge window and
  say so; otherwise main keeps its own copy.
- A new folder over ~100 MB goes into `.gitignore` (with its `.meta`) before
  the first commit that would include it — never commit it.
- A scene copied across by hand is lost on the next rebuild; regenerate it.

## Rules

- ✅ Ask first, with the cost stated; small tasks get no question.
- ✅ Baseline commit, branch and worktree each need the user's yes.
- ✅ Verify `Application.dataPath` before driving either Editor.
- ✅ Build order lives in a repo file; re-run it after a scene conflict.
- ✅ Full EditMode suite before the merge counts as done.
- ❌ **NEVER** branch, commit or add a worktree silently.
- ✅ After `worktree add`, copy every gitignored folder the project needs (heavy SDKs, `Library/`).
- ❌ **NEVER** copy a `.unity`/`.prefab` across by hand, or commit `Library/` or a folder over ~100 MB.
- ❌ **NEVER** open two Editors on one folder.

## Related Skills

- `unity-live-editor-loop` — the loop each Editor runs; port/target checks
- `unity-scene-script` (studio) — idempotent build scripts
- `utk-test-runner`, `utk-console-triage`, `utk-cli-core`
