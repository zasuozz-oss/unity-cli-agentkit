---
name: unity-ugui-layout
description: "Use when building Unity uGUI layout: Canvas Scaler reference resolution, RectTransform anchors/pivot, Layout Group child rules, ContentSizeFitter constraints, TMP font sizing, UILayoutSpec constants, or safe area initial setup."
---

# uGUI Responsive Layout — Agent Rules

## How to use this skill
1. Read the Canvas Setup section and confirm values match the project.
2. Read the Element Spec section. If a reference image or design file is provided,
   extract exact values from it. If not, ask before inventing any value.
3. Use UILayoutSpec constants for every size, font, and color. Never use magic numbers.
4. Follow the DO NOT LIST at the bottom without exception.

---

## Canvas setup (confirm before touching any element)

| Setting | Value |
|---|---|
| Canvas Scaler mode | Scale With Screen Size |
| Reference resolution | [PROJECT_REF_WIDTH] × [PROJECT_REF_HEIGHT] |
| Match Width Or Height | [PROJECT_MATCH_VALUE] — see note below |

**Match = 0 (Width):** Scale driven by width. Height stretches freely.
Use for landscape games or fixed-width layouts.

**Match = 1 (Height):** Scale driven by height. Width stretches freely.
Use for portrait mobile games. `sizeDelta.x` must be 0 on stretch-anchored elements.

**Match = 0.5 (Blend):** Both axes influence scale.
Use for multi-orientation or tablet-first layouts.

> Replace `[PROJECT_REF_WIDTH]`, `[PROJECT_REF_HEIGHT]`, `[PROJECT_MATCH_VALUE]`
> with the actual project values before using this skill.

---

## Element spec — fill from reference image or design file

> When a reference image is provided: extract position, size, color, and font values
> directly from it. Do not approximate — measure or ask if uncertain.
> When no reference is provided: ask the user before setting any value.

### How to read a reference image and fill this table

For each UI element visible in the image:
- Estimate anchor preset from its position relative to screen edges
  (corner-anchored, center-anchored, or stretch).
- Estimate `sizeDelta` from its apparent size relative to the reference resolution.
- Note whether it is a fixed-size element or a stretch element.
- Record font size relative to other text elements (heading > label > body).
- Record colors using the dominant hue visible.

### Element spec table (fill one row per GameObject)

| GameObject | anchorMin | anchorMax | pivot | sizeDelta | anchoredPos | Notes |
|---|---|---|---|---|---|---|
| [ELEMENT_NAME] | (?,?) | (?,?) | (?,?) | (?, ?) | (?, ?) | [component, layout notes] |

> Add rows as needed. Group by screen region (top bar, content area, bottom bar, etc.).

---

## TMP font spec — one row per text role

> Extract font sizes from the reference image by comparing text elements visually.
> Heading text is always larger than label text; label is always larger than body.
> Never assign the same fontSize to two elements with different visual hierarchy roles.

| Role | fontSize | fontStyle | color (hex) | overflowMode | wordWrap |
|---|---|---|---|---|---|
| [HEADING_ROLE] | [SIZE] | Bold | [#HEX] | Overflow | false |
| [LABEL_ROLE] | [SIZE] | Regular | [#HEX] | Truncate | false |
| [BODY_ROLE] | [SIZE] | Regular | [#HEX] | Wrap | true |
| [ACCENT_ROLE] | [SIZE] | Bold | [#ACCENT_HEX] | Truncate | false |

> Replace role names and values with those from the actual project.
> Accent color (used for counts, highlights, CTAs) must be different from body color.

### Mandatory TMP rules (apply to every project)
- `autoSizing = false` — never enable; font jumps unpredictably on container resize.
- Text inside a Layout Group: add `LayoutElement.preferredHeight`; set `flexibleWidth=1` if stretch needed.
- Multi-line text: `enableWordWrapping=true`, `alignment=Center` or `MidlineLeft` as appropriate.
- Text inside a fixed container: `overflowMode=Truncate` or `Ellipsis`.
- Text inside a CSF-driven container: `overflowMode=Overflow`, `enableWordWrapping=true`.

---

## Layout Group rules (apply to every project)

### Use Layout Group only when:
- Number of children is dynamic at runtime (lists, grids, inventories).
- Automatic spacing between a variable number of siblings is required.
- No child needs an independent `anchoredPosition` different from its siblings.

### Never add Layout Group when:
- Children are static (fixed count known at design time).
- Any child needs its own size that differs from siblings.
- The layout is already achieved via anchors alone.
- The goal is just to center something — use anchor + pivot instead.

### When using Layout Group:
- Set `childForceExpandWidth` and `childForceExpandHeight` explicitly.
  Default is `true`, which destroys child sizes. Turn both OFF unless required.
- Add `LayoutElement` to each child that needs a specific size.
- Use `padding` instead of child `anchoredPosition` offsets.

---

## Content Size Fitter rules (apply to every project)

### Allowed:
- `verticalFit = PreferredSize` on a container whose height must grow with dynamic content.
- Must be paired with a `LayoutElement` with `maxHeight` to prevent unbounded growth.

### Never:
- `horizontalFit = PreferredSize` on anything that must respect screen width.
- CSF and Layout Group on the same GameObject — put Layout Group on parent, CSF on child.

### Pattern for scrollable dynamic list:
```
ScrollView
  └── Viewport
        └── Content  ← VerticalLayoutGroup + ContentSizeFitter(verticalFit=PreferredSize)
              ├── Item  ← LayoutElement(preferredHeight=[ITEM_HEIGHT])
              └── ...
```
Content RectTransform: `anchorMin=(0,1)`, `anchorMax=(1,1)`, `pivot=(0.5,1)`, `sizeDelta.x=0`.

---

## UILayoutSpec.cs — fill constants from project values

Create at `Assets/Scripts/UI/UILayoutSpec.cs`. Agent uses this file instead of magic numbers.

```csharp
using UnityEngine;

public static class UILayoutSpec
{
    // Canvas Scaler reference resolution
    private const float REF_W = [PROJECT_REF_WIDTH];
    private const float REF_H = [PROJECT_REF_HEIGHT];

    // Canvas match value: 0 = width, 1 = height, 0.5 = blend
    private const float MATCH = [PROJECT_MATCH_VALUE];

    // Scale factor — use to convert authored px to screen px at runtime
    public static float Scale =>
        Mathf.Lerp((float)Screen.width / REF_W, (float)Screen.height / REF_H, MATCH);

    // Convert authored px to screen px
    public static float Px(float authored) => authored * Scale;

    // -------------------------------------------------------------------
    // Font sizes — replace placeholders with values from font spec table
    // -------------------------------------------------------------------
    public const float FONT_HEADING = [HEADING_SIZE];
    public const float FONT_LABEL   = [LABEL_SIZE];
    public const float FONT_BODY    = [BODY_SIZE];
    public const float FONT_ACCENT  = [ACCENT_SIZE];
    // Add more roles as needed

    // -------------------------------------------------------------------
    // Colors — replace with actual hex values from design
    // -------------------------------------------------------------------
    public static readonly Color COLOR_PRIMARY = Hex("[#PRIMARY_HEX]");
    public static readonly Color COLOR_ACCENT  = Hex("[#ACCENT_HEX]");
    public static readonly Color COLOR_WHITE   = Color.white;
    // Add more as needed

    // -------------------------------------------------------------------
    // Authored heights (px at reference resolution)
    // Fill from element spec table — one constant per bar/panel height
    // -------------------------------------------------------------------
    public const float H_TOP_BAR    = [TOP_BAR_HEIGHT];
    public const float H_BOTTOM_BAR = [BOTTOM_BAR_HEIGHT];
    // Add more as needed

    // -------------------------------------------------------------------
    // Grid (if project has a grid)
    // -------------------------------------------------------------------
    public const int   GRID_COLS    = [GRID_COLUMN_COUNT];
    public const float GRID_SPACING = [GRID_SPACING_PX];
    public const float GRID_PADDING = [GRID_PADDING_PX];

    /// <summary>
    /// Compute square cell size to fill containerWidth with GRID_COLS columns.
    /// Call in Start() or OnRectTransformDimensionsChange().
    /// </summary>
    public static float GridCellSize(float containerWidth)
    {
        float usable = containerWidth
                       - GRID_PADDING * 2
                       - GRID_SPACING * (GRID_COLS - 1);
        return Mathf.Floor(usable / GRID_COLS);
    }

    // -------------------------------------------------------------------
    // Safe area helper
    // -------------------------------------------------------------------
    /// <summary>
    /// Apply safe area insets to a root RectTransform.
    /// Call in Start() on every screen-root panel.
    /// </summary>
    public static void ApplySafeArea(RectTransform rt)
    {
        Rect safe = Screen.safeArea;
        Vector2 size = new Vector2(Screen.width, Screen.height);
        rt.anchorMin = safe.position / size;
        rt.anchorMax = (safe.position + safe.size) / size;
    }

    private static Color Hex(string hex)
    {
        ColorUtility.TryParseHtmlString(hex, out Color c);
        return c;
    }
}
```

### Correct usage pattern
```csharp
// Font — use named constants, never raw numbers
headingTMP.fontSize = UILayoutSpec.FONT_HEADING;
labelTMP.fontSize   = UILayoutSpec.FONT_LABEL;   // never reuse FONT_HEADING

// Color — accent must differ from primary
accentTMP.color = UILayoutSpec.COLOR_ACCENT;

// Height — convert authored px at runtime
rt.sizeDelta = new Vector2(0, UILayoutSpec.Px(UILayoutSpec.H_BOTTOM_BAR));

// Width on stretch anchor — always 0
rt.anchorMin = new Vector2(0, rt.anchorMin.y);
rt.anchorMax = new Vector2(1, rt.anchorMax.y);
rt.sizeDelta = new Vector2(0, rt.sizeDelta.y);

// Grid cell size — always computed, never hardcoded
float cell = UILayoutSpec.GridCellSize(gridContainer.rect.width);
gridLayout.cellSize = new Vector2(cell, cell);

// Safe area — apply to every screen-root panel
UILayoutSpec.ApplySafeArea(GetComponent<RectTransform>());
```

---

## DO NOT LIST

| Forbidden | Reason |
|---|---|
| Multiple TMP with same fontSize for different visual roles | Destroys hierarchy — every role has its own size |
| `autoSizing = true` on any TMP | Font jumps unpredictably on resize |
| Non-zero `sizeDelta.x` on stretch-anchored elements | Breaks layout on non-reference screen widths |
| Hardcoded `cellSize` in GridLayoutGroup | Use `GridCellSize()` instead |
| Accent color used for non-accent text | Loses visual hierarchy signal |
| Layout Group added to a static layout | Overrides all child RectTransforms |
| `ContentSizeFitter.horizontalFit = PreferredSize` | Panel expands beyond screen width |
| CSF and Layout Group on the same GameObject | They fight — split across parent/child |
| `transform.localPosition` to move UI elements | Bypasses RectTransform layout system |
| `anchorMin == anchorMax == (0.5,0.5)` on non-center elements | Position drifts at other resolutions |
| Inventing sizes without a reference | Always extract from image or ask the user |

---

## Anchor quick reference

```
Top-left fixed:     anchorMin=(0,1)     anchorMax=(0,1)     pivot=(0,1)
Top-center fixed:   anchorMin=(0.5,1)   anchorMax=(0.5,1)   pivot=(0.5,1)
Top-right fixed:    anchorMin=(1,1)     anchorMax=(1,1)     pivot=(1,1)
Top-stretch:        anchorMin=(0,1)     anchorMax=(1,1)     pivot=(0.5,1)   sizeDelta.x=0
Middle-stretch:     anchorMin=(0,0.5)   anchorMax=(1,0.5)   pivot=(0.5,0.5) sizeDelta.x=0
Bottom-stretch:     anchorMin=(0,0)     anchorMax=(1,0)     pivot=(0.5,0)   sizeDelta.x=0
Bottom-left fixed:  anchorMin=(0,0)     anchorMax=(0,0)     pivot=(0,0)
Bottom-right fixed: anchorMin=(1,0)     anchorMax=(1,0)     pivot=(1,0)
Full-stretch:       anchorMin=(0,0)     anchorMax=(1,1)     pivot=(0.5,0.5) sizeDelta=(0,0)
Center-fixed:       anchorMin=(0.5,0.5) anchorMax=(0.5,0.5) pivot=(0.5,0.5)
```

---

## Related Skills
- `@unity-ui-performance` — Canvas rebuild optimization, raycast target cleanup, scroll view recycling after layout is built
- `@unity-csharp-standards` — Naming conventions and coding rules for UI scripts
