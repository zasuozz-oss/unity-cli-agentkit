package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
		{"test"},                           // filter required
		{"shot", "--script", "s.sh"},       // scene required
		{"scene"},                          // cmd or script required
		{"dance"},                          // unknown kind
		{"test", "--filter", "Game.Tests"}, // whole suite (namespace)
		{"test", "--filter", "A", "--wait", "2h"},         // not a number: 0 would withdraw at once
		{"scene", "--script", "s.sh", "--timeout", "10m"}, // not a number: would silently become 300
		{"scene", "--script", "s.sh", "--est-seconds", "x"},
	} {
		if _, _, err := parseSubmit(bad, now); err == nil {
			t.Errorf("parseSubmit(%v) accepted", bad)
		}
	}
	r, _, _ = parseSubmit([]string{"scene", "--cmd", "exec --file x.cs", "--est-seconds", "5"}, now)
	if r.Args.EstS != 5 || len(r.Args.Cmd) != 3 || r.Args.TimeoutS != 300 {
		t.Fatalf("scene args: %+v", r.Args)
	}
	// The coordinator runs scripts from the project root, not the submitter's cwd.
	r, _, _ = parseSubmit([]string{"scene", "--script", "s.sh"}, now)
	if !filepath.IsAbs(r.Args.Script) || filepath.Base(r.Args.Script) != "s.sh" {
		t.Fatalf("script not absolute: %q", r.Args.Script)
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

type fakeCall struct {
	name string
	args []string
	env  []string
}

// fakeLine is the key a fake answer is matched on: "bash <script> a b" becomes
// "<script-basename> a b", and the test binary (utk.test) reads as "utk".
func fakeLine(name string, args []string) (string, []string) {
	if filepath.Base(name) == "bash" && len(args) > 0 {
		name, args = args[0], args[1:]
	}
	return strings.TrimSuffix(filepath.Base(name), ".test"), args
}

// fakeExec answers by matching a key against "name arg0 arg1…" prefixes.
func fakeExec(t *testing.T, answers map[string]struct {
	out  string
	code int
}) *[]fakeCall {
	t.Helper()
	var calls []fakeCall
	var mu sync.Mutex // the lock toucher calls in from its own goroutine
	old := queueExec
	queueExec = func(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
		n, a := fakeLine(name, args)
		mu.Lock()
		calls = append(calls, fakeCall{n, a, env})
		mu.Unlock()
		line := n + " " + strings.Join(a, " ")
		// Longest key wins: "...--filter ATests" is a prefix of the merged
		// "...--filter ATests;BTests" line, and map order is random.
		best := ""
		for k := range answers {
			if strings.HasPrefix(line, k) && len(k) >= len(best) {
				best = k
			}
		}
		if ans, ok := answers[best]; ok {
			return ans.out, ans.code
		}
		if strings.HasPrefix(line, "utk editor gc") {
			return "destroyed 0 leaked OS fallback fonts\n", 0 // housekeeping every cycle does
		}
		switch {
		case strings.HasPrefix(line, "utk editor wait"): // before every refresh
			return "ready (waited 0s)\n", 0
		case strings.HasPrefix(line, "utk exec "+probeSnippet): // the main-thread probe
			return "alive\n", 0
		case strings.HasPrefix(line, "utk cancel_tests"): // after a stopped test job
			return "cancelled\n", 0
		case strings.HasPrefix(line, "utk set_autotick"):
			return "autotick set\n", 0
		case strings.HasPrefix(line, "utk editor status"): // the PLAY-UNOWNED check: not playing unless a test says so
			return `{"playMode":"stopped","status":"ready"}`, 0
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

type fakeAnswers = map[string]struct {
	out  string
	code int
}

func TestCycleGateFailsCompileAndTestOnly(t *testing.T) {
	calls := fakeExec(t, fakeAnswers{
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
	calls := fakeExec(t, fakeAnswers{
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

func TestCycleRefreshFailedMarksCompileAndTest(t *testing.T) {
	fakeExec(t, fakeAnswers{
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"error CS0102: dup\n", 1},
	})
	reqs := []queueRequest{
		{ID: "1", Kind: "compile", Cwd: "/p"},
		{ID: "2", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "A", Mode: "editmode"}},
		{ID: "3", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "x"}}},
	}
	var stderr bytes.Buffer
	c := newCycle(t.TempDir(), reqs, &stderr)
	if c.runGateAndRefresh() {
		t.Fatal("refresh failed but editor marked OK")
	}
	for _, id := range []string{"1", "2"} {
		if r := c.results[id]; r.Status != "FAIL" || r.Exit != 1 || !strings.Contains(r.Stdout, "CS0102") {
			t.Errorf("req %s: %+v", id, r)
		}
	}
	if _, done := c.results["3"]; done {
		t.Error("scene request must survive a failed refresh")
	}
	if !strings.Contains(stderr.String(), "gate-mismatch") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func envelope(results string) string {
	return `{"success":true,"data":{"result":"{\"status\":\"completed\",\"summary\":{\"total\":3,\"passed\":2,\"failed\":1},\"results\":[` + results + `]}"}}`
}

func TestCycleMergesFiltersAndSplitsResults(t *testing.T) {
	queueMergeFilters = true
	t.Cleanup(func() { queueMergeFilters = false })
	rep := envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"},{\"FullName\":\"G.BTests.Two\",\"Status\":\"Failed\",\"Message\":\"boom\"},{\"FullName\":\"G.ATests.Three\",\"Status\":\"Passed\"}`)
	calls := fakeExec(t, fakeAnswers{
		"utk run_tests --mode editor --filter ATests;BTests": {rep, 1},
	})
	reqs := []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "b", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "BTests", Mode: "editmode"}},
		{ID: "c", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "CTests", Mode: "editmode"}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runTests()
	if n := calledWith(*calls, "utk run_tests"); n != 1 {
		t.Fatalf("run_tests called %d times, want 1 merged run", n)
	}
	// Without it the filtered path prints the compact view, which has no FullName to split on.
	if env := strings.Join((*calls)[0].env, " "); !strings.Contains(env, "UTK_RAW_REPORT=1") {
		t.Errorf("merged run env = %q, want UTK_RAW_REPORT=1", env)
	}
	if r := c.results["a"]; r.Status != "PASS" || r.Exit != 0 || !strings.Contains(r.Stdout, "ATests.One") || strings.Contains(r.Stdout, "BTests") {
		t.Errorf("a: %+v", r)
	}
	if r := c.results["b"]; r.Status != "FAIL" || r.Exit != 1 || !strings.Contains(r.Stdout, "boom") {
		t.Errorf("b: %+v", r)
	}
	if strings.Contains(c.results["b"].Stdout, "ATests") {
		t.Errorf("b leaked a's tests: %q", c.results["b"].Stdout)
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
	queueMergeFilters = true
	t.Cleanup(func() { queueMergeFilters = false })
	crash := `{"success":false,"errors":[{"code":"TEST_RUN_CRASHED","message":"An unexpected error happened while running tests"}]}`
	okA := envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`)
	calls := fakeExec(t, fakeAnswers{
		"utk run_tests --mode editor --filter ATests;BTests": {crash, 1},
		"utk run_tests --mode editor --filter ATests":        {okA, 0},
		"utk run_tests --mode editor --filter BTests":        {crash, 1},
	})
	reqs := []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "b", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "BTests", Mode: "editmode"}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runTests()
	if n := calledWith(*calls, "utk run_tests"); n != 3 {
		t.Fatalf("expected merged + 2 sequential runs, got %d", n)
	}
	if c.results["a"].Status != "PASS" || c.results["b"].Status != "FAIL" || !strings.Contains(c.results["b"].Stdout, "TEST_RUN_CRASHED") {
		t.Fatalf("a=%+v b=%+v", c.results["a"], c.results["b"])
	}
}

func TestCycleRunsModesSeparately(t *testing.T) {
	calls := fakeExec(t, fakeAnswers{
		"utk run_tests --mode editor --filter ATests":   {envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`), 0},
		"utk run_tests --mode playmode --filter PTests": {envelope(`{\"FullName\":\"G.PTests.One\",\"Status\":\"Passed\"}`), 0},
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

func TestRenderTestResults(t *testing.T) {
	cases := []struct {
		name   string
		in     []queueTestResult
		failed bool
		want   string
	}{
		{"passed", []queueTestResult{{FullName: "A", Status: "Passed"}}, false, ""},
		{"skipped", []queueTestResult{{FullName: "A", Status: "Passed"}, {FullName: "S", Status: "Skipped"}}, false, "S: Skipped"},
		{"inconclusive", []queueTestResult{{FullName: "I", Status: "Inconclusive"}}, true, ""},
		{"failed", []queueTestResult{{FullName: "F", Status: "Failed"}}, true, ""},
	}
	for _, tc := range cases {
		out, failed := renderTestResults(tc.in)
		if failed != tc.failed || !strings.Contains(out, tc.want) {
			t.Errorf("%s: failed=%v out=%q", tc.name, failed, out)
		}
	}
}

func TestCycleSplitNoDuplicatesOnOverlap(t *testing.T) {
	rep := envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"},{\"FullName\":\"G.ATests.Two\",\"Status\":\"Passed\"}`)
	fakeExec(t, fakeAnswers{"utk run_tests": {rep, 0}})
	c := newCycle(t.TempDir(), []queueRequest{{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests;ATests.One", Mode: "editmode"}}}, io.Discard)
	c.runTests()
	if out := c.results["a"].Stdout; !strings.Contains(out, "2 passed, 0 not passed of 2") {
		t.Errorf("duplicates: %q", out)
	}
}

func TestCycleTimeoutSkipsFallback(t *testing.T) {
	queueMergeFilters = true
	t.Cleanup(func() { queueMergeFilters = false })
	calls := fakeExec(t, fakeAnswers{"utk run_tests": {"", 124}})
	c := newCycle(t.TempDir(), []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "b", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "BTests", Mode: "editmode"}},
	}, io.Discard)
	c.runTests()
	if n := calledWith(*calls, "utk run_tests"); n != 1 {
		t.Fatalf("run_tests called %d times, want 1", n)
	}
	for _, id := range []string{"a", "b"} {
		if r := c.results[id]; r.Status != "TIMEOUT" || r.Exit != 124 {
			t.Errorf("%s: %+v", id, r)
		}
	}
}

func TestCycleSceneJobsShortestFirstWithTimeout(t *testing.T) {
	calls := fakeExec(t, fakeAnswers{
		"utk exec --file long.cs":  {"done long\n", 0},
		"utk exec --file short.cs": {"done short\n", 0},
		"hang.sh":                  {"…\nTIMEOUT after 5s\n", 124},
	})
	reqs := []queueRequest{
		{ID: "1", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "--file", "long.cs"}, EstS: 300, TimeoutS: 300}},
		{ID: "2", Kind: "scene", Cwd: "/p", Args: queueArgs{Script: "hang.sh", EstS: 10, TimeoutS: 5}},
		{ID: "3", Kind: "scene", Cwd: "/p", Args: queueArgs{Cmd: []string{"exec", "--file", "short.cs"}, EstS: 5, TimeoutS: 300}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runSceneJobs()
	order, jobs := []string{}, []fakeCall{}
	for _, call := range *calls {
		if !strings.HasPrefix(strings.Join(call.args, " "), "exec "+probeSnippet) { // skip freeEditor's probe after the hung job
			jobs = append(jobs, call)
		}
	}
	for _, call := range jobs {
		// fakeLine folds `bash <script>` into name=<script> with no args.
		if len(call.args) == 0 {
			order = append(order, call.name)
		} else {
			order = append(order, call.args[len(call.args)-1])
		}
	}
	if strings.Join(order, ",") != "short.cs,hang.sh,long.cs" {
		t.Fatalf("order = %v", order)
	}
	// A scene script that wraps unity-job.sh must see the coordinator as the lock owner.
	for _, call := range jobs {
		if env := strings.Join(call.env, " "); !strings.Contains(env, "UNITY_LOCK_OWNER=coordinator") || !strings.Contains(env, "UNITY_QUEUE_REQUEST=") {
			t.Errorf("%s env = %q", call.name, env)
		}
	}
	if c.results["2"].Status != "TIMEOUT" || c.results["2"].Exit != 124 {
		t.Errorf("hung job: %+v", c.results["2"])
	}
	if c.results["1"].Status != "PASS" || c.results["3"].Status != "PASS" {
		t.Errorf("1=%+v 3=%+v", c.results["1"], c.results["3"])
	}
}

func TestCycleShotsShareOnePlaySession(t *testing.T) {
	calls := fakeExec(t, fakeAnswers{
		"utk editor play":   {"play: started\n", 0},
		"utk editor status": {`{"playMode":"playing"}`, 0},
		"utk editor stop":   {"stopped\n", 0},
		"utk exec":          {"loaded\n", 0},
		"a.sh":              {"ARTIFACT: Temp/shots/a.png\n", 0},
		"b.sh":              {"fail\n", 1},
		"iso.sh":            {"ARTIFACT: Temp/shots/iso.png\n", 0},
		"edit.sh":           {"ARTIFACT: Temp/shots/e.png\n", 0},
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
	if last := (*calls)[len(*calls)-1]; last.name != "edit.sh" {
		t.Errorf("edit shot not last: %+v", last)
	}
}

func TestLoadSceneSnippetEscapes(t *testing.T) {
	s := loadSceneSnippet(`Assets/Scenes/Main "Menu".unity`)
	if !strings.Contains(s, `SceneManager.LoadScene("Main \"Menu\"")`) {
		t.Fatalf("snippet: %s", s)
	}
}

func TestCyclePlayRestartFailureFailsRemaining(t *testing.T) {
	var calls []fakeCall
	plays := 0
	old := queueExec
	queueExec = func(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
		n, a := fakeLine(name, args)
		calls = append(calls, fakeCall{n, a, env})
		switch line := n + " " + strings.Join(a, " "); {
		case strings.HasPrefix(line, "utk editor play"):
			plays++
			if plays > 1 {
				return "play: refused\n", 1
			}
			return "play: started\n", 0
		case strings.HasPrefix(line, "utk editor status"):
			return `{"playMode":"playing"}`, 0
		case strings.HasPrefix(line, "a.sh"):
			return "hung\n", 124
		}
		return "ok\n", 0
	}
	t.Cleanup(func() { queueExec = old })
	reqs := []queueRequest{
		{ID: "a", Kind: "shot", Cwd: "/p", Args: queueArgs{Script: "a.sh", Scene: "Assets/Scenes/M.unity", TimeoutS: 300}},
		{ID: "b", Kind: "shot", Cwd: "/p", Args: queueArgs{Script: "b.sh", Scene: "Assets/Scenes/M.unity", TimeoutS: 300}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runShots()
	if c.results["a"].Status != "TIMEOUT" {
		t.Errorf("a: %+v", c.results["a"])
	}
	if b := c.results["b"]; b.Status != "FAIL" || !strings.Contains(b.Stdout, "re-enter Play") {
		t.Errorf("b: %+v", b)
	}
	if n := calledWith(calls, "utk editor play"); n != 2 {
		t.Errorf("editor play called %d times, want 2", n)
	}
}

func TestServeOnceWritesResultsAndTelemetry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	calls := fakeExec(t, fakeAnswers{
		"utk status":            {"ok\n", 0},
		"unity-lock.sh acquire": {"acquired\n", 0},
		"unity-lock.sh release": {"released\n", 0},
		"utk set_autotick":      {"ok\n", 0},
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"compile: completed\n", 0},
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
		if len(c.args) > 1 && c.args[0] == "editor" && c.args[1] == "status" {
			continue // the PLAY-UNOWNED check runs before the gate; it is not Editor work
		}
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
	fakeExec(t, fakeAnswers{
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
	pid := os.Getpid() // Windows: pidIsServe is pidAlive
	if runtime.GOOS != "windows" {
		// pidIsServe wants a process that reads as `queue serve` in ps.
		srv := exec.Command("sleep", "30")
		srv.Args[0] = "utk queue serve"
		if err := srv.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { srv.Process.Kill(); srv.Wait() })
		pid = srv.Process.Pid
	}
	os.WriteFile(filepath.Join(dir, "serve.pid"), []byte(strconv.Itoa(pid)), 0o644)
	var errb bytes.Buffer
	if code := runServe(dir, io.Discard, &errb); code != 0 || !strings.Contains(errb.String(), "already") {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestServeOnceGateFailsSkipsLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	calls := fakeExec(t, fakeAnswers{
		"utk status":            {"ok\n", 0},
		"unity-test.sh offline": {"error CS1002\n", 1},
	})
	writeRequest(dir, queueRequest{ID: "20261005-100000-c1", Kind: "compile", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now()})
	writeRequest(dir, queueRequest{ID: "20261005-100000-t1", Kind: "test", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now(), Args: queueArgs{Filter: "Foo", Mode: "editmode"}})
	if n := serveOnce(dir, io.Discard); n != 2 {
		t.Fatalf("handled %d, want 2", n)
	}
	for _, c := range *calls {
		if c.name == "unity-lock.sh" {
			t.Fatalf("lock taken after a failed gate: %v", *calls)
		}
	}
	for _, id := range []string{"20261005-100000-c1", "20261005-100000-t1"} {
		if res, ok := readResult(dir, id); !ok || res.Status != "GATE_FAILED" {
			t.Fatalf("%s: %+v ok=%v", id, res, ok)
		}
	}
}

// A cycle outlives unity-lock.sh's 15-min stale threshold; without a touch a
// legacy waiter deletes the coordinator's lock and takes the Editor mid-cycle.
func TestServeOnceTouchesLockDuringCycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	old := lockTouchEvery
	lockTouchEvery = 10 * time.Millisecond
	t.Cleanup(func() { lockTouchEvery = old })
	calls := fakeExec(t, fakeAnswers{
		"utk status":                      {"ok\n", 0},
		"unity-lock.sh acquire":           {"acquired\n", 0},
		"unity-lock.sh touch coordinator": {"touched\n", 0},
		"unity-lock.sh release":           {"released\n", 0},
		"utk set_autotick":                {"ok\n", 0},
		"unity-test.sh offline":           {"COMPILE OK\n", 0},
		"utk editor refresh":              {"compile: completed\n", 0},
	})
	fake := queueExec
	queueExec = func(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
		if len(args) > 1 && args[0] == "editor" && args[1] == "refresh" {
			time.Sleep(50 * time.Millisecond) // a long Editor step
		}
		return fake(cwd, timeout, env, name, args...)
	}
	writeRequest(dir, queueRequest{ID: "20261005-100000-c1", Kind: "compile", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now()})
	serveOnce(dir, io.Discard)
	if calledWith(*calls, "unity-lock.sh touch coordinator") == 0 {
		t.Fatalf("lock never touched: %+v", *calls)
	}
	// the toucher must be gone once the lock is released
	if last := (*calls)[len(*calls)-1]; last.name != "unity-lock.sh" || last.args[0] != "release" {
		t.Fatalf("last call %+v, want release", last)
	}
}

// A withdrawn request (submitter timed out during the lock wait) must not cost
// Editor time; results nobody collected are swept after an hour.
func TestServeOnceDropsWithdrawnAndSweepsResults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	calls := fakeExec(t, fakeAnswers{
		"utk status":            {"ok\n", 0},
		"unity-lock.sh":         {"ok\n", 0},
		"utk set_autotick":      {"ok\n", 0},
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"compile: completed\n", 0},
		"utk run_tests --mode editor --filter ATests": {envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`), 0},
	})
	fake := queueExec
	queueExec = func(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
		if len(args) > 1 && args[1] == "acquire" {
			removeRequest(dir, "20261005-100000-b") // b's submitter withdrew while we queued for the lock
		}
		return fake(cwd, timeout, env, name, args...)
	}
	writeRequest(dir, queueRequest{ID: "20261005-100000-a", Kind: "test", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now(), Args: queueArgs{Filter: "ATests", Mode: "editmode"}})
	writeRequest(dir, queueRequest{ID: "20261005-100000-b", Kind: "test", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now(), Args: queueArgs{Filter: "BTests", Mode: "editmode"}})
	writeResult(dir, queueResult{ID: "old"})
	stale := time.Now().Add(-2 * time.Hour)
	os.Chtimes(filepath.Join(dir, "results", "old.json"), stale, stale)
	serveOnce(dir, io.Discard)
	if _, ok := readResult(dir, "20261005-100000-b"); ok {
		t.Error("withdrawn request got a result")
	}
	for _, c := range *calls {
		if strings.Contains(strings.Join(c.args, " "), "BTests") {
			t.Errorf("withdrawn filter ran: %+v", c)
		}
	}
	if res, ok := readResult(dir, "20261005-100000-a"); !ok || res.Status != "PASS" {
		t.Errorf("a: %+v ok=%v", res, ok)
	}
	if _, ok := readResult(dir, "old"); ok {
		t.Error("hour-old result not swept")
	}
}

// An idle coordinator exits so the next submit respawns it with the fresh
// binary and env.
func TestRunServeExitsWhenIdle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_QUEUE_ONCE", "")
	fakeExec(t, nil)
	old := serveIdleExit
	serveIdleExit = 0
	t.Cleanup(func() { serveIdleExit = old })
	done := make(chan int, 1)
	go func() { done <- runServe(dir, io.Discard, io.Discard) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("code = %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServe did not exit when idle")
	}
	if _, err := os.Stat(filepath.Join(dir, "serve.lock.d")); err == nil {
		t.Fatal("serve.lock.d left behind")
	}
}

// A serve.pid whose pid was reused by an unrelated process must not count as a
// running coordinator, or every submit waits out its --wait for nothing.
func TestEnsureServeIgnoresReusedPid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pidIsServe falls back to pidAlive on Windows")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "serve.pid"), []byte(strconv.Itoa(os.Getpid())), 0o644)
	spawned := false
	old := serveSpawn
	serveSpawn = func(string) error { spawned = true; return nil }
	t.Cleanup(func() { serveSpawn = old })
	ensureServe(dir, io.Discard)
	if !spawned {
		t.Fatal("a live non-serve pid blocked the spawn")
	}
}

// queueExec merges the child's stderr, and `utk run_tests` prints its waiting
// hints there before the envelope; the report must still decode.
func TestDecodeTestReportSkipsStderrPreamble(t *testing.T) {
	raw := "utk: another test run (another agent?) is in progress on this Editor; waiting for it instead of cancelling it\n" +
		"utk: this run is taking a while — for long suites run `utk run_tests` with run_in_background\n" +
		envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`)
	results, errCode, ok := decodeTestReport([]byte(raw))
	if !ok || errCode != "" || len(results) != 1 || results[0].FullName != "G.ATests.One" {
		t.Fatalf("ok=%v code=%q results=%+v", ok, errCode, results)
	}
}

// The default: the pipeline's --filter is a substring, so each agent's filter
// is its own run — three requests, three calls, all under the one refresh.
func TestCycleDefaultRunsEachFilterAlone(t *testing.T) {
	rep := envelope(`{\"FullName\":\"G.ATests.One\",\"Status\":\"Passed\"}`)
	calls := fakeExec(t, fakeAnswers{"utk run_tests": {rep, 0}})
	reqs := []queueRequest{
		{ID: "a", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "ATests", Mode: "editmode"}},
		{ID: "b", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "BTests", Mode: "editmode"}},
		{ID: "c", Kind: "test", Cwd: "/p", Args: queueArgs{Filter: "CTests", Mode: "editmode"}},
	}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runTests()
	if n := calledWith(*calls, "utk run_tests"); n != 3 {
		t.Fatalf("run_tests called %d times, want one per filter", n)
	}
	if n := calledWith(*calls, "utk run_tests --mode editor --filter ATests;"); n != 0 {
		t.Fatalf("filters were merged: %d", n)
	}
}

func TestQueueCancelRemovesTheRequest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_QUEUE_DIR", dir)
	writeRequest(dir, queueRequest{ID: "20261005-100000-aaaa", Kind: "compile", PID: os.Getpid()})
	var out, errb bytes.Buffer
	if code := runQueue([]string{"cancel", "20261005-100000-aaaa"}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if reqs, _ := readRequests(dir); len(reqs) != 0 {
		t.Fatalf("still queued: %+v", reqs)
	}
	if code := runQueue([]string{"cancel", "20261005-100000-aaaa"}, &out, &errb); code != 1 {
		t.Fatalf("cancelling nothing exit %d, want 1", code)
	}
}

// A job whose request is gone (cancelled, or its submitter was killed by its
// own deadline) is Editor time for nobody: it stops instead of running out
// its timeout while everyone else waits behind it.
func TestRunWithAbortStopsACommandNobodyWaitsFor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sleep")
	}
	started := time.Now()
	out, code := runWithAbort("", time.Minute, nil, func() bool { return time.Since(started) > 200*time.Millisecond }, "sleep", "30")
	if code != 130 || !strings.Contains(out, "CANCELLED") {
		t.Fatalf("code %d out %q", code, out)
	}
	if time.Since(started) > 10*time.Second {
		t.Fatalf("took %s", time.Since(started))
	}
}

func TestCycleCancelledJobReportsCancelled(t *testing.T) {
	fakeExec(t, fakeAnswers{"a.sh": {"half done\n", 130}})
	reqs := []queueRequest{{ID: "a", Kind: "scene", Cwd: "/p", Args: queueArgs{Script: "a.sh"}}}
	c := newCycle(t.TempDir(), reqs, io.Discard)
	c.runSceneJobs()
	if c.results["a"].Status != "CANCELLED" {
		t.Fatalf("result: %+v", c.results["a"])
	}
}

// Leaked fonts become collectable at a domain reload, and the refresh is where
// a cycle reloads — so that is where the cycle collects them.
func TestServeOnceCollectsLeakedFontsAfterTheRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	calls := fakeExec(t, fakeAnswers{
		"utk status":            {"ok\n", 0},
		"unity-lock.sh acquire": {"acquired\n", 0},
		"unity-lock.sh release": {"released\n", 0},
		"utk set_autotick":      {"ok\n", 0},
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"compile: completed\n", 0},
		"utk editor gc":         {"destroyed 25\n", 0},
	})
	writeRequest(dir, queueRequest{ID: "20261005-100000-c1", Kind: "compile", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now()})
	serveOnce(dir, io.Discard)
	var seq []string
	for _, c := range *calls {
		seq = append(seq, c.name+" "+strings.Join(c.args, " "))
	}
	joined := strings.Join(seq, "|")
	gc, refresh, release := strings.Index(joined, "utk editor gc"), strings.Index(joined, "utk editor refresh"), strings.Index(joined, "unity-lock.sh release")
	if gc < 0 || gc < refresh || gc > release {
		t.Fatalf("order: %v", seq)
	}
}

func TestMaybeRestart(t *testing.T) {
	answers := fakeAnswers{
		"unity-lock.sh acquire": {"acquired\n", 0},
		"unity-lock.sh release": {"released\n", 0},
		"utk editor restart":    {"utk: Editor restarted in 70s\n", 0},
	}
	oldDeg := editorDegraded
	t.Cleanup(func() { editorDegraded = oldDeg })
	for _, c := range []struct {
		name            string
		degraded, queue bool
		env             string
		want            int
	}{
		{"degraded and idle", true, false, "", 1},
		{"healthy", false, false, "", 0},
		{"someone is waiting", true, true, "", 0},
		{"switched off", true, false, "1", 0},
	} {
		dir := t.TempDir()
		calls := fakeExec(t, answers)
		t.Setenv("UTK_NO_AUTO_RESTART", c.env)
		editorDegraded = func(string) (string, bool) { return "the Editor has degraded", c.degraded }
		if c.queue {
			writeRequest(dir, queueRequest{ID: "20261005-100000-aaaa", Kind: "compile", PID: os.Getpid(), Cwd: "/p"})
		}
		maybeRestart(dir, "/p", io.Discard)
		if n := calledWith(*calls, "utk editor restart"); n != c.want {
			t.Errorf("%s: restart called %d times, want %d", c.name, n, c.want)
		}
		if c.want == 1 && (calledWith(*calls, "unity-lock.sh acquire") != 1 || calledWith(*calls, "unity-lock.sh release") != 1) {
			t.Errorf("%s: restart must hold the Editor lock: %v", c.name, *calls)
		}
	}
}

// A request cancelled while still queued gets no result, ever: its submitter
// must notice the request is gone instead of sitting out its --wait.
func TestSubmitReturnsWhenItsQueuedRequestIsCancelled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UTK_QUEUE_DIR", dir)
	oldSpawn, oldPoll := serveSpawn, submitPoll
	serveSpawn, submitPoll = func(string) error { return nil }, 10*time.Millisecond
	t.Cleanup(func() { serveSpawn, submitPoll = oldSpawn, oldPoll })
	go func() {
		for {
			if ents, _ := os.ReadDir(filepath.Join(dir, "requests")); len(ents) == 1 {
				os.Remove(filepath.Join(dir, "requests", ents[0].Name()))
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() { done <- runSubmit(dir, []string{"compile", "--wait", "60"}, io.Discard, &errb) }()
	select {
	case code := <-done:
		if code != 130 || !strings.Contains(errb.String(), "cancelled") {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("submit kept waiting for a cancelled request")
	}
}
