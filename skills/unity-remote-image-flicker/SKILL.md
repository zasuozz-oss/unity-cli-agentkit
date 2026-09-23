---
name: unity-remote-image-flicker
description: "Use when Unity UI images (icons, badges, thumbnails, avatars) flicker, flash a placeholder or pop in the first time a popup/panel opens, when a remote/CMS image of unknown or varying aspect must fit a fixed box without SetNativeSize or comes out stretched, when localized text flashes its placeholder before strings load, or when a loading indicator sits in the wrong place, disappears with its content, or does not reappear on a second open/tab switch."
---

# First-Open Flicker, Remote Images and Loading Indicators

Flicker on first open is almost never "the asset is slow". Three causes cover
nearly every case: the image is not in memory when the panel binds, the panel
binds more than once, or something is sized or positioned before the thing it
depends on exists. Find which one before changing anything.

## 1. Diagnose before fixing

Record the first frames of the open (`@utk-playmode-driving`: pause, step
frames, screenshot or read state each frame). For every flickering element,
log per frame: whether its sprite/texture is set, whether it is a
placeholder, whether the `Image` is enabled, and how many times the panel's
bind/redraw ran. Numbers settle the cause:

| What the frames show | Cause |
|---|---|
| Sprite null/placeholder for N frames, then set | Not in RAM: disk read + decode on every open, or not downloaded yet |
| Sprite set, unset, set again | The panel binds 2–3× per open, and each bind resets to a placeholder |
| Sprite set to a destroyed object, or `none` | A cache holding a dead entry, or a placeholder pointing at a deleted asset |
| Correct image, wrong size for a frame | Size/aspect code ran before the texture was assigned |

## 2. Get the image into memory before the panel needs it

- **A disk cache is not enough.** Reading and decoding a file and calling
  `Sprite.Create` on every open costs frames even when the file is cached.
  Keep decoded sprites in a RAM cache keyed by URL or id.
- **Prefetch in the background** at a moment you control (boot, menu idle):
  the catalog/metadata plus the images the first open will show. Fire and
  forget, but never ahead of anything the boot must not wait on
  (`@unity-startup-loading`).
- **Reuse the project's existing cache.** Almost every project already has one
  (a thumbnail or sprite cache). Adding a second cache splits the truth.
- **De-duplicate in-flight loads.** Keep a dictionary of URL →
  `TaskCompletionSource` (single-flight, `@unity-async-patterns`), so a bind
  racing the prefetch awaits the same download instead of starting another.
- **A cache must overwrite dead entries.** A static cache that survives a
  scene change, or a play session with domain reload off, can hold a `Sprite`
  whose texture was destroyed. A write that skips "key already exists" poisons
  that key forever. Check `entry == null` (Unity's destroyed-object check) and
  overwrite.
- **Placeholders must exist.** A placeholder sprite referencing a deleted asset
  renders as nothing. Either disable the `Image` until a real sprite is present,
  or fix the reference.

## 3. Bind once

- Count bind/redraw calls per open before blaming the asset. Two or three
  redundant binds (open → data refresh → layout callback) re-flash the
  placeholder even with a perfect cache.
- A bind must not reset an image that is already showing the right sprite. Set
  the placeholder only when the id changes.
- If the data arrives after open, keep the element hidden until the data and
  the first sprite are both ready, for example with a `CanvasGroup` at alpha 0
  that fades in. Do not show an empty state and then swap it.

## 4. Fit remote images of unknown aspect

- `preserveAspect = true` stops distortion; it does not size the box. For a
  fixed box (e.g. 135×135): size the **container**, stretch the `Image` inside
  it (anchors 0–1, `sizeDelta` 0), keep `preserveAspect` on, and never call
  `SetNativeSize()`.
- Under a layout group, the **root** of the cell is what the group sizes. A cell
  whose root is smaller than the intended box (while the image is bigger)
  reads as an aspect bug but is a layout bug.
- Want crop-to-fill instead of letterbox? Add a "cover" mode: scale by the
  larger ratio and let a `RectMask2D`/`Mask` on the container crop the
  overflow.
- **Aspect-fit runs after the texture is assigned.** Code that clears the
  texture, calls `FitAspect()`, and only then assigns the new texture is a no-op
  whenever `FitAspect` guards on `texture == null`. The rect keeps the previous
  item's size. This is especially visible in recycled cells
  (`@unity-scrollview-recycling`).
- Authored local sprites with a known aspect are `@unity-ugui-layout`'s
  "Sprites keep their aspect".

## 5. Localized text must not flash its placeholder

`LocalizeStringEvent` (and similar components) refresh in `OnEnable`, which runs
the moment the prefab is instantiated. If the string table is still loading,
the prefab's authored placeholder text shows for a few frames.
- Load the popup's string table **before** `Instantiate` (await it in the open
  path). A redraw after spawn does not help.
- Verify by reading the table's state (loaded, entry count) at the moment you
  instantiate.

## 6. Loading indicators

- **Never a child of the content it covers.** Hiding the content to show
  loading then hides the spinner too. Make it a sibling above the content.
- **Show on every load, hide on every exit.** Show at load start, hide in
  `finally` (`@unity-event-safety`). A second tab switch that never shows the
  spinner means the show is tied to the first open or to a one-time flag.
- **Do not compute its position mid-transition.** A world-space "centre on
  screen" computed while the page is still sliding lands off-screen once the
  slide settles. Prefer a fixed anchored position (centre anchors, fixed
  offset), which is stable under `ScaleWithScreenSize`. Otherwise compute it
  after the transition completes.
- While loading, hide the content and the empty-state text too, not just dim
  them. A blank card plus a spinner reads as broken.

## 7. Verify

- Re-run the frame recording from step 1. The placeholder/unset frame count
  must be 0 on first open, on a second open, and on a first open after
  clearing the RAM cache (disk-only path).
- Also run a session with stale cache state (domain reload off, second play)
  when the cache is static.
- Unit tests cover cache and single-flight logic, but not the flicker itself.
  If you did not record frames in play mode, say the fix is visually unverified.

## Related Skills
- `@unity-async-patterns` — single-flight loads, cancellation.
- `@unity-scrollview-recycling` — ownership checks for async images in recycled cells.
- `@unity-ugui-layout` — sprite aspect rules for authored assets, anchors.
- `@unity-startup-loading` — where background prefetch may and may not sit in boot.
- `@unity-spine-ui` — fading assets in after load.
