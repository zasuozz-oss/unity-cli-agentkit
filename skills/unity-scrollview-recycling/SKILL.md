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
- `localPosition ... is not valid. Input localPosition is { 0, NaN, 0 }` on a cell/group, or a grid tab rendering zero cells.
- A `NullReferenceException` inside the recycler's own `Update`/`LateUpdate`.
- An item changes state (bought, unlocked) but stays in the wrong section until the panel is reopened.
- A grid is off-centre or overflows on some screen widths only.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Update-path completeness** | Every visual field a cell shows must be (re)set in the recycle update callback (`UpdateViewsHolder`), not only on click/initial load. |
| **Async ownership check** | Async results (textures, API data) must verify the cell still represents the same item before applying. |
| **Semaphore-gated thumbnails** | Concurrent runtime texture loads are capped by a semaphore + cache; releases must be exception-safe. |
| **Append, don't reset** | New pages are appended to the data helper; full resets cause visible flash/jump. |
| **Layout-not-ready reentry** | A scroll-to / pivot / inset calculation run synchronously right after resetting or inserting items, before the layout pass has sized the new cells, divides by a size that is still 0 → `0/0 = NaN`. The NaN flows into the content inset and surfaces as an invalid `localPosition` assignment. Guard the division (`size <= 0 → retry next frame`) in the shared calculation, not only at the call site that happened to trigger it. Right after enabling a grid, cells-per-group can be 0 for a few frames too. |
| **Refresh ≠ repartition** | The recycler's refresh/rebind re-renders visible cells with current data. It does **not** recompute upstream grouping (owned vs. for-sale sections, sort buckets, category splits). A state change that can move an item between sections needs an explicit rebuild of the sections from the cached source ids. |
| **Viewport stretches with its container** | A viewport hardcoded to a fixed width and centre anchor is only correct at one canvas width. With a stretched viewport, let the adapter derive cell size from the live width (its "preserve cell aspect ratio"-style option) instead of a fixed pixel width. |

## Best Practices
- ✅ **Always** set ALL visual state in the cell-update path — treat the cell as dirty garbage from a previous item.
- ✅ Capture the item id before `await`; after `await`, bail out if the holder now displays a different id.
- ✅ Append page data (`InsertItemsAtEnd`-style) instead of resetting the whole list when paging.
- ✅ Debounce or disable tab/filter buttons that trigger list reloads until the current reload finishes.
- ✅ When a panel is reused across users/contexts, clear or re-fetch per-context data on each open (stale previous-user data is a recurring bug).
- ✅ After any change that can move an item between sections (ownership, unlock, category), call the section rebuild. Verify it by checking the item's section right after the change, without leaving and re-entering.
- ✅ For a recycler NaN or null crash, add one-time diagnostic logging that names the offending value (which group, which field, pivot vs. size). A stack trace alone does not say which upstream value was bad.
- ✅ Before calling a recycler fix verified, re-run the **exact** reported repro steps, not only a synthetic before/after test. A synthetic test can pass while a second, independent cause remains.
- ✅ A guard inside the recycler's `Update`/`LateUpdate` that skips the tick when internal state is null (and logs once) is a firewall, not a root-cause fix. Say so, and keep the diagnostic log that will explain the lifecycle desync next time.
- ✅ When a grid is off-centre, compare the viewport's anchors with sibling panels before touching cell alignment. Avoid force-expanding cell width or centring cells in a row, because both distort a lone last-row item.
- ✅ Mark edits inside vendored recycler code with a comment, so they survive the next vendor upgrade.
- ❌ **NEVER** apply an async-loaded texture directly to the cell without an ownership check.
- ❌ **NEVER** set conditional UI (badges, stars, sale tags) only in the success branch — explicitly hide them in the else branch too.
- ❌ **NEVER** assume a bug is recycler-related because it is near a list. First confirm the panel actually uses a recycled adapter, not a plain `ScrollRect` or a manual layout. Tab strips, clip shaders and undo/redo bugs near lists often have nothing to do with recycling.

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
- `@unity-remote-image-flicker` - First-open flicker and aspect fitting of remote images.
- `@unity-iap-purchase-ownership` - Re-deriving ownership in lists after a grant.
