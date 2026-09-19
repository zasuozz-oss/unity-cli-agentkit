---
name: unity-editor-tools
description: "Use when the user explicitly requests Unity Editor tooling only: EditorWindow, CustomEditor, PropertyDrawer, MenuItem, SceneView overlays, SerializedObject, or asset pipeline automation."
---

# Custom Editor Scripting

## Overview
Extend the Unity Editor with custom inspectors, property drawers, editor windows, and scene tools. Automate workflows and improve designer experience.

## When to Use
- Use for custom inspectors with visual previews
- Use for property drawers (reusable across types)
- Use for editor windows (batch operations, tools)
- Use for scene view overlays and handles
- Use for menu items and context menus

## Editor Script Types

| Type | Use Case | Base Class |
|------|----------|------------|
| **CustomEditor** | Override inspector for a type | `Editor` |
| **PropertyDrawer** | Custom field rendering | `PropertyDrawer` |
| **EditorWindow** | Standalone tool window | `EditorWindow` |
| **SceneView overlay** | In-scene editing tools | `EditorTool` |

## Best Practices
- ✅ Use `SerializedProperty` and `SerializedObject` for Undo support
- ✅ Place all editor scripts in an Editor folder or Editor-only asmdef
- ✅ Use `EditorGUILayout` for automatic layout
- ✅ Call `serializedObject.ApplyModifiedProperties()` after changes
- ❌ **NEVER** use editor classes in runtime code
- ❌ **NEVER** modify fields directly (breaks Undo)
- ❌ **NEVER** forget `[CustomEditor(typeof(T))]` attribute

## Few-Shot Examples

### Example 1: Custom Inspector with Preview
**User**: "Custom inspector for enemy config."

**Agent**:
```csharp
[CustomEditor(typeof(EnemyConfig))]
public class EnemyConfigEditor : Editor
{
    private SerializedProperty _nameProp;
    private SerializedProperty _healthProp;
    private SerializedProperty _speedProp;
    private SerializedProperty _iconProp;

    private void OnEnable()
    {
        _nameProp = serializedObject.FindProperty("_name");
        _healthProp = serializedObject.FindProperty("_maxHealth");
        _speedProp = serializedObject.FindProperty("_moveSpeed");
        _iconProp = serializedObject.FindProperty("_icon");
    }

    public override void OnInspectorGUI()
    {
        serializedObject.Update();

        EditorGUILayout.PropertyField(_nameProp);

        EditorGUILayout.Space();
        EditorGUILayout.LabelField("Combat Stats", EditorStyles.boldLabel);
        EditorGUILayout.Slider(_healthProp, 1f, 1000f, "Max Health");
        EditorGUILayout.Slider(_speedProp, 0.5f, 20f, "Move Speed");

        EditorGUILayout.Space();
        EditorGUILayout.PropertyField(_iconProp);

        // Sprite preview
        var sprite = (Sprite)_iconProp.objectReferenceValue;
        if (sprite != null)
        {
            GUILayout.Label(AssetPreview.GetAssetPreview(sprite),
                GUILayout.Width(64), GUILayout.Height(64));
        }

        serializedObject.ApplyModifiedProperties();
    }
}
```

### Example 2: Editor Window for Batch Operations
**User**: "Window to find all missing scripts."

**Agent**:
```csharp
public class MissingScriptFinder : EditorWindow
{
    private readonly List<GameObject> _results = new();
    private Vector2 _scrollPos;

    [MenuItem("Tools/Find Missing Scripts")]
    public static void ShowWindow()
    {
        GetWindow<MissingScriptFinder>("Missing Scripts");
    }

    private void OnGUI()
    {
        if (GUILayout.Button("Scan Scene"))
        {
            _results.Clear();
            foreach (var go in FindObjectsOfType<GameObject>(true))
            {
                var components = go.GetComponents<Component>();
                foreach (var c in components)
                {
                    if (c == null)
                    {
                        _results.Add(go);
                        break;
                    }
                }
            }
        }

        EditorGUILayout.LabelField($"Found: {_results.Count}");

        _scrollPos = EditorGUILayout.BeginScrollView(_scrollPos);
        foreach (var go in _results)
        {
            EditorGUILayout.BeginHorizontal();
            EditorGUILayout.ObjectField(go, typeof(GameObject), true);
            if (GUILayout.Button("Select", GUILayout.Width(60)))
                Selection.activeGameObject = go;
            EditorGUILayout.EndHorizontal();
        }
        EditorGUILayout.EndScrollView();
    }
}
```

### Example 3: Property Drawer
**User**: "Custom drawer for a MinMax range."

**Agent**:
```csharp
// Attribute
public class MinMaxRangeAttribute : PropertyAttribute
{
    public float Min;
    public float Max;
    public MinMaxRangeAttribute(float min, float max) { Min = min; Max = max; }
}

// Drawer
[CustomPropertyDrawer(typeof(MinMaxRangeAttribute))]
public class MinMaxRangeDrawer : PropertyDrawer
{
    public override void OnGUI(Rect position, SerializedProperty property, GUIContent label)
    {
        var attr = (MinMaxRangeAttribute)attribute;
        var minProp = property.FindPropertyRelative("Min");
        var maxProp = property.FindPropertyRelative("Max");

        float minVal = minProp.floatValue;
        float maxVal = maxProp.floatValue;

        EditorGUI.BeginProperty(position, label, property);

        position = EditorGUI.PrefixLabel(position, label);
        float fieldWidth = 50f;
        var minRect = new Rect(position.x, position.y, fieldWidth, position.height);
        var sliderRect = new Rect(position.x + fieldWidth + 5, position.y,
            position.width - fieldWidth * 2 - 10, position.height);
        var maxRect = new Rect(position.xMax - fieldWidth, position.y,
            fieldWidth, position.height);

        minVal = EditorGUI.FloatField(minRect, minVal);
        EditorGUI.MinMaxSlider(sliderRect, ref minVal, ref maxVal, attr.Min, attr.Max);
        maxVal = EditorGUI.FloatField(maxRect, maxVal);

        minProp.floatValue = minVal;
        maxProp.floatValue = maxVal;

        EditorGUI.EndProperty();
    }
}
```

## Related Skills
- `@unity-csharp-standards` - Script quality review
