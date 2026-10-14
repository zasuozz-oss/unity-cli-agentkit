#!/bin/bash
# usage: SESSION=<PROFILE>-bg as_run.sh <functionName>
# Verify the selected function, click Run, poll the execution log until done or error.
: "${SESSION:?set SESSION to the headless work session, e.g. <PROFILE>-bg}"
AB="perl -e alarm(40);exec(@ARGV) agent-browser --session $SESSION"
FN=$($AB eval '(()=>{const l=document.querySelector("[aria-label=\"Select function to run\"],[aria-label=\"Chọn hàm để chạy\"]");return l&&l.innerText})()' | tail -1 | tr -d '"')
[ "$FN" != "$1" ] && { echo "WRONG FN: $FN (want $1)"; exit 1; }
$AB eval '(()=>{const b=[...document.querySelectorAll("button")].find(x=>/^(Run the selected function|Chạy hàm đã chọn)$/.test(x.getAttribute("aria-label")||""));b.click();return "run"})()' | tail -1
for i in $(seq 1 30); do
  sleep 3
  T=$($AB eval '(()=>{const t=document.body.innerText,i=Math.max(t.lastIndexOf("Execution started"),t.lastIndexOf("Đã bắt đầu quá trình thực thi"));if(i<0)return "...";const s=t.slice(i);return /Execution completed|Đã hoàn tất quá trình thực thi/.test(s)?"DONE\n"+s.slice(0,1500):(/\t(Error|Lỗi)\t/.test(s)?"ERROR\n"+s.slice(0,1500):"...")})()' | tail -1)
  [[ "$T" == \"DONE* || "$T" == \"ERROR* ]] && { echo -e "${T//\\n/\\n}"; exit 0; }
done
echo "TIMEOUT waiting for execution log"; exit 1
