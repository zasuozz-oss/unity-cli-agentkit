---
name: unity-record-video
description: Use in any Unity game when the user asks for a video or clip of the running game — "quay video", "quay lại video", record a flow (start → first quest, a tutorial, an unlock), the day/night clock, a loading screen, an FX or animation — or when an earlier take came out choppy, unplayable ("moov atom not found"), too fast/slow, frozen, or cut short. Ships a Unity Recorder template, a deterministic frame recorder and an ffmpeg encoder with labels (scripts/).
---

# Record a video of the running game

Without this skill every video request grows a new one-off `rec_*.cs`: one
game collected 12 of them, about 650 lines, in one week. The shape never
changes; only the game-specific steps do. Copy a script from `scripts/` (next
to this file), fill the marked section, run the fixed recipe below. Play,
autotick and Game view size are covered in **`unity-live-editor-loop`**.

## 1. Pick the capture path

| Need | Path | Script |
|---|---|---|
| A **flow** played in real time: UI taps, dialogue, scene loads | **Unity Recorder** (`com.unity.recorder`), mp4 out | `scripts/rec_template.cs` |
| **Exact timing** independent of Editor speed: a clock sweep, fog dissolving, a walk, FX, a loading screen; on-screen labels | **FrameRecorder**, PNG frames → `encode.py` | `scripts/FrameRecorder.cs` + `scripts/encode.py` |

- **Recorder** records what the Editor actually plays, across scene loads.
  Game time is real time, so a slow Editor makes a slow take. Not installed?
  Add `com.unity.recorder` to `Packages/manifest.json` (or
  `UnityEditor.PackageManager.Client.Add`), then `utk editor refresh` before
  using it — the package add reloads the domain.
- **FrameRecorder** sets `Time.captureFramerate`, so every frame advances
  game time by exactly 1/fps however slow the capture is: smooth and
  repeatable, and `onFrame(i)` can drive the clock, the hero and labels frame
  by frame. It copies into an Editor folder of the game (`Assets/<Game>/Editor/`)
  and runs on a host MonoBehaviour, so a scene change that destroys the host
  kills the take. Frames = seconds × fps; whatever the take shows must run on
  `Time` (deltaTime/time), not real time or `dspTime`. A 10 s clock sweep:
  ```csharp
  return FrameRecorder.Begin("hours", 300, host, i => {
      clock.SetHour(6f + 12f * i / 299f);              // the game's own setter
      if (i % 30 == 0) FrameRecorder.Label($"{6 + 12 * i / 299:00}:00");
  });
  ```
- **Neither records audio.** Say so up front if the user needs sound.

## 2. The recipe

1. **Back up the save** (and PlayerPrefs) to a folder you never overwrite —
   recipe in `utk-playmode-driving`. A take needs a fresh or staged save;
   stage it only in that throwaway state.
2. **Game view size** = the store aspect (1080×1920 or 1080×2340,
   `PlayModeWindow.SetCustomRenderingResolution`), then `utk editor play`,
   then **`utk set_autotick --enable true`**.
3. **Stage the state** with a small setup snippet that returns `READY` or
   `NOT READY: <why>`: close stray popups, set the hour, place the hero.
   Rerun it until it says READY.
4. **Drive input the way a finger would.** uGUI buttons: `ExecuteEvents`
   pointerDown → pointerUp → pointerClick (the template's `Click()`); pass the
   handler explicitly (`ExecuteEvents.pointerClickHandler`) or the generic
   type cannot be inferred. World taps: call the handler the game's tap
   raycast ends in. Wait on game signals (`Until(...)`), never a bare sleep.
5. **Log events**: the template's `Ev("tap play")` writes `seconds<TAB>what`;
   FrameRecorder's `Label("06:00")` marks a frame. The log tells you where to
   trim and proves the flow reached its end.
6. **Run the take**: `utk exec --file Tools/rec_<name>.cs --timeout 60000`.
   The script returns at once; the take runs on `EditorApplication.update` or
   a coroutine. Wait on its marker with a bounded loop (or
   `run_in_background`):
   ```sh
   for i in $(seq 1 60); do grep -q finish Temp/rec/<name>_events.txt 2>/dev/null && break; sleep 5; done   # Recorder
   for i in $(seq 1 60); do [ -f Temp/rec/<name>/done.txt ] && break; sleep 3; done                          # FrameRecorder
   ```
   `finish: done` = worked; `TIMEOUT at step N` / `ERROR …` names the step
   that broke. Abort: set AppDomain data `<NAME>_stop = true` and run the
   script again, or leave Play.
7. **Only now `utk editor stop`.** A Recorder mp4 is finalized after
   `StopRecording`; stopping Play or reading it earlier leaves a file with no
   index (`moov atom not found`). Confirm with
   `ffprobe -v error -show_entries format=duration -of csv=p=0 <raw>.mp4`
   returning a number.
8. **Encode / trim.**
   - FrameRecorder: `python3 <this skill>/scripts/encode.py Temp/rec/<name> [--start 20] [--width 540] [--crf 23]`
     (refuses to run without `done.txt`; labels need Pillow).
   - Recorder: already mp4. Trim/shrink with times from the events file:
     `ffmpeg -y -ss <a> -to <b> -i Temp/rec/<name>_raw.mp4 -vf scale=540:-2 -c:v libx264 -crf 23 -pix_fmt yuv420p -movflags +faststart <out>.mp4`
9. **Watch it before sending** — one contact sheet beats N stills:
   `ffmpeg -v error -y -i out.mp4 -vf "fps=1,scale=180:-2,tile=8x2" -frames:v 1 Temp/rec/sheet.png`,
   then Read the PNG. Check the flow reached its end, the HUD is right,
   nothing froze.
10. **Clean up**: stop Play, restore the save backup, `utk set_autotick
    --enable false`. Move the mp4 out of `Temp/` if it must survive, and send
    it as an absolute path plus one line on length and content.

## 3. Gotchas (each one cost a retake)

| Symptom | Cause → fix |
|---|---|
| Unity log `Unable to encode video frame N` / `appendPixelBuffer() failed`, file unplayable | `FrameRatePlayback.Variable` → Constant, 30 fps, `CapFrameRate = true` (the template does this) |
| `moov atom not found`, `Invalid data found when processing input` | Play stopped or file read before finalize → step 7; the file cannot be repaired, retake |
| Take in slow motion, stalls, `Main thread operation timed out` | Unfocused Editor crawls → autotick on; if loading still stalls, `PlayerSettings.runInBackground = true` |
| Rhythm/audio-timed part out of sync | Runs on `AudioSettings.dspTime`, which FrameRecorder does not slow → record that part with the Recorder at ≥30 fps |
| First ~20 frames show the camera catching up | Teleport settle → `encode.py --start 20` |
| `No such filter: 'drawtext'` | Homebrew ffmpeg lacks it → `encode.py` renders labels to PNG and overlays them |
| FrameRecorder take dies at a scene load | Host destroyed → pick a host that survives, or Recorder |
| Project test "no file writes outside the save store" fails | FrameRecorder writes PNGs → exempt Editor folders in that test |

## Rules

- ✅ Reuse `scripts/`; only the GAME-SPECIFIC section is new. Save the result
  as `Tools/rec_<name>.cs` in the game repo so a retake is one command.
- ✅ A debug slow-down (e.g. a minimum loading-screen time) is a static that
  resets on the next Play, never a saved setting.
- ✅ The Editor has no mic or finger: answer voice/timing mini games from game
  state (tap when the marker is in the zone).
- ❌ **NEVER** record over the user's real save without a backup, or leave
  the Editor playing / on autotick.
- ❌ **NEVER** fake a step the flow could not do. A `TIMEOUT` is a bug to
  report, not to paper over by editing state mid-take.

## Related skills

- `unity-live-editor-loop` — Play, autotick, Game view size
- `utk-playmode-driving` — save backup/restore, `simulate_pointer`, screenshots
- `utk-exec-query` — the `utk exec` snippet format
