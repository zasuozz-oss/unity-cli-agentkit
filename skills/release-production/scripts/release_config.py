#!/usr/bin/env python3
"""Apply / verify dev vs prod critical config for any Unity game, driven by app-config-checklist.md.

Run from the Unity project root:
  release_config.py init                 create the checklist table by probing common SDK files (Dev column = now)
  release_config.py check dev|prod       verify every row on disk matches that env; exit 1 on mismatch
  release_config.py apply dev|prod       write that env's values (kind regex/file) into the project
  release_config.py next-version         suggest next version name/code from tags + prod branch
  release_config.py set-version NAME CODE
  release_config.py tidy                 undo GUID-only churn on .meta files of checklist targets
  release_config.py setting KEY          print a setting (dev_branch, prod_branch, tag)

Checklist table (after the <!-- release-config:table --> marker), one row per critical value:
  | Key | Kind | Target | Pattern | Dev | Prod |
  Key     platform.name, e.g. android.admob.app_id / ios.admob.app_id (prefix drives the cross-platform check)
  Kind    regex  = Pattern (exactly one capture group) is read AND written in Target
                   (single-line pattern with the other env kept as `// ` lines → apply toggles comments, values untouched)
          verify = same, read-only (files the editor regenerates, e.g. google-services.xml)
          file   = Target must be byte-identical to the source file named in Dev/Prod
  Dev == Prod on every row of a Target → that file is presence-checked only (skipped by the leak scan).
  —       value must be empty (regex/verify) / Target must not exist (file). Escape | in a pattern as \\|.
Settings (after <!-- release-config:settings -->): "- key: value" lines. Defaults below.
"""
import re, shutil, subprocess, sys
from pathlib import Path

ROOT = Path.cwd()
CHECKLIST = ROOT / "app-config-checklist.md"
PROJECT_SETTINGS = ROOT / "ProjectSettings/ProjectSettings.asset"
TABLE_MARKER = "<!-- release-config:table -->"
SETTINGS_MARKER = "<!-- release-config:settings -->"
NONE = "—"
DEFAULT_SETTINGS = {"dev_branch": "main", "prod_branch": "production", "tag": "v{name}-{code}"}

# Standard Unity SDK locations probed by `init`. Rows whose Target doesn't exist are dropped.
# Game-specific places (keys hardcoded in C#, custom configs) must be added by hand.
TEMPLATE = [
    ("android.package", "regex", "ProjectSettings/ProjectSettings.asset", r"applicationIdentifier:\n(?:    .*\n)*?    Android: (\S+)"),
    ("ios.bundle", "regex", "ProjectSettings/ProjectSettings.asset", r"applicationIdentifier:\n(?:    .*\n)*?    iPhone: (\S+)"),
    ("android.levelplay.app_key", "regex", "Assets/LevelPlay/Resources/LevelPlayMediationSettings.asset", r"^  AndroidAppKey: ?(.*)$"),
    ("ios.levelplay.app_key", "regex", "Assets/LevelPlay/Resources/LevelPlayMediationSettings.asset", r"^  IOSAppKey: ?(.*)$"),
    ("android.admob.app_id", "regex", "Assets/GoogleMobileAds/Resources/GoogleMobileAdsSettings.asset", r"^  adMobAndroidAppId: ?(.*)$"),
    ("ios.admob.app_id", "regex", "Assets/GoogleMobileAds/Resources/GoogleMobileAdsSettings.asset", r"^  adMobIOSAppId: ?(.*)$"),
    ("android.admob.manifest", "verify", "Assets/Plugins/Android/GoogleMobileAdsPlugin.androidlib/AndroidManifest.xml", r'gms\.ads\.APPLICATION_ID" android:value="([^"]*)"'),
    ("android.gpgs.app_id", "verify", "Assets/Plugins/Android/GooglePlayGamesManifest.androidlib/AndroidManifest.xml", r'games\.APP_ID"\s+android:value="\\u003(\d+)"'),
    ("android.gpgs.setup_app_id", "verify", "ProjectSettings/GooglePlayGameSettings.txt", r"^proj\.AppId=(.*)$"),
    ("android.gpgs.setup_client_id", "verify", "ProjectSettings/GooglePlayGameSettings.txt", r"^and\.ClientId=(.*)$"),
    ("android.firebase.config", "file", "Assets/google-services.json", ""),
    ("android.firebase.project_id", "verify", "Assets/google-services.json", r'"project_id": "([^"]*)"'),
    ("android.firebase.package", "verify", "Assets/google-services.json", r'"package_name": "([^"]*)"'),
    ("android.firebase.xml", "verify", "Assets/Plugins/Android/FirebaseApp.androidlib/res/values/google-services.xml", r'name="project_id"[^>]*>([^<]*)<'),
    ("android.firebase.desktop", "verify", "Assets/StreamingAssets/google-services-desktop.json", r'"project_id": "([^"]*)"'),
    ("ios.firebase.config", "file", "Assets/GoogleService-Info.plist", ""),
    ("ios.firebase.desktop", "verify", "Assets/StreamingAssets/GoogleService-Info-desktop.json", r'"project_id": "([^"]*)"'),
]


def fail(msg):
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(1)


def read(p):
    return Path(p).read_text(encoding="utf-8")


def git(*args):
    return subprocess.run(["git", *args], capture_output=True, text=True).stdout.strip()


# ---------- checklist ----------

def checklist_text():
    if not CHECKLIST.exists():
        fail(f"{CHECKLIST.name} not found — run `init` first")
    return read(CHECKLIST)


def cell(c):
    c = c.strip().replace("\\|", "|")
    return c[1:-1] if len(c) >= 2 and c[0] == c[-1] == "`" else c


def load_rows():
    text = checklist_text()
    if TABLE_MARKER not in text:
        fail(f"{CHECKLIST.name} has no {TABLE_MARKER} table — run `init`")
    rows = []
    for line in text.split(TABLE_MARKER, 1)[1].splitlines():
        if not line.strip().startswith("|"):
            if rows:
                break
            continue
        cells = [cell(c) for c in re.split(r"(?<!\\)\|", line.strip().strip("|"))]
        if len(cells) != 6 or cells[1] not in ("regex", "verify", "file"):
            continue  # header / separator
        key, kind, target, pattern, dev, prod = cells
        if kind != "file" and re.compile(pattern).groups != 1:
            fail(f"{key}: pattern must have exactly one capture group")
        rows.append({"key": key, "kind": kind, "target": ROOT / target, "pattern": pattern, "dev": dev, "prod": prod})
    if not rows:
        fail("checklist table has no rows")
    return rows


def settings():
    s = dict(DEFAULT_SETTINGS)
    text = checklist_text()
    if SETTINGS_MARKER in text:
        for m in re.finditer(r"^- (\w+): `?([^`\n]+)`?$", text.split(SETTINGS_MARKER, 1)[1], re.M):
            s[m.group(1)] = m.group(2).strip()
    return s


def value(row, env):
    v = row[env]
    if not v or v == "?":
        fail(f"{row['key']}: no {env} value in checklist")
    return "" if v == NONE else v


# ---------- check ----------

def matches(row):
    return re.findall(row["pattern"], read(row["target"]), re.M) if row["target"].exists() else None


def strip_comments(text):
    return "\n".join(l for l in text.splitlines() if not l.lstrip().startswith(("//", "#")))


def cmd_check(env):
    rows = load_rows()
    other_env = "dev" if env == "prod" else "prod"
    errors = []

    for r in rows:
        exp = value(r, env)
        rel = r["target"].relative_to(ROOT)
        if r["kind"] == "file":
            if not exp:
                if r["target"].exists():
                    errors.append(f"{r['key']}: {rel} must not exist for {env}")
            elif not (ROOT / exp).exists():
                errors.append(f"{r['key']}: source {exp} missing")
            elif not r["target"].exists() or read(r["target"]) != read(ROOT / exp):
                errors.append(f"{r['key']}: {rel} differs from {exp}")
            continue
        found = matches(r)
        if found is None:
            if exp:
                errors.append(f"{r['key']}: {rel} missing")
        elif not found and exp:
            errors.append(f"{r['key']}: pattern not found in {rel}")
        elif any(f.strip() != exp for f in found):
            errors.append(f"{r['key']}: {rel} has {sorted(set(found))}, {env} expects '{exp or NONE}'")

    # Leak: a value that belongs only to the other env must not appear in any target (comments ignored)
    mine = {value(r, env) for r in rows if r["kind"] != "file"}
    # Files whose rows all hold one shared value (dev == prod, e.g. Play Games ids tied to the release
    # package) are presence-checked only: they may legitimately contain the other env's values.
    scanned = {r["target"] for r in rows if r["kind"] == "file" or r["dev"] != r["prod"]}
    corpus = {t: strip_comments(read(t)) for t in scanned if t.exists()}
    leaked = set()
    for r in rows:
        v = value(r, other_env)
        if r["kind"] == "file" or len(v) < 6 or v in mine:
            continue
        for path, text in corpus.items():
            if v in text and (v, path) not in leaked:
                leaked.add((v, path))
                errors.append(f"LEAK: {other_env} value of {r['key']} ('{v}') found in {path.relative_to(ROOT)}")

    # Cross-platform: android.X and ios.X must differ, and neither may sit in the other's own file
    by_key = {r["key"]: r for r in rows}
    for k, a in by_key.items():
        if not k.startswith("android.") or a["kind"] == "file":
            continue
        i = by_key.get("ios." + k[len("android."):])
        if not i or i["kind"] == "file":
            continue
        va, vi = value(a, env), value(i, env)
        if va and va == vi:
            errors.append(f"PLATFORM: {k} and {i['key']} have the same value '{va}'")
        for v, src, dst in ((va, a, i), (vi, i, a)):
            if v and len(v) >= 6 and dst["target"] != src["target"] and dst["target"] in corpus and v in corpus[dst["target"]]:
                errors.append(f"PLATFORM: {src['key']} value '{v}' found in {dst['target'].relative_to(ROOT)}")

    name, code = get_version()
    print(f"env={env} version={name} ({code}) rows={len(rows)}")
    if errors:
        print("\n".join(f"  FAIL {e}" for e in dict.fromkeys(errors)))
        sys.exit(1)
    print(f"  OK all critical config matches {env}")


# ---------- apply ----------

COMMENTED = re.compile(r"^(\s*)//\s?(.*)$")


def toggle_comments(text, pattern, v):
    """Code keeps both envs as `// ...` blocks (e.g. //Test + //Release): switch by commenting the
    active line and uncommenting the one holding v. Returns None when no line holds v (fall back to
    replacing the value) or the pattern spans lines."""
    if "\n" in pattern:
        return None
    rx = re.compile(pattern)
    lines = text.splitlines(keepends=True)
    active, commented = [], []
    for i, line in enumerate(lines):
        body = line.rstrip("\r\n")
        m = COMMENTED.match(body)
        if m and (h := rx.search(m.group(1) + m.group(2))):
            commented.append((i, h.group(1).strip()))
        elif not m and (h := rx.search(body)):
            active.append((i, h.group(1).strip()))
    if not any(val == v for _, val in active + commented):
        return None
    for i, val in active:
        if val != v:
            indent = re.match(r"\s*", lines[i]).group(0)
            lines[i] = indent + "// " + lines[i][len(indent):]
    if not any(val == v for _, val in active):
        i = next(i for i, val in commented if val == v)
        m = COMMENTED.match(lines[i].rstrip("\r\n"))
        lines[i] = m.group(1) + m.group(2) + lines[i][len(lines[i].rstrip("\r\n")):]
    return "".join(lines)

def cmd_apply(env):
    for r in load_rows():
        v = value(r, env)
        if r["kind"] == "verify":
            continue
        if r["kind"] == "file":
            if v:
                shutil.copyfile(ROOT / v, r["target"])
            elif r["target"].exists():
                fail(f"{r['key']}: {r['target'].relative_to(ROOT)} must not exist for {env} — remove it (and its .meta) by hand")
            continue
        text = read(r["target"])
        toggled = toggle_comments(text, r["pattern"], v)
        if toggled is not None:
            r["target"].write_text(toggled, encoding="utf-8")
            continue
        new, n = re.subn(r["pattern"], lambda m: text[m.start():m.start(1)] + v + text[m.end(1):m.end()], text, flags=re.M)
        if n == 0:
            fail(f"{r['key']}: pattern not found in {r['target'].relative_to(ROOT)}")
        r["target"].write_text(new, encoding="utf-8")
    print(f"applied {env}. Next: `utk editor refresh` (editor regenerates verify-rows), `tidy`, then `check {env}`")


# ---------- version ----------

def get_version(text=None):
    text = text if text is not None else read(PROJECT_SETTINGS)
    n = re.search(r"^  bundleVersion: (.*)$", text, re.M)
    c = re.search(r"^  AndroidBundleVersionCode: (\d+)$", text, re.M)
    return (n.group(1).strip() if n else "?", int(c.group(1)) if c else -1)


def cmd_set_version(name, code):
    if not re.fullmatch(r"\d+\.\d+\.\d+", name) or not code.isdigit():
        fail("usage: set-version 1.2.3 130")
    text = read(PROJECT_SETTINGS)
    text, a = re.subn(r"^(  bundleVersion: ).*$", lambda m: m.group(1) + name, text, flags=re.M)
    text, b = re.subn(r"^(  AndroidBundleVersionCode: )\d+$", lambda m: m.group(1) + code, text, flags=re.M)
    text = re.sub(r"(\n  buildNumber:\n(?:    .*\n)*?    iPhone: )\S+", lambda m: m.group(1) + code, text)
    if not (a and b):
        fail("bundleVersion / AndroidBundleVersionCode not found in ProjectSettings.asset")
    PROJECT_SETTINGS.write_text(text, encoding="utf-8")
    print(f"version set to {name} ({code}) — Android versionCode + iOS buildNumber")


def cmd_next_version():
    s = settings()
    tag_re = re.escape(s["tag"]).replace(r"\{name\}", r"(\d+\.\d+\.\d+)").replace(r"\{code\}", r"(\d+)")
    seen = []
    for tag in git("tag", "-l").split():
        m = re.fullmatch(tag_re, tag)
        if m:
            seen.append((m.group(1), int(m.group(2))))
    for ref in (s["prod_branch"], f"origin/{s['prod_branch']}"):
        t = git("show", f"{ref}:ProjectSettings/ProjectSettings.asset")
        if t:
            seen.append(get_version(t))
    seen = [(n, c) for n, c in seen if re.fullmatch(r"\d+\.\d+\.\d+", n) and c >= 0]
    if not seen:
        fail("no previous release (no matching tag, no prod branch) — ask the user for version")
    last = max(tuple(map(int, n.split("."))) for n, _ in seen)
    code = max(c for _, c in seen)
    print(f"last={'.'.join(map(str, last))} ({code})  suggested={last[0]}.{last[1]}.{last[2] + 1} ({code + 1})")


# ---------- misc ----------

def cmd_tidy():
    # Editor plugins (e.g. Firebase) delete + recreate generated files, so Unity mints a new GUID in the
    # .meta — pure churn. Restore the committed GUID when that's the only change.
    restored = []
    for r in load_rows():
        meta = f"{r['target'].relative_to(ROOT)}.meta"
        diff = git("diff", "-U0", "--", meta)
        changed = [l for l in diff.splitlines() if l[:1] in "+-" and not l.startswith(("+++", "---"))]
        if changed and all(l[1:].startswith("guid:") for l in changed):
            git("checkout", "--", meta)
            restored.append(meta)
    print(f"restored GUID: {', '.join(restored)} — run `utk editor refresh` once more" if restored else "nothing to tidy")


def cmd_setting(key):
    s = settings()
    if key not in s:
        fail(f"unknown setting {key}")
    print(s[key])


def cmd_init():
    if CHECKLIST.exists() and TABLE_MARKER in read(CHECKLIST):
        fail(f"{CHECKLIST.name} already has the table")
    lines = []
    for key, kind, target, pattern in TEMPLATE:
        p = ROOT / target
        if kind == "file":
            dev = "?" if p.exists() else NONE
        else:
            if not p.exists():
                continue
            found = re.findall(pattern, read(p), re.M)
            dev = (found[0].strip() or NONE) if found else "?"
        pat = f"`{pattern.replace('|', chr(92) + '|')}`" if pattern else ""
        lines.append(f"| `{key}` | {kind} | `{target}` | {pat} | `{dev}` | ? |")
    body = (f"{SETTINGS_MARKER}\n" + "".join(f"- {k}: `{v}`\n" for k, v in DEFAULT_SETTINGS.items()) +
            f"\n{TABLE_MARKER}\n| Key | Kind | Target | Pattern | Dev | Prod |\n|---|---|---|---|---|---|\n" + "\n".join(lines) + "\n")
    head = "# 📋 CHECKLIST CẤU HÌNH GAME\n\n## 🔐 Mã quan trọng (skill release-production ghi & kiểm tra)\n\n"
    old = read(CHECKLIST) if CHECKLIST.exists() else ""
    CHECKLIST.write_text(head + body + ("\n---\n\n" + old if old else ""), encoding="utf-8")
    print(f"wrote {CHECKLIST.name} with {len(lines)} rows — fill every `?`, add game-specific rows (keys in code), then `check dev`")


if __name__ == "__main__":
    a = sys.argv[1:]
    cmds = {"check": (cmd_check, 1), "apply": (cmd_apply, 1), "set-version": (cmd_set_version, 2),
            "next-version": (cmd_next_version, 0), "init": (cmd_init, 0), "tidy": (cmd_tidy, 0),
            "setting": (cmd_setting, 1)}
    if not a or a[0] not in cmds or len(a) - 1 != cmds[a[0]][1] or (a[0] in ("check", "apply") and a[1] not in ("dev", "prod")):
        print(__doc__)
        sys.exit(2)
    cmds[a[0]][0](*a[1:])
