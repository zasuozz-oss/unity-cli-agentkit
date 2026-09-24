// Sprite distortion audit: every uGUI Image drawn off its sprite's shape.
// Run: utk exec --file sprite_distortion_audit.cs --timeout 300000
// Scans prefabs under ROOT plus the scenes already open (never opens a scene).
// In Play mode it also sees runtime sizes — set fill bars to a tiny progress first.
// CANVAS lines are context (scaler settings), not faults; any other line is a finding.
const string ROOT = "Assets";   // narrow to the UI folder you touched
var sb = new System.Text.StringBuilder();
string PathOf(Transform t) { var s = t.name; while (t.parent != null) { t = t.parent; s = t.name + "/" + s; } return s; }
string Driver(RectTransform rt)
{
    // A layout group or fitter that sets this rect's size is where a stretch really comes from.
    var p = rt.parent;
    if (p == null) return "";
    var hv = p.GetComponent<UnityEngine.UI.HorizontalOrVerticalLayoutGroup>();
    if (hv != null && (hv.childControlWidth || hv.childControlHeight)) return " (size set by " + hv.GetType().Name + " on parent)";
    if (p.GetComponent<UnityEngine.UI.GridLayoutGroup>() != null) return " (size set by GridLayoutGroup cellSize)";
    if (rt.GetComponent<UnityEngine.UI.AspectRatioFitter>() == null && (rt.anchorMin.x != rt.anchorMax.x || rt.anchorMin.y != rt.anchorMax.y)) return " (stretch anchors: size follows parent)";
    return "";
}
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
        if (r.x <= 0.5f || r.y <= 0.5f) continue;   // size not resolved here (prefab asset outside a canvas, fitter not run yet) — run in Play for those
        float d = (r.x / r.y) / (ss.x / ss.y);       // rect aspect vs sprite aspect; 1 = same shape
        bool offShape = d > 1.04f || d < 0.96f;
        string shrunk = "", issue = "";
        if (UnityEditor.AssetImporter.GetAtPath(UnityEditor.AssetDatabase.GetAssetPath(sp)) is UnityEditor.TextureImporter ti
            && ti.spriteImportMode == UnityEditor.SpriteImportMode.Single)
        { ti.GetSourceTextureWidthAndHeight(out int w, out int h); if (w > ss.x + 1 || h > ss.y + 1) shrunk = $" SHRUNK src {w}x{h}"; }
        if (img.type == UnityEngine.UI.Image.Type.Simple && !img.preserveAspect && offShape) issue = $" STRETCH x{d:F2}" + Driver(rt);
        if (img.type == UnityEngine.UI.Image.Type.Sliced || img.type == UnityEngine.UI.Image.Type.Tiled)
        {
            var b = sp.border / (img.pixelsPerUnitMultiplier * sp.pixelsPerUnit / 100f);   // canvas units, ref PPU 100
            if (b.x + b.z > r.x + 0.5f || b.y + b.w > r.y + 0.5f) issue = $" SLICE-SQUASH border {sp.border} ppum {img.pixelsPerUnitMultiplier} (rect narrower than its two caps)";
            if (sp.border == Vector4.zero && img.type == UnityEngine.UI.Image.Type.Sliced && offShape) issue = $" SLICED-NO-BORDER x{d:F2} (stretches like Simple)";
        }
        if (img.type == UnityEngine.UI.Image.Type.Filled && !img.preserveAspect && offShape) issue = $" FILLED-STRETCH x{d:F2} (Filled draws like Simple: sprite stretched to the full rect)";
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
