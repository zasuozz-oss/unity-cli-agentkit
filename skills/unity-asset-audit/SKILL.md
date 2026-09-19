---
name: unity-asset-audit
description: "Use when auditing Unity asset import settings: texture compression (ASTC/PVRTC), audio format, mesh read/write, material shader selection, sprite atlas packing, or model import optimization."
---

# Asset Verify Checklist — Unity Performance

> Comprehensive checklist for **assets, settings, import, rendering config, materials, textures, models, audio, scene setup** and all non-code factors affecting performance.
>
> **Usage**: Mark ✅ on items that have been checked and passed.
>
> **Marking convention**: `[x]` = Passed · `[ ]` = Not checked · `[!]` = Violation, needs fix · `[~]` = Not applicable

---

## 1. Rendering — General Settings

### 1.1 Draw Calls & Render Pass
- [ ] Draw calls < 100–150 on mobile (high-end may tolerate 200–300)
- [ ] Draw calls < 1000 on desktop
- [ ] Avoid multi-camera setup — each camera = 1 full render pass

### 1.2 Lighting & Shadows
- [ ] Set light modes to **Baked** (no realtime)
- [ ] Maximum 0–1 real-time per-pixel light on mobile
- [ ] Avoid real-time shadows on mobile
- [ ] Disable precomputed real-time GI
- [ ] Do not use real-time reflection probes
- [ ] Pre-bake: lighting, shadows, reflection probes
- [ ] Use baked lightmaps instead of real-time lighting on mobile
- [ ] Use light probes for dynamic objects in baked scenes

### 1.3 Rendering Pipeline
- [ ] Use **Forward rendering** and disable HDR on mobile
- [ ] Use **gamma** color space on mobile (avoid linear-to-sRGB conversion overhead)
- [ ] Try lighter APIs: Vulkan (Android), Metal (iOS)
- [ ] Disable depth prepass on mobile
- [ ] Clear only with pure black color

### 1.4 Post-Processing
- [ ] Full-screen post-processing is forbidden on mobile
- [ ] Heavy effects (bloom, AO, DoF, SSR, GI) — use very sparingly, lower quality on low-end
- [ ] Disable soft particles on mobile

### 1.5 VR-Specific
- [ ] Use single-pass stereo rendering
- [ ] Use foveated rendering to reduce fragment shading
- [ ] Set Oculus GPU/CPU hardware levels = 4
- [ ] Reduce eye/render resolution when necessary
- [ ] Avoid standard shaders on mobile VR
- [ ] Consider baking lighting onto diffuse texture for static elements
- [ ] Use RenderDoc for VR debugging

---

## 2. Batching & Instancing

### 2.1 SRP Batcher
- [ ] Enable SRP Batcher in URP/HDRP Asset settings
- [ ] Verify via Frame Debugger → check SRP Batch entries
- [ ] ⚠️ Shader variant explosion fragments batches → minimize shader keywords
- [ ] ⚠️ MaterialPropertyBlock makes renderers incompatible with SRP Batcher
- [ ] ⚠️ GPU Instancing and SRP Batcher are **mutually exclusive** per renderer/material

### 2.2 GPU Resident Drawer
- [ ] Enable for URP (Forward+ path) when many copies of same mesh (props, foliage, rocks)
- [ ] Project Settings > Graphics: set BatchRendererGroup Variants = **Keep All**
- [ ] URP: enable SRP Batcher → set GPU Resident Drawer = **Instanced Drawing** → Rendering Path = **Forward+**
- [ ] HDRP: set GPU Resident Drawer = **Instanced Drawing** in HDRP Asset
- [ ] ⚠️ Requires compute shaders — does not work on OpenGL ES
- [ ] ⚠️ Requirements: MeshRenderer, DOTS instancing shader, no MaterialPropertyBlock
- [ ] Verify via Frame Debugger → look for "Hybrid Batch Group" draw calls
- [ ] If using GPU Resident Drawer: **disable Static Batching** (they conflict)
- [ ] To exclude specific objects: add **Disallow GPU Driven Rendering** component

### 2.3 Static Batching
- [ ] Enable batching static flag on static elements
- [ ] Automatically merges static objects sharing a material — limit 64k vertices/indices per batch
- [ ] ⚠️ Increases memory (each mesh is duplicated into batched mesh)
- [ ] ⚠️ May break sort order → increased overdraw

### 2.4 GPU Instancing
- [ ] Enable per-material in Material Inspector ("Enable Instancing" checkbox)
- [ ] Works with non-static objects, same mesh + same material
- [ ] ⚠️ On URP/HDRP: mutually exclusive with SRP Batcher — profile both to choose

### 2.5 Dynamic Batching
- [ ] Legacy technique — only for meshes < 300 vertices, < 900 vertex attributes, single-pass shader
- [ ] ⚠️ Usually CPU cost of building batches > CPU cost saved — avoid unless profiling proves benefit

### 2.6 General Batching
- [ ] Enable static and dynamic batching in Player Settings
- [ ] Lack of effective batching = hundreds of individual prefabs = hundreds of draw calls

---

## 3. Materials & Shaders

### 3.1 Material Organization
- [ ] Merge materials using the same shader into shared materials + texture atlases
- [ ] Use **Material Variants** instead of duplicating materials (base material + overrides)
- [ ] Material Variants: keep keywords consistent (avoid shader variant explosion)
- [ ] Material Variants only for art direction knobs (tint, smoothness, normal strength) — NOT for toggling shader features
- [ ] Decimal values (metallic, specular): find common average or use channel packing (R/G/B/A)

### 3.2 Shader Optimization
- [ ] Inventory all shaders → use a small set of unique shaders, strip the rest
- [ ] Strip unused shaders
- [ ] Use Shader Stripping options to reduce variants
- [ ] ⚠️ Too many shader variants = increased build size + load times
- [ ] Pre-compile shaders to avoid runtime hiccups
- [ ] Preload commonly used shaders to avoid gameplay spikes
- [ ] Enable only necessary built-in shader settings in GraphicsSettings
- [ ] Complex shaders (multiple texture lookups, complex lighting) are expensive on mobile
- [ ] Consider simpler shaders (Unlit, Mobile/Diffuse) when possible on mobile

### 3.3 Verification
- [ ] Confirm all assets use the intended optimized formats with **A+ Asset Explorer**

---

## 4. Textures — Import Settings

### 4.1 Compression & Format
- [ ] Always use compressed formats — uncompressed consumes significantly more GPU memory + bandwidth
- [ ] Mobile: use **ASTC** on modern Android and iOS
- [ ] Mobile: use **PVRTC** on legacy iOS devices
- [ ] Set correct Texture Import Settings: Compressed format, Max Size, Mip Maps

### 4.2 Resolution & Memory
- [ ] Texture resolution must match on-screen size — don't use 4096×4096 for small objects
- [ ] Disable **Read/Write Enabled** unless specifically required (GetPixel, etc.) — enabled = **doubles memory**
- [ ] High-resolution textures directly consume vast amounts of RAM — optimize for target device
- [ ] Performance budget: < 150MB texture memory on mobile

### 4.3 Mipmaps
- [ ] Disable mipmaps where not needed → saves 33% memory
- [ ] ⚠️ Lack of mipmaps causes inefficient GPU sampling for distant objects → enable for 3D scene textures
- [ ] Consider texture streaming to load mipmap levels on demand (reduces memory but may show lower-res textures briefly)

### 4.4 Atlas & Optimization
- [ ] Optimize texture space usage with color palettes and efficient UVs
- [ ] Improve GPU caching by using unique big texture atlases (Mesh Baker)
- [ ] Atlas textures and reduce sizes
- [ ] Use **SpriteAtlas** to reduce UI draw calls (alternative: ShoeBox)
- [ ] Detect/remove duplicated content with **A+ Asset Explorer**

---

## 5. Models & Meshes

### 5.1 Polygon Budget
- [ ] < 200k–300k vertices on mobile
- [ ] Scene triangle count: 100k–500k/frame for mid-range mobile, up to 1–2M for high-end
- [ ] Optimize static environment meshes to be as low-poly as visually acceptable
- [ ] A 50k-triangle character model as a distant NPC is wasteful — match poly count to screen size

### 5.2 LOD & Culling
- [ ] High-poly models: use **LOD Groups**
- [ ] Use **CullingGroup** to disable renderers for off-screen objects
- [ ] Use **Occlusion Culling** for interiors / complex indoor / urban environments
- [ ] Mark static geometry: "Occluder Static" + "Occludee Static"
- [ ] Bake occlusion data: Window > Rendering > Occlusion Culling
- [ ] Consider impostor rendering / spritesheet animations for distant characters

### 5.3 Import Settings
- [ ] Set optimal compression settings for meshes
- [ ] Disable **Read/Write** on mesh import unless specifically required — enabled = **doubles memory**
- [ ] ⚠️ If using mesh colliders: Read/Write **must be enabled** on mesh importer
- [ ] Avoid flat-shading (it multiplies vertices)
- [ ] Remove geometry intersections and overlaps
- [ ] Reduce vertices while maintaining quality using normal + displacement maps

### 5.4 Tools
- [ ] Consider **Simplygon** to automatically reduce vertex count
- [ ] Use **Mesh Baker** to merge materials + textures

---

## 6. Animation — Asset Settings

- [ ] Enable **Optimize Game Objects** in rigging import settings
- [ ] Reduce blend tree complexity: < 6 blend nodes
- [ ] Reduce bone count for skinning: maximum **2 bones/vertex** on mobile
- [ ] Reduce triangle count for animated characters: < 3k on mobile
- [ ] Consider impostor rendering / spritesheet animations for distant characters
- [ ] Set optimal compression settings for animations
- [ ] Use **GPU skinning** in PlayerSettings (transfers skinning from CPU to GPU)
- [ ] ⚠️ GPU skinning: good when CPU-bound, bad when GPU-bound

---

## 7. Audio

### 7.1 Import Settings
- [ ] Short SFX: use **Decompress on Load**
- [ ] Long AudioClips: use **Streaming** or **Load in Background**
- [ ] ⚠️ Do not overuse Decompress on Load — only for short clips
- [ ] Compress audio files for mobile — uncompressed audio consumes vast amounts of RAM
- [ ] Performance budget: < 30MB audio memory on mobile

### 7.2 Audio Architecture
- [ ] Use **AudioMixer groups** for each category (Master → Music, SFX, Voice, Ambient)
- [ ] Pool AudioSources for frequent SFX — do NOT create new AudioSource per sound
- [ ] Use logarithmic scale (dB) for volume sliders, not linear 0–1
- [ ] Use **Audio Snapshots** for environmental transitions
- [ ] Crossfade music tracks using dual AudioSources instead of stop/start

---

## 8. Scene Hierarchy & Setup

- [ ] Scene hierarchies shallow: < 5 levels deep
- [ ] Scene hierarchies narrow: < 50 children per node
- [ ] Dynamic root objects: < 3 levels deep, < 50 total children
- [ ] Set static flags on static elements
- [ ] Keep packages up to date (Addressables, TextMesh Pro, etc.)

---

## 9. Canvas & UI — Asset Setup

### 9.1 Canvas Strategy
- [ ] Split canvases by update frequency: **Static** (never changes) · **HUD** (changes on events) · **Dynamic** (changes every frame)
- [ ] Disable `raycastTarget` on ALL non-interactive UI elements (Labels, decorative Images, backgrounds)
- [ ] ⚠️ Animating UI transforms on a canvas with many static children → full canvas rebuild → isolate animated elements
- [ ] Profile `Canvas.BuildBatch` and `Canvas.SendWillRenderCanvases` in CPU Profiler
- [ ] Use **TheGamedevGuru canvas rebuild detector** to find excessive rebuilds

### 9.2 Sprites & Overdraw
- [ ] Avoid stacking > 2 layers of UI on top of each other
- [ ] Unity UI Images render as full rectangles — transparent areas waste GPU fill rate
- [ ] Sprite: set Mesh Type = **Tight** (not Full Rect) in import settings
- [ ] Sprite Editor → Custom Outline mode → create outline following sprite content to reduce overdraw
- [ ] Favor **SpriteRenderer** over UI Image — SpriteRenderer supports tight mesh, reducing overdraw
- [ ] Create tighter meshes for sprite renderers using Sprite Editor
- [ ] Consider additive blending instead of alpha blend to reduce overdraw cost
- [ ] Consider opaque UI + alpha to coverage (advanced)

### 9.3 UI Architecture Choices
- [ ] **Fine-Grained UI**: many small elements → low overdraw but high draw calls → good for GPU-bound
- [ ] **Coarse-Grained UI**: bake into one large sprite → few draw calls but high overdraw → good for CPU-bound
- [ ] **Massive PP UI**: SpriteRenderer + tight mesh + opaque shader + stencil → most optimal but highest effort
- [ ] Use **UI Toolkit ListView** (virtualization) for lists of 100+ items instead of ScrollRect + manual pooling
- [ ] Consider **TSTableView** for tables

### 9.4 Frame Debugger
- [ ] Use Frame Debugger to check opaque geometry renders front-to-back
- [ ] Be willing to disable batching if it breaks optimal object sorting

---

## 10. Overdraw — Particles & Effects

- [ ] Cut transparent regions in sprites
- [ ] Reduce particle count
- [ ] Reduce particle sizes
- [ ] Keep particle systems small in world space (small bounding boxes)
- [ ] Use simpler shaders for particles
- [ ] Use GPU-instanced particles for large numbers
- [ ] Reduce overlapping transparent particles → reduce overdraw
- [ ] ⚠️ Many semi-transparent particles + complex alpha UI overlapping = severe overdraw
- [ ] Use Frame Debugger to visualize overdraw

---

## 11. Memory — Build Size & Loading

### 11.1 Build
- [ ] Measure build size components with **Build Report Tool**
- [ ] Use **LZ4** for fast compression (addressable asset bundles)
- [ ] Use **LZMA** for maximum compression (slower)
- [ ] Reduce build sizes with CDNs
- [ ] Consider Unity Tiny for ultra-small builds
- [ ] Performance budget: < 150MB app size (store initial download limit)

### 11.2 Runtime Memory
- [ ] Remove direct references to inaccessible content
- [ ] Read/Write Enabled = doubles memory (Unity keeps copy in both CPU and GPU memory)
- [ ] Properly unload assets when leaving a level — avoid memory leaks
- [ ] ⚠️ Frequent Instantiation/Destruction → memory fragmentation

---

## 12. Mobile-Specific Settings

### 12.1 Build Settings
- [ ] Architecture: target **ARM64 only** (not ARMv7) in Player Settings
- [ ] Scripting Backend: **IL2CPP** (not Mono) for both Android and iOS
- [ ] Minimum API: Android 7 (API 24) / iOS 14

### 12.2 Performance Budgets
- [ ] < 150MB texture memory
- [ ] < 30MB audio memory
- [ ] < 150MB app size (store initial download)
- [ ] < 33ms frame time (30fps) / < 16ms (60fps)

### 12.3 Adaptive Performance
- [ ] Use `OnDemandRendering.renderFrameInterval` for idle/menu screens → reduce GPU work + save battery
- [ ] Use `Screen.SetResolution` for dynamic resolution scaling based on device tier
- [ ] Implement **Adaptive Quality system**: auto-detect device tier (RAM, CPU cores) → apply appropriate Quality Level
- [ ] Consider lower resolution rendering on low-end devices (50–75% of native)

---

## 13. Performance Trade-offs Reference

| Technique | Benefit | Trade-off |
|-----------|---------|-----------|
| Static Batching | ↓ CPU draw calls | ↑ Memory (mesh duplication), may ↑ overdraw (sort order broken) |
| Addressables | ↓ Loading times | ↑ Content update overhead, ↑ Memory if not unloaded |
| SpriteRenderer vs Image | ↑ CPU (fewer canvas rebuilds) + ↑ GPU (tight mesh) | Harder for UI layout |
| GPU Skinning | Moves skinning to GPU | Good when CPU-bound, bad when GPU-bound |
| Texture Streaming | ↓ Memory | May show lower-res textures briefly |
| Dynamic Batching | May reduce draw calls | CPU cost of building batches usually > savings |
| Incremental GC | Smoother GC pauses | Total GC time may slightly increase |

---

## 14. Tools Recommended

| Tool | Purpose |
|------|---------|
| **Frame Debugger** | Verify draw calls, SRP Batch, Hybrid Batch Group, overdraw |
| **Unity Profiler** | CPU vs GPU bound, render thread bottleneck |
| **Memory Profiler** | Detailed memory snapshots |
| **A+ Asset Explorer** | Inventory shaders, verify formats, detect duplicates |
| **Mesh Baker** | Merge materials, texture atlases, combine meshes |
| **SpriteAtlas** | Reduce UI draw calls |
| **Sprite Editor** | Tight meshes for sprites |
| **Mali Offline Shader Compiler** | Measure shader complexity for ARM GPUs |
| **RenderDoc** | Detailed GPU debugging (especially VR) |
| **Build Report Tool** | Analyze build size components |
| **ShoeBox** | Alternative sprite atlasing |
| **TheGamedevGuru Rebuild Detector** | Detect excessive canvas rebuilds |

---

## Few-Shot Examples

### Example 1: Texture Import Audit (File-Based)
**User**: "Audit texture import settings in Assets/UI/Textures."

**Agent**:
```
# Reading .meta files for texture import settings...

find Assets/UI/Textures -name "*.png.meta" -o -name "*.jpg.meta" | head -20
# Found 34 texture assets. Checking import settings...
```

```
# Findings — Texture Import (§4)

🔴 §4.1 Texture Compression — 12 textures using Uncompressed
   Files: bg_main.png, bg_shop.png, popup_frame.png ... (12 total)
   Setting: textureCompression = 0 (None)
   Fix: Set Override for Android → ASTC 6×6, iOS → ASTC 6×6

🟡 §4.2 Read/Write Enabled — 8 textures have Read/Write ON
   Files: icon_avatar.png, btn_primary.png ... (8 total)
   Impact: Doubles memory — each texture stored in both CPU + GPU
   Fix: Disable Read/Write in Import Settings (unless GetPixel is needed)

🟡 §4.3 Mipmaps enabled on UI textures — 22 of 34 textures
   Impact: +33% memory waste (UI textures are always screen-sized)
   Fix: Disable "Generate Mip Maps" for all UI textures

🟢 §4.2 Oversized textures — 3 textures at 2048×2048 for small UI icons
   Files: icon_star.png (64×64 on screen), icon_gem.png, icon_coin.png
   Fix: Reduce Max Size to 256 or 512

Summary: 34 checked / 9 passed / 25 violations
🔴 Critical: 12 | 🟡 High: 30 | 🟢 Low: 3
```

### Example 2: Audio Settings Audit
**User**: "Check audio import settings for mobile optimization."

**Agent**:
```
# Scanning audio assets...
find Assets -name "*.wav.meta" -o -name "*.mp3.meta" -o -name "*.ogg.meta" | wc -l
# Found 47 audio files. Reading .meta importers...
```

```
# Findings — Audio (§7)

🔴 §7.1 Uncompressed audio on mobile — 5 music tracks using PCM
   Files: bgm_main.wav (18MB), bgm_battle.wav (22MB) ... (5 total)
   Impact: ~85MB uncompressed audio in RAM
   Fix: Set Load Type → Streaming for long tracks (>10s)

🟡 §7.1 Short SFX using Streaming — 12 sound effects
   Files: sfx_click.wav, sfx_coin.wav ... (12 total)
   Impact: Streaming overhead for tiny files (<100KB)
   Fix: Set Load Type → Decompress on Load for short clips (<1s)

🟡 §7.2 No AudioMixer groups configured
   Impact: Cannot adjust volume categories independently
   Fix: Create AudioMixer with groups: Master → Music, SFX, Voice, Ambient

🟢 §7.1 WAV format used instead of compressed — 15 SFX files
   Impact: Build size bloat (~45MB could be ~8MB with Vorbis)
   Fix: Set Compression Format → Vorbis (Quality 70%)

Summary: 47 checked / 15 passed / 32 violations
Budget: ~103MB audio memory (limit: 30MB) — ⚠️ OVER BUDGET
```

### Example 3: Canvas & UI Overdraw Audit (utk Mode)
**User**: "Audit UI canvas setup and overdraw in MainMenu scene."

**Agent**:
```
# Using utk exec to inspect scene hierarchy...
utk exec 'return FindObjectsOfType<Canvas>().Select(c => c.name + ":" + c.GetComponentsInChildren<Transform>().Length).ToArray();'
# Found 1 Canvas — "MainCanvas" with 847 children

# Checking raycastTarget on non-interactive elements...
```

```
# Findings — Canvas & UI (§9)

🔴 §9.1 Single canvas for entire UI — 847 elements on 1 canvas
   Impact: ANY change rebuilds entire canvas (Canvas.BuildBatch spike)
   Fix: Split into 3 sub-canvases:
   - StaticCanvas (background, frames — never changes)
   - HUDCanvas (scores, currency — event-driven updates)
   - DynamicCanvas (animations, timers — per-frame changes)

🟡 §9.1 raycastTarget enabled on 312 non-interactive elements
   Impact: Extra physics queries on every touch input
   Fix: Bulk-disable raycastTarget on Labels, decorative Images, backgrounds
   Tool: Use RaycastTargetOptimizer editor script (see unity-ui-performance skill)

🟡 §9.2 UI overdraw — 4 stacked full-screen layers detected
   Layers: bg_gradient → bg_pattern → panel_frame → content_bg
   Impact: 4× fill rate for entire screen area
   Fix: Merge bg layers into single pre-composited sprite, or use opaque shader

🟢 §9.2 Sprite Mesh Type = Full Rect on 28 sprites
   Impact: Transparent areas rendered as opaque rectangles
   Fix: Set Mesh Type → Tight in Sprite Import Settings

Summary: 847 elements checked / 205 passed / 642 violations
Draw calls from UI: 89 (budget: < 50) — ⚠️ OVER BUDGET
```

---

## Related Skills
- `@unity-ui-performance` — UI performance optimization and state safety
- `@unity-addressables` — Asset loading and memory-safe release patterns
