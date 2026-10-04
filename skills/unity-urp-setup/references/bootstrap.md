# URP bootstrap (Unity 6000.0.x, live Editor)

The scripts below are the whole bootstrap. Do NOT rediscover any step.

Every utk call: `--project-path "<repo>"` (without it utk may report
"Multiple Unity Editor instances" for one Editor, a quirk, never a reason to
inspect processes). Never batchmode, never play mode.

## Facts that cost turns

| Fact | Consequence |
|---|---|
| URP 17.0.4 is bundled with 6000.0.80f1 (resolves offline) | pin `"com.unity.render-pipelines.universal": "17.0.4"`; other Editor versions: look in `<Editor>/Data/Resources/PackageManager/BuiltInPackages` |
| Package types are not loaded after ONE `utk editor refresh` | run refresh **twice**, poll `utk status` between |
| `utk exec` assembly does not reference `Unity.RenderPipelines.Universal.Runtime` | no `using UnityEngine.Rendering.Universal`; use `System.Type.GetType("..., Unity.RenderPipelines.Universal.Runtime")` + `SerializedObject` |
| 3D renderer = `UniversalRendererData`, 2D = `Renderer2DData` | pick by argument; `Renderer2DData` with 3D meshes renders nothing |
| Quality levels override the Graphics pipeline | set `m_QualitySettings[i].customRenderPipeline` on EVERY level, else some levels stay Built-in |

## Steps

1. **Manifest**: add the dependency to `Packages/manifest.json` (keep the
   existing `com.unity.pipeline` utk entry).
2. **Load the package**:
   ```
   utk editor refresh --project-path "<repo>"
   utk status --project-path "<repo>"          # poll until ready, cap 120 s
   utk editor refresh --project-path "<repo>"  # second refresh loads package types
   ```
3. **Setup script**: write `Temp/urp_setup.cs` (below), edit the two asset
   names and the renderer type, then
   `utk exec --file Temp/urp_setup.cs --project-path "<repo>"`.
   Output `URP types not found` means step 2 did not finish: refresh once more.
4. **PlayerSettings**: write `Temp/player_settings.cs` (below), edit names,
   run it the same way.
5. **Verify** (one pass):
   ```
   utk console --type error --project-path "<repo>"
   grep m_CustomRenderPipeline ProjectSettings/GraphicsSettings.asset   # fileID: 11400000, guid: <urp asset>
   ```
   The setup script already returns `active=True`
   (`GraphicsSettings.currentRenderPipeline != null`). Screenshot only if a
   criterion asks (no pink materials).

## Temp/urp_setup.cs

```csharp
// URP types are not referenced by the exec assembly: string type names + SerializedObject only.
const string rendererPath = "Assets/Settings/Game_Renderer.asset";
const string urpPath = "Assets/Settings/Game_URP.asset";
// 3D: UniversalRendererData. 2D: Renderer2DData.
var rendererType = System.Type.GetType("UnityEngine.Rendering.Universal.UniversalRendererData, Unity.RenderPipelines.Universal.Runtime");
var urpType = System.Type.GetType("UnityEngine.Rendering.Universal.UniversalRenderPipelineAsset, Unity.RenderPipelines.Universal.Runtime");
if (rendererType == null || urpType == null) return "URP types not found";

if (!UnityEditor.AssetDatabase.IsValidFolder("Assets/Settings"))
    UnityEditor.AssetDatabase.CreateFolder("Assets", "Settings");

var renderer = UnityEditor.AssetDatabase.LoadAssetAtPath(rendererPath, rendererType);
if (renderer == null) { renderer = ScriptableObject.CreateInstance(rendererType); UnityEditor.AssetDatabase.CreateAsset(renderer, rendererPath); }
var urp = UnityEditor.AssetDatabase.LoadAssetAtPath(urpPath, urpType);
if (urp == null) { urp = ScriptableObject.CreateInstance(urpType); UnityEditor.AssetDatabase.CreateAsset(urp, urpPath); }

var soUrp = new UnityEditor.SerializedObject(urp);
var list = soUrp.FindProperty("m_RendererDataList");
list.arraySize = 1;
list.GetArrayElementAtIndex(0).objectReferenceValue = renderer;
soUrp.FindProperty("m_DefaultRendererIndex").intValue = 0;
soUrp.ApplyModifiedPropertiesWithoutUndo();
UnityEditor.EditorUtility.SetDirty(urp);

var gfx = UnityEditor.AssetDatabase.LoadAllAssetsAtPath("ProjectSettings/GraphicsSettings.asset")[0];
var soGfx = new UnityEditor.SerializedObject(gfx);
soGfx.FindProperty("m_CustomRenderPipeline").objectReferenceValue = urp;
soGfx.ApplyModifiedPropertiesWithoutUndo();

var qs = UnityEditor.AssetDatabase.LoadAllAssetsAtPath("ProjectSettings/QualitySettings.asset")[0];
var soQs = new UnityEditor.SerializedObject(qs);
var levels = soQs.FindProperty("m_QualitySettings");
for (int i = 0; i < levels.arraySize; i++)
    levels.GetArrayElementAtIndex(i).FindPropertyRelative("customRenderPipeline").objectReferenceValue = urp;
soQs.ApplyModifiedPropertiesWithoutUndo();

UnityEditor.AssetDatabase.SaveAssets();
UnityEditor.EditorApplication.ExecuteMenuItem("File/Save Project");
return renderer.GetType().FullName + " qualityLevels=" + levels.arraySize
     + " active=" + (UnityEngine.Rendering.GraphicsSettings.currentRenderPipeline != null);
```

## Temp/player_settings.cs

```csharp
PlayerSettings.companyName = "Company";
PlayerSettings.productName = "Game";
PlayerSettings.SetApplicationIdentifier(UnityEditor.Build.NamedBuildTarget.Android, "com.company.game");

PlayerSettings.defaultInterfaceOrientation = UIOrientation.Portrait;
PlayerSettings.allowedAutorotateToPortrait = true;
PlayerSettings.allowedAutorotateToPortraitUpsideDown = false;
PlayerSettings.allowedAutorotateToLandscapeLeft = false;
PlayerSettings.allowedAutorotateToLandscapeRight = false;
PlayerSettings.colorSpace = ColorSpace.Linear;

// Active Input Handling has no public API: poke the settings asset.
// 0 = Input Manager (legacy), 1 = Input System, 2 = both.
var ps = UnityEditor.AssetDatabase.LoadAllAssetsAtPath("ProjectSettings/ProjectSettings.asset")[0];
var so = new UnityEditor.SerializedObject(ps);
so.FindProperty("activeInputHandler").intValue = 0;
so.ApplyModifiedPropertiesWithoutUndo();

UnityEditor.AssetDatabase.SaveAssets();
UnityEditor.EditorApplication.ExecuteMenuItem("File/Save Project");
return PlayerSettings.productName + " | orient=" + PlayerSettings.defaultInterfaceOrientation
     + " | input=" + so.FindProperty("activeInputHandler").intValue;
```

## Not in scope

Converting existing materials (`Standard` to `Universal Render Pipeline/Lit`
is the interactive Render Pipeline Converter), post-processing volumes, 2D
lights. Built-in pipeline: `unity-builtin-setup`.
