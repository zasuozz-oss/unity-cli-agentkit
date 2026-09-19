---
name: unity-async-patterns
description: "Use when writing Unity async code: UniTask, CancellationToken, destroyCancellationToken, coroutine lifecycle, 'this == null' guard after await, async void prohibition, or CancellationTokenSource Cancel/Dispose/Recreate pattern."
---

# Asynchronous Programming

## Overview
Asynchronous programming in Unity using Coroutines, async/await with Task, and UniTask. Covers loading, networking, sequential operations, cancellation, and error handling.

## Async Options Comparison

| Feature | Coroutine | Task | UniTask |
|---------|:---------:|:----:|:-------:|
| Cancellation | StopCoroutine | CancellationToken | CancellationToken |
| Return Value | ❌ | ✅ | ✅ |
| Error Handling | ❌ | try/catch | try/catch |
| Thread-safe | Main only | Any thread | Main (default) |
| GC Allocation | Some | Heavy | Zero |
| Recommendation | Simple delays | Server/PC | Production |

## Best Practices
- ✅ **Always** use CancellationToken with async/UniTask
- ✅ Use `destroyCancellationToken` for MonoBehaviour-bound tasks
- ✅ Use `UniTask.WhenAll` for parallel operations
- ✅ Guard against destroyed objects after await
- ✅ Use `async UniTaskVoid` for fire-and-forget (not `async void`)
- ❌ **NEVER** use `async void` (silent exception swallowing)
- ❌ **NEVER** await without CancellationToken in MonoBehaviour
- ❌ **NEVER** access `this` after await without null guard

## Few-Shot Examples

### Example 1: Sequential Loading with UniTask
**User**: "Load player data, then inventory, then start game."

**Agent**:
```csharp
public class GameLoader : MonoBehaviour
{
    [SerializeField] private Slider _progressBar;

    private async UniTaskVoid Start()
    {
        var ct = destroyCancellationToken;

        try
        {
            _progressBar.value = 0f;

            var playerData = await LoadPlayerDataAsync(ct);
            _progressBar.value = 0.33f;

            var inventory = await LoadInventoryAsync(playerData.Id, ct);
            _progressBar.value = 0.66f;

            var world = await LoadWorldAsync(playerData.LastScene, ct);
            _progressBar.value = 1f;

            InitializeGame(playerData, inventory, world);
        }
        catch (OperationCanceledException)
        {
            Debug.Log("Loading cancelled (scene unloaded)");
        }
        catch (Exception e)
        {
            Debug.LogError($"Loading failed: {e.Message}");
            ShowRetryDialog();
        }
    }

    private async UniTask<PlayerData> LoadPlayerDataAsync(CancellationToken ct)
    {
        var json = await File.ReadAllTextAsync(GetSavePath(), ct);
        return JsonUtility.FromJson<PlayerData>(json);
    }
}
```

### Example 2: Parallel Loading
**User**: "Load textures, audio, and config at the same time."

**Agent**:
```csharp
public class AssetPreloader : MonoBehaviour
{
    public async UniTask PreloadAsync(CancellationToken ct)
    {
        // All three load simultaneously
        var (textures, audioClips, config) = await UniTask.WhenAll(
            LoadTexturesAsync(ct),
            LoadAudioAsync(ct),
            LoadConfigAsync(ct)
        );

        Debug.Log($"Loaded {textures.Length} textures, {audioClips.Length} clips");
        ApplyConfig(config);
    }

    private async UniTask<Texture2D[]> LoadTexturesAsync(CancellationToken ct)
    {
        var handles = new List<UniTask<Texture2D>>();
        foreach (var key in _textureKeys)
        {
            handles.Add(Addressables.LoadAssetAsync<Texture2D>(key)
                .ToUniTask(cancellationToken: ct));
        }
        return await UniTask.WhenAll(handles);
    }
}
```

### Example 3: Timeout and Retry Pattern
**User**: "API call with timeout and retry."

**Agent**:
```csharp
public class ApiClient
{
    private const int MaxRetries = 3;
    private const float TimeoutSeconds = 10f;

    public async UniTask<T> RequestWithRetry<T>(string url, CancellationToken ct)
    {
        for (int attempt = 0; attempt < MaxRetries; attempt++)
        {
            try
            {
                return await RequestWithTimeout<T>(url, TimeoutSeconds, ct);
            }
            catch (TimeoutException)
            {
                Debug.LogWarning($"Attempt {attempt + 1}/{MaxRetries} timed out");
                if (attempt == MaxRetries - 1) throw;
                await UniTask.Delay(TimeSpan.FromSeconds(Math.Pow(2, attempt)), cancellationToken: ct);
            }
        }
        throw new InvalidOperationException("Should not reach here");
    }

    private async UniTask<T> RequestWithTimeout<T>(string url, float timeout, CancellationToken ct)
    {
        using var timeoutCts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        timeoutCts.CancelAfter(TimeSpan.FromSeconds(timeout));

        using var request = UnityWebRequest.Get(url);
        await request.SendWebRequest().ToUniTask(cancellationToken: timeoutCts.Token);

        if (request.result != UnityWebRequest.Result.Success)
            throw new Exception(request.error);

        return JsonUtility.FromJson<T>(request.downloadHandler.text);
    }
}
```

## Lifecycle Safety Guards

### Null Check After Await
Every `await` in a MonoBehaviour may resume after the object is destroyed.
```csharp
// ❌ BAD: Accessing this after await without guard
var data = await APIManager.GetData();
UpdateUI(data); // MissingReferenceException if destroyed!

// ✅ GOOD: Guard after every await
var data = await APIManager.GetData();
if (this == null) return;  // Survival check
UpdateUI(data);
```

### Coroutine After Await
```csharp
// ❌ BAD: StartCoroutine on inactive/destroyed object
await LongTask();
StartCoroutine(AnimateDots()); // Crash if destroyed or inactive!

// ✅ GOOD: Guard both existence and active state
await LongTask();
if (this == null || !gameObject.activeInHierarchy) return;
_animateRoutine = StartCoroutine(AnimateDots());
```

### Stored Coroutine Reference
```csharp
// ❌ BAD: StopCoroutine(Method()) creates new instance
StopCoroutine(AnimateDots()); // Does NOT stop the running one!

// ✅ GOOD: Store and stop the exact reference
private Coroutine _routine;
_routine = StartCoroutine(AnimateDots());
// Later:
if (_routine != null) { StopCoroutine(_routine); _routine = null; }
```

## CancellationTokenSource Management

Always follow the **Cancel → Dispose → Recreate** pattern:
```csharp
private CancellationTokenSource _cts;

public void StartLoading()
{
    // 1. Cancel and dispose previous
    _cts?.Cancel();
    _cts?.Dispose();
    _cts = new CancellationTokenSource();

    // 2. Pass token to async method
    LoadData(_cts.Token).Forget();
}

private async UniTask LoadData(CancellationToken ct)
{
    await TaskA();
    if (ct.IsCancellationRequested || this == null) return;

    await UniTask.Delay(300, cancellationToken: ct);
    if (ct.IsCancellationRequested || this == null) return;

    ProcessData();
}

private void OnDestroy()
{
    _cts?.Cancel();
    _cts?.Dispose();
    _cts = null;
}
```

### The stale-writer race: capture your own CTS locally

In a cancel-and-replace pattern, an async continuation that re-reads the
**field** after an await reads its *successor's* token, not its own. The
replaced call therefore sees a fresh, uncancelled token, sails through every
`ThrowIfCancellationRequested()`, and writes its stale result — possibly
overwriting the correct result of the newer call.

This surfaces the moment someone moves an apply-step after a long await (e.g.
"mask before show" reordering): spam-switching items then renders the new
texture at the **old** item's coordinates, because the stale call's write won
the race.

```csharp
public async UniTask LoadItem(ItemData data)
{
    _cts?.Cancel();
    _cts?.Dispose();
    _cts = new CancellationTokenSource();
    var cts = _cts;                      // ← capture MY cts, once

    try
    {
        var bytes = await LoadBinaryData(cts.Token);
        cts.Token.ThrowIfCancellationRequested();   // ✅ my token
        // ❌ _cts.Token.ThrowIfCancellationRequested() — the successor's token

        await CheckHighestMask(cts);
        cts.Token.ThrowIfCancellationRequested();
        SetSprites(data);                // final write, guarded by MY token
    }
    catch (OperationCanceledException) { }
}
```

A `data != _cache` guard placed *before* the awaits does not protect writes
that happen *after* them. Guard the write, not the entry.

### Sharing one in-flight async result (single-flight)

`UniTask.Preserve()` (MemoizeSource) only allows re-awaiting **after** the task
completes — while still in flight it accepts exactly ONE continuation. A boot
prefetch kicked with `.Forget()` plus a later `await` on the cached task throws
`InvalidOperationException: Already continuation registered, can not await
twice`.

- ❌ Cache a `Preserve()`'d `UniTask` and await it from several places.
- ✅ Cache a `UniTaskCompletionSource<T>` and hand out its `.Task` — genuinely
  multi-awaiter safe. Complete it from a wrapper method that runs the work once.

```csharp
private UniTaskCompletionSource<Config> _configTcs;

public UniTask<Config> GetConfigAsync()
{
    if (_configTcs != null) return _configTcs.Task;   // every caller awaits safely
    _configTcs = new UniTaskCompletionSource<Config>();
    FetchAsync().Forget();
    return _configTcs.Task;
}

private async UniTaskVoid FetchAsync()
{
    try { _configTcs.TrySetResult(await Api.LoadConfig()); }
    catch (Exception e) { _configTcs.TrySetException(e); _configTcs = null; } // allow retry
}
```

Note the `= null` on failure: see `@unity-startup-loading` for why a cached
prefetch must never serve a stale failure.

## Hybrid Safety Pattern (Recycled Views)

For pooled objects or OSA adapters where a MonoBehaviour survives but gets reassigned to different data:
```csharp
private CancellationTokenSource _cts;

public async void InitItem(ItemData itemData)
{
    // 1. Cancel previous
    _cts?.Cancel();
    _cts?.Dispose();
    _cts = new CancellationTokenSource();
    var token = _cts.Token;

    this.data = itemData;

    var sprite = await LoadSpriteAsync(itemData.path);

    // 2. Triple guard
    if (this == null) return;               // Destroyed?
    if (token.IsCancellationRequested) return; // Cancelled?
    if (this.data != itemData) return;      // Recycled?

    thumbImage.sprite = sprite;
}
```

## Error Handling

### Try-Finally for UI Cleanup

> Full ShowLoading/HideLoading pattern → see `@unity-event-safety` §2 Resource Pairing Principle.

Key rule: `ShowLoading()` must ALWAYS have `HideLoading()` in a `finally` block — applies on success, failure, cancellation, and early return.

### Fire-and-Forget Safety
```csharp
// ❌ BAD: Silent failure
_ = SomeAsyncOperation();

// ✅ GOOD: Conscious fire-and-forget
SomeAsyncOperation().Forget();
```

## Editor Play Mode Safety
```csharp
public async UniTask InitSDK(CancellationToken ct)
{
    #if UNITY_EDITOR
    if (!Application.isPlaying) return;
    #endif

    await PrepareConfig();

    #if UNITY_EDITOR
    if (!Application.isPlaying || ct.IsCancellationRequested) return;
    #endif

    ExternalSDK.Initialize(config); // May call DontDestroyOnLoad internally
}
```

## Cancellation Patterns

| Pattern | When |
|---------|------|
| `destroyCancellationToken` | MonoBehaviour lifetime |
| `CancellationTokenSource` | Manual control (Cancel → Dispose → Recreate) |
| `CreateLinkedTokenSource` | Combine multiple tokens |
| `CancelAfter(timeout)` | Timeout |

## Choosing the Right Async Model

| Need | Tool | Why |
|------|------|-----|
| Simple delay (one-off) | Coroutine / UniTask.Delay | Lightweight, lifecycle-managed |
| Sequential async chain | UniTask / async-await | Readable, cancellable |
| Periodic polling | Update + timer | No coroutine overhead |
| React to state changes | C# events / UnityEvent | No polling, zero-cost when idle |
| Frame-aligned updates | Update / LateUpdate | Deterministic, profiler-visible |

### Anti-Patterns

| Anti-Pattern | Risk |
|-------------|------|
| Coroutine started in Update | ⚠️ Leak |
| UniTask without CancellationToken | ⚠️ Ghost task |
| WaitForSeconds in tight loop | ⚠️ GC |
| async void (not UniTaskVoid) | 🔴 Silent failure |

## Related Skills
- `@unity-addressables` - Async asset loading
- `@unity-dotween-safety` - DOTween async patterns
- `@unity-ui-performance` - UI cleanup and state consistency
