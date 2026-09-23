// Popup layout check: button size, text fit, placement inside the panel, overlaps, screen edges.
// Run with the popup(s) open (Play mode, or Edit mode on an open prefab/scene):  utk exec --file popup_layout_check.cs
// Only UnityEngine / UnityEditor are in scope (no LINQ). Game-agnostic. All sizes are in root-canvas units.
//
// Params — edit before running:
string[] targets = new string[0];  // popup root names; empty = auto-detect (Popup/Dialog/Modal in a name or component type)
string panelName = "";             // the popup's frame Image; empty = the largest non-backdrop, non-button Image in the popup
float panelPadding = 24f;          // content keeps this far inside the frame's edge (units)
float minTouchDp = 48f;            // Material touch target 48x48 dp (Apple HIG: 44x44 pt)
float narrowWidthDp = 360f;        // narrowest common phone width the canvas maps onto
float minGapDp = 8f;               // Material: separate touch targets by 8 dp or more
float edgeMargin = 24f;            // min distance from the canvas edge for any element (units)
float rowTolerance = 8f;           // buttons whose centres differ by less than this share a row

var sb = new System.Text.StringBuilder();
int findings = 0;

bool IsTarget(Transform t)
{
    if (targets.Length > 0) { foreach (var n in targets) if (t.name == n) return true; return false; }
    var n2 = t.name.ToLowerInvariant();
    if (n2.Contains("popup") || n2.Contains("dialog") || n2.Contains("modal")) return true;
    foreach (var mb in t.GetComponents<MonoBehaviour>())
    { if (mb == null) continue; var tn = mb.GetType().Name.ToLowerInvariant(); if (tn.Contains("popup") || tn.Contains("dialog") || tn.Contains("modal")) return true; }
    return false;
}
bool Visible(Transform t)
{
    if (!t.gameObject.activeInHierarchy) return false;
    var c = t.GetComponentInParent<Canvas>(); if (c != null && !c.enabled) return false;
    var cg = t.GetComponentInParent<CanvasGroup>(); if (cg != null && cg.alpha <= 0.001f) return false;
    return true;
}
Rect InSpace(RectTransform r, Transform space)
{
    var k = new Vector3[4]; r.GetWorldCorners(k);
    Vector2 a = space.InverseTransformPoint(k[0]), b = space.InverseTransformPoint(k[2]);
    return Rect.MinMaxRect(Mathf.Min(a.x, b.x), Mathf.Min(a.y, b.y), Mathf.Max(a.x, b.x), Mathf.Max(a.y, b.y));
}
bool Overlaps(Rect a, Rect b) => a.xMin < b.xMax - 1f && b.xMin < a.xMax - 1f && a.yMin < b.yMax - 1f && b.yMin < a.yMax - 1f;
bool Inside(Rect inner, Rect r) => r.xMin >= inner.xMin - 1f && r.xMax <= inner.xMax + 1f && r.yMin >= inner.yMin - 1f && r.yMax <= inner.yMax + 1f;
string R(Rect r) => $"{r.width:0}x{r.height:0} @({r.center.x:0},{r.center.y:0})";
void Find(string msg) { sb.AppendLine("  " + msg); findings++; }

var roots = new System.Collections.Generic.List<Transform>();
foreach (var rt in UnityEngine.Object.FindObjectsByType<RectTransform>(FindObjectsSortMode.None))
{
    if (!IsTarget(rt) || !Visible(rt)) continue;
    bool nested = false; for (var p = rt.parent; p != null; p = p.parent) if (IsTarget(p)) { nested = true; break; }
    if (!nested) roots.Add(rt);
}
if (roots.Count == 0) return "popup_layout_check: no open popup found (open one, or set targets)";

foreach (var root in roots)
{
    var canvas = root.GetComponentInParent<Canvas>().rootCanvas;
    var space = canvas.transform;
    var screen = ((RectTransform)space).rect;
    float dp = screen.width / narrowWidthDp; // canvas units per dp on the narrowest phone
    float minTouch = minTouchDp * dp;
    sb.AppendLine($"{root.name}  (canvas {screen.width:0}x{screen.height:0}, min touch {minTouch:0} units)");

    // Panel (the frame art) and its inner area. Not the 9-slice border: that is a stretch-safe zone, content may sit on it.
    UnityEngine.UI.Image panel = null; float best = 0f;
    foreach (var img in root.GetComponentsInChildren<UnityEngine.UI.Image>())
    {
        if (!Visible(img.transform) || img.sprite == null) continue;
        if (panelName.Length > 0) { if (img.name == panelName || img.transform.parent.name == panelName) { panel = img; break; } continue; }
        if (img.GetComponentInParent<UnityEngine.UI.Selectable>() != null) continue;
        var a = InSpace(img.rectTransform, space);
        if (a.width >= screen.width * 0.9f && a.height >= screen.height * 0.9f) continue; // dim / backdrop, not the frame
        if (a.width * a.height > best) { best = a.width * a.height; panel = img; }
    }
    Rect inner = screen;
    if (panel == null) Find("NO PANEL: no frame Image found — set panelName; inside-panel checks use the whole canvas");
    else
    {
        var pr = InSpace(panel.rectTransform, space);
        inner = Rect.MinMaxRect(pr.xMin + panelPadding, pr.yMin + panelPadding, pr.xMax - panelPadding, pr.yMax - panelPadding);
        sb.AppendLine($"  panel {panel.name} {R(pr)}, inner {R(inner)}");
    }

    // Collect visible texts and buttons.
    var els = new System.Collections.Generic.List<object[]>(); // {name, Rect, kind, Transform}
    var buttons = new System.Collections.Generic.List<object[]>();
    foreach (var sel in root.GetComponentsInChildren<UnityEngine.UI.Selectable>())
    {
        if (!Visible(sel.transform) || !sel.interactable) continue;
        var hit = sel.targetGraphic != null ? sel.targetGraphic.rectTransform : (RectTransform)sel.transform;
        var r = InSpace(hit, space);
        var e = new object[] { sel.name, r, "button", sel.transform };
        els.Add(e); buttons.Add(e);
        if (r.width < minTouch - 0.5f || r.height < minTouch - 0.5f) Find($"SMALL TARGET: {sel.name} hit area {r.width:0}x{r.height:0} < {minTouch:0}x{minTouch:0}");
    }
    foreach (var t in root.GetComponentsInChildren<TMPro.TMP_Text>())
    {
        if (!Visible(t.transform) || string.IsNullOrEmpty(t.text)) continue;
        t.ForceMeshUpdate();
        // Rendered glyph bounds, not the rect: a wide rect with short copy is fine, glyphs past the rect are not.
        var tb = t.textBounds;
        Vector2 g0 = space.InverseTransformPoint(t.transform.TransformPoint(tb.min)), g1 = space.InverseTransformPoint(t.transform.TransformPoint(tb.max));
        var r = Rect.MinMaxRect(Mathf.Min(g0.x, g1.x), Mathf.Min(g0.y, g1.y), Mathf.Max(g0.x, g1.x), Mathf.Max(g0.y, g1.y));
        bool insideButton = t.GetComponentInParent<UnityEngine.UI.Selectable>() != null;
        if (!insideButton) els.Add(new object[] { t.name, r, "text", t.transform });
        var box = t.rectTransform.rect;
        if (t.isTextOverflowing || tb.size.x > box.width + 1f || tb.size.y > box.height + 1f)
            Find($"TEXT OVERFLOW: {t.name} \"{t.text.Replace("\n", "\\n")}\" renders {tb.size.x:0}x{tb.size.y:0} at size {t.fontSize:0.#} in a {box.width:0}x{box.height:0} rect"
                 + (!t.enableAutoSizing ? " (no auto-size)" : t.fontSize <= t.fontSizeMin + 0.01f ? $" (auto-size floor {t.fontSizeMin:0.#} reached)" : ""));
        else if (t.enableAutoSizing && t.fontSize <= t.fontSizeMin + 0.01f)
            Find($"TEXT AT FLOOR: {t.name} shrank to fontSizeMin {t.fontSizeMin:0.#} — widen the rect or shorten the copy");
    }

    // Placement: inside the panel's inner area (the close button and title ribbon are allowed on the frame), on screen, no overlaps.
    foreach (var e in els)
    {
        var n = ((string)e[0]).ToLowerInvariant(); var r = (Rect)e[1];
        bool frameItem = n.Contains("close") || n.Contains("title") || n.Contains("ribbon") || n == "x";
        if (!frameItem && panel != null && !Inside(inner, r)) Find($"OUT OF PANEL: {e[0]} {R(r)} leaves the inner area {R(inner)}");
        if (r.xMin < screen.xMin + edgeMargin || r.xMax > screen.xMax - edgeMargin || r.yMin < screen.yMin + edgeMargin || r.yMax > screen.yMax - edgeMargin)
            Find($"NEAR EDGE: {e[0]} {R(r)} is within {edgeMargin:0} units of the canvas edge");
    }
    for (int i = 0; i < els.Count; i++)
        for (int j = i + 1; j < els.Count; j++)
        {
            var ti = (Transform)els[i][3]; var tj = (Transform)els[j][3];
            if (ti.IsChildOf(tj) || tj.IsChildOf(ti)) continue;
            if (Overlaps((Rect)els[i][1], (Rect)els[j][1])) Find($"OVERLAP: {els[i][0]} {R((Rect)els[i][1])} and {els[j][0]} {R((Rect)els[j][1])}");
        }

    // Buttons sharing a row: same height, a finger-width gap.
    for (int i = 0; i < buttons.Count; i++)
        for (int j = i + 1; j < buttons.Count; j++)
        {
            Rect a = (Rect)buttons[i][1], b = (Rect)buttons[j][1];
            if (Mathf.Abs(a.center.y - b.center.y) > rowTolerance) continue;
            if (Mathf.Abs(a.height - b.height) > Mathf.Max(a.height, b.height) * 0.05f) Find($"ROW HEIGHT: {buttons[i][0]} ({a.height:0}) and {buttons[j][0]} ({b.height:0}) share a row with different heights");
            float gap = a.center.x < b.center.x ? b.xMin - a.xMax : a.xMin - b.xMax;
            if (gap < minGapDp * dp) Find($"TIGHT ROW: {buttons[i][0]} / {buttons[j][0]} gap {gap:0} < {minGapDp * dp:0}");
        }
    foreach (var e in els) sb.AppendLine($"    {e[2],-6} {e[0],-24} {R((Rect)e[1])}");
}
sb.Insert(0, $"popup_layout_check: {roots.Count} popup(s), {findings} finding(s)\n");
return sb.ToString();
