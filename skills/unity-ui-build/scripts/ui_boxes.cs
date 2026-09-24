// UI boxes: draws every visible text/image/button rect onto a screenshot, so a visual check reads geometry
// instead of guessing it. Texts also get two crosses: RED = centre of the rendered glyphs, BLUE = centre of
// the box they are meant to be centred in. Crosses apart = off-centre, however small it looks in the PNG.
// Run: utk screenshot --max 0   → note the "path" it prints, set pngPath below, then
//      utk exec --file ui_boxes.cs     → writes <name>_boxes.png beside it; open that PNG (Read it).
// Same Editor state as the screenshot (do not change the Game view size in between). Never modifies the scene.
// Colours: GREEN text rect, CYAN button, YELLOW image.
//
// Params — edit before running:
string pngPath = "";        // absolute path printed by `utk screenshot`
string target = "";         // root to draw (name or path); empty = every visible canvas

if (!System.IO.File.Exists(pngPath)) return $"ui_boxes: no PNG at '{pngPath}' — run utk screenshot --max 0 and paste its path";
var tex = new Texture2D(2, 2, TextureFormat.RGBA32, false);
tex.LoadImage(System.IO.File.ReadAllBytes(pngPath));
int drawn = 0; string sizeNote = "";

bool Visible(Transform t)
{
    if (!t.gameObject.activeInHierarchy) return false;
    var c = t.GetComponentInParent<Canvas>(); if (c == null || !c.enabled) return false;
    foreach (var cg in t.GetComponentsInParent<CanvasGroup>()) { if (cg.alpha <= 0.001f) return false; if (cg.ignoreParentGroups) break; }
    return true;
}
void Px(int x, int y, Color c) { if (x >= 0 && y >= 0 && x < tex.width && y < tex.height) tex.SetPixel(x, y, c); }
void Box(Rect r, Color c) // 2 px outline
{
    for (int k = 0; k < 2; k++)
    {
        for (int x = (int)r.xMin; x <= (int)r.xMax; x++) { Px(x, (int)r.yMin + k, c); Px(x, (int)r.yMax - k, c); }
        for (int y = (int)r.yMin; y <= (int)r.yMax; y++) { Px((int)r.xMin + k, y, c); Px((int)r.xMax - k, y, c); }
    }
}
void Cross(Vector2 p, Color c) { for (int d = -6; d <= 6; d++) { Px((int)p.x + d, (int)p.y, c); Px((int)p.x, (int)p.y + d, c); Px((int)p.x + d, (int)p.y + 1, c); Px((int)p.x + 1, (int)p.y + d, c); } }

foreach (var canvas in UnityEngine.Object.FindObjectsByType<Canvas>(FindObjectsSortMode.None))
{
    if (!canvas.isRootCanvas || !Visible(canvas.transform)) continue;
    var cam = canvas.renderMode == RenderMode.ScreenSpaceOverlay ? null : canvas.worldCamera;
    var screenPx = canvas.pixelRect;                       // the Game view the screenshot was taken from
    if (screenPx.width < 1) continue;
    float k = tex.width / screenPx.width;                  // screenshot px per screen px (differs when --max downscaled)
    if (Mathf.Abs(tex.height / screenPx.height - k) > 0.02f * k) sizeNote = $" (warning: PNG {tex.width}x{tex.height} is not the Game view's shape {screenPx.width:0}x{screenPx.height:0} — the Game view size changed since the capture)";
    Rect ToPng(Vector3[] w)
    {
        Vector2 a = RectTransformUtility.WorldToScreenPoint(cam, w[0]), b = RectTransformUtility.WorldToScreenPoint(cam, w[2]);
        return Rect.MinMaxRect(Mathf.Min(a.x, b.x) * k, Mathf.Min(a.y, b.y) * k, Mathf.Max(a.x, b.x) * k, Mathf.Max(a.y, b.y) * k); // PNG rows start at the bottom, like screen y
    }
    Rect RectOf(RectTransform r) { var w = new Vector3[4]; r.GetWorldCorners(w); return ToPng(w); }
    Transform root = canvas.transform;
    if (target.Length > 0)
    {
        root = null;
        foreach (var r in canvas.GetComponentsInChildren<Transform>()) if (r.name == target || r.name == target.Substring(target.LastIndexOf('/') + 1)) { root = r; break; }
        if (root == null) continue; // target is not under this canvas
    }
    foreach (var rt in root.GetComponentsInChildren<RectTransform>())
    {
        if (!Visible(rt)) continue;
        var g = rt.GetComponent<UnityEngine.UI.Graphic>();
        if (g == null || !g.enabled) continue;
        var sel = rt.GetComponent<UnityEngine.UI.Selectable>();
        if (g is TMPro.TMP_Text t)
        {
            if (string.IsNullOrEmpty(t.text)) continue;
            t.ForceMeshUpdate();
            Box(RectOf(rt), Color.green);
            var tb = t.textBounds;
            var gw = new Vector3[] { rt.TransformPoint(tb.min), Vector3.zero, rt.TransformPoint(tb.max), Vector3.zero };
            Cross(ToPng(gw).center, Color.red);
            // Same box rule as ui_audit: a label that is its background's only graphic is centred on that background.
            var p = rt.parent as RectTransform; var box = rt;
            if (p != null && p.GetComponent<UnityEngine.UI.Graphic>() != null && p.GetComponent<UnityEngine.UI.LayoutGroup>() == null)
            { int n = 0; foreach (Transform c in p) if (c.gameObject.activeInHierarchy && c.GetComponent<UnityEngine.UI.Graphic>() != null) n++; if (n == 1) box = p; }
            Cross(RectOf(box).center, Color.blue);
        }
        else Box(RectOf(rt), sel != null ? Color.cyan : Color.yellow);
        drawn++;
    }
}
var outPath = System.IO.Path.Combine(System.IO.Path.GetDirectoryName(pngPath), System.IO.Path.GetFileNameWithoutExtension(pngPath) + "_boxes.png");
tex.Apply();
System.IO.File.WriteAllBytes(outPath, tex.EncodeToPNG());
UnityEngine.Object.DestroyImmediate(tex);
return $"ui_boxes: {drawn} element(s) drawn → {outPath}{sizeNote}";
