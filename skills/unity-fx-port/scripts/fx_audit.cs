// FX audit: lists particle prefabs that will render wrong or never end. Edit mode is fine.
// utk exec --file fx_audit.cs   (copy it into the project first: --file resolves from the project root)
// Only UnityEngine / UnityEditor are in scope (no LINQ). Game-agnostic.
//
// Params — edit before running:
string[] folders = { "Assets" };   // where the FX prefabs live
float longLife = 10f;              // a constant startLifetime above this (seconds) is flagged

var sb = new System.Text.StringBuilder();
int checkedPrefabs = 0;
foreach (var g in AssetDatabase.FindAssets("t:Prefab", folders))
{
    var go = AssetDatabase.LoadAssetAtPath<GameObject>(AssetDatabase.GUIDToAssetPath(g));
    var systems = go.GetComponentsInChildren<ParticleSystem>(true);
    if (systems.Length == 0) continue;
    checkedPrefabs++;
    foreach (var t in go.GetComponentsInChildren<Transform>(true))
        if (GameObjectUtility.GetMonoBehavioursWithMissingScriptCount(t.gameObject) > 0)
            sb.AppendLine(go.name + "/" + t.name + " MISSING_SCRIPT");
    foreach (var ps in systems)
    {
        var r = ps.GetComponent<ParticleSystemRenderer>();
        string bad = "";
        if (r == null) bad += " NO_RENDERER";
        else
        {
            if (!r.enabled && ps.emission.enabled) bad += " RENDERER_OFF";
            var mats = new System.Collections.Generic.List<Material>(r.sharedMaterials);
            if (r.trailMaterial != null || ps.trails.enabled) mats.Add(r.trailMaterial);
            foreach (var m in mats)
            {
                if (m == null) { bad += " NULL_MAT"; continue; }
                if (!m.shader.isSupported || m.shader.name == "Hidden/InternalErrorShader") bad += " BROKEN_SHADER:" + m.name;
                else if (!m.shader.name.StartsWith("Universal Render Pipeline") && !m.shader.name.StartsWith("Shader Graphs/"))
                    bad += " NON_URP:" + m.shader.name;
            }
        }
        var ts = ps.textureSheetAnimation;
        if (ts.enabled && ts.mode == ParticleSystemAnimationMode.Sprites)
            for (int i = 0; i < ts.spriteCount; i++) if (ts.GetSprite(i) == null) bad += " NULL_SPRITE" + i;
        if (ps.main.loop) bad += " LOOP";
        if (ps.main.startLifetime.constantMax > longLife) bad += " LONG_LIFE" + ps.main.startLifetime.constantMax;
        if (bad != "") sb.AppendLine(go.name + "/" + ps.name + bad);
    }
}
return checkedPrefabs + " FX prefabs checked\n" + (sb.Length == 0 ? "clean" : sb.ToString());
