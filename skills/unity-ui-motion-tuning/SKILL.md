---
name: unity-ui-motion-tuning
description: "Use when authoring or tuning a Unity UI animation — a DOTween sequence, popup/celebration choreography, overlap and stagger offsets, easing choice, or any note that a motion is 'too fast', 'too slow', 'not smooth', 'doesn't match the reference video/prototype', or 'snaps at the end'. Covers scrubbing a sequence to sample frames, changing timings without a recompile, and calling DOTween from a utk exec snippet."
---

# Unity UI Motion — Tune It Without Recompiling

Tuning motion is a search for numbers a human judges by eye. The cost is not
the edit, it is the round-trip. Measured over one celebration popup's animation
in a real project (three sessions):

| Session | `utk editor refresh` | `utk screenshot` |
|---|---|---|
| A | 584 | 189 |
| B | 218 | 6 |
| C | 228 | 7 |

Sessions B and C recompiled ~220 times each and looked at the result seven
times — every value came back from the user ("still too fast", "0.45", "0.47").
Two causes, both fixable, and together they are what this skill is for:

- every timing was a `private const float`, so changing `0.4f` → `0.45f` meant
  an edit, a compile and a domain reload;
- the animation was played in real time and never sampled, so the agent could
  not see what it had made.

## Rule 1 — a tunable is never `const`

`const` is inlined at compile time: nothing can write it, not even reflection.
Put the numbers a human will argue about in one place as **`public static`**
fields (not `readonly`) and the whole sweep costs no compiles:

```csharp
// MyGame.UI/PopupMotion.cs — tunables, not constants
public static class PopupMotion
{
    public static float IconHop      = 0.47f;  // the value 15 round-trips found
    public static float IconPeakAt   = 0.5f;
    public static float IconHopPower = 260f;
}
```

```bash
utk exec 'MyGame.UI.PopupMotion.IconHop = 0.45f; return MyGame.UI.PopupMotion.IconHop;'
```

Keep `const` for anything the design does not tune (a physical ratio, an index).
A `[SerializeField]` on the component works too and persists into the prefab,
but it is only writable while that instance exists; a static tunable is writable
before the popup is ever built, which is what a sweep needs.

## Rule 2 — scrub the sequence, don't replay it

Retain the root `Sequence` in a field and build it with auto-kill off. Then any
frame is one call away, with no play mode, no waiting and no timing jitter:

```csharp
private Sequence _intro;                       // keep the handle
_intro = DOTween.Sequence().SetAutoKill(false).SetLink(gameObject);
```

Verified against a live editor — `Goto` samples the sequence exactly, in edit
mode, without playing it:

```
dur=2  0.00 x=0.0 s=1.00   0.25 x=25.0 s=1.00   0.50 x=50.0 s=1.00
       1.00 x=100.0 s=1.00 1.50 x=100.0 s=1.50  2.00 x=100.0 s=2.00
```

A filmstrip is then one exec per frame plus one screenshot, and **you** judge it:

```bash
for t in 0 0.1 0.2 0.3 0.4 0.5; do
  utk exec "var s = FindIntroSequence(); DG.Tweening.TweenExtensions.Goto(s, ${t}f, false); return ${t}f;"
  utk screenshot --output /tmp/strip/t${t}.png
done
```

Read the strip before reporting. Asking the user to re-watch a real-time
playback is what turns one question into fifteen.

## DOTween inside a `utk exec` snippet

DOTween's fluent API is **entirely extension methods**, and an exec snippet has
no `using` (see utk-exec-query), so instance syntax does not compile — measured:
four failed compiles before the first snippet landed. Call them statically:

| Written normally | In a snippet |
|---|---|
| `seq.SetAutoKill(false)` | `DG.Tweening.TweenSettingsExtensions.SetAutoKill(seq, false)` |
| `seq.Append(tw)`, `.Join`, `.SetDelay`, `.SetEase` | `DG.Tweening.TweenSettingsExtensions.<Name>(...)` |
| `seq.Goto(t, false)`, `.Pause`, `.Play`, `.Kill`, `.Duration`, `.Complete` | `DG.Tweening.TweenExtensions.<Name>(...)` |
| `t.DOScale(2f, 1f)`, `.DOLocalMoveX`, `.DOFade`, `.DOAnchorPos` | `DG.Tweening.ShortcutExtensions.<Name>(target, …)` |

`Pause`/`Play`/`Goto` live on `TweenExtensions`, not `TweenSettingsExtensions` —
guessing that one costs a round-trip. The `mscorlib 2.0 vs 4.0` warning DOTween
produces on every snippet is harmless; ignore it and read the real errors below.

## Sweep, don't ping-pong

The user says "too fast". Do not answer with one new number. Sample three or
four candidates in a single pass — a filmstrip each — then present the one you
would ship and say what the others looked like. One exchange instead of eight:
`0.40 / 0.47 / 0.55` answers "how fast" far better than `0.45` does.

## Two bugs that eat whole sessions

- **A parent's scale reaches every child.** Tweening a group from `1.2` → `1`
  scales the label inside it too; five consecutive round-trips went to "the
  label is *still* scaling". Counter-scaling fights the layout — take the child
  **out of the scaled group** and tween it on its own.
- **Layout snaps back when the motion ends.** Tweening `anchoredPosition` or
  size under a LayoutGroup / ContentSizeFitter works until the next rebuild
  overwrites it, which reads as a jerk at the end. Either disable the driver for
  the duration of the tween, or tween a child the driver does not control (see
  unity-ugui-layout).

## Matching a reference video or HTML prototype

Do not eyeball it. Pull the reference's frames at known times and sample your
sequence at the same times, then compare:

```bash
ffprobe -v error -show_entries format=duration -of csv=p=0 ref.mp4
ffmpeg -v error -i ref.mp4 -vf fps=10 /tmp/ref/f%03d.png
```

Frame `f008` at `fps=10` is t=0.8s: scrub to `0.8f`, screenshot, compare the two
images. What comes back is a number to change, not "it doesn't look right".

## Related Skills
- `@unity-dotween-safety` — tween lifecycle: `SetLink`, `DOKill`, leak rules.
  This skill is about authoring and tuning; that one keeps it from leaking.
- `@unity-ugui-layout` — anchors, LayoutGroup and ContentSizeFitter rules.
- `@utk-exec-query` — the snippet contract every command here relies on.
- `@unity-spine-ui` — when the motion is a Spine clip rather than a tween.
