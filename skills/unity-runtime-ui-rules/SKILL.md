---
name: unity-runtime-ui-rules
description: Use when writing or fixing runtime uGUI code in any Unity game — HUD/layout that looks shrunk or wrong after rotation, Game view resize, safe-area or banner changes; a popup drawn under world content or other UI; a nested canvas that stays visible when its popup closes; overrideSorting that "doesn't work"; tutorial hands or pointers that drift off target; reparenting children of a prefab instance.
---

# Runtime uGUI rules that kept causing rework

Each rule below cost at least one user-visible bug report. Canvas setup, match
values and the per-region HUD scale formula live in `unity-ugui-layout`
(read it first; fallback path `~/.unity-cli-agentkit/skills/unity-ugui-layout/SKILL.md`).

## 1. Layout is reactive, never computed once

Anything derived from `Screen.width/height`, `Screen.safeArea`, a banner height
or the canvas size must be recomputed **whenever any of them changes** —
Game view resize, rotation, foldables, the banner loading late.

```csharp
Vector2Int _screen; Rect _safe; float _banner;
void LateUpdate() {
    float banner = Ads.BannerHeight;                       // whatever inset source the game has
    if (banner == _banner && _screen.x == Screen.width && _screen.y == Screen.height
        && _safe == Screen.safeArea) return;
    Apply(banner);                                         // store _screen/_safe/_banner inside
    Changed?.Invoke();                                     // listeners refit
}
```

(`OnRectTransformDimensionsChange` on the canvas also works for size; it does
not fire for a banner or safe-area change.)

Seen: a HUD scaled once in `Awake` froze at 0.21 instead of 0.78 when Play
started with a landscape/Free Aspect Game view — "the UI is tiny". The same
bug sat in a second component (a canvas-scaler inset on another scene).
**Grep for every `Screen.` read in `Awake`/`Start` when you fix one.**

## 2. Everything placed from a layout re-places after it

World positions baked from UI or board positions (tutorial hand, coach mark,
tooltip, focus ring, camera fit) go stale when the layout reruns. On the
`Changed` event: refit the board/camera, **then** re-point every active
pointer. Seen: after fixing #1, the tutorial hand pointed at the old cell.

## 3. `overrideSorting` only sticks on an enabled, nested canvas

- On a **root** canvas (a prefab root, a scene root) `overrideSorting` is
  ignored — root canvases sort by their own `sortingOrder`.
- On a **disabled** canvas the flag you set is not applied; set it after the
  canvas/popup is enabled (right after `Open()`), or bake it on a nested
  child canvas that is enabled.
- Document the game's sorting ladder in one place (e.g. HUD 10 < board-on-top
  15 < FX 20 < popups over board 25 < transition 30) and give every new canvas
  an explicit order from it.

Seen: a popup meant to cover the finished board drew under it because the
order was set on the prefab root and later on the disabled instance.

## 4. A popup's show/hide must include its nested canvases

If popups show/hide by toggling `Canvas.enabled` (cheaper than `SetActive`),
a child with its **own** `Canvas` (an override-sorted overlay, a rim, a hand
layer) is not hidden by the parent's toggle. Toggle them all:

```csharp
foreach (var c in GetComponentsInChildren<Canvas>(true)) c.enabled = visible;
```

and author such child canvases disabled by default. Seen: an overlay canvas
stayed on screen during gameplay after its popup closed.

## 5. Raise UI above a dim by giving it a canvas, not by reparenting

To lift a HUD element above a modal dim (forced tutorial on a button), add a
`Canvas` + `GraphicRaycaster` to that element in the build script and set
`overrideSorting`/`sortingOrder` at runtime; restore on exit. Children of a
**prefab instance** cannot be reparented in a scene — use a component on the
instance (a canvas, a scaler-driven inset) instead of moving objects.

## 6. Don't assume the canvas width

Code that hardcodes the reference width (`720f`, `1080f`) or assumes the
canvas is always that wide breaks once the canvas expands on narrow phones.
Read the real canvas rect. Narrow-aspect fixes: `unity-ui-narrow-aspect-fit`.

## 7. Input during forced flows

A forced tutorial/intro must block every other input path explicitly (board
taps, other buttons) in code, not only visually with a dim — raycasts through
a separate board camera are not stopped by a UI dim.

## Rules

- ✅ Recompute on screen/safe-area/inset change; fire one `Changed` event.
- ✅ Re-point pointers after every refit.
- ✅ `overrideSorting` on nested + enabled canvases, set after `Open()`.
- ✅ Popup toggle covers `GetComponentsInChildren<Canvas>(true)`.
- ❌ **NEVER** read `Screen.*` once in `Awake`/`Start` and keep the result.
- ❌ **NEVER** fix one instance of a layout bug without grepping for siblings.

## Related Skills

- `unity-ugui-layout` — canvas setup, match, HUD region scaling (read first)
- `unity-ui-narrow-aspect-fit` — 9:21+ fixes
- `unity-ugui-aspect-overlap` — rows colliding on tall phones
- `unity-popup-queue`, `unity-panel-navigation` — popup ownership and order
- `figma-unity-workflow` — building the screens these rules run on
- `unity-live-editor-loop` — reproducing a resize bug in the Editor
- `unity-layer-audit` — prove nothing draws or taps through a popup (run after any sorting change)
