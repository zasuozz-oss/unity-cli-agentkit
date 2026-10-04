#!/bin/bash
# Compile a Unity game repo WITHOUT the Editor, in seconds: `dotnet build` on the solution and
# csproj files Unity generated (measured 2026-10-04 on a game with 29 projects, ~3 s, catches CS errors).
#   <this skill>/scripts/unity-compile.sh <game-repo>
# Prints the CS errors (deduplicated) and "COMPILE OK" / "COMPILE FAILED"; exit 0 / 1.
# Runs one build at a time per repo (mkdir lock): msbuild writes Temp/obj, so two agents building at
# once would race. Unity itself compiles in Library/Bee and ignores Temp/obj, so an open Editor is fine.
# Limit: the csproj files only list the .cs files the Editor knew at its last project sync. A new .cs
# file is reported as NOT-IN-CSPROJ and is compiled only by the next Editor job (utk editor refresh).
set -u
#   <this skill>/scripts/unity-compile.sh --selftest   (offline: a throwaway net10.0 project, one good and one bad build)
if [ "${1:-}" = --selftest ]; then
  command -v dotnet >/dev/null || { echo "unity-compile selftest SKIP: dotnet not installed"; exit 0; }
  T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
  mkdir -p "$T/Assets"; git -C "$T" init -q
  printf '<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Assets/A.cs" /></ItemGroup></Project>\n' > "$T/G.csproj"
  printf '<Solution><Project Path="G.csproj" /></Solution>\n' > "$T/G.slnx"
  echo 'class A { int F() => 1; }' > "$T/Assets/A.cs"
  "$0" "$T" | grep -q '^COMPILE OK$' || { echo "selftest FAIL: good build"; exit 1; }
  echo 'class A { int F() => "s"; }' > "$T/Assets/A.cs"; touch "$T/Assets/B.cs"
  O=$("$0" "$T"); [ $? = 1 ] || { echo "selftest FAIL: bad build exit"; exit 1; }
  echo "$O" | grep -q 'error CS0029' && echo "$O" | grep -q 'NOT-IN-CSPROJ Assets/B.cs' || { echo "selftest FAIL: output"; echo "$O"; exit 1; }
  [ ! -d "$T/Temp/unity-compile.lock.d" ] || { echo "selftest FAIL: lock left"; exit 1; }
  echo "unity-compile selftest OK"; exit 0
fi
R=${1:?usage: unity-compile.sh <game-repo>}
command -v dotnet >/dev/null || { echo "unity-compile: dotnet not found (brew install dotnet)"; exit 2; }
cd "$R" || exit 2
SLN=$(ls *.slnx *.sln 2>/dev/null | head -1)
[ -n "$SLN" ] || { echo "no .sln/.slnx in $R: open the project in the Editor once to generate it"; exit 2; }
LOCK=Temp/unity-compile.lock.d
mkdir -p Temp
for i in $(seq 1 120); do mkdir "$LOCK" 2>/dev/null && break; sleep 1; done
[ -d "$LOCK" ] || { echo "unity-compile: lock busy for 120 s ($LOCK)"; exit 2; }
trap 'rmdir "$LOCK" 2>/dev/null' EXIT
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
[ $rc = 0 ] && echo "COMPILE OK" || echo "COMPILE FAILED (exit $rc)"
exit $([ $rc = 0 ] && echo 0 || echo 1)
