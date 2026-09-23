---
name: unity-ui-narrow-aspect-fit
description: "Use when Unity uGUI must fit screens narrower than the design aspect (tall phones 9:21, 9:22, 9:23 and beyond under a height-matched CanvasScaler) while wider screens stay pixel-identical — lists or grids that overflow or drop a column, side padding that looks too wide, popups/cards/currency blocks that clip, buttons or badges that stick out of shrunk cells, text wrapping early, tutorial holes and hands drifting after a layout change. Covers the clamped-fit model, a symptom→fix catalog, recycled scroll views (OSA), and a numbers-first verification recipe."
---

# Fit uGUI to Narrow Aspects Without Touching Wide Ones

A height-matched canvas gets *narrower* on every phone taller than the design
aspect. The fix is never "make the layout responsive" in general: it is a set
of clamped adjustments that are exactly zero at the design width and above, so
the screens that already look right cannot change.

A single row whose two edge-pinned groups collide (typically a top bar) is the
narrow case of `@unity-ugui-aspect-overlap`; start there for that one. This
skill is for everything else on the screen.

## 0. Read the parameters before anything else

Read these from the project (CanvasScaler on the root canvases, the design
file) or ask. Never carry numbers over from another game.

| Parameter | Where it comes from |
|---|---|
| Reference resolution `refW × refH` | CanvasScaler |
| Screen Match Mode + match value | CanvasScaler |
| Design aspect | the mockup frame (usually `refW : refH`) |
| Narrowest aspect to support | the user / target device list (e.g. 9:23) |
| "Must not change" cases | the user — e.g. "tablets keep 3 columns", "font size stays" |

Then list the affected screens and fix **one screen at a time**.

## 1. The model

With `ScaleWithScreenSize` and match = 1 (height):

```
canvasW = refH × screenAspect        // screenAspect = screenW / screenH
```

Worked example, ref 720×1600: 9:23 → 626, 9:22 → 654, 9:20 → 720, 9:16 → 900,
3:4 → 1200. For any other match value derive the formula from the scaler
(`@unity-ugui-aspect-overlap` shows Expand and 0.5); do not reuse this one.

At runtime, read the width from the canvas itself:
`((RectTransform)rootCanvas.transform).rect.width` (equivalently
`Screen.width / rootCanvas.scaleFactor` inside the game loop).

> **Editor trap.** Called from a `utk exec` or any editor tool context,
> `Screen.width` is the size of the *calling editor window*, not the Game view.
> Read `rootCanvas` rect width, or verify from play-mode state — never from
> `Screen.width` in a tool call.

### Every formula clamps at the design width

The rule users insist on: **aspects at or wider than the design stay
pixel-identical; only narrower ones are customized.** So every adjustment is
one of these two shapes, and both collapse to the original at
`w >= designW`:

```csharp
float s   = Mathf.Min(1f, w / designW);                         // uniform scale
float pad = Mathf.Lerp(narrowValue, designValue,
                       Mathf.InverseLerp(narrowW, designW, w)); // value lerp
```

`Mathf.InverseLerp` clamps to [0, 1], so `pad` is `designValue` for every
`w >= designW` and `narrowValue` for every `w <= narrowW`.

Measure at three widths, before and after every change: **narrowest**,
**design**, and **one wide** (9:16 or 3:4). Design and wide must diff to zero.

## 2. Symptom → fix

### A. Full-width lists or grids overflow or clip

Keep the design width, allow shrinking only:

- Parent: `HorizontalLayoutGroup`, `childControlWidth = true`,
  `childForceExpandWidth = false`.
- Child: `LayoutElement`, `minWidth = 0`, `preferredWidth = designW`.
- Result: `width = min(designW, parentW)`.

Gotchas:
- In a VLG/HLG the cross-axis child size is clamped to the parent rect —
  negative padding **cannot** widen a child.
- Siblings that must not take part (empty-state text, overlays) need
  `LayoutElement.ignoreLayout = true`.

### B. Side padding: lerp to a fixed narrow value

Scaling padding linearly with width barely changes anything visible. Lerp it to
a chosen narrow value instead, through one small helper in the project's common
UI folder:

```csharp
public static class NarrowScreenPadding
{
    // Canvas widths at the design aspect and at the narrowest supported one —
    // fill from the project (section 0), do not keep these example numbers.
    public const float DesignWidth = 720f;
    public const float NarrowWidth = 626f;

    public static float Lerp(Component owner, float designPad, float narrowPad)
    {
        var canvas = owner.GetComponentInParent<Canvas>().rootCanvas;
        float w = ((RectTransform)canvas.transform).rect.width;
        return Mathf.Lerp(narrowPad, designPad, Mathf.InverseLerp(NarrowWidth, DesignWidth, w));
    }
}
```

Users judge the gap to the **visible art**, not to the cell rect. If the card
art sits `N` units inside its element's rect, the padding target is
`edgeGap − N` — possibly negative. Measure the inset first (section 3).

### C. Recycled scroll views (OSA and similar)

Correctness rules for recycled cells are in `@unity-scrollview-recycling`; the
layout-specific ones:

- OSA caches `ContentPadding` at init (`Start`). Set it **before**
  `base.Start()`, or call `_InternalState.CacheScrollViewInfo()` after a
  runtime change — otherwise the change is silently ignored. Negative padding
  works.
- A grid with an inferred column count (`MaxCellsPerGroup = -1`) drops a column
  on narrow screens. Set the count explicitly; the group's HLG then shrinks the
  cells instead.
- Cell templates with fixed-width, centre-anchored children must become
  stretched (left/right offsets) so the children shrink with the cell.

### D. Fixed-size composite blocks: scale uniformly

Popups, cover cards, currency grids — anything whose internal proportions must
hold — get one uniform scale, never a relayout:

```csharp
[RequireComponent(typeof(RectTransform))]
public sealed class FitWidthScaler : MonoBehaviour
{
    [Tooltip("The block's real visible width plus the margin wanted around it, in canvas units.")]
    [SerializeField] float designWidth = 720f;

    void OnEnable() => Apply();
    void OnRectTransformDimensionsChange() => Apply();

    void Apply()
    {
        if (designWidth <= 0f) return;
        float s = Mathf.Min(1f, ((RectTransform)transform).rect.width / designWidth);
        transform.localScale = new Vector3(s, s, 1f);
    }
}
```

- It must sit on a **stretched** rect: `OnRectTransformDimensionsChange` fires
  only when the rect's size changes, and a fixed-size rect never does.
- Pivot x must be 0.5, or the block shrinks toward one side.
- `designWidth` = the block's *measured* visible width plus the desired margin.
  Measure it; do not guess.

### E. Popups whose show/hide helper tweens the panel's scale

A helper that tweens `panel.localScale` to 1 overwrites the scaler every time.

- Never put the scaler on the animated panel.
- Insert a stretched wrapper (e.g. `FitFrame`) carrying the scaler between the
  popup root and the panel — or inside the panel, reparenting its children.
- The full-screen dim/mask stays **outside** the wrapper; scaled with it, it
  leaves uncovered bands at the edges.

### F. Parts inside shrunk cells stay fixed and stick out

Buttons, price bars, badges and icons anchored to cell corners keep their size
and offset while the cell shrinks.

- Apply the cell's factor to them: `localScale = min(1, factor)` and
  `anchoredPosition = designPos × factor`, so corner offsets shrink too.
- Cache the design positions **once** — cells are recycled, and re-reading
  scaled values compounds the factor.
- Any code that re-positions a part later must multiply by the same factor.
- A button with its own press/appear scale tween: scale its **container**, not
  the button.

### G. Text wraps early, and "don't touch the font size"

- Scale the text transform by `s = min(1, factor)`, and size its stretched rect
  in *unscaled* units so it still fills the background:
  `sizeDelta.x = parentW / s + inset − parentW`, `sizeDelta.y = h / s − h`
  (`inset` = the rect's original `sizeDelta.x`). The text then wraps at the
  design width.
- Scale the adjacent icon and its offset by the same `s`, so the whole line
  matches the design exactly.
- Recompute after any height change (line-count-driven height →
  `extraLineHeight × s`).
- If init reads `lineCount` right after `Instantiate`, the width fit must run in
  `Awake`, not `Start`, or the first measurement uses the old width.

### H. Code that centres from `parent.rect.width` breaks after stretching

Once a template is stretched, `parent.rect.width` read at init is wrong — the
layout has not been computed yet.

- Replace it with anchor-based centring: `anchorMin.x = anchorMax.x = 0.5`,
  `anchoredPosition.x = 0`.
- After converting **any** template to stretched anchors, grep its scripts for
  `rect.width` and `sizeDelta` reads.

### I. Tutorials, coach marks and focus masks drift

Symptom: hole and hand positions hardcoded in canvas coordinates.

- Compute the hole from the real target: its world corners → screen point via
  the **target** canvas's camera → local point in the **overlay** canvas.
  Helpers that pass a `null` camera break for `ScreenSpace-Camera` canvases.
- Keep the designer's inset. The original hole was usually smaller than the
  card so the feather stays inside; subtract `designCardSize − designHoleSize`
  from the measured rect, or the feather lights the card's border.
- Keep the hand at the same offset from the hole centre.
- Keep the old constants as the fallback when the target is missing.
- Grep **every** tutorial view for hardcoded `SetHole` / `ShowHand` style
  values on the screens you changed.

### J. UI relative to a panel that already shrinks

When a sidebar or panel already scales by a ratio (e.g. `canvasW / designW`),
put a `FitWidthScaler` (design = the child's fixed width) on its stretched
child container instead of recomputing the ratio a second time.

## 3. Verify: numbers first, screenshots second

1. **Exact Game view resolutions**, not aspect presets: set fixed sizes with
   the `gameview_size` snippet from `@utk-playmode-driving` — design (e.g.
   1080×2400 for 9:20), narrowest (e.g. 1080×2760 for 9:23), one wide. An
   aspect-mode preset in a small window can mislead the user's own screenshots;
   when their result disagrees with yours, ask them to switch to a fixed
   resolution.
2. **Measure canvas-space edges**: `GetWorldCorners` →
   `rootCanvas.transform.InverseTransformPoint`, printing the x-range of the
   element, of its visible art, and of key children.
3. **Full-resolution screenshot** (`utk screenshot --max <height>`) plus a pixel
   scan (e.g. PIL). `1 canvas unit = Screen.width / canvasW` px. Look for the
   first dark column after the white run; thin edge lines and shadows are
   false positives.
4. **Small Game views lie by resolution.** At ~300 px wide, 2 canvas units is
   about 1 view pixel — "I see no change" can be resolution, not a bug. Read the
   runtime values before editing again.
5. **Fill every state with fake data.** Inject runtime data so every tab and
   list is populated; force hidden sections (first-time-user headers, empty
   states) active and reparent them into their runtime container to measure.
   Restore afterwards, and **never touch saved user progress**.
6. **Focus-mask shaders** may render magenta or not at all in utk screenshots.
   Verify holes by reading the mask material's centre/size vectors.

## 4. Editor gotchas

- `PrefabUtility.EditPrefabContentsScope` can throw "2 roots" right after play
  mode stops, and an exec can drop with a connection error right after a domain
  reload. Retry once, not in a loop.
- Scene objects: `EditorSceneManager.OpenScene(path, OpenSceneMode.Additive)` →
  modify → `SaveScene` → `CloseScene`. Diff afterwards; expect float-rounding
  noise in the YAML.
- Structural edits (adding components, inserting wrapper GameObjects) go through
  `utk exec` (`@utk-exec-query`). Plain field tweaks may be YAML edits
  (`@utk-asset-edit`).
- Stop play mode before `utk editor refresh`; run `utk set_autotick --enable false` at the
  end (`@utk-playmode-driving`).

## 5. Output contract

1. Parameters read (section 0) and the list of affected screens.
2. Fix one screen at a time. Reuse the project's existing helpers first; add at
   most the two generic components above (`NarrowScreenPadding`,
   `FitWidthScaler`), in the project's common UI folder.
3. Report a per-screen table, then the skipped screens:

| Screen | Element | Narrow before | Narrow after | Design before → after | Wide before → after |
|---|---|---|---|---|---|
| … | … | x-range | x-range | must be identical | must be identical |
