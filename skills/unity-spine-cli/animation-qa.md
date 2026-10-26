# Spine animation QA and retargeting

Companion to `SKILL.md`. Static frame montages hide what users reject
("looks like walking", "zombie", "broken arm"): judge motion with numbers.

## Measure motion, not frames

- **Sample, don't eyeball.** Load both skeletons with spine-core
  (spine-ts / spine-csharp), step the animation at the export FPS and record
  every bone's world position. Compare per-bone displacement amplitude
  (chest, head, hands, feet) against the reference rig.
- **Onion skin** one full cycle per character (all frames overlaid) and put
  the two side by side; a stick-figure overlay from the sampled bones makes
  joint breaks obvious.
- **Sway pivot height.** Transferring root/hip sway between rigs: the same
  angle on a pivot placed at a different height throws chest/head/hands
  several times further. Move the pivot to the equivalent height, then
  re-check amplitudes.
- **Stylistic motion** ("a proper catwalk", "like this dance"): ask for a
  reference clip **up front**. `ffprobe`/`ffmpeg` to extract the first N
  frames, crop around the character, build a phase-aligned sheet (video row
  vs render row). Without a clip, every guessed style round produces a new
  complaint.
- **Smoothness:** measure peak frame-to-frame acceleration of ankles/hands
  before and after a change; prefer `sin²` bumps and cosine blends (zero
  velocity at contact/lift) over `max(sin, 0)`. Extremes must line up (hip
  peak on the swing-leg peak frame).
- **Planted foot must not slide** — check its world position across the
  contact frames.
- Check the **dressed** preview too (shoes, hair, clothing slots), not just
  the bare body.

## Retarget in world space

- Copy **world-space motion** (joint positions per frame; length change →
  `scaleX`), not only bone rotation angles — a rig whose source animation
  lifts the foot or shortens limbs by scale goes static or breaks otherwise.
- **IK targets** scaled to the new limb length: keep the source extension
  ratio (shoulder–wrist distance / arm length). Keep the previous bend
  direction when the limb is nearly straight, or the elbow flips per frame.
- **Soft limits**, not hard clamps: `L * tanh(v / L)`. A hard clamp holds a
  bone still for several frames, then jerks.
- Crossing legs/feet need separate slots and a draw-order timeline in the
  rig; check rig capability before promising the pose.

## Iteration hygiene

- No git in the workspace? `cp -R` a snapshot before every visual-tuning
  round, so "go back to the previous version" is one command.
- Find the root cause with numbers (per-frame reach, angles, amplitudes)
  **before** changing code; stacked blind fixes each add a defect.
- Isolate a defect by rendering variants that each drop one key group (no
  arm scale / no arm rotation / no torso rotation).
