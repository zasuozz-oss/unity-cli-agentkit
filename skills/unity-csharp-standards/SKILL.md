---
name: unity-csharp-standards
description: "Use when writing, generating, or reviewing any Unity C# script as the baseline standard for naming rules, field conventions, formatting, Update loop performance, GC allocation reduction, object pooling, and mobile frame budgets."
---

# C# Quality (Unity 6)

BASELINE skill — applies to ALL Unity C# code.

---

## §1 — Naming & Casing

| Element | Convention | Example |
|---------|------------|---------|
| Class / Struct / Enum | PascalCase | `HealthManager` |
| Interface | IPascalCase | `IDamageable` |
| Method | PascalCase (verb) | `TakeDamage()` |
| Property | PascalCase | `CurrentHealth` |
| Private field | _camelCase | `_isDead` |
| Local / Parameter | camelCase | `damageAmount` |
| Constant | PascalCase | `MaxRetries` |
| Boolean | is/has/can/should prefix | `_isVisible`, `HasChanges` |
| Async method | suffix Async | `LoadDataAsync()` |
| Event | past/present participle | `event Action SettingsApplied` |
| Event raiser | prefix On | `OnSettingsApplied()` |
| Event handler | prefix Handle | `HandleApplyClicked()` |
| Collection | plural noun | `_inventoryItems` |

---

## §2 — Field & Serialization Rules

```csharp
// ✅ Correct field conventions
[Header("Configuration")]
[SerializeField, Min(1)] private float _maxHealth = 100f;
[SerializeField, Tooltip("Damage VFX")] private GameObject _damageVfx;

private bool _isDead;
public float CurrentHealth => _currentHealth;  // Read-only property
public event Action<float> OnHealthChanged;     // Action<T> preferred
```

- Prefer `[SerializeField] private` over public fields
- Use `[Header]`, `[Tooltip]`, `[Range]`, `[Min]` for Inspector UX
- Group related data with `[Serializable]` structs
- No Hungarian notation

### Button.onClick — Code Prohibition
- **NEVER** use `Button.onClick.AddListener()` — wire as persistent UnityEvent in Inspector (Mode: `Runtime Only`, target the handler's GameObject)
- Applies to all buttons, including dynamic/pooled — never fall back to `AddListener`

---

## §3 — Formatting

- Brace style: **Allman** — braces MUST NOT be omitted (even single-line)
- Indentation: 4 spaces
- Max line length: 80–120 chars
- Comments: explain WHY, not WHAT — no commented-out code

---

## §4 — Performance Red Flags

### Update Loop — NEVER do these per-frame
- ❌ `Camera.main` (calls FindGameObjectWithTag internally — cache it)
- ❌ `GetComponent<T>()` (cache in Awake/Start)
- ❌ `FindObjectOfType` at runtime
- ❌ LINQ in hot paths (`.Where`, `.Select`, `.Any`)
- ❌ String concatenation (`+`, `$""`)
- ❌ `new List<T>()` / closures / lambdas
- ❌ `new T[]` per-frame — pre-allocate buffers

### ALWAYS do these
- ✅ `CompareTag()` instead of `tag == "string"`
- ✅ `sqrMagnitude` instead of `Vector3.Distance`
- ✅ `Animator.StringToHash` for parameter lookups
- ✅ Non-allocating Physics: `RaycastNonAlloc`, `OverlapSphereNonAlloc`
- ✅ `for` instead of `foreach` on non-List collections
- ✅ Cache delegates/lambdas as fields
- ✅ Replace per-object `Update()` with `UpdateManager` for 10+ entities

### Mobile Budget

| Metric | Budget |
|--------|:------:|
| Frame Time | < 33ms (30fps) |
| Draw Calls | < 100 |
| Triangles | < 200K |
| Texture Memory | < 150MB |

- ❌ **NEVER** real-time shadows on mobile — bake them
- ❌ **NEVER** `Debug.Log` in release — use `[Conditional("UNITY_EDITOR")]` wrapper
- ❌ **NEVER** reflection in hot paths
- ✅ Single camera on mobile — each camera = 1 full render pass
- ✅ Avoid Animators in UI — use DOTween instead (see `@unity-dotween-safety`)

---

## §5 — Object Pooling

Apply when objects spawn > 5 times/minute. Pre-warm during loading, not gameplay.

- [ ] `SetActive(false)` to return — never `Destroy()`
- [ ] Reset ALL state before reuse (position, scale, refs, timers, flags)
- [ ] Set max pool size — prevent unbounded growth
- [ ] Use `IPoolable` interface (`OnSpawn`/`OnDespawn`)

---

## §6 — Crash & Stability

### Singleton Guard
```csharp
private static GameManager _instance;
private void Awake()
{
    if (_instance != null) { Destroy(gameObject); return; }
    _instance = this;
    DontDestroyOnLoad(gameObject);
}
private void OnDestroy() { if (_instance == this) _instance = null; }
```

### Critical Rules
- [ ] Never sync IO on main thread (`File.ReadAllText`, `PlayerPrefs.Save`) → 🔴 ANR
- [ ] Never busy-wait: `while (!www.isDone) {}` → 🔴 ANR
- [ ] `Destroy(texture)` on runtime `new Texture2D` → 🔴 GPU leak
- [ ] Static collections with Unity refs — clean on scene unload → 🟡 leak
- [ ] No `Instantiate`/`AddComponent` in `OnDestroy` → 🟡 crash
- [ ] Save in `OnApplicationPause(true)` — `OnApplicationQuit` may not fire on mobile
- [ ] `CancellationToken` for threads — never `Thread.Abort()`
- [ ] No hardcoded test IDs/URLs in production → 🔴 data safety
- [ ] No `GrabPass` on mobile → 🔴 copies framebuffer

---

## §7 — Script Design

### Role Assignment (before generating scripts)

| Role | When to Use |
|------|-------------|
| **MonoBehaviour** | Needs Transform, collisions, or Unity lifecycle |
| **ScriptableObject** | Authored data, shared between instances |
| **Pure C# service** | Stateless logic, testable without Unity |
| **Presenter** | Bridges domain logic to UI or visuals |

### Review Checklist
1. **Single Responsibility** — one reason to change?
2. **Coupling** — dependencies injected or hard-wired?
3. **Lifecycle** — event subscriptions symmetric (OnEnable/OnDisable)?
4. **Inspector UX** — fields organized with `[Header]`, `[Tooltip]`?

---

## Example: Correct Script Structure

```csharp
public class HealthManager : MonoBehaviour
{
    [Header("Configuration")]
    [SerializeField, Min(1)] private float _maxHealth = 100f;

    private float _currentHealth;
    private bool _isDead;

    public float CurrentHealth => _currentHealth;
    public bool IsDead => _isDead;
    public event Action<float, float> OnHealthChanged;

    public void TakeDamage(float amount)
    {
        if (_isDead) return;

        _currentHealth = Mathf.Max(0f, _currentHealth - amount);
        OnHealthChanged?.Invoke(_currentHealth, _maxHealth);

        if (_currentHealth <= 0f)
        {
            _isDead = true;
            HandleDeath();
        }
    }

    private void HandleDeath() { /* death logic */ }
}
```

---

## Related Skills
- `@unity-event-safety` — Event subscription symmetry, state flag safety
- `@unity-async-patterns` — Async/await lifecycle and cancellation
- `@unity-dotween-safety` — DOTween lifecycle and memory patterns
- `@unity-addressables` — Asset management and memory-safe release
- `@unity-ui-performance` — UI canvas performance and state safety
