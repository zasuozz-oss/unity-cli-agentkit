// Top-down sort audit: upright things on the World layer whose order does not follow their foot, ties between
// overlapping things, and upright props standing on the same ground (which no order can fix).
// utk exec --file topdown_sort_audit.cs   (map scene open; Edit or Play mode; in Play run it in every map state)
// Only UnityEngine / UnityEditor are in scope (no LINQ). Game-agnostic.
//
// Params — edit before running:
string worldLayer = "World";        // the layer holding every upright thing
string overlayLayer = "WorldOverlay"; // children on this layer never define a composite's foot
float perUnit = 100f;               // must match YSort.PerUnit
int tolerance = 5;                  // orders of slack (0.05 u at 100/unit) before STALE
float baseFrac = 0.25f;             // a prop's base = bottom 25% of its bounds
float maxBaseOverlap = 0.30f;       // FOOTPRINT when two bases overlap by more than this share of the smaller
int maxLines = 60;

var sb = new System.Text.StringBuilder();
int stale = 0, ties = 0, footprints = 0, lines = 0;
void Report(string s) { if (lines++ < maxLines) sb.AppendLine(s); }
int Order(float footY) => Mathf.RoundToInt(-footY * perUnit);
string PathOf(Transform t) => (t.parent != null ? t.parent.name + "/" : "") + t.name;
var items = new System.Collections.Generic.List<(string name, Bounds b, int order)>();

// composites: a SortingGroup's order must follow the bottom of the art showing NOW (not hidden states)
foreach (var g in Object.FindObjectsByType<UnityEngine.Rendering.SortingGroup>(FindObjectsSortMode.None))
{
    if (g.sortingLayerName != worldLayer || g.transform.parent?.GetComponentInParent<UnityEngine.Rendering.SortingGroup>() != null) continue;
    bool any = false;
    var b = new Bounds();
    foreach (var sr in g.GetComponentsInChildren<SpriteRenderer>()) // active children only
    {
        if (!sr.enabled || sr.sprite == null || sr.sortingLayerName == overlayLayer) continue;
        if (!any) { b = sr.bounds; any = true; } else b.Encapsulate(sr.bounds);
    }
    if (!any) continue;
    int want = Order(b.min.y);
    if (Mathf.Abs(g.sortingOrder - want) > tolerance) { stale++; Report($"STALE group {PathOf(g.transform)} order {g.sortingOrder}, showing art foot y={b.min.y:0.00} wants ~{want}"); }
    items.Add((PathOf(g.transform), b, g.sortingOrder));
}

// single sprites outside any group: order must follow bounds.min.y (the art's bottom), never the pivot/centre
foreach (var sr in Object.FindObjectsByType<SpriteRenderer>(FindObjectsSortMode.None))
{
    if (!sr.enabled || sr.sprite == null || sr.sortingLayerName != worldLayer) continue;
    if (sr.GetComponentInParent<UnityEngine.Rendering.SortingGroup>() != null) continue;
    int want = Order(sr.bounds.min.y);
    if (Mathf.Abs(sr.sortingOrder - want) > tolerance) { stale++; Report($"STALE {PathOf(sr.transform)} order {sr.sortingOrder}, foot y={sr.bounds.min.y:0.00} wants ~{want}"); }
    items.Add((PathOf(sr.transform), sr.bounds, sr.sortingOrder));
}

// pairs: O(n²), fine for a few thousand items
for (int i = 0; i < items.Count; i++)
    for (int j = i + 1; j < items.Count; j++)
    {
        var a = items[i]; var c = items[j];
        if (!a.b.Intersects(c.b)) continue;
        if (a.order == c.order) { ties++; Report($"TIE {a.name} / {c.name} order {a.order}"); }
        var ra = new Rect(a.b.min.x, a.b.min.y, a.b.size.x, a.b.size.y * baseFrac);
        var rc = new Rect(c.b.min.x, c.b.min.y, c.b.size.x, c.b.size.y * baseFrac);
        float w = Mathf.Min(ra.xMax, rc.xMax) - Mathf.Max(ra.xMin, rc.xMin);
        float h = Mathf.Min(ra.yMax, rc.yMax) - Mathf.Max(ra.yMin, rc.yMin);
        if (w <= 0 || h <= 0) continue;
        float share = w * h / Mathf.Max(1e-6f, Mathf.Min(ra.width * ra.height, rc.width * rc.height));
        if (share > maxBaseOverlap) { footprints++; Report($"FOOTPRINT {a.name} / {c.name} bases overlap {share:P0}"); }
    }

return $"{items.Count} upright items: STALE {stale}, TIE {ties}, FOOTPRINT {footprints}" + (lines > maxLines ? $" (first {maxLines} shown)" : "") + "\n" + sb;
