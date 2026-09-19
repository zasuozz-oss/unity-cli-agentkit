---
name: unity-ui-performance
description: "Use when optimizing Unity UI runtime performance: Canvas rebuild spikes, draw call reduction, raycast target cleanup, scroll view recycling, tab/panel animation races, or Canvas split strategy."
---

# UI Optimization

## Overview
Comprehensive UI optimization covering **rendering performance** (canvas splits, draw calls, raycast targets), **state safety** (animation race conditions, resource pairing, flicker prevention), and **responsive design** (safe area, multi-device, Canvas Scaler).

## When to Use
- Use when UI causes frame drops or stutters
- Use when Profiler shows Canvas.BuildBatch spikes
- Use when draw calls are high from UI
- Use when UI gets stuck in wrong state after rapid interaction
- Use when animations fight (flickering, snapping)
- Use when loading spinners never disappear
- Use when tab/panel transitions overlap or break
- Use for multi-platform / multi-device UI adaptation
- Use for safe area handling (notches)

## Performance Best Practices
- ✅ Split canvases by update frequency (static HUD vs dynamic damage numbers)
- ✅ Disable `raycastTarget` on non-interactive elements (Labels, Images)
- ✅ Use object pooling for scroll view items — never instantiate/destroy
- ✅ Cache component references in `Awake()`
- ❌ **NEVER** animate UI transforms on a canvas with many static children
- ❌ **NEVER** enable raycastTarget on decorative elements

## State Safety Principles
- ✅ **Resource Pairing**: Every `ShowLoading()` must have `HideLoading()` on ALL exit paths (use `finally`)
- ✅ **Last-One-Wins**: New animation requests cancel previous ones
- ✅ **Immediate State**: Critical state changes happen immediately, not in OnComplete callbacks
- ✅ **Kill Before Reset**: `DOTween.Kill(target)` before manual property changes
- ❌ **NEVER** rely on `OnComplete` for critical state (tween can be killed)
- ❌ **NEVER** trigger full UI rebuild for empty data updates

## Responsive Design Rules
- ✅ Use percentage and flex units — never hardcode pixel positions
- ✅ Handle `Screen.safeArea` for notches on mobile
- ✅ Test on multiple resolutions and aspect ratios
- ✅ Use min/max constraints for layout stability
- ❌ **NEVER** assume fixed aspect ratio
- ❌ **NEVER** ignore safe areas on mobile

## Few-Shot Examples

### Example 1: Canvas Split by Update Frequency
**User**: "My HUD causes rebuilds every frame."

**Agent**:
```csharp
public class HUDManager : MonoBehaviour
{
    [Header("Static Canvas — never rebuilt")]
    [SerializeField] private Canvas _staticCanvas;

    [Header("HUD Canvas — rebuilt on events only")]
    [SerializeField] private Canvas _hudCanvas;

    [Header("Dynamic Canvas — rebuilt frequently")]
    [SerializeField] private Canvas _dynamicCanvas;

    public void ShowDamageNumber(float damage, Vector3 worldPos)
    {
        var dmgText = _damagePool.Get();
        dmgText.transform.SetParent(_dynamicCanvas.transform, false);
        dmgText.Setup(damage, worldPos);
    }
}
```

### Example 2: Raycast Target Optimizer
**User**: "Bulk-disable raycastTarget on non-interactive elements."

**Agent**:
```csharp
#if UNITY_EDITOR
public class RaycastTargetOptimizer : MonoBehaviour
{
    [ContextMenu("Optimize Raycast Targets")]
    private void OptimizeRaycastTargets()
    {
        int optimized = 0;
        var graphics = GetComponentsInChildren<Graphic>(true);

        foreach (var graphic in graphics)
        {
            bool isInteractive = graphic.GetComponent<Selectable>() != null
                || graphic.GetComponent<IPointerClickHandler>() != null
                || graphic.GetComponent<IDragHandler>() != null;

            if (!isInteractive && graphic.raycastTarget)
            {
                graphic.raycastTarget = false;
                optimized++;
                EditorUtility.SetDirty(graphic);
            }
        }

        Debug.Log($"Disabled raycastTarget on {optimized} non-interactive elements");
    }
}
#endif
```

### Example 3: Resource Pairing (Loading Indicator)
**User**: "Loading spinner never disappears when API fails."

> Full ShowLoading/HideLoading patterns → see `@unity-event-safety` §2 Resource Pairing Principle

**Key rule**: `ShowLoading()` must ALWAYS have `HideLoading()` in a `finally` block — never rely on success path alone.
```

### Example 4: Tab Switching Spam Protection
**User**: "Rapid tab clicks cause animations to fight."

**Agent**:
```csharp
// ❌ BEFORE: Each click starts new animation without cancelling
public void OnTabClick(int tabIndex)
{
    _slider.DOAnchorPos(tabPositions[tabIndex], 0.2f);
}

// ✅ AFTER: Cancel previous + kill fighting tweens
private Tween _sliderTween;
private CancellationTokenSource _tabCts;

public async UniTaskVoid OnTabClick(int tabIndex)
{
    _tabCts?.Cancel();
    _tabCts?.Dispose();
    _tabCts = new CancellationTokenSource();

    _sliderTween?.Kill();
    SetButtonsInteractable(false);

    _sliderTween = _slider.DOAnchorPos(tabPositions[tabIndex], 0.2f)
        .SetLink(_slider.gameObject);

    try
    {
        await _sliderTween.ToUniTask(cancellationToken: _tabCts.Token);
        SetButtonsInteractable(true);
        OpenPanel(tabIndex);
    }
    catch (OperationCanceledException) { }
}
```

### Example 5: Immediate State vs OnComplete
**User**: "Panel sometimes stays visible after navigation."

**Agent**:
```csharp
// ❌ BEFORE: State change in OnComplete — unreliable if tween killed
void ClosePanel()
{
    _panel.DOScale(0, 0.3f).OnComplete(() =>
    {
        _panel.SetActive(false);  // Never runs if tween killed!
    });
}

// ✅ AFTER: Immediate state change + animation is cosmetic only
void ClosePanel()
{
    _panel.SetActive(false);       // IMMEDIATE — always executes
    _panel.DOScale(0, 0.3f)
        .SetLink(_panel);
}
```

### Example 6: Safe Area Handler (Notch — Orientation Change Support)
**User**: "Handle notch on mobile with orientation change support."

> For static safe area (no orientation change): use `UILayoutSpec.ApplySafeArea(rt)` in `Start()` — see `@unity-ugui-layout`.
> Use this MonoBehaviour pattern only when the app supports runtime orientation changes.

**Agent**:
```csharp
// Use only when orientation changes at runtime — polls Screen.safeArea in Update
public class CanvasSafeArea : MonoBehaviour
{
    [SerializeField] private RectTransform _safeAreaRect;
    private Rect _lastSafeArea;

    private void Update()
    {
        if (Screen.safeArea != _lastSafeArea)
        {
            ApplySafeArea();
            _lastSafeArea = Screen.safeArea;
        }
    }

    private void ApplySafeArea()
    {
        var safeArea = Screen.safeArea;
        var anchorMin = safeArea.position;
        var anchorMax = safeArea.position + safeArea.size;

        anchorMin.x /= Screen.width;
        anchorMin.y /= Screen.height;
        anchorMax.x /= Screen.width;
        anchorMax.y /= Screen.height;

        _safeAreaRect.anchorMin = anchorMin;
        _safeAreaRect.anchorMax = anchorMax;
    }
}
```

## UI Code Patterns (Script-level)

Rules for C# code that interacts with the UI system at runtime.

- [ ] Change material color via script (`material.color = ...`) instead of swapping sprites for color variations
- [ ] Disable auto-layout components (`ContentSizeFitter`, `LayoutElement`, layout groups) on **static containers** once layout is finalized — they recalculate every frame unnecessarily
  - Keep enabled on dynamic containers (scroll view Content, text that grows with input, runtime-populated lists)
  - Disable on fixed-size containers that only lay out once at startup
  - Grep: `grep -rn "ContentSizeFitter\|LayoutElement\|HorizontalLayoutGroup\|VerticalLayoutGroup\|GridLayoutGroup" --include="*.cs"`
  - Severity: 🟡 HIGH
- [ ] Avoid per-frame property changes on UI components (`RectTransform`, colors, sprites, text) — they trigger canvas rebuilds
- [ ] Update `text` property only when value actually changes — compare old vs new before assigning `myText.text`
- [ ] Flatten complex/deeply nested UI hierarchies — many layout components cascading is CPU-intensive
- [ ] Unique materials break batching — even slight `_Color` variation on identical materials increases draw calls
- [ ] `MaterialPropertyBlock` breaks SRP Batcher in URP/HDRP — use Material Variants or texture atlases instead
  - Grep: `grep -rn "MaterialPropertyBlock" --include="*.cs"`
  - Severity: 🟡 HIGH
- [ ] Prefer `TextMeshProUGUI` — `UnityEngine.UI.Text` is forbidden
  - Grep: `grep -rn "UnityEngine\.UI\.Text\|using UnityEngine.UI" --include="*.cs"`
  - Severity: 🟡 HIGH
- [ ] Consider `TextMeshPro` (non-UI variant) over `TextMeshProUGUI` when Canvas system is not needed — lighter CPU
- [ ] Consider `SpriteRenderer` over `Image` for UI elements not requiring Canvas layout — tight mesh, less overdraw

---

## Profiling Checklist
- [ ] Check Canvas.BuildBatch in CPU Profiler
- [ ] Check Canvas.SendWillRenderCanvases
- [ ] Count draw calls from UI (Frame Debugger)
- [ ] Verify raycastTarget off on decorative elements
- [ ] Profile scroll views with 100+ items

## State Safety Review
- [ ] Every `ShowLoading()` has `HideLoading()` in `finally`
- [ ] Every `SetInteractable(false)` has `SetInteractable(true)` on all paths
- [ ] All tween targets get `DOKill()` before manual property reset
- [ ] OnComplete contains only non-critical visual polish
- [ ] Rapid-fire interactions have cancellation logic

## Platform Considerations

| Platform | Focus |
|----------|-------|
| **Mobile** | Touch, safe area, portrait/landscape |
| **Tablet** | Both orientations, larger touch targets |
| **Desktop** | Mouse hover, keyboard navigation |
| **Console** | Gamepad focus, overscan |

## Related Skills
- `@unity-dotween-safety` - DOTween lifecycle and memory patterns
- `@unity-async-patterns` - Async/await lifecycle and cancellation
- `@unity-addressables` - Asset management and memory-safe release
