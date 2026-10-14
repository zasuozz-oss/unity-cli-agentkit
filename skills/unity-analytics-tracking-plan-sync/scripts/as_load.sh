#!/bin/bash
# usage: SESSION=<PROFILE>-bg as_load.sh <file.gs>
# Put code into the Apps Script editor of the headless work session, save, and verify
# the editor holds exactly the file (stray keyboard/IME text gets prepended otherwise).
set -e
: "${SESSION:?set SESSION to the headless work session, e.g. <PROFILE>-bg}"
AB="perl -e alarm(40);exec(@ARGV) agent-browser --session $SESSION"
CODE=$(python3 -c 'import json,sys;print(json.dumps(open(sys.argv[1]).read()))' "$1")
$AB eval "monaco.editor.getModels()[0].setValue($CODE); 'set'" | tail -1
$AB eval "(()=>{const b=[...document.querySelectorAll('button')].find(x=>/^(Save project to Drive|Lưu dự án vào Drive)$/.test(x.getAttribute('aria-label')||''));if(!b)return 'NO SAVE BUTTON';b.click();return 'saved'})()" | tail -1
sleep 8
OK=$($AB eval "monaco.editor.getModels()[0].getValue()===$CODE" | tail -1)
[ "$OK" = "true" ] || { echo "EDITOR MISMATCH after save — do not run"; exit 1; }
echo "verified"
