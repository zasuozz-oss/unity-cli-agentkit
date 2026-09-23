// Layer audit: what draws above an open popup, and which overlapping items tie on sort order.
// Run in Play mode with the popup(s) open:  utk exec --file layer_audit.cs
// Only UnityEngine / UnityEditor are in scope (no LINQ). Game-agnostic.
//
// Params — edit before running:
string[] targets = new string[0];   // popup root names to audit; empty = auto-detect (Popup/Dialog/Modal in a name or component type)
bool dumpLadder = true;              // print the full draw ladder, lowest first
float minOverlap = 0.02f;            // ignore overlaps under 2 % of the popup's screen area

var sb = new System.Text.StringBuilder();
var items = new System.Collections.Generic.List<object[]>(); // {key double[4], kind, path, Rect viewport, Transform root, string sortInfo}

Canvas SortingCanvas(Canvas c)
{
    while (c != null && !c.isRootCanvas && !c.overrideSorting)
        c = c.transform.parent != null ? c.transform.parent.GetComponentInParent<Canvas>() : null;
    return c;
}
string PathOf(Transform t) { var p = t.name; while (t.parent != null) { t = t.parent; p = t.name + "/" + p; } return p; }
Rect Union(Rect a, Rect b) => Rect.MinMaxRect(Mathf.Min(a.xMin, b.xMin), Mathf.Min(a.yMin, b.yMin), Mathf.Max(a.xMax, b.xMax), Mathf.Max(a.yMax, b.yMax));
float Area(Rect r) => Mathf.Max(0f, r.width) * Mathf.Max(0f, r.height);
Rect Inter(Rect a, Rect b) => Rect.MinMaxRect(Mathf.Max(a.xMin, b.xMin), Mathf.Max(a.yMin, b.yMin), Mathf.Min(a.xMax, b.xMax), Mathf.Min(a.yMax, b.yMax));
Camera CamFor(int layer)
{
    Camera best = null;
    foreach (var c in Camera.allCameras) if (c.enabled && (c.cullingMask & (1 << layer)) != 0 && (best == null || c.depth > best.depth)) best = c;
    return best;
}

// 1. Canvases: one entry per sorting canvas, rect = union of its visible graphics.
var rects = new System.Collections.Generic.Dictionary<Canvas, Rect>();
foreach (var g in UnityEngine.Object.FindObjectsByType<UnityEngine.UI.Graphic>(FindObjectsSortMode.None))
{
    if (!g.isActiveAndEnabled || g.canvas == null || !g.canvas.isActiveAndEnabled || g.color.a <= 0.001f) continue;
    if (g.canvasRenderer != null && g.canvasRenderer.GetAlpha() <= 0.001f) continue;
    var sc = SortingCanvas(g.canvas); if (sc == null || !sc.enabled) continue;
    var cg = g.GetComponentInParent<CanvasGroup>(); if (cg != null && cg.alpha <= 0.001f) continue;
    var cam = sc.renderMode == RenderMode.ScreenSpaceOverlay ? null : (sc.worldCamera != null ? sc.worldCamera : Camera.main);
    var corners = new Vector3[4]; g.rectTransform.GetWorldCorners(corners);
    Rect r = default; bool first = true;
    foreach (var w in corners)
    {
        Vector2 v;
        if (cam == null) { var pr = sc.pixelRect; v = new Vector2(w.x / pr.width, w.y / pr.height); }
        else { var s = RectTransformUtility.WorldToScreenPoint(cam, w); v = new Vector2(s.x / cam.pixelWidth, s.y / cam.pixelHeight); }
        r = first ? new Rect(v, Vector2.zero) : Union(r, new Rect(v, Vector2.zero)); first = false;
    }
    rects[sc] = rects.TryGetValue(sc, out var old) ? Union(old, r) : r;
}
foreach (var kv in rects)
{
    var c = kv.Key;
    bool overlay = c.renderMode == RenderMode.ScreenSpaceOverlay || (c.renderMode == RenderMode.ScreenSpaceCamera && c.worldCamera == null);
    var cam = overlay ? null : (c.worldCamera != null ? c.worldCamera : Camera.main);
    double camKey = overlay ? 1e9 : (cam != null ? cam.depth : 0);
    double dist = overlay || cam == null ? 0 : -(c.renderMode == RenderMode.ScreenSpaceCamera ? c.planeDistance : Vector3.Distance(cam.transform.position, c.transform.position));
    items.Add(new object[] { new double[] { camKey, SortingLayer.GetLayerValueFromID(c.sortingLayerID), c.sortingOrder, dist },
        "Canvas(" + c.renderMode + ")", PathOf(c.transform), kv.Value, c.transform,
        $"{SortingLayer.IDToName(c.sortingLayerID)}/{c.sortingOrder}" + (overlay ? " overlay" : $" cam={cam?.name}") });
}

// 2. Renderers outside UI graphics: sprites, meshes, particles, lines, trails, world text.
foreach (var rd in UnityEngine.Object.FindObjectsByType<Renderer>(FindObjectsSortMode.None))
{
    if (!rd.enabled || !rd.gameObject.activeInHierarchy) continue;
    if (rd is SpriteRenderer sr && (sr.sprite == null || sr.color.a <= 0.001f)) continue;
    if (rd is ParticleSystemRenderer) { var ps = rd.GetComponent<ParticleSystem>(); if (ps != null && ps.particleCount == 0 && !ps.isPlaying) continue; }
    var cam = CamFor(rd.gameObject.layer); if (cam == null) continue;
    var b = rd.bounds; Rect r = default; bool first = true;
    for (int i = 0; i < 8; i++)
    {
        var w = new Vector3((i & 1) == 0 ? b.min.x : b.max.x, (i & 2) == 0 ? b.min.y : b.max.y, (i & 4) == 0 ? b.min.z : b.max.z);
        var v = (Vector2)cam.WorldToViewportPoint(w);
        r = first ? new Rect(v, Vector2.zero) : Union(r, new Rect(v, Vector2.zero)); first = false;
    }
    if (Area(Inter(r, new Rect(0, 0, 1, 1))) <= 0f) continue; // off screen
    var inCanvas = rd.GetComponentInParent<Canvas>();
    items.Add(new object[] { new double[] { cam.depth, SortingLayer.GetLayerValueFromID(rd.sortingLayerID), rd.sortingOrder, -Vector3.Distance(cam.transform.position, b.center) },
        rd.GetType().Name + (inCanvas != null ? " (inside a Canvas!)" : ""), PathOf(rd.transform), r, rd.transform,
        $"{SortingLayer.IDToName(rd.sortingLayerID)}/{rd.sortingOrder} cam={cam.name}" });
}

int Cmp(object[] a, object[] b)
{
    var ka = (double[])a[0]; var kb = (double[])b[0];
    for (int i = 0; i < 4; i++) if (ka[i] != kb[i]) return ka[i].CompareTo(kb[i]);
    return 0;
}
items.Sort(Cmp);

if (dumpLadder)
{
    sb.AppendLine("LADDER (drawn first → last):");
    foreach (var it in items) { var r = (Rect)it[3]; sb.AppendLine($"  {it[5],-28} {it[1],-26} {it[2]}  vp=({r.xMin:0.00},{r.yMin:0.00})-({r.xMax:0.00},{r.yMax:0.00})"); }
}

// 3. Findings: anything drawn after an open popup that overlaps it and is not part of it.
bool IsTarget(Transform t)
{
    if (targets.Length > 0) { foreach (var n in targets) if (t.name == n) return true; return false; }
    var n2 = t.name.ToLowerInvariant();
    if (n2.Contains("popup") || n2.Contains("dialog") || n2.Contains("modal")) return true;
    foreach (var mb in t.GetComponents<MonoBehaviour>())
    { if (mb == null) continue; var tn = mb.GetType().Name.ToLowerInvariant(); if (tn.Contains("popup") || tn.Contains("dialog") || tn.Contains("modal")) return true; }
    return false;
}
int findings = 0;
for (int i = 0; i < items.Count; i++)
{
    var p = items[i]; var pt = (Transform)p[4];
    if (!((string)p[1]).StartsWith("Canvas") || !IsTarget(pt)) continue;
    var pr = (Rect)p[3]; float pa = Area(pr); if (pa <= 0f) continue;
    for (int j = i + 1; j < items.Count; j++)
    {
        var q = items[j]; var qt = (Transform)q[4];
        if (qt.IsChildOf(pt)) continue; // the popup's own layers (hand, rim, fx) are meant to be above it
        float ov = Area(Inter(pr, (Rect)q[3])) / pa;
        if (ov < minOverlap) continue;
        sb.AppendLine($"ABOVE POPUP: {q[2]} [{q[5]}] draws over {p[2]} [{p[5]}], covering {ov:P0} of it");
        findings++;
    }
}
// 4. Ties: same camera + layer + order and overlapping → Unity picks the order (distance/batching), not you.
for (int i = 0; i + 1 < items.Count; i++)
{
    var a = items[i]; var b2 = items[i + 1];
    var ka = (double[])a[0]; var kb = (double[])b2[0];
    if (ka[0] == kb[0] && ka[1] == kb[1] && ka[2] == kb[2] && Area(Inter((Rect)a[3], (Rect)b2[3])) > 0f
        && !((Transform)b2[4]).IsChildOf((Transform)a[4]) && !((Transform)a[4]).IsChildOf((Transform)b2[4]))
    { sb.AppendLine($"TIE: {a[2]} and {b2[2]} share {a[5]} and overlap — order is not guaranteed"); findings++; }
}
// 5. Input: an open popup must also block input — a raycaster on its canvas chain and a raycastTarget
//    graphic covering (nearly) the whole screen. World-space input (board taps, pinch) needs code gates too.
foreach (var it in items)
{
    var t = (Transform)it[4];
    if (!((string)it[1]).StartsWith("Canvas") || !IsTarget(t)) continue;
    var c = t.GetComponent<Canvas>();
    bool raycaster = t.GetComponentInParent<UnityEngine.UI.GraphicRaycaster>() != null;
    bool blocker = false;
    foreach (var g in t.GetComponentsInChildren<UnityEngine.UI.Graphic>())
    {
        if (!g.isActiveAndEnabled || !g.raycastTarget) continue;
        var cam = c.renderMode == RenderMode.ScreenSpaceOverlay ? null : (c.worldCamera != null ? c.worldCamera : Camera.main);
        var k = new Vector3[4]; g.rectTransform.GetWorldCorners(k);
        Vector2 a = cam == null ? (Vector2)k[0] / c.pixelRect.size : RectTransformUtility.WorldToScreenPoint(cam, k[0]) / new Vector2(cam.pixelWidth, cam.pixelHeight);
        Vector2 b = cam == null ? (Vector2)k[2] / c.pixelRect.size : RectTransformUtility.WorldToScreenPoint(cam, k[2]) / new Vector2(cam.pixelWidth, cam.pixelHeight);
        if (a.x <= 0.05f && a.y <= 0.05f && b.x >= 0.95f && b.y >= 0.95f) { blocker = true; break; }
    }
    if (!raycaster || !blocker)
    { sb.AppendLine($"INPUT LEAK RISK: {it[2]} is open but has {(raycaster ? "" : "no GraphicRaycaster, ")}{(blocker ? "" : "no full-screen raycastTarget")} — taps/pinch can reach what is behind it"); findings++; }
}
sb.Insert(0, $"layer_audit: {items.Count} draw items, {findings} finding(s)\n");
return sb.ToString();
