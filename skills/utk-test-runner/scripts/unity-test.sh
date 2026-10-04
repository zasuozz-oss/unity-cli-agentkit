#!/bin/bash
# Headless Unity compile check + test runner (EditMode/PlayMode).
# CLI args verified against official Unity 6 docs on 2026-08-21 — sources and
# gotchas in references/headless-testing.md.
#
# Usage:
#   <this skill>/scripts/unity-test.sh compile  <game-repo-path>
#   <this skill>/scripts/unity-test.sh editmode <game-repo-path> [results.xml]
#   <this skill>/scripts/unity-test.sh playmode <game-repo-path> [results.xml]
#   <this skill>/scripts/unity-test.sh --selftest
#
# Unity binary: $UNITY_BIN if set, else the project's pinned version under
# /Applications/Unity/Hub/Editor (macOS Hub default), else any installed.
# Timeout: $UNITY_TIMEOUT seconds (default 1200) via timeout/gtimeout if present.
set -u

MODE="${1:-}"

parse_results() { # <nunit3-xml> — prints summary; exit 0 only if all passed
    python3 - "$1" <<'PY'
import sys, xml.etree.ElementTree as ET
a = ET.parse(sys.argv[1]).getroot().attrib
print(f"result={a.get('result')} total={a.get('total')} "
      f"passed={a.get('passed')} failed={a.get('failed')} skipped={a.get('skipped')}")
sys.exit(0 if str(a.get('result', '')).startswith('Passed') else 1)
PY
}

if [ "$MODE" = "--selftest" ]; then
    TMP=$(mktemp)
    echo '<test-run testcasecount="3" total="3" passed="3" failed="0" skipped="0" result="Passed(Success)"/>' > "$TMP"
    parse_results "$TMP" > /dev/null || { echo "selftest FAIL: passing XML not accepted"; rm -f "$TMP"; exit 1; }
    echo '<test-run testcasecount="3" total="3" passed="2" failed="1" skipped="0" result="Failed(Child)"/>' > "$TMP"
    if parse_results "$TMP" > /dev/null; then echo "selftest FAIL: failing XML accepted"; rm -f "$TMP"; exit 1; fi
    rm -f "$TMP"
    echo "selftest ok (2 checks)"
    exit 0
fi

PROJECT="${2:-}"
if [ -z "$MODE" ] || [ -z "$PROJECT" ]; then
    echo "usage: unity-test.sh compile|editmode|playmode <game-repo-path> [results.xml]" >&2
    exit 64
fi
[ -d "$PROJECT" ] || { echo "not a directory: $PROJECT" >&2; exit 64; }

find_unity() {
    if [ -n "${UNITY_BIN:-}" ]; then echo "$UNITY_BIN"; return; fi
    local ver
    ver=$(awk '/m_EditorVersion:/{print $2}' "$PROJECT/ProjectSettings/ProjectVersion.txt" 2>/dev/null)
    local dir
    for dir in "/Applications/Unity/Hub/Editor/$ver" /Applications/Unity/Hub/Editor/*; do
        if [ -x "$dir/Unity.app/Contents/MacOS/Unity" ]; then
            echo "$dir/Unity.app/Contents/MacOS/Unity"
            return
        fi
    done
}
UNITY=$(find_unity)
[ -n "$UNITY" ] || { echo "Unity editor not found — set UNITY_BIN=/path/to/Unity" >&2; exit 69; }

TIMEOUT_S="${UNITY_TIMEOUT:-1200}"
TIMEOUT_CMD=""
command -v gtimeout > /dev/null && TIMEOUT_CMD="gtimeout $TIMEOUT_S"
command -v timeout  > /dev/null && TIMEOUT_CMD="timeout $TIMEOUT_S"
[ -z "$TIMEOUT_CMD" ] && echo "note: timeout(1) not found (brew install coreutils) — running unbounded" >&2

LOG=$(mktemp)
case "$MODE" in
    compile)
        # -quit is safe ONLY without -runTests. Unity exits 1 on compile errors.
        $TIMEOUT_CMD "$UNITY" -batchmode -nographics -quit \
            -projectPath "$PROJECT" -accept-apiupdate -logFile "$LOG"
        rc=$?
        if [ $rc -eq 0 ]; then
            echo "COMPILE OK"
        else
            echo "COMPILE FAILED (exit $rc) — errors:" >&2
            grep -E "error CS|Exception|Assets/.*\.cs" "$LOG" | head -30 >&2
        fi
        rm -f "$LOG"
        exit $rc
        ;;
    editmode|playmode)
        PLATFORM=EditMode
        [ "$MODE" = "playmode" ] && PLATFORM=PlayMode
        RESULTS="${3:-$PROJECT/test-results-$MODE.xml}"
        # NEVER add -quit here: it kills in-progress -runTests runs (Unity docs).
        $TIMEOUT_CMD "$UNITY" -batchmode -nographics -projectPath "$PROJECT" \
            -runTests -testPlatform "$PLATFORM" -testResults "$RESULTS" -logFile "$LOG"
        rc=$?
        if [ -f "$RESULTS" ]; then
            rm -f "$LOG"
            parse_results "$RESULTS"
            exit $?
        fi
        echo "no results file written (unity exit $rc) — last log lines:" >&2
        tail -30 "$LOG" >&2
        rm -f "$LOG"
        exit 1
        ;;
    *)
        echo "unknown mode: $MODE (compile|editmode|playmode|--selftest)" >&2
        exit 64
        ;;
esac
