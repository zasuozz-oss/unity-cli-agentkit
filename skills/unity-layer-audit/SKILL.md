---
name: unity-layer-audit
description: Use in any Unity game BEFORE fixing anything that draws or receives input in the wrong layer — game objects, sprites, meshes, particles or FX showing over a popup/HUD, a popup, tooltip, toast or hint bubble under the board/tray ("lỗi layer", "nằm dưới", "bị che", "bị đè", "đè lên"), a dim or mask drawn over content it should sit behind, two overlays fighting, taps or pinch reaching the board behind a popup — and ALWAYS after adding a popup, a Canvas, a Renderer/ParticleSystem, or changing any sortingOrder/sortingLayer/renderMode/camera depth. Ships a runnable audit (scripts/layer_audit.cs).
---

# Layer audit: nothing draws or taps through a popup

Agents change sorting orders, add FX, add popups — and never check what now
draws over what. The result ships as a user screenshot: game pieces on top of
a popup. This skill makes the check mechanical: **dump the real draw order,
flag every overlap above an open popup, flag ties, flag input leaks**.

## 1. How Unity decides who is on top (the model to reason with)

For everything a camera renders in the transparent queue (sprites, UI in
Screen Space - Camera / World Space, particles, lines, trails, TMP world text):

1. **Camera** — higher `depth` draws later (URP: overlay cameras in the stack draw after the base).
2. **Sorting layer** (its index in Tags & Layers), then **order in layer** (`sortingOrder`).
3. Only on a full tie: distance to camera / render queue / batching — **not something you control**.

Plus:
- **Screen Space - Overlay** canvases draw after every camera — always on top
  of world content, whatever the numbers say. A Screen Space - Camera canvas with
  no camera behaves as Overlay.
- A **nested Canvas** without `overrideSorting` sorts with its parent canvas,
  in hierarchy order. With `overrideSorting` it takes its own layer/order — but
  only when it is **enabled and not a root** (see `unity-runtime-ui-rules` §3).
  ⚠ The #1 cause of "game pieces over a popup": popups nested under the HUD
  canvas **without** `overrideSorting` draw at the HUD's order, so every tray,
  flying piece or FX with its own canvas/renderer above the HUD beats the popup,
  dim included. Every popup gets an explicit order from the ladder.
- `SpriteRenderer`, `MeshRenderer`, `ParticleSystemRenderer`, `LineRenderer`,
  `TrailRenderer` each carry their own `sortingLayer`/`sortingOrder`; a particle
  system placed inside a Canvas still sorts by its renderer's order, not the canvas's.
  A `SortingGroup` makes its children sort as one unit — but it has a renderer
  cap (~4k); past it Unity logs "Number of renderers and sorting groups
  handled…" and **drops the group's sorting silently**. Never wrap a
  data-sized board in one; set `sortingOrder` per renderer.
- **A custom UI shader's `Queue` beats hierarchy order.** A material with
  `"Queue"="Overlay"` (4000) on a canvas draws after its `Transparent` (3000)
  siblings wherever it sits in the hierarchy — a ported dim/hole-mask shader
  tinted the cards it was placed behind. When a UI element ignores its
  sibling order, read the shader's/material's queue before touching orders.

## 2. Keep ONE ladder, in code

Every game gets a single source of truth for draw order — a static class of
constants (or one table in the build script) — and every canvas, renderer
and FX reads from it. Example shape (numbers are per game):

| Order | Layer | When |
|---|---|---|
| -10 | background | always |
| 0–3 | world: cells, flying pieces, flashes, fx | gameplay |
| 10 | HUD canvas | always |
| 11–13 | world pieces that fly over the HUD, tray, tray fx | gameplay only |
| 14 | popups (dim + panel) | modal |
| 15 | world content raised over a popup (finish zoom) | finish state only |
| 20 | celebration fx | finish state |
| 25 | popup over the finished board | finish state |
| 30 | transition cover | transitions |

**Transient overlays are layers too** — tooltip, toast, hint bubble, tutorial
hand. Each gets its own ladder slot and its **own `Canvas` with
`overrideSorting`**; decide explicitly whether it sits above or below popups
and write why. Seen: a booster tooltip lived under the booster canvas (10),
so the tray bed (12) and tray FX (13) covered it; a comment claimed it had
"its own canvas" — it had none. The second fix gave it 14, the popups' order,
creating a tie.

**Adding any sub-layer re-derives the whole stack.** A new tray background,
FX canvas or overlay changes what sits above everything nearby: list every
canvas/renderer order around it (HUD, tray, FX, overlays, popups) and place
the new one, don't bump one number.

**Hard invariant: game assets always draw below popup UI.** Board, pieces,
tray, sprites, meshes, particles, world FX and world text — every gameplay layer
sits below the lowest popup order, whether the popup is open or not. Build the
ladder bottom-up from it: world < HUD < gameplay layers raised over the HUD <
**popups**. A popup never has to "win" against gameplay; it is above by
construction.

The only exception is a game asset that **is the popup's content** (e.g. the
finished board zoomed into the level-complete frame, a celebration burst over
the result card). Such an asset:
- is raised by that popup's open code and restored on every close path;
- stays below any popup that can open on top of it (the next popup order is
  higher than the raise);
- is listed by name next to the ladder as a declared exception.
Anything else above a popup is a bug, not a design choice.

Rules for the ladder:
- **Every order that is raised at runtime has an owner state and a restore on
  every exit path** (close, next level, go home, app pause). A raise that
  outlives its state is the classic "game assets over a popup" bug.
- **Popups sit above everything that can be alive while they are open** —
  flying pieces, tray FX, particles still playing. List those per popup.
- **No two layers that can be on screen together share an order.** Two ladder
  constants with the same value are fine only when their states are mutually
  exclusive — write that down next to the constants (and why).
- Magic numbers outside the ladder are findings — in runtime scripts **and**
  build/authoring scripts. Grep for them:
  `grep -rnE "sortingOrder|sortingLayer|overrideSorting|renderMode|\.depth" Assets Tools`.

## 3. Run the audit — numbers, not eyeballs

`scripts/layer_audit.cs` (next to this file) runs through `utk exec --file`
in Play mode and prints:

- **LADDER** — every visible canvas and renderer in real draw order, with its
  sorting layer/order, camera and screen rect;
- **ABOVE POPUP** — anything drawn after an open popup that overlaps it and is
  not part of it;
- **TIE** — overlapping items with identical camera/layer/order;
- **INPUT LEAK RISK** — an open popup with no `GraphicRaycaster` or no
  full-screen `raycastTarget`, so taps/pinch reach what is behind it.

Popups are auto-detected by "popup/dialog/modal" in a name or component type;
set `targets` at the top of the script for other names. Overlays (`*tooltip*`,
`*toast*`, `*tip`, or the `overlays` list) get their own check, since they
often have no canvas of their own: **OVERLAY** says which canvas they really
draw at, **COVERS OVERLAY** lists what is drawn over them. Run it with the
overlay **showing** — a hidden tooltip is not audited.

**When to run** — every one of these, before reporting UI done:
0. **No popup open**, mid-gameplay: every enabled canvas above the HUD must be
   one you expect (tray, flying pieces). A leftover override canvas (hand
   layer, rim) from a closed popup shows up here.
1. Each popup open over **populated gameplay** — tray full, a piece in flight,
   hint/tutorial active. Opening the popup the natural way often hides the bug
   (at a real level finish the tray is already empty), so force-open it mid-level.
2. Each popup open over **every special state** that raises orders (finish
   zoom, tutorials, intros, boss/combo FX).
3. After any change to a sorting order, sorting layer, render mode, camera depth,
   or after adding a Canvas / Renderer / ParticleSystem.

Seen in one game (all found by the user's screenshot first, not by the agent):
- Tray jewels (tray canvas 12, fx 13, flying sprites 11) over every popup:
  popups were nested under the HUD (10) with no override. Fix: the popup
  component sets `overrideSorting` + its ladder order **after** enabling its
  canvases; a test asserts every popup order > the highest gameplay layer.
- A hand-layer canvas (16) and a rim canvas (17) authored enabled inside their
  popups stayed live during gameplay after the popup closed.
- False alarm, same game: the audit reported the board (15/16) over Settings
  (14), a Tutorial/Settings tie at 14 and two ladder constants both at 15. All
  three came from a **forced test state**: a popup closed by reflection left the
  board raised, then Settings was opened directly. In real play the finish
  popup's dim blocks the Settings button, Settings refuses to open during the
  tutorial, and the two 15s never exist at once.

**Triage every finding before calling it a bug.** The audit sees one frame
and cannot know whether that frame is reachable. For each finding: was this
state reached through the real flow (buttons, game code), or forced (reflection,
`exec` calling a private method, closing a popup behind the game's back)? If
forced, check in code whether the two layers can really coexist (the entry
guards, what blocks input under the popup, what resets orders on exit). Report
only reachable ones; write the mutually exclusive pairs next to the ladder so
the next agent doesn't re-report them.

Gotchas:
- Prefer the real flow to reach a state; after a forced state, reload the
  scene before auditing the next one — leftover raises poison later runs.
- `Screen.width/height` read inside `utk exec` can be wrong (1407×20 seen);
  the script works in camera pixel / viewport space for that reason.
- If the user is playing in the Editor, the audit is read-only — run it, don't
  open/close popups under them (`utk-playmode-driving` → "someone's game").
- Take a screenshot of each finding (`unity-live-editor-loop` capture recipe)
  and put the ladder line next to it in the report.

## 4. Input follows the same layers

Drawing on top is not blocking input:
- A modal popup needs a `GraphicRaycaster` on its canvas chain and a
  full-stretch `raycastTarget` graphic (the dim).
- World-space input (board taps, drag, **two-finger pinch/pan**) bypasses UI
  raycasts unless the input code asks: check `EventSystem` over-UI for every
  pointer **and** gate on "a modal is open" in code. Seen: tutorial taps still
  painted the board; pinch still zoomed the board behind any popup.

## Rules

- ✅ Game assets always below popup UI; the only exceptions are assets that
  are a popup's own content, declared next to the ladder.
- ✅ One ladder in code; every canvas/renderer/FX order comes from it.
- ✅ Runtime raises have an owner state and a restore on every exit path.
- ✅ Run `layer_audit.cs` with each popup open in each state before "done".
- ✅ Modal = draws on top **and** blocks UI raycasts **and** gates world input.
- ❌ **NEVER** give a gameplay layer an order ≥ the popup order to "fix" it
  showing under the HUD — raise it just above the HUD, still below popups.
- ❌ **NEVER** pick a sortingOrder number without placing it in the ladder.
- ❌ **NEVER** fix one overlap by bumping a number without re-running the audit —
  bumps cascade into the next overlap.
- ❌ **NEVER** write "has its own canvas / draws over everything" in a comment
  or report unless `utk exec` shows the Canvas and its order. A change deferred
  (Editor in Play, compile pending) is reported as **not done**, and no comment
  may claim it — the next session trusts the comment.
- ❌ **NEVER** give an overlay or a new layer an order equal to one that can
  be on screen with it (popups included) — that is a TIE, not a fix.

## Related Skills

- `unity-runtime-ui-rules` — overrideSorting on enabled nested canvases, popup child canvases
- `unity-live-editor-loop` — capture recipe, running scripts in the open Editor
- `utk-exec-query` — reading state back as numbers
- `unity-popup-queue`, `unity-panel-navigation` — which popup owns the screen
- `figma-unity-workflow` — building the popups that this audit checks
