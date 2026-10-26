---
name: unity-ripped-project-repair
description: "Use when a Unity project recovered from a build with AssetRipper or a similar decompiler opens broken — \"missing script\" everywhere, compile errors in stub scripts, missing packages, `Unable to parse file` / `Parser Failure` YAML errors, sprites drawn as white squares, UI or particle materials rendering black or pink, animation clips that move nothing."
---

# Repairing a ripped Unity project

Fix each shared root cause once, project-wide.

1. **Back up first** (usually no git): copy touched files outside `Assets/`.
2. **Compile first.** One compile error makes *every* script "missing".
   Scripts are stubs (bodies lost): add missing `using`s, retype absent
   third-party types to `UnityEngine.Object`, add animation-event receivers
   (bodies are guesses — say so).
3. **Packages — only what's needed** in `Packages/manifest.json`:
   `com.unity.ugui`, plugins whose DLL wasn't exported (e.g.
   `com.coffee.ui-particle` by git URL), `com.unity.pipeline` for `utk`. No
   unrelated `scopedRegistries`.
4. **DLL script refs** (`m_Script` with an odd `fileID`, `type: 3`): `fileID`
   = first 4 bytes (LE) of `MD4("s\0\0\0" + Namespace + ClassName)`; remap
   matches to the package script guid, `fileID: 11500000`.
5. **Empty-sequence YAML.** The ripper writes `key:\n    []` (or `{}`);
   Unity logs `Parser Failure … Expect ':' between key and value` and
   silently drops every field after it (`m_Sprite` → white square, empty
   particle lists). Rewrite to `key: []` in all `.prefab .unity .asset .mat
   .controller .anim`, force-reimport, clear the Console (stale errors
   linger), grep `Editor.log` for `Unable to parse file`.
6. **Lost shaders** become `Standard` (`m_Shader: {fileID: 46, guid:
   0000000000000000f000000000000000, type: 0}`) → black squares in UI. Pick
   per material from its texture: transparent sprite → `UI/Default`
   (`fileID: 10770`), glow/fire → `UI/Additive` (shipped with the
   ui-particle package). A/B screenshot each; no blind mass swap.
7. **Clips animating nothing:** numeric `path:` values are CRC32 of the
   transform path — `zlib.crc32` every path in the prefab and restore names.
8. **UI particles all tiny** (ui-particle v4): raise
   `defaultViewSizeForBaking` (default 10 caps size), or a per-UIParticle
   custom view if the prefab leaves the project.
9. **Verify in Play** with screenshots. A prefab/scene open in the Editor
   can save over disk edits — close or Discard.

## Related Skills

- `unity-fx-port` — moving the repaired FX into another project
- `utk-cli-core` — connecting `utk` once the pipeline package is in
- `utk-console-triage` — reading the error flood
- `utk-playmode-driving` — capturing frames on an unfocused Editor
- `unity-ui-sprite-distortion` — sprites stretched by the original prefab
