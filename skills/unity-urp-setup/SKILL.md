---
name: unity-urp-setup
description: "Use when configuring the Universal Render Pipeline: choosing a rendering path (Forward / Forward+ / Deferred), tuning the URP Asset and Universal Renderer for mobile, quality tiers, MSAA vs post-processing, render scale, depth/opaque textures, or debugging 'my URP setting has no effect'."
---

# URP Asset & Renderer Setup

## Overview
URP splits its config across **two assets** — a `UniversalRenderPipelineAsset`
(per Quality level) and one or more `UniversalRendererData` it points at. Most
"URP is slow" and "this setting does nothing" reports come from editing the
wrong asset, from a setting the active rendering path silently ignores, or from
a Renderer Feature forcing an intermediate texture on a tile-based mobile GPU.

## When to Use
- Picking or changing the rendering path for a project.
- A lighting/shadow setting in the URP Asset appears to have no effect.
- Cutting GPU cost or bandwidth on mobile.
- Frame time regressed after adding post-processing or a Renderer Feature.
- Setting up per-platform quality tiers.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Two assets, two scopes** | URP Asset = quality/lighting/shadow budget, selected per Quality level. Renderer Data = rendering path, Renderer Features, depth priming, intermediate texture. |
| **Quality level binds the asset** | `QualitySettings.renderPipeline` overrides `GraphicsSettings.defaultRenderPipeline`. Editing the default asset while a Quality level overrides it changes nothing at runtime. |
| **Forward+ ignores settings** | Selecting Forward+ makes URP disregard *Additional Lights*, *Main Light*, *Additional Lights > Per Object Limit* (URP Asset) and *Reflection Probes > Probe Blending* (Lighting window). The fields stay editable — they just do nothing. |
| **Intermediate texture** | Rendering straight to the backbuffer is the fast path on mobile. Any feature that needs a full-screen read (`Intermediate Texture = Always`, most Blit-based Renderer Features, opaque texture) forces a resolve out of tile memory. |
| **Tile memory** | On mobile TBDR GPUs, MSAA is nearly free *until* something resolves the tile. Post-processing forces the resolve, so MSAA + post is far more expensive than either alone. |

## Rendering Path Selection
- **Forward** — per-object light limit applies. Default; safest on low-end GLES.
- **Forward+** — screen tiled into light lists; no per-object light limit, but a
  per-camera visible-light limit remains. Required for **GPU Resident Drawer**.
- **Deferred / Deferred+** — many lights on desktop/console. Not for mobile TBDR;
  incompatible with depth priming and with MSAA.

- ✅ Mobile with few lights → **Forward**. Mobile/desktop with many small lights → **Forward+**.
- ❌ **NEVER** switch to Forward+ and then keep tuning *Per Object Limit* — that
  field is dead on this path. Reduce actual light count instead.
- ❌ **NEVER** ship Deferred on Android/iOS without profiling on the target device.

## Renderer Settings
- ✅ **Depth Priming Mode**: leave `Disabled` on mobile — `Auto` is unsupported on
  Android, iOS and tvOS, and it does not combine with deferred or MSAA.
- ✅ **Intermediate Texture**: keep `Auto`. `Always` guarantees Renderer Feature
  compatibility at a significant, permanent cost.
- ✅ **Copy Depth Mode** `After Transparents` on mobile — a documented memory
  bandwidth win.
- ✅ **Accurate G-buffer normals**: off (deferred only).
- ❌ **NEVER** enable *Depth Texture* / *Opaque Texture* "just in case". Each adds
  a full-screen copy every frame. If Opaque Texture is genuinely needed, set
  **Opaque Downsampling = 4x Bilinear**.

## URP Asset Settings — mobile budget
Exact field names, from Unity's URP optimization guidance:

| Section | Setting | Value |
|---|---|---|
| Quality | Render Scale | below `1.0` |
| Quality | Upscaling Filter | `Bilinear` or `Nearest-Neighbor` |
| Quality | LOD Cross Fade Dither | `Bayer Matrix` |
| Lighting | Main Light > Cast Shadows | disable if the scene is baked |
| Lighting | Additional Lights > Cast Shadows | disable |
| Lighting | Additional Lights > Per Object Limit | lowest acceptable (Forward only) |
| Lighting | Additional Lights > Shadow Atlas / Shadow Resolution | lowest acceptable |
| Lighting | Additional Lights > Cookie Atlas Format | `Color Low` |
| Lighting | Additional Lights > Cookie Atlas Resolution | lowest acceptable |
| Shadows | Cascade Count | lowest acceptable |
| Shadows | Max Distance | reduce — the single biggest shadow win |
| Shadows | Soft Shadows | disable, or `Low` |
| Shadows | Conservative Enclosing Sphere | enable |
| Post Processing | Grading Mode | `Low Dynamic Range` |
| Post Processing | LUT Size | lowest acceptable |
| Post Processing | Fast sRGB/Linear conversion | enable |
| Decals (feature) | Technique | `Screen Space`, Normal Blend `Low`/`Medium` |

## Few-Shot Examples

### Example 1: "I lowered Per Object Limit and nothing changed"
```
Renderer Data > Rendering > Rendering Path = Forward+
```
Forward+ ignores that field entirely. Either switch the path back to Forward, or
cut the number of lights whose volume overlaps the camera — on Forward+ cost
tracks *visible lights per tile*, not per object.

### Example 2: mobile frame time doubled after enabling Bloom
Bloom pulls in an intermediate texture and post-processing resolve. On a TBDR
GPU that ends free MSAA. Fix order: drop MSAA to `Disabled` while post is on,
set Render Scale to `0.8`, keep `Grading Mode = Low Dynamic Range` and the
smallest LUT that still looks right.

### Example 3: which asset is actually live
```bash
utk exec 'var rp = UnityEngine.QualitySettings.renderPipeline ?? UnityEngine.Rendering.GraphicsSettings.defaultRenderPipeline;
Debug.Log("Quality level: " + UnityEngine.QualitySettings.names[UnityEngine.QualitySettings.GetQualityLevel()]);
Debug.Log("Active RP asset: " + (rp == null ? "Built-in (no SRP)" : UnityEditor.AssetDatabase.GetAssetPath(rp)));'
```
Edit *that* path. A per-quality-level override is the usual reason a change to
the project-default asset does nothing.

## Verifying with `utk`
```bash
# Dump every serialized field of the live URP asset (names differ per URP version)
utk exec 'var rp = UnityEngine.QualitySettings.renderPipeline ?? UnityEngine.Rendering.GraphicsSettings.defaultRenderPipeline;
var so = new UnityEditor.SerializedObject(rp); var p = so.GetIterator(); int n = 0;
while (p.NextVisible(true)) { Debug.Log(p.propertyPath + " : " + p.propertyType); n++; }
return n;'

# Eyeball the result after a settings change
utk screenshot
```
Change settings through `SerializedObject` + `ApplyModifiedProperties()`, then
`utk editor refresh` — most URP asset fields are private and have no public setter.

## Debugging Checklist
1. Setting has no effect → wrong asset (quality-level override) or a path that ignores it.
2. Sudden mobile frame-time jump → intermediate texture forced by a Renderer Feature, post-processing, or Opaque/Depth Texture.
3. Shadows heavy → Max Distance and Cascade Count before anything else.
4. Banding after enabling post → Grading Mode / LUT size too low; raise only that one.
5. Feature works in Editor, breaks on device → GLES-only limitation (compute, Forward+ tiling, GPU Resident Drawer).

## Related Skills
- `@unity-urp-renderer-feature` - Writing Renderer Features with the Render Graph API.
- `@unity-3d-rendering-performance` - Batching, instancing, LOD, overdraw.
- `@unity-3d-lighting` - Lightmaps, probes, APV.
- `@unity-android-build` - Shipping the resulting settings to a device.
