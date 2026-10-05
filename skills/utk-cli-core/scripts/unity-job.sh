#!/bin/bash
# One Unity Editor job under the FIFO lock, with a wall-clock watchdog.
#   <this skill>/scripts/unity-job.sh <owner> <log> [--task ID] [--timeout S] [--kind compile|test|scene|shot|other] [--gate <repo>] -- <command...>
#   <this skill>/scripts/unity-job.sh --selftest
# Takes the Editor lock (UNITY_LOCK_NAME), turns utk autotick on, runs <command> with a
# wall-clock limit (default 900 s), turns autotick off, releases the lock, and writes <log>:
# the command output, then "EXIT <code>" and "RESULT: PASS|FAIL|TIMEOUT". With --task and
# UNITY_JOB_BOARD set (a command prefix, e.g. "python3 /path/board.py") it runs
# `$UNITY_JOB_BOARD status <task> wait-editor|doing` and `$UNITY_JOB_BOARD lockwait <task> <secs>`;
# unset = no board calls. Skips acquiring when $UNITY_LOCK_OWNER
# already holds the lock. UNITY_JOB_DRY=1 skips utk (selftest).
# No utk command closes a modal dialog (checked `utk list` 2026-10-04): a dialog ends as TIMEOUT.
# Run it with the Bash tool's run_in_background so the agent keeps coding while it waits.
# --gate <repo> runs `unity-test.sh offline <repo>` before queueing; a failing build exits 1 without taking the lock.
# UNITY_JOB_NO_TICK=1 leaves autotick alone: a batchmode job in a worktree lane must not
# toggle the main Editor's autotick under another agent's job.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
LOCK=$HERE/unity-lock.sh

selftest() {
  local tmp; tmp=$(mktemp -d)
  export UNITY_LOCK_DIR=$tmp/locks UNITY_LOCK_POLL=0.1 UNITY_LOCK_NAME=t UNITY_JOB_DRY=1 UTK_TELEMETRY="$tmp/telemetry.jsonl" UNITY_JOB_BOARD="$tmp/fakeboard"
  printf '#!/bin/bash\necho "$*" >> "%s/board.log"\n' "$tmp" > "$tmp/fakeboard"; chmod +x "$tmp/fakeboard"
  "$0" own "$tmp/ok.log" --task J1 -- echo hello >/dev/null
  grep -q '^RESULT: PASS$' "$tmp/ok.log" && grep -q hello "$tmp/ok.log" || { echo "FAIL: pass log"; exit 1; }
  [ "$(tr '\n' '|' < "$tmp/board.log" | sed -E 's/lockwait J1 [0-9]+/lockwait J1 N/')" = "status J1 wait-editor|lockwait J1 N|status J1 doing|" ] || { echo "FAIL: board calls: $(cat "$tmp/board.log")"; exit 1; }
  # telemetry: one JSON line per job, kind + wait + hold
  "$0" own "$tmp/tl.log" --kind test -- true >/dev/null
  line=$(tail -1 "$UTK_TELEMETRY")
  echo "$line" | grep -q '"kind":"test"' && echo "$line" | grep -q '"via":"job"' && echo "$line" | grep -q '"result":"PASS"' \
    || { echo "FAIL: telemetry line: $line"; exit 1; }
  "$0" own "$tmp/tl2.log" -- true >/dev/null
  tail -1 "$UTK_TELEMETRY" | grep -q '"kind":"other"' || { echo "FAIL: default kind"; exit 1; }
  # --gate: a failing offline build never reaches the lock
  printf '#!/bin/bash\necho "error CS0001: boom"; echo "COMPILE FAILED (exit 1)"; exit 1\n' > "$tmp/gate-bad"; chmod +x "$tmp/gate-bad"
  printf '#!/bin/bash\necho "COMPILE OK"\n' > "$tmp/gate-ok"; chmod +x "$tmp/gate-ok"
  UNITY_JOB_GATE_CMD="$tmp/gate-bad" "$0" own "$tmp/g1.log" --gate /nowhere -- true >"$tmp/g1.out"; rc=$?
  [ $rc = 1 ] && grep -q 'CS0001' "$tmp/g1.out" && [ ! -e "$tmp/g1.log" ] || { echo "FAIL: gate should block (rc=$rc)"; exit 1; }
  [ "$("$LOCK" status)" = free ] || { echo "FAIL: gate took the lock"; exit 1; }
  UNITY_JOB_GATE_CMD="$tmp/gate-ok" "$0" own "$tmp/g2.log" --gate /nowhere -- true >/dev/null || { echo "FAIL: gate should pass"; exit 1; }
  tail -1 "$UTK_TELEMETRY" | grep -q '"result":"PASS"' || { echo "FAIL: gated job telemetry"; exit 1; }
  "$0" own "$tmp/bad.log" -- false >/dev/null; [ $? = 1 ] && grep -q '^RESULT: FAIL$' "$tmp/bad.log" || { echo "FAIL: fail log"; exit 1; }
  "$0" own "$tmp/slow.log" --timeout 1 -- sleep 5 >/dev/null; [ $? = 124 ] || { echo "FAIL: timeout code"; exit 1; }
  grep -q '^RESULT: TIMEOUT$' "$tmp/slow.log" || { echo "FAIL: timeout log"; exit 1; }
  [ "$("$LOCK" status)" = free ] || { echo "FAIL: lock left held"; exit 1; }
  # a timeout stops the command's children too, not only the command
  "$0" own "$tmp/kids.log" --timeout 1 -- bash -c "(sleep 2; touch '$tmp/late') & wait" >/dev/null
  sleep 2.5; [ ! -e "$tmp/late" ] || { echo "FAIL: timed-out child kept running"; exit 1; }
  # a SIGKILLed job leaves a lock nobody keeps fresh (it goes stale instead of blocking forever)
  UNITY_JOB_TOUCH_S=0.2 "$0" own "$tmp/k9.log" -- sleep 30 >/dev/null & local pj=$!
  sleep 1; kill -KILL $pj; wait $pj 2>/dev/null; sleep 0.5; local m1; m1=$(stat -f %m "$tmp/locks/t.lock.d")
  sleep 1.5; [ "$(stat -f %m "$tmp/locks/t.lock.d")" = "$m1" ] || { echo "FAIL: orphan toucher keeps lock fresh"; exit 1; }
  pkill -f "sleep 30" 2>/dev/null; rm -rf "$tmp/locks/t.lock.d"
  rm -rf "$tmp"; echo "unity-job selftest OK"
}

[ "${1:-}" = --selftest ] && { selftest; exit 0; }
OWNER=${1:?owner} LOGF=${2:?log}; shift 2
TASK="" TO=900 KIND=other GATE=""
while [ $# -gt 0 ] && [ "$1" != -- ]; do
  case "$1" in --task) TASK=$2; shift 2 ;; --timeout) TO=$2; shift 2 ;; --kind) KIND=$2; shift 2 ;; --gate) GATE=$2; shift 2 ;; *) echo "unknown option $1"; exit 2 ;; esac
done
[ "${1:-}" = -- ] && shift
[ $# -gt 0 ] || { echo "usage: $0 <owner> <log> [--task ID] [--timeout S] [--kind compile|test|scene|shot|other] [--gate <repo>] -- <command...>"; exit 2; }
board() { [ -n "$TASK" ] && [ -n "${UNITY_JOB_BOARD:-}" ] && $UNITY_JOB_BOARD "$@" >/dev/null 2>&1; true; }
utk_tick() { [ "${UNITY_JOB_DRY:-}" = 1 ] || [ "${UNITY_JOB_NO_TICK:-}" = 1 ] || utk set_autotick --enable "$1" >/dev/null 2>&1 || true; }

# One agent's compile error blocks every agent on the Editor (11 builders, hours):
# a job that carries compile|test work proves the tree builds on plain .NET first.
if [ -n "$GATE" ]; then
  if [ -n "${UNITY_JOB_GATE_CMD:-}" ]; then read -ra gate_cmd <<< "$UNITY_JOB_GATE_CMD"; else gate_cmd=("$HERE/../../utk-test-runner/scripts/unity-test.sh" offline); fi
  if ! gout=$("${gate_cmd[@]}" "$GATE" 2>&1); then
    echo "$gout"; echo "GATE_FAILED: offline build did not pass (see output above); fix and resubmit - no Editor time used"; exit 1
  fi
fi

GOT=0
if [ "$("$LOCK" status | cut -d' ' -f1)" != "${UNITY_LOCK_OWNER:-}" ] || [ -z "${UNITY_LOCK_OWNER:-}" ]; then
  board status "$TASK" wait-editor
  t0=$SECONDS
  "$LOCK" acquire "$OWNER" 3600 >/dev/null || { echo "lock: $("$LOCK" status)"; exit 1; }
  GOT=1; WAIT_S=$((SECONDS - t0)); board lockwait "$TASK" $WAIT_S
fi
board status "$TASK" doing
HOLD0=$SECONDS; WAIT_S=${WAIT_S:-0}
# the toucher stops once this script is gone (SIGKILL skips the EXIT trap): the lock then goes stale
( while sleep "${UNITY_JOB_TOUCH_S:-240}"; kill -0 $$ 2>/dev/null; do "$LOCK" touch "$OWNER" >/dev/null 2>&1; done ) & TOUCHER=$!; disown $TOUCHER
cleanup() { kill $TOUCHER 2>/dev/null; utk_tick false; [ $GOT = 1 ] && "$LOCK" release "$OWNER" >/dev/null; }
trap cleanup EXIT

utk_tick true
mkdir -p "$(dirname "$LOGF")"
{ echo "# $(date '+%F %T') owner=$OWNER task=${TASK:-none} timeout=${TO}s"; echo "# \$ $*"; } > "$LOGF"
# The command runs in its own process group; on timeout the whole group gets TERM, then KILL,
# so a hung utk child cannot keep driving the Editor after the lock is released. 142 = timeout.
perl -e '$t = shift; $p = fork; die "fork: $!" unless defined $p;
  if (!$p) { setpgrp(0, 0); exec @ARGV or exit 127 }
  setpgrp($p, $p);
  $SIG{ALRM} = sub { kill "TERM", -$p; sleep 2; kill "KILL", -$p; waitpid($p, 0); exit 142 };
  alarm $t; waitpid($p, 0); exit($? & 127 ? 128 + ($? & 127) : $? >> 8)' "$TO" "$@" >> "$LOGF" 2>&1
code=$?
if [ $code = 142 ]; then r=TIMEOUT code=124; elif [ $code = 0 ]; then r=PASS; else r=FAIL; fi
printf 'EXIT %s\nRESULT: %s\n' "$code" "$r" >> "$LOGF"
# ponytail: flat JSONL, `utk queue stats` reads it; a real store when a week of lines gets slow
TL=${UTK_TELEMETRY:-$HOME/.unity-cli-agentkit/telemetry.jsonl}; mkdir -p "$(dirname "$TL")"
printf '{"ts":"%s","owner":"%s","task":"%s","kind":"%s","wait_s":%d,"hold_s":%d,"result":"%s","via":"job"}\n' \
  "$(date -u +%FT%TZ)" "$OWNER" "${TASK:-}" "$KIND" "${WAIT_S:-0}" "$((SECONDS - HOLD0))" "$r" >> "$TL"
echo "RESULT: $r ($LOGF)"
exit $code
