---
name: unity-popup-layout
description: Use in any Unity game when carrying the game's own popup design into Unity — placing or sizing the buttons, texts, icons and close ✕ inside a popup/dialog — building a new popup, adding a row or a second button, changing copy, resizing a panel, or when a popup's text spills out of the frame, buttons are too small/too close, elements overlap, a button sits off the panel, or the ✕ touches the screen edge. Ships a runnable check (scripts/popup_layout_check.cs).
---

# Popup layout: where buttons and texts go, and how big

Popups were reworked round after round for the same few reasons: text spilling
out of the frame, content placed at fixed screen coordinates that no longer
matched the resized panel, buttons measured in the wrong space, the ✕ touching
the screen edge on tall phones. The layout itself is the game's; this skill is how to carry it
into Unity so it survives resizes, plus a check that measures the result.

Frame chrome (panel art, ribbon, ✕, fonts) comes from the game's existing
popups — `figma-unity-workflow` §1. Canvas setup, anchors and TMP roles —
`unity-ugui-layout`.

## 1. The layout comes from the game, not from this skill

This skill imposes **no layout**: no zone order, no button position, no
alignment. Where the title, art, texts, buttons and ✕ go is decided by the
game's own design source, in this order:

1. The Figma frame for this popup (`unity-figma-cli`).
2. The game's existing popups — same family, same arrangement
   (`figma-unity-workflow` §1).
3. Neither exists → ask the user. Never invent an arrangement.

What this skill adds is **how to carry that layout over so it survives**:

- Read each element's position **relative to the panel** from the design
  (offset from the panel's edge or centre), and place it that way — never at
  screen coordinates. When the panel is resized for new content, elements keep
  their place on it.
- If the panel's height changes with the content, derive it from the design's
  margins (the design's gap between the last element and the panel edge),
  clamp it to ≥ the 9-slice top + bottom border, and keep the popup where the
  design puts it on screen.
- Adding an element the design doesn't have (a second button, a new row)?
  Copy the spacing and alignment of the nearest equivalent in the game's
  popups, or ask.

## 2. Sizes

| Element | Rule | Source |
|---|---|---|
| Any tappable element (button, toggle, ✕) | hit area ≥ 48×48 dp (iOS: 44×44 pt) | [Material accessibility](https://m2.material.io/design/usability/accessibility.html), [Apple HIG accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility) |
| Space between two touch targets | ≥ 8 dp | Material accessibility (same page) |
| Text rect | inside the panel's inner area; a title between the ribbon's end caps | `figma-unity-workflow` §3 |
| Buttons, labels, texts | the sizes from the game's design; the same role keeps the same size across the game's popups | the game's Figma / existing popups |

Convert dp to canvas units on the **narrowest phone the canvas maps onto**:
`units per dp = canvas width / 360` (360 dp = the narrow Android width the
check assumes; set yours). Canvas 720 wide → 48 dp = 96 units; 1080 wide → 144.

**Measure in root-canvas units, after every parent scale.** Popups are often
built at design size and scaled (`localScale`) to fit. A button authored
420×150 inside a body scaled by 0.78 is 328×117 on the canvas — that is the size
the finger gets. Read `GetWorldCorners` and convert into the root canvas; never
trust `sizeDelta` of a scaled child.

A small ✕ is fine visually — enlarge its **hit area**, not its art: a
transparent `raycastTarget` Image (or padding on the button rect) around the icon.

## 3. Position checks (whatever the layout is)

These hold for any arrangement the game uses:

- **Inside the frame.** Texts, icons and buttons stay inside the panel art with
  a margin, unless the design puts them on the frame (ribbon title, ✕, a badge).
- **Screen edge.** Corner elements (✕, badges) keep a margin from the canvas
  edge on the **narrowest aspect** (9:20–9:23), not only at the design size.
  Seen: a corner ✕ touched the edge on 9:20 until the popup body was scaled down
  (1600/1920 → 1500/1920) to leave ~40 units.
- **No overlaps** between siblings the design keeps apart. Fix with the
  design's spacing, not by shrinking fonts.
- **Button label = button rect minus padding** (stretch anchors, offsets for an
  icon inside the button) so the label moves and fits with the button.

## 4. Text fitting (every placed text)

- Rect sized to the inner area; `enableWordWrapping` on for body, off for
  one-line titles.
- Copy can change (booster names, localisation, rewards): `enableAutoSizing`
  with `fontSizeMax` = design size and `fontSizeMin` ≈ 60 % of it. Fixed copy
  that fits at design size can stay fixed.
- A text that hits its floor is a finding — widen the rect or shorten the copy.
- Check every variant of the copy (each booster, each reward), not the first one.

## 5. Run the check — numbers, not eyeballs

`scripts/popup_layout_check.cs` (next to this file), with the popup open:
`utk exec --file popup_layout_check.cs`. Per popup it lists every button and
text with its canvas-space size and position, and flags:

| Flag | Meaning |
|---|---|
| `SMALL TARGET` | hit area under 48 dp (in canvas units) |
| `TIGHT ROW` | two buttons in a row closer than 8 dp |
| `ROW HEIGHT` | buttons in one row with different heights (ignore if the design means it) |
| `TEXT OVERFLOW` | glyphs render outside their rect (`isTextOverflowing` or glyph bounds bigger than the rect) |
| `TEXT AT FLOOR` | auto-size shrank to `fontSizeMin` |
| `OUT OF PANEL` | an element leaves the frame (minus `panelPadding`); title/✕ exempt |
| `NEAR EDGE` | an element within `edgeMargin` of the canvas edge |
| `OVERLAP` | two sibling texts/buttons intersect |
| `NO PANEL` | no frame Image found — set `panelName` |

Params at the top: `targets`, `panelName`, `minTouchDp`, `narrowWidthDp`,
`minGapDp`, `panelPadding`, `edgeMargin`. Text placement uses rendered glyph
bounds, so a wide rect holding short copy is not flagged.

Triage before fixing: popups with no single frame (buttons below a picture, a
full-screen result) report `OUT OF PANEL` for every button — set `panelName`
to the element that bounds the layout, or read those lines as expected.

Run it at the design size **and** at the narrowest aspect (9:22/9:23), for each
copy variant. In Edit mode on a scene whose popups are hidden, a test copy of the
script with the canvas-enabled check removed measures them without opening them.

## Rules

- ✅ Layout from the game's Figma / existing popups; positions kept relative
  to the panel; panel height clamped to the 9-slice borders.
- ✅ Every tappable element ≥ 48 dp hit area, ≥ 8 dp apart, measured in
  root-canvas units after parent scales.
- ✅ Same role, same size across the game's popups.
- ✅ Run `popup_layout_check.cs` at design size and narrowest aspect before "done".
- ❌ **NEVER** impose a layout (zone order, button position, alignment) the
  game's design doesn't have — follow it, or ask.
- ❌ **NEVER** place popup content at screen coordinates independent of the panel.
- ❌ **NEVER** fix an overlap or overflow by shrinking a font below its role's
  floor — move, widen, or shorten.
- ❌ **NEVER** judge a button's size from `sizeDelta` inside a scaled parent.

## Related Skills

- `figma-unity-workflow` — reuse the game's popup frame; 9-slice borders; text fit
- `unity-ugui-layout` — canvas setup, anchors, TMP roles
- `unity-ui-narrow-aspect-fit` — the aspect sweep
- `unity-layer-audit` — the popup draws and blocks input above everything else
- `unity-live-editor-loop` — capture recipe for the screenshots
