package main

// gcSnippet destroys the OS fallback fonts Unity leaks (measured on Unity
// 6000.0.84f1; UnityCsReference TextSettings.cs). TextSettings.OnEnable nulls
// the static list of five OS fallback FontAssets and the next text layout
// builds five new ones; the old ones are HideFlags.DontSave, so nothing ever
// unloads them. Every domain reload, test run and Play session leaks a set:
// 21,171 of them (each with its atlas texture and material) after 22 hours,
// by which point a reload restores 118k objects and takes 35 s instead of 3.
//
// What may go: a nameless, asset-less DontSave DynamicOS font of one of the
// live list's families that is not in the live list AND whose lookup table is
// null. That table is not serialized, so null means no code has read the font
// since the last domain reload — nothing managed can still point at it. A
// font leaked since that reload keeps its table and waits for the next one.
// Any internal this relies on going missing turns the call into a no-op.
const gcSnippet = `var BF = System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.NonPublic | System.Reflection.BindingFlags.Public;
var dict = typeof(UnityEngine.TextCore.Text.FontAsset).GetField("m_CharacterLookupDictionary", BF);
var prop = typeof(UnityEngine.TextCore.Text.TextSettings).GetProperty("fallbackOSFontAssets", BF);
if (dict == null || prop == null) return "skipped: this Unity version keeps its OS fallback fonts differently";
var live = new System.Collections.Generic.HashSet<UnityEngine.Object>();
var families = new System.Collections.Generic.HashSet<string>();
foreach (var ts in UnityEngine.Resources.FindObjectsOfTypeAll<UnityEngine.TextCore.Text.TextSettings>()) {
  var list = prop.GetValue(ts) as System.Collections.IEnumerable;
  if (list == null) continue;
  foreach (var o in list) { var fa = o as UnityEngine.TextCore.Text.FontAsset; if (fa != null) { live.Add(fa); families.Add(fa.faceInfo.familyName); } }
}
int destroyed = 0, waiting = 0;
foreach (var fa in UnityEngine.Resources.FindObjectsOfTypeAll<UnityEngine.TextCore.Text.FontAsset>()) {
  if (live.Contains(fa) || fa.name != "" || fa.hideFlags != UnityEngine.HideFlags.DontSave || fa.atlasPopulationMode != UnityEngine.TextCore.Text.AtlasPopulationMode.DynamicOS) continue;
  if (UnityEditor.AssetDatabase.GetAssetPath(fa) != "" || !families.Contains(fa.faceInfo.familyName)) continue;
  if (dict.GetValue(fa) != null) { waiting++; continue; }
  if (fa.atlasTextures != null) foreach (var t in fa.atlasTextures) if (t != null) UnityEngine.Object.DestroyImmediate(t);
  if (fa.material != null) UnityEngine.Object.DestroyImmediate(fa.material);
  UnityEngine.Object.DestroyImmediate(fa);
  destroyed++;
}
return "destroyed " + destroyed + " leaked OS fallback fonts (" + waiting + " more wait for the next domain reload, " + live.Count + " in use)";`
