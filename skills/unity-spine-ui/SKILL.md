---
name: unity-spine-ui
description: "Use when playing Spine animations (SkeletonGraphic, SkeletonAnimation) in UI, synchronizing animation durations with popup panels, handling CanvasGroup fades, or fixing flickering during UI loading."
---

# Unity Spine Animation & UI Layer Fading

## Overview
Guidelines for managing Spine animations inside the Unity UI canvas system. Ensures smooth canvas layer fades, synchronizes animation event times with panel controls, and prevents flickering bugs (e.g. during Optimized Scrollview Adapter updates).

## When to Use
- Playing Spine animations (like gift boxes, character poses, or transitions) inside UI panels.
- Syncing delay times between Spine animation clips and the closing of popup panels.
- Configuring Canvas or CanvasGroup fades during asset initialization.
- Resolving layout blinking/flickering when dynamic content finishes loading.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **SkeletonGraphic** | Spine component used inside standard Unity UI (Canvas/RectTransform). |
| **Canvas Fade Integration** | Setting canvas alpha = 0 or disabling components during asset load, then fading in once fully loaded to prevent sudden visual pops. |
| **Animation Event Timing** | Querying the actual duration of a Spine clip programmatically instead of relying on hardcoded float wait delays. |
| **Blinking / Flickering** | A common artifact when UI adapters (like OSA) rebuild layouts before items are fully loaded/rendered. |

## Best Practices
- ✅ **Always** retrieve Spine animation lengths dynamically (`TrackEntry.Animation.Duration`) instead of hardcoding timing values.
- ✅ Set `CanvasGroup.alpha = 0` during loading or initialization, and trigger the fade-in only when assets are fully loaded.
- ✅ Unsubscribe from Spine animation complete events to avoid leaks (`SkeletonGraphic.AnimationState.Complete -= OnComplete`).
- ✅ Use dedicated layout groups or placeholder bounds to reserve space, preventing layout snaps when dynamic graphics pop in.
- ❌ **NEVER** leave a Canvas active with incomplete assets rendered (use alpha = 0 or disable layout rendering).
- ❌ **NEVER** call Spine animation state play commands directly without verifying that the `SkeletonGraphic` component is active and enabled.

## Few-Shot Examples

### Example 1: Dynamic Spine Animation Callback
**User**: "Fix the timing of the reward popup that uses the Spine box. Wait for the animation to finish before closing the panel."

**Agent**:
```csharp
public class RewardSpineController : MonoBehaviour
{
    [SerializeField] private SkeletonGraphic _spineAnimation;
    [SerializeField] private CanvasGroup _panelCanvasGroup;

    public async UniTask PlayRewardFlowAsync(string animationName, CancellationToken ct)
    {
        _panelCanvasGroup.alpha = 1f;

        // Play animation and capture track entry to get dynamic duration
        var trackEntry = _spineAnimation.AnimationState.SetAnimation(0, animationName, false);
        float duration = trackEntry.Animation.Duration;

        // Wait dynamically for duration
        await UniTask.Delay(TimeSpan.FromSeconds(duration), cancellationToken: ct);

        // Fade out panel cleanly
        await _panelCanvasGroup.DOFade(0f, 0.4f).WithCancellation(ct);
        gameObject.SetActive(false);
    }
}
```

### Example 2: Fade Asset after Load to Prevent Flickering
**User**: "Fix the profile item flickering when the image loads and the adapter rebuilds."

**Agent**:
```csharp
public class ProfileGridItem : MonoBehaviour
{
    [SerializeField] private CanvasGroup _canvasGroup;
    [SerializeField] private Image _thumbnailImage;

    public void Initialize()
    {
        // Set alpha to 0 before load begins to hide any placeholders or uninitialized structures
        _canvasGroup.alpha = 0f;
    }

    public async UniTask SetDataAsync(string url, CancellationToken ct)
    {
        _thumbnailImage.sprite = await ImageLoader.LoadAsync(url, ct);

        // Fade in only after the texture is completely loaded and assigned
        await _canvasGroup.DOFade(1f, 0.25f).WithCancellation(ct);
    }
}
```

## Related Skills
- `@unity-dotween-safety` - For managing DOTween fade fades safely.
- `@unity-ui-performance` - For reducing GC and draw calls on complex Spine UI elements.
