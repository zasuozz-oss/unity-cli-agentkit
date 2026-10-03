---
name: unity-fx-port
description: Use in any Unity URP game when bringing particle FX in from another project, an asset-store pack or a ripped build — copying FX prefabs/materials across, "port FX", "lấy hiệu ứng từ game X" — or when imported effects show pink/magenta, invisible, as a single dot or a white box, tiny or huge, never disappear, sit at a UI element's corner, draw under the canvas, or log "has a null material", "Unknown error occurred while loading", "Destroy may not be called from edit mode". Ships a URP fix-up, an audit and a one-shot spawn helper (scripts/).
---

# Port particle FX into a URP project

Imported FX fail in the same five ways every time: legacy shaders (pink),
missing `UIParticle`-style scripts (invisible), scale authored for another
canvas (dot or giant), looping/100-second particles (never end), and sorting
from another project (under the UI). Run the steps in order; verify with
numbers and a screenshot, never by eye in the Scene view.

## 1. Import

- Copy prefabs, materials and textures **with their `.meta` files** so GUIDs
  survive. Clear `assetBundleName` in the copied `.meta`s.
- After copying, **before** `utk editor refresh`, check GUID collisions with
  what the project already has:
  `grep -rh '^guid:' Assets --include='*.meta' | sort | uniq -d` — any output
  is a collision; on import Unity silently re-GUIDs one side and breaks its
  references.
- **Ripped packs** (assets from a decompiled build): sprite references point
  at stub `.asset` files (`--- !u!213`), not real sprites. Rewrite each stub
  guid to the real png's guid with `type: 3` — with `type: 2` the png fails
  with `Unknown error occurred while loading '…png'`. Sprite-sheet pngs
  (`spriteMode: 2`) need their `sprites:` slices back in the `.meta`.
- `utk editor refresh`, then continue. Package-level dependencies
  (`com.coffee.ui-particle` …) are **not** imported: their components become
  missing scripts, handled in step 2.

## 2. Fix up — `scripts/fx_port_urp.cs`

Copy it to `Tools/fx_port_<pack>.cs`, set the params at the top, run
`utk exec --file Tools/fx_port_<pack>.cs --timeout 120000`. Idempotent. It:

- remaps every non-URP material to **`Universal Render Pipeline/Particles/Unlit`**,
  reading `_MainTex`/`_TintColor` through `SerializedObject` (the legacy
  shader is gone, so `GetTexture` returns nothing), additive when the name
  says `Add`, `_BaseColor = 2 × _TintColor` (legacy shaders doubled it);
- strips missing scripts (`RemoveMonoBehavioursWithMissingScript`) and
  **re-enables every `ParticleSystemRenderer`** — UIParticle drew them itself,
  so the source shipped with renderers off: stripping it leaves invisible FX;
- fills null / `Default-*` material slots with a fallback you choose;
- rebases sorting into this project's bands, keeping the pack's relative
  order: UI FX just above the canvas order (FX on a Screen Space - Camera
  canvas sort against the canvas by layer/order), world FX inside the world
  band (see `unity-layer-audit`);
- sets root scale per prefab. **UIParticle `m_Scale` 100** = sizes authored in
  world units at 100 px/unit; under a Screen Space - Camera canvas
  (0.01 per px) the root needs ×100. Read each source prefab's `m_Scale`; some
  are already authored at ×100 and get ×1.

## 3. Audit — `scripts/fx_audit.cs`

Copy into the project, run `utk exec --file fx_audit.cs` (Edit mode). Flags
per system: `MISSING_SCRIPT`, `RENDERER_OFF`, `NULL_MAT` (trail material
included), `BROKEN_SHADER`, `NON_URP`, `NULL_SPRITE<i>` in a sprite-mode
texture sheet, `LOOP`, `LONG_LIFE`. Port is done when only intended `LOOP`s
remain. Turn the same checks into an EditMode test per FX folder so the next
import cannot regress (`unity-editmode-tests`).

## 4. Spawn and place — `scripts/FxBurst.cs`

Copy into the game's runtime scripts. `FxBurst.Play(prefab, parent, pos)`:

- `Stop(true, StopEmittingAndClear)` first (`playOnAwake` already started
  it), `loop = false`, clamps `duration` and constant `startLifetime` to
  3 s — pack "Rotate"/sunburst effects are one 100-second particle built for
  a looping popup and otherwise stay on screen — then `Play(true)` and a
  timed `Destroy` (covers child systems and trails, which `stopAction` does
  not).
- Returns null outside Play mode: `Destroy may not be called from edit mode`
  in EditMode tests is fixed by this guard, not by `LogAssert.Expect`.
- Place on a UI element with `FxBurst.Center(rect)` =
  `rt.TransformPoint(rt.rect.center)` — `transform.position` is the pivot,
  which may sit on an edge. A world point onto a canvas: `FxBurst.ToCanvas`.

## 5. Verify with numbers, then eyes

In Play mode with the FX on its real canvas: spawn it, then measure live
`particleCount` and `renderer.bounds` converted to screen px (a gather FX
reading ~130 px is right; 4 px or 1100 px is a scale bug), then
`utk screenshot --output Temp/shots/fx.png` and look. Force `main.loop =
true` in a dev scene for stable shots. Without Play: `NewPreviewScene` + an
ortho camera with a RenderTexture + `ParticleSystem.Simulate(t, true, true,
true)` at a few `t`.

## Pitfalls

| Symptom | Cause → fix |
|---|---|
| Pink/magenta | legacy/Built-in shader → step 2 remap |
| Invisible, `particleCount` > 0 | renderer disabled after stripping UIParticle → `r.enabled = true` |
| `X has a null material` | stub material without shader → remap + fallback |
| A single dot, or a white/beige box | trail-only FX spawned standing still (needs movement), or low-res source texture → pick a burst-type FX; judge each by screenshot |
| Huge glow covering the screen | ×100 applied to a prefab already authored at ×100, or the source asset is just big → measure bounds px |
| Effect never ends | loop / 100 s lifetime → `FxBurst` clamps |
| Burst at a button's corner | pivot used → `rt.rect.center` |
| FX under the popup/canvas | sorting from the source project → step 2 bands, then `unity-layer-audit` |
| `BaseShaderGUI does not exist in namespace UnityEditor.Rendering.Universal.ShaderGUI` | it is `UnityEditor.BaseShaderGUI` |
| Wired FX fields empty after a UI rebuild | build scripts recreate panels → keep the wiring in a script and rerun it after every rebuild |

## Related skills

- `unity-layer-audit` — sorting bands; run it after adding FX to a popup
- `unity-runtime-ui-rules` §4 — popup FX traps
- `utk-asset-import` — copying assets with `.meta`
