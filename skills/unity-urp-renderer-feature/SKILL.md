---
name: unity-urp-renderer-feature
description: "Use when writing or fixing a URP ScriptableRendererFeature / ScriptableRenderPass in Unity 6: Render Graph API (RecordRenderGraph, AddRasterRenderPass, UseTexture, SetRenderFunc), porting legacy Execute()/Blit() passes, blit and full-screen effects, injection points, or errors about undeclared resources and pass culling."
---

# URP Renderer Features (Render Graph)

## Overview
Unity 6 (URP 17) replaced the imperative `ScriptableRenderPass.Execute(context,
ref renderingData)` with the **Render Graph**: you *declare* a pass's inputs and
outputs in `RecordRenderGraph`, and Unity records the actual commands later.
Almost every wrong Renderer Feature an agent writes is legacy-API code — it
compiles under Compatibility Mode and then silently does nothing, or throws
about resources it never declared.

## When to Use
- Writing a new Renderer Feature / full-screen effect on Unity 6.
- Porting a 2022/2023-era pass that used `Execute()`, `cmd.Blit`, `ConfigureTarget`.
- A pass compiles but never renders, or gets culled out of the graph.
- Errors mentioning undeclared textures, or reading and writing the same target.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Declare, then record** | `RecordRenderGraph` only declares inputs/outputs. It must not add commands to a command buffer. |
| **PassData** | A plain class holding *only* what the render function needs. Extra fields cost performance. |
| **Static render func** | `SetRenderFunc` must take a static method or static lambda. A lambda capturing `this` allocates and outlives the frame. |
| **Resource handles** | Textures are `TextureHandle`s obtained from `frameData.Get<UniversalResourceData>()`, not `RTHandle`s you allocate. |
| **Pass culling** | A pass whose outputs nothing consumes is removed from the graph. "My pass never runs" is usually this. |
| **Compatibility Mode** | Legacy `Execute()` path, kept behind a Graphics setting and deprecated. Treat it as migration-only, not a target. |

## Canonical Skeleton
```csharp
public class MyPass : ScriptableRenderPass
{
    class PassData
    {
        public TextureHandle source;
        public Material material;
    }

    public override void RecordRenderGraph(RenderGraph renderGraph, ContextContainer frameData)
    {
        var resourceData = frameData.Get<UniversalResourceData>();
        var cameraData   = frameData.Get<UniversalCameraData>();

        using (var builder = renderGraph.AddRasterRenderPass<PassData>("My Pass", out var passData))
        {
            passData.source   = resourceData.activeColorTexture;
            passData.material = _material;

            builder.UseTexture(passData.source);                  // read
            builder.SetRenderAttachment(destination, 0);          // write
            builder.SetRenderFunc(static (PassData data, RasterGraphContext ctx) => Execute(data, ctx));
        }
    }

    static void Execute(PassData data, RasterGraphContext ctx)
    {
        // ctx.cmd only — no context.ExecuteCommandBuffer, no state set outside this func
        Blitter.BlitTexture(ctx.cmd, data.source, new Vector4(1, 1, 0, 0), data.material, 0);
    }
}
```

## Best Practices
- ✅ Pick the right builder: `AddRasterRenderPass` for raster work,
  `AddComputePass` for compute, `AddUnsafePass` **only** when you genuinely need
  legacy `cmd.SetRenderTarget`-style control.
- ✅ Get every texture from `UniversalResourceData` (`activeColorTexture`,
  `activeDepthTexture`, `cameraDepthTexture`, `cameraNormalsTexture`).
- ✅ Allocate scratch targets as render-graph textures so the graph can alias and
  free them — never a hand-managed `RTHandle` allocated per frame.
- ✅ Set `renderPassEvent` deliberately; `AfterRenderingPostProcessing` and
  `BeforeRenderingTransparents` behave very differently for a full-screen effect.
- ✅ Dispose materials/resources in the feature's `Dispose(bool)`.
- ✅ Use the **Render Graph Viewer** (Window > Analysis) to see whether the pass
  was recorded, merged, or culled, and which resources it actually touched.
- ❌ **NEVER** read and write the same texture in one raster pass — ping-pong
  between two handles.
- ❌ **NEVER** touch a texture you did not declare with `UseTexture` /
  `SetRenderAttachment`. Undeclared access is the #1 runtime error.
- ❌ **NEVER** capture `this` or a member field in the `SetRenderFunc` lambda —
  copy it into `PassData` first.
- ❌ **NEVER** call `CommandBufferHelpers`/`cmd.Blit` legacy blits; use `Blitter`
  or the render graph's blit helper.
- ❌ **NEVER** write a new pass against Compatibility Mode. Port instead.

## Few-Shot Examples

### Example 1: legacy pass → Render Graph
```csharp
// ❌ BEFORE (URP 14 style — compiles, does nothing under Render Graph)
public override void Execute(ScriptableRenderContext context, ref RenderingData renderingData)
{
    var cmd = CommandBufferPool.Get("My Pass");
    cmd.Blit(source, dest, _material);
    context.ExecuteCommandBuffer(cmd);
    CommandBufferPool.Release(cmd);
}

// ✅ AFTER — see the canonical skeleton above: declare in RecordRenderGraph,
//    blit inside a static render func with ctx.cmd.
```

### Example 2: pass never runs
Symptom: breakpoint in `RecordRenderGraph` hits, nothing renders.
Cause: the pass writes to a texture nothing downstream reads, so the graph culls
it. Fix: write into `resourceData.activeColorTexture` (or mark the output as
consumed), and confirm in the Render Graph Viewer that the pass survived.

### Example 3: capturing `this` by accident
```csharp
// ❌ allocates every frame, reads a field that may have changed by execution time
builder.SetRenderFunc((PassData data, RasterGraphContext ctx) =>
    Blitter.BlitTexture(ctx.cmd, data.source, _scale, _material, 0));

// ✅ copy into PassData, keep the lambda static
passData.scale = _scale; passData.material = _material;
builder.SetRenderFunc(static (PassData data, RasterGraphContext ctx) =>
    Blitter.BlitTexture(ctx.cmd, data.source, data.scale, data.material, 0));
```

## Verifying with `utk`
```bash
utk editor refresh                 # blocks on the compile, exits non-zero with errors
utk console --type error           # graph-time errors surface here, not at compile time
utk editor play && utk screenshot  # confirm the effect actually reaches the frame
```
Render Graph errors are **runtime** errors — a clean `utk editor refresh` proves
nothing about a pass. Always enter play mode (or Game view) once and read the
console before reporting the feature done.

## Version Note
Helper names moved between URP 17.0 (Unity 6.0) and 17.1+. Before using
`renderGraph.AddBlitPass`, `RenderGraphUtils.BlitMaterialParameters`, or
`UniversalRenderer.CreateRenderGraphTexture`, check the installed package:
```bash
utk exec 'Debug.Log(UnityEditor.PackageManager.PackageInfo.FindForAssetPath(
  "Packages/com.unity.render-pipelines.universal/package.json").version);'
```

## Debugging Checklist
1. Nothing renders → pass culled (Render Graph Viewer) or wrong `renderPassEvent`.
2. Runtime error about a resource → missing `UseTexture` / `SetRenderAttachment`.
3. Flicker or garbage → reading and writing the same handle; ping-pong.
4. GC spike per frame → non-static render func capturing members.
5. Works in Editor, black on device → effect depends on Depth/Opaque Texture that is disabled in the mobile URP Asset.

## Related Skills
- `@unity-urp-setup` - Renderer Data settings, intermediate texture, injection cost.
- `@unity-shader-authoring` - The material/shader the pass blits with.
- `@utk-test-runner` - Compile verification loop.
