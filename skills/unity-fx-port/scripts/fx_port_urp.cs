// FX port fix-up: makes imported (legacy / Built-in / UIParticle) particle prefabs render under URP.
// Remaps every non-URP material in <Dir>Materials to "Universal Render Pipeline/Particles/Unlit" (additive when the
// name contains "Add", else alpha; BaseColor = 2 x legacy _TintColor, which the legacy shaders doubled), strips
// missing scripts, re-enables renderers, assigns sorting and root scale. Idempotent.
// Copy to <repo>/Tools/fx_port_<pack>.cs, edit the params, then:  utk exec --file Tools/fx_port_<pack>.cs --timeout 120000
// Only UnityEngine / UnityEditor are in scope (no LINQ). Game-agnostic.
//
// Params — edit before running:
const string Dir = "Assets/Art/FX/Pack/";           // holds Materials/ and Prefabs/
const string Fallback = "";                          // a ported alpha material for null / Default-* slots, e.g. Dir + "Materials/X.mat"
const string UiLayer = "UI", WorldLayer = "Default"; // sorting layers that exist in this project
const int UiBase = 2010, WorldBase = 50;             // UI FX: above the canvas order; world FX: inside the world band
System.Func<string, bool> isUi = n => n.EndsWith("_UI"); // which prefabs are UI FX
// Root localScale per prefab name. UIParticle drew UI FX in "world units" (its m_Scale, usually 100 = 1 unit per
// 100 canvas px); under a Screen Space - Camera canvas at 0.01/px that needs x100 on the root. Read each source
// prefab's UIParticle m_Scale before guessing; prefabs already authored at x100 get 1.
var scales = new System.Collections.Generic.Dictionary<string, float> { /* { "Eff_Burst_UI", 100 }, */ };

var urp = Shader.Find("Universal Render Pipeline/Particles/Unlit");
if (urp == null) return "ABORT: URP Particles/Unlit not found (is URP installed?)";
var log = new System.Text.StringBuilder();
var fallback = Fallback == "" ? null : UnityEditor.AssetDatabase.LoadAssetAtPath<Material>(Fallback);

foreach (var guid in UnityEditor.AssetDatabase.FindAssets("t:Material", new[] { Dir + "Materials" }))
{
    var m = UnityEditor.AssetDatabase.LoadAssetAtPath<Material>(UnityEditor.AssetDatabase.GUIDToAssetPath(guid));
    if (m.shader == urp) continue; // already ported
    // the legacy shader is gone, so its properties are only reachable through the serialized data
    var so = new UnityEditor.SerializedObject(m);
    Texture tex = null;
    var tint = new Color(0.5f, 0.5f, 0.5f, 0.5f);
    var texEnvs = so.FindProperty("m_SavedProperties.m_TexEnvs");
    for (int i = 0; i < texEnvs.arraySize; i++)
    {
        var e = texEnvs.GetArrayElementAtIndex(i);
        if (e.FindPropertyRelative("first").stringValue == "_MainTex")
            tex = e.FindPropertyRelative("second.m_Texture").objectReferenceValue as Texture;
    }
    var colors = so.FindProperty("m_SavedProperties.m_Colors");
    for (int i = 0; i < colors.arraySize; i++)
    {
        var e = colors.GetArrayElementAtIndex(i);
        if (e.FindPropertyRelative("first").stringValue == "_TintColor") tint = e.FindPropertyRelative("second").colorValue;
    }
    bool add = m.name.Contains("Add");
    m.shader = urp;
    m.SetTexture("_BaseMap", tex);
    m.SetColor("_BaseColor", new Color(Mathf.Min(1, tint.r * 2), Mathf.Min(1, tint.g * 2), Mathf.Min(1, tint.b * 2), Mathf.Min(1, tint.a * 2)));
    m.SetFloat("_Surface", 1);
    m.SetFloat("_Blend", add ? 2 : 0);
    m.SetFloat("_SrcBlend", 5);
    m.SetFloat("_DstBlend", add ? 1 : 10);
    m.SetFloat("_SrcBlendAlpha", 1);
    m.SetFloat("_DstBlendAlpha", add ? 1 : 10);
    m.SetFloat("_ZWrite", 0);
    m.SetFloat("_Cull", 0);
    m.EnableKeyword("_SURFACE_TYPE_TRANSPARENT");
    m.SetOverrideTag("RenderType", "Transparent");
    m.renderQueue = 3000;
    UnityEditor.EditorUtility.SetDirty(m);
    log.AppendLine("mat " + m.name + (add ? " add" : " alpha") + " tex=" + (tex != null ? tex.name : "-"));
}
UnityEditor.AssetDatabase.SaveAssets();

foreach (var guid in UnityEditor.AssetDatabase.FindAssets("t:Prefab", new[] { Dir + "Prefabs" }))
{
    var path = UnityEditor.AssetDatabase.GUIDToAssetPath(guid);
    var root = UnityEditor.PrefabUtility.LoadPrefabContents(path);
    int removed = 0;
    foreach (var t in root.GetComponentsInChildren<Transform>(true))
        removed += UnityEditor.GameObjectUtility.RemoveMonoBehavioursWithMissingScript(t.gameObject);
    bool ui = isUi(root.name);
    var rs = root.GetComponentsInChildren<ParticleSystemRenderer>(true);
    int min = int.MaxValue;
    foreach (var r in rs) min = Mathf.Min(min, r.sortingOrder);
    int nullSlots = 0;
    foreach (var r in rs)
    {
        // keep the pack's relative order, rebased into this project's band
        r.sortingLayerName = ui ? UiLayer : WorldLayer;
        r.sortingOrder = (ui ? UiBase : WorldBase) + (r.sortingOrder - min);
        // UIParticle drew these, so the renderer was off: turn it on, and fill null / builtin-default slots
        var mats = r.sharedMaterials;
        var slot = mats.Length > 0 ? mats[0] : null;
        if (slot == null || slot.name.StartsWith("Default-")) { slot = fallback; nullSlots++; }
        r.sharedMaterials = new[] { slot };
        r.enabled = true;
    }
    if (scales.TryGetValue(root.name, out var s)) root.transform.localScale = Vector3.one * s;
    UnityEditor.PrefabUtility.SaveAsPrefabAsset(root, path);
    UnityEditor.PrefabUtility.UnloadPrefabContents(root);
    log.AppendLine("prefab " + System.IO.Path.GetFileName(path) + " ps=" + rs.Length + " strippedScripts=" + removed
        + (nullSlots > 0 ? " fallbackSlots=" + nullSlots + (fallback == null ? " (NO FALLBACK SET)" : "") : "") + (ui ? " ui" : " world"));
}
UnityEditor.AssetDatabase.SaveAssets();
return log.ToString();
