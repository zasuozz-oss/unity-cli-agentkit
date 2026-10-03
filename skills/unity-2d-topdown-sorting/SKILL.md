---
name: unity-2d-topdown-sorting
description: Use in any Unity 2D top-down or 3/4-view map (sprites for trees, props, houses, characters on one ground plane) when things draw in the wrong front/back order — "layer sai", "lỗi layer trên map", "nhân vật bị cây che", "đồ nằm trên thân cây", "pivot sai", hero drawn behind a tree it stands in front of, props standing inside each other, a house sorting wrong after it changes state, a long wall/hedge that is in front at one end and behind at the other, map content over the HUD — and before building or rescaling such a map. Ships Y-sort components and an audit (scripts/).
---

# Top-down draw order: sort by the foot, keep feet apart

Agents fixing "wrong layer" on a top-down map shuffle sortingOrder numbers and
get rejected three times, because three different problems look the same in a
screenshot:

| What the user sees | Real problem | Fix lives in |
|---|---|---|
| Map things over the HUD/popup; overlays under trees | **bands**: world, overlays and UI share one order range | §1 sorting layers |
| Hero behind a tree he stands in front of; a house in front of the hero standing north of it | **foot**: order comes from the pivot/centre, or from art that is not showing | §2 foot rule |
| Crates inside a trunk; two houses whose parts interleave wrongly | **footprint**: two upright things stand on the same ground — no order can fix it | §3 layout |
| A long diagonal wall/hedge: in front at one end, behind at the other | **one foot for a long sprite** | §4 strips |

Diagnose which row it is (run the audit, §5) before touching any number.

## 1. Sorting layers, one band per meaning

A higher sorting layer always draws over a lower one whatever the orders, so
each band only has to be consistent inside itself. Bottom to top:

| Layer | Holds | Order |
|---|---|---|
| `Ground` | ground plates, bridge decks, flat decals (roads, ponds, mud) — never Y-sorted | fixed bands, e.g. plates −3000+, decals −2899…−1600 |
| `World` | **everything upright**: props, trees, houses, characters, pickups | `round(-footY × 100)` and nothing else |
| `WorldOverlay` | fog, ambient FX, day/night shade, bubbles/markers, lock icons | fixed bands (markers may Y-sort among themselves) |
| `UI` | canvases only | canvas order |

Keep the constants in one static class every build tool and script reads;
an EditMode test asserts no world renderer sits on `UI` and every canvas
does. Grid/scale changes move y-derived orders (a ×2 map doubles them) — with
layers they can never climb into the UI band. Keep `|footY| × 100` inside
±32767.

## 2. The foot rule

- **Foot = the bottom of the art** — `SpriteRenderer.bounds.min.y` (world
  space) — **never the transform pivot**. Imported art usually has a centred
  pivot; reading the foot from bounds makes pivots irrelevant, so do not
  "fix pivots" asset by asset.
- **Statics** (props, trees): order assigned once by the map build tool from
  `bounds.min.y`; then make orders unique per layer — sort by (order, x) and
  bump ties: `order = max(want, last + 1)`.
- **Movers** (hero, NPCs): `scripts/YSort.cs` sets the order from
  `transform.position.y` every `LateUpdate` — so the mover's root must sit at
  its feet. Offset +1 wins a tie against a prop level with its feet.
- **Composites** (a house of several sprites, a character of parts, a
  landmark): a `SortingGroup` on the root plus `scripts/YSortGroup.cs`, whose
  foot is the lowest `bounds.min.y` of the **sprites showing now** — enabled,
  active, sprite set, not on `WorldOverlay`, not in `_ignore` (glows, hints).
  Inactive children (other states, hidden piles, other levels) sort a house
  1–3 units too far south. A child "hidden" with alpha 0 or an empty
  animation frame still counts — hide state art by disabling it, or list it in
  `_ignore`. Recomputed every `LateUpdate` because the art flips
  from many places; ~250 groups cost nothing measurable.
- **Camera sort axis alternative:** URP's 2D Renderer can sort by
  Transparency Sort Axis (0, 1, 0) with Sprite Sort Point = Pivot — zero
  code, but only correct when **every** pivot sits at the feet and there are no
  composites. With imported art, use the explicit rule above.

## 3. Footprints: two things never stand on the same ground

Order cannot interleave two overlapping upright things. Enforce this in the
map build tool (it is layout, not sorting):

- base = bottom 25 % of the sprite bounds; a tree's trunk = middle 30 % width
  of its bottom 20 %;
- two upright props may overlap by at most 30 % of the smaller base; decor
  never sits on a trunk;
- houses/landmarks/spawn spots reserve the bottom 60 % of the **union of all
  their states and levels** — a site that grows into a bigger house must not
  grow into its neighbour;
- on a conflict move the smaller prop to the nearest free ground within one
  hero height (rings × 16 directions), else hide it;
- ground cover (grass, pebbles, flowers) is exempt; authored set pieces may
  be exempt by name, written down.

Scaling props about their feet (e.g. making trees 2–3× bigger) re-creates
overlaps — rerun the footprint pass after every rescale or rebuild, and make
the pass idempotent (restore a snapshot, reapply).

## 4. Long sprites and tall canopies

- A diagonal wall/hedge has one foot that is wrong along most of its length:
  cut it into ~32 px vertical strips, each sorted by its own lowest opaque
  pixel, and block the walk grid along each strip's ground.
- Walk-behind fade: fade **only the canopy** above the trunk (a shader cut
  height), never the whole tree — a fully faded tree makes the hero look
  ghosted.

## 5. Audit and verify

- `scripts/topdown_sort_audit.cs` (copy into the project, `utk exec --file
  topdown_sort_audit.cs`, map open; in Play run it in every map state):
  `STALE` = order does not follow the foot of the showing art (§2), `TIE` =
  overlapping things with equal order, `FOOTPRINT` = bases overlapping > 30 %
  (§3). Small ground cover on `World` shows up as `FOOTPRINT` — move it to
  `Ground` or ignore it.
- EditMode tests that keep it fixed: composite footprints never overlap in
  any state (build through the real map refresh); each composite's order
  matches its showing art within 0.05 u in two states; no two upright bases
  share ground.
- Screenshots: teleport the hero just **south** of a house/tree/landmark (hero
  over it) and just **north** (it covers the hero's legs), plus a dense forest
  and a HUD/popup at the far south of the map — `utk screenshot --output`.
  Then `unity-layer-audit` for overlays and popups.

## Related skills

- `unity-layer-audit` — overlays, popups, canvases over the world
- `unity-editmode-tests` — writing the guard tests
- `utk-playmode-driving` — teleport + screenshot
