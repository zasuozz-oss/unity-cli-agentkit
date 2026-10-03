#!/usr/bin/env python3
"""encode.py — turns a FrameRecorder take (<dir>/f_%05d.png + labels.txt + done.txt) into an H.264 mp4.

labels.txt lines are "frame<TAB>text"; each label shows (white on a dark pill, top left) from its frame to the next
label. Homebrew ffmpeg has no drawtext, so every label is rendered to a PNG with Pillow and overlaid with
enable=between(n,a,b). Dev video only, never game art.

    python3 encode.py Temp/rec/fog_walk                 # -> Temp/rec/fog_walk.mp4
    python3 encode.py Temp/rec/hours --start 20         # drop 20 camera-settle frames
    python3 encode.py Temp/rec/x --width 720 --crf 20 -o ~/Desktop/x.mp4
"""
import argparse, glob, os, subprocess, sys

FONT = "/System/Library/Fonts/Supplemental/Arial Bold.ttf"

ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
ap.add_argument("dir", help="take folder holding f_00000.png ... and done.txt")
ap.add_argument("-o", "--out", help="output mp4 (default: <dir>.mp4)")
ap.add_argument("--start", type=int, default=0, help="first frame to keep (drop camera-settle frames)")
ap.add_argument("--fps", type=int, default=30, help="must match the fps the take was recorded at")
ap.add_argument("--width", type=int, default=540, help="output width; height keeps the aspect (default 540)")
ap.add_argument("--crf", type=int, default=23, help="x264 quality, lower = better/bigger (default 23)")
ap.add_argument("--labels", help="label file (default: <dir>/labels.txt if present)")
ap.add_argument("--font-size", type=int, default=30)
a = ap.parse_args()

src = os.path.abspath(os.path.expanduser(a.dir))
frames = sorted(glob.glob(os.path.join(src, "f_*.png")))
if not os.path.exists(os.path.join(src, "done.txt")):
    sys.exit(f"{src}: no done.txt, the take is not finished (or was aborted)")
if len(frames) <= a.start:
    sys.exit(f"{src}: {len(frames)} frames, nothing after --start {a.start}")
n = len(frames) - a.start

labels = []  # (first output frame, text)
lp = a.labels or os.path.join(src, "labels.txt")
if os.path.exists(lp):
    for line in open(lp, encoding="utf-8").read().splitlines():
        f, _, text = line.partition("\t")
        if text:
            labels.append((max(0, int(f) - a.start), text))

inputs = ["-framerate", str(a.fps), "-start_number", str(a.start), "-i", os.path.join(src, "f_%05d.png")]
chain, last = [f"[0:v]scale={a.width}:-2:flags=lanczos[v0]"], "[v0]"
if labels:
    from PIL import Image, ImageDraw, ImageFont
    try:
        font = ImageFont.truetype(FONT, a.font_size)
    except OSError:
        font = ImageFont.load_default()
    for i, (f0, text) in enumerate(labels):
        f1 = labels[i + 1][0] - 1 if i + 1 < len(labels) else n
        box = font.getbbox(text)
        img = Image.new("RGBA", (box[2] + 28, box[3] + 18), (0, 0, 0, 0))
        d = ImageDraw.Draw(img)
        d.rounded_rectangle((0, 0, img.width - 1, img.height - 1), radius=12, fill=(10, 12, 20, 170))
        d.text((14, 7), text, font=font, fill=(255, 255, 255, 255))
        png = os.path.join(src, f"label_{i:02d}.png")
        img.save(png)
        inputs += ["-i", png]
        chain.append(f"{last}[{i + 1}:v]overlay=x=24:y=24:enable='between(n,{f0},{f1})'[v{i + 1}]")
        last = f"[v{i + 1}]"

out = os.path.abspath(os.path.expanduser(a.out or src.rstrip("/") + ".mp4"))
cmd = ["ffmpeg", "-y", "-hide_banner", "-loglevel", "error", *inputs, "-filter_complex", ";".join(chain), "-map", last,
       "-frames:v", str(n), "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", str(a.crf), "-r", str(a.fps),
       "-movflags", "+faststart", out]
subprocess.run(cmd, check=True, timeout=600)
print(f"{out}: {n} frames, {n / a.fps:.1f} s, {len(labels)} labels")
