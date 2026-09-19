---
name: unity-shader-authoring
description: "Use when writing or debugging URP shaders — HLSL ShaderLab or Shader Graph: SRP Batcher compatibility, custom lighting (main/additional lights, shadows, GI), required LightMode passes (ShadowCaster/DepthNormals/Meta), keywords and variant stripping, MaterialPropertyBlock breakage, or a material that renders pink / casts no shadow."
---

# URP Shader Authoring

## Overview
A URP shader is only "correct" when it is **SRP Batcher compatible**, declares
**every pass URP asks for**, and handles the **keywords** of the active rendering
path. Missing any one produces a specific, recognisable failure — a batching
regression, a missing shadow, or a black object under additional lights — not a
compile error.

## When to Use
- Writing a custom lit/unlit shader for URP, by hand or in Shader Graph.
- An object renders pink, casts no shadow, or is ignored by SSAO/decals.
- SRP Batcher shows "not compatible" in the shader Inspector.
- Adding custom lighting (toon, ramp, rim) that must still respect shadows and GI.
- Build size or shader compile time exploded from variants.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **SRP Batcher** | Keeps material constant buffers resident in GPU memory instead of rebinding per draw. It reduces *state changes*, not draw call count. |
| **`UnityPerMaterial`** | One CBUFFER holding **every** material property, declared **identically in every pass** of the shader. Any divergence, or a material property outside it, breaks compatibility. |
| **`UnityPerDraw`** | Per-object data URP fills in (transforms, light probes, lightmap ST). Do not add material data here. |
| **LightMode pass tags** | `UniversalForward` (or `UniversalForwardOnly`), `ShadowCaster`, `DepthOnly`, `DepthNormals`, `Meta`. Each missing pass disables a specific feature. |
| **Keyword axes** | Shadows, cascades, additional lights, light cookies, lightmaps, fog, `_FORWARD_PLUS`. Missing keywords = feature silently off, not an error. |
| **Shader Graph** | SRP Batcher compatible and emits the required passes by default — prefer it unless you need lighting maths it cannot express. |

## Best Practices
- ✅ Declare all material properties inside one `CBUFFER_START(UnityPerMaterial)`
  block, and keep it byte-identical across passes (put it in a shared `.hlsl`).
- ✅ Always ship a `ShadowCaster` pass (cast) and a `DepthNormals` pass
  (SSAO, screen-space decals, depth-normals-based effects need it).
- ✅ Add a `Meta` pass on anything that should contribute to baked GI.
- ✅ Include `Packages/com.unity.render-pipelines.universal/ShaderLibrary/Lighting.hlsl`
  and use `GetMainLight(shadowCoord)` / `GetAdditionalLightsCount()` +
  `GetAdditionalLight(i, positionWS)` rather than reinventing light lookup.
- ✅ For Shader Graph custom lighting, reuse the community-standard sub-graphs
  (Main Light, Main Light Shadows, Additional Lights, Sample Shadowmask, Mix Fog)
  instead of hand-wiring `_MainLightPosition` — they already handle cascades,
  Forward+, cookies, and shadowmask.
- ✅ Use `shader_feature` for anything toggled per material (stripped when unused);
  reserve `multi_compile` for state that changes at runtime.
- ✅ Check the Inspector line: selecting the shader asset shows **SRP Batcher:
  compatible / not compatible** plus the reason.
- ❌ **NEVER** use `MaterialPropertyBlock` to vary a color/float per renderer —
  it takes the object off the SRP Batcher path. Use per-material variants or GPU
  instancing (`UNITY_INSTANCING_BUFFER`) instead.
- ❌ **NEVER** sample `_CameraOpaqueTexture` / `_CameraDepthTexture` without
  enabling them in the URP Asset — on mobile each adds a full-screen copy, and
  when disabled the sample returns garbage.
- ❌ **NEVER** leave a property out of `UnityPerMaterial` "because it is only used
  in one pass" — the CBUFFER layout must match everywhere.
- ❌ **NEVER** author a Built-in RP shader (`UnityCG.cginc`, `LightMode=ForwardBase`)
  for a URP project. That is the pink-material cause.

## Few-Shot Examples

### Example 1: fixing SRP Batcher incompatibility
```hlsl
// ❌ BEFORE — _Color lives outside the CBUFFER; batcher reports "not compatible"
float4 _Color;
CBUFFER_START(UnityPerMaterial)
    float4 _BaseMap_ST;
CBUFFER_END

// ✅ AFTER — every material property inside, same order in every pass
CBUFFER_START(UnityPerMaterial)
    float4 _BaseMap_ST;
    float4 _Color;
    float  _Smoothness;
CBUFFER_END
```

### Example 2: object has no shadow
Missing `ShadowCaster`. Minimum viable pass:
```hlsl
Pass
{
    Name "ShadowCaster"
    Tags { "LightMode" = "ShadowCaster" }
    ZWrite On ZTest LEqual ColorMask 0
    HLSLPROGRAM
    #pragma vertex ShadowPassVertex
    #pragma fragment ShadowPassFragment
    #pragma multi_compile_instancing
    #include "Packages/com.unity.render-pipelines.universal/Shaders/ShadowCasterPass.hlsl"
    ENDHLSL
}
```

### Example 3: per-instance colour without breaking batching
```csharp
// ❌ BEFORE
var mpb = new MaterialPropertyBlock();
mpb.SetColor("_Color", c);
renderer.SetPropertyBlock(mpb);        // drops out of the SRP Batcher

// ✅ AFTER — GPU instancing path, shader declares the property per-instance
UNITY_INSTANCING_BUFFER_START(Props)
    UNITY_DEFINE_INSTANCED_PROP(float4, _Color)
UNITY_INSTANCING_BUFFER_END(Props)
```

### Example 4: additional lights in hand-written HLSL
```hlsl
uint count = GetAdditionalLightsCount();
for (uint i = 0u; i < count; ++i)
{
    Light l = GetAdditionalLight(i, positionWS, shadowMask);
    float atten = l.distanceAttenuation * l.shadowAttenuation;
    color += l.color * atten * saturate(dot(normalWS, l.direction));
}
```
This loop is Forward+ aware only when the `_FORWARD_PLUS` keyword variants are
compiled — do not strip it.

## Verifying with `utk`
```bash
# Compile errors/warnings for one shader
utk exec 'var sh = UnityEditor.AssetDatabase.LoadAssetAtPath<UnityEngine.Shader>("Assets/Shaders/MyLit.shader");
Debug.Log("messages: " + UnityEditor.ShaderUtil.GetShaderMessageCount(sh));
foreach (var m in UnityEditor.ShaderUtil.GetShaderMessages(sh)) Debug.Log(m.severity + " " + m.line + ": " + m.message);'

# Find every material still on a Built-in RP shader (pink-material sweep)
utk exec 'int bad = 0;
foreach (var g in UnityEditor.AssetDatabase.FindAssets("t:Material")) {
  var p = UnityEditor.AssetDatabase.GUIDToAssetPath(g);
  var m = UnityEditor.AssetDatabase.LoadAssetAtPath<UnityEngine.Material>(p);
  if (m != null && m.shader != null && m.shader.name.StartsWith("Standard")) { Debug.Log(p); bad++; }
}
return bad;'
```
SRP Batcher compatibility itself has no public API — read it off the shader
asset's Inspector, or use the Frame Debugger's "SRP Batch" node count.

## Reference Implementations
Read these before writing lighting maths from scratch:
- `ColinLeung-NiloCat/UnityURPToonLitShaderExample` — a fully commented custom
  URP lit shader written for teaching; the SRP Batcher and mobile notes are the
  valuable part.
- `Cyanilux/URP_ShaderGraphCustomLighting` — Main Light / Additional Lights /
  Shadowmask / Subtractive GI / Mix Fog sub-graphs, kept current with URP 17.1+.
- `phi-lira/UniversalShaderExamples` — unlit → PBR → clear coat, by URP's lead.
- `UnityTechnologies/ShaderGraph_ExampleLibrary` — first-party graph patterns.

## Debugging Checklist
1. Pink material → Built-in RP shader, or shader failed to compile (check messages).
2. No shadow cast → missing `ShadowCaster` pass.
3. SSAO / screen-space decals skip the object → missing `DepthNormals` pass.
4. Object black under point lights → additional-lights loop or its keywords missing.
5. Batch count exploded → property outside `UnityPerMaterial`, or a `MaterialPropertyBlock`.
6. Build/compile time exploded → `multi_compile` where `shader_feature` would do.

## Related Skills
- `@unity-urp-setup` - Which textures/keywords the active URP Asset actually enables.
- `@unity-urp-renderer-feature` - Driving a shader from a full-screen pass.
- `@unity-3d-rendering-performance` - Measuring the batching effect of a shader change.
