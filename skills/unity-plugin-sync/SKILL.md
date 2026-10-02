---
name: unity-plugin-sync
description: Use when bringing a third-party Unity plugin/SDK from a sibling project or another machine — "ghi đè plugin bằng bản từ repo khác", "import các plugin còn thiếu", "export unitypackage cho phần git không chứa" — or when a fresh clone/other machine fails with `DllNotFoundException` (e.g. `FirebaseCppApp-*`), `CS0246 type or namespace … could not be found` for an SDK type, or compile errors right after a plugin was overwritten with a newer version.
---

# Sync a plugin between Unity projects / machines

A plugin folder is code **plus** `.meta` GUIDs **plus** native binaries that
git often doesn't carry. Copying it is easy; what breaks is a GUID the game
references, a namespace the new version renamed, or a native library that was
never in the repo.

## 1. Missing type or library — find out why before copying anything

| Symptom | Check first | Usual cause |
|---|---|---|
| `CS0246 'AuthScope' could not be found` on one machine only | `git ls-files <plugin dir>`; the file's `#if UNITY_ANDROID`; asmdef `includePlatforms`; `utk exec 'return EditorUserBuildSettings.activeBuildTarget.ToString();'` | File is in git but compiled only for one platform — that machine's build target (lives in `Library/`, not git) differs. Switch target, don't copy files |
| `DllNotFoundException: FirebaseCppApp-13_5_0` | `git check-ignore -v <file>` | `.gitignore` drops files >100 MB (`FirebaseCppApp-*.bundle/.so`) — the Editor native lib never reached this clone |
| Still `DllNotFoundException` after the file is there | `file <lib>` (arch: arm64/x86_64/universal); the `.meta` enables Editor + this OS; `head -c 200 <lib>` isn't a Git LFS pointer; `xattr -l <lib>` | Wrong OS/arch build, LFS not pulled, macOS quarantine (`xattr -dr com.apple.quarantine <dir>`), or Unity not restarted — **native plugins load only at Editor start** |

## 2. Copy from a sibling project

1. **Same version only.** Compare the plugin's version file / package name
   on both sides. Different major → treat as an upgrade (step 4).
2. **Byte-compare what both sides have** (`cmp`, `diff -rq`) before copying
   the missing files; identical shared files mean the source is a valid donor.
3. Copy with metadata: `cp -p <file> <file>.meta` — the `.meta` is the GUID.
4. **Overwriting with a different version:**
   - Diff GUIDs: for each `.meta` in the old plugin, compare `guid:` with the
     new one. For every GUID that changes, `grep -rl <guid> Assets --include=*.unity --include=*.prefab --include=*.asset`
     **outside** the plugin folder. Any hit = a game reference that will
     break → keep the old `.meta` for that file or re-link it.
   - Major versions rename namespaces (OSA 6 → 7: `Com.TheFallenGames.OSA` →
     `Com.ForbiddenByte.OSA`). Fix the game's `using` lines; don't edit the plugin.
   - Delete the old folder first (files removed upstream otherwise linger and
     collide), then copy the new one in.
5. `utk editor refresh`, then verify compile from the source of truth:
   `utk exec 'return UnityEditor.EditorUtility.scriptCompilationFailed;'` —
   the console can still show entries from before the refresh.
6. Native library added → ask the user to restart the Editor; until then a
   `DllNotFoundException` proves nothing.

## 3. Hand off what git doesn't carry (another machine)

```csharp
// utk exec — paths = every git-ignored asset the project needs (git status --ignored)
var paths = new string[] { "Assets/Firebase/Plugins/x86_64/FirebaseCppApp-13_5_0.bundle", /* … */ };
UnityEditor.AssetDatabase.ExportPackage(paths, "GitIgnoredAssets.unitypackage",
    UnityEditor.ExportPackageOptions.Recurse);
return "ok";
```

- Write the package where git ignores it (or outside the repo) — never commit it.
- Tell the user it covers only ignored files; everything else comes from git.
- If the other machine still fails, walk table 1 on *that* machine — the
  package being correct here doesn't make the receiving Editor load it.

## Rules

- ✅ Root cause first: "missing" usually means *not compiled here* or *not
  loaded yet*, not *not present*.
- ✅ Tell the user every other clone has the same gap until the ignore rule
  or LFS setup changes.
- ❌ Don't copy a plugin from a different version "to fill the gap".
- ❌ Don't edit plugin sources to fix a namespace — fix the callers.

## Related Skills

- `@utk-asset-import` — bringing a single external file in
- `@utk-cli-core` — `utk editor refresh`, Safe Mode when the copy broke compilation
- `@unity-android-build` — Gradle/AGP template diffs against a sibling project
