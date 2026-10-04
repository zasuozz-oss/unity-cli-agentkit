#!/usr/bin/env bash
# PreToolUse(Bash) hook of the unity-cli-agentkit plugin: refuses a
# `utk run_tests` (or `unity command run_tests`, which skips utk's own
# check) that would run the whole suite (no --filter, an assembly
# filter, or a `<Game>.Tests` namespace). 1000+ tests hold the shared Editor
# for minutes and every other agent on it waits. `UTK_FULL_SUITE=1 utk
# run_tests ...` lets the full run through when the user asked for it.
set -u
cmd=$(jq -r '.tool_input.command // empty' 2>/dev/null) || exit 0
[ -n "$cmd" ] || exit 0
reason=""
# One segment per command, so a filtered run does not cover an unfiltered one.
for sep in '&&' '||' ';' '|'; do cmd=${cmd//"$sep"/$'\n'}; done
while IFS= read -r seg; do
  [[ $seg =~ (^|[/[:space:]])(utk|unity[[:space:]]+command([[:space:]]+[^[:space:]]+)*)[[:space:]]+run_tests($|[[:space:]]) ]] || continue
  [[ $seg == *UTK_FULL_SUITE=1* ]] && continue
  filter=""
  [[ $seg =~ --filter[=[:space:]]+[\"\']?([^\"\'[:space:]]+) ]] && filter=${BASH_REMATCH[1]}
  if [ -z "$filter" ]; then
    reason="no --filter: that is the whole suite"
  elif [[ $seg =~ --filter_type[=[:space:]]+[\"\']?[Aa]ssembly ]]; then
    reason="--filter_type assembly runs the whole test assembly"
  elif [[ $filter =~ \.Tests(\.[A-Za-z]+Mode)?\.?$ ]]; then
    reason="--filter $filter is a namespace and matches every test"
  fi
  [ -n "$reason" ] && break
done <<< "$cmd"
[ -n "$reason" ] || exit 0
msg="Blocked run_tests ($reason). It holds the shared Unity Editor for minutes and every other agent waits behind it. Run only your own tests: first \`unity-test.sh offline <repo> --tests <YourTestClass>\` (utk-test-runner skill, no Editor), then \`utk run_tests --mode editor --filter <YourTestClass>\`. Run the full suite only when the user asked for it: prefix the command with UTK_FULL_SUITE=1."
msg="${msg//\\/\\\\}"
msg="${msg//\"/\\\"}"
printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}\n' "$msg"
