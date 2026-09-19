---
name: unity-addressables
description: "Use when loading Unity assets at runtime: Addressables.LoadAssetAsync, AssetReference, AsyncOperationHandle release, InstantiateAsync, preload by label, migrating from Resources.Load, or fixing first-instantiate lag/hitch/spike from addressable content."
---

# Addressables Asset Management

## Overview
Unity Addressables for asynchronous, memory-safe asset loading. Replace direct references and Resources.Load with addressable keys and AssetReferences for scalable content management.

## When to Use
- Use when loading assets at runtime
- Use when replacing Resources.Load
- Use when managing remote/downloadable content
- Use when optimizing memory with load/release patterns
- Use when sharing assets across scenes

## Key Concepts

| Concept | Description |
|---------|-------------|
| **AssetReference** | Inspector-assignable addressable reference |
| **Address/Label** | String key or tag for loading |
| **Handle** | AsyncOperationHandle — tracks load state, MUST be released |
| **Release** | Free memory when done |
| **Catalog** | Index of all addressable assets |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   ADDRESSABLE FLOW                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  AssetReference               Handle                        │
│  (Inspector)    →  LoadAsync  →  Use Asset  →  Release      │
│                                                             │
│  ⚠️ EVERY LoadAsync MUST have a matching Release            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Best Practices
- ✅ **Always** release handles when done (`Addressables.Release(handle)`)
- ✅ Use `AssetReference` in Inspector for type safety
- ✅ Use labels for batch operations (preload by label)
- ✅ Track active handles for cleanup
- ✅ Use `WaitForCompletion()` only in synchronous contexts
- ❌ **NEVER** forget to release loaded assets (memory leak)
- ❌ **NEVER** release an already-released handle
- ❌ **NEVER** use `Resources.Load` for addressable assets

## First-Instantiate Hitch (cold-load lag)

`InstantiateAsync` is only asynchronous for the *loading*; the instantiate step itself is synchronous (official docs: "the instantiation itself is synchronous. The asynchronous aspect of this API comes from all the loading-related activity"). The first call pays everything at once: bundle IO + decompression, asset deserialization, texture/mesh GPU upload, shader/PSO compilation, then `Awake/Start`. Fix in this order:

1. **Preload behind a loading screen** — `LoadAssetAsync` / `LoadAssetsAsync(label)` first. Docs: "If the GameObject has been preloaded using LoadAssetAsync or LoadAssetsAsync the operation and instantiation becomes synchronous" — the later spawn is cheap.
2. **Cache ONE load handle + manual `Object.Instantiate(handle.Result)`** — official docs note `InstantiateAsync` "incurs overhead that can be mitigated by handling instantiation manually"; keep the load handle alive, `Instantiate` as many copies as needed, `Destroy` instances yourself, release the single load handle when the last instance is gone.
3. **Pool instances** — `UnityEngine.Pool.ObjectPool<GameObject>` over instantiate/destroy churn for repeated spawns.
4. **Prewarm shaders/PSOs** — usually the biggest first-time spike on mobile. Put a `ShaderVariantCollection` in Graphics Settings → Preloaded Shaders (auto-warmed at startup) or call `ShaderVariantCollection.WarmUp()` on the loading screen; verify via profiler markers `Shader.CreateGPUProgram` / `CreateGraphicsPipelineImpl`. Unity 6 + Metal/Vulkan/DX12: prefer `GraphicsStateCollection` (BeginTrace/EndTrace on device → `WarmUpProgressively`) — plain WarmUp can miss PSOs when vertex layout/render target differ from the warm-up draw.
5. **Tune the Async Upload Pipeline while loading** — texture/mesh binary data streams to GPU time-sliced per frame; raise `QualitySettings.asyncUploadTimeSlice` (range 1–33 ms) and `asyncUploadBufferSize` while a loading screen hides the cost, restore for gameplay.
6. **Bundle compression** — local groups = **LZ4** (chunk-based, loads parts without decompressing the whole bundle); **LZMA only for remote** (smallest download; the cache recompresses to LZ4 on device). LZMA on a *local* group = full-bundle decompress on first load = exactly this lag.
7. **Pay init cost up front** — `Addressables.InitializeAsync()` at boot so catalog/runtime init doesn't land inside the first spawn.

Anti-patterns:
- ❌ `WaitForCompletion()` on a cold (not preloaded) asset — it completes **all** in-flight loads synchronously; never on remote bundles, never in `Awake`, never on WebGL, and never in the same call stack after `SceneManager.LoadScene` (flushes the engine preload queue mid scene-switch — project-proven NRE source).
- ❌ Profiling the hitch in Editor "Use Asset Database" play mode — no bundle IO/decompression happens there; measure on a device build.

## Few-Shot Examples

### Example 1: Asset Loading Service
**User**: "Load and manage prefabs with Addressables."

**Agent**:
```csharp
public class AssetLoadingService : MonoBehaviour
{
    private readonly Dictionary<string, AsyncOperationHandle> _loadedAssets = new();

    public async UniTask<T> LoadAssetAsync<T>(string key, CancellationToken ct) where T : Object
    {
        if (_loadedAssets.TryGetValue(key, out var existing))
            return (T)existing.Result;

        var handle = Addressables.LoadAssetAsync<T>(key);
        await handle.ToUniTask(cancellationToken: ct);

        if (handle.Status == AsyncOperationStatus.Succeeded)
        {
            _loadedAssets[key] = handle;
            return handle.Result;
        }

        Debug.LogError($"Failed to load asset: {key}");
        Addressables.Release(handle);
        return null;
    }

    public void ReleaseAsset(string key)
    {
        if (_loadedAssets.TryGetValue(key, out var handle))
        {
            Addressables.Release(handle);
            _loadedAssets.Remove(key);
        }
    }

    private void OnDestroy()
    {
        foreach (var handle in _loadedAssets.Values)
            Addressables.Release(handle);
        _loadedAssets.Clear();
    }
}
```

### Example 2: Prefab Spawning with AssetReference
**User**: "Spawn enemies from AssetReference."

**Agent**:
```csharp
public class EnemySpawner : MonoBehaviour
{
    [SerializeField] private AssetReferenceGameObject _enemyPrefab;

    private readonly List<AsyncOperationHandle<GameObject>> _spawnedHandles = new();

    public async UniTask<GameObject> SpawnAsync(Vector3 position, CancellationToken ct)
    {
        var handle = _enemyPrefab.InstantiateAsync(position, Quaternion.identity);
        await handle.ToUniTask(cancellationToken: ct);

        if (handle.Status == AsyncOperationStatus.Succeeded)
        {
            _spawnedHandles.Add(handle);
            return handle.Result;
        }

        Debug.LogError("Failed to spawn enemy");
        return null;
    }

    public void Despawn(GameObject enemy)
    {
        var handle = _spawnedHandles.FirstOrDefault(h => h.Result == enemy);
        if (handle.IsValid())
        {
            _spawnedHandles.Remove(handle);
            Addressables.ReleaseInstance(enemy);
        }
    }

    private void OnDestroy()
    {
        foreach (var handle in _spawnedHandles)
        {
            if (handle.IsValid())
                Addressables.ReleaseInstance(handle);
        }
        _spawnedHandles.Clear();
    }
}
```

### Example 3: Preload by Label
**User**: "Preload all 'level-1' tagged assets."

**Agent**:
```csharp
public class LevelPreloader : MonoBehaviour
{
    private AsyncOperationHandle<IList<Object>> _preloadHandle;

    public async UniTask PreloadLevelAsync(string label, CancellationToken ct)
    {
        _preloadHandle = Addressables.LoadAssetsAsync<Object>(
            label, asset => Debug.Log($"Loaded: {asset.name}"));

        await _preloadHandle.ToUniTask(cancellationToken: ct);

        Debug.Log($"Preloaded {_preloadHandle.Result.Count} assets for '{label}'");
    }

    public void UnloadLevel()
    {
        if (_preloadHandle.IsValid())
        {
            Addressables.Release(_preloadHandle);
        }
    }

    private void OnDestroy() => UnloadLevel();
}
```

## Memory Management & Resources Migration

Rules for migrating from `Resources.Load` and managing asset memory across scene transitions.

- [ ] Do NOT use the `Resources` folder — migrate all assets to Addressables
  - Grep: `grep -rn "Resources\.Load" --include="*.cs"`
  - Severity: 🟡 HIGH
- [ ] Call `Resources.UnloadUnusedAssets()` after scene transitions when still using Resources — aware it causes a hitch
- [ ] Properly unload Addressables handles when leaving a scene — `Addressables.Release(handle)` in `OnDestroy`
- [ ] Event cleanup in `OnDestroy` → see `@unity-event-safety` §1 Event Subscription Symmetry
- [ ] Enable Incremental GC: Project Settings → Player → Other Settings → Use Incremental GC `[EDITOR-ONLY]`
- [ ] Take Memory Profiler snapshots before and after scene transitions to detect unreleased handles `[EDITOR-ONLY]`

---

## Related Skills
- `@unity-csharp-standards` - Hot-path rules, GC reduction, memory leak prevention
- `@unity-event-safety` - Event subscription cleanup in OnDestroy
- `@unity-startup-loading` - Boot-time sequencing for preload/init work
