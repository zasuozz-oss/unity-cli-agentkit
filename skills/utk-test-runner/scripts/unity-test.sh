#!/bin/bash
# Unity compile check + test runner. ALWAYS run `offline` first: it needs no Unity Editor at all.
# Editor/batchmode modes only for what offline cannot cover (PlayMode, scenes/assets, runtime tests).
# CLI args verified against official Unity 6 docs on 2026-08-21 — sources and
# gotchas in references/headless-testing.md.
#
# Usage:
#   <this skill>/scripts/unity-test.sh offline  <game-repo-path> [--tests]   (no Editor, ~3-10 s)
#   <this skill>/scripts/unity-test.sh compile  <game-repo-path>
#   <this skill>/scripts/unity-test.sh editmode <game-repo-path> [results.xml]
#   <this skill>/scripts/unity-test.sh playmode <game-repo-path> [results.xml]
#   <this skill>/scripts/unity-test.sh --selftest
#
# offline: `dotnet build` on the solution/csproj files Unity generated (~3 s, catches CS errors).
#   Prints CS errors (deduplicated) and "COMPILE OK" / "COMPILE FAILED"; exit 0 / 1.
#   --tests then runs the built EditMode test assemblies with NUnitLite on plain .NET (no Unity):
#   tests that touch native UnityEngine/UnityEditor code fail with an ECall SecurityException and
#   are reported as SKIPPED-NEEDS-EDITOR, not FAIL; only real assertion failures exit 1.
#   Tests run with the repo as cwd (some read project files): do not use it on a repo whose tests write
#   tracked files. First use builds the runner into $UTK_OFFLINE_RUNNER (default
#   ~/.unity-cli-agentkit/offline-runner; needs NUnitLite 3.14 from NuGet).
#   One build at a time per repo (mkdir lock): msbuild writes Temp/obj. Unity itself compiles in
#   Library/Bee, so an open Editor is fine.
#   Limit: csproj files only list the .cs files the Editor knew at its last project sync; a new .cs
#   is reported NOT-IN-CSPROJ and is compiled only by the next Editor job (utk editor refresh).
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

RUNNER_CS='using System.Reflection; using System.Runtime.Loader; using NUnitLite;
var dirs = (Environment.GetEnvironmentVariable("UTK_PROBE_DIRS") ?? "").Split(":", StringSplitOptions.RemoveEmptyEntries);
AssemblyLoadContext.Default.Resolving += (ctx, n) => {
  foreach (var d in dirs) { var p = Path.Combine(d, n.Name + ".dll"); if (File.Exists(p)) return ctx.LoadFromAssemblyPath(p); }
  return null; };
return new AutoRun(Assembly.LoadFrom(args[0])).Execute(args.Skip(1).ToArray());'
RUNNER_PROJ='<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings></PropertyGroup><ItemGroup><PackageReference Include="NUnitLite" Version="3.14.0" /></ItemGroup></Project>'

summarize_nunit() { # <nunit3-xml> <assembly> — counts; exit 1 only on real failures
    python3 - "$1" "$2" <<'PY'
import sys, xml.etree.ElementTree as ET
ran = ok = bad = ed = 0
for t in ET.parse(sys.argv[1]).getroot().iter("test-case"):
    ran += 1
    if t.get("result") == "Passed": ok += 1; continue
    m = " ".join((t.findtext(".//message") or "").split())
    if "ECall methods" in m or "No log scope" in m or "MissingMethodException" in m:
        ed += 1
    else:
        bad += 1; print("FAIL", t.get("fullname"), "-", m[:160])
if ran: print(f"TESTS {sys.argv[2]}: ran={ran} passed={ok} failed={bad} SKIPPED-NEEDS-EDITOR={ed}")
sys.exit(1 if bad else 0)
PY
}

offline_tests() { # cwd = repo, after a good build
    local R=$PWD runner=${UTK_OFFLINE_RUNNER:-$HOME/.unity-cli-agentkit/offline-runner}
    if [ ! -f "$runner/bin/runner.dll" ]; then
        mkdir -p "$runner/src"; printf '%s\n' "$RUNNER_PROJ" > "$runner/src/runner.csproj"; printf '%s\n' "$RUNNER_CS" > "$runner/src/Program.cs"
        (cd "$runner/src" && dotnet build -nologo -v q -o "$runner/bin" >"$runner/build.log" 2>&1) \
            || { echo "offline tests: runner build failed (see $runner/build.log)"; return 2; }
    fi
    local contents; contents=$(grep -ho '/[^<"]*Unity.app/Contents' *.csproj 2>/dev/null | head -1)
    local probe="$R/Temp/bin/Debug:$R/Library/ScriptAssemblies"
    [ -n "$contents" ] && probe="$probe:$contents/Managed:$contents/Managed/UnityEngine:$contents/NetStandard/compat/2.1.0/shims/netstandard"
    local rc=0 proj name dll xml; xml=$(mktemp)
    for proj in $(grep -l 'nunit.framework' *.csproj 2>/dev/null); do
        name=${proj%.csproj}; dll=Temp/bin/Debug/$name.dll
        [ -f "$dll" ] || { echo "$name: no built dll ($dll)"; continue; }
        UTK_PROBE_DIRS=$probe perl -e 'alarm shift; exec @ARGV' "${UNITY_TIMEOUT:-1200}" \
            dotnet "$runner/bin/runner.dll" "$dll" --result="$xml" --work="$(dirname "$xml")" >/dev/null 2>&1
        [ -s "$xml" ] || { echo "$name: runner wrote no results"; rc=1; continue; }
        summarize_nunit "$xml" "$name" || rc=1
        : > "$xml"
    done
    rm -f "$xml"; return $rc
}

offline_mode() { # <repo> [--tests]
    command -v dotnet >/dev/null || { echo "unity-test offline: dotnet not found (brew install dotnet)"; return 2; }
    cd "$1" || return 2
    local SLN; SLN=$(ls *.slnx *.sln 2>/dev/null | head -1)
    [ -n "$SLN" ] || { echo "no .sln/.slnx in $1: open the project in the Editor once to generate it"; return 2; }
    LOCK=Temp/unity-compile.lock.d
    mkdir -p Temp
    for i in $(seq 1 120); do mkdir "$LOCK" 2>/dev/null && break; sleep 1; done
    [ -d "$LOCK" ] || { echo "unity-test offline: lock busy for 120 s ($LOCK)"; return 2; }
    trap 'rmdir "$LOCK" 2>/dev/null' EXIT
    local OUT rc
    OUT=$(perl -e 'alarm 300; exec @ARGV' dotnet build "$SLN" -nologo -v q 2>&1)
    rc=$?
    echo "$OUT" | grep -E "error CS|error NETSDK|error MSB" | sed -E 's/ \[[^]]*\]$//' | sort -u
    # .cs files under Assets that no csproj lists: new since the last Editor project sync
    git ls-files -co --exclude-standard -- 'Assets/*.cs' | python3 -c '
import sys, glob, re
listed = set()
for p in glob.glob("*.csproj"):
    listed.update(m.replace("\\\\", "/") for m in re.findall(r"<Compile Include=\"([^\"]+)\"", open(p, encoding="utf-8-sig").read()))
for f in sys.stdin.read().split("\n"):
    if f and f not in listed and "/Editor Default Resources/" not in f: print("NOT-IN-CSPROJ", f)'
    [ $rc = 0 ] || { echo "COMPILE FAILED (exit $rc)"; return 1; }
    echo "COMPILE OK"
    [ "${2:-}" = --tests ] && { offline_tests; return $?; }
    return 0
}

offline_selftest() {
    command -v dotnet >/dev/null || { echo "offline selftest SKIP: dotnet not installed"; return 0; }
    local T; T=$(mktemp -d); trap 'rm -rf "$T"' RETURN
    mkdir -p "$T/Assets"; git -C "$T" init -q
    printf '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Assets/A.cs" /></ItemGroup></Project>\n' > "$T/G.csproj"
    printf '<Solution><Project Path="G.csproj" /></Solution>\n' > "$T/G.slnx"
    echo 'class A { int F() => 1; }' > "$T/Assets/A.cs"
    "$0" offline "$T" | grep -q '^COMPILE OK$' || { echo "selftest FAIL: good build"; return 1; }
    echo 'class A { int F() => "s"; }' > "$T/Assets/A.cs"; touch "$T/Assets/B.cs"
    local O; O=$("$0" offline "$T"); [ $? = 1 ] || { echo "selftest FAIL: bad build exit"; return 1; }
    echo "$O" | grep -q 'error CS0029' && echo "$O" | grep -q 'NOT-IN-CSPROJ Assets/B.cs' || { echo "selftest FAIL: output"; echo "$O"; return 1; }
    [ ! -d "$T/Temp/unity-compile.lock.d" ] || { echo "selftest FAIL: lock left"; return 1; }
    echo "offline selftest ok (3 checks)"
    # --tests: one pass, one fake native-Unity failure (needs Editor), one real failure
    local nu; nu=$(ls "$HOME"/.nuget/packages/nunit/3.14.0/lib/netstandard2.0/nunit.framework.dll 2>/dev/null)
    [ -n "$nu" ] || { echo "offline --tests selftest SKIP: NUnit 3.14 not in the NuGet cache"; return 0; }
    printf '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>false</EnableDefaultCompileItems><OutputPath>Temp/bin/Debug/</OutputPath><AppendTargetFrameworkToOutputPath>false</AppendTargetFrameworkToOutputPath></PropertyGroup><ItemGroup><Compile Include="Assets/T.cs" /><Reference Include="nunit.framework"><HintPath>%s</HintPath></Reference></ItemGroup></Project>\n' "$nu" > "$T/G.csproj"
    cat > "$T/Assets/T.cs" <<'CS'
using NUnit.Framework;
[TestFixture] public class T {
  [Test] public void Passes() => Assert.AreEqual(2, 1 + 1);
  [Test] public void NeedsEditor() => throw new System.Security.SecurityException("ECall methods must be packaged into a system module.");
  [Test] public void RealFail() => Assert.AreEqual(3, 1 + 1);
}
CS
    O=$(UTK_OFFLINE_RUNNER=${UTK_OFFLINE_RUNNER:-$T/runner} "$0" offline "$T" --tests); local rc=$?
    echo "$O" | grep -q 'TESTS G: ran=3 passed=1 failed=1 SKIPPED-NEEDS-EDITOR=1' && [ $rc = 1 ] || { echo "selftest FAIL: --tests ($rc)"; echo "$O"; return 1; }
    echo "offline --tests selftest ok"
}

if [ "$MODE" = "--selftest" ]; then
    TMP=$(mktemp)
    echo '<test-run testcasecount="3" total="3" passed="3" failed="0" skipped="0" result="Passed(Success)"/>' > "$TMP"
    parse_results "$TMP" > /dev/null || { echo "selftest FAIL: passing XML not accepted"; rm -f "$TMP"; exit 1; }
    echo '<test-run testcasecount="3" total="3" passed="2" failed="1" skipped="0" result="Failed(Child)"/>' > "$TMP"
    if parse_results "$TMP" > /dev/null; then echo "selftest FAIL: failing XML accepted"; rm -f "$TMP"; exit 1; fi
    rm -f "$TMP"
    echo "selftest ok (2 checks)"
    offline_selftest; exit $?
fi

[ "$MODE" = offline ] && { [ -n "${2:-}" ] || { echo "usage: unity-test.sh offline <game-repo> [--tests]" >&2; exit 64; }; offline_mode "$2" "${3:-}"; exit $?; }
PROJECT="${2:-}"
if [ -z "$MODE" ] || [ -z "$PROJECT" ]; then
    echo "usage: unity-test.sh offline|compile|editmode|playmode <game-repo-path> [results.xml]" >&2
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
        echo "unknown mode: $MODE (offline|compile|editmode|playmode|--selftest)" >&2
        exit 64
        ;;
esac
