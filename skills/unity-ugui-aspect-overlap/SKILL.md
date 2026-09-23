---
name: unity-ugui-aspect-overlap
description: "Use when Unity uGUI elements overlap, collide or đè nhau on tall/narrow phones (9:21, 9:22, 9:23 and taller, notch/punch-hole devices) but look fine at the reference aspect — typically a top bar or bottom row with groups pinned to opposite edges, under a CanvasScaler that matches height. Covers computing the canvas width per aspect, the collision width of a row, and three fixes with their trade-offs: Screen Match Mode Expand, a shrinking layout-group row, or a design change."
---

# uGUI Rows Colliding on Tall Phones

A canvas that matches **height** keeps its height and lets width follow the
screen. Every phone taller than the reference aspect therefore has *less*
canvas width than the mockup, and any row built from fixed-width pieces pinned
to opposite edges collides at a width you can compute before opening a device.
Canvas setup itself (reference resolution, match choice, anchors) is
`@unity-ugui-layout`; this skill starts when a row already overlaps.
The rest of a screen that must fit narrower aspects (lists, popups, padding,
text, tutorial holes) is `@unity-ui-narrow-aspect-fit`.

## 1. Compute the canvas width for every target aspect

With `ScaleWithScreenSize` and `Match Width Or Height = 1`:

```
canvasH = refH
canvasW = refH * (screenW / screenH)
```

Illustration with a sample reference of 720×1600 (9:20); use the project's own:

| Aspect | 9:16 | 9:20 (ref) | 9:21 | 9:22 | 9:23 | 9:24 |
|---|---|---|---|---|---|---|
| canvasW | 900 | 720 | 686 | 654 | 626 | 600 |

Anything taller than the reference loses horizontal space; wider screens gain
it. Read `refW`, `refH` and match from the project's CanvasScaler — never assume
the example's values.

## 2. Compute when the row collides

Two groups with fixed widths, one anchored to the left edge and one to the
right, collide at a width that follows from their rects:

```
minRequiredWidth = leftGroupRightEdge + rightGroupSpan + gap
```

- `leftGroupRightEdge` — distance from the canvas's left edge to the left
  group's right edge.
- `rightGroupSpan` — distance from the canvas's right edge to the right
  group's left edge.
- `gap` — the smallest spacing design will accept (0 = touching).

The row is broken on every aspect where `canvasW < minRequiredWidth`.

Illustration (sample numbers, not from any project): a left group whose right
edge sits at x≈240, and a right group that starts 419 units from the right
edge.
`240 + 419 = 659` → they collide once `canvasW < 659`, i.e. from about 9:22
(654), and 9:21 (686) survives.

Read the numbers from the Editor, not from a screenshot:

```sh
utk exec '
var rt = (RectTransform)GameObject.Find("TopBar/Left").transform;
var c = new Vector3[4]; rt.GetWorldCorners(c);
var canvas = rt.GetComponentInParent<Canvas>().rootCanvas.GetComponent<RectTransform>();
var local = canvas.InverseTransformPoint(c[2]);
return new { right = local.x + canvas.rect.width * 0.5f, canvasW = canvas.rect.width };'
```

`GetWorldCorners` returns bottom-left, top-left, top-right, bottom-right; convert
to the root canvas's local space so the answer is in canvas units, the same
units as the table above.

## 3. Pick a fix

### A. Global: `ScreenMatchMode = Expand`

The canvas never goes below the reference on either axis: narrower screens
match width, wider ones match height, so `canvasW ≥ refW` everywhere.

| Cost | Detail |
|---|---|
| UI is smaller on tall phones | scale ratio vs match-height = `aspect / refAspect` → 9% smaller at 9:22, 13% at 9:23 (ref 9:20) |
| Extra height appears | `canvasH = refW / aspect` → 1760 at 9:22, 1840 at 9:23 |
| Anchors must be right everywhere | top bar top-anchored, bottom nav bottom-anchored, the middle stretched or scrolling — anything centred in fixed space floats |
| Backgrounds | full-screen art must stretch or envelope (`AspectRatioFitter` Envelope Parent), or bands show |

It fixes every screen at once, and it changes every screen at once — screenshot
all of them before accepting it. `Match = 0.5` is not a substitute: it gives
`canvasW = sqrt(refW * refH * aspect)` (671 at 9:23 for 720×1600), which only
halves the loss and guarantees nothing.

### B. Local, no code: a row that shrinks only when it has to

Fix the one row, leave every other screen untouched. This is a deliberate
exception to `@unity-ugui-layout`'s "no Layout Group on static children" rule:
here the group is the shrink mechanism, not a positioning shortcut.

1. **Stretch the right group's container** from just past the left element to
   the right edge: `anchorMin.x = 0`, `anchorMax.x = 1`,
   `offsetMin.x = leftElementRightEdge + gap`, `offsetMax.x` = the old right
   inset (negative or 0). Keep its vertical anchors as they were.
2. **Add a `HorizontalLayoutGroup`** to it: `childControlWidth = true`,
   `childControlHeight` as the children need, `childForceExpandWidth = false`,
   `childForceExpandHeight = false`, `childAlignment = MiddleRight`, and
   `padding` / `spacing` equal to the old hand-placed gaps.
3. **Give each child a `LayoutElement`**: `preferredWidth` = its current width,
   `minWidth` smaller (the narrowest it may still read at). uGUI shrinks
   children from preferred toward min **only when space runs out**, so wide
   screens look identical to before.
4. **Let the text shrink with its pill.** A value TMP anchored to a fixed width
   on the right does not follow the pill. Change it to a horizontal stretch
   that keeps the same left/right insets, with TMP auto-size on (min/max font
   size set explicitly), so the text shrinks with the pill instead of spilling.

### C. Design change

Move an element to another row, drop a label, or abbreviate values (1.2M
instead of 1,200,000). Abbreviation needs code in the formatter, not layout —
say so rather than faking it with a smaller font.

## Gotchas (fix B)

- **Child order flips.** `HorizontalLayoutGroup` lays out in hierarchy order,
  left to right. Children hand-placed from the right edge are usually listed
  right-to-left, so the row comes out mirrored. Set
  `reverseArrangement = true` rather than reordering the hierarchy (code may
  index children).
- **Toggled children collapse.** Once a layout group owns the row, a child
  that code hides (`SetActive(false)`) is removed from the layout and its
  neighbours shift into its slot. With `MiddleRight`, hiding the item at the
  *start* (left) end is safe; hiding one in the middle moves the others. Grep
  for code that toggles these children before committing.
- **Code that positions children loses.** Anything setting `anchoredPosition`
  or `sizeDelta` on those children is overwritten on every layout rebuild. Grep
  for it and move that intent into the `LayoutElement`.
- **Prefab instances.** If the container is a prefab used in several scenes,
  apply the change as **scene overrides on the instance that collides**, not on
  the prefab. Otherwise a scene whose container has a fixed width gets its
  children squeezed by the new group.
- **Safe area is a separate problem.** The same tall devices have notches and
  punch-holes; that is a vertical inset (`Screen.safeArea`), not this
  horizontal collision. Fix it separately — see `@unity-ugui-layout`.

## 4. Verify numerically, then by eye

Set the Game view to each target aspect — the reference, 9:16, 9:21, 9:22,
9:23 and one tablet (3:4) — and for each:

1. Force the layout before measuring, or you read last frame's rects:
   `LayoutRebuilder.ForceRebuildLayoutImmediate(container)` (and
   `Canvas.ForceUpdateCanvases()`).
2. Measure the gap between neighbours in canvas units: the left group's right
   edge against the right group's first child's left edge, and each child's
   right edge against the next one's left edge. Every gap must be `≥ gap`.
3. Take a screenshot (`utk screenshot --view game`) — a positive gap can still
   hide clipped text or an icon pushed out of its pill.

The reference aspect passing proves nothing; the narrowest target is the one
that fails.
