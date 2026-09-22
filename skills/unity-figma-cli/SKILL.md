---
name: unity-figma-cli
description: Use when a Figma design has to become Unity uGUI — `figma-cli unity_export` + `FigmaHeadlessImporter.Import`, or reading a frame's geometry, colors, text and auto-layout by hand, exporting sprites (9-slice, halos), building the RectTransform hierarchy through `utk`, or checking a built screen against the design across aspect ratios.
---

# Figma → Unity uGUI

`figma-cli` reads the file the user has open in Figma Desktop; `utk` writes the
Unity scene. This skill is **only the bridge**. CLI mechanics — preflight,
`--params`, trimming, `no_cdp`/`ambiguous_tab`/`boot_failed` — live in the
`figma-cli` skill; read it first and don't restate it here.

Start with `figma-cli status`. Every failure it reports needs a human
(relaunch Figma, close a tab, save work) — report and hand back, never loop.

- **`status` ok is not "the right file is open".** One open tab is not
  ambiguous, but it can be a different design than the one you were told about.
  An empty `list_nodes`, a `get_node` on a known id that finds nothing, or an
  importer reporting the frame "may have been deleted" almost always means the
  wrong file is in front — check the tab `status` names (or pass `--file <key>`)
  before concluding the frame is gone.
- **`figma-cli` is not the npm package of that name.** `npm i -g figma-cli`
  installs an unrelated style-guide exporter; the real one is installed from its
  own clone (`npm install -g .`, Node 22+). If `figma-cli status` does not
  answer with `{cdp, tab, boot, …}`, the wrong binary is on PATH.
- **Never guess a tool or param name.** `figma-cli list` / `figma-cli list <tool>`
  first — a guessed name costs a usage page per try.

## Two routes — prefer the importer when the project has one

| Project has | Route |
|---|---|
| the `FigmaImporter` package (`FigmaImporter.FigmaHeadlessImporter` resolves) | `figma-cli unity_export` → `FigmaHeadlessImporter.Import` — one call builds the whole prefab: anchors from `constraints`, pixel-measured 9-slice, TMP font matching, layout groups, sprite dedup |
| no importer | the manual route: read geometry (below), export sprites, build with one `run_script` |

```sh
figma-cli unity_export --params '{"nodeId":"<frame>","outputDir":"<abs>/.unity-figma/<frame>","scale":2}'
utk exec 'return FigmaImporter.FigmaHeadlessImporter.Import("<abs>/.unity-figma/<frame>", "Prefab", "Assets/UI/Prefabs/", "Assets/UI/Sprites/");'
```

`Import` returns a JSON **string**: `{success, rootName, textureCount, outputMode, log[]}`.
Three things fail on a fresh project, all seen in real runs:

- **TMP Essentials not imported** → `success:false`, `"TextMeshPro Essentials
  are not imported"`. It is a one-time per-project import; do it before the
  first import, not after the failure:
  `AssetDatabase.ImportPackage("Packages/com.unity.ugui/Package Resources/TMP Essential Resources.unitypackage", false)`
  (older TMP-as-package projects: `Packages/com.unity.textmeshpro/...`).
- **Destination folder missing** → `"Given path does not exist"`. The importer
  does not create intermediate folders — `AssetDatabase.CreateFolder` each
  segment first.
- **`success:true` with `"Warning: … font \"X\" not found, using default"`**
  in `log[]`. This is the silent font fallback below, caught for you — **read
  `log[]` every time**; `success` alone says nothing about fonts.

The rest of this skill is the manual route, and the mapping/verification rules
apply to both.

## Read with two tools, not one

`list_nodes` gives one flat row per node; `get_node` gives the full property
set of a subtree. Both report geometry in **page-absolute** coordinates under
different names — no tool hands back a parent-relative box called `rect`.

| You need | Tool | Coordinates |
|---|---|---|
| tree shape, `fontName`, `letterSpacing`, `hasImage` (this node needs a PNG), `renderRect` (ink bounds incl. shadow) | `list_nodes --node <frame> --depth 6` | `rect` — **page-absolute** |
| `fills`/gradients, `cornerRadius`/`cornerRadii`, `constraints`, `effects`, auto-layout (`layoutMode` `itemSpacing` `padding*`), INSTANCE `mainComponentId`, full text metrics (`fontName` `lineHeight` `letterSpacing` `textAlignHorizontal/Vertical` `textAutoResize`, `styledSegments` for mixed runs) | `get_node --node <id> --depth 4` | `x`/`y` (trimmed: `box [x,y,w,h]`) **parent-relative**; `absRect` page-absolute; `absRenderRect` only when ink spills past the box |

- Older figma-cli builds returned no font or alignment from `get_node`. If a
  TEXT node comes back with only `characters`/`fontSize`, the CLI is stale —
  update it rather than guessing the face or alignment.
- Parent-relative from `list_nodes` = child `rect` − parent `rect`. Don't mix
  `list_nodes.rect` with `get_node.x/y` in one computation.
- Dump both to files and let the build script read them:

```sh
figma-cli list_nodes --node <frame-id> --depth 6 > /tmp/fig/tree.json
figma-cli get_node   --node <frame-id> --depth 4 > /tmp/fig/geo.json
```

Do not retype coordinates node by node into your context.

## The mapping

Figma measures Y **down** from the parent's top-left. Putting both anchor and
pivot at the parent's top-left is the only mapping with no per-node math:

```csharp
rt.anchorMin = rt.anchorMax = rt.pivot = new Vector2(0f, 1f);
rt.anchoredPosition = new Vector2(x, -y);   // box[0], -box[1]
rt.sizeDelta        = new Vector2(w, h);    // box[2], box[3]
```

It breaks on a rotated node — pivot (0,1) and Figma's rotation origin no longer
agree. Handle those one at a time, or carry `rotation` and a `localScale` with
them.

| Figma | Unity |
|---|---|
| `fill: "#RRGGBB"` (lone opaque SOLID, collapsed) | `ColorUtility.TryParseHtmlString` |
| `fills[0].color {r,g,b}` 0–1 + `fills[0].opacity` | `new Color(r, g, b, opacity)` — same 0–1 sRGB convention, no conversion, in either color space |
| node `opacity` < 1 on a parent | `CanvasGroup.alpha`, not a tint on each child `Image` |
| `clipsContent: true` **on a frame that scrolls** | `RectMask2D` — without it a list spills past its viewport. Figma sets `clipsContent` on plain panels too; masking every one of them (a shipped importer bug) adds a mask and a rebuild cost for nothing |
| `characters`, `fontSize` | TMP text, `fontSize` 1:1 (only while the reference resolution matches the frame) |
| `letterSpacing {value, unit}` | TMP `characterSpacing` is 1/100 em — measured, not inferred (`fontSize 40` + spacing `10` = +4px per gap): `PERCENT` → value as-is, `PIXELS` → `value / fontSize * 100` |
| `constraints` (absent = `MIN/MIN`) | re-anchor after placing — table below |
| `layoutMode` + `itemSpacing` + `padding*` | Horizontal/VerticalLayoutGroup, `spacing`, `padding` |
| INSTANCE `mainComponentId` | one prefab per component id; every instance is an instance of it |
| INSTANCE `variantProperties` | prefab variant or a `Selectable` sprite state — never a fresh prefab per instance |

A `Color` is sRGB in both color spaces, so the numbers above need no conversion.
What Linear space *does* break is elsewhere: a texture imported without its sRGB
flag, and several stacked alpha layers, which composite differently than Figma
flattened them.

**Constraints → anchors** (parent `W × H`, child `x,y,w,h`):

| Figma | anchorMin.x / anchorMax.x (H axis; vertical is the same flipped) |
|---|---|
| `MIN` | 0 / 0 — left edge (vertical `MIN` = **top**, so 1 / 1) |
| `MAX` | 1 / 1 (vertical `MAX` = bottom, 0 / 0) |
| `CENTER` | 0.5 / 0.5 |
| `STRETCH` | 0 / 1, offsets = the two margins |

**Figma grouping is visual, not behavioral.** Two traps from real imports:

- A corner button grouped with a centered title imports centered with it; on a
  narrower phone it drifts off the corner. Anchor it to the screen edge on its
  own, whatever group it came from.
- A static child (progress bar, label) nested under a group that animates at
  runtime moves with the animation. Before parenting a child where Figma put
  it, ask whether that parent moves independently; if so, make the child a
  sibling of it.

**Children of an auto-layout frame ignore `anchoredPosition`.** Inside a Layout
Group, apply only the size (via `LayoutElement.preferred*`) and drop the
position mapping — a Figma frame that hugs its content needs a
`ContentSizeFitter` on the Unity side, which uGUI does not infer.

## Text fails silently — check it first

- **A `TMP_FontAsset` for `fontName` must already exist in the project.** Figma
  hands you `{family, style}`; TMP needs an SDF asset someone generated. When it
  is missing, TMP falls back to the default face, the text still renders, and
  **the build reports success** — nothing anywhere says the design's font never
  loaded. Resolve every distinct `fontName` to an asset *before* building and
  stop if one is unresolved; a missing font is a question for the user, not a
  thing to substitute.
- **TMP does not wrap where Figma wrapped.** Identical width and `fontSize`
  still break the line somewhere else. It is the most common visible difference
  in a screenshot diff — check multi-line text explicitly.

## What uGUI cannot draw — export the pixels

A `GRADIENT_*` fill, an `IMAGE` fill (`hasImage: true`), `cornerRadius > 0`, a
`drop-shadow` effect, and every `VECTOR`/`BOOLEAN_OPERATION` node have no
runtime equivalent. Export them instead of rebuilding them:

```sh
figma-cli save_screenshots --params '{"items":[
  {"nodeId":"<bg-id>","outputPath":"/tmp/fig/bg.png","scale":3},
  {"nodeId":"<icon-id>","outputPath":"/tmp/fig/icon.png","scale":3}]}'
```

- Paths resolve from the CLI's cwd — **pass absolute paths**.
- Pick `scale` so the PNG is at least the pixel size it occupies on the
  highest-res target device (3 for a 360-wide phone frame). `--max` trims
  `get_screenshot` only; it does nothing for `save_screenshots`.
- **A node with a shadow exports bigger than its layout rect.** That is what
  `renderRect` in `list_nodes` is for — when it differs from `rect`, size the
  RectTransform from `renderRect` or the art lands offset by the blur.
- A rounded rect or a stretched panel wants a 9-sliced sprite: export it once
  and set the border in the importer, don't export one PNG per size. **The
  border is in the PNG's pixels, the `box` is in design units** — at `scale: 3`
  a 12-unit corner radius is a 36px border. Forget the multiply and the corners
  come out visibly wrong.
- **Measure the border from the pixels, not from `cornerRadius`.** A stroke or
  shadow moves the real edge; scan each side inward until the pixels change. A
  shape with no straight run (diamond, blob) has no valid border — use `Simple`,
  don't invent one.
- **The on-screen corner size also needs `pixelsPerUnitMultiplier`.** A sliced
  `Image` draws its border at sprite-pixel size, so a 3× export draws its
  corners 3× too big on a 1:1 canvas. Set `Image.pixelsPerUnitMultiplier =
  exportScale / canvasScale` (canvas units per design unit).
- **A pill has a zero border on its short axis.** Size that axis from the
  sprite's native pixel size × `canvasScale / exportScale`, not from the
  bled render bounds, or the rounded ends squash.
- **Soft edges get a black halo.** Fully transparent texels just outside a
  shadow/feather usually carry black RGB, and bilinear filtering samples it at
  any non-1:1 scale. Dilate the opaque RGB outward into the transparent ring
  (alpha untouched) before import — changing the filter mode does not fix it.
- **Read sprite size from the PNG header, not the imported `Texture2D`.**
  A `maxTextureSize` below the source silently shrinks the texture, and sizing
  from it makes the element too small (a shipped importer bug).

Then it is an ordinary import (see utk-asset-import — path, never base64):

```sh
for f in /tmp/fig/*.png; do utk import "$f" "UI/Result/$(basename "$f")"; done
utk set_import_settings --asset Assets/UI/Result/bg.png --settings '{"textureType":"Sprite"}'
utk get_import_settings --asset Assets/UI/Result/bg.png   # verify, keys are not validated for you
```

## Build the hierarchy in one pass

A screen is dozens of nodes. That is **one** `utk run_script --file build.cs`
that reads `/tmp/fig/*.json` and walks the tree — not one `utk exec` per node,
which pays a round-trip each and cannot hold a `Dictionary<string,GameObject>`
of figma-id → GameObject between calls. Keep the file outside `Assets/`
(`AgentScripts/`) and dry-run it first:

```sh
utk run_script --file AgentScripts/BuildResultScreen.cs --entry BuildResultScreen.Run --dry_run true
```

## Verify against the design, not against `ok: true`

Capture both and look:

```sh
figma-cli get_screenshot --node <frame-id> --max 0 -o /tmp/fig/design.png
utk screenshot --max 0
```

`Read` the two PNGs. For anything the eye cannot settle — a 4px gap, a
mis-anchored element — read the geometry back as numbers instead
(utk-exec-query → "UI layout") and diff it against `box`.

**One screenshot at the reference resolution only proves the design matches
itself.** The Figma frame is one aspect ratio; phones are many. Before calling
it done:

- Screenshot at least one aspect **narrower** and one **wider** than the frame
  (e.g. 9:20 and 3:4 against a 9:16 frame). Real failures from a single-frame
  extrapolation: a HUD too small on phones, a corner button off its corner.
- Check the **extreme content** case too — fewest items, longest string, an
  overlay/tutorial state — not a typical one. Two "average" checks passed while
  the smallest-content screen overlapped.
- Scan every `Image` as numbers, not pixels — the audit below. A screenshot
  review missed every one of these; the scan found them in one call.
- Colors uniformly washed out vs the Figma hex → post-processing / tonemapping
  is hitting the UI camera (URP template Global Volume), not a wrong import.

### Distortion audit

Keep it as a file and run it after every import or UI build script. It checks
prefabs under `ROOT` plus the scenes **already open** — it never opens a scene,
so the user's work stays loaded. Silence means clean.

```csharp
// AgentScripts/ui_audit.cs — run with: utk exec --file AgentScripts/ui_audit.cs --timeout 300000
const string ROOT = "Assets";   // narrow to the UI folder you touched
var sb = new System.Text.StringBuilder();
string PathOf(Transform t) { var s = t.name; while (t.parent != null) { t = t.parent; s = t.name + "/" + s; } return s; }
void Check(GameObject root, string where)
{
    foreach (var rt in root.GetComponentsInChildren<RectTransform>(true))
    {
        var cs = rt.GetComponent<UnityEngine.UI.CanvasScaler>();
        if (cs != null) sb.AppendLine($"{where} {PathOf(rt)} CANVAS ref {cs.referenceResolution} match {cs.matchWidthOrHeight} mode {cs.uiScaleMode}");
        var ls = rt.localScale;
        if (Mathf.Abs(Mathf.Abs(ls.x) - Mathf.Abs(ls.y)) > 0.01f) sb.AppendLine($"{where} {PathOf(rt)} SCALE {ls}");
        var img = rt.GetComponent<UnityEngine.UI.Image>();
        if (img == null || img.sprite == null) continue;
        var r = rt.rect.size; var sp = img.sprite; var ss = sp.rect.size;
        string shrunk = "", issue = "";
        if (UnityEditor.AssetImporter.GetAtPath(UnityEditor.AssetDatabase.GetAssetPath(sp)) is UnityEditor.TextureImporter ti
            && ti.spriteImportMode == UnityEditor.SpriteImportMode.Single)
        { ti.GetSourceTextureWidthAndHeight(out int w, out int h); if (w > ss.x + 1 || h > ss.y + 1) shrunk = $" SHRUNK src {w}x{h}"; }
        if (img.type == UnityEngine.UI.Image.Type.Simple && !img.preserveAspect && r.x > 0 && r.y > 0)
        { float d = (r.x / r.y) / (ss.x / ss.y); if (d > 1.04f || d < 0.96f) issue = $" STRETCH x{d:F2}"; }
        if (img.type == UnityEngine.UI.Image.Type.Sliced)
        {
            var b = sp.border / (img.pixelsPerUnitMultiplier * sp.pixelsPerUnit / 100f);   // canvas units, ref PPU 100
            if (b.x + b.z > r.x + 0.5f || b.y + b.w > r.y + 0.5f) issue = $" SLICE-SQUASH border {sp.border} ppum {img.pixelsPerUnitMultiplier}";
            if (sp.border == Vector4.zero) issue = " SLICED-NO-BORDER";
        }
        if (issue != "" || shrunk != "") sb.AppendLine($"{where} {PathOf(rt)} [{sp.name} {ss.x}x{ss.y} -> {r.x:F0}x{r.y:F0} {img.type}]{issue}{shrunk}");
    }
}
foreach (var g in UnityEditor.AssetDatabase.FindAssets("t:Prefab", new[] { ROOT }))
{
    var p = UnityEditor.AssetDatabase.GUIDToAssetPath(g);
    Check(UnityEditor.AssetDatabase.LoadAssetAtPath<GameObject>(p), System.IO.Path.GetFileNameWithoutExtension(p));
}
for (int i = 0; i < UnityEngine.SceneManagement.SceneManager.sceneCount; i++)
{
    var scene = UnityEngine.SceneManagement.SceneManager.GetSceneAt(i);
    if (scene.isLoaded) foreach (var go in scene.GetRootGameObjects()) Check(go, scene.name);
}
return sb.Length == 0 ? "clean" : sb.ToString();
```

| Flag | Means | Fix |
|---|---|---|
| `STRETCH xN` | `Simple` image drawn N× off its sprite's aspect | right sprite for that shape, `preserveAspect`, or 9-slice (`unity-ugui-layout` → "Sprites keep their aspect") |
| `SLICE-SQUASH` | the two borders are wider than the rect, so the corners overlap | raise `pixelsPerUnitMultiplier` or re-measure the border |
| `SLICED-NO-BORDER` | `Sliced` with a zero border stretches like `Simple` | set the border, or switch to `Simple` |
| `SHRUNK src WxH` | `maxTextureSize` below the PNG; anything sized from the texture is too small | raise `maxTextureSize` |
| `SCALE` | non-uniform `localScale` squashes everything under it | size the rect, keep scale uniform |
| `CANVAS` | not a fault — the scaler settings, to check against the choice you pinned | — |

`STRETCH` on an image that is *meant* to stretch (a full-screen background, a
bar fill) is expected; judge each hit, don't mechanically "fix" all of them.
Scenes to check that aren't open: ask, or open them `--additive` yourself and
close them after.

**Don't touch runtime state while the user is in Play mode.** Check
`EditorApplication.isPlaying` before writing PlayerPrefs, saves or scene
objects to "set up" a screenshot — it overwrote a user's live progress twice.

## Rules

- ✅ Set the CanvasScaler reference resolution from the frame **before** placing
  anything; every px number in the design is meaningless without it.
  `matchWidthOrHeight` defaults to `1` (`unity-ugui-layout` → "Match: default
  1"); never pick `0.5` yourself. Pin the value with an EditMode test.
- ✅ Re-anchor from `constraints` once the absolute layout is correct. Building
  responsive-first from a static design is how you get a screen that is wrong
  everywhere instead of wrong on one device.
- ❌ **NEVER** invent a font face, a text alignment, or a substitute for a
  missing TMP font asset. Those are the three things that render plausibly and
  wrong, with nothing failing. Ask.
- ❌ **NEVER** rebuild a gradient, a shadow or a rounded corner out of uGUI
  primitives when a 5KB sprite is one `save_screenshots` away.
- ❌ **NEVER** write to the Figma file while mapping a design into Unity. This
  direction is read-only; the user has that file open.

## Related Skills

- `figma-cli` (global) — the CLI itself: preflight, call forms, error codes
- `unity-ugui-layout` — canvas setup, anchors, TMP rules, the element spec table this skill fills in
- `utk-asset-import` — importing the exported PNGs and their importer settings
- `unity-texture-pipeline` — compression and atlas math once the sprites are in
- `utk-exec-query` — reading built UI geometry back as numbers
