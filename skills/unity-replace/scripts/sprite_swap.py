"""Swap an existing Unity sprite PNG for new art without touching its .meta.

  python sprite_swap.py inspect <Assets/.../old.png>
      size, alpha bbox, guid, spriteMode, spriteBorder, pixelsPerUnit
  python sprite_swap.py fit <new.png> <Assets/.../old.png> [--backup DIR] [--dry-run]
      resize new art to the old pixel size (LANCZOS, letterbox, keeps aspect),
      back the old file up, overwrite it in place. The .meta is never written.
  python sprite_swap.py selftest
"""
import os, re, shutil, sys, tempfile
from PIL import Image


def meta_fields(png):
    try:
        text = open(png + ".meta", encoding="utf-8").read()
    except FileNotFoundError:
        return {"meta": "MISSING - not an imported asset"}
    get = lambda key: (re.search(rf"^\s*{key}: (.+)$", text, re.M) or [None, None])[1]
    return {"guid": get("guid"), "spriteMode": get("spriteMode"),
            "spriteBorder": get("spriteBorder"), "spritePixelsToUnits": get("spritePixelsToUnits"),
            "maxTextureSize": get("maxTextureSize")}


def inspect(png):
    im = Image.open(png)
    bbox = im.convert("RGBA").getchannel("A").getbbox()
    info = {"path": png, "size": im.size, "alpha_bbox": bbox, **meta_fields(png)}
    for k, v in info.items():
        print(f"{k}: {v}")
    if info.get("spriteMode") == "2":
        print("WARNING: spriteMode 2 (Multiple) - a sheet of sub-sprites; overwriting it breaks their rects.")
    return info


def fit(new_png, size):
    """New art scaled to fit `size`, centered on a transparent canvas."""
    src = Image.open(new_png).convert("RGBA")
    w, h = size
    scale = min(w / src.width, h / src.height)
    scaled = src.resize((max(1, round(src.width * scale)), max(1, round(src.height * scale))), Image.LANCZOS)
    out = Image.new("RGBA", size, (0, 0, 0, 0))
    out.paste(scaled, ((w - scaled.width) // 2, (h - scaled.height) // 2))
    return out, abs(src.width / src.height - w / h) / (w / h)


def cmd_fit(new_png, old_png, backup, dry):
    size = Image.open(old_png).size
    out, aspect_diff = fit(new_png, size)
    print(f"old {size}, new {Image.open(new_png).size}, aspect diff {aspect_diff:.1%}")
    if aspect_diff > 0.02:
        print("WARNING: aspect differs >2% - result is letterboxed; the slot may need a new size.")
    if dry:
        return
    os.makedirs(backup, exist_ok=True)
    dst = os.path.join(backup, os.path.basename(old_png))
    if not os.path.exists(dst):  # keep the first original, never the previous attempt
        shutil.copy2(old_png, dst)
    out.save(old_png)
    print(f"wrote {old_png} (backup {dst}); .meta untouched")


def selftest():
    d = tempfile.mkdtemp()
    new = os.path.join(d, "new.png")
    Image.new("RGBA", (200, 100), (255, 0, 0, 255)).save(new)
    out, diff = fit(new, (100, 100))
    assert out.size == (100, 100)
    assert out.getchannel("A").getbbox() == (0, 25, 100, 75), out.getchannel("A").getbbox()
    assert diff > 0.9
    print("selftest ok")


if __name__ == "__main__":
    a = sys.argv[1:]
    if a[:1] == ["inspect"] and len(a) == 2:
        inspect(a[1])
    elif a[:1] == ["fit"] and len(a) >= 3:
        backup = a[a.index("--backup") + 1] if "--backup" in a else os.path.join(tempfile.gettempdir(), "sprite_swap_backup")
        cmd_fit(a[1], a[2], backup, "--dry-run" in a)
    elif a == ["selftest"]:
        selftest()
    else:
        sys.exit(__doc__)
