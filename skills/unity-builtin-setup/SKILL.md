---
name: unity-builtin-setup
description: Use when bootstrapping a fresh Unity 6 project on the Built-in Render Pipeline through utk against the open Editor — confirm no SRP is active, clear leftover URP assets (pink materials), set PlayerSettings, verify. Also for "no URP", "legacy pipeline", "Standard shader project".
---

# Built-in RP setup (Unity 6000.0.x, live Editor)

Built-in is what a project created with `Unity -createProject` gives you:
nothing to install. The job is to make sure nothing *else* is active
(`currentRenderPipeline == null`, no URP in manifest) and to set PlayerSettings.

Every utk call: `--project-path "<repo>"` (utk quirk: without it one Editor
can show as "Multiple Unity Editor instances"). Never batchmode, never play mode.

## Steps

1. **Manifest**: `Packages/manifest.json` must NOT list
   `com.unity.render-pipelines.universal` / `.high-definition`. A URP-template
   project (Unity Hub) ships with it: remove the line, then
   `utk editor refresh --project-path "<repo>"` (once is enough for a removal).
2. **Clear SRP assets** only if a template left them
   (`Assets/Settings/*URP*.asset`, `*Renderer*.asset`): delete the files AND run
   `Temp/builtin_clear.cs` below. A dangling `m_CustomRenderPipeline` keeps the
   Editor on a missing pipeline and everything renders pink.
3. **PlayerSettings**: write `Temp/player_settings.cs` (below), edit names,
   `utk exec --file Temp/player_settings.cs --project-path "<repo>"`.
4. **Verify** (one pass):
   ```
   utk console --type error --project-path "<repo>"
   grep m_CustomRenderPipeline ProjectSettings/GraphicsSettings.asset   # {fileID: 0}
   ```

## Temp/builtin_clear.cs

```csharp
var gfx = UnityEditor.AssetDatabase.LoadAllAssetsAtPath("ProjectSettings/GraphicsSettings.asset")[0];
var soGfx = new UnityEditor.SerializedObject(gfx);
soGfx.FindProperty("m_CustomRenderPipeline").objectReferenceValue = null;
soGfx.ApplyModifiedPropertiesWithoutUndo();

var qs = UnityEditor.AssetDatabase.LoadAllAssetsAtPath("ProjectSettings/QualitySettings.asset")[0];
var soQs = new UnityEditor.SerializedObject(qs);
var levels = soQs.FindProperty("m_QualitySettings");
for (int i = 0; i < levels.arraySize; i++)
    levels.GetArrayElementAtIndex(i).FindPropertyRelative("customRenderPipeline").objectReferenceValue = null;
soQs.ApplyModifiedPropertiesWithoutUndo();

UnityEditor.AssetDatabase.SaveAssets();
UnityEditor.EditorApplication.ExecuteMenuItem("File/Save Project");
return "builtin active=" + (UnityEngine.Rendering.GraphicsSettings.currentRenderPipeline == null);
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

## Shaders

Built-in = `Standard`, `Mobile/Diffuse`, `Sprites/Default`, `UI/Default`.
`Universal Render Pipeline/*` materials render pink here; do not use them.
Switching later: `unity-urp-setup` (its `references/bootstrap.md`).
