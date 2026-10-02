// Top-N slowest frames in the Profiler buffer, each broken down by marker.
// Run after recording:  utk run_script --file prof_spikes.cs --entry ProfSpikes.Run
// Works on whatever the Profiler window holds: Editor Play mode or a connected device.
// Frame time excludes "EditorLoop" so Editor-only overhead doesn't rank frames.
using System.Collections.Generic;
using System.Linq;
using System.Text;
using UnityEditor.Profiling;
using UnityEditorInternal;

public static class ProfSpikes
{
    const int TopFrames = 5;      // frames to break down
    const float MinMs = 2f;       // hide markers cheaper than this
    const int MaxLines = 45;      // per frame
    const int MaxDepth = 14;

    static HierarchyFrameDataView View(int f) =>
        ProfilerDriver.GetHierarchyFrameDataView(f, 0,
            HierarchyFrameDataView.ViewModes.MergeSamplesWithTheSameName,
            HierarchyFrameDataView.columnTotalTime, false);

    static float EditorLoopMs(HierarchyFrameDataView v)
    {
        var kids = new List<int>();
        v.GetItemChildren(v.GetRootItemID(), kids);
        foreach (var k in kids)
            if (v.GetItemName(k) == "EditorLoop")
                return v.GetItemColumnDataAsFloat(k, HierarchyFrameDataView.columnTotalTime);
        return 0f;
    }

    public static string Run()
    {
        var sb = new StringBuilder();
        int first = ProfilerDriver.firstFrameIndex, last = ProfilerDriver.lastFrameIndex;
        if (first < 0 || last < first) return "no frames recorded";

        var frames = new List<(int idx, float ms)>();
        for (int f = first; f <= last; f++)
            using (var v = View(f))
                if (v.valid) frames.Add((f, v.frameTimeMs - EditorLoopMs(v)));
        if (frames.Count == 0) return "no valid frames";

        var sorted = frames.Select(x => x.ms).OrderBy(x => x).ToList();
        sb.AppendLine($"frames {frames.Count} ({first}-{last}) median {sorted[sorted.Count / 2]:F1}ms " +
                      $"p95 {sorted[(int)(sorted.Count * 0.95f)]:F1}ms max {sorted[sorted.Count - 1]:F1}ms");

        foreach (var fr in frames.OrderByDescending(x => x.ms).Take(TopFrames))
        {
            sb.AppendLine($"== frame {fr.idx} {fr.ms:F1}ms");
            using (var v = View(fr.idx))
            {
                int lines = 0;
                void Walk(int id, int depth)
                {
                    var kids = new List<int>();
                    v.GetItemChildren(id, kids);
                    foreach (var k in kids)
                    {
                        if (lines >= MaxLines) return;
                        string n = v.GetItemName(k);
                        float t = v.GetItemColumnDataAsFloat(k, HierarchyFrameDataView.columnTotalTime);
                        if (t < MinMs || n == "EditorLoop") continue;
                        float self = v.GetItemColumnDataAsFloat(k, HierarchyFrameDataView.columnSelfTime);
                        sb.AppendLine($"{new string(' ', depth)}{n} {t:F0} (self {self:F0})");
                        lines++;
                        if (depth < MaxDepth) Walk(k, depth + 1);
                    }
                }
                Walk(v.GetRootItemID(), 0);
            }
        }
        return sb.ToString();
    }
}
