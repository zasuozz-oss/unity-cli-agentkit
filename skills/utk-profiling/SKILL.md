---
name: utk-profiling
description: Use when a Unity game lags, hitches, stutters or drops FPS — "lag khi vào menu", "khựng khi chuyển scene", "FPS tụt", "giật khi đổi đồ", slow scene load, a long frame on init (Spine, Addressables, sprites) — and the cause must be measured with the Unity Profiler from an agent through `utk`, in Editor Play mode or on a connected Android device, instead of guessed from code.
---

# Profile through `utk` — measure, don't guess

The user's standing rule: **"dùng profiler để đo, không đoán bừa"**. A cause
read from code is a hypothesis; name it as one until a frame breakdown shows
the marker and its milliseconds.

## Record in Editor Play mode

```sh
utk exec 'UnityEditorInternal.ProfilerDriver.ClearAllFrames();
  UnityEditorInternal.ProfilerDriver.profileEditor = false;
  UnityEditorInternal.ProfilerDriver.enabled = true; return "on";'
# drive the slow flow (utk-playmode-driving): tap / LoadScene / open the tab
utk exec 'UnityEditorInternal.ProfilerDriver.enabled = false;
  return UnityEditorInternal.ProfilerDriver.firstFrameIndex + "-" + UnityEditorInternal.ProfilerDriver.lastFrameIndex;'
utk run_script --file <this skill>/scripts/prof_spikes.cs --entry ProfSpikes.Run
```

- Start recording **right before** the action and stop right after — the
  buffer keeps ~300 frames by default; a long idle pushes the spike out.
- Autotick on (`utk set_autotick`), or an unfocused Editor records nothing
  useful. Check `Time.timeScale` and `isPaused` first (utk-playmode-driving →
  "State that lies to you").
- `prof_spikes.cs` prints count/median/p95/max, then the 5 slowest frames as
  a marker tree with total/self ms (edit the constants at the top to change).
- Measure twice (cold + warm); report both. A second run often hides the
  spike because data is already cached.
- Setting `ProfilerDriver.enabled = true` through `exec` has crashed a Unity
  6000.0 Editor once (`profiling::Dispatcher::AcquireFreeBuffer`; next calls
  say `No Pipeline instance found`). If it does, wait for the Editor to come
  back and take counters instead: start
  `Unity.Profiling.ProfilerRecorder.StartNew(Unity.Profiling.ProfilerCategory.Render, "Draw Calls Count", 300)`
  (or `ProfilerCategory.Internal, "Main Thread"`) in one exec and park it in
  `System.AppDomain.CurrentDomain.SetData("rec", r)`; read it back with
  `GetData("rec")` in a second exec a few seconds later.

## Editor numbers that don't exist on device

| Marker | Why it misleads |
|---|---|
| `GC.Collect` inside `LoadLevelAsync Integrate`, hundreds of ms | Editor's managed heap (often >1 GB with no Play) — not the game's |
| `Mono.JIT` | IL2CPP builds have no JIT |
| `EditorLoop` | Editor UI; `prof_spikes.cs` already subtracts it |
| Addressables loads | Editor reads the project directly (AssetDatabase mode), not bundles |

If the remaining real markers don't explain what the user sees, **go to the
device** before optimising further.

## Record on an Android device

1. Build with **Development Build + Autoconnect Profiler**, IL2CPP like
   release; Deep Profiling **off** (5-10× slower, numbers unrepresentative).
2. Install, launch; the Profiler window attaches (or
   `adb forward tcp:34999 localabstract:Unity-<package>` and pick the device
   as target). Turn off *Profile Editor*.
3. Record **1000+ frames** across the slow flow (raise the buffer in
   Preferences → Analysis if needed); the user drives if `adb input` is blocked
   (MIUI — see `@unity-device-testing`).
4. Same `prof_spikes.cs` on the recorded buffer.
5. A dev build carries overhead: treat its numbers as an upper bound; the
   ranking of markers is what matters.

## Findings that came up repeatedly

- `SpriteMeshGenerator.TraceShape/Simplify` under `Start()` → runtime
  `Sprite.Create(...)` defaults to a **Tight** mesh and traces every big image
  (~15 ms each). Pass `SpriteMeshType.FullRect` for UI/background sprites.
- Many `Start()`s decoding images for **hidden** panels inside the
  scene-load frame → preload them during an existing loading wait, 1 decode
  per frame, cached across the scene change.
- Spine: each item `.skel` carrying the full skeleton + every animation,
  parsed on the main thread on equip → strip unused animations from item
  files, preload the equipped set during loading, prefetch visible grid cells.
- "Lag on tap" that the Profiler shows as 1-5 ms frames is **latency**, not a
  hitch (N frames of waiting for a load) — fix by preloading, not by
  optimising code.
- "The whole game is slow" with cheap scripts: time every enabled
  `Update`/`LateUpdate` first (call each via a delegate N times in one exec —
  µs/frame and GC bytes/frame). When scripts are cheap, the next suspect is
  **texture memory**: textures that fell back to RGBA32 on Android
  (unity-asset-audit §4.1), then overdraw and draw calls.

## Report

Table per change: marker → before ms → after ms, Editor or device, frame
count. Say which numbers are Editor-only. Remove temporary timing logs
(`PerfLog`, stopwatches) before finishing unless the user wants them kept.

## Related Skills

- `@utk-playmode-driving` — driving the flow while recording; autotick
- `@unity-device-testing` — dev build install, logcat, MIUI input
- `@unity-ui-performance` — canvas rebuild / overdraw fixes once a marker points there
- `@unity-spine-ui` — Spine runtime loading and skins
