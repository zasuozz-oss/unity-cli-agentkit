---
name: unity-event-safety
description: "Use when Unity C# code uses += / -= event subscriptions, ShowLoading/HideLoading pairing, boolean flags (_isLoading/_isFetching) that risk getting stuck, CancellationTokenSource acquire/dispose, or ghost handlers from missing OnDisable unsubscribe. NOT for Button.onClick (use persistent Inspector wiring)."
---

# Event Safety & Resource Pairing

## Overview
Prevents event memory leaks (ghost listeners) and stuck UI states (loading spinners, disabled buttons) by enforcing symmetric subscription patterns and the Resource Pairing Principle across all acquire/release pairs.

## Severity Reference

| Severity | Examples |
|----------|----------|
| 🔴 CRITICAL | Ghost listener causing crash, loading spinner permanently stuck, CTS leak |
| 🟡 HIGH | Memory leak accumulating per scene reload, UI stuck after error |

---

## §1 — Event Subscription Symmetry

Every `subscribe` MUST have a matching `unsubscribe` in the symmetric lifecycle method.

> ⚠️ **Scope boundary**: This rule applies to **C# events** (`+=` / `-=`) and non-Button UnityEvents only.
> `Button.onClick` must **never** use `AddListener` — persistent Inspector wiring (Mode: `Runtime Only`) is required instead.

- [ ] Every `OnEnable` subscribe → matching `OnDisable` unsubscribe
  - Grep: `grep -rn "OnEnable\|OnDisable" --include="*.cs"`
  - Cross-check: count `+=` in OnEnable vs `-=` in OnDisable
  - Severity: 🔴 CRITICAL
- [ ] Every `AddListener` on non-Button UnityEvents → matching `RemoveListener`
  - Grep: `grep -rn "AddListener\|RemoveListener" --include="*.cs"`
  - Cross-check: count per file — must be equal; flag any on `Button.onClick`
  - Severity: 🔴 CRITICAL
- [ ] No event subscriptions in `Awake`/`Start` without unsubscription in `OnDisable`/`OnDestroy`
- [ ] Static event subscriptions are especially dangerous — always verify unsubscribe path
  - Grep: `grep -rn "static.*event\|static.*Action\|static.*Event" --include="*.cs"`
  - Severity: 🔴 CRITICAL
- [ ] Delegate chains (`+=`) never accumulate duplicates across re-enable cycles

```csharp
// ❌ BAD: Subscribe in Start, never unsubscribe
void Start() { PlayerHealth.OnDamaged += UpdateHealthBar; }

// ✅ GOOD: Symmetric OnEnable/OnDisable — C# events only
private void OnEnable()  { PlayerHealth.OnDamaged += UpdateHealthBar; }
private void OnDisable() { PlayerHealth.OnDamaged -= UpdateHealthBar; }

// ❌ NEVER do this for Button.onClick — use persistent Inspector wiring instead
private void OnEnable()  { _button.onClick.AddListener(OnClicked); }
private void OnDisable() { _button.onClick.RemoveListener(OnClicked); }
```

---

## §2 — Resource Pairing Principle

Every "acquire" operation MUST have a matching "release" on ALL exit paths (success, failure, cancellation, early return).

### ShowLoading / HideLoading

- [ ] `ShowLoading()` ↔ `HideLoading()` — verified in success, failure, cancellation, and early return paths
  - Grep: `grep -rn "ShowLoading\|HideLoading" --include="*.cs"`
  - Cross-check: count Show vs Hide per method
  - Severity: 🔴 CRITICAL
- [ ] Loading spinners never stuck — check every `return` statement in async methods for `HideLoading()` call

```csharp
// ❌ BAD: HideLoading only on success path
loadingElement.ShowLoading();
var data = await LoadData();
loadingElement.HideLoading(); // Never reached if exception!

// ✅ GOOD: finally guarantees cleanup on ALL paths
loadingElement.ShowLoading();
try
{
    var data = await LoadData().AttachExternalCancellation(ct);
    ProcessData(data);
}
catch (OperationCanceledException) { }
catch (Exception e) { Debug.LogError($"Failed: {e.Message}"); }
finally
{
    loadingElement.HideLoading(); // GUARANTEED on all paths
}
```

### CancellationTokenSource

- [ ] `CancellationTokenSource` created → `Cancel()` + `Dispose()` called before recreation
  - Grep: `grep -rn "new CancellationTokenSource" --include="*.cs"`
  - Cross-check: verify `Cancel + Dispose` before new instance
  - Severity: 🔴 CRITICAL
  - Full pattern & examples → see `@unity-async-patterns` §CTS Management

### Other Pairings

- [ ] `Subscribe` ↔ `Unsubscribe` — symmetric in enable/disable
- [ ] `DOTween.Play()` ↔ `Kill()` — linked or explicitly killed (see unity-dotween-safety)
- [ ] File/Stream `Open` ↔ `Close` — wrapped in `using` or `finally`
  - Grep: `grep -rn "File\.Open\|StreamReader\|StreamWriter" --include="*.cs"`
  - Cross-check: verify `using` or `finally` block
  - Severity: 🟡 HIGH
- [ ] `SetInteractable(false)` ↔ `SetInteractable(true)` on all paths

---

## §3 — State Flag Safety

Boolean flags (`_isLoading`, `_isFetching`) must be reset on ALL exit paths. A stuck flag permanently locks out functionality until app restart.

- [ ] Every flag set `true` has a matching `false` in `finally`
  - Grep: `grep -rn "_isLoading\|_isFetching\|_isProcessing" --include="*.cs"`
  - Severity: 🔴 CRITICAL
- [ ] No "Early Gatekeeper Bug": a caller must NOT set `_isFetching = true` before calling a method that also sets it — causes permanent lock-out
- [ ] `forceRefresh` parameters bypass "Already Active" guards but respect cooldowns

```csharp
// ❌ BAD: flag stuck if exception
_isFetching = true;
var data = await LoadData(ct);
_isFetching = false; // Never reached on exception!

// ✅ GOOD: finally resets on ALL paths
_isFetching = true;
try
{
    var data = await LoadData(ct);
    ProcessData(data);
}
catch (OperationCanceledException) { }
catch (Exception e) { Debug.LogError($"Failed: {e.Message}"); }
finally { _isFetching = false; }
```

---

## Verification Rules

### Event subscription symmetry
- **Grep:** `grep -rn "AddListener\|RemoveListener\|\+=\|\-=" --include="*.cs"`
- **Verify:**
  1. Count `AddListener` vs `RemoveListener` in same file — must be equal → **PASS**
  2. Check subscribe location (OnEnable) has matching unsubscribe (OnDisable) → **PASS**
  3. `Awake`/`Start` subscribe without `OnDestroy` unsubscribe? → **🔴 CRITICAL**
  4. Static event subscribe without unsubscribe? → **🔴 CRITICAL**
  5. Lambda subscribe `+=` without corresponding `-=`? → **🔴 CRITICAL**
  6. `AddListener` / `RemoveListener` on `Button.onClick`? → **🔴 CRITICAL** — must use persistent Inspector wiring

### ShowLoading / HideLoading pairing
- **Grep:** `grep -rn "ShowLoading\|HideLoading" --include="*.cs"`
- **Verify:**
  1. Every `ShowLoading` has matching `HideLoading` in try-catch-finally? → **PASS**
  2. Early return after ShowLoading without HideLoading? → **🔴 CRITICAL**
  3. HideLoading only in success path, not in catch/finally? → **🔴 CRITICAL**

### CancellationTokenSource management
- **Grep:** `grep -rn "new CancellationTokenSource" --include="*.cs"`
- **Verify:**
  1. Previous CTS cancelled + disposed before new? `_cts?.Cancel(); _cts?.Dispose();` → **PASS**
  2. No Cancel/Dispose before reassignment? → **🔴 CRITICAL** — CTS leak

---

## Related Skills
- `@unity-async-patterns` — CancellationToken lifecycle, async guards
- `@unity-dotween-safety` — DOTween kill and SetLink patterns
- `@unity-ui-performance` — UI state safety and animation races
- `@unity-csharp-standards` — Crash prevention, singleton guards, ANR prevention
