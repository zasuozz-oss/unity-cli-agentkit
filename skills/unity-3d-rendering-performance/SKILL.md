---
name: unity-3d-rendering-performance
description: "Use when a 3D scene is slow: triaging CPU vs GPU bottlenecks, cutting draw calls / SetPass calls, SRP Batcher vs GPU instancing, GPU Resident Drawer, LOD Groups, occlusion culling, overdraw and fill rate, skinned mesh cost, or rendering huge instance counts like grass and crowds."
---

# 3D Rendering Performance

## Overview
"The scene is slow" is three different problems with three different fixes. The
CPU can be busy *preparing* draws, the GPU can be busy *filling pixels*, or
either can be stalled on memory bandwidth. Optimising the wrong one wastes days.
Profile first; the rest of this skill is what to do once you know which.

## When to Use
- Frame rate drops in a 3D scene, or on device but not in the Editor.
- Draw call / SetPass call counts are high.
- Deciding between SRP Batcher, GPU instancing, and the GPU Resident Drawer.
- Rendering thousands+ of instances (foliage, crowds, debris).
- Thermal throttling on mobile after a few minutes.

## Triage First
| Symptom | Likely bound | First move |
|---|---|---|
| CPU frame time high, GPU idle | CPU / render thread | Cut SetPass calls, batching, culling |
| GPU time high, small screen helps | Fill rate / overdraw | Reduce transparency, fragment cost, render scale |
| Cost tracks resolution linearly | Fill rate | Render Scale, simpler fragment shaders |
| Cost tracks triangle count | Vertex | LODs, mesh decimation, fewer UV seams |
| Cost tracks texture size, not resolution | Bandwidth | Mipmaps, compressed formats, smaller RTs |
| Editor fine, device throttles after minutes | Thermal | Overall budget, `OnDemandRendering` on static screens |

Never skip this: *"you must profile your application to identify the cause of the
problem"* before making changes.

## Reducing CPU Rendering Work
- ✅ **SRP Batcher** is the default win in URP — it keeps material constant
  buffers GPU-resident and cuts state changes. Requires SRP Batcher compatible
  shaders (see `@unity-shader-authoring`).
- ✅ **GPU instancing** for many copies of the same mesh+material where the SRP
  Batcher path does not apply. A `MaterialPropertyBlock` disables SRP batching —
  if you need per-object data, go instancing, not property blocks.
- ✅ **GPU Resident Drawer** (Unity 6) makes Unity drive rendering through
  `BatchRendererGroup` automatically. Requirements: **Forward+** path, compute
  shader support (**not** OpenGL ES), Mesh Renderer components, and
  *Project Settings > Graphics > Shader Stripping > BatchRendererGroup Variants =
  Keep All*. Its GPU occlusion culling uses the **previous frame's depth** for a
  Hi-Z test — one frame of latency, so fast camera whips can pop.
- ✅ **Occlusion culling** for indoor/dense scenes; camera far clip and per-layer
  cull distances for everything else.
- ✅ A skybox instead of distant geometry.
- ❌ **NEVER** rely on dynamic batching for 3D meshes — it costs CPU to merge
  vertices and is largely superseded in URP.

## Reducing GPU Work
- ✅ **LOD Group** on every mid/large mesh; tune `LOD Bias` per quality level and
  set LOD Cross Fade Dither to `Bayer Matrix`.
- ✅ Attack **overdraw** before fragment complexity: overlapping transparent
  quads, particles, and full-screen UI are the usual cost. Use the Rendering
  Debugger's overdraw view.
- ✅ Mipmaps on anything viewed at varying distance — this is a bandwidth fix,
  not a quality nicety.
- ✅ Compressed texture formats (ASTC on mobile) — see `@unity-texture-pipeline`.
- ✅ Dynamic resolution / Render Scale below 1.0 when fill-bound.
- ❌ **NEVER** use a full PBR Lit shader on objects that are small on screen —
  a Simple Lit / Unlit variant is a large fragment saving.

## Extreme Instance Counts
When the count runs to tens of thousands or more, per-renderer approaches stop
scaling. Proven patterns:
- **`Graphics.DrawMeshInstancedIndirect`** with GPU frustum culling in a compute
  shader — `ColinLeung-NiloCat/UnityURP-MobileDrawMeshInstancedIndirectExample`
  reaches 10M grass instances at 30–60 fps on Adreno 506/612 class mobile GPUs
  with a single indirect draw. Cost tracks *visible* instances, not total.
- **Animation baked to texture** for crowds — `chenjd/Render-Crowd-Of-Animated-Characters`
  bakes skinned animation into a texture so characters render as instanced static
  meshes, removing per-character skinning from the CPU.
- **`Unity-Technologies/AutoLOD`** for generating LOD chains across a whole scene.

## Few-Shot Examples

### Example 1: measuring what the scene actually contains
```bash
utk exec 'int renderers = 0; long tris = 0; int lodCovered = 0;
var mats = new System.Collections.Generic.HashSet<UnityEngine.Material>();
foreach (var r in UnityEngine.Object.FindObjectsByType<UnityEngine.Renderer>(
    UnityEngine.FindObjectsSortMode.None))
{
  renderers++;
  foreach (var m in r.sharedMaterials) if (m != null) mats.Add(m);
  if (r.GetComponentInParent<UnityEngine.LODGroup>() != null) lodCovered++;
  var mf = r.GetComponent<UnityEngine.MeshFilter>();
  if (mf != null && mf.sharedMesh != null) tris += mf.sharedMesh.triangles.Length / 3;
}
Debug.Log("renderers=" + renderers + " uniqueMaterials=" + mats.Count +
          " triangles=" + tris + " underLODGroup=" + lodCovered);
return renderers;'
```
Unique material count is the practical ceiling on SRP batches. `renderers -
lodCovered` is your LOD backlog.

### Example 2: per-object colour without killing batching
```csharp
// ❌ BEFORE — every renderer with a property block leaves the SRP Batcher path
foreach (var r in renderers) { _mpb.SetColor("_Color", RandomColor()); r.SetPropertyBlock(_mpb); }

// ✅ AFTER — instanced property, one instanced draw
// shader: UNITY_DEFINE_INSTANCED_PROP(float4, _Color)
// material: Enable GPU Instancing = on
```

### Example 3: GPU Resident Drawer enabled but nothing changed
Check all four requirements at once — one missing silently disables it:
Forward+ path, non-GLES graphics API with compute, Mesh Renderers (not Skinned),
and `BatchRendererGroup Variants = Keep All`.

## Verifying with `utk`
```bash
utk editor play                 # measure in play mode, never in edit mode
utk screenshot                  # visual confirmation the change did not break the frame
utk console --type error
```
Draw-call and SetPass numbers come from the **Frame Debugger** and **Profiler**,
which are interactive-only — ask the user for the numbers, or add a build-time
counter. Do not claim a batching improvement you have not measured.

## Debugging Checklist
1. Slow on device, fine in Editor → mobile GPU fill/bandwidth, or a GLES-only limitation.
2. High SetPass calls → too many unique materials, or shaders not SRP Batcher compatible.
3. High draw calls, few materials → instancing not enabled, or property blocks in use.
4. Cost disappears when the window shrinks → fill-bound; overdraw and render scale.
5. Spikes on camera movement → culling/streaming, or GPU occlusion's one-frame latency.
6. Steady decline over minutes → thermal throttling, not a code regression.

## Related Skills
- `@unity-urp-setup` - The URP Asset settings that set the frame budget.
- `@unity-shader-authoring` - Making shaders SRP Batcher compatible.
- `@unity-3d-model-pipeline` - Mesh, LOD, and import-side cost.
- `@unity-3d-lighting` - Light count and shadow cost.
- `@unity-asset-audit` - Finding the oversized assets behind bandwidth limits.
