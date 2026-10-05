# Editor Queue Coordinator — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Nhiều agent dùng chung một Unity Editor trả chi phí compile / reload / vào Play **một lần mỗi chu kỳ** thay vì mỗi agent một lần, lỗi compile của một agent không chặn agent khác, job ngắn không chờ sau job dài, và có số liệu chờ/giữ theo loại job.

**Architecture:** Một coordinator Go (`utk queue serve`, một process mỗi Editor) đọc request JSON từ `~/.unity-cli-agentkit/queue/<editor>/requests/`, mỗi chu kỳ: cổng `dotnet build` offline → lấy lock `unity-lock.sh` → một `refresh` → một `run_tests` gộp filter rồi tách kết quả → scene job tuần tự ngắn-trước → một phiên Play cho mọi screenshot → nhả lock → ghi `results/` + `telemetry.jsonl`. Agent gọi `utk queue submit <kind>` (chặn tới khi có kết quả của mình); việc không cần hàng đợi (sửa prefab, query) gọi thẳng `utk exec`. `unity-job.sh` hiện có nhận thêm `--kind`/`--gate` và ghi cùng telemetry để chuyển dần.

**Tech Stack:** Go 1.26 stdlib (không thêm dependency — `setup-cli.sh` build không tải module), bash (`unity-lock.sh`, `unity-job.sh`, `unity-test.sh`), `utk` gọi chính nó qua `os.Executable()`.

**Spec:** `docs/superpowers/specs/2026-10-05-editor-queue-coordinator-design.md`.

## Global Constraints

- Go stdlib only; `go vet ./... && go test ./...` xanh sau mỗi task.
- Không commit / branch trừ khi user yêu cầu (quy ước CLAUDE.md của user). Các bước "Commit" trong template bị bỏ.
- Mọi lệnh có timeout hữu hạn; không `for … sleep` chờ Editor ngoài các vòng poll có deadline.
- Coordinator gọi Editor **chỉ qua `utk`** (subprocess `os.Executable()`), không gọi `unity` trực tiếp — để tái dùng `pollRecompile`, `pollTests`, `CaptureRetry`.
- Lệnh gọi subprocess trong coordinator đi qua một biến hàm `queueExec` để test thay bằng fake; không test nào cần Editor thật trừ Task 13–14 (đánh dấu MANUAL).
- Tên Editor = `UNITY_LOCK_NAME` (mặc định `unity-editor`); queue dir = `$UTK_QUEUE_DIR` hoặc `~/.unity-cli-agentkit/queue/<UNITY_LOCK_NAME>`; skills dir = `$UTK_SKILLS_DIR` hoặc `~/.unity-cli-agentkit/skills`; telemetry = `$UTK_TELEMETRY` hoặc `~/.unity-cli-agentkit/telemetry.jsonl`.
- Telemetry một dòng JSON: `{"ts","owner","task","kind","wait_s","hold_s","result","via"}`; `via` = `job` (unity-job.sh) hoặc `queue`.
- Status của result: `PASS|FAIL|TIMEOUT|GATE_FAILED|NO_TESTS_MATCHED|EDITOR_BLOCKED`.
- Comment trong code: tiếng Anh, giọng như code hiện có (giải thích *vì sao*, dẫn sự cố).

## File Structure

| File | Trách nhiệm |
|---|---|
| `skills/utk-cli-core/scripts/unity-job.sh` (sửa) | `--kind`, `--gate <repo>`, ghi telemetry; selftest thêm case |
| `cmd/utk/tests.go` (sửa) | `splitTestFilter`; `foreignTest` hiểu filter `A;B` |
| `cmd/utk/telemetry.go` (mới) | `telemetryPath()`, `appendTelemetry()`, `telemetryStats()` dùng chung cho `queue stats` |
| `cmd/utk/queue.go` (mới) | Kiểu `queueRequest`/`queueResult`/`queueArgs`, thư mục, đọc/ghi atomic, `pidAlive`, dispatch `runQueue` (submit/status/stats/serve) |
| `cmd/utk/queue_cycle.go` (mới) | `queueExec` var, `runCycle` và 4 pha: gate+refresh, tests, scene, shot |
| `cmd/utk/queue_proc_unix.go` / `queue_proc_windows.go` (mới) | `runWithTimeout`: process group + TERM→KILL (unix); bản windows chỉ `cmd.Process.Kill()` |
| `cmd/utk/queue_serve.go` (mới) | Vòng `serve`: `serve.lock.d`, `serve.pid`, EDITOR_BLOCKED, lock acquire/release, ghi result + telemetry; `ensureServe` spawn từ `submit` |
| `cmd/utk/queue_test.go`, `telemetry_test.go` (mới) | Test với fake `queueExec` |
| `cmd/utk/main.go`, `cmd/utk/usage.go` (sửa) | Route `utk queue …`, usage |
| `skills/utk-editor-queue/SKILL.md` (mới) | Khi nào gọi thẳng, khi nào `submit`; 4 kind; giới hạn static state |
| `skills/utk-cli-core/SKILL.md`, `skills/utk-test-runner/SKILL.md`, `skills/utk-playmode-driving/SKILL.md`, `internal/initcmd/agents-block.md`, `README.md` (sửa) | Trỏ sang queue; `--gate`/`--kind` |

---

### Task 1: `unity-job.sh --kind` và telemetry

**Files:**
- Modify: `skills/utk-cli-core/scripts/unity-job.sh`

**Interfaces:**
- Produces: dòng telemetry JSON `{"ts":"<RFC3339>","owner":"…","task":"…","kind":"compile|test|scene|shot|other","wait_s":N,"hold_s":N,"result":"PASS|FAIL|TIMEOUT","via":"job"}` tại `${UTK_TELEMETRY:-$HOME/.unity-cli-agentkit/telemetry.jsonl}`. Task 3 đọc đúng schema này.

- [ ] **Step 1: Thêm case selftest (fail trước)**

Trong hàm `selftest()` của `unity-job.sh`, sau dòng kiểm tra `board calls`, thêm:

```bash
  # telemetry: one JSON line per job, kind + wait + hold
  export UTK_TELEMETRY="$tmp/telemetry.jsonl"
  "$0" own "$tmp/tl.log" --kind test -- true >/dev/null
  line=$(tail -1 "$UTK_TELEMETRY")
  echo "$line" | grep -q '"kind":"test"' && echo "$line" | grep -q '"via":"job"' && echo "$line" | grep -q '"result":"PASS"' \
    || { echo "FAIL: telemetry line: $line"; exit 1; }
  "$0" own "$tmp/tl2.log" -- true >/dev/null
  tail -1 "$UTK_TELEMETRY" | grep -q '"kind":"other"' || { echo "FAIL: default kind"; exit 1; }
```

- [ ] **Step 2: Chạy selftest, xác nhận fail**

Run: `bash skills/utk-cli-core/scripts/unity-job.sh --selftest`
Expected: `unknown option --kind` hoặc `FAIL: telemetry line:`.

- [ ] **Step 3: Cài đặt**

Trong vòng parse option, thêm `--kind`:

```bash
TASK="" TO=900 KIND=other
while [ $# -gt 0 ] && [ "$1" != -- ]; do
  case "$1" in --task) TASK=$2; shift 2 ;; --timeout) TO=$2; shift 2 ;; --kind) KIND=$2; shift 2 ;; *) echo "unknown option $1"; exit 2 ;; esac
done
```

Sau `board status "$TASK" doing` ghi nhớ mốc bắt đầu giữ lock: `HOLD0=$SECONDS; WAIT_S=${WAIT_S:-0}` — và trong nhánh acquire đổi `board lockwait "$TASK" $((SECONDS - t0))` thành:

```bash
  GOT=1; WAIT_S=$((SECONDS - t0)); board lockwait "$TASK" $WAIT_S
```

Ngay trước `echo "RESULT: $r ($LOGF)"` thêm:

```bash
# ponytail: flat JSONL, `utk queue stats` reads it; a real store when a week of lines gets slow
TL=${UTK_TELEMETRY:-$HOME/.unity-cli-agentkit/telemetry.jsonl}; mkdir -p "$(dirname "$TL")"
printf '{"ts":"%s","owner":"%s","task":"%s","kind":"%s","wait_s":%d,"hold_s":%d,"result":"%s","via":"job"}\n' \
  "$(date -u +%FT%TZ)" "$OWNER" "${TASK:-}" "$KIND" "${WAIT_S:-0}" "$((SECONDS - HOLD0))" "$r" >> "$TL"
```

Cập nhật usage string ở đầu file và dòng `usage:` : `[--task ID] [--timeout S] [--kind compile|test|scene|shot|other]`.

- [ ] **Step 4: Chạy selftest, xác nhận pass**

Run: `bash skills/utk-cli-core/scripts/unity-job.sh --selftest`
Expected: `unity-job selftest OK`.

---

### Task 2: `unity-job.sh --gate <repo>`

**Files:**
- Modify: `skills/utk-cli-core/scripts/unity-job.sh`

**Interfaces:**
- Consumes: `skills/utk-test-runner/scripts/unity-test.sh offline <repo>` in `COMPILE OK` khi build xanh, exit 1 khi lỗi. `UNITY_JOB_GATE_CMD` ghi đè lệnh gate (selftest).

- [ ] **Step 1: Selftest (fail trước)**

Thêm vào `selftest()`:

```bash
  # --gate: a failing offline build never reaches the lock
  printf '#!/bin/bash\necho "error CS0001: boom"; echo "COMPILE FAILED (exit 1)"; exit 1\n' > "$tmp/gate-bad"; chmod +x "$tmp/gate-bad"
  printf '#!/bin/bash\necho "COMPILE OK"\n' > "$tmp/gate-ok"; chmod +x "$tmp/gate-ok"
  UNITY_JOB_GATE_CMD="$tmp/gate-bad" "$0" own "$tmp/g1.log" --gate /nowhere -- true >"$tmp/g1.out"; rc=$?
  [ $rc = 1 ] && grep -q 'CS0001' "$tmp/g1.out" && [ ! -e "$tmp/g1.log" ] || { echo "FAIL: gate should block (rc=$rc)"; exit 1; }
  [ "$("$LOCK" status)" = free ] || { echo "FAIL: gate took the lock"; exit 1; }
  UNITY_JOB_GATE_CMD="$tmp/gate-ok" "$0" own "$tmp/g2.log" --gate /nowhere -- true >/dev/null || { echo "FAIL: gate should pass"; exit 1; }
  tail -1 "$UTK_TELEMETRY" | grep -q '"result":"PASS"' || { echo "FAIL: gated job telemetry"; exit 1; }
```

- [ ] **Step 2: Chạy, xác nhận fail**

Run: `bash skills/utk-cli-core/scripts/unity-job.sh --selftest` → `unknown option --gate`.

- [ ] **Step 3: Cài đặt**

Option parse: `--gate) GATE=$2; shift 2 ;;` (khởi tạo `GATE=""`). Sau khi parse xong và trước `GOT=0`:

```bash
# One agent's compile error blocks every agent on the Editor (11 builders, hours):
# a job that carries compile|test work proves the tree builds on plain .NET first.
if [ -n "$GATE" ]; then
  gate_cmd=${UNITY_JOB_GATE_CMD:-"$HERE/../../utk-test-runner/scripts/unity-test.sh offline"}
  if ! gout=$($gate_cmd "$GATE" 2>&1); then
    echo "$gout"; echo "GATE_FAILED: fix the compile errors above, then resubmit (no Editor time used)"; exit 1
  fi
fi
```

Thêm vào header comment: `--gate <repo>` chạy `unity-test.sh offline <repo>` trước khi xếp hàng.

- [ ] **Step 4: Chạy selftest, xác nhận pass**

Run: `bash skills/utk-cli-core/scripts/unity-job.sh --selftest` → `unity-job selftest OK`.

---

### Task 3: `telemetry.go` + `utk queue stats`

**Files:**
- Create: `cmd/utk/telemetry.go`, `cmd/utk/telemetry_test.go`

**Interfaces:**
- Produces:
  ```go
  type telemetryEntry struct {
      TS string `json:"ts"`; Owner string `json:"owner"`; Task string `json:"task"`; Kind string `json:"kind"`
      WaitS int `json:"wait_s"`; HoldS int `json:"hold_s"`; Result string `json:"result"`; Via string `json:"via"`
  }
  func telemetryPath() string                         // $UTK_TELEMETRY or ~/.unity-cli-agentkit/telemetry.jsonl
  func appendTelemetry(e telemetryEntry) error        // one JSON line, mkdir -p
  func telemetryStats(r io.Reader, since time.Time) string // table per kind: n, p50/p95 wait, p50/p95 hold, gate-blocked count
  ```
  Task 10 gọi `appendTelemetry`; Task 11 route `utk queue stats`.

- [ ] **Step 1: Test (fail trước)**

```go
package main

import (
	"strings"
	"testing"
	"time"
)

func TestTelemetryStats(t *testing.T) {
	lines := `{"ts":"2026-10-05T01:00:00Z","owner":"a","task":"","kind":"test","wait_s":10,"hold_s":100,"result":"PASS","via":"job"}
{"ts":"2026-10-05T01:05:00Z","owner":"b","task":"","kind":"test","wait_s":30,"hold_s":120,"result":"FAIL","via":"queue"}
{"ts":"2026-10-05T01:06:00Z","owner":"c","task":"","kind":"shot","wait_s":600,"hold_s":20,"result":"PASS","via":"job"}
{"ts":"2026-10-05T01:07:00Z","owner":"d","task":"","kind":"compile","wait_s":0,"hold_s":0,"result":"GATE_FAILED","via":"queue"}
{"ts":"2026-09-01T00:00:00Z","owner":"old","task":"","kind":"test","wait_s":9999,"hold_s":9999,"result":"PASS","via":"job"}
not json`
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out := telemetryStats(strings.NewReader(lines), since)
	for _, want := range []string{"test", "n=2", "shot", "wait p50=600", "gate blocked: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "9999") {
		t.Errorf("entry before --since counted:\n%s", out)
	}
}

func TestAppendTelemetryRoundTrip(t *testing.T) {
	t.Setenv("UTK_TELEMETRY", t.TempDir()+"/sub/t.jsonl")
	if err := appendTelemetry(telemetryEntry{TS: "2026-10-05T01:00:00Z", Kind: "scene", HoldS: 5, Result: "PASS", Via: "queue"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(telemetryPath())
	if !strings.Contains(string(b), `"kind":"scene"`) || !strings.HasSuffix(string(b), "\n") {
		t.Fatalf("bad line: %q", b)
	}
}
```
(thêm `"os"` vào import.)

- [ ] **Step 2: Chạy, xác nhận fail**

Run: `go test ./cmd/utk -run 'TestTelemetry|TestAppendTelemetry' -v` → `undefined: telemetryStats`.

- [ ] **Step 3: Cài đặt**

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// telemetryEntry is one lock-wait/hold record. unity-job.sh writes the same
// shape (via "job"); the coordinator writes via "queue". Both land in one
// file so `utk queue stats` sees the whole Editor, not one path through it.
type telemetryEntry struct {
	TS     string `json:"ts"`
	Owner  string `json:"owner"`
	Task   string `json:"task"`
	Kind   string `json:"kind"`
	WaitS  int    `json:"wait_s"`
	HoldS  int    `json:"hold_s"`
	Result string `json:"result"`
	Via    string `json:"via"`
}

func kitHome() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".unity-cli-agentkit")
}

func telemetryPath() string {
	if p := os.Getenv("UTK_TELEMETRY"); p != "" {
		return p
	}
	return filepath.Join(kitHome(), "telemetry.jsonl")
}

func appendTelemetry(e telemetryEntry) error {
	p := telemetryPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, err = f.Write(append(b, '\n'))
	return err
}

func percentile(xs []int, p float64) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	i := int(float64(len(s)-1) * p)
	return s[i]
}

// telemetryStats renders per-kind wait/hold percentiles since a point in time.
// Unparseable lines are skipped: a half-written line from a killed job must
// not hide a week of good ones.
func telemetryStats(r io.Reader, since time.Time) string {
	type agg struct{ wait, hold []int }
	kinds := map[string]*agg{}
	gate := 0
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var e telemetryEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil || ts.Before(since) {
			continue
		}
		if e.Result == "GATE_FAILED" {
			gate++
			continue
		}
		a := kinds[e.Kind]
		if a == nil {
			a = &agg{}
			kinds[e.Kind] = a
		}
		a.wait = append(a.wait, e.WaitS)
		a.hold = append(a.hold, e.HoldS)
	}
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "since %s\n", since.Format(time.RFC3339))
	for _, k := range names {
		a := kinds[k]
		fmt.Fprintf(&b, "%-8s n=%d  wait p50=%ds p95=%ds  hold p50=%ds p95=%ds\n", k, len(a.wait),
			percentile(a.wait, .5), percentile(a.wait, .95), percentile(a.hold, .5), percentile(a.hold, .95))
	}
	fmt.Fprintf(&b, "gate blocked: %d\n", gate)
	return b.String()
}
```

- [ ] **Step 4: Chạy, xác nhận pass**

Run: `go test ./cmd/utk -run 'TestTelemetry|TestAppendTelemetry' -v` → PASS.

---

### Task 4: `foreignTest` hiểu filter gộp `A;B`

**Files:**
- Modify: `cmd/utk/tests.go:167-182`
- Test: `cmd/utk/tests_test.go`

**Interfaces:**
- Produces: `func splitTestFilter(filter string) []string` — tách theo `;`, trim, bỏ rỗng, lowercase. Task 7 dùng để tách report theo agent.

- [ ] **Step 1: Test (fail trước)**

Thêm vào `TestForeignTest` cases:

```go
		{"merged filter, both ours", "MineTests;OtherTests", "", ""},
		{"merged filter, one foreign", "MineTests;Nope", "", "G.OtherTests.Two"},
```

Và test mới:

```go
func TestSplitTestFilter(t *testing.T) {
	got := splitTestFilter(" A ;b;;C")
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("splitTestFilter = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Chạy, xác nhận fail**

Run: `go test ./cmd/utk -run 'TestForeignTest|TestSplitTestFilter' -v` → fail (`undefined: splitTestFilter`, và case merged báo foreign).

- [ ] **Step 3: Cài đặt**

```go
// splitTestFilter breaks a Unity Test Framework name filter ("A;B", as the
// framework's -testFilter takes it) into lowercase parts. The coordinator
// merges every agent's filter into one run this way, so a report may
// legitimately hold tests from several parts.
func splitTestFilter(filter string) []string {
	var out []string
	for _, p := range strings.Split(filter, ";") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

Trong `foreignTest`, thay `f := strings.ToLower(filter)` và vòng lặp bằng:

```go
	parts := splitTestFilter(filter)
	for _, r := range d.Results {
		name := strings.ToLower(r.FullName)
		ours := false
		for _, p := range parts {
			if strings.Contains(name, p) {
				ours = true
				break
			}
		}
		if !ours {
			return r.FullName
		}
	}
	return ""
```

- [ ] **Step 4: Chạy, xác nhận pass**

Run: `go test ./cmd/utk -v` → PASS toàn bộ.

---

### Task 5: `queue.go` — kiểu dữ liệu, thư mục, đọc/ghi atomic, `submit`/`status`

**Files:**
- Create: `cmd/utk/queue.go`, `cmd/utk/queue_test.go`

**Interfaces:**
- Produces:
  ```go
  type queueArgs struct {
      Filter string `json:"filter,omitempty"`; Mode string `json:"mode,omitempty"`
      Cmd []string `json:"cmd,omitempty"`; Script string `json:"script,omitempty"`
      EstS int `json:"est_s,omitempty"`; TimeoutS int `json:"timeout_s,omitempty"`
      Scene string `json:"scene,omitempty"`; Isolated bool `json:"isolated,omitempty"`; Edit bool `json:"edit,omitempty"`
  }
  type queueRequest struct { ID, Kind, Agent string; PID int; Cwd string; Submitted time.Time; Args queueArgs }
  type queueResult struct { ID, Status string; Exit int; Stdout string; Artifacts []string; WaitS, RunS int }
  func queueDir() string
  func newRequestID(now time.Time) string               // "20061005-143012-<4 hex>"
  func writeRequest(dir string, r queueRequest) error   // requests/<id>.json via .tmp + rename
  func readRequests(dir string) ([]queueRequest, error) // sorted by ID; drops (and deletes) dead-pid requests
  func writeResult(dir string, r queueResult) error     // results/<id>.json via .tmp + rename
  func readResult(dir, id string) (queueResult, bool)
  func pidAlive(pid int) bool
  func parseSubmit(args []string, now time.Time) (queueRequest, int, error) // returns waitS (--wait, default 3600)
  func runQueue(args []string, stdout, stderr io.Writer) int               // submit|status|stats|serve
  ```
  `serve` và `ensureServe` được cài ở Task 10; ở task này `runQueue("serve")` trả `2` với thông báo "not implemented yet" để biên dịch được.

- [ ] **Step 1: Test (fail trước)**

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueueRequestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := queueRequest{ID: "20261005-100000-aaaa", Kind: "test", Agent: "tab1", PID: os.Getpid(), Cwd: "/p",
		Submitted: time.Now(), Args: queueArgs{Filter: "MineTests", Mode: "editmode"}}
	if err := writeRequest(dir, r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", r.ID+".json.tmp")); err == nil {
		t.Fatal("tmp file left behind")
	}
	got, err := readRequests(dir)
	if err != nil || len(got) != 1 || got[0].Args.Filter != "MineTests" || got[0].Kind != "test" {
		t.Fatalf("readRequests = %+v, %v", got, err)
	}
}

// A request whose submitter died must not cost Editor time (unity-lock.sh drops
// dead tickets the same way).
func TestReadRequestsDropsDeadPid(t *testing.T) {
	dir := t.TempDir()
	writeRequest(dir, queueRequest{ID: "20261005-100000-dead", Kind: "compile", PID: 999999999})
	writeRequest(dir, queueRequest{ID: "20261005-100001-live", Kind: "compile", PID: os.Getpid()})
	got, _ := readRequests(dir)
	if len(got) != 1 || got[0].ID != "20261005-100001-live" {
		t.Fatalf("got %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", "20261005-100000-dead.json")); err == nil {
		t.Fatal("dead request not deleted")
	}
}

func TestParseSubmit(t *testing.T) {
	now := time.Date(2026, 10, 5, 14, 30, 12, 0, time.UTC)
	r, wait, err := parseSubmit([]string{"test", "--filter", "MineTests", "--agent", "tab3", "--wait", "120"}, now)
	if err != nil || r.Kind != "test" || r.Args.Filter != "MineTests" || r.Agent != "tab3" || wait != 120 {
		t.Fatalf("%+v wait=%d err=%v", r, wait, err)
	}
	if !strings.HasPrefix(r.ID, "20261005-143012-") || r.Args.Mode != "editmode" {
		t.Fatalf("id/mode defaults: %+v", r)
	}
	for _, bad := range [][]string{
		{"test"},                              // filter required
		{"shot", "--script", "s.sh"},          // scene required
		{"scene"},                             // cmd or script required
		{"dance"},                             // unknown kind
		{"test", "--filter", "Game.Tests"},    // whole suite (namespace)
	} {
		if _, _, err := parseSubmit(bad, now); err == nil {
			t.Errorf("parseSubmit(%v) accepted", bad)
		}
	}
	r, _, _ = parseSubmit([]string{"scene", "--cmd", "exec --file x.cs", "--est-seconds", "5"}, now)
	if r.Args.EstS != 5 || len(r.Args.Cmd) != 3 || r.Args.TimeoutS != 300 {
		t.Fatalf("scene args: %+v", r.Args)
	}
}

func TestQueueStatusLists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_QUEUE_DIR", dir)
	writeRequest(dir, queueRequest{ID: "20261005-100000-aaaa", Kind: "test", Agent: "tab1", PID: os.Getpid(), Args: queueArgs{Filter: "A"}})
	var out, errb bytes.Buffer
	if code := runQueue([]string{"status"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if !strings.Contains(out.String(), "tab1") || !strings.Contains(out.String(), "test A") {
		t.Fatalf("status output: %s", out.String())
	}
}
```

- [ ] **Step 2: Chạy, xác nhận fail**

Run: `go test ./cmd/utk -run 'TestQueue|TestParseSubmit|TestReadRequests' -v` → `undefined: queueRequest`.

- [ ] **Step 3: Cài đặt `queue.go`**

```go
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// queueArgs is the per-kind payload of a request (see the spec, §5).
type queueArgs struct {
	Filter   string   `json:"filter,omitempty"`    // test: testName filter, "A;B" allowed
	Mode     string   `json:"mode,omitempty"`      // test: editmode|playmode
	Cmd      []string `json:"cmd,omitempty"`       // scene: utk argv
	Script   string   `json:"script,omitempty"`    // scene|shot: bash script path
	EstS     int      `json:"est_s,omitempty"`     // scene: shortest-job-first key
	TimeoutS int      `json:"timeout_s,omitempty"` // scene|shot: wall clock per job
	Scene    string   `json:"scene,omitempty"`     // shot: scene path to (re)load before the script
	Isolated bool     `json:"isolated,omitempty"`  // shot: own Play session
	Edit     bool     `json:"edit,omitempty"`      // shot: no Play at all
}

type queueRequest struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Agent     string    `json:"agent"`
	PID       int       `json:"pid"`
	Cwd       string    `json:"cwd"`
	Submitted time.Time `json:"submitted"`
	Args      queueArgs `json:"args"`
}

type queueResult struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Exit      int      `json:"exit"`
	Stdout    string   `json:"stdout"`
	Artifacts []string `json:"artifacts,omitempty"`
	WaitS     int      `json:"wait_s"`
	RunS      int      `json:"run_s"`
}

// queueDir is one directory per Editor, named like the lock so a worktree
// Editor with its own UNITY_LOCK_NAME gets its own queue.
func queueDir() string {
	if d := os.Getenv("UTK_QUEUE_DIR"); d != "" {
		return d
	}
	name := os.Getenv("UNITY_LOCK_NAME")
	if name == "" {
		name = "unity-editor"
	}
	return filepath.Join(kitHome(), "queue", name)
}

func newRequestID(now time.Time) string {
	var b [2]byte
	rand.Read(b[:])
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

// writeJSONAtomic lands the file whole or not at all: the coordinator lists
// the directory while agents write into it.
func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeRequest(dir string, r queueRequest) error {
	return writeJSONAtomic(filepath.Join(dir, "requests", r.ID+".json"), r)
}

func writeResult(dir string, r queueResult) error {
	return writeJSONAtomic(filepath.Join(dir, "results", r.ID+".json"), r)
}

func readResult(dir, id string) (queueResult, bool) {
	var r queueResult
	b, err := os.ReadFile(filepath.Join(dir, "results", id+".json"))
	if err != nil || json.Unmarshal(b, &r) != nil {
		return r, false
	}
	return r, true
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// readRequests returns pending requests oldest first and drops the ones whose
// submitter is gone — nobody is waiting for that result, so it must not cost
// Editor time (unity-lock.sh drops dead tickets the same way).
func readRequests(dir string) ([]queueRequest, error) {
	ents, err := os.ReadDir(filepath.Join(dir, "requests"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []queueRequest
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, "requests", e.Name())
		b, err := os.ReadFile(p)
		var r queueRequest
		if err != nil || json.Unmarshal(b, &r) != nil {
			continue
		}
		if !pidAlive(r.PID) {
			os.Remove(p)
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func removeRequest(dir, id string) { os.Remove(filepath.Join(dir, "requests", id+".json")) }

// parseSubmit turns `utk queue submit <kind> [flags]` into a request. It
// refuses what the Editor would refuse later, so a bad request never waits
// for a cycle to learn it was bad.
func parseSubmit(args []string, now time.Time) (queueRequest, int, error) {
	if len(args) == 0 {
		return queueRequest{}, 0, errors.New("usage: utk queue submit compile|test|scene|shot [flags]")
	}
	r := queueRequest{ID: newRequestID(now), Kind: args[0], PID: os.Getpid(), Submitted: now,
		Args: queueArgs{Mode: "editmode", EstS: 60, TimeoutS: 300}}
	r.Cwd, _ = os.Getwd()
	if p := localProjectRoot(); p != "" {
		r.Cwd = p
	}
	r.Agent = os.Getenv("UNITY_LOCK_OWNER")
	if r.Agent == "" {
		r.Agent = "pid-" + strconv.Itoa(r.PID)
	}
	wait := 3600
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		next := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("%s needs a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		var v string
		var err error
		switch rest[i] {
		case "--filter":
			v, err = next(); r.Args.Filter = v
		case "--mode":
			v, err = next(); r.Args.Mode = v
		case "--cmd":
			v, err = next(); r.Args.Cmd = strings.Fields(v)
		case "--script":
			v, err = next(); r.Args.Script = v
		case "--scene":
			v, err = next(); r.Args.Scene = v
		case "--agent":
			v, err = next(); r.Agent = v
		case "--est-seconds":
			v, err = next(); r.Args.EstS, _ = strconv.Atoi(v)
		case "--timeout":
			v, err = next(); r.Args.TimeoutS, _ = strconv.Atoi(v)
		case "--wait":
			v, err = next(); wait, _ = strconv.Atoi(v)
		case "--isolated":
			r.Args.Isolated = true
		case "--edit":
			r.Args.Edit = true
		default:
			err = fmt.Errorf("unknown flag %s", rest[i])
		}
		if err != nil {
			return r, 0, err
		}
	}
	switch r.Kind {
	case "compile":
	case "test":
		if r.Args.Filter == "" {
			return r, 0, errors.New("test needs --filter <TestClass>")
		}
		if why := wholeSuite([]string{"--filter", r.Args.Filter}); why != "" {
			return r, 0, errors.New(why)
		}
		if r.Args.Mode != "editmode" && r.Args.Mode != "playmode" {
			return r, 0, errors.New("--mode must be editmode or playmode")
		}
	case "scene":
		if len(r.Args.Cmd) == 0 && r.Args.Script == "" {
			return r, 0, errors.New("scene needs --cmd '<utk args>' or --script <file>")
		}
	case "shot":
		if r.Args.Script == "" || (r.Args.Scene == "" && !r.Args.Edit) {
			return r, 0, errors.New("shot needs --script <file> and --scene <path> (or --edit)")
		}
	default:
		return r, 0, fmt.Errorf("unknown kind %q (compile|test|scene|shot)", r.Kind)
	}
	return r, wait, nil
}

// runQueue dispatches `utk queue <sub>`.
func runQueue(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: utk queue submit|status|stats|serve")
		return 2
	}
	dir := queueDir()
	switch args[0] {
	case "submit":
		return runSubmit(dir, args[1:], stdout, stderr)
	case "status":
		reqs, err := readRequests(dir)
		if err != nil {
			fmt.Fprintln(stderr, "utk queue:", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s: %d pending\n", dir, len(reqs))
		for _, r := range reqs {
			fmt.Fprintf(stdout, "%s %-7s %s %s %s\n", r.ID, r.Agent, r.Kind, describeArgs(r), r.Submitted.Format("15:04:05"))
		}
		return 0
	case "stats":
		since := time.Now().Add(-24 * time.Hour)
		if v := findFlag(args[1:], "--since"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				since = time.Now().Add(-d)
			}
		}
		f, err := os.Open(telemetryPath())
		if err != nil {
			fmt.Fprintln(stderr, "utk queue stats: no telemetry yet at", telemetryPath())
			return 1
		}
		defer f.Close()
		fmt.Fprint(stdout, telemetryStats(f, since))
		return 0
	case "serve":
		return runServe(dir, stdout, stderr)
	}
	fmt.Fprintln(stderr, "usage: utk queue submit|status|stats|serve")
	return 2
}

func describeArgs(r queueRequest) string {
	switch r.Kind {
	case "test":
		return r.Args.Filter
	case "scene":
		if r.Args.Script != "" {
			return r.Args.Script
		}
		return strings.Join(r.Args.Cmd, " ")
	case "shot":
		return r.Args.Scene + " " + r.Args.Script
	}
	return ""
}

// runSubmit writes the request, makes sure a coordinator is up, and blocks
// until its own result lands (or --wait runs out).
func runSubmit(dir string, args []string, stdout, stderr io.Writer) int {
	r, wait, err := parseSubmit(args, time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "utk queue submit:", err)
		return 2
	}
	if err := writeRequest(dir, r); err != nil {
		fmt.Fprintln(stderr, "utk queue submit:", err)
		return 1
	}
	ensureServe(dir, stderr)
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for time.Now().Before(deadline) {
		if res, ok := readResult(dir, r.ID); ok {
			fmt.Fprint(stdout, res.Stdout)
			if !strings.HasSuffix(res.Stdout, "\n") && res.Stdout != "" {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintf(stderr, "utk queue: %s %s (waited %ds, ran %ds)\n", r.Kind, res.Status, res.WaitS, res.RunS)
			os.Remove(filepath.Join(dir, "results", r.ID+".json"))
			return res.Exit
		}
		time.Sleep(time.Second)
	}
	removeRequest(dir, r.ID)
	fmt.Fprintf(stderr, "utk queue: no result for %s after %ds; request withdrawn\n", r.ID, wait)
	return 1
}
```

Tạm thời để biên dịch được, thêm vào cuối file (Task 10 sẽ thay):

```go
// Replaced in queue_serve.go (Task 10).
func runServe(dir string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "utk queue serve: not implemented yet")
	return 2
}
func ensureServe(dir string, stderr io.Writer) {}
```

- [ ] **Step 4: Chạy, xác nhận pass**

Run: `go vet ./cmd/utk && go test ./cmd/utk -run 'TestQueue|TestParseSubmit|TestReadRequests' -v` → PASS.

---

### Task 6: `queue_cycle.go` pha 1 — `queueExec`, gate, refresh, kind `compile`

**Files:**
- Create: `cmd/utk/queue_cycle.go`; thêm test vào `cmd/utk/queue_test.go`

**Interfaces:**
- Produces:
  ```go
  var queueExec func(cwd string, timeout time.Duration, env []string, name string, args ...string) (out string, code int)
  func utkSelf() string                      // os.Executable()
  func kitSkillsDir() string                 // $UTK_SKILLS_DIR or ~/.unity-cli-agentkit/skills
  func offlineScript() string                // <skills>/utk-test-runner/scripts/unity-test.sh
  func lockScript() string                   // <skills>/utk-cli-core/scripts/unity-lock.sh
  func gateCompile(cwd string) (ok bool, out string)
  func refreshEditor(cwd string) (ok bool, out string)
  type cycle struct { dir string; reqs []queueRequest; results map[string]queueResult; start time.Time; stderr io.Writer; cwd string }
  func newCycle(dir string, reqs []queueRequest, stderr io.Writer) *cycle
  func (c *cycle) finish(r queueRequest, status string, exit int, out string, artifacts ...string)
  func (c *cycle) byKind(kind string) []queueRequest
  func (c *cycle) runGateAndRefresh() (editorOK bool)   // §6 bước 1 và 3
  ```
  Các task 7–9 thêm method vào `cycle`; Task 10 gọi `runCycle`.

- [ ] **Step 1: Test (fail trước)**

Thêm vào `queue_test.go` một fake exec ghi lại lời gọi:

```go
type fakeCall struct{ name string; args []string }

// fakeExec answers by matching a key against "name arg0 arg1…" prefixes.
func fakeExec(t *testing.T, answers map[string]struct{ out string; code int }) *[]fakeCall {
	t.Helper()
	var calls []fakeCall
	old := queueExec
	queueExec = func(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
		calls = append(calls, fakeCall{filepath.Base(name), args})
		line := filepath.Base(name) + " " + strings.Join(args, " ")
		for k, a := range answers {
			if strings.HasPrefix(line, k) {
				return a.out, a.code
			}
		}
		t.Fatalf("unexpected exec: %s", line)
		return "", 1
	}
	t.Cleanup(func() { queueExec = old })
	return &calls
}

func calledWith(calls []fakeCall, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c.name+" "+strings.Join(c.args, " "), prefix) {
			n++
		}
	}
	return n
}

func TestCycleGateFailsCompileAndTestOnly(t *testing.T) {
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"unity-test.sh offline": {"error CS0001: boom\nCOMPILE FAILED (exit 1)\n", 1},
	})
	reqs := []queueRequest{
		{ID: "1", Kind: "compile", Cwd: "/p"},
		{ID: "2", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "A", Mode: "editmode"}},
		{ID: "3", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "x"}}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	if ok := c.runGateAndRefresh(); ok {
		t.Fatal("gate failed but editor marked OK")
	}
	for _, id := range []string{"1", "2"} {
		if r := c.results[id]; r.Status != "GATE_FAILED" || !strings.Contains(r.Stdout, "CS0001") {
			t.Errorf("req %s: %+v", id, r)
		}
	}
	if _, done := c.results["3"]; done {
		t.Error("scene request must survive a failed gate")
	}
	if calledWith(*calls, "utk editor refresh") != 0 {
		t.Error("refresh ran after a failed gate")
	}
}

func TestCycleRefreshOnceForEveryone(t *testing.T) {
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"compile: completed\n", 0},
	})
	reqs := []queueRequest{
		{ID: "1", Kind: "compile", Cwd: "/p"}, {ID: "2", Kind: "compile", Cwd: "/p"},
		{ID: "3", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "A", Mode: "editmode"}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	if !c.runGateAndRefresh() {
		t.Fatal("editor should be OK")
	}
	if n := calledWith(*calls, "utk editor refresh"); n != 1 {
		t.Fatalf("refresh called %d times, want 1", n)
	}
	for _, id := range []string{"1", "2"} {
		if c.results[id].Status != "PASS" {
			t.Errorf("compile %s: %+v", id, c.results[id])
		}
	}
	if _, done := c.results["3"]; done {
		t.Error("test request finished before the test phase")
	}
}

func TestCycleNoCompileWorkSkipsGate(t *testing.T) {
	calls := fakeExec(t, nil)
	c := newCycle(t.TempDir(), []queueRequest{{ID: "1", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "x"}}}}, io.Discard)
	if !c.runGateAndRefresh() || len(*calls) != 0 {
		t.Fatalf("scene-only cycle must not build or refresh: %+v", *calls)
	}
}
```
(import thêm `"io"`.)

- [ ] **Step 2: Chạy, xác nhận fail**

Run: `go test ./cmd/utk -run TestCycle -v` → `undefined: queueExec`.

- [ ] **Step 3: Cài đặt `queue_cycle.go`**

```go
package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// queueExec runs a subprocess with a working directory, a wall-clock limit
// and extra env, returning combined output and exit code. It is a variable so
// tests swap in a fake: nothing in the coordinator is tested against a real
// Editor. The default lives in queue_proc_*.go (process-group kill on timeout).
var queueExec = runWithTimeout

func utkSelf() string {
	p, err := os.Executable()
	if err != nil {
		return "utk"
	}
	return p
}

func kitSkillsDir() string {
	if d := os.Getenv("UTK_SKILLS_DIR"); d != "" {
		return d
	}
	return filepath.Join(kitHome(), "skills")
}

func offlineScript() string {
	return filepath.Join(kitSkillsDir(), "utk-test-runner", "scripts", "unity-test.sh")
}

func lockScript() string {
	return filepath.Join(kitSkillsDir(), "utk-cli-core", "scripts", "unity-lock.sh")
}

// gateCompile is the offline dotnet build: ~3-10 s, no Editor, and the one
// thing that keeps one agent's compile error from stalling everyone else.
func gateCompile(cwd string) (bool, string) {
	out, code := queueExec(cwd, 5*time.Minute, nil, "bash", offlineScript(), "offline", cwd)
	return code == 0 && strings.Contains(out, "COMPILE OK"), out
}

// refreshEditor is one `utk editor refresh` for every change in the cycle.
// utk itself waits out the compile and exits non-zero on errors.
func refreshEditor(cwd string) (bool, string) {
	out, code := queueExec(cwd, 6*time.Minute, nil, utkSelf(), "editor", "refresh")
	return code == 0, out
}

type cycle struct {
	dir     string
	reqs    []queueRequest
	results map[string]queueResult
	start   time.Time
	stderr  io.Writer
	cwd     string // project root; every request of one Editor shares it
}

func newCycle(dir string, reqs []queueRequest, stderr io.Writer) *cycle {
	c := &cycle{dir: dir, reqs: reqs, results: map[string]queueResult{}, start: time.Now(), stderr: stderr}
	if len(reqs) > 0 {
		c.cwd = reqs[0].Cwd
	}
	return c
}

func (c *cycle) finish(r queueRequest, status string, exit int, out string, artifacts ...string) {
	c.results[r.ID] = queueResult{ID: r.ID, Status: status, Exit: exit, Stdout: out, Artifacts: artifacts,
		WaitS: int(c.start.Sub(r.Submitted).Seconds()), RunS: int(time.Since(c.start).Seconds())}
}

// byKind returns the cycle's still-unfinished requests of one kind.
func (c *cycle) byKind(kind string) []queueRequest {
	var out []queueRequest
	for _, r := range c.reqs {
		if _, done := c.results[r.ID]; !done && r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// runGateAndRefresh is steps 1 and 3 of a cycle (spec §6). It returns whether
// the Editor holds a fresh, good assembly for the test phase. Scene and shot
// requests are untouched either way: the old assembly still serves them.
func (c *cycle) runGateAndRefresh() bool {
	compiles, tests := c.byKind("compile"), c.byKind("test")
	if len(compiles)+len(tests) == 0 {
		return true
	}
	fail := func(status string, out string) {
		for _, r := range append(compiles, tests...) {
			c.finish(r, status, 1, out)
		}
	}
	if ok, out := gateCompile(c.cwd); !ok {
		fail("GATE_FAILED", out+"\nGATE_FAILED: fix the compile errors above and resubmit; no Editor time was used\n")
		return false
	}
	ok, out := refreshEditor(c.cwd)
	if !ok {
		// The offline build passed but the Editor disagrees (a new .cs the
		// csproj does not list yet, an Editor-only define): tell both sides.
		io.WriteString(c.stderr, "utk queue: WARN gate-mismatch: dotnet build passed but the Editor compile failed\n")
		fail("FAIL", out)
		return false
	}
	for _, r := range compiles {
		c.finish(r, "PASS", 0, "COMPILE OK\n"+out)
	}
	return true
}

// shellOut is a small helper for the phases that run plain commands.
func shellOut(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
	return queueExec(cwd, timeout, env, name, args...)
}

var _ = bytes.MinRead // keep bytes imported for Task 7's report decoding
var _ = exec.Command
```
(Hai dòng `var _` chỉ để file biên dịch cho tới Task 7; xoá ở Task 7.)

- [ ] **Step 4: Tạo `queue_proc_unix.go` và `queue_proc_windows.go`**

`cmd/utk/queue_proc_unix.go`:

```go
//go:build !windows

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// runWithTimeout runs name args… in cwd inside its own process group; on
// timeout the whole group gets TERM, then KILL. A hung utk child must not keep
// driving the Editor after the coordinator moved on (unity-job.sh learned this
// the hard way with perl setpgrp). Exit 124 marks a timeout, like coreutils.
func runWithTimeout(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return err.Error(), 127
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if ee, ok := err.(*exec.ExitError); ok {
			return buf.String(), ee.ExitCode()
		}
		if err != nil {
			return buf.String() + err.Error(), 1
		}
		return buf.String(), 0
	case <-ctx.Done():
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		return buf.String() + "\nTIMEOUT after " + timeout.String() + "\n", 124
	}
}
```

`cmd/utk/queue_proc_windows.go`:

```go
//go:build windows

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

// runWithTimeout on Windows kills only the direct child: there is no process
// group to signal. Good enough for the Windows agents, who run one job at a time.
func runWithTimeout(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if ctx.Err() != nil {
		return buf.String() + "\nTIMEOUT after " + timeout.String() + "\n", 124
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return buf.String(), ee.ExitCode()
	}
	if err != nil {
		return buf.String() + err.Error(), 1
	}
	return buf.String(), 0
}
```

Thêm test cho timeout thật (unix) vào `queue_test.go`:

```go
func TestRunWithTimeoutKillsChildren(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups")
	}
	marker := filepath.Join(t.TempDir(), "late")
	out, code := runWithTimeout("", time.Second, nil, "bash", "-c", "(sleep 3; touch '"+marker+"') & wait")
	if code != 124 || !strings.Contains(out, "TIMEOUT") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	time.Sleep(3500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("grandchild survived the timeout")
	}
}
```
(import `"runtime"`.)

- [ ] **Step 5: Chạy, xác nhận pass**

Run: `go vet ./cmd/utk && GOOS=windows go vet ./cmd/utk && go test ./cmd/utk -run 'TestCycle|TestRunWithTimeout' -v` → PASS.

---

### Task 7: Pha test — gộp filter, tách report, `NO_TESTS_MATCHED`, fallback khi crash

**Files:**
- Modify: `cmd/utk/queue_cycle.go`; test trong `cmd/utk/queue_test.go`

**Interfaces:**
- Consumes: `splitTestFilter` (Task 4); `utk --raw run_tests --mode <editor|playmode> --filter "A;B"` in envelope JSON (`official.Parse` → payload là report có `Results[].{FullName,Status,Message}`; lỗi `TEST_RUN_CRASHED` nằm trong `errors[].code`).
- Produces:
  ```go
  const queueMergeFilters = true   // Task 13 có thể đặt false nếu pipeline không nhận ";"
  type queueTestResult struct { FullName, Status, Message string }
  func decodeTestReport(raw []byte) (results []queueTestResult, errCode string, ok bool)
  func (c *cycle) runTests()       // cả hai mode; gọi runTestsMode cho editmode rồi playmode
  func (c *cycle) runTestsMode(mode string, reqs []queueRequest)
  func renderTestResults(rs []queueTestResult) (out string, failed bool)
  ```
  Mode map: `editmode` → `--mode editor`, `playmode` → `--mode playmode` (tên mà `utk run_tests` nhận).

- [ ] **Step 1: Test (fail trước)**

```go
func envelope(results string) string {
	return `{"success":true,"data":{"result":"{\"status\":\"completed\",\"summary\":{\"total\":3,\"passed\":2,\"failed\":1},\"results\":[` + results + `]}"}}`
}

func TestCycleMergesFiltersAndSplitsResults(t *testing.T) {
	rep := envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"},{\"FullName\":\"G.BTests.Two\",\"Status\":\"Failed\",\"Message\":\"boom\"},{\"FullName\":\"G.ATests.Three\",\"Status\":\"Passed\"}`)
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"utk --raw run_tests --mode editor --filter ATests;BTests": {rep, 1},
	})
	reqs := []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "b", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "BTests", Mode: "editmode"}},
		{ID: "c", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "CTests", Mode: "editmode"}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runTests()
	if n := calledWith(*calls, "utk --raw run_tests"); n != 1 {
		t.Fatalf("run_tests called %d times, want 1 merged run", n)
	}
	if r := c.results["a"]; r.Status != "PASS" || r.Exit != 0 || !strings.Contains(r.Stdout, "ATests.One") || strings.Contains(r.Stdout, "BTests") {
		t.Errorf("a: %+v", r)
	}
	if r := c.results["b"]; r.Status != "FAIL" || r.Exit != 1 || !strings.Contains(r.Stdout, "boom") {
		t.Errorf("b: %+v", r)
	}
	if r := c.results["c"]; r.Status != "NO_TESTS_MATCHED" {
		t.Errorf("c: %+v", r)
	}
	// filter "CTests" must still have been part of the merged run
	if !strings.Contains((*calls)[0].args[len((*calls)[0].args)-1], "CTests") {
		t.Errorf("merged filter missing CTests: %v", (*calls)[0].args)
	}
}

func TestCycleCrashFallsBackToSequential(t *testing.T) {
	crash := `{"success":false,"errors":[{"code":"TEST_RUN_CRASHED","message":"An unexpected error happened while running tests"}]}`
	okA := envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`)
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"utk --raw run_tests --mode editor --filter ATests;BTests": {crash, 1},
		"utk --raw run_tests --mode editor --filter ATests":        {okA, 0},
		"utk --raw run_tests --mode editor --filter BTests":        {crash, 1},
	})
	reqs := []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "b", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "BTests", Mode: "editmode"}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runTests()
	if n := calledWith(*calls, "utk --raw run_tests"); n != 3 {
		t.Fatalf("expected merged + 2 sequential runs, got %d", n)
	}
	if c.results["a"].Status != "PASS" || c.results["b"].Status != "FAIL" || !strings.Contains(c.results["b"].Stdout, "TEST_RUN_CRASHED") {
		t.Fatalf("a=%+v b=%+v", c.results["a"], c.results["b"])
	}
}

func TestCycleRunsModesSeparately(t *testing.T) {
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"utk --raw run_tests --mode editor --filter ATests":   {envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`), 0},
		"utk --raw run_tests --mode playmode --filter PTests": {envelope(`{\"FullName\":\"G.PTests.One\",\"Status\":\"Passed\"}`), 0},
	})
	c := newCycle(t.TempDir(), []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "p", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "PTests", Mode: "playmode"}},
	}, io.Discard)
	c.runTests()
	if len(*calls) != 2 || c.results["a"].Status != "PASS" || c.results["p"].Status != "PASS" {
		t.Fatalf("calls=%+v a=%+v p=%+v", *calls, c.results["a"], c.results["p"])
	}
}
```

- [ ] **Step 2: Chạy, xác nhận fail**

Run: `go test ./cmd/utk -run 'TestCycleMerges|TestCycleCrash|TestCycleRunsModes' -v` → `c.runTests undefined`.

- [ ] **Step 3: Cài đặt**

Xoá hai dòng `var _ =` ở cuối `queue_cycle.go`, thêm import `"encoding/json"`, `"fmt"`, `"github.com/zasuo/unity-cli-agentkit/internal/official"`, và:

```go
// queueMergeFilters merges every agent's test filter into one "A;B" run (the
// framework's own -testFilter syntax). Task 13 verifies the pipeline's
// run_tests passes it through; false falls back to one run per filter under
// the same refresh, which still saves the compile + reload that dominate.
const queueMergeFilters = true

type queueTestResult struct {
	FullName string `json:"FullName"`
	Status   string `json:"Status"`
	Message  string `json:"Message"`
}

// decodeTestReport unwraps `utk --raw run_tests` output: the official
// envelope, whose payload is the report as a JSON *string* (see testReport).
// errCode carries the envelope's first error code (TEST_RUN_CRASHED,
// TEST_RESULT_FOREIGN) when the run did not produce a report.
func decodeTestReport(raw []byte) (results []queueTestResult, errCode string, ok bool) {
	env, err := official.Parse(raw)
	if err != nil {
		return nil, "", false
	}
	if !env.Success {
		var e struct {
			Errors []struct{ Code string `json:"code"` } `json:"errors"`
		}
		json.Unmarshal(raw, &e)
		if len(e.Errors) > 0 {
			return nil, e.Errors[0].Code, false
		}
		return nil, "FAILED", false
	}
	payload := env.Payload(true)
	if len(payload) > 0 && payload[0] == '"' {
		var s string
		if json.Unmarshal(payload, &s) != nil {
			return nil, "", false
		}
		payload = []byte(s)
	}
	var d struct {
		Results []queueTestResult `json:"results"`
	}
	if json.Unmarshal(payload, &d) != nil {
		return nil, "", false
	}
	return d.Results, "", true
}

func renderTestResults(rs []queueTestResult) (string, bool) {
	var b strings.Builder
	failed := false
	pass := 0
	for _, r := range rs {
		if r.Status == "Passed" {
			pass++
			continue
		}
		failed = failed || r.Status == "Failed"
		fmt.Fprintf(&b, "%s: %s", r.FullName, r.Status)
		if r.Message != "" {
			fmt.Fprintf(&b, " — %s", firstLine(r.Message))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%d passed, %d not passed of %d\n", pass, len(rs)-pass, len(rs))
	for _, r := range rs {
		if r.Status == "Passed" {
			fmt.Fprintf(&b, "  ok %s\n", r.FullName)
		}
	}
	return b.String(), failed
}

func utkTestMode(mode string) string {
	if mode == "playmode" {
		return "playmode"
	}
	return "editor"
}

// runTests is step 4 of a cycle: one merged run per mode, results split per
// requester by name. Only the filters of THIS cycle's requests may appear in
// the report; anything else is another run's (TEST_RESULT_FOREIGN territory).
func (c *cycle) runTests() {
	for _, mode := range []string{"editmode", "playmode"} {
		var reqs []queueRequest
		for _, r := range c.byKind("test") {
			if r.Args.Mode == mode {
				reqs = append(reqs, r)
			}
		}
		if len(reqs) > 0 {
			c.runTestsMode(mode, reqs)
		}
	}
}

func (c *cycle) runTestsMode(mode string, reqs []queueRequest) {
	if queueMergeFilters && len(reqs) > 1 {
		parts := make([]string, len(reqs))
		for i, r := range reqs {
			parts[i] = r.Args.Filter
		}
		raw, _ := queueExec(c.cwd, 15*time.Minute, nil, utkSelf(), "--raw", "run_tests", "--mode", utkTestMode(mode), "--filter", strings.Join(parts, ";"))
		results, errCode, ok := decodeTestReport([]byte(raw))
		if ok {
			c.splitResults(reqs, results)
			return
		}
		io.WriteString(c.stderr, "utk queue: merged run failed ("+errCode+"); rerunning each filter alone\n")
	}
	for _, r := range reqs {
		raw, _ := queueExec(c.cwd, 15*time.Minute, nil, utkSelf(), "--raw", "run_tests", "--mode", utkTestMode(mode), "--filter", r.Args.Filter)
		results, errCode, ok := decodeTestReport([]byte(raw))
		if !ok {
			c.finish(r, "FAIL", 1, errCode+"\n"+raw)
			continue
		}
		c.splitResults([]queueRequest{r}, results)
	}
}

// splitResults hands each requester the tests its own filter selects.
func (c *cycle) splitResults(reqs []queueRequest, results []queueTestResult) {
	for _, r := range reqs {
		var mine []queueTestResult
		for _, part := range splitTestFilter(r.Args.Filter) {
			for _, t := range results {
				if strings.Contains(strings.ToLower(t.FullName), part) {
					mine = append(mine, t)
				}
			}
		}
		if len(mine) == 0 {
			c.finish(r, "NO_TESTS_MATCHED", 1, "no test matched --filter "+r.Args.Filter+"\n")
			continue
		}
		out, failed := renderTestResults(mine)
		if failed {
			c.finish(r, "FAIL", 1, out)
		} else {
			c.finish(r, "PASS", 0, out)
		}
	}
}
```

- [ ] **Step 4: Chạy, xác nhận pass**

Run: `go vet ./cmd/utk && go test ./cmd/utk -run TestCycle -v` → PASS.

---

### Task 8: Pha scene — tuần tự, ngắn trước, timeout riêng

**Files:**
- Modify: `cmd/utk/queue_cycle.go`; test trong `cmd/utk/queue_test.go`

**Interfaces:**
- Produces: `func (c *cycle) runSceneJobs()` — sort theo `EstS` tăng dần (ổn định theo ID khi bằng), `Cmd` → `utk <cmd…>`, `Script` → `bash <script>`, timeout `TimeoutS` giây, exit 124 → `TIMEOUT`.

- [ ] **Step 1: Test (fail trước)**

```go
func TestCycleSceneJobsShortestFirstWithTimeout(t *testing.T) {
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"utk exec --file long.cs":  {"done long\n", 0},
		"utk exec --file short.cs": {"done short\n", 0},
		"bash hang.sh":             {"…\nTIMEOUT after 5s\n", 124},
	})
	reqs := []queueRequest{
		{ID: "1", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "--file", "long.cs"}, EstS: 300, TimeoutS: 300}},
		{ID: "2", Kind: "scene", Cwd: "/p", Args: queueArgs{Script: "hang.sh", EstS: 10, TimeoutS: 5}},
		{ID: "3", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "--file", "short.cs"}, EstS: 5, TimeoutS: 300}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runSceneJobs()
	order := []string{}
	for _, call := range *calls {
		order = append(order, call.args[len(call.args)-1])
	}
	if strings.Join(order, ",") != "short.cs,hang.sh,long.cs" {
		t.Fatalf("order = %v", order)
	}
	if c.results["2"].Status != "TIMEOUT" || c.results["2"].Exit != 124 {
		t.Errorf("hung job: %+v", c.results["2"])
	}
	if c.results["1"].Status != "PASS" || c.results["3"].Status != "PASS" {
		t.Errorf("1=%+v 3=%+v", c.results["1"], c.results["3"])
	}
}
```

- [ ] **Step 2: Chạy, xác nhận fail** → `c.runSceneJobs undefined`.

- [ ] **Step 3: Cài đặt** (thêm import `"sort"`)

```go
// runSceneJobs is step 5: the only exclusive work. Shortest first — a 5 s
// reference fix must not wait behind a 15 min level build.
func (c *cycle) runSceneJobs() {
	jobs := c.byKind("scene")
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].Args.EstS < jobs[j].Args.EstS })
	for _, r := range jobs {
		c.finishCommand(r, c.runJobCommand(r, nil))
	}
}

// runJobCommand runs a request's --cmd (utk argv) or --script (bash) with its
// own wall clock.
func (c *cycle) runJobCommand(r queueRequest, env []string) (string, int) {
	to := time.Duration(r.Args.TimeoutS) * time.Second
	if to <= 0 {
		to = 300 * time.Second
	}
	if r.Args.Script != "" {
		return queueExec(c.cwd, to, env, "bash", r.Args.Script)
	}
	return queueExec(c.cwd, to, env, utkSelf(), r.Args.Cmd...)
}

// finishCommand maps a command's exit onto a result status and collects
// `ARTIFACT: <path>` lines a script printed.
func (c *cycle) finishCommand(r queueRequest, out string, code int) {
	var arts []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "ARTIFACT: ") {
			arts = append(arts, strings.TrimSpace(strings.TrimPrefix(l, "ARTIFACT: ")))
		}
	}
	switch {
	case code == 124:
		c.finish(r, "TIMEOUT", 124, out, arts...)
	case code == 0:
		c.finish(r, "PASS", 0, out, arts...)
	default:
		c.finish(r, "FAIL", code, out, arts...)
	}
}
```

- [ ] **Step 4: Chạy, xác nhận pass** → `go test ./cmd/utk -run TestCycle -v`.

---

### Task 9: Pha shot — một phiên Play, LoadScene giữa các script, `isolated`, `edit`

**Files:**
- Modify: `cmd/utk/queue_cycle.go`; test trong `cmd/utk/queue_test.go`

**Interfaces:**
- Consumes: `utk editor play`, `utk editor stop`, `utk editor status` (payload chứa `"isPlaying":true` khi đang Play), `utk exec '<snippet>'`.
- Produces:
  ```go
  func (c *cycle) runShots()
  func (c *cycle) playSession(reqs []queueRequest)   // play → (LoadScene → script)* → stop
  func (c *cycle) waitPlaying() bool                 // poll editor status ≤ 120 s
  func loadSceneSnippet(scene string) string          // C# for utk exec
  ```
  Script env: `UNITY_IN_PLAY=1`, `UNITY_LOCK_OWNER=coordinator`, `UNITY_QUEUE_REQUEST=<id>`.

- [ ] **Step 1: Test (fail trước)**

```go
func TestCycleShotsShareOnePlaySession(t *testing.T) {
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"utk editor play":   {"play: started\n", 0},
		"utk editor status": {`{"isPlaying":true}`, 0},
		"utk editor stop":   {"stopped\n", 0},
		"utk exec":          {"loaded\n", 0},
		"bash a.sh":         {"ARTIFACT: Temp/shots/a.png\n", 0},
		"bash b.sh":         {"fail\n", 1},
		"bash iso.sh":       {"ARTIFACT: Temp/shots/iso.png\n", 0},
		"bash edit.sh":      {"ARTIFACT: Temp/shots/e.png\n", 0},
	})
	reqs := []queueRequest{
		{ID: "a", Kind: "shot", Cwd: "/p", Args: queueArgs{Script: "a.sh", Scene: "Assets/Scenes/MainMenu.unity", TimeoutS: 300}},
		{ID: "b", Kind: "shot", Cwd: "/p", Args: queueArgs{Script: "b.sh", Scene: "Assets/Scenes/MainMenu.unity", TimeoutS: 300}},
		{ID: "i", Kind: "shot", Cwd: "/p", Args: queueArgs{Script: "iso.sh", Scene: "Assets/Scenes/GamePlay.unity", Isolated: true, TimeoutS: 300}},
		{ID: "e", Kind: "shot", Cwd: "/p", Args: queueArgs{Script: "edit.sh", Edit: true, TimeoutS: 300}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runShots()
	if n := calledWith(*calls, "utk editor play"); n != 2 {
		t.Fatalf("editor play called %d times, want 2 (shared + isolated)", n)
	}
	if n := calledWith(*calls, "utk editor stop"); n != 2 {
		t.Fatalf("editor stop called %d times, want 2", n)
	}
	if n := calledWith(*calls, "utk exec"); n != 3 {
		t.Fatalf("LoadScene exec called %d times, want 3 (a, b, iso)", n)
	}
	if r := c.results["a"]; r.Status != "PASS" || len(r.Artifacts) != 1 || r.Artifacts[0] != "Temp/shots/a.png" {
		t.Errorf("a: %+v", r)
	}
	if c.results["b"].Status != "FAIL" || c.results["i"].Status != "PASS" || c.results["e"].Status != "PASS" {
		t.Errorf("b=%+v i=%+v e=%+v", c.results["b"], c.results["i"], c.results["e"])
	}
	// the edit-mode shot must run after Play has stopped
	last := (*calls)[len(*calls)-1]
	if last.name != "bash" || last.args[0] != "edit.sh" {
		t.Errorf("edit shot not last: %+v", last)
	}
}

func TestLoadSceneSnippetEscapes(t *testing.T) {
	s := loadSceneSnippet(`Assets/Scenes/Main "Menu".unity`)
	if !strings.Contains(s, `SceneManager.LoadScene("Main \"Menu\"")`) {
		t.Fatalf("snippet: %s", s)
	}
}
```

- [ ] **Step 2: Chạy, xác nhận fail** → `c.runShots undefined`.

- [ ] **Step 3: Cài đặt**

```go
// loadSceneSnippet reloads a scene inside Play so the next script starts from
// the scene's own state, not the panel the previous script left open.
// LoadScene takes the scene *name*, not the asset path.
func loadSceneSnippet(scenePath string) string {
	name := strings.TrimSuffix(filepath.Base(scenePath), ".unity")
	name = strings.ReplaceAll(strings.ReplaceAll(name, `\`, `\\`), `"`, `\"`)
	return `UnityEngine.SceneManagement.SceneManager.LoadScene("` + name + `"); return "loaded";`
}

func (c *cycle) waitPlaying() bool {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		out, code := queueExec(c.cwd, time.Minute, nil, utkSelf(), "editor", "status")
		if code == 0 && strings.Contains(out, `"isPlaying":true`) {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// runShots is step 6: one Play session for every shared shot, then one per
// --isolated shot, then the --edit shots with Play off. Static state leaks
// between scripts of one session exactly as it does with domain reload off;
// --isolated is the way out for a script that needs a clean domain.
func (c *cycle) runShots() {
	var shared, isolated, edit []queueRequest
	for _, r := range c.byKind("shot") {
		switch {
		case r.Args.Edit:
			edit = append(edit, r)
		case r.Args.Isolated:
			isolated = append(isolated, r)
		default:
			shared = append(shared, r)
		}
	}
	if len(shared) > 0 {
		c.playSession(shared)
	}
	for _, r := range isolated {
		c.playSession([]queueRequest{r})
	}
	for _, r := range edit {
		c.finishCommand(r, c.runJobCommand(r, []string{"UNITY_LOCK_OWNER=coordinator", "UNITY_QUEUE_REQUEST=" + r.ID}))
	}
}

func (c *cycle) playSession(reqs []queueRequest) {
	if out, code := queueExec(c.cwd, 2*time.Minute, nil, utkSelf(), "editor", "play"); code != 0 || !c.waitPlaying() {
		for _, r := range reqs {
			c.finish(r, "FAIL", 1, "could not enter Play mode\n"+out)
		}
		return
	}
	for _, r := range reqs {
		if out, code := queueExec(c.cwd, time.Minute, nil, utkSelf(), "exec", loadSceneSnippet(r.Args.Scene)); code != 0 {
			c.finish(r, "FAIL", code, "LoadScene failed\n"+out)
			continue
		}
		env := []string{"UNITY_IN_PLAY=1", "UNITY_LOCK_OWNER=coordinator", "UNITY_QUEUE_REQUEST=" + r.ID}
		out, code := c.runJobCommand(r, env)
		c.finishCommand(r, out, code)
		if code == 124 {
			// A hung script may have left Play in an unknown state: restart
			// the session for whoever is left (spec §7).
			queueExec(c.cwd, time.Minute, nil, utkSelf(), "editor", "stop")
			if _, code := queueExec(c.cwd, 2*time.Minute, nil, utkSelf(), "editor", "play"); code != 0 || !c.waitPlaying() {
				break
			}
		}
	}
	queueExec(c.cwd, 2*time.Minute, nil, utkSelf(), "editor", "stop")
}
```

Thêm `runCycle` — điểm vào cho Task 10:

```go
// runCycle is spec §6 end to end, minus the lock (the server holds it).
func runCycle(dir string, reqs []queueRequest, stderr io.Writer) map[string]queueResult {
	c := newCycle(dir, reqs, stderr)
	c.runGateAndRefresh()
	c.runTests()
	c.runSceneJobs()
	c.runShots()
	return c.results
}
```

- [ ] **Step 4: Chạy, xác nhận pass** → `go vet ./cmd/utk && go test ./cmd/utk -v`.

---

### Task 10: `queue_serve.go` — vòng serve, lock, `serve.pid`, EDITOR_BLOCKED, telemetry, `ensureServe`

**Files:**
- Create: `cmd/utk/queue_serve.go`; xoá stub `runServe`/`ensureServe` ở cuối `queue.go`; test trong `cmd/utk/queue_test.go`

**Interfaces:**
- Consumes: `runCycle` (Task 9), `appendTelemetry` (Task 3), `lockScript()` (Task 6), `utk status` (exit 0 khi Editor trả lời).
- Produces:
  ```go
  func runServe(dir string, stdout, stderr io.Writer) int           // vòng lặp; UTK_QUEUE_ONCE=1 → chạy đúng một chu kỳ rồi thoát (test)
  func serveOnce(dir string, stderr io.Writer) (handled int)        // một chu kỳ: lock → runCycle → unlock → results + telemetry
  func acquireEditorLock(cwd string) bool / func releaseEditorLock(cwd string)
  func editorAnswers(cwd string) bool                               // utk status trong 60 s
  func ensureServe(dir string, stderr io.Writer)                    // spawn `utk queue serve` detached nếu serve.pid chết
  ```

- [ ] **Step 1: Test (fail trước)**

```go
func TestServeOnceWritesResultsAndTelemetry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	calls := fakeExec(t, map[string]struct{ out string; code int }{
		"utk status":                 {"ok\n", 0},
		"unity-lock.sh acquire":      {"acquired\n", 0},
		"unity-lock.sh release":      {"released\n", 0},
		"utk set_autotick":           {"ok\n", 0},
		"unity-test.sh offline":      {"COMPILE OK\n", 0},
		"utk editor refresh":         {"compile: completed\n", 0},
	})
	writeRequest(dir, queueRequest{ID: "20261005-100000-c1", Kind: "compile", Agent: "tab1", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now().Add(-30 * time.Second)})
	if n := serveOnce(dir, io.Discard); n != 1 {
		t.Fatalf("handled %d, want 1", n)
	}
	res, ok := readResult(dir, "20261005-100000-c1")
	if !ok || res.Status != "PASS" || res.WaitS < 29 {
		t.Fatalf("result: %+v ok=%v", res, ok)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", "20261005-100000-c1.json")); err == nil {
		t.Fatal("request not removed after result")
	}
	b, _ := os.ReadFile(telemetryPath())
	if !strings.Contains(string(b), `"via":"queue"`) || !strings.Contains(string(b), `"kind":"compile"`) {
		t.Fatalf("telemetry: %s", b)
	}
	seq := []string{}
	for _, c := range *calls {
		seq = append(seq, c.name+" "+c.args[0])
	}
	// lock brackets the Editor work; the gate runs before the lock
	joined := strings.Join(seq, "|")
	if strings.Index(joined, "unity-test.sh offline") > strings.Index(joined, "unity-lock.sh acquire") ||
		strings.Index(joined, "unity-lock.sh acquire") > strings.Index(joined, "utk editor") ||
		strings.Index(joined, "utk editor") > strings.Index(joined, "unity-lock.sh release") {
		t.Fatalf("order: %v", seq)
	}
}

func TestServeOnceEditorBlocked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	fakeExec(t, map[string]struct{ out string; code int }{
		"utk status": {"UNREACHABLE\n", 1},
	})
	writeRequest(dir, queueRequest{ID: "20261005-100000-s1", Kind: "scene", PID: os.Getpid(), Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "x"}}})
	serveOnce(dir, io.Discard)
	res, _ := readResult(dir, "20261005-100000-s1")
	if res.Status != "EDITOR_BLOCKED" {
		t.Fatalf("result: %+v", res)
	}
}

func TestServeOnceNothingPending(t *testing.T) {
	fakeExec(t, nil)
	if n := serveOnce(t.TempDir(), io.Discard); n != 0 {
		t.Fatalf("handled %d on an empty queue", n)
	}
}

func TestServeLockRefusesSecondServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_QUEUE_ONCE", "1")
	fakeExec(t, nil)
	if err := os.MkdirAll(filepath.Join(dir, "serve.lock.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "serve.pid"), []byte(strconv.Itoa(os.Getpid())), 0o644)
	var errb bytes.Buffer
	if code := runServe(dir, io.Discard, &errb); code != 0 || !strings.Contains(errb.String(), "already") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}
```
(import `"strconv"`.)

- [ ] **Step 2: Chạy, xác nhận fail** → `undefined: serveOnce`.

- [ ] **Step 3: Cài đặt `queue_serve.go`**

```go
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	serveIdleSleep  = 2 * time.Second
	editorProbeWait = 60 * time.Second
)

func editorAnswers(cwd string) bool {
	_, code := queueExec(cwd, editorProbeWait, nil, utkSelf(), "status")
	return code == 0
}

func acquireEditorLock(cwd string) bool {
	_, code := queueExec(cwd, 61*time.Minute, nil, "bash", lockScript(), "acquire", "coordinator", "3600")
	return code == 0
}

func releaseEditorLock(cwd string) {
	queueExec(cwd, time.Minute, nil, "bash", lockScript(), "release", "coordinator")
}

// serveOnce runs one cycle over whatever is pending. It returns how many
// requests it answered. The gate runs before the lock: a failing dotnet build
// must cost nobody else Editor time.
func serveOnce(dir string, stderr io.Writer) int {
	reqs, err := readRequests(dir)
	if err != nil {
		fmt.Fprintln(stderr, "utk queue serve:", err)
		return 0
	}
	if len(reqs) == 0 {
		return 0
	}
	cwd := reqs[0].Cwd
	var results map[string]queueResult
	if !editorAnswers(cwd) {
		// A modal dialog or a dead server: nothing we send will run. Say so
		// to everyone now rather than let them wait out an hour each.
		results = map[string]queueResult{}
		for _, r := range reqs {
			results[r.ID] = queueResult{ID: r.ID, Status: "EDITOR_BLOCKED", Exit: 1,
				Stdout: "the Editor does not answer `utk status` (modal dialog? not running?); fix it and resubmit\n"}
		}
	} else {
		c := newCycle(dir, reqs, stderr)
		editorOK := c.runGateAndRefreshBeforeLock()
		if acquireEditorLock(cwd) {
			queueExec(cwd, time.Minute, nil, utkSelf(), "set_autotick", "--enable", "true")
			if editorOK {
				c.runEditorRefresh()
			}
			c.runTests()
			c.runSceneJobs()
			c.runShots()
			queueExec(cwd, time.Minute, nil, utkSelf(), "set_autotick", "--enable", "false")
			releaseEditorLock(cwd)
		} else {
			for _, r := range c.byKind("compile") {
				c.finish(r, "FAIL", 1, "could not take the Editor lock within an hour\n")
			}
			for _, kind := range []string{"test", "scene", "shot"} {
				for _, r := range c.byKind(kind) {
					c.finish(r, "FAIL", 1, "could not take the Editor lock within an hour\n")
				}
			}
		}
		results = c.results
	}
	for _, r := range reqs {
		res, ok := results[r.ID]
		if !ok {
			continue
		}
		if err := writeResult(dir, res); err != nil {
			fmt.Fprintln(stderr, "utk queue serve: write result:", err)
			continue
		}
		removeRequest(dir, r.ID)
		appendTelemetry(telemetryEntry{TS: time.Now().UTC().Format(time.RFC3339), Owner: r.Agent, Task: r.ID,
			Kind: r.Kind, WaitS: res.WaitS, HoldS: res.RunS, Result: res.Status, Via: "queue"})
	}
	return len(results)
}

// runServe is the coordinator loop: one per queue dir, guarded by a mkdir lock
// and a pid file so a second `serve` (two agents' submits racing) exits.
func runServe(dir string, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(stderr, "utk queue serve:", err)
		return 1
	}
	lock := filepath.Join(dir, "serve.lock.d")
	pidFile := filepath.Join(dir, "serve.pid")
	if err := os.Mkdir(lock, 0o755); err != nil {
		if b, _ := os.ReadFile(pidFile); pidAlive(atoi(string(b))) {
			fmt.Fprintln(stderr, "utk queue serve: already running (pid", strings.TrimSpace(string(b))+")")
			return 0
		}
		os.RemoveAll(lock) // stale: the previous server died without cleaning up
		if err := os.Mkdir(lock, 0o755); err != nil {
			fmt.Fprintln(stderr, "utk queue serve:", err)
			return 1
		}
	}
	defer os.RemoveAll(lock)
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)
	defer os.Remove(pidFile)
	fmt.Fprintln(stdout, "utk queue serve:", dir)
	for {
		if serveOnce(dir, stderr) == 0 {
			if os.Getenv("UTK_QUEUE_ONCE") == "1" {
				return 0
			}
			time.Sleep(serveIdleSleep)
			continue
		}
		if os.Getenv("UTK_QUEUE_ONCE") == "1" {
			return 0
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// ensureServe starts a detached coordinator when none is alive, so the first
// submit of a session needs no separate setup step.
func ensureServe(dir string, stderr io.Writer) {
	if b, err := os.ReadFile(filepath.Join(dir, "serve.pid")); err == nil && pidAlive(atoi(string(b))) {
		return
	}
	logf, _ := os.OpenFile(filepath.Join(dir, "serve.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	cmd := exec.Command(utkSelf(), "queue", "serve")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Env = os.Environ()
	detach(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(stderr, "utk queue: could not start the coordinator:", err)
		return
	}
	cmd.Process.Release()
	fmt.Fprintln(stderr, "utk queue: started coordinator for", dir)
}
```

Trong `queue_cycle.go`, tách `runGateAndRefresh` thành hai bước để gate chạy **trước** lock và refresh chạy **sau** lock (giữ hành vi test Task 6 bằng cách để `runGateAndRefresh` gọi cả hai):

```go
// runGateAndRefreshBeforeLock is the gate half: it needs no Editor, so it runs
// before the lock. Returns whether the refresh half should run.
func (c *cycle) runGateAndRefreshBeforeLock() bool {
	compiles, tests := c.byKind("compile"), c.byKind("test")
	if len(compiles)+len(tests) == 0 {
		return false
	}
	if ok, out := gateCompile(c.cwd); !ok {
		for _, r := range append(compiles, tests...) {
			c.finish(r, "GATE_FAILED", 1, out+"\nGATE_FAILED: fix the compile errors above and resubmit; no Editor time was used\n")
		}
		return false
	}
	return true
}

// runEditorRefresh is the refresh half, under the lock.
func (c *cycle) runEditorRefresh() bool {
	compiles, tests := c.byKind("compile"), c.byKind("test")
	ok, out := refreshEditor(c.cwd)
	if !ok {
		io.WriteString(c.stderr, "utk queue: WARN gate-mismatch: dotnet build passed but the Editor compile failed\n")
		for _, r := range append(compiles, tests...) {
			c.finish(r, "FAIL", 1, out)
		}
		return false
	}
	for _, r := range compiles {
		c.finish(r, "PASS", 0, "COMPILE OK\n"+out)
	}
	return true
}

// runGateAndRefresh keeps Task 6's single-call shape for tests that do not
// care about the lock boundary. Decide "nothing to do" BEFORE the gate runs:
// after a failed gate the compile/test requests are finished, so byKind would
// read as empty and wrongly report the Editor as OK.
func (c *cycle) runGateAndRefresh() bool {
	if len(c.byKind("compile"))+len(c.byKind("test")) == 0 {
		return true
	}
	if !c.runGateAndRefreshBeforeLock() {
		return false
	}
	return c.runEditorRefresh()
}
```

`detach` theo platform — thêm vào `queue_proc_unix.go`:

```go
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
```
và `queue_proc_windows.go`:
```go
func detach(cmd *exec.Cmd) {}
```

Xoá stub `runServe`/`ensureServe` ở `queue.go`.

- [ ] **Step 4: Chạy, xác nhận pass** → `go vet ./cmd/utk && GOOS=windows go vet ./cmd/utk && go test ./cmd/utk -v`. Kiểm tra `TestCycleGateFailsCompileAndTestOnly` và `TestCycleRefreshOnceForEveryone` vẫn xanh sau khi tách hàm.

---

### Task 11: Route `utk queue` trong `main.go` + usage

**Files:**
- Modify: `cmd/utk/main.go:34-36` (cạnh `if cmd == "init"`), `cmd/utk/usage.go` (const `usage`)
- Test: `cmd/utk/main_test.go`

- [ ] **Step 1: Test (fail trước)**

```go
func TestRun_QueueStatusNeedsNoEditor(t *testing.T) {
	t.Setenv("UTK_QUEUE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := run([]string{"queue", "status"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "0 pending") {
		t.Fatalf("code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}
}

func TestUsageMentionsQueue(t *testing.T) {
	if !strings.Contains(usage, "utk queue submit") {
		t.Fatal("usage does not document utk queue")
	}
}
```

- [ ] **Step 2: Chạy, xác nhận fail** → `go test ./cmd/utk -run 'TestRun_QueueStatus|TestUsageMentionsQueue'` fail (queue bị map thành `unity command queue`).

- [ ] **Step 3: Cài đặt**

`main.go`, ngay sau khối `if cmd == "init"`:

```go
	// `queue` is utk's own coordinator for a shared Editor; it never reaches
	// the official CLI as a tool name.
	if cmd == "queue" {
		return runQueue(rest, stdout, stderr)
	}
```

`usage.go`, thêm sau khối `utk test [args…]`:

```
  utk queue submit compile | test --filter <Class> [--mode editmode|playmode]
                 | scene --cmd '<utk args>'|--script <f.sh> [--est-seconds N] [--timeout S]
                 | shot --script <f.sh> --scene <path> [--isolated] [--edit]
                          shared-Editor queue: one compile/reload, one merged test
                          run and one Play session per cycle for every agent;
                          blocks until YOUR result (exit = its exit)
  utk queue status | stats [--since 24h] | serve
```

- [ ] **Step 4: Chạy, xác nhận pass** → `go test ./cmd/utk -v`.

---

### Task 12: Skill `utk-editor-queue` và cập nhật hướng dẫn

**Files:**
- Create: `skills/utk-editor-queue/SKILL.md`
- Modify: `skills/utk-cli-core/SKILL.md` (mục "## Editor lock & jobs", dòng 272–278), `skills/utk-test-runner/SKILL.md` (mục chạy test trên Editor), `skills/utk-playmode-driving/SKILL.md` (đầu mục screenshot), `internal/initcmd/agents-block.md` (bullet "Offline first" và "Run only your own tests"), `README.md` (danh sách skill)

- [ ] **Step 1: Viết `skills/utk-editor-queue/SKILL.md`**

```markdown
---
name: utk-editor-queue
description: Use when two or more agents share one Unity Editor and you need to compile, run tests, run a scene/asset builder, or take screenshots — or when you are about to call `utk editor refresh`, `utk run_tests`, `utk editor play`/`screenshot`, `utk open_scene`/`save_scene` and another agent may be on the same Editor. Routes the work through `utk queue submit` so compile/reload/Play cost is paid once per cycle for everyone.
---

# Shared Editor queue

One Editor serves one job at a time. The queue (`utk queue`) does not add Editors; it
removes repeated work: per cycle it runs ONE `dotnet build` gate, ONE `refresh`, ONE
merged `run_tests`, the scene jobs shortest-first, and ONE Play session for every
screenshot script. You get back only your own results.

## What goes through the queue — and what does not

| You want to | Do |
|---|---|
| Edit a prefab you own | Call `utk exec` directly: `LoadPrefabContents → edit → SaveAsPrefabAsset → UnloadPrefabContents → ImportAsset(ForceSynchronousImport)` in ONE snippet (utk-asset-edit). No queue. |
| Query, console, layout check on a prefab | `utk exec` / `utk batch` directly. No queue. |
| Check that C# compiles | `utk queue submit compile` |
| Run your tests | `utk queue submit test --filter <YourTestClass> [--mode playmode]` |
| Edit a scene, open another scene, run a builder that writes many assets | `utk queue submit scene --cmd 'exec --file build.cs' --est-seconds 30` or `--script run.sh` |
| Screenshot / runtime UI check | write `shots.sh` with your usual `utk exec`/`wait_for`/`simulate_pointer`/`utk screenshot` steps, print `ARTIFACT: <png>` per file, then `utk queue submit shot --script shots.sh --scene Assets/Scenes/MainMenu.unity` |

Rules:
- `submit` blocks until your result and exits with its exit code. Run it with the
  Bash tool's `run_in_background` and keep working.
- Never `utk editor refresh` / `run_tests` / `editor play` directly while others
  share the Editor — that is the queue's job. Direct `exec` is fine.
- `GATE_FAILED` = your tree does not build on plain .NET; nobody else was blocked.
  Fix, resubmit. `NO_TESTS_MATCHED` = your filter selects nothing.
- In a `shot` script: Play is already on and your scene freshly loaded
  (`UNITY_IN_PLAY=1`); do not `editor_play`/`editor_stop`. Static state from another
  agent's script may be present — need a clean domain? add `--isolated`. Static UI
  without Play: `--edit`.
- File ownership is yours to keep: two agents editing one prefab or one scene is a
  merge conflict the queue cannot prevent. Name your files on the task board.

## Inspect
`utk queue status` (pending), `utk queue stats --since 24h` (wait/hold per kind),
`~/.unity-cli-agentkit/queue/<editor>/serve.log`. The coordinator starts on the first
`submit`; `UNITY_LOCK_NAME` picks the Editor, like unity-lock.sh.
```

- [ ] **Step 2: Sửa `skills/utk-cli-core/SKILL.md` mục "Editor lock & jobs"**

Thay đoạn mở đầu bằng: "Several agents on one Editor → `utk queue submit` (skill `utk-editor-queue`): one compile/reload, one merged test run and one Play session per cycle. `unity-job.sh` remains for a single long exclusive job; give it `--kind` (telemetry) and `--gate <repo>` for compile/test work." Giữ phần mô tả `UNITY_LOCK_NAME`, `UNITY_JOB_NO_TICK`, stale lock.

- [ ] **Step 3: Sửa `skills/utk-test-runner/SKILL.md`**

Trong mục Editor tests, thêm trước lệnh `utk run_tests`: "Shared Editor (another agent may be on it)? `utk queue submit test --filter <Class>` instead — same output, merged into one run with everyone else's. Direct `utk run_tests` only when you are alone on the Editor."

- [ ] **Step 4: Sửa `skills/utk-playmode-driving/SKILL.md`**

Đầu mục screenshot: "Shared Editor: put the steps below in a script and `utk queue submit shot --script <f.sh> --scene <path>`; the queue enters Play once for everyone and reloads your scene before your script (`UNITY_IN_PLAY=1` is set: do not play/stop yourself)."

- [ ] **Step 5: Sửa `internal/initcmd/agents-block.md`**

Bullet "Auto Refresh is off": thêm "— on a shared Editor use `utk queue submit compile` instead of a direct `refresh`."
Bullet "Run only your own tests": thêm "On a shared Editor: `utk queue submit test --filter <YourTestClass>`."
Thêm bullet mới sau "Batch, don't fan out": "**Shared Editor = queue, not lock-and-wait:** compile, tests, scene edits and screenshots go through `utk queue submit` (skill `utk-editor-queue`); prefab edits and queries go direct. `unity-job.sh` jobs carry `--kind` and, for compile/test work, `--gate <repo>`."

- [ ] **Step 6: README**

Thêm `utk-editor-queue` vào bảng skill và một dòng trong mục "Multiple agents / one Editor" (nếu chưa có mục, thêm 3 dòng dưới phần `unity-job.sh`).

- [ ] **Step 7: Kiểm tra**

Run: `go test ./... && bash skills/utk-cli-core/scripts/unity-job.sh --selftest && bash skills/utk-cli-core/scripts/unity-lock.sh --selftest && bash hooks/guard-tests.test.sh` → tất cả OK. `grep -rn "utk queue submit" skills internal README.md | wc -l` ≥ 6.

---

### Task 13 (MANUAL, cần Editor DU04 mở): kiểm chứng filter `;` và chạy baseline 3 agent

**Files:**
- Modify (tuỳ kết quả): `cmd/utk/queue_cycle.go` hằng `queueMergeFilters`
- Modify: `docs/superpowers/specs/2026-10-05-editor-queue-coordinator-design.md` §11

**Consumes:** `bash setup-cli.sh` để cài binary + skills mới vào `~/.unity-cli-agentkit`.

- [ ] **Step 1: Cài kit**

Run: `bash setup-cli.sh` → `~/.unity-cli-agentkit/bin/utk queue status` in `0 pending`.

- [ ] **Step 2: Kiểm chứng `;`**

Trong `~/Unity/DU04` với Editor mở, chọn hai test class có thật (lấy từ `utk list_tests --mode editor | head`), ví dụ `X` và `Y`:
Run: `UTK_RAW_REPORT=1 utk run_tests --mode editor --filter "X;Y"` (timeout 600 s; đây đúng là lệnh coordinator gọi — `--raw` không chờ kết quả nên không dùng).
- Report chứa test của **cả hai** → `;` được nhận, giữ `queueMergeFilters = true`.
- Report rỗng hoặc chỉ một → đặt `queueMergeFilters = false`, ghi lý do vào comment của hằng, `go test ./cmd/utk` (test gộp sẽ cần đổi kỳ vọng sang `len(reqs)` lần gọi — sửa `TestCycleMergesFiltersAndSplitsResults` cho khớp).

- [ ] **Step 2b: Kiểm chứng hai giả định của coordinator trên Editor thật**

- `utk editor play && utk editor status` → output phải chứa đúng chuỗi `"isPlaying":true` (`waitPlaying` tìm chuỗi này; nếu tên trường khác, sửa `waitPlaying` trong `queue_cycle.go`). Sau đó `utk editor stop`.
- Mở một modal dialog trong Editor (ví dụ Build Settings → một hộp thoại xác nhận) rồi chạy `utk status`; lệnh phải **thoát khác 0** hoặc timeout — `serveOnce` dựa vào đó để trả `EDITOR_BLOCKED`. Nếu nó vẫn trả 0, ghi lại để siết điều kiện probe trong `queue_serve.go`.

- [ ] **Step 3: Baseline "trước"**

Ba job tuần tự kiểu cũ (mỗi job một compile): `for f in X Y Z; do unity-job.sh a-$f /tmp/$f.log --kind test --gate ~/Unity/DU04 -- bash -c "utk editor refresh && utk run_tests --mode editor --filter $f"; done`, đo tổng bằng `time`. Ghi vào §11.

- [ ] **Step 4: Baseline "sau"**

Ba `submit` song song: `for f in X Y Z; do (cd ~/Unity/DU04 && utk queue submit test --filter $f --agent a-$f) & done; time wait`. Kiểm tra `serve.log` có đúng **một** `editor refresh`. Ghi tổng thời gian và số lần refresh vào §11.

- [ ] **Step 5: `utk queue stats --since 1h`** in bảng theo kind; dán vào §11.

---

### Task 14 (MANUAL, ngoài repo): Enter Play Mode Options ở DU04 rồi DU02

**Files:** `~/Unity/DU04/ProjectSettings/EditorSettings.asset` (qua UI), code game.

- [ ] **Step 1: Baseline ảnh** — 3 screenshot panel chính qua `utk queue submit shot` trước khi đổi.
- [ ] **Step 2: Rà static** — `grep -rnE 'static [^(]*=|static event|static (List|Dictionary|HashSet)<' Assets --include=*.cs | grep -v -E 'Plugins/|ThirdParty/|const |readonly '` → ghi danh sách; mỗi hit: thêm reset trong `[RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.SubsystemRegistration)]` hoặc ghi "vô hại vì …".
- [ ] **Step 3: Bật** Project Settings → Editor → Enter Play Mode Options: tick, **bỏ tick Reload Domain**, giữ Reload Scene.
- [ ] **Step 4: Kiểm** — 5 lần `utk queue submit shot` liên tiếp cùng script; so ảnh với baseline; `utk console --type error` sạch.
- [ ] **Step 5: Lặp cho DU02.** Lỗi → tắt lại, ghi hit chưa xử lý vào task board, không giữ nửa vời.

---

## Self-review

**Spec coverage**
- §3 bảng "chỉ lock cái toàn cục" → Task 12 (skill nói rõ gọi thẳng vs submit).
- §4 kiến trúc, file JSON, lock giữ nguyên, Go → Task 5, 10, 11.
- §4.1/4.2 request/result → Task 5.
- §5 bốn kind → Task 6 (compile), 7 (test + fallback), 8 (scene SJF), 9 (shot: batch, LoadScene, isolated, edit, ARTIFACT).
- §6 thứ tự chu kỳ, gate trước lock, autotick → Task 10.
- §7 lỗi: GATE_FAILED (6/10), gate-mismatch (10), NO_TESTS_MATCHED (7), crash fallback (7), TIMEOUT process group (6/8), shot fail/timeout restart (9), EDITOR_BLOCKED (10), coordinator chết → stale lock + ensureServe (10), agent chết → pid (5), hai serve (10). Dedup "id mới thay id cũ cùng agent+kind+filter" **không** cài — agent chỉ resubmit sau khi có result nên request cũ đã bị xoá; ghi vào spec §7 là "không cần" khi thực hiện Task 12.
- §8 telemetry + gate trong unity-job.sh + `stats` → Task 1, 2, 3; agents-block → Task 12.
- §9 Enter Play Mode Options → Task 14.
- §10 kiểm thử → mỗi task; chạy thật → Task 13.
- §11 baseline → Task 13.

**Placeholder scan:** không còn "TBD/TODO"; Task 12 Step 2–6 mô tả câu chữ cần chèn thay vì code vì là Markdown; Task 13–14 là MANUAL có lệnh cụ thể.

**Type consistency:** `queueExec(cwd, timeout, env, name, args...)` dùng nhất quán Task 6–10; `c.finish(r, status, exit, out, artifacts...)`; `runJobCommand(r, env) (string, int)`; `finishCommand(r, out, code)`; `decodeTestReport(raw) (results, errCode, ok)`; `telemetryEntry` field `Task` nhận request ID ở Task 10; `runGateAndRefresh` giữ cho test Task 6 sau khi tách ở Task 10.
