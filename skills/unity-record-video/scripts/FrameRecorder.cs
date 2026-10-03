// FrameRecorder.cs — copy into the game's Editor assembly (e.g. Assets/<Game>/Scripts/Editor/), set the namespace.
using System;
using System.Collections;
using System.Collections.Generic;
using System.IO;
using UnityEngine;

namespace Game.EditorTools
{
    /// <summary>
    /// Dev-only deterministic capture (play mode, driven by a `utk exec` script). While it runs, Time.captureFramerate
    /// pins game time to 1/fps per frame however slow the capture is, and every frame's Game view (UI included) goes to
    /// Temp/rec/&lt;name&gt;/f_00000.png. <see cref="Label"/> lines go to labels.txt (frame, text) for encode.py;
    /// done.txt appears after the last frame. The coroutine runs on <c>host</c>, so a scene change that destroys the
    /// host kills it: pick a host that survives the take, or call <see cref="Stop"/> after the change.
    /// </summary>
    public static class FrameRecorder
    {
        private static readonly List<string> Labels = new List<string>();
        private static Coroutine _run;
        private static MonoBehaviour _host;

        public static bool Busy { get; private set; }
        /// <summary>Index of the frame being made (0-based).</summary>
        public static int Frame { get; private set; }
        public static string Dir { get; private set; }

        /// <summary>Marks a label change at the current frame (drawn by the encoder until the next one).</summary>
        public static void Label(string text) => Labels.Add(Frame + "\t" + text);

        /// <summary>
        /// Records <paramref name="frames"/> frames into Temp/rec/<paramref name="name"/> (wiped first).
        /// <paramref name="onFrame"/>(i) runs before frame i is drawn, so it can drive the hero, the clock, input…
        /// </summary>
        public static string Begin(string name, int frames, MonoBehaviour host, Action<int> onFrame = null, int fps = 30)
        {
            if (!Application.isPlaying) return "ABORT: play mode only";
            if (Busy) return "ABORT: already recording " + Dir;
            if (host == null) return "ABORT: no host MonoBehaviour (is the scene loaded?)";
            _host = host;
            Dir = Path.GetFullPath(Path.Combine("Temp", "rec", name));
            if (Directory.Exists(Dir)) Directory.Delete(Dir, true);
            Directory.CreateDirectory(Dir);
            Labels.Clear();
            Frame = 0;
            Busy = true;
            Time.captureFramerate = fps;
            _run = _host.StartCoroutine(Run(frames, onFrame));
            return $"recording {frames} frames ({frames / (float)fps:0.0} s) to {Dir}";
        }

        /// <summary>Ends a take early (or after a scene change killed it) and restores real time.</summary>
        public static void Stop()
        {
            if (_run != null && _host != null) _host.StopCoroutine(_run);
            Finish();
        }

        private static IEnumerator Run(int frames, Action<int> onFrame)
        {
            var endOfFrame = new WaitForEndOfFrame();
            for (Frame = 0; Frame < frames; Frame++)
            {
                try { onFrame?.Invoke(Frame); }
                catch (Exception e)
                {
                    Debug.LogException(e);
                    Labels.Add(Frame + "\tERROR " + e.Message);
                    Finish();
                    yield break;
                }
                yield return endOfFrame;
                var tex = ScreenCapture.CaptureScreenshotAsTexture();
                File.WriteAllBytes(Path.Combine(Dir, $"f_{Frame:00000}.png"), tex.EncodeToPNG());
                UnityEngine.Object.Destroy(tex);
                yield return null;
            }
            Finish();
        }

        private static void Finish()
        {
            Time.captureFramerate = 0;
            if (Busy && Dir != null)
            {
                File.WriteAllLines(Path.Combine(Dir, "labels.txt"), Labels);
                File.WriteAllText(Path.Combine(Dir, "done.txt"), Frame + " frames");
            }
            Busy = false;
            _run = null;
        }
    }
}
