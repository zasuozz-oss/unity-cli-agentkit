#!/bin/bash
# Launch the Unity Editor for a project with -automated and wait until the
# utk pipeline is reachable. -automated is MANDATORY for unattended runs:
# without it any modal dialog blocks the pipeline server until a human clicks
# (Unity Hub cannot pass the flag). Source:
# unity-cli-agentkit plugin, skill utk-cli-core: "Editor must run with -automated".
#
# Usage:
#   <this skill>/scripts/unity-editor-open.sh <game-repo-path> [--headless]
#   <this skill>/scripts/unity-editor-open.sh --selftest
#
# --headless adds -batchmode (omit -quit): resident Editor serving the
# pipeline API on a box with no display. GUI mode: keep the window in the
# FOREGROUND — a backgrounded Editor is throttled by the OS.
# Unity binary: $UNITY_BIN, else the project's pinned version under
# /Applications/Unity/Hub/Editor. Wait cap: $EDITOR_WAIT seconds (default 300).
set -u

if [ "${1:-}" = "--selftest" ]; then
    TMP=$(mktemp -d)
    if bash "$0" "$TMP/nope" >/dev/null 2>&1; then
        echo "selftest FAIL: accepted a missing project"; rm -r "$TMP"; exit 1; fi
    mkdir -p "$TMP/proj/Assets" "$TMP/proj/ProjectSettings" "$TMP/bin"
    printf '#!/bin/bash\necho reachable\nexit 0\n' > "$TMP/bin/utk"
    chmod +x "$TMP/bin/utk"
    # utk already reachable -> returns 0 without launching anything
    PATH="$TMP/bin:$PATH" bash "$0" "$TMP/proj" >/dev/null 2>&1 \
        || { echo "selftest FAIL: reachable editor not detected"; rm -r "$TMP"; exit 1; }
    # unreachable utk + missing Unity binary -> clean failure, no hang
    printf '#!/bin/bash\nexit 1\n' > "$TMP/bin/utk"
    if PATH="$TMP/bin:$PATH" UNITY_BIN="$TMP/no-unity" EDITOR_WAIT=1 bash "$0" "$TMP/proj" >/dev/null 2>&1; then
        echo "selftest FAIL: missing Unity binary accepted"; rm -r "$TMP"; exit 1; fi
    # `utk status` exit 0 while reporting UNREACHABLE must NOT count as reachable
    # (real utk behaviour: exit 0, "… port 0  UNREACHABLE" on stdout)
    printf '#!/bin/bash\necho "proj  pipeline 0.5.0  pid 0  port 0  UNREACHABLE"\nexit 0\n' > "$TMP/bin/utk"
    if PATH="$TMP/bin:$PATH" UNITY_BIN="$TMP/no-unity" EDITOR_WAIT=1 bash "$0" "$TMP/proj" >/dev/null 2>&1; then
        echo "selftest FAIL: UNREACHABLE treated as reachable"; rm -r "$TMP"; exit 1; fi
    # same for "no editor instances" and SAFE MODE
    printf '#!/bin/bash\necho "no editor instances (is Unity open?)"\nexit 0\n' > "$TMP/bin/utk"
    if PATH="$TMP/bin:$PATH" UNITY_BIN="$TMP/no-unity" EDITOR_WAIT=1 bash "$0" "$TMP/proj" >/dev/null 2>&1; then
        echo "selftest FAIL: 'no editor instances' treated as reachable"; rm -r "$TMP"; exit 1; fi
    printf '#!/bin/bash\necho "proj  pipeline 0.5.0  pid 42  port 5001  SAFE MODE"\nexit 0\n' > "$TMP/bin/utk"
    if PATH="$TMP/bin:$PATH" UNITY_BIN="$TMP/no-unity" EDITOR_WAIT=1 bash "$0" "$TMP/proj" >/dev/null 2>&1; then
        echo "selftest FAIL: SAFE MODE treated as reachable"; rm -r "$TMP"; exit 1; fi
    rm -r "$TMP"
    echo "selftest ok (6 checks)"
    exit 0
fi

REPO="${1:-}"
[ -n "$REPO" ] || { echo "usage: unity-editor-open.sh <game-repo-path> [--headless]" >&2; exit 64; }
[ -d "$REPO/Assets" ] && [ -d "$REPO/ProjectSettings" ] || {
    echo "not a Unity project (need Assets/ and ProjectSettings/): $REPO" >&2; exit 65; }

# `utk status` exits 0 even with NO live editor — it reports reachability in its
# output ("… port 0  UNREACHABLE", "no editor instances", "SAFE MODE"), not in
# the exit code. Trusting the exit code here silently skips the launch below and
# leaves every later utk call failing with COMMAND_FAILED (exit 6).
utk_reachable() {
    local out
    out=$(cd "$1" && utk status 2>&1) || return 1
    printf '%s' "$out" | grep -qi 'UNREACHABLE\|no editor instances\|SAFE MODE' && return 1
    printf '%s' "$out" | grep -qi 'reachable'
}

if utk_reachable "$REPO"; then
    echo "editor already reachable for $REPO"
    exit 0
fi

if [ -n "${UNITY_BIN:-}" ]; then
    UNITY="$UNITY_BIN"
else
    VER=$(awk '/m_EditorVersion:/{print $2}' "$REPO/ProjectSettings/ProjectVersion.txt" 2>/dev/null)
    UNITY="/Applications/Unity/Hub/Editor/$VER/Unity.app/Contents/MacOS/Unity"
fi
[ -x "$UNITY" ] || { echo "Unity editor not found: $UNITY — set UNITY_BIN" >&2; exit 69; }

FLAGS=(-projectPath "$REPO" -automated -buildTarget "${BUILD_TARGET:-android}" -logFile "$REPO/Logs/agent-editor.log")
[ "${2:-}" = "--headless" ] && FLAGS+=(-batchmode)   # resident: no -quit, keeps serving utk
mkdir -p "$REPO/Logs"
nohup "$UNITY" "${FLAGS[@]}" >/dev/null 2>&1 &
echo "launched Unity (pid $!) with -automated — waiting for utk pipeline..."

WAIT="${EDITOR_WAIT:-300}"
for _ in $(seq 1 $((WAIT / 5))); do
    sleep 5
    if utk_reachable "$REPO"; then
        echo "editor reachable for $REPO"
        exit 0
    fi
done
echo "editor not reachable after ${WAIT}s — check $REPO/Logs/agent-editor.log" >&2
echo "(compile errors boot the Editor into SAFE MODE where utk cannot connect —" >&2
echo " grep 'error CS' in the log; see utk-cli-core SKILL 'Safe Mode')" >&2
exit 1
