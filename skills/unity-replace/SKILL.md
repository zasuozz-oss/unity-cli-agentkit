---
name: unity-replace
description: Use when replacing an existing sprite/icon/button image in a Unity project with re-made art (exported from Figma, AI-generated, redrawn) — "thay lại vào unity", "replace the sprite", "đổi icon/nút trong unity", a Figma hand-off of one element into a screen that is already built. Keeps the GUID so every prefab/scene reference survives; works with or without a running Editor.
---

# Replace an existing Unity sprite with new art

The whole job is: **same path, same `.meta`, new pixels at the old size.** The
GUID lives in the `.meta`, so every prefab, scene and script that references
the sprite keeps working. Every step below exists because a session got it
wrong: searched references by filename, imported the art as a new file,
changed the pixel size and stretched a 9-slice, or mixed Bash and PowerShell
until the user interrupted.

Run everything from Bash (Git Bash on Windows). The helper is
`scripts/sprite_swap.py` next to this file (needs Pillow):
`SKILL=~/.unity-cli-agentkit/skills/unity-replace` (or wherever this file lives).

## 1. Unity project path

Use a cached path first (e.g. paint-kit's `project/<game>/project.json` →
`"unity": {"project": "..."}`). Otherwise read it from the running Editor
once, then cache it:

```sh
powershell -NoProfile -c "(Get-CimInstance Win32_Process -Filter \"Name='Unity.exe'\").CommandLine" \
  | grep -oiP '(?<=-projectpath )("[^"]+"|\S+)' | tr -d '"'
```

Several Editors open → ask which one. None open → ask the user; don't guess.
All later commands run from the project root (`cd "$UNITY"`).

## 2. Find the sprite and read the old one

```sh
find Assets -iname "*<name>*.png"                     # several hits → ask, show paths
python "$SKILL/scripts/sprite_swap.py" inspect "Assets/.../old.png"
```

`inspect` prints pixel size, alpha bbox, `guid`, `spriteMode`,
`spriteBorder` (x=left y=bottom z=right w=top, px) and `maxTextureSize`.

- `spriteMode: 2` (Multiple) is a sheet of sub-sprites — overwriting it breaks
  their rects. Stop and ask.
- `maxTextureSize` smaller than the pixel size means Unity downscales on import;
  keep that in mind when judging sharpness.

## 3. Find references by GUID, never by filename

```sh
grep -rl "<guid>" Assets --include=*.prefab --include=*.unity --include=*.asset --include=*.mat
```

Note each user's `m_SizeDelta` (the slot size). This is the blast radius you
report, and it tells you whether the new art fits the slot.

## 4. Export the new art

From Figma: `figma-cli get_screenshot --node <id> --max 0` (scale 1). Check
`figma-cli list get_screenshot` for a scale option and export at
`old_px / node_px` when the old sprite was authored at 2x, so step 5 does not
upscale. Keep the file outside `Assets/` (scratch dir).

## 5. Fit to the old pixel size and overwrite in place

```sh
python "$SKILL/scripts/sprite_swap.py" fit /tmp/new.png "Assets/.../old.png" --dry-run   # read the aspect warning
python "$SKILL/scripts/sprite_swap.py" fit /tmp/new.png "Assets/.../old.png"
```

It backs up the first original (OS temp `sprite_swap_backup/`, not the
project's `Temp/`, which Unity deletes), resizes with LANCZOS, letterboxes to
keep the aspect, and writes the same path. The `.meta` is never written.

- **Aspect warning (>2%)** → the art no longer matches the slot. Ask: accept
  the letterbox, or resize the slot. Resizing the slot = edit `m_SizeDelta` of
  the `--- !u!224` RectTransform doc in that prefab/scene surgically
  (utk-asset-edit — and never text-edit the scene the Editor has open).
- **9-slice** (`spriteBorder` not all 0): same pixel size keeps the borders
  valid only if the new art has its corners where the old one did. Compare
  the two images' corners; if they moved, fix `spriteBorder` via
  `utk exec` (TextureImporter.spriteBorder + SaveAndReimport), not by hand.

## 6. Reimport and verify

```sh
utk status && utk exec 'UnityEditor.AssetDatabase.ImportAsset("Assets/.../old.png", UnityEditor.ImportAssetOptions.ForceUpdate); return "ok";'
git -C "$UNITY" diff --stat        # expect only the PNG (+ YAML lines you meant to touch)
```

- `.meta` in the diff → something re-imported it as new; restore it from git.
- Editor reachable → screenshot one screen that uses it (`utk screenshot`,
  utk-playmode-driving) and check it at the slot size.
- Editor not reachable → tell the user to focus Unity (or right-click →
  Reimport) and say plainly nothing was verified in Unity.

## Rules

- ✅ Overwrite the same path; the `.meta`/GUID is the reference.
- ✅ References by GUID grep, never by filename.
- ✅ Keep the old pixel size unless the user agrees to resize the slot.
- ❌ **NEVER** delete + re-import, `utk import` to a new name, or copy a `.meta`.
- ❌ **NEVER** overwrite a `spriteMode: 2` sheet.

## Related Skills

- `utk-asset-edit` — surgical YAML edits, open-scene rule
- `utk-asset-import` — bringing a *new* file in (not this)
- `unity-figma-cli` — building a new screen from Figma
- `unity-ui-sprite-distortion` — the swapped art looks stretched afterwards
