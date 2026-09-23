---
name: unity-panel-navigation
description: "Use when working on panel/popup navigation back-stacks, drag-to-close or swipe-to-close panels, Android back button handling, or bugs mentioning 'back does nothing', duplicate/stacked panels, or a panel that only closes on the second back press."
---

# Unity Panel Navigation & Back Stack

## Overview
Panels that participate in a navigation back-stack must pop their stack entry through **every** close path — button, drag/swipe, Android back, programmatic. The recurring bug class: one close path skips the pop, leaving a stale self-referential entry, so the next back press is a visible no-op ("back does nothing, second press works").

## When to Use
- Adding a new closeable panel to a nav-stack-managed UI.
- Implementing drag-to-close / swipe-to-close on a panel that also pushes a nav entry.
- Debugging back presses that do nothing, panels that stack duplicates, or close animations that skip side effects.

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Push/pop symmetry** | Every open that pushes a nav entry needs exactly one pop, regardless of HOW the panel closes. |
| **The drag-to-close trap** | Drag close calls the draggable component's `Close()` directly, bypassing the nav pop unless a before-close hook is wired. |
| **Stale self-entry** | A panel whose entry was never popped becomes its own back target — back "returns" to the current panel (no-op). |
| **Open-twice guard** | Entry points must check whether the same panel (for the same target/user) is already open before opening another instance. |

## Best Practices
- ✅ **Always** route ALL close paths through one handler that pops the nav entry (e.g. `NavigationManager.TryBack(this)`).
- ✅ Wire the draggable's `onBeforeClose` hook to that handler: return `false` when nav handled the close (suppress the component's own close), `true` to let the normal drag-close animation run.
- ✅ Mirror button-close side effects (pause video, restore HUD elements) in the drag-close fallback path.
- ✅ Guard entry points: re-opening the same panel for the same target must focus/refresh the existing instance, not stack a new one.
- ✅ Make "navigate to self" a no-op (e.g. tapping a profile link while already on that profile).
- ❌ **NEVER** add a new close gesture/path without checking what the button close does to the nav stack.

## Few-Shot Examples

### Example 1: Drag-to-close that pops the nav entry
**User**: "Fix the bug where after closing a panel by dragging it down, the first back press does nothing and only the second one works."

**Agent**:
```csharp
public class ItemDetailPanel : MonoBehaviour
{
    [SerializeField] private DraggableClosePanel _draggable;

    private void Awake()
    {
        // Drag close bypasses ClosePanel(), so the nav pop must happen here
        _draggable.onBeforeClose = HandleBeforeClose;
    }

    private bool HandleBeforeClose()
    {
        if (NavigationManager.TryBack(this))
            return false; // nav popped + closed us; suppress the component's own close
        return true;      // not in the stack; let normal drag-close animate
    }

    public void OnCloseButton() => NavigationManager.TryBack(this); // same single path
}
```

## Checklist for Any New Closeable Panel
- [ ] Open pushes a nav entry exactly once
- [ ] Button close pops via the shared handler
- [ ] Drag/swipe close wires `onBeforeClose` to the same handler
- [ ] Side effects of close run on every path
- [ ] Re-opening the same panel/target does not stack

## Related Skills
- `@unity-event-safety` - Hook subscription/unsubscription hygiene.
- `@unity-ugui-layout` - Panel canvas/layer setup.
- `@unity-popup-queue` - Popups the game shows on its own: queue turns, one-time popups, layering.
