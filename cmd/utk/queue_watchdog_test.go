package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The Hung Editor rule: silent 5 min → one restart; silent again within 30 min
// of it → no restart, a HUNG line for the producer; answering again clears it.
func TestWatchdogRestartsOnceThenGivesUp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UNITY_LOCK_DIR", dir) // no lock held
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	hung := true
	var calls []string
	old := queueExec
	queueExec = func(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
		n, a := fakeLine(name, args)
		line := n + " " + strings.Join(a, " ")
		calls = append(calls, line)
		if strings.HasPrefix(line, "utk exec "+probeSnippet) && hung {
			return "Main thread operation timed out after 20000ms", 1
		}
		return "ok\n", 0
	}
	t.Cleanup(func() { queueExec = old })
	clock := time.Date(2026, 10, 6, 4, 0, 0, 0, time.UTC)
	w := newWatchdog(dir)
	w.now = func() time.Time { return clock }
	restarts := func() int { return calledLines(calls, "utk editor restart --force") }

	for i := 0; i < 10; i++ { // 4:30 silent: not yet
		w.tick("/p", io.Discard)
		clock = clock.Add(30 * time.Second)
	}
	if restarts() != 0 {
		t.Fatalf("restarted before 5 min: %v", calls)
	}
	w.tick("/p", io.Discard) // 5:00
	clock = clock.Add(30 * time.Second)
	w.tick("/p", io.Discard) // 5:30
	if restarts() != 1 || calledLines(calls, "utk set_autotick --enable true") != 1 || calledLines(calls, "unity-lock.sh acquire coordinator") != 1 {
		t.Fatalf("want one restart with lock + autotick: %v", calls)
	}
	// hung again 10 min later, for over 5 min: give up, do not restart
	var stderr bytes.Buffer
	for i := 0; i < 12; i++ {
		clock = clock.Add(30 * time.Second)
		w.tick("/p", &stderr)
	}
	if restarts() != 1 {
		t.Fatalf("second hang within 30 min restarted again")
	}
	if n := hungNotice(dir); !strings.Contains(n, "hung again") {
		t.Fatalf("HUNG notice = %q", n)
	}
	b, _ := os.ReadFile(dir + "/t.jsonl")
	if !strings.Contains(string(b), "GAVE_UP") || strings.Count(string(b), "RESTARTED") != 1 {
		t.Errorf("telemetry = %s", b)
	}
	if newWatchdog(dir).lastRestart.IsZero() {
		t.Error("restart time not kept for a respawned coordinator")
	}
	hung = false
	clock = clock.Add(30 * time.Second)
	w.tick("/p", io.Discard)
	if hungNotice(dir) != "" {
		t.Error("HUNG notice not cleared once the Editor answers")
	}
}

// A live unity-job holding the lock is busy inside its own timeout, not hung.
func TestWatchdogLeavesALiveLockHolderAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UNITY_LOCK_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "unity-editor.lock.d"), 0o755)
	os.WriteFile(filepath.Join(dir, "unity-editor.lock.d", "owner"), []byte("ui-dev 04:00:00\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "unity-editor.lock.d", "pid"), []byte(strconvItoa(os.Getpid())+"\n"), 0o644)
	calls := fakeExec(t, nil) // any exec (a probe) fails the test
	clock := time.Now()
	w := newWatchdog(dir)
	w.now = func() time.Time { return clock }
	for i := 0; i < 20; i++ {
		w.tick("/p", io.Discard)
		clock = clock.Add(30 * time.Second)
	}
	if len(*calls) != 0 {
		t.Fatalf("probed/restarted under a live holder: %+v", *calls)
	}
	// holder gone: its silence is no longer work
	os.WriteFile(filepath.Join(dir, "unity-editor.lock.d", "pid"), []byte("999999\n"), 0o644)
	if busyElsewhere() {
		t.Fatal("dead holder still counts as busy")
	}
	// taken by hand (an Editor freeze): no pid recorded -> busy, never restarted under it
	os.Remove(filepath.Join(dir, "unity-editor.lock.d", "pid"))
	if !busyElsewhere() {
		t.Fatal("a pid-less (hand-taken) lock must count as busy")
	}
}

// A test job honors its own --timeout; when it ends from the client side the
// Editor-side run is cancelled and checked, and the job reads TIMEOUT/ORPHANED.
func TestCycleTestTimeoutAndOrphanFreeTheEditor(t *testing.T) {
	calls := fakeExec(t, nil)
	var timeouts []time.Duration
	oldJob := queueJobExec
	queueJobExec = func(cwd string, timeout time.Duration, env []string, gone func() bool, name string, args ...string) (string, int) {
		timeouts = append(timeouts, timeout)
		if strings.Contains(strings.Join(args, " "), "Orphan") {
			return "CANCELLED\n", 130
		}
		return "TIMEOUT after 60s\n", 124
	}
	t.Cleanup(func() { queueJobExec = oldJob })
	c := newCycle(t.TempDir(), []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", PID: os.Getpid(), Args: queueArgs{Filter: "Spin", Mode: "editmode", TimeoutS: 60}},
		{ID: "b", Kind: "test", Cwd: "/p", PID: 999999, Args: queueArgs{Filter: "Orphan", Mode: "editmode", TimeoutS: 600}},
	}, io.Discard)
	c.runTests()
	if len(timeouts) != 2 || timeouts[0] != time.Minute {
		t.Fatalf("job timeouts = %v, want the request's 60s first", timeouts)
	}
	if r := c.results["a"]; r.Status != "TIMEOUT" || r.Exit != 124 || !strings.Contains(r.Stdout, "editor: freed") {
		t.Errorf("a: %+v", r)
	}
	if r := c.results["b"]; r.Status != "ORPHANED" || r.Exit != 130 {
		t.Errorf("b: %+v", r)
	}
	if n := calledWith(*calls, "utk cancel_tests"); n != 2 {
		t.Errorf("cancel_tests called %d times, want 2", n)
	}
}

// After cancel_tests a main thread still silent past freeWait means a test
// that never yields: restart, wait, autotick on.
func TestFreeEditorRestartsWhenCancelCannotLand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	old := freeWait
	freeWait = 0
	t.Cleanup(func() { freeWait = old })
	calls := fakeExec(t, fakeAnswers{"utk exec " + probeSnippet: {"timed out", 124}, "utk editor restart": {"restarted\n", 0}})
	c := newCycle(dir, nil, io.Discard)
	c.cwd = "/p"
	if note := c.freeEditor(true, "x TIMEOUT"); !strings.Contains(note, "restarted") {
		t.Fatalf("note = %q", note)
	}
	for _, want := range []string{"utk cancel_tests", "utk editor restart --force", "utk editor wait", "utk set_autotick --enable true"} {
		if calledWith(*calls, want) != 1 {
			t.Errorf("%s: %+v", want, *calls)
		}
	}
}

func TestPollTestsBudgetIsAFailureNotAnEmptyReport(t *testing.T) {
	old := testPollBudget
	testPollBudget = 0
	t.Cleanup(func() { testPollBudget = old })
	raw, code := pollTests([]byte(`{"success":true,"data":{"result":"{\"status\":\"running\"}"}}`), time.Now(), "", io.Discard)
	if _, errCode, _ := decodeTestReport(raw); code == 0 || errCode != "TEST_RUN_TIMEOUT" {
		t.Fatalf("code %d raw %s", code, raw)
	}
}

// include_explicit only when every filter part names a method or case: a
// class filter must not start the [Explicit] sweeps in that class.
func TestRunTestsExplicitOnlyForMethodFilters(t *testing.T) {
	if got, _, _ := mapVerb("run_tests", []string{"--mode", "editor", "--filter", "A.B"}); strings.Contains(strings.Join(got, " "), "include_explicit") {
		t.Fatalf("mapVerb decides include_explicit on its own: %v", got)
	}
	var snippet string
	answer, code := "METHODS:yes", 0
	old := editorSnippet
	editorSnippet = func(s, project string) (string, int) { snippet = s; return answer, code }
	t.Cleanup(func() { editorSnippet = old })
	if !filterNamesOnlyMethods("QuestBotTests.QuestBot_TradeSweep_m1_on; X.Y(1,\"a\")", "") ||
		!strings.Contains(snippet, `new[] { "QuestBotTests.QuestBot_TradeSweep_m1_on","X.Y" }`) {
		t.Fatalf("parts not passed as C# literals without case args: %s", snippet)
	}
	answer = "METHODS:no QuestBotTests"
	if filterNamesOnlyMethods("QuestBotTests", "") {
		t.Fatal("a class filter included [Explicit] tests")
	}
	answer, code = "", 1
	if filterNamesOnlyMethods("A.B", "") {
		t.Fatal("must fail closed when the Editor cannot answer")
	}
}

// The exec answer ends with the Editor's log lines: only the TOOLTESTS line counts.
func TestToolTestsScanIgnoresTrailingLogLines(t *testing.T) {
	answer := "TOOLTESTS:\"\"\n[Log] Executing IPostBuildCleanup for: Unity.PerformanceTesting.Editor.TestRunBuilder.\n"
	old := editorSnippet
	editorSnippet = func(s, project string) (string, int) { return answer, 0 }
	t.Cleanup(func() { editorSnippet = old })
	if got := toolTestsNotExplicit(""); got != "" {
		t.Fatalf("clear suite refused: %q", got)
	}
	answer = "TOOLTESTS:QuestBotTests.Sweep\n[Log] x\n"
	if got := toolTestsNotExplicit(""); got != "QuestBotTests.Sweep" {
		t.Fatalf("got %q", got)
	}
}

func calledLines(lines []string, prefix string) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}
