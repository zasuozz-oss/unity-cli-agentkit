---
name: unity-3d-lighting
description: "Use when setting up 3D scene lighting in Unity: baked vs mixed vs realtime lights, lightmapping and lightmap UVs, Light Probes vs Adaptive Probe Volumes, reflection probes, light/rendering layers, or debugging dark instantiated prefabs, black dynamic objects, seams, and long bake times."
---

# 3D Scene Lighting

## Overview
Real-time lighting is the most expensive thing a mobile 3D scene can do, and
every cheap alternative — lightmaps, probes, APV — has one specific hole.
Lightmaps do not follow objects spawned at runtime. Light Probe Groups light
per-object, so a large mesh gets one blended value. Adaptive Probe Volumes fix
that but cannot be hand-placed. Choosing wrong shows up as "the prefab is black".

## When to Use
- Setting up lighting for a 3D scene or level.
- Prefabs instantiated at runtime appear unlit/dark while the same object placed
  in the scene looks right.
- Deciding between Light Probe Groups and Adaptive Probe Volumes.
- Bake times or lightmap memory are out of budget.
- Lightmap seams, splotches, or leaking through walls.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Light Mode** | `Realtime` (dynamic, expensive), `Baked` (static only, free at runtime, no effect on dynamic objects), `Mixed` (baked indirect + realtime direct/shadows). |
| **Mixed sub-modes** | `Baked Indirect`, `Shadowmask` (baked static shadows + realtime dynamic), `Subtractive` (cheapest, one shadow colour, mobile-grade). Set project-wide in Lighting > Mixed Lighting. |
| **Lightmap UVs** | Baking needs a non-overlapping UV2. Either author it in the DCC or enable **Generate Lightmap UVs** on the Model importer. |
| **Contribute GI flag** | Only renderers with `Contribute Global Illumination` (static) get lightmapped. Everything else falls back to probes. |
| **Light Probe Group** | Per-*object* sampling: the whole renderer gets one blended probe value. |
| **Adaptive Probe Volume (APV)** | Unity 6, URP+HDRP. Per-*pixel* probe sampling, automatic brick subdivision, streaming for open worlds, Lighting Scenarios and sky occlusion. |
| **Reflection Probes** | Baked cubemaps for specular. Probe **blending is ignored on the Forward+ path**. |

## Best Practices
- ✅ Mobile default: **one realtime Directional light** (Main Light) + everything
  else baked. Additional light shadows off.
- ✅ Mark every static mesh `Contribute GI` and give it a sane lightmap scale —
  scale, not global resolution, is the right knob for one oversized wall.
- ✅ Use **Shadowmask** when dynamic characters must cast shadows into a baked
  scene; use **Subtractive** when the budget is tight and one shadow colour is
  acceptable.
- ✅ For anything moving or spawned: light it from probes. Prefer **APV** on
  Unity 6 — per-pixel sampling avoids the "large mesh, one probe value" artifact.
- ✅ Enable APV **streaming** for open worlds: baked data is split into cells and
  only the cells in the camera frustum load, so the bake can exceed memory.
- ✅ Use **Lighting Scenarios** (APV) for day/night or lights-on/off, not a second
  set of baked lightmaps switched at runtime.
- ✅ Set Emissive materials' **Global Illumination** flag to `Baked` (or `None`) —
  `Realtime` GI is a separate, expensive system.
- ✅ Use **Rendering Layers** to keep a light off geometry it should not touch,
  instead of adding another light.
- ❌ **NEVER** expect a Baked light to affect a runtime-instantiated prefab — it
  cannot. That is the dark-prefab bug, every time.
- ❌ **NEVER** plan to convert Light Probe Groups into an APV — Unity offers no
  conversion, and APV probe positions cannot be moved by hand.
- ❌ **NEVER** bake with auto-generated lightmap UVs on a mesh whose UV2 is
  already authored — you get double work and seams.
- ❌ **NEVER** leave `Auto Generate` lighting on in a real project; it re-bakes
  on every scene edit.

## Few-Shot Examples

### Example 1: instantiated prefab is black
Scene is fully baked, the prefab spawns at runtime → it receives no lightmap.
Fixes, in order of preference:
1. Add an **Adaptive Probe Volume** covering the playable area (Unity 6).
2. Or place a **Light Probe Group** through the volume the prefab moves in.
3. Or, if the object spawns at a *fixed* known transform, bake its lightmap and
   restore the lightmap index/ST at instantiation — the pattern in
   `Ayfel/PrefabLightmapping`.

### Example 2: audit what is actually realtime
```bash
utk exec 'int rt = 0, mixed = 0, baked = 0;
foreach (var l in UnityEngine.Object.FindObjectsByType<UnityEngine.Light>(
    UnityEngine.FindObjectsInactive.Include, UnityEngine.FindObjectsSortMode.None))
{
  if (l.lightmapBakeType == UnityEngine.LightmapBakeType.Realtime) { rt++; Debug.Log("REALTIME " + l.name + " shadows=" + l.shadows); }
  else if (l.lightmapBakeType == UnityEngine.LightmapBakeType.Mixed) mixed++;
  else baked++;
}
Debug.Log("realtime=" + rt + " mixed=" + mixed + " baked=" + baked);
return rt;'
```
On mobile that realtime count should be **1**. Each extra realtime light with
shadows on adds a shadow map render.

### Example 3: find meshes that will never be lightmapped
```bash
utk exec 'int n = 0;
foreach (var r in UnityEngine.Object.FindObjectsByType<UnityEngine.MeshRenderer>(
    UnityEngine.FindObjectsSortMode.None))
{
  var flags = UnityEditor.GameObjectUtility.GetStaticEditorFlags(r.gameObject);
  if ((flags & UnityEditor.StaticEditorFlags.ContributeGI) == 0) { Debug.Log(r.name); n++; }
}
return n;'
```

## Bake Cost Checklist
Long bakes are almost always one of these:
1. Lightmap **resolution** (texels per unit) too high globally — lower it, then
   raise **Scale In Lightmap** on the few objects that need it.
2. A huge mesh with a tiny lightmap scale, or many small meshes each claiming a
   full chart. Combine static geometry first.
3. Too many **Bounces** / high **Sample** counts for a first look — bake draft
   quality while iterating.
4. `Auto Generate` on, re-baking silently on every change.
5. Lightmap UVs regenerated per import on heavy meshes.

## Debugging Checklist
1. Dynamic/instantiated object dark → no probe coverage; add APV or probes.
2. Large object lit flatly/wrongly → Light Probe Group per-object sampling; move to APV.
3. Light leaks through a wall → probe/brick resolution too coarse near the wall, or wall has no thickness.
4. Seams on a baked mesh → overlapping or unpadded UV2.
5. Reflections do not blend → Forward+ ignores Probe Blending; use one dominant probe or switch path.
6. Scene looks right in Editor, flat on device → probe data or APV cells not included in the build / streaming disabled.

## Related Skills
- `@unity-urp-setup` - Shadow distance, cascades, additional-light budget.
- `@unity-3d-rendering-performance` - What the light count costs per frame.
- `@unity-shader-authoring` - `Meta` pass, GI contribution from custom shaders.
- `@unity-addressables` - Shipping probe/lightmap data with streamed content.
