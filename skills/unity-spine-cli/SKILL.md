---
name: unity-spine-cli
description: Use when working with Spine Editor data from the shell for a Unity game — exporting a `.spine` project or a `.skel`/`.skel.bytes` to JSON, comparing the Spine project with what Unity actually ships (missing/extra skins, items, animations), scripting a new animation or skin as JSON and importing it, rendering a preview PNG without opening the editor, or "Spine có CLI/MCP không".
---

# Spine Editor from the command line

Spine has an official CLI (the editor binary itself); there is no official
MCP — community ones wrap the same CLI. The CLI does **import, export, pack,
render, info**. Everything else (new animation, new skin) = edit the **JSON**
in a script, then import it. JSON holds the full data: bones, slots, meshes,
weights, deform keys.

## Basics

```sh
S=/Applications/Spine.app/Contents/MacOS/Spine        # macOS; `$S --help`, `--advanced`
run(){ perl -e 'alarm 300; exec @ARGV' "$S" "$@"; }   # no `timeout` on macOS; never let it hang
ls ~/Library/Application\ Support/Spine/updates       # installed editor versions
```

- **Always pass `-u <version>`** = the version the Unity runtime uses
  (`spine-unity` package / `skeleton.spine` in an exported JSON). Without it
  the launcher picks the newest, possibly a beta — a project saved by 4.3
  doesn't open in 4.2 again.
- Work on **copies under `/tmp`**, never on the `.spine` the user has open.
- Don't `kill` a running editor (unsaved work). `osascript -e 'tell
  application "Spine" to quit'`; `User canceled (-128)` = a save prompt —
  ask the user to quit it.

| Task | Command |
|---|---|
| Project/skeleton summary (counts of bones, slots, skins, animations) | `run -u 4.2.43 -i X.spine` |
| Project → JSON | `run -u 4.2.43 -i X.spine -o out -e json` |
| Unity binary → JSON (to diff what ships) | `cp Model.skel.bytes m/Model.skel; run -u 4.2.43 -i m/Model.skel -o out -e json` |
| JSON → new project | `run -u 4.2.43 -i new.json -o new.spine -r` |
| Export with settings (nonessential, binary, PNG) | `run -u 4.2.43 -i X.spine -o out -e settings.json` |

Settings JSON templates: `EsotericSoftware/spine-runtimes` →
`examples/export/` (`json.json`, `binary.json`, `png-*.json`); fetch with
`gh api repos/EsotericSoftware/spine-runtimes/contents/examples/export/json.json --jq .content | base64 -d`.
Set `packAtlas: null` when you only want data.

## Compare the Spine project with Unity

1. Export both to JSON (project with `-e json`; each Unity `.skel.bytes` via
   the binary → JSON row).
2. Diff in Python per section: `bones`, `slots`, `ik`/`physics` (should be
   equal and in order), then `skins` by name and `animations` by name.
3. Check attachment paths against the `img/` folder; case and numbering
   mismatches are most of the "missing" noise.
4. Unity has items the project lacks → Unity was exported from a **newer**
   project. Don't re-export over it; find the newer `.spine` first
   (`mdfind -name .spine`, sort by mtime).

## Script a new animation

- Read the existing animations first: which bones they key, duration/FPS,
  whether the face/eyes change by attachment. Match them.
- Generate keys in Python (sampled curves, ~15 keys/s, an envelope that is 0
  at both ends for a seamless loop), add to `animations` in the exported
  JSON, import with `-r` into a `/tmp` project.
- Verify by **rendering**, not by reading JSON: PNG settings from the
  template with `animation`, `skin`, `fps`, `scale`; on macOS set
  `"msaa": 0` (render fails otherwise). A `MISSING` attachment in the render
  = an attachment path not found under the images folder.
- Ship to Unity: export binary with nonessential off, then **round-trip
  check** — convert the new `.skel` back to JSON and assert old animations,
  bones, slots and skins are identical to the current Unity file, and only
  the new animation differs. Then the code side: the animation name in
  `SetAnimation`.

## Rules

- ✅ `-u` pinned to the runtime version, every call.
- ✅ Bounded runs (`perl alarm`), output to a file, grep for `error|complete`.
- ✅ Round-trip compare before replacing any `.skel.bytes` in Unity.
- ❌ Don't open/save the user's project with a newer editor version.
- ❌ Don't hand-edit `.skel` binaries — go through JSON.

## Related Skills

- `@unity-spine-ui` — runtime: SkeletonGraphic, mixing, skins in uGUI
- `@unity-ui-motion-tuning` — tuning the animation once it is in game
