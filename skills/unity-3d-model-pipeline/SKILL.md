---
name: unity-3d-model-pipeline
description: "Use when importing or fixing 3D models in Unity: FBX/glTF import settings, mesh compression and Read/Write, normals and tangents, scale factor, LOD Groups, rig setup (Humanoid vs Generic, Optimize Game Objects, skin weights), material import and remapping, or URP Lit texture channel packing."
---

# 3D Model Import Pipeline

## Overview
Import settings decide runtime memory, mesh cost, and whether a rig animates at
all — and they are set once, per asset, usually wrong by default. The two
expensive defaults are **Read/Write Enabled** (doubles mesh memory by keeping a
CPU copy) and **Import Materials** (creates a new material per FBX instead of
reusing yours, exploding the SRP batch count).

## When to Use
- Bringing an FBX/glTF/OBJ into the project, or fixing one already there.
- Model comes in at the wrong scale or rotated 90°.
- Lighting on an imported mesh looks faceted, inverted, or shimmery.
- Setting up LODs, or a character rig that must retarget.
- Mesh memory or build size is over budget.
- Assigning URP Lit maps and getting metallic/smoothness wrong.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Read/Write Enabled** | Keeps a CPU-side copy of the mesh. Needed only for runtime mesh access (`mesh.vertices`, some colliders/baking). Otherwise pure waste. |
| **Mesh Compression** | Quantises vertex data on disk. Reduces build size, can introduce artifacts on large meshes. Off by default. |
| **Optimize Mesh** | Reorders vertices/indices for GPU cache coherency. Free win, leave on. |
| **Normals / Tangents** | `Import` uses the DCC's data; `Calculate` recomputes with a smoothing angle. Mismatched choices are the cause of faceted or seam-lit meshes. |
| **Scale Factor / Convert Units** | Unity is metre-based, Y-up, left-handed. Wrong scale here propagates into physics, not just visuals. |
| **Rig type** | `Generic` for anything non-biped (cheaper, exact bones). `Humanoid` only when retargeting between characters is actually needed. |
| **Optimize Game Objects** | Strips the bone `Transform` hierarchy, leaving only exposed transforms. Large CPU win for characters; anything that must find a bone by name has to be in the exposed list. |
| **Skin Weights** | Bones per vertex. Quality Settings caps it project-wide — `2 Bones` or `4 Bones` on mobile. |

## Import Settings Defaults
- ✅ **Read/Write Enabled: off** unless the mesh is read at runtime.
- ✅ **Optimize Mesh: on**; **Weld Vertices: on** for static props.
- ✅ **Normals: Import** when the artist authored them; `Calculate` with a
  matching smoothing angle otherwise. Pick one and keep the whole asset set consistent.
- ✅ **Tangents: Calculate Mikktspace** unless the DCC exported tangents that the
  normal maps were baked against.
- ✅ **Import Materials: off** — remap to existing project materials via
  *Materials > Search and Remap*, so one material serves many models.
- ✅ **Import Cameras / Lights / Visibility: off** for prop FBXs.
- ✅ **Blend Shape Normals: None** on characters that do not need them.
- ✅ Name LOD meshes `<Name>_LOD0`, `_LOD1`, … in the DCC — Unity builds the
  **LOD Group** on import automatically.
- ❌ **NEVER** leave Read/Write on across an asset library "in case something
  needs it" — it is a per-mesh memory doubling.
- ❌ **NEVER** fix scale by scaling the root Transform. Fix **Scale Factor** on
  the importer, or the export units in the DCC. A scaled root breaks physics,
  non-uniform child scaling, and batching.
- ❌ **NEVER** switch a rig to Humanoid unless retargeting is needed — the avatar
  mapping adds cost and a whole class of "arms are wrong" bugs.

## URP Lit Texture Channels
URP's **Lit** shader does *not* use an HDRP-style packed mask map. Get this right
or metals read as dielectrics:

| Map | Channels |
|---|---|
| **Base Map** | RGB = albedo, A = alpha (or smoothness, when *Smoothness Source = Albedo Alpha*) |
| **Metallic Map** | R = metallic, A = smoothness (when *Smoothness Source = Metallic Alpha*, the default) |
| **Occlusion Map** | G channel is read |
| **Normal Map** | Tangent-space; the texture must be imported as *Normal map* type |

- ✅ Author metallic and smoothness into **one** texture (R + A) — it is one
  sample instead of two.
- ❌ **NEVER** feed a grayscale roughness map straight into smoothness — invert it.

## Few-Shot Examples

### Example 1: import and configure in two calls
```bash
utk import ~/Downloads/rock_01.fbx Assets/Art/Props     # copies + imports, returns the GUID
utk set_import_settings --path Assets/Art/Props/rock_01.fbx \
  --settings '{"isReadable": false, "optimizeMesh": true, "importMaterials": false}'
utk editor refresh
```
Never push model bytes through `utk exec` as base64 — get the file on disk, then
import by path (see `@utk-asset-import`).

### Example 2: audit meshes wasting memory
```bash
utk exec 'int readable = 0; long verts = 0;
foreach (var g in UnityEditor.AssetDatabase.FindAssets("t:Mesh")) {
  var p = UnityEditor.AssetDatabase.GUIDToAssetPath(g);
  var m = UnityEditor.AssetDatabase.LoadAssetAtPath<UnityEngine.Mesh>(p);
  if (m == null) continue;
  verts += m.vertexCount;
  if (m.isReadable) { readable++; Debug.Log("READABLE " + p + " verts=" + m.vertexCount); }
}
Debug.Log("readableMeshes=" + readable + " totalVerts=" + verts);
return readable;'
```
Every readable mesh listed there is a mesh stored twice. Turn Read/Write off on
the ones nothing reads at runtime.

### Example 3: bulk-fix importer flags
```bash
utk exec 'int n = 0;
foreach (var g in UnityEditor.AssetDatabase.FindAssets("t:Model", new string[]{"Assets/Art/Props"})) {
  var p = UnityEditor.AssetDatabase.GUIDToAssetPath(g);
  var mi = UnityEditor.AssetImporter.GetAtPath(p) as UnityEditor.ModelImporter;
  if (mi == null || !mi.isReadable) continue;
  mi.isReadable = false; mi.optimizeMeshPolygons = true; mi.optimizeMeshVertices = true;
  UnityEditor.AssetDatabase.ImportAsset(p); n++;
}
UnityEditor.AssetDatabase.SaveAssets();
return n;'
```
Reimporting a folder of models is slow — run it in the background and read the
result, never in a poll loop.

## Debugging Checklist
1. Model 100× too big/small → Scale Factor / DCC export units, not the Transform.
2. Faceted or blotchy shading → Normals `Calculate` with the wrong smoothing angle, or missing tangents.
3. Normal map looks inverted → green-channel convention (DirectX vs OpenGL); flip at export or in the texture.
4. Character T-poses / limbs wrong → Humanoid avatar mapping; try Generic.
5. Bone lookup by name returns null → *Optimize Game Objects* stripped it; add it to the exposed transform list.
6. Batch count high after adding models → *Import Materials* created one material per FBX; remap them.
7. Memory over budget with few meshes → Read/Write Enabled left on.

## Related Skills
- `@utk-asset-import` - Getting the file into `Assets/` by path and tuning the importer.
- `@unity-3d-rendering-performance` - LOD, instancing, and what the mesh costs per frame.
- `@unity-3d-lighting` - Lightmap UV2 generation on imported meshes.
- `@unity-texture-pipeline` - Compression for the maps these materials sample.
- `@unity-asset-audit` - Project-wide sweep for oversized assets.
