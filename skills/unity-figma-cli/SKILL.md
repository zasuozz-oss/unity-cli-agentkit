---
name: unity-figma-cli
description: Use when a Figma design has to become Unity uGUI — reading a frame's geometry, colors, text and auto-layout with `figma-cli`, exporting its sprites, building the RectTransform hierarchy through `utk`, or checking a built screen against the design it came from.
---

# Figma → Unity uGUI

`figma-cli` reads the file the user has open in Figma Desktop; `utk` writes the
Unity scene. This skill is **only the bridge**. CLI mechanics — preflight,
`--params`, trimming, `no_cdp`/`ambiguous_tab`/`boot_failed` — live in the
`figma-cli` skill; read it first and don't restate it here.

Start with `figma-cli status`. Every failure it reports needs a human
(relaunch Figma, close a tab, save work) — report and hand back, never loop.

## Read with two tools, not one

`list_nodes` and `get_node` serialize **different** fields. Neither is a
superset, and the one you reach for first is usually the wrong one.

| You need | Tool | Coordinates |
|---|---|---|
| tree shape, `fontName`, `letterSpacing`, `hasImage` (this node needs a PNG), `renderRect` (ink bounds incl. shadow) | `list_nodes --node <frame> --depth 6` | `rect` is **absolute page** coords |
| `box`, `fills`/gradients, `cornerRadius`, `constraints`, `effects`, auto-layout (`layoutMode` `itemSpacing` `padding*`), INSTANCE `mainComponentId` | `get_node --node <id> --depth 4` | `box` is `[x,y,w,h]`, **parent-relative** |

- **`get_node` never returns the font face.** It gives `characters` and
  `fontSize` and nothing else about type; `list_nodes` is the only source of
  `fontName`/`letterSpacing`. Don't guess a face — ask.
- **Text alignment is in neither, but the data exists.** `textAlignHorizontal`
  /`textAlignVertical` are live in the Figma Plugin API; they are missing
  because the field whitelist in figma-cli's `serializeNode` drops them. That
  is a few lines in the CLI, not a permanent hole — until it lands, ask.
- Dump both to files and let the build script read them:

```sh
figma-cli list_nodes --node 2473:43677 --depth 6 > /tmp/fig/tree.json
figma-cli get_node   --node 2473:43677 --depth 4 > /tmp/fig/geo.json
```

Do not retype coordinates node by node into your context.

## The mapping

Figma measures Y **down** from the parent's top-left. Putting both anchor and
pivot at the parent's top-left is the only mapping with no per-node math:

```csharp
rt.anchorMin = rt.anchorMax = rt.pivot = new Vector2(0f, 1f);
rt.anchoredPosition = new Vector2(x, -y);   // box[0], -box[1]
rt.sizeDelta        = new Vector2(w, h);    // box[2], box[3]
```

It breaks on a rotated node — pivot (0,1) and Figma's rotation origin no longer
agree. Handle those one at a time, or carry `rotation` and a `localScale` with
them.

| Figma | Unity |
|---|---|
| `fill: "#RRGGBB"` (lone opaque SOLID, collapsed) | `ColorUtility.TryParseHtmlString` |
| `fills[0].color {r,g,b}` 0–1 + `fills[0].opacity` | `new Color(r, g, b, opacity)` — same 0–1 sRGB convention, no conversion, in either color space |
| node `opacity` < 1 on a parent | `CanvasGroup.alpha`, not a tint on each child `Image` |
| `clipsContent: true` | `RectMask2D` — without it a popup's content spills outside the frame |
| `characters`, `fontSize` | TMP text, `fontSize` 1:1 (only while the reference resolution matches the frame) |
| `letterSpacing {value, unit}` | TMP `characterSpacing` is 1/100 em — measured, not inferred (`fontSize 40` + spacing `10` = +4px per gap): `PERCENT` → value as-is, `PIXELS` → `value / fontSize * 100` |
| `constraints` (absent = `MIN/MIN`) | re-anchor after placing — table below |
| `layoutMode` + `itemSpacing` + `padding*` | Horizontal/VerticalLayoutGroup, `spacing`, `padding` |
| INSTANCE `mainComponentId` | one prefab per component id; every instance is an instance of it |
| INSTANCE `variantProperties` | prefab variant or a `Selectable` sprite state — never a fresh prefab per instance |

A `Color` is sRGB in both color spaces, so the numbers above need no conversion.
What Linear space *does* break is elsewhere: a texture imported without its sRGB
flag, and several stacked alpha layers, which composite differently than Figma
flattened them.

**Constraints → anchors** (parent `W × H`, child `x,y,w,h`):

| Figma | anchorMin.x / anchorMax.x (H axis; vertical is the same flipped) |
|---|---|
| `MIN` | 0 / 0 — left edge (vertical `MIN` = **top**, so 1 / 1) |
| `MAX` | 1 / 1 (vertical `MAX` = bottom, 0 / 0) |
| `CENTER` | 0.5 / 0.5 |
| `STRETCH` | 0 / 1, offsets = the two margins |

**Children of an auto-layout frame ignore `anchoredPosition`.** Inside a Layout
Group, apply only the size (via `LayoutElement.preferred*`) and drop the
position mapping — a Figma frame that hugs its content needs a
`ContentSizeFitter` on the Unity side, which uGUI does not infer.

## Text fails silently — check it first

- **A `TMP_FontAsset` for `fontName` must already exist in the project.** Figma
  hands you `{family, style}`; TMP needs an SDF asset someone generated. When it
  is missing, TMP falls back to the default face, the text still renders, and
  **the build reports success** — nothing anywhere says the design's font never
  loaded. Resolve every distinct `fontName` to an asset *before* building and
  stop if one is unresolved; a missing font is a question for the user, not a
  thing to substitute.
- **TMP does not wrap where Figma wrapped.** Identical width and `fontSize`
  still break the line somewhere else. It is the most common visible difference
  in a screenshot diff — check multi-line text explicitly.

## What uGUI cannot draw — export the pixels

A `GRADIENT_*` fill, an `IMAGE` fill (`hasImage: true`), `cornerRadius > 0`, a
`drop-shadow` effect, and every `VECTOR`/`BOOLEAN_OPERATION` node have no
runtime equivalent. Export them instead of rebuilding them:

```sh
figma-cli save_screenshots --params '{"items":[
  {"nodeId":"2473:43681","outputPath":"/tmp/fig/bg.png","scale":3},
  {"nodeId":"2473:43702","outputPath":"/tmp/fig/cash.png","scale":3}]}'
```

- Paths resolve from the CLI's cwd — **pass absolute paths**.
- Pick `scale` so the PNG is at least the pixel size it occupies on the
  highest-res target device (3 for a 360-wide phone frame). `--max` trims
  `get_screenshot` only; it does nothing for `save_screenshots`.
- **A node with a shadow exports bigger than its layout rect.** That is what
  `renderRect` in `list_nodes` is for — when it differs from `rect`, size the
  RectTransform from `renderRect` or the art lands offset by the blur.
- A rounded rect or a stretched panel wants a 9-sliced sprite: export it once
  and set the border in the importer, don't export one PNG per size. **The
  border is in the PNG's pixels, the `box` is in design units** — at `scale: 3`
  a 12-unit corner radius is a 36px border. Forget the multiply and the corners
  come out visibly wrong.

Then it is an ordinary import (see utk-asset-import — path, never base64):

```sh
for f in /tmp/fig/*.png; do utk import "$f" "UI/Result/$(basename "$f")"; done
utk set_import_settings --asset Assets/UI/Result/bg.png --settings '{"textureType":"Sprite"}'
utk get_import_settings --asset Assets/UI/Result/bg.png   # verify, keys are not validated for you
```

## Build the hierarchy in one pass

A screen is dozens of nodes. That is **one** `utk run_script --file build.cs`
that reads `/tmp/fig/*.json` and walks the tree — not one `utk exec` per node,
which pays a round-trip each and cannot hold a `Dictionary<string,GameObject>`
of figma-id → GameObject between calls. Keep the file outside `Assets/`
(`AgentScripts/`) and dry-run it first:

```sh
utk run_script --file AgentScripts/BuildResultScreen.cs --entry BuildResultScreen.Run --dry_run true
```

## Verify against the design, not against `ok: true`

Capture both and look:

```sh
figma-cli get_screenshot --node 2473:43677 --max 0 -o /tmp/fig/design.png
utk screenshot --max 0
```

`Read` the two PNGs. For anything the eye cannot settle — a 4px gap, a
mis-anchored element — read the geometry back as numbers instead
(utk-exec-query → "UI layout") and diff it against `box`.

## Rules

- ✅ Set the CanvasScaler reference resolution from the frame **before** placing
  anything; every px number in the design is meaningless without it.
- ✅ Re-anchor from `constraints` once the absolute layout is correct. Building
  responsive-first from a static design is how you get a screen that is wrong
  everywhere instead of wrong on one device.
- ❌ **NEVER** invent a font face, a text alignment, or a substitute for a
  missing TMP font asset. Those are the three things that render plausibly and
  wrong, with nothing failing. Ask.
- ❌ **NEVER** rebuild a gradient, a shadow or a rounded corner out of uGUI
  primitives when a 5KB sprite is one `save_screenshots` away.
- ❌ **NEVER** write to the Figma file while mapping a design into Unity. This
  direction is read-only; the user has that file open.

## Related Skills

- `figma-cli` (global) — the CLI itself: preflight, call forms, error codes
- `unity-ugui-layout` — canvas setup, anchors, TMP rules, the element spec table this skill fills in
- `utk-asset-import` — importing the exported PNGs and their importer settings
- `unity-texture-pipeline` — compression and atlas math once the sprites are in
- `utk-exec-query` — reading built UI geometry back as numbers
