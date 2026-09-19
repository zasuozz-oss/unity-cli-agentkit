---
name: unity-scrollview-recycling
description: "Use when working on recycled scroll views/grids (OSA, custom recyclers) or bugs mentioning flicker/flash on scroll, wrong cell data after recycle, thumbnails swapping between items, ratings/labels missing while scrolling, or visible jumps when appending pages."
---

# Unity Recycled ScrollView Correctness

## Overview
Recycled list views (OSA / `SimpleDataHelper`, or custom recyclers) reuse cell GameObjects. Almost every "sometimes wrong cell" bug is **recycled-cell state leaking into the wrong item**, or async work finishing after the cell was reused. This skill is about correctness; for draw calls and rebuild cost see `@unity-ui-performance`.

## When to Use
- A cell shows another item's thumbnail/label/rating, briefly or permanently.
- Visual state appears on load but disappears when scrolling.
- Lists flash or jump when a new page of data is appended.
- Spam-clicking a tab that reloads list data breaks the list.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Update-path completeness** | Every visual field a cell shows must be (re)set in the recycle update callback (`UpdateViewsHolder`), not only on click/initial load. |
| **Async ownership check** | Async results (textures, API data) must verify the cell still represents the same item before applying. |
| **Semaphore-gated thumbnails** | Concurrent runtime texture loads are capped by a semaphore + cache; releases must be exception-safe. |
| **Append, don't reset** | New pages are appended to the data helper; full resets cause visible flash/jump. |

## Best Practices
- ✅ **Always** set ALL visual state in the cell-update path — treat the cell as dirty garbage from a previous item.
- ✅ Capture the item id before `await`; after `await`, bail out if the holder now displays a different id.
- ✅ Append page data (`InsertItemsAtEnd`-style) instead of resetting the whole list when paging.
- ✅ Debounce or disable tab/filter buttons that trigger list reloads until the current reload finishes.
- ✅ When a panel is reused across users/contexts, clear or re-fetch per-context data on each open (stale previous-user data is a recurring bug).
- ❌ **NEVER** apply an async-loaded texture directly to the cell without an ownership check.
- ❌ **NEVER** set conditional UI (badges, stars, sale tags) only in the success branch — explicitly hide them in the else branch too.

## Few-Shot Examples

### Example 1: Ownership check after async thumbnail load
**User**: "Fix the bug where thumbnails briefly show the wrong item and only correct themselves when scrolling fast."

**Agent**:
```csharp
public async UniTaskVoid LoadThumbnailAsync(ItemData item, CancellationToken ct)
{
    _displayedItemId = item.Id;            // ownership marker for this holder
    _thumbnail.enabled = false;            // never show the previous item's texture

    await ThumbSemaphore.WaitAsync(ct);    // cap concurrent decodes
    try
    {
        var tex = await RuntimeTextureCache.GetAsync(item.ThumbUrl, ct);
        if (_displayedItemId != item.Id) return; // cell was recycled mid-flight
        _thumbnail.texture = tex;
        _thumbnail.enabled = true;
    }
    finally
    {
        ThumbSemaphore.Release();          // exception-safe release
    }
}
```

## Rule of Thumb
Before fixing a "sometimes wrong cell" bug, reproduce by **fast-scrolling**. If fast scroll triggers it, it is recycle-state, not data.

## Related Skills
- `@unity-ui-performance` - Rebuild/draw-call side of scroll views.
- `@unity-async-patterns` - Cancellation of in-flight loads on recycle.
