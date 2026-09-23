---
name: figma-unity-workflow
description: Use when turning a Figma design into Unity UI for any game — a new screen or popup, "redo the UI from Figma", a popup that must match the existing ones, a stretched/distorted panel, text spilling out of a frame, or board/world content that must look clipped by a UI frame. The process layer over `unity-figma-cli`; covers tool discovery, reusing authored components, 9-slice sizing, text fit and world-space clipping.
---

# Figma → Unity: the workflow that avoids redo rounds

`unity-figma-cli` is the mechanics (export, mapping, importer, distortion
audit). This skill is the **order of work** and the mistakes that each cost a
full user-correction round in real sessions. Load `unity-figma-cli` first; if
the project has no copy, read `~/.unity-cli-agentkit/skills/unity-figma-cli/SKILL.md`.

## 0. Discover the tooling before saying anything is blocked

Before telling the user "I can't reach Figma" or "you need to run plugin X":

1. `figma-cli status` (and `figma-cli list`) — the CLI exports straight from
   Figma Desktop over CDP; **no Figma plugin run is needed** (`unity_export`).
2. Look for an importer in the project (`FigmaHeadlessImporter`, an
   `Assets/*Figma*` folder, `Tools/*figma*`).
3. Only a `status` failure is a real blocker — report it and hand back.

Never infer the workflow from a README or a script header comment alone.
Seen: an agent read an importer README, told the user to run a uGUI bridge
plugin by hand, and the user had to repeat the request before `unity_export`
was found — a whole wasted round and a false "can't do it".

## 1. Reuse an authored component before designing a new one

A new popup/screen in an existing game belongs to the game's design family.
Before drawing anything:

- List the already-imported frames/prefabs (settings, level complete, shop…)
  and pick the one whose chrome fits: panel, title ribbon, close button,
  primary button, text style.
- Build the new popup **by instantiating that frame** in the build script and
  replacing only its content (drop the old rows, keep panel/ribbon/✕/fonts).
- No match in Figma and no authored frame fits → ask the user; do not invent a
  card style, palette or font. Inventing one is "creating a design system", and
  the user will reject it.

Seen: two new popups built as a self-designed card were rejected ("reuse the
settings popup, don't create a design system") and rebuilt from scratch.

## 2. 9-slice: measure the border, then set it on the importer

- Borders default to `(0,0,0,0)` on import. A reused rounded panel must get its
  `spriteBorder` set explicitly (`utk-asset-import` / importer settings) before
  it can resize without warping.
- Border insets ≥ the **widest decorative edge measured on the source PNG**:
  rounded corner **plus** glow/shadow bleed, per side. Measure it (alpha scan or
  zoomed screenshot), never eyeball it. Seen: insets 60/50 px on art whose
  corner+glow was ~80/90 px — corners visibly warped, found by the user after
  "done". Fixed at 95/100 px.
- Clamp the panel's minimum size so top and bottom borders never overlap
  (min height ≥ top + bottom inset).
- Verify at the **user's real device resolution** as well as the design size.

## 3. Text in a reused or resized frame must be made to fit

Borrowed layouts were sized for the old copy. For every TMP text you place:

- Size the text rect to the **inner safe area** (panel width minus margins;
  a ribbon title between the ribbon's end caps), not to the panel width.
- `enableAutoSizing = true`, `fontSizeMax` = design size, `fontSizeMin` ≈ 60 %
  of it; `enableWordWrapping = true` for body text, off for one-line titles.
- Break lines by hand when a single word would be orphaned.
- When editing one-line C# with trailing comments, check you didn't comment out
  the statement after it (seen: `fontSizeMax` silently commented out).

Screenshot every variant of the copy (all intro texts, all titles), not one.

## 4. World-space content cannot be clipped by a UI Mask

A board/sprite/mesh rendered by a camera is not a UI Graphic — `Mask` and
`RectMask2D` do nothing to it. Options, cheapest first:

1. Let the content overflow slightly and draw an **opaque rim overlay** (the
   frame's border as its own sprite) on a canvas sorted above the content.
   Works only when the surrounding background is opaque/flat.
2. A stencil shader on the content + a stencil-writing frame, when the
   background behind is a gradient/image.

Decide this before the first build; "edge to edge, no seams" requests took
three correction rounds when the agent kept resizing the frame instead.

## 5. Verify like the user will

- Numbers first (`utk-exec-query`: rect sizes, text `isTextOverflowing`,
  `preferredWidth` vs rect width), then screenshots.
- Aspect sweep including the narrow end: 9:16, 9:19.5, 9:20, 9:21, 9:22, 9:23
  and a tablet 3:4 (`unity-ui-narrow-aspect-fit`). A layout proven only up to
  9:20 shipped a P1 clipping bug at 9:22/9:23.
- Compare side by side with the Figma frame and with the existing popup you
  reused — same chrome, same margins.

## Rules

- ✅ `figma-cli status` + project importer search before any "blocked" claim.
- ✅ New popup = instance of an existing authored frame + new content.
- ✅ 9-slice insets from a measured corner+glow width; importer border set.
- ✅ Every placed text: inner-area rect + auto-size with a floor.
- ❌ **NEVER** invent a card style, colour or font for a game that already has
  a design family. Ask.
- ❌ **NEVER** report UI "done" from the design resolution alone.

## Related Skills

- `unity-figma-cli` — export, mapping, importer, distortion audit (read first)
- `figma-cli` — the CLI itself
- `unity-ugui-layout` — canvas setup, TMP rules, HUD scaling
- `unity-ui-narrow-aspect-fit` — the narrow-aspect sweep and fixes
- `utk-asset-import` — sprite importer settings (borders)
- `unity-runtime-ui-rules` — sorting, nested canvases, reactive layout
- `unity-live-editor-loop` — building/verifying in the open Editor
- `unity-popup-layout` — where buttons/texts go inside a popup, sizes, and the layout check
