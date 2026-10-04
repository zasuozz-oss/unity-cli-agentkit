#!/usr/bin/env bash
# Self-check for guard-tests.sh: bash hooks/guard-tests.test.sh
cd "$(dirname "$0")"
fail=0
check() { # <want: deny|allow> <command>
  got=allow
  jq -n --arg c "$2" '{tool_input:{command:$c}}' | bash guard-tests.sh | grep -q '"deny"' && got=deny
  [ "$got" = "$1" ] || { echo "FAIL want $1: $2"; fail=1; }
}
check deny  'utk run_tests'
check deny  'utk run_tests --mode editor'
check deny  'utk editor refresh && ~/.unity-cli-agentkit/bin/utk run_tests --mode playmode --timeout 600'
check deny  'utk run_tests --filter EchoPals.Tests'
check deny  'utk run_tests --filter "EchoPals.Tests.EditMode"'
check deny  'utk run_tests --filter_type assembly --filter EchoPals.Tests.EditMode'
check deny  'utk run_tests --filter A; utk run_tests'
check allow 'utk run_tests --mode editor --filter CatchGameTests'
check allow 'utk run_tests --filter=CatchGameTests --timeout 300'
check allow "utk run_tests --filter 'EchoPals.Tests.CatchGameTests'"
check allow 'UTK_FULL_SUITE=1 utk run_tests --mode editor'
check deny  'unity command run_tests --mode editor --json'
check deny  'unity command --project-path /p run_tests --filter EchoPals.Tests'
check allow 'unity command run_tests --mode editor --filter CatchGameTests'
check allow 'utk list_tests'
check allow 'grep run_tests skills/utk-test-runner/SKILL.md'
[ $fail = 0 ] && echo "guard-tests: ok"
exit $fail
