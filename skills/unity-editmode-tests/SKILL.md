---
name: unity-editmode-tests
description: "Use when writing or fixing Unity EditMode/NUnit tests, when a test assembly throws CS0246/CS0234 ('type or namespace not found') for game types in Assembly-CSharp, or when deciding whether game logic can be unit-tested at all."
---

# Unity EditMode Tests & the Assembly-CSharp Boundary

## Overview
When game code lives in the predefined `Assembly-CSharp` (no asmdef) and tests live in an asmdef-based assembly, the tests **can never reference game types directly**: Unity compiles asmdef assemblies BEFORE Assembly-CSharp, and listing "Assembly-CSharp" in asmdef references is silently ignored → CS0246/CS0234. This breaks test files after every class move/rename.

## When to Use
- A test assembly fails with CS0246/CS0234 for a game type.
- Writing new EditMode tests for logic that lives in Assembly-CSharp.
- After moving/renaming game classes that have reflection-based tests.
- Deciding whether a guard/interface can be unit-tested or must be verified manually.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Compilation order** | asmdef assemblies compile before predefined Assembly-CSharp; the reference direction cannot be reversed. |
| **Reflection bridge** | `Type.GetType("Ns.Type, Assembly-CSharp")` + `MethodInfo.Invoke` is the only direct access path from test asmdefs. |
| **Pure static helpers** | Extract testable logic into pure static methods (no Unity object deps) in the game class, then invoke via reflection. |
| **Interface hard limit** | Interfaces defined in Assembly-CSharp cannot be implemented/faked from a test asmdef at all — verify such guards by compile + manual STR instead. |

## Best Practices
- ✅ Extract decision logic into pure static helpers before trying to test it; don't instantiate MonoBehaviours from tests.
- ✅ Keep reflection type strings greppable: after renaming/moving a game class, grep the test folders for the old `"Namespace.Type"` string — reflection does NOT refactor automatically.
- ✅ Run tests through the project's editor CLI (e.g. `utk test`, `utk test --filter Ns.Class.Test`) after a compile-check.
- ❌ **NEVER** add "Assembly-CSharp" to an asmdef's references expecting it to work — the failure is silent until CS0246.
- ❌ **NEVER** burn time trying to fake an Assembly-CSharp interface from a test asmdef — it is structurally impossible.

## Few-Shot Examples

### Example 1: Testing Assembly-CSharp logic via reflection
**User**: "the EditMode test reports CS0246: the logic class is not found after renaming the namespace."

**Agent**:
```csharp
[Test]
public void Decide_ReturnsExpected_ForKnownInput()
{
    // Reflection bridge: asmdef tests cannot reference Assembly-CSharp directly.
    // NOTE: this string breaks on rename/move — grep tests for the old name.
    var type = Type.GetType("MyGame.Runtime.SomeLogic, Assembly-CSharp");
    Assert.NotNull(type, "Type moved/renamed? Update the reflection string.");

    var decide = type.GetMethod("Decide", BindingFlags.Public | BindingFlags.Static);
    var result = (bool)decide.Invoke(null, new object[] { 3 });

    Assert.IsTrue(result);
}
```

## Related Skills
- `@unity-csharp-standards` - Keeping logic extractable (pure helpers).
- `@unity-qa-generator` - Generating contract/verification tests.
