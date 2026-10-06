#!/bin/bash
# Unity Editor lock, served first come first served. mkdir is atomic; a lock untouched for
# 15 min is stale, and a lock whose holder process is gone is free at once.
#   unity-lock.sh acquire <owner> [wait_s=540]  -> "acquired" (exit 0) | "TIMEOUT held by X" (exit 1)
#   unity-lock.sh touch|release <owner> ; status ; want <owner> ; queue
#   unity-lock.sh yield <owner>   between steps of a long job: if anyone waits, release, let them
#                                 run, then queue again (FIFO); else keep the lock ("kept")
#   unity-lock.sh --selftest
# The holder pid is UNITY_LOCK_PID, else the caller ($PPID): unity-job.sh exports its own pid so
# the lock lives as long as the job, not as long as whichever child called acquire.
# Each waiter drops a ticket <epoch>-<pid>-<owner> in <name>.queue/; the oldest live ticket
# goes next, and a ticket whose process is gone is dropped. `want` still jumps the queue.
# UNITY_LOCK_NAME picks the Editor (one lock per worktree Editor), UNITY_LOCK_DIR the folder
# (default ~/.unity-cli-agentkit/locks), UNITY_LOCK_POLL the poll interval in seconds (default 2).
set -u
N=${UNITY_LOCK_NAME:-unity-editor}
D=${UNITY_LOCK_DIR:-$HOME/.unity-cli-agentkit/locks}
L=$D/$N.lock.d W=$D/$N.want Q=$D/$N.queue
mkdir -p "$Q"

head_ticket() {
  local t pid
  for t in $(ls "$Q" | sort); do
    pid=$(echo "$t" | cut -d- -f2); pid=$((10#$pid))
    if kill -0 "$pid" 2>/dev/null; then echo "$t"; return; fi
    rm -f "$Q/$t"
  done
}

# holder_gone: the lock records a pid and that process no longer exists.
holder_gone() {
  local p; p=$(cat "$L/pid" 2>/dev/null) || return 1
  [ -n "$p" ] && ! kill -0 "$p" 2>/dev/null
}

selftest() {
  local tmp; tmp=$(mktemp -d)
  export UNITY_LOCK_DIR=$tmp UNITY_LOCK_POLL=0.1 UNITY_LOCK_NAME=t
  local me=$0
  "$me" acquire A 5 >/dev/null || { echo "FAIL: A"; exit 1; }
  "$me" acquire B 10 > "$tmp/b.out" & local pb=$!
  sleep 0.5
  "$me" acquire C 10 > "$tmp/c.out" & local pc=$!
  sleep 0.5
  [ "$("$me" queue | wc -l | tr -d ' ')" = 2 ] || { echo "FAIL: queue"; "$me" queue; exit 1; }
  : > "$tmp/t.queue/0000000001-0999999999-ghost"           # dead waiter must not block
  "$me" release A >/dev/null
  wait $pb; [ "$(cut -d' ' -f1 < "$tmp/t.lock.d/owner")" = B ] || { echo "FAIL: B not next"; exit 1; }
  "$me" release B >/dev/null
  wait $pc; [ "$(cut -d' ' -f1 < "$tmp/t.lock.d/owner")" = C ] || { echo "FAIL: C not next"; exit 1; }
  "$me" release C >/dev/null
  [ "$("$me" status)" = free ] || { echo "FAIL: not free"; exit 1; }
  # a lock whose holder died is free at once, not after 15 min
  sleep 60 & local ph=$!
  UNITY_LOCK_PID=$ph "$me" acquire D 5 >/dev/null; [ "$("$me" status | cut -d' ' -f1)" = D ] || { echo "FAIL: D"; exit 1; }
  kill $ph; wait $ph 2>/dev/null
  "$me" acquire E 3 2>/dev/null | grep -q "holder pid .* is gone" || { echo "FAIL: dead holder not freed"; exit 1; }
  [ "$(cut -d' ' -f1 < "$tmp/t.lock.d/owner")" = E ] || { echo "FAIL: E did not take the freed lock"; exit 1; }
  # yield: nobody waits -> kept; a waiter -> it runs, then the yielder has the lock again
  [ "$(UNITY_LOCK_PID=$$ "$me" yield E)" = kept ] || { echo "FAIL: yield with no waiter"; exit 1; }
  ( "$me" acquire F 10 >/dev/null; sleep 0.5; "$me" release F >/dev/null ) & local pf=$!
  sleep 0.5
  UNITY_LOCK_PID=$$ "$me" yield E 2>/dev/null | grep -q acquired || { echo "FAIL: yield did not come back"; exit 1; }
  wait $pf; [ "$(cut -d' ' -f1 < "$tmp/t.lock.d/owner")" = E ] || { echo "FAIL: E not back after yield"; exit 1; }
  "$me" release E >/dev/null
  rm -rf "$tmp"; echo "unity-lock selftest OK"
}

case "${1:-}" in
--selftest) selftest ;;
acquire)
  OWNER=${2:?owner}; end=$((SECONDS + ${3:-540}))
  T=$(printf '%010d-%010d-%s' "$(date +%s)" $$ "$OWNER"); : > "$Q/$T"
  trap 'rm -f "$Q/$T"' EXIT
  while true; do
    want=$(cat "$W" 2>/dev/null)
    if [ -n "$want" ] && [ "$want" != "$OWNER" ] && [ -z "$(find "$W" -maxdepth 0 -mmin +30 2>/dev/null)" ]; then :
    elif { [ "$want" = "$OWNER" ] || [ "$(head_ticket)" = "$T" ]; } && mkdir "$L" 2>/dev/null; then break; fi
    if [ -d "$L" ] && holder_gone; then
      echo "lock of $(cat "$L/owner" 2>/dev/null) removed: holder pid $(cat "$L/pid") is gone"; rm -rf "$L"; continue; fi
    if [ -d "$L" ] && [ -n "$(find "$L" -maxdepth 0 -mmin +15 2>/dev/null)" ]; then
      echo "stale lock of $(cat "$L/owner" 2>/dev/null) removed"; rm -rf "$L"; continue; fi
    [ $SECONDS -ge $end ] && { echo "TIMEOUT held by $(cat "$L/owner" 2>/dev/null)"; exit 1; }
    sleep "${UNITY_LOCK_POLL:-2}"
  done
  [ "$want" = "$OWNER" ] && rm -f "$W"
  echo "$OWNER $(date +%H:%M:%S)" > "$L/owner"; echo "${UNITY_LOCK_PID:-$PPID}" > "$L/pid"; echo acquired ;;
yield)
  OWNER=${2:?owner}
  [ "$(cut -d' ' -f1 "$L/owner" 2>/dev/null)" = "$OWNER" ] || { echo "not owner"; exit 1; }
  [ -n "$(head_ticket)" ] || { echo kept; exit 0; }
  rm -rf "$L"; echo "yielded to $(head_ticket | cut -d- -f3-)"
  UNITY_LOCK_PID=${UNITY_LOCK_PID:-$PPID} exec "$0" acquire "$OWNER" "${3:-3600}" ;;
want) echo "$2" > "$W"; echo "priority set for $2" ;;
touch) [ "$(cut -d' ' -f1 "$L/owner" 2>/dev/null)" = "$2" ] && touch "$L" && echo touched || { echo "not owner"; exit 1; } ;;
release) if [ "$(cut -d' ' -f1 "$L/owner" 2>/dev/null)" = "$2" ]; then rm -rf "$L"; echo released; else echo "not owner ($(cat "$L/owner" 2>/dev/null))"; exit 1; fi ;;
status) if [ -d "$L" ] && holder_gone; then echo "free (holder $(cut -d' ' -f1 "$L/owner") pid $(cat "$L/pid") gone)"
        else o=$(cat "$L/owner" 2>/dev/null) && echo "$o$( [ -f "$L/pid" ] && echo " pid $(cat "$L/pid")")" || echo free; fi ;;
queue) i=0; for t in $(ls "$Q" | sort); do i=$((i+1)); echo "$i ${t#*-*-} waited $(( $(date +%s) - 10#${t%%-*} ))s"; done ;;
*) echo "usage: $0 acquire|touch|release|yield <owner> | status | want <owner> | queue | --selftest"; exit 2 ;;
esac
