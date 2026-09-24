---
name: unity-ui-build
description: "Use in any Unity game BEFORE building or changing uGUI through `utk` — a new screen, popup, panel, HUD element, button, label or icon; moving, resizing or restyling one; applying a design or mockup — and before reporting any UI task done. The entry workflow for UI work: reuse before build, layout recipes that stay centred and undistorted, rules against the knobs that cause 'text không căn giữa / lệch', 'ảnh méo / bẹp / giãn', 'scale sai / to quá / nhỏ quá', 'tràn / bị cắt / đè nhau', and a numeric done gate (scripts/ui_audit.cs, run per aspect) plus a boxes overlay for screenshots (scripts/ui_boxes.cs)."
---

# Building uGUI That Holds — Workflow and Done Gate

An agent builds UI blind: it types anchors, sizes and pivots, then judges the
result from a screenshot. A label 6 units off-centre or an icon stretched 8%
looks fine in a downscaled PNG, and a screen checked at one aspect breaks on
the next phone. Both failures escape because "looks right" is not a
measurement.

This skill makes the check numeric and mandatory. The specialist skills
(sprite distortion, popup layout, narrow aspect, layers) hold the deep rules.
This one is the order of work and the gate every UI change passes.

## 1. Reuse before you build

- Find what the project already has **before** creating anything: the prefab
  of a sibling screen or popup, the project's button/label/panel prefabs, its
  9-slice sprites, its fonts and TMP presets. Grep prefabs for the component
  or sprite names you would otherwise recreate.
- Duplicate or variant a working sibling rather than assembling from
  `new GameObject` + `AddComponent`. An authored prefab already carries the
  right anchors, slicing, font sizes and raycast settings. A primitive-built copy
  gets each of them wrong once.
- Match the project's `CanvasScaler` (reference resolution, match). Never add a
  second scaler or a nested canvas to "fix" a size (`@unity-runtime-ui-rules`).

## 2. Recipes

Build the common elements this way. Each recipe avoids a defect class the
audit reports.

**Centred label on a button or background.** Stretch the label over its
parent and let alignment do the centring. Do not position a fixed-size text
box by hand.

```csharp
var r = label.rectTransform;
r.anchorMin = Vector2.zero; r.anchorMax = Vector2.one;   // fill the button
r.offsetMin = new Vector2(pad, 0); r.offsetMax = new Vector2(-pad, 0); // same pad both sides
label.alignment = TMPro.TextAlignmentOptions.Center;      // single line: prefer vertical Geometry
label.margin = Vector4.zero;
```

A label is off-centre when its rect is offset inside the button, its margins
are asymmetric, or vertical `Middle` centres the font's line box instead of
the glyphs. `Geometry` centres what is drawn and suits single-line labels.

**Icon that keeps its shape.** Size the rect to the sprite's aspect, or keep
`preserveAspect` on inside a fixed box. Never stretch-anchor a Simple image
over a box of a different shape.

```csharp
img.type = UnityEngine.UI.Image.Type.Simple;
img.preserveAspect = true;                                // or: rt.sizeDelta = new Vector2(h * sprite.rect.width / sprite.rect.height, h)
```

**Button or panel that resizes.** Use a 9-slice sprite (border set in the Sprite
Editor), `Image.Type.Sliced`, and a rect at least as large as its two caps.
To make caps smaller, raise `pixelsPerUnitMultiplier` instead of shrinking the
rect below the border.

**Full-width container.** Stretch horizontally with margins
(`anchorMin.x = 0, anchorMax.x = 1, offsetMin.x = m, offsetMax.x = -m`), never a
fixed width equal to the design canvas width. A fixed width tuned at one aspect
overflows on every narrower phone, and a centre-anchored fixed-width viewport
also shifts its content off-centre there. Read widths with `rect.width`:
`sizeDelta.x` is 0 or negative on a stretched rect, and code that scales by it
collapses to 0 or NaN.

**Text that can grow** (counts, prices, localized copy). Give it auto-size with
a floor (`enableAutoSizing`, `fontSizeMin` at about 70% of `fontSizeMax`) and
room in the rect for the longest expected string. Hitting the floor is a
finding, not a fix.

## 3. Never

- ❌ `localScale` to size anything. It also scales text and 9-slice caps, and a
  non-uniform scale squashes every child. Scale is for animation only, and must
  return to 1.
- ❌ `SetNativeSize()` on remote or variable-size sprites
  (`@unity-remote-image-flicker`).
- ❌ Hand-tuned `anchoredPosition` to "nudge" a label into the centre. Fix
  the anchors, margins or alignment that put it off-centre.
- ❌ A `ContentSizeFitter` on a child whose layout group controls child size. Use
  a `LayoutElement` there.
- ❌ Text-editing the YAML of the scene or prefab that is open. Build through
  `utk exec` (`@utk-asset-edit`).

## 4. The done gate: audit per element, per aspect

`scripts/ui_audit.cs` (next to this file) measures the live UI and prints one
line per defect. It reads only; it never opens or saves anything. Copy it
outside `Assets/`, set `target` to the root you changed, and run it:

```
utk exec --file <copy>/ui_audit.cs --timeout 120000
```

Run it on what is live: the open scene (Edit or Play), or the prefab in Prefab
Mode. **Run it after each element you build**, not once at the end. Fixes
compound, and one audit over a finished screen turns into a whack-a-mole of
side effects.

| Finding | Meaning | Go deeper |
|---|---|---|
| `TEXT-OFF-CENTER-X/Y` | Rendered glyphs off the centre of the label's box, or of the button it is the only graphic on | recipe "Centred label" |
| `TEXT-OVERFLOW`, `TEXT-AT-FLOOR` | Copy does not fit, or auto-size has bottomed out | `@unity-popup-layout` |
| `IMAGE-STRETCH`, `SLICED-NO-BORDER`, `SLICE-SQUASH` | Sprite drawn off its shape | `@unity-ui-sprite-distortion` |
| `SCALE-NONUNIFORM`, `SCALE-SIZING` | Sized with scale (ignore `SCALE-SIZING` on something mid-animation) | §3 |
| `FIXED-WIDTH` | Container pinned to a width near the canvas width | `@unity-ui-narrow-aspect-fit` |
| `PAST-SCREEN-EDGE`, `OFF-SCREEN`, `CLIPPED` | Content crosses the canvas edge or is cut by a mask | `@unity-ui-narrow-aspect-fit` |
| `OVERLAP` | Text or button covering another text or button | `@unity-ugui-aspect-overlap` |
| `LAYOUT-CONFLICT`, `ZERO-SIZE` | Two components own one size, or a rect collapsed | `@unity-ugui-layout` |

A finding you keep on purpose (full-bleed art that is meant to overhang, a
badge meant to overlap its card) is stated in the report with its reason. It
is never silently ignored.

**The aspect sweep.** A layout checked at one aspect is unverified. Aspect-only
defects are the ones that most often ship. Set the Game view with the
`gameview_size.cs` snippet in `@utk-playmode-driving`, then rerun the audit in
a separate `utk exec`, because the canvas resizes on the next frame, not
inside the call that changed it. Use at least three sizes:

| Size | Why |
|---|---|
| the design size (the scaler's reference aspect) | what the designer drew |
| the narrowest supported (e.g. 1080×2760, 9:23) | fixed widths overflow, text wraps early |
| one wide (e.g. 1536×2048, 3:4) | stretched rects widen, and images stretch with them |

A popup also needs its content states: the longest copy, the largest count,
every button variant. Show each state, then audit it.

## 5. Look at it, with the boxes on

Numbers catch what they measure. A screenshot catches the rest (wrong art,
wrong colour, the wrong screen). Take both:

1. `utk screenshot --max 0`, and note the `path` it prints.
2. Set that path in a copy of `scripts/ui_boxes.cs` and run it with
   `utk exec --file`. It writes `<name>_boxes.png` with every text rect (green),
   button (cyan) and image (yellow) outlined. Each text also gets a red cross
   at its glyph centre and a blue cross at its box centre. Crosses that do not
   coincide show an off-centre label that the plain PNG hides.
3. Open the `_boxes` PNG. Compare it against the design or the sibling screen
   it must match (`@figma-unity-workflow` when there is a Figma frame).

Do not change the Game view size between the screenshot and `ui_boxes`. The
boxes are mapped through the current view, and the script warns when the shapes
disagree.

## 6. Reporting done

Report a UI change as done only with all of:

- `ui_audit` output for every aspect in the sweep: clean, or each remaining
  finding listed with the reason it stays;
- the `_boxes` screenshot at the design size (and at the narrowest, if it
  changed anything);
- the states covered (long copy, empty, max count), and any not covered.

A gate that could not run (Editor in Play under the user, no Game view, a
compile error) is reported as **not verified**. It is not reported as "should
be fine".

## Related Skills
- `@unity-ugui-layout` — anchors, CanvasScaler, layout groups, TMP sizing rules.
- `@unity-popup-layout` — sizes and spacing inside popups; `popup_layout_check.cs`.
- `@unity-ui-sprite-distortion` — every way an image goes off-shape; project-wide audit.
- `@unity-ui-narrow-aspect-fit` — fitting screens narrower than the design.
- `@unity-ugui-aspect-overlap` — rows and siblings colliding on tall phones.
- `@unity-runtime-ui-rules` — runtime layout, nested canvases, sorting.
- `@unity-layer-audit` — draw order and input layers.
- `@figma-unity-workflow` — carrying a Figma design into Unity.
- `@unity-live-editor-loop` — the build → compile → Play → screenshot loop.
- `@utk-playmode-driving` — Game view size, stepping frames, screenshots in Play.
