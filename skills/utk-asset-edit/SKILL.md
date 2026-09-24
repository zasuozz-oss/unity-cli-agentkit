---
name: utk-asset-edit
description: Use when renaming, retargeting, or tweaking Unity assets stored as text (YAML) — prefab/asset renames, m_Name changes, serialized field value tweaks, GUID reference swaps, bulk find-and-replace — and you must decide between editing files directly and going through the editor with utk.
---

# Unity Asset Edits — Direct YAML vs `utk`

Unity asset files (.prefab, .unity, .asset, .mat) are YAML text, and an
asset's GUID lives in its sibling `.meta` file. Deterministic text
substitutions are faster done directly on disk — no editor round-trip, no
subagent. But Unity does not support hand-editing asset *structure*
("manually modifying Asset files is a risky operation and is not supported by
Unity" — official Unity blog), so structural changes go through `utk exec`,
and every batch of direct edits MUST be validated through the editor.

## Decision table

**Edit files directly** — scalar/text substitutions only, never structure:

| Task | How |
|---|---|
| Rename a prefab/asset file | `mv Foo.prefab Bar.prefab && mv Foo.prefab.meta Bar.prefab.meta` — ALWAYS move the `.meta` together; references point at the GUID inside it, so nothing breaks. Then update the root GameObject's `m_Name:` in the YAML. |
| Rename GameObjects inside a prefab/scene | Edit the `m_Name:` values. |
| Tweak serialized field values (numbers/bools/strings/enums) | Edit the value in place; never change keys, indentation, or block layout. |
| Retarget a reference to an **existing** asset | Edit the `guid:` (and `fileID` as it already appears for the target asset elsewhere). Read the target's `.meta` for its GUID. |
| Bulk find-and-replace across many assets | sed/Edit in one batch, limited to the scalar patterns above. |

**Use `utk`** — structural changes or editor-computed state:

| Task | Command |
|---|---|
| Add/remove components or GameObjects, create new assets, invent new fileID/GUID values | `utk exec` — batch ALL same-type operations into ONE C# snippet (never one exec per object) |
| Importer- or live-state-dependent work (import settings, postprocessors, runtime state) | `utk exec` / the matching utk command |
| Validate a batch of direct edits (**mandatory**) | `utk editor refresh` FIRST (imports your edits), then `utk console --type error`. Reserialize only after that — see below |
| Change a setting the **importer** owns (SpriteAtlas Include-in-Build, texture import mode, localization String Tables) | `utk exec` through the editor API, then grep the YAML to confirm it changed. A text edit here loses to the editor's in-memory copy |
| Change anything in the scene that is **currently open** | `utk exec` — a text edit hangs the Editor behind a modal dialog (next section) |

## Rename recipe (prefab/asset)

1. Check the target name is free: `ls Assets/.../Bar.prefab*` must not exist.
2. `mv Foo.prefab Bar.prefab && mv Foo.prefab.meta Bar.prefab.meta` — one
   command, both files, every time. A `.prefab` without its `.meta` gets a
   new GUID on import and every reference to it breaks.
3. Update `m_Name: Foo` → `m_Name: Bar` on the root GameObject in the YAML
   (the file name and the serialized name are independent).
4. Repeat for the whole batch, then validate ONCE (next section).

## Mandatory validation — once per batch, not per file

The editor only sees external file changes after a refresh (Auto Refresh is
typically off in agent-driven projects). Never report the task done with
unvalidated YAML edits:

```sh
utk editor refresh                                          # import the edits FIRST
utk console --type error                                    # import errors refresh does not report
```

`refresh` waits for the import/recompile to finish before returning, so the
console it is followed by describes *these* edits and not the previous batch.
Fix any errors before reporting done. If an edit produced a broken file,
prefer redoing that change via `utk exec` instead of patching the YAML
further.

### Never text-edit the scene that is currently open

Rewriting the open scene's `.unity` on disk makes the Editor raise **"The open
scene(s) have been modified externally — Ignore / Reload"**. That dialog runs
on the main thread and pumps its own event loop until somebody clicks it, so
every main-thread call queued behind it expires:

```
utk status          → reachable                       # the socket is served off the main thread
utk editor refresh  → Pipeline command 'recompile' timed out after 30000ms
utk exec …          → 400 Bad Request: Main thread operation timed out after 60000ms
```

The Unity process sits at ~0% CPU throughout — it is blocked, not compiling,
and no amount of waiting or retrying clears it. On macOS (with Accessibility
permission for your terminal) `utk` answers this particular dialog itself —
**Reload**, then it retries the call once and prints what it clicked; without
that permission it just names the dialog and you answer it in the Unity
window. Either way the edit still costs a 30-60s stall, so don't rely on it.

- ✅ Change the open scene through `utk exec` (`SerializedObject` /
  `EditorSceneManager.MarkSceneDirty` + `SaveOpenScenes`).
- ✅ Text edit is fine on a scene that is *not* open — check first:
  `utk exec 'return UnityEditor.SceneManagement.EditorSceneManager.GetActiveScene().path;'`
- ⚠️ **After a `git pull` that touched any `.unity`**, the same dialog is
  already waiting. **Reload** adopts the pulled file; **Ignore** keeps the
  Editor's copy and overwrites the pull on the next save — which is why
  `utk`'s automatic answer is Reload. If you had unsaved in-Editor changes to
  that scene, save them (or set `UTK_NO_AUTO_DIALOG=1`) *before* the pull.

### Dirty scenes: never let the save prompt decide

**"Scene(s) Have Been Modified — Do you want to save the changes you made in
the scenes? Save / Don't Save / Cancel"** blocks the main thread the same way.
It appears when something closes a dirty scene the interactive way: a
`File/…` menu item via `ExecuteMenuItem`,
`EditorSceneManager.SaveCurrentModifiedScenesIfUserWantsTo()`, or quitting or
reopening the project. Neither Save nor Don't Save is safe to pick blind. Save
writes the Editor's copy over the file on disk, which may be a pull. Don't Save
drops the edits.

- ✅ **Save your own edits in the same `utk exec`** that made them:
  `EditorSceneManager.MarkSceneDirty(scene); EditorSceneManager.SaveScene(scene);`.
  A scene you changed should never be left dirty between commands.
- ✅ **Check before opening or switching a scene, or entering Play:**
  `SceneManager.GetSceneAt(i).isDirty` for every open scene. Dirty from your
  own edits → save it. Dirty and not yours → stop and ask the user.
- ❌ Don't call `ExecuteMenuItem("File/…")` or
  `SaveCurrentModifiedScenesIfUserWantsTo()` from `exec`. They are the
  prompting paths.
- ⚠️ `EditorSceneManager.OpenScene(path)` does **not** prompt: it drops unsaved
  changes to the scene it replaces. That is why the `isDirty` check comes
  first.

If the prompt still appears, `utk` (on macOS, with Accessibility permission)
clicks **Cancel**, and only Cancel, and does not retry the call: whatever
raised the prompt would raise it again. The action the prompt interrupted did
not happen. Resolve the dirty scene as above, then redo the action.

### `utk reserialize` before a refresh silently reverts your edit

`reserialize` is `AssetDatabase.ForceReserializeAssets`: it writes the
**editor's in-memory copy** back over the file. If the editor has not imported
your external change yet, that in-memory copy is the *pre-edit* version, and
your edit is gone — no error, no warning, and `utk console` stays clean.

Observed failure: six `.spriteatlas` files edited with `sed` (`bindAsDefault:
1` → `0`), then reserialized. All six reverted to `1` and the device build
shipped duplicate atlas textures. The same clobber hit a localization String
Table, with a tell-tale asymmetry: only the **active** locale (loaded in
memory) was reverted, while the inactive locales survived — producing
duplicate/orphan entries that looked like a merge bug.

- ✅ Order is always: edit file → `utk editor refresh` → verify → reserialize
  only if you still need it.
- ✅ For anything the editor holds loaded or the importer owns, skip text
  edits: mutate via `utk exec` (e.g. `SpriteAtlasExtensions.SetIncludeInBuild`,
  `LocalizationEditorSettings` → `AddKey`/`AddEntry`) + `EditorUtility.SetDirty`
  + `AssetDatabase.SaveAssets()`, then grep the file to confirm.
- ❌ Never `utk reserialize` straight after a text edit.

### Prefab edits through `utk exec` need a forced re-import to survive

`PrefabUtility.LoadPrefabContents` → `SaveAsPrefabAsset` writes to disk, but
the editor keeps its stale imported copy in memory and re-serializes it back
over your file on the **next domain reload** (`utk test`, a recompile) — the
edit vanishes hours later, far from the command that made it.
`UnloadPrefabContents` alone does not fix it; it only frees the temp copy.

```csharp
var root = UnityEditor.PrefabUtility.LoadPrefabContents(path);
// …mutate root…
UnityEditor.PrefabUtility.SaveAsPrefabAsset(root, path);
UnityEditor.PrefabUtility.UnloadPrefabContents(root);          // 1. free temp copy
UnityEditor.AssetDatabase.ImportAsset(path,                    // 2. adopt disk bytes
    UnityEditor.ImportAssetOptions.ForceUpdate |
    UnityEditor.ImportAssetOptions.ForceSynchronousImport);
UnityEditor.AssetDatabase.SaveAssets();                        // 3. commit
```

Verify persistence **across a domain reload** (run `utk run_tests` or
`utk editor refresh`, then re-grep the file) — checking right after the exec
proves nothing.

## When to fall back to `utk exec`

- The substitution is not purely textual (Unity must compute or create
  something: components, new objects, new GUIDs/fileIDs).
- The same logical change appears with different YAML shapes across files
  (different serializedVersion, prefab overrides) — a text pattern would be
  guess-work.
- A previous direct edit on this file failed validation.

Run `utk` inline in your own context — do not spawn a subagent for a handful
of commands (see `utk-cli-core`).
