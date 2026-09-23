---
name: unity-dotween-safety
description: "Use when writing DOTween code: SetLink on every tween, Sequence SetTarget, DOKill before reset, looping tween cleanup, tween in OnDestroy prevention, or DOTween leak warnings."
---

# DOTween Safety

## Overview
DOTween memory safety and lifecycle management. Prevent leaks, crashes, and "Some objects were not cleaned up" warnings by following strict linking, killing, and cleanup patterns.

## When to Use
- Use when creating any DOTween animation
- Use when reviewing scripts that use DOTween
- Use when investigating DOTween leak warnings
- Use when objects animate after being destroyed
- Use when scene transitions cause tween warnings

## The Golden Rules
- ✅ **Always** `.SetLink(gameObject)` on EVERY tween — primary defense (cost ≈ 0)
- ✅ `.Kill()` tweens in `OnDisable` — SetLink default only handles Destroy, not Disable
- ✅ `DOKill()` before starting a new tween on the same target — but NEVER on a
  button's own transform from its `onClick` (see below)
- ✅ `DOTween.Kill(target)` before manually resetting animated properties
- ✅ Await multi-stage tween sequences with `.ToUniTask()`
- ✅ **Always** `.SetTarget()` on `DOTween.Sequence()` — sequences do NOT inherit target
- 🔄 **REFACTOR**: `DOKill()` in `OnDestroy` → must be replaced with `.SetLink()` at tween creation
- ❌ **NEVER** start new tweens inside `OnDestroy` (DOTween singleton recreation leak)
- ❌ **NEVER** leave looping tweens (`.SetLoops(-1)`) without SetLink or explicit kill
- ❌ **NEVER** fire-and-forget tweens in async methods without awaiting

## `DOKill()` in an onClick handler freezes the button

`IPointerUpHandler` runs **before** `onClick` in the same frame. A press-animation
component (a custom animated button, a punch/jump-scale effect, …) has therefore
just started its release tween
on the button's own `RectTransform` when your handler runs. A blanket
`transform.DOKill()` there kills that tween at its **start value** — the button
stays visually pressed/squashed forever, and it looks like a UI bug rather than
a tween-ownership bug.

```csharp
// ❌ kills the press-animation component's release tween
public void OnClick() { transform.DOKill(); transform.DOScale(1.2f, 0.2f); }

// ✅ own only your tween
private Tween _tween;
public void OnClick()
{
    _tween?.Kill();
    _tween = transform.DOScale(1.2f, 0.2f).SetLink(gameObject);
}
```

Rule: `DOKill(target)` kills **every** tween on that target, including ones
another component owns. Kill your own tween reference (or a tween id), never
the transform, whenever a second component animates the same transform.

The same failure exists beyond buttons: whenever two tweens or animation phases
can be live on one target (an enter and an exit animation, an idle loop plus a
punch), killing leaves the value wherever the **other** tween left it, not
at the start value. Give each phase its own handle, and set the value you want
explicitly after a kill instead of assuming it.

## LinkBehaviour Options

`.SetLink(gameObject)` defaults to `KillOnDestroy`. Other options:

| LinkBehaviour | Behavior |
|---------------|----------|
| `KillOnDestroy` (default) | Kill when the GameObject is destroyed |
| `KillOnDisable` | Kill when the GameObject is disabled |
| `PauseOnDisable` | Pause when disabled |
| `PauseOnDisablePlayOnEnable` | Pause when disabled, play when enabled |
| `PauseOnDisableRestartOnEnable` | Pause when disabled, restart when enabled |
| `CompleteOnDisable` | Complete immediately when disabled |
| `RewindOnDisable` | Rewind to start when disabled |

```csharp
// UI panels that toggle with SetActive — kill on disable
canvasGroup.DOFade(1f, 0.3f)
    .SetLink(gameObject, LinkBehaviour.KillOnDisable);
```

## Identification Checklist
Flag any of the following as **high risk**:
1. **Property Tweens on Scalars**: `DOTween.To(() => myInt, ...)` — global virtual tween, MUST have SetLink
2. **Lack of `.SetLink()`**: On ANY tween, not just virtual tweens
3. **Tweens in Transitions**: Any tween that starts during scene/panel close
4. **Looping Tweens**: `.SetLoops(-1)` must ALWAYS have a link or explicit kill
5. **Tweens in OnDestroy**: Spawning animations during cleanup = recreation leak
6. **DOKill in OnDestroy**: Must refactor → add SetLink at tween creation, remove DOKill from OnDestroy
7. **Sequences without SetTarget**: `DOTween.Kill(target)` won't find orphaned Sequences

## Few-Shot Examples

### Example 1: SetLink for Virtual Tweens
**User**: "Animate a number counter."

**Agent**:
```csharp
// ❌ BEFORE: Global virtual tween — survives scene transition = LEAK
public async UniTask AnimateCounter(int target, float duration)
{
    await DOTween.To(() => _currentValue, x =>
    {
        _currentValue = x;
        _counterText.text = x.ToString("0");
    }, target, duration).SetEase(Ease.Linear);
}

// ✅ AFTER: Linked to gameObject — auto-killed on destroy
public async UniTask AnimateCounter(int target, float duration, CancellationToken ct)
{
    await DOTween.To(() => _currentValue, x =>
    {
        _currentValue = x;
        _counterText.text = x.ToString("0");
    }, target, duration)
        .SetEase(Ease.Linear)
        .SetLink(gameObject)         // Auto-kill when destroyed
        .ToUniTask(cancellationToken: ct);
}
```

### Example 2: Kill Before Reset (Flicker Prevention)
**User**: "Reset UI state before showing panel."

**Agent**:
```csharp
// ❌ BEFORE: Active tween fights with manual reset → flickering
private void ResetUI()
{
    _shadow.color = Color.clear;     // Tween immediately overrides this!
    _content.alpha = 0f;
}

// ✅ AFTER: Kill active tweens, then reset
private void ResetUI()
{
    DOTween.Kill(_shadow);           // Stop any active color tween
    DOTween.Kill(_content);          // Stop any active alpha tween

    _shadow.color = Color.clear;     // Now safe — no tween fighting
    _content.alpha = 0f;
}
```

### Example 3: Refactor DOKill-in-OnDestroy → SetLink
**User**: "Script has DOKill cleanup in OnDestroy."

**Agent**:
```csharp
// ❌ BEFORE: Manual DOKill in OnDestroy — redundant, fragile
private Tween _fadeTween;

private void ShowPanel()
{
    _fadeTween = canvasGroup.DOFade(1f, 0.3f); // No SetLink!
}

private void OnDestroy()
{
    _fadeTween?.Kill();  // Manual cleanup — must refactor
}

// ✅ AFTER: SetLink at creation — OnDestroy no longer needs DOKill
private void ShowPanel()
{
    canvasGroup.DOFade(1f, 0.3f)
        .SetLink(gameObject);  // Auto-killed on destroy
}
// OnDestroy: nothing needed — SetLink handles it
```

### Example 4: OnDestroy — No New Tweens
**User**: "Clean up loading animation when destroyed."

**Agent**:
```csharp
// ❌ BEFORE: OnDestroy triggers new tween → DOTween singleton recreation
private void OnDestroy()
{
    _loadingElement.HideLoading(); // Calls DOFade(0, 0.25f) internally!
    // DOTween manager may already be destroyed → creates new one → LEAK
}

// ✅ AFTER: Immediate state change — no new animations
private void OnDestroy()
{
    _loadingElement?.HideLoadingImmediate(); // SetActive(false), no animation
}
```

### Example 5: Awaiting Sequential Tweens
**User**: "Button shrinks, then bot appears, then player moves."

**Agent**:
```csharp
// ❌ BEFORE: Fire-and-forget — all happen simultaneously
public async void SubmitAsync()
{
    btnSubmit.DOScale(0, 0.25f);     // Not awaited!
    MakeBot();                        // Runs while button still shrinking
    await playerModel.DOMoveX(-0.35f, 0.35f);
}

// ✅ AFTER: Sequential with proper cancellation
public async UniTask SubmitAsync(CancellationToken ct)
{
    await btnSubmit.DOScale(0, 0.25f)
        .SetEase(Ease.InBack)
        .SetLink(btnSubmit.gameObject)
        .ToUniTask(cancellationToken: ct);

    MakeBot();

    await playerModel.DOMoveX(-0.35f, 0.35f)
        .SetLink(playerModel.gameObject)
        .ToUniTask(cancellationToken: ct);
}
```

### Example 6: SetTarget for Sequences
**User**: "Popup show animation with mask + content + scale."

**Agent**:
```csharp
// ❌ BEFORE: Sequence has no target — DOTween.Kill(gameObject) won't find it
var seq = DOTween.Sequence();
seq.Append(maskImage.DOFade(0.75f, 0.25f));
seq.Join(canvasGroup.DOFade(1f, 0.3f));
seq.Join(panel.DOScale(1f, 0.3f).SetEase(Ease.OutBack));

// ✅ AFTER: SetTarget + SetLink on Sequence
var seq = DOTween.Sequence();
seq.Append(maskImage.DOFade(0.75f, 0.25f));
seq.Join(canvasGroup.DOFade(1f, 0.3f));
seq.Join(panel.DOScale(1f, 0.3f).SetEase(Ease.OutBack));
seq.SetTarget(gameObject);      // DOTween.Kill(gameObject) finds this
seq.SetLink(gameObject);        // Auto-kill on destroy
```

### Example 7: Paused Game UI (SetUpdate)
**User**: "Pause menu fade-in while game is paused."

**Agent**:
```csharp
// ❌ BEFORE: Animation freezes — Time.timeScale = 0
Time.timeScale = 0f;
canvasGroup.DOFade(1f, 0.3f);   // Won't play!

// ✅ AFTER: SetUpdate(true) — time-independent
Time.timeScale = 0f;
canvasGroup.DOFade(1f, 0.3f)
    .SetUpdate(true)            // Uses unscaled time
    .SetLink(gameObject);
```

## Sequence vs Async Decision

| Use Case | Approach |
|----------|----------|
| Pure animation chain (no logic between) | `DOTween.Sequence()` |
| Logic between animation steps | `await tween.ToUniTask()` |
| Parallel animations, wait for all | `UniTask.WhenAll(a.ToUniTask(), b.ToUniTask())` |

## Performance Optimization

### Tween Recycling
```csharp
// Global: reduce GC allocations (~0.7KB per tween)
DOTween.Init(recycleAllByDefault: true);

// Per-tween
transform.DOMoveX(4, 1).SetRecyclable(true);
```
⚠️ With recycling, stored tween references may point to a DIFFERENT tween after kill. Clear in `OnKill`:
```csharp
_myTween = transform.DOScale(1.2f, 0.5f)
    .OnKill(() => _myTween = null);
```

### Cache & Reuse (SetAutoKill + Restart)
For frequently repeated animations:
```csharp
private Tween _pulseTween;

private void Awake()
{
    _pulseTween = transform.DOScale(1.1f, 0.3f)
        .SetLoops(-1, LoopType.Yoyo)
        .SetAutoKill(false)  // Don't destroy on complete
        .SetLink(gameObject)
        .Pause();
}

public void StartPulse() => _pulseTween.Restart();
public void StopPulse() => _pulseTween.Pause();
```

## Output Format
- **Leak risks**: Virtual tweens without SetLink
- **Crash risks**: Tweens in OnDestroy
- **Flicker risks**: Missing Kill before reset
- **Race condition risks**: Un-awaited sequential tweens
- **Refactor**: DOKill in OnDestroy → SetLink at creation

## Related Skills
- `@unity-async-patterns` - Async/await with CancellationToken
- `@unity-ui-performance` - UI performance and state safety
- `@unity-ui-motion-tuning` - Authoring and tuning the motion itself: scrubbing a sequence, live-tunable timings, DOTween from a utk exec snippet
