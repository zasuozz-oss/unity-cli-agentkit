// UI audit: the numeric "done" gate for any uGUI screen, popup or prefab an agent built or changed.
// Run: utk exec --file ui_audit.cs --timeout 120000   (copy it outside Assets/ first; edit the params below)
// Reads what is live: an open scene (Edit or Play mode) or the prefab open in Prefab Mode. Never opens,
// saves or modifies anything. Run it once per aspect in the sweep (set the Game view size between runs).
// Every line starting with a RULE in capitals is a finding; "clean" means none. Sizes are canvas units.
// Only UnityEngine / UnityEditor / TMPro are in scope (no LINQ). Game-agnostic.
//
// Params — edit before running:
string target = "";          // name or path of the root to audit (e.g. "ShopPopup" or "Canvas/ShopPopup"); empty = every visible canvas
float centerTolPx = 2f;      // glyphs this far off their box's centre are a finding (units)
float aspectTol = 0.04f;     // Simple/Filled image whose rect aspect differs from the sprite's by more than this is stretched
bool listElements = false;   // true = also print every checked text/image with its rect (for building a report)

var sb = new System.Text.StringBuilder();
int findings = 0;
void Find(string rule, Transform t, string msg) { sb.AppendLine($"{rule} {PathOf(t)}: {msg}"); findings++; }
string PathOf(Transform t) { var s = t.name; while (t.parent != null && t.parent.GetComponent<Canvas>() == null) { t = t.parent; s = t.name + "/" + s; } return s; }
bool Visible(Transform t)
{
    if (!t.gameObject.activeInHierarchy) return false;
    var c = t.GetComponentInParent<Canvas>(); if (c == null || !c.enabled) return false;
    foreach (var cg in t.GetComponentsInParent<CanvasGroup>()) { if (cg.alpha <= 0.001f) return false; if (cg.ignoreParentGroups) break; }
    return true;
}
Rect InSpace(RectTransform r, Transform space)
{
    var k = new Vector3[4]; r.GetWorldCorners(k);
    Vector2 a = space.InverseTransformPoint(k[0]), b = space.InverseTransformPoint(k[2]);
    return Rect.MinMaxRect(Mathf.Min(a.x, b.x), Mathf.Min(a.y, b.y), Mathf.Max(a.x, b.x), Mathf.Max(a.y, b.y));
}
Rect GlyphsInSpace(TMPro.TMP_Text t, Transform space)
{
    var tb = t.textBounds;
    Vector2 a = space.InverseTransformPoint(t.transform.TransformPoint(tb.min)), b = space.InverseTransformPoint(t.transform.TransformPoint(tb.max));
    return Rect.MinMaxRect(Mathf.Min(a.x, b.x), Mathf.Min(a.y, b.y), Mathf.Max(a.x, b.x), Mathf.Max(a.y, b.y));
}
string R(Rect r) => $"{r.width:0}x{r.height:0} @({r.center.x:0},{r.center.y:0})";
bool Overlaps(Rect a, Rect b) => a.xMin < b.xMax - 1f && b.xMin < a.xMax - 1f && a.yMin < b.yMax - 1f && b.yMin < a.yMax - 1f;
// The box a centred label is meant to be centred in: the button/background it sits on when its own rect is
// only a text box inside it, else its own rect. "Text not centred on the button" is usually this, not alignment.
RectTransform CentreBox(TMPro.TMP_Text t)
{
    var p = t.transform.parent as RectTransform;
    if (p != null && p.GetComponent<UnityEngine.UI.Graphic>() != null && p.GetComponent<UnityEngine.UI.LayoutGroup>() == null)
    {
        int siblings = 0; foreach (Transform c in p) if (c.gameObject.activeInHierarchy && c.GetComponent<UnityEngine.UI.Graphic>() != null) siblings++;
        if (siblings == 1) return p; // the label is the only graphic on its background: centre it on the background
    }
    return t.rectTransform;
}

// Roots: the prefab in Prefab Mode, else the open scenes' canvases.
var roots = new System.Collections.Generic.List<Transform>();
var stage = UnityEditor.SceneManagement.PrefabStageUtility.GetCurrentPrefabStage();
var candidates = new System.Collections.Generic.List<GameObject>();
if (stage != null) candidates.Add(stage.prefabContentsRoot);
else for (int i = 0; i < UnityEngine.SceneManagement.SceneManager.sceneCount; i++)
{
    var scene = UnityEngine.SceneManagement.SceneManager.GetSceneAt(i);
    if (scene.isLoaded) foreach (var go in scene.GetRootGameObjects()) candidates.Add(go);
}
if (Application.isPlaying) // DontDestroyOnLoad UI lives in a scene the loop above cannot list
    foreach (var c in UnityEngine.Object.FindObjectsByType<Canvas>(FindObjectsSortMode.None))
        if (c.isRootCanvas && !candidates.Contains(c.transform.root.gameObject)) candidates.Add(c.transform.root.gameObject);
foreach (var go in candidates)
    foreach (var rt in go.GetComponentsInChildren<RectTransform>(true))
    {
        if (target.Length > 0)
        {
            if (rt.name == target || PathOf(rt) == target || PathOf(rt).EndsWith("/" + target)) roots.Add(rt);
        }
        else { var c = rt.GetComponent<Canvas>(); if (c != null && c.isRootCanvas) roots.Add(rt); }
    }
if (roots.Count == 0) return target.Length > 0 ? $"ui_audit: '{target}' not found in the open scenes/prefab stage (open it, or check the name)" : "ui_audit: no canvas in the open scenes";
Canvas.ForceUpdateCanvases();

foreach (var root in roots)
{
    if (!Visible(root)) { sb.AppendLine($"(skipped {PathOf(root)}: inactive or hidden)"); continue; }
    var canvas = root.GetComponentInParent<Canvas>().rootCanvas;
    var space = canvas.transform;
    var screen = ((RectTransform)space).rect;
    var scaler = canvas.GetComponent<UnityEngine.UI.CanvasScaler>();
    sb.AppendLine($"== {PathOf(root)}  canvas {screen.width:0}x{screen.height:0}" + (scaler != null ? $" ref {scaler.referenceResolution.x:0}x{scaler.referenceResolution.y:0} match {scaler.matchWidthOrHeight:0.##}" : " (no CanvasScaler)"));
    if (screen.width < 1 || screen.height < 1) { sb.AppendLine("  canvas has no size yet — open the Game view (Edit mode) or enter Play, then rerun"); continue; }

    var boxes = new System.Collections.Generic.List<object[]>(); // {Transform, Rect, kind} for the overlap pass
    foreach (var rt in root.GetComponentsInChildren<RectTransform>())
    {
        if (!Visible(rt)) continue;
        var local = rt.rect;

        // SCALE: a non-uniform scale anywhere up the chain squashes everything below it; a non-1 scale on a
        // non-animated element is sizing done with the wrong knob (it also scales text and 9-slice caps).
        var ls = rt.localScale;
        if (Mathf.Abs(Mathf.Abs(ls.x) - Mathf.Abs(ls.y)) > 0.01f) Find("SCALE-NONUNIFORM", rt, $"localScale {ls.x:0.###}x{ls.y:0.###} squashes this and every child — size with sizeDelta/anchors, keep scale uniform");
        else if (rt != space && Mathf.Abs(Mathf.Abs(ls.x) - 1f) > 0.01f && rt.GetComponent<Canvas>() == null)
            Find("SCALE-SIZING", rt, $"localScale {ls.x:0.###} — if this is not mid-animation, set the size instead (scale also shrinks text and 9-slice borders)");

        // ZERO-SIZE: a visible graphic with no area is a broken anchor/size, not an invisible element.
        var g = rt.GetComponent<UnityEngine.UI.Graphic>();
        if (g != null && g.enabled && (local.width < 0.5f || local.height < 0.5f) && !(g is TMPro.TMP_Text tz && string.IsNullOrEmpty(tz.text)))
            Find("ZERO-SIZE", rt, $"rect {local.width:0.#}x{local.height:0.#} — anchors or a fitter collapsed it");

        // LAYOUT-CONFLICT: two components each think they own this rect's size.
        var fitter = rt.GetComponent<UnityEngine.UI.ContentSizeFitter>();
        if (fitter != null && fitter.enabled && rt.parent != null)
        {
            var pg = rt.parent.GetComponent<UnityEngine.UI.HorizontalOrVerticalLayoutGroup>();
            if (pg != null && pg.enabled && ((pg.childControlWidth && fitter.horizontalFit != UnityEngine.UI.ContentSizeFitter.FitMode.Unconstrained) || (pg.childControlHeight && fitter.verticalFit != UnityEngine.UI.ContentSizeFitter.FitMode.Unconstrained)))
                Find("LAYOUT-CONFLICT", rt, $"ContentSizeFitter under a {pg.GetType().Name} that controls child size — remove the fitter, give the child a LayoutElement");
            var arf = rt.GetComponent<UnityEngine.UI.AspectRatioFitter>();
            if (arf != null && arf.enabled && arf.aspectMode != UnityEngine.UI.AspectRatioFitter.AspectMode.None)
                Find("LAYOUT-CONFLICT", rt, "ContentSizeFitter and AspectRatioFitter both size this rect — keep one");
        }

        var abs = InSpace(rt, space);

        // FIXED-WIDTH: a container pinned to one x anchor with a width close to the canvas's. It was tuned at one
        // aspect; on a narrower phone the canvas shrinks and it does not, so it overflows, clips or centres wrong.
        if (rt != space && rt.GetComponent<Canvas>() == null && rt.anchorMin.x == rt.anchorMax.x && abs.width >= screen.width * 0.9f
            && rt.parent != null && rt.parent.GetComponent<UnityEngine.UI.LayoutGroup>() == null)
        {
            bool art = rt.childCount == 0 && rt.GetComponent<UnityEngine.UI.Image>() is UnityEngine.UI.Image ai && ai.sprite != null;
            Find("FIXED-WIDTH", rt, $"width {local.width:0} fixed at anchor x {rt.anchorMin.x:0.##} on a {screen.width:0}-wide canvas — " + (art
                ? "art: stretching would distort it; fit it instead (AspectRatioFitter FitInParent in a stretched box, or accept the overhang and say so)"
                : "stretch it (anchors x 0..1, offsets = margins) so it follows narrower screens"));
        }

        // IMAGE: drawn off its sprite's shape (full rules and causes: unity-ui-sprite-distortion).
        var img = g as UnityEngine.UI.Image;
        if (img != null && img.enabled && img.sprite != null && local.width >= 0.5f && local.height >= 0.5f)
        {
            var ss = img.sprite.rect.size;
            float d = (local.width / local.height) / (ss.x / ss.y);
            bool off = d > 1f + aspectTol || d < 1f - aspectTol;
            if ((img.type == UnityEngine.UI.Image.Type.Simple || img.type == UnityEngine.UI.Image.Type.Filled) && !img.preserveAspect && off)
                Find("IMAGE-STRETCH", rt, $"sprite {img.sprite.name} {ss.x:0}x{ss.y:0} drawn at {local.width:0}x{local.height:0} (x{d:0.00}) — resize the rect to the sprite's aspect, or preserveAspect, or 9-slice it");
            if (img.type == UnityEngine.UI.Image.Type.Sliced)
            {
                var b = img.sprite.border;
                if (b == Vector4.zero && off) Find("SLICED-NO-BORDER", rt, $"Sliced but sprite {img.sprite.name} has no border — it stretches like Simple (x{d:0.00})");
                else
                {
                    float k = 100f / (img.pixelsPerUnitMultiplier * img.sprite.pixelsPerUnit); // canvas units per sprite px at ref PPU 100
                    if ((b.x + b.z) * k > local.width + 0.5f || (b.y + b.w) * k > local.height + 0.5f)
                        Find("SLICE-SQUASH", rt, $"rect {local.width:0}x{local.height:0} is smaller than its caps (border {b}) — the corners squash; raise pixelsPerUnitMultiplier or enlarge the rect");
                }
            }
            if (listElements) sb.AppendLine($"  image {PathOf(rt)} {R(abs)} {img.type}");
        }

        // TEXT: overflow, auto-size floor, and centring measured on the rendered glyphs.
        var t = g as TMPro.TMP_Text;
        if (t != null && t.enabled && !string.IsNullOrEmpty(t.text))
        {
            t.ForceMeshUpdate();
            var glyphs = GlyphsInSpace(t, space);
            string quote = t.text.Length > 24 ? t.text.Substring(0, 24).Replace("\n", " ") + "…" : t.text.Replace("\n", " ");
            if (t.isTextOverflowing || t.textBounds.size.x > local.width + 1f || t.textBounds.size.y > local.height + 1f)
                Find("TEXT-OVERFLOW", rt, $"\"{quote}\" renders {t.textBounds.size.x:0}x{t.textBounds.size.y:0} at size {t.fontSize:0.#} in {local.width:0}x{local.height:0}" + (t.enableAutoSizing ? $" (auto-size min {t.fontSizeMin:0.#})" : " (no auto-size)"));
            else if (t.enableAutoSizing && t.fontSize <= t.fontSizeMin + 0.01f)
                Find("TEXT-AT-FLOOR", rt, $"\"{quote}\" shrank to fontSizeMin {t.fontSizeMin:0.#} — widen the rect or shorten the copy");

            var ha = t.horizontalAlignment; var va = t.verticalAlignment;
            bool hc = ha == TMPro.HorizontalAlignmentOptions.Center || ha == TMPro.HorizontalAlignmentOptions.Geometry;
            bool vc = va == TMPro.VerticalAlignmentOptions.Middle || va == TMPro.VerticalAlignmentOptions.Geometry || va == TMPro.VerticalAlignmentOptions.Capline;
            var boxRt = CentreBox(t);
            var box = InSpace(boxRt, space);
            string on = boxRt == t.rectTransform ? "its rect" : $"parent {boxRt.name}";
            if (hc)
            {
                float dx = glyphs.center.x - box.center.x;
                if (Mathf.Abs(dx) > centerTolPx)
                    Find("TEXT-OFF-CENTER-X", rt, $"glyphs {dx:+0.#;-0.#} units off the centre of {on}" + (boxRt != t.rectTransform ? $" (text rect centre {(abs.center.x - box.center.x):+0.#;-0.#})" : "") + (Mathf.Abs(t.margin.x - t.margin.z) > 0.5f ? $" — margins L{t.margin.x:0}/R{t.margin.z:0} differ" : ""));
            }
            if (vc)
            {
                float dy = glyphs.center.y - box.center.y;
                // Middle centres the line box (ascender..descender), not the glyphs: caps/digits sit high or low by font.
                float tolY = Mathf.Max(centerTolPx, va == TMPro.VerticalAlignmentOptions.Middle ? t.fontSize * 0.08f : 0f);
                if (Mathf.Abs(dy) > tolY)
                    Find("TEXT-OFF-CENTER-Y", rt, $"glyphs {dy:+0.#;-0.#} units off the centre of {on} (vertical {va}" + (va == TMPro.VerticalAlignmentOptions.Middle ? "; try Geometry for single-line labels" : "") + ")" + (Mathf.Abs(t.margin.y - t.margin.w) > 0.5f ? $" — margins T{t.margin.y:0}/B{t.margin.w:0} differ" : ""));
            }
            if (listElements) sb.AppendLine($"  text  {PathOf(rt)} \"{quote}\" glyphs {R(glyphs)} box {R(box)} {ha}/{va}");
            if (t.GetComponentInParent<UnityEngine.UI.Selectable>() == null) boxes.Add(new object[] { rt, glyphs, "text" });
        }

        // CLIPPED / OFF-SCREEN: visible content cut by a mask or outside the canvas (scroll content is exempt).
        if (g != null && g.enabled && local.width >= 0.5f && local.height >= 0.5f && rt.GetComponentInParent<UnityEngine.UI.ScrollRect>() == null)
        {
            if (abs.xMax < screen.xMin || abs.xMin > screen.xMax || abs.yMax < screen.yMin || abs.yMin > screen.yMax) Find("OFF-SCREEN", rt, $"{R(abs)} is entirely outside the canvas {screen.width:0}x{screen.height:0}");
            else if (abs.xMin < screen.xMin - 1f || abs.xMax > screen.xMax + 1f || abs.yMin < screen.yMin - 1f || abs.yMax > screen.yMax + 1f)
            {
                bool backdrop = abs.width >= screen.width * 0.95f || abs.height >= screen.height * 0.95f; // full-bleed art is meant to overhang
                if (!backdrop) Find("PAST-SCREEN-EDGE", rt, $"{R(abs)} crosses the canvas edge");
            }
            var mask = rt.parent != null ? rt.parent.GetComponentInParent<UnityEngine.UI.RectMask2D>() : null;
            if (mask != null && Visible(mask.transform))
            {
                var m = InSpace(mask.rectTransform, space);
                if (abs.xMin < m.xMin - 1f || abs.xMax > m.xMax + 1f || abs.yMin < m.yMin - 1f || abs.yMax > m.yMax + 1f) Find("CLIPPED", rt, $"{R(abs)} is cut by RectMask2D {mask.name} {R(m)}");
            }
        }

        var sel = rt.GetComponent<UnityEngine.UI.Selectable>();
        if (sel != null && sel.interactable && sel.targetGraphic != null) boxes.Add(new object[] { rt, InSpace(sel.targetGraphic.rectTransform, space), "button" });
    }

    // OVERLAP: texts and buttons that are not parent/child of each other must not cover one another.
    for (int i = 0; i < boxes.Count; i++)
        for (int j = i + 1; j < boxes.Count; j++)
        {
            var ti = (Transform)boxes[i][0]; var tj = (Transform)boxes[j][0];
            if (ti.IsChildOf(tj) || tj.IsChildOf(ti)) continue;
            if (Overlaps((Rect)boxes[i][1], (Rect)boxes[j][1]))
                Find("OVERLAP", ti, $"{boxes[i][2]} {R((Rect)boxes[i][1])} covers {boxes[j][2]} {PathOf(tj)} {R((Rect)boxes[j][1])}");
        }
}
sb.Insert(0, $"ui_audit: {roots.Count} root(s), {findings} finding(s)" + (Application.isPlaying ? " [Play]" : stage != null ? " [Prefab Mode]" : " [Edit]") + "\n");
return findings == 0 && !listElements ? sb.ToString().Split('\n')[0] + " — clean" : sb.ToString();
