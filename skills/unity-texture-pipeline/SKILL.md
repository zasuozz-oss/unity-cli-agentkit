---
name: unity-texture-pipeline
description: "Use when working on texture compression, ktx2/basis/ETC1S/ASTC transcode targets, texture downscaling or atlas/composite generation, or debugging blurry textures, colored halos/fringes on transparent edges, off-center sprites after resizing, or missing alpha detail."
---

# Unity Texture Pipeline

## Overview
Rules for compressing, resizing, and compositing textures offline. The recurring
failure is **treating a lossy intermediate as if it were the source**, or
**changing one dimension of the math and not the other**.

## When to Use
- Changing a transcode target (ETC1S/UASTC → ASTC/BC7/RGBA32) hoping for quality.
- Adding or fixing a downscale / atlas / composite step.
- Debugging halos, fringes, blur, off-center placement, or lost alpha detail.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **ETC1S is the ceiling** | Once encoded to ETC1S/basis, the detail is gone; the transcode target only decides how faithfully the *loss* is reproduced. |
| **Block padding vs. layout math** | ETC2/ASTC need dimensions rounded up to a block multiple (`(w+3)&~3`); every center/offset computed from that padded width is now wrong. |
| **Premultiplied downscale** | Filtering straight (non-premultiplied) RGBA blends transparent pixels' garbage RGB into visible edges → colored fringes. |
| **Alpha as a data channel** | Near-zero alpha may be deliberate (masks, IDs, hidden layers), not noise. |

## Best Practices
- ✅ **Measure before believing** a compression change helped:
  `basisu -unpack <file>` and diff against the lossless original. A target swap
  that "looks better" usually changed nothing measurable.
- ✅ Re-encode from the **lossless source**, never from the shipped ktx2.
- ✅ Keep the **original** width/height alongside the padded one; all placement,
  center, and UV math uses the original — padding is a storage detail only.
- ✅ Downscale premultiplied: blit through a premultiply shader, filter, then
  unpremultiply in place.
- ✅ Histogram the lossless original's alpha before raising any alpha threshold —
  confirm the low values are noise and not payload.
- ✅ A fixed-canvas compositor assumes **design-resolution** inputs. Keep the
  display-downscale path and the share/merge path separate, and **grep every
  caller** when you touch either — fixing one caller silently breaks the other.
- ❌ **NEVER** change the transcode target as a "quality fix" for an ETC1S asset.
- ❌ **NEVER** feed an already-downscaled texture into a compositor that expects
  design resolution.

## Few-Shot Examples

### Example 1: padding breaks centering
```csharp
// ❌ BEFORE: sprite drifts up-left after switching to ETC2
int padW = (srcW + 3) & ~3;
int padH = (srcH + 3) & ~3;
var center = new Vector2(padW * 0.5f, padH * 0.5f);   // padded, not real

// ✅ AFTER: pad storage, keep layout on the original size
var center = new Vector2(srcW * 0.5f, srcH * 0.5f);
```

### Example 2: verifying a compression change is real
```bash
basisu -unpack before.ktx2 -output_file before.png
basisu -unpack after.ktx2  -output_file after.png
# compare BOTH against the lossless source, not against each other
compare -metric RMSE source.png before.png null:
compare -metric RMSE source.png after.png  null:
```
Same RMSE → the target swap did nothing; the loss was baked in at encode time.

## Debugging Checklist
1. Blurry after a "fix" → was the input already ETC1S? Re-encode from source.
2. Colored halo on transparent edges → non-premultiplied filtering.
3. Off-center / cropped after a format change → padded dimension leaked into
   layout math.
4. Detail vanished at low alpha → threshold raised over a real data channel.
5. Wrong scale in one feature only → two callers, one shared compositor.

## Related Skills
- `@unity-asset-audit` - Finding oversized / mis-imported textures.
- `@unity-addressables` - Bundle compression vs. texture compression.
