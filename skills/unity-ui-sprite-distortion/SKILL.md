---
name: unity-ui-sprite-distortion
description: "Use in any Unity game BEFORE touching a uGUI image that looks distorted — a fill/progress/loading bar whose rounded ends warp, squash or turn into a flat sliver at low progress, a bar that shows full before resetting, an icon, button, chip, pill or badge that is stretched, squashed, oval, blurry or 'méo / bẹp / giãn / bị kéo / bị scale' — and ALWAYS after a build script, Figma import or layout change assigns sprites or sizes. Covers Simple vs Sliced vs Filled, 9-slice borders, preserveAspect, layout groups that resize icons, progress bars driven by width, and ships a runnable audit (scripts/sprite_distortion_audit.cs)."
---

# Sprite distortion: bars and icons keep their shape

The same few causes came back as "méo" round after round — a Home chip row,
then "most popups in the game", then a tray fill bar — each fixed by eye on
the one screen the user sent. They are mechanical: an `Image` whose rect has a
different shape from its sprite, drawn in a mode that stretches. **Measure
first, fix the class, re-run the audit.**

## 1. Pick the Image mode from what the art must do

| Art | Rect vs sprite | Mode |
|---|---|---|
| icon, avatar, logo, gem, character | any | `Simple` + `preserveAspect = true` |
| panel, button, pill, bar, chip meant to stretch | different | `Sliced`, border measured from the pixels |
| anything | same aspect (±4%) | `Simple` |
| radial timer, cooldown wedge | square | `Filled` Radial360 |
| no sprite of that shape exists | — | export one, or ask — **never borrow another element's sprite** |

Seen: popups reused the round booster button sprite in 320×120 rects (2.6×
too wide); a square icon background reused on a wide chip. Both "fixed" on
one popup and reappeared on the next — the audit found all of them at once.

## 2. 9-slice: the border is the fix, and it has a minimum size

- `Sliced` with `sprite.border == 0` **stretches exactly like `Simple`** — the
  mode alone does nothing. Measure the cap from the pixels (a rounded end of
  a 20 px-tall pill needs ≈10 px left/right) and set it on the importer
  (`spriteBorder` in the `.meta`, or `set_import_settings`).
- A sliced rect narrower than `border.left + border.right` (or shorter than
  top + bottom), in canvas units, **squashes the caps**. Canvas units =
  pixels ÷ (`pixelsPerUnitMultiplier` × spritePPU/100). Raise
  `pixelsPerUnitMultiplier` to shrink the caps, or keep the rect larger.
- Caps taller than the rect (a pill cap drawn at 100 px inside a 79 px chip)
  squash vertically — same rule, other axis.

## 3. Fill / progress bars

Choose the technique before writing code — each has one failure:

| Technique | Ends | Fails when |
|---|---|---|
| `Sliced` fill, width = full × progress | rounded at every length | width < height → caps squash into a sliver. **Clamp `width ≥ height`** (tiny progress shows a dot) or hide it at 0 |
| full-size `Sliced` fill moved inside a `Mask`/`RectMask2D` shaped like the track | rounded, clipped by the track | the mask graphic isn't the track shape |
| `Filled` Horizontal, `fillAmount = progress` | leading edge cut **flat** | the art has a rounded end — the cut shows. Also stretches like `Simple`: sprite must match the rect's aspect |
| `localScale.x = progress` | ❌ never | always squashes the caps and any child text/icon |

Rules:
- Fill anchored left (`anchorMin.x = anchorMax.x = 0`, pivot x = 0), same
  height and vertical anchor as the track. Drive `sizeDelta.x`, not scale.
- **The bar is not a child of anything that animates.** A fill under a
  bouncing gem/body moves and scales with it — reparent it under the cell
  root, don't counter-offset.
- **Author the scene in the state the code starts from.** A fill authored
  at full width shows full for a frame before the script resets it
  ("hiện full rồi mới chạy lại"). Author it empty, or disabled.
- Check at **0, a tiny value (0.02), 0.5 and 1** — the bug lives at the small end.
  Pin a regression test: fill type, `sprite.border.x/z > 0`, and width ≥ height
  at progress 0.02.

## 4. Icons

- `preserveAspect` on every icon `Image`, including ones whose sprite is
  swapped at runtime to a different shape (reward icons, avatars from a URL).
- A `HorizontalLayoutGroup`/`VerticalLayoutGroup` with `childControlWidth/Height`
  (or a `GridLayoutGroup` cell size) **sets the icon's rect** — the icon looks
  fine in the prefab and stretched in the row. Put the icon in a child under
  a plain container, or give it `preserveAspect`/`AspectRatioFitter`.
- Stretch anchors (min ≠ max) make the icon follow its parent's shape.
- Non-uniform `localScale` (x ≠ y) distorts everything below it — usually a
  leftover from a tween or an import script.
- Blurry "méo": `maxTextureSize` smaller than the source PNG (`SHRUNK` below).

## 5. Run the audit — numbers, not eyeballs

`scripts/sprite_distortion_audit.cs` (next to this file). Copy it outside
`Assets/` and run `utk exec --file <path> --timeout 300000`. It scans prefabs
under `ROOT` plus the scenes already open (it never opens a scene — for others,
ask, or open them additive and close them after). In Play
mode it also sees runtime sizes, so set bars to a tiny progress first.

| Flag | Means | Fix |
|---|---|---|
| `STRETCH xN` (+ driver) | `Simple` image N× off its sprite's aspect; the note says what sets the size | §1: `preserveAspect`, 9-slice, or the right sprite; §4 for layout-driven |
| `SLICED-NO-BORDER` | `Sliced` with a zero border | §2: measure and set the border |
| `SLICE-SQUASH` | rect narrower/shorter than its two caps | §2/§3: `pixelsPerUnitMultiplier`, or clamp the bar width |
| `FILLED-STRETCH xN` | `Filled` image off its sprite's aspect | right-shaped sprite or `preserveAspect`; for bars see §3 |
| `SCALE (x,y,z)` | non-uniform localScale | reset to uniform; animate size, not scale |
| `SHRUNK src WxH` | texture imported below source size | raise `maxTextureSize` |
| `CANVAS` | not a fault — the scaler settings, for context | — |

Judge each hit: `STRETCH` on a flat-colour background meant to fill the
screen is expected; on anything with rounded ends, an outline or a drawn
shape (a bar fill, a pill, a button) it is the bug.

When to run: after any Figma import or UI build script, after a sprite swap
or layout change, and **before reporting any "méo" fix done** — on the whole
project, not only the screen the user sent.

## Rules

- ✅ Fix the class: when one image is "méo", run the audit and fix every hit
  of the same cause in the same pass.
- ✅ Verify a bar at progress 0, 0.02, 0.5, 1 with a cropped, zoomed screenshot.
- ✅ Every progress bar has a test pinning its type, border and minimum width.
- ❌ **NEVER** stretch a `Simple` icon to "fit" — `preserveAspect` or a new sprite.
- ❌ **NEVER** set a sprite to `Sliced` without checking its border is non-zero.
- ❌ **NEVER** drive progress with `localScale`.
- ❌ **NEVER** reuse another element's sprite for a different shape.

## Related Skills

- `unity-ugui-layout` — canvas setup; "Sprites keep their aspect"
- `unity-figma-cli` — exporting the right-shaped sprite; importer settings
- `figma-unity-workflow` — measuring 9-slice borders from the design
- `unity-live-editor-loop` — capture recipe for the cropped screenshots
- `utk-exec-query` — running the audit
