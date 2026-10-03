// rec_template.cs — real-time take of a scripted flow with Unity Recorder (com.unity.recorder), for `utk exec`.
// Copy to <repo>/Tools/rec_<name>.cs, set the PARAMETERS, write the GAME-SPECIFIC section, then:
//   utk set_autotick --enable true          (an unfocused Editor crawls otherwise)
//   utk exec --file Tools/rec_<name>.cs --timeout 60000
// Output: Temp/rec/<NAME>_raw.mp4 + Temp/rec/<NAME>_events.txt (seconds since record start <TAB> what), for trimming.
// Abort: run once with AppDomain data "<NAME>_stop" = true (stops hook + recorder); leaving Play also ends it.
// The script returns immediately; the take runs on EditorApplication.update. Poll for "finish:" in the events file.

// ===== PARAMETERS
const string NAME = "flow";
const int W = 1080, H = 1920, FPS = 30;
const float STEP_TIMEOUT_S = 90f;         // a step that never completes ends the take with TIMEOUT
// =====

if (!UnityEngine.Application.isPlaying) return "ABORT: enter play mode first";
var dom = System.AppDomain.CurrentDomain;
var oldHook = dom.GetData(NAME + "_hook") as UnityEditor.EditorApplication.CallbackFunction;
if (oldHook != null) UnityEditor.EditorApplication.update -= oldHook;
var oldCtl = dom.GetData(NAME + "_ctl") as UnityEditor.Recorder.RecorderController;
if (oldCtl != null && oldCtl.IsRecording()) oldCtl.StopRecording();
if (dom.GetData(NAME + "_stop") is bool stopOnly && stopOnly) { dom.SetData(NAME + "_stop", false); return "stopped"; }

string dir = System.IO.Path.GetFullPath(System.IO.Path.Combine("Temp", "rec"));
System.IO.Directory.CreateDirectory(dir);
string raw = System.IO.Path.Combine(dir, NAME + "_raw");
string evPath = System.IO.Path.Combine(dir, NAME + "_events.txt");
foreach (var f in new[] { raw + ".mp4", evPath }) if (System.IO.File.Exists(f)) System.IO.File.Delete(f);

// ---- recorder: CONSTANT fps (Variable made AVAssetWriter reject a frame -> unplayable file)
var cs = UnityEngine.ScriptableObject.CreateInstance<UnityEditor.Recorder.RecorderControllerSettings>();
var mv = UnityEngine.ScriptableObject.CreateInstance<UnityEditor.Recorder.MovieRecorderSettings>();
mv.name = NAME;
mv.Enabled = true;
mv.EncoderSettings = new UnityEditor.Recorder.Encoder.CoreEncoderSettings
{
    Codec = UnityEditor.Recorder.Encoder.CoreEncoderSettings.OutputCodec.MP4,
    EncodingQuality = UnityEditor.Recorder.Encoder.CoreEncoderSettings.VideoEncodingQuality.High
};
mv.CaptureAudio = false;
mv.ImageInputSettings = new UnityEditor.Recorder.Input.GameViewInputSettings { OutputWidth = W, OutputHeight = H };
mv.OutputFile = raw;
cs.AddRecorderSettings(mv);
cs.SetRecordModeToManual();
cs.FrameRatePlayback = UnityEditor.Recorder.FrameRatePlayback.Constant;
cs.FrameRate = FPS;
cs.CapFrameRate = true;
var ctl = new UnityEditor.Recorder.RecorderController(cs);
dom.SetData(NAME + "_ctl", ctl);

// ---- helpers
var events = new System.Collections.Generic.List<string>();
float t0 = -1f;
float Now() => UnityEngine.Time.realtimeSinceStartup;
void Ev(string what)
{
    events.Add($"{(t0 < 0 ? 0 : Now() - t0):0.000}\t{what}");
    System.IO.File.WriteAllLines(evPath, events);
}
UnityEngine.EventSystems.PointerEventData Ptr() =>
    new UnityEngine.EventSystems.PointerEventData(UnityEngine.EventSystems.EventSystem.current) { button = UnityEngine.EventSystems.PointerEventData.InputButton.Left };
// what a finger does to a uGUI button: down, up, click (hold buttons need down/up only)
void Click(UnityEngine.GameObject go)
{
    var e = Ptr();
    UnityEngine.EventSystems.ExecuteEvents.Execute(go, e, UnityEngine.EventSystems.ExecuteEvents.pointerDownHandler);
    UnityEngine.EventSystems.ExecuteEvents.Execute(go, e, UnityEngine.EventSystems.ExecuteEvents.pointerUpHandler);
    UnityEngine.EventSystems.ExecuteEvents.Execute(go, e, UnityEngine.EventSystems.ExecuteEvents.pointerClickHandler);
}
// an active GameObject by name anywhere under root (buttons often sit in nested prefabs)
UnityEngine.GameObject Find(UnityEngine.Transform root, string name)
{
    foreach (var t in root.GetComponentsInChildren<UnityEngine.Transform>(true))
        if (t.name == name && t.gameObject.activeInHierarchy) return t.gameObject;
    throw new System.Exception($"no active {name} under {root.name}");
}
var steps = new System.Collections.Generic.List<(string name, System.Func<float, bool> run)>();
void Wait(float s) => steps.Add(("wait " + s, t => t >= s));                   // t = seconds since the step began
void Until(string n, System.Func<bool> c) => steps.Add((n, t => c()));          // poll a game signal, never sleep
void Do(string n, System.Action a) => steps.Add((n, t => { a(); return true; }));
System.Action everyTick = null;                                                  // optional per-tick player (mini games…)

// ===== GAME-SPECIFIC: state reset + the flow. Keep it to Wait/Until/Do and the game's own handlers.
// World taps: call the handler the game's tap raycast ends in (e.g. a TapObject(collider) method),
// not synthetic screen touches. The Editor has no mic: answer voice/timing games by reading the game's state.
// Example:
//   Until("menu", () => UnityEngine.GameObject.Find("MainMenu") != null);
//   Wait(1.5f);
//   Do("play", () => { Click(Find(UnityEngine.GameObject.Find("MainMenu").transform, "PlayButton")); Ev("tap play"); });
//   Until("level loaded", () => UnityEngine.SceneManagement.SceneManager.GetActiveScene().name == "Level1");
//   Wait(3f);
System.Action resetAndGo = () =>
{
    // e.g. game.ResetProgress(); SceneManager.LoadScene("Boot");   (the save was backed up BEFORE this script)
};
// ===== END GAME-SPECIFIC

int idx = 0;
float stepStart = 0f;
UnityEditor.EditorApplication.CallbackFunction hook = null;
void Finish(string why)
{
    UnityEditor.EditorApplication.update -= hook;
    if (ctl.IsRecording()) ctl.StopRecording();
    Ev("finish: " + why);
}
hook = () =>
{
    if (!UnityEngine.Application.isPlaying) { UnityEditor.EditorApplication.update -= hook; return; }
    try
    {
        everyTick?.Invoke();
        if (idx >= steps.Count) { Finish("done"); return; }
        if (steps[idx].run(Now() - stepStart)) { idx++; stepStart = Now(); }
        else if (Now() - stepStart > STEP_TIMEOUT_S) Finish($"TIMEOUT at step {idx} {steps[idx].name}");
    }
    catch (System.Exception e) { Finish($"ERROR at step {idx} {(idx < steps.Count ? steps[idx].name : "")}: {e.Message}"); }
};
dom.SetData(NAME + "_hook", hook);

ctl.PrepareRecording();
if (!ctl.StartRecording()) return "ABORT: recorder did not start (is com.unity.recorder installed?)";
t0 = Now();
stepStart = t0;
Ev("recording");
resetAndGo();
UnityEditor.EditorApplication.update += hook;
return $"recording to {raw}.mp4, {steps.Count} steps; events in {evPath}";
