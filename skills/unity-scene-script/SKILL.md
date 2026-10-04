---
name: unity-scene-script
description: Use when building or rebuilding a Unity scene or prefab set from one idempotent Editor C# script run through `utk exec --file` against the open Editor, instead of ad-hoc exec snippets — "build scene", "scene setup script", "create prefab from script", "wire scene objects".
---

# Unity scene script (`Tools/build_<scene>.cs` via `utk exec --file`)

One scene = one script = one command. The script is the source of truth for
the scene's object tree; `.unity` is its build output. Re-running must
produce the same scene (idempotent), so a later change edits the script and
re-runs it rather than patching the scene by hand.

## Rules

| Rule | Why |
|---|---|
| `utk exec --file Tools/build_<scene>.cs --timeout 60000 --project-path "<repo>"` | default exec timeout is 5000 ms; a scene build exceeds it and the reply says timeout even though the script finished |
| Timeout reply → check the artifact on disk (`ls Assets/Scenes`, `grep` the .unity), never re-run blind | re-running a half-finished build duplicates objects |
| Script body only: no `using`, no class, no method — top-level statements ending in `return "<summary>";` | that is what `utk exec` compiles (Roslyn eval) |
| Fully-qualify Editor types (`UnityEditor.SceneManagement.EditorSceneManager`, `UnityEditor.PrefabUtility`) | the eval assembly has `UnityEngine` + `UnityEditor` only; SRP/package types go through `System.Type.GetType` (see `unity-urp-setup`) |
| Find-or-create every object by name; never `new GameObject` unconditionally | idempotence |
| Prefabs: `PrefabUtility.SaveAsPrefabAsset` from a temp object, then `Object.DestroyImmediate` the temp | leaves nothing in the scene |
| End with `EditorSceneManager.SaveScene` + `AssetDatabase.SaveAssets` | otherwise the Editor holds the change in memory only |
| After the script: `utk editor refresh` → `utk console --type error` | `utk editor refresh` blocks until compile finishes and exits non-zero on errors |

## Skeleton

```csharp
// Tools/build_<Scene>.cs — idempotent, run via utk exec --file (timeout 60000)
const string scenePath = "Assets/Scenes/<Scene>.unity";
var scene = System.IO.File.Exists(scenePath)
    ? UnityEditor.SceneManagement.EditorSceneManager.OpenScene(scenePath)
    : UnityEditor.SceneManagement.EditorSceneManager.NewScene(
        UnityEditor.SceneManagement.NewSceneSetup.DefaultGameObjects,
        UnityEditor.SceneManagement.NewSceneMode.Single);

GameObject Find(string name) { var go = GameObject.Find(name); return go != null ? go : new GameObject(name); }
T Ensure<T>(GameObject go) where T : Component { var c = go.GetComponent<T>(); return c != null ? c : go.AddComponent<T>(); }

var root = Find("<Scene>Root");
var canvasGo = Find("Canvas"); canvasGo.transform.SetParent(root.transform, false);
var canvas = Ensure<Canvas>(canvasGo); canvas.renderMode = RenderMode.ScreenSpaceOverlay;
Ensure<UnityEngine.UI.CanvasScaler>(canvasGo);
Ensure<UnityEngine.UI.GraphicRaycaster>(canvasGo);
// ... one block per object, each find-or-create ...

UnityEditor.SceneManagement.EditorSceneManager.SaveScene(scene, scenePath);
UnityEditor.AssetDatabase.SaveAssets();
return "<Scene> built: " + root.transform.childCount + " children";
```

Popup panels are prefabs under `Assets/Prefabs/UI/<Key>.prefab`, toggled via
the Canvas component, never additive scenes.

## Not in scope

Play-mode verification, screenshots, prefab variants, Addressables.
