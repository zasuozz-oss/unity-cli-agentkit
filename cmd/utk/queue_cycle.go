package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/initcmd"
	"github.com/zasuo/unity-cli-agentkit/internal/official"
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
	// Skills ship next to the binary; queue state and telemetry do not (see kitHome).
	if d, err := initcmd.SkillsDir(); err == nil {
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

// queueMergeFilters would merge every agent's test filter into one "A;B" run.
// Off: com.unity.pipeline's run_tests (checked on 0.8.0-exp.1, DU04,
// 2026-10-05) matches --filter as one case-insensitive substring of FullName
// (ShouldIncludeTest → IndexOf), so "A;B" matches nothing. One run per filter
// under the same refresh still saves the compile + reload that dominate.
// A var so the merged path stays tested for a pipeline that learns lists.
var queueMergeFilters = false

type queueTestResult struct {
	FullName string `json:"FullName"`
	Status   string `json:"Status"`
	Message  string `json:"Message"`
}

// decodeTestReport unwraps `UTK_RAW_REPORT=1 utk run_tests` output: the official
// envelope, whose payload is the report as a JSON *string* (see testReport).
// errCode carries the envelope's first error code (TEST_RUN_CRASHED,
// TEST_RESULT_FOREIGN) when the run did not produce a report.
func decodeTestReport(raw []byte) (results []queueTestResult, errCode string, ok bool) {
	// queueExec merges the child's stderr into raw, and `utk run_tests` talks
	// there while it waits ("another test run is in progress", the 90 s hint).
	// The envelope is the last thing printed; start from its opening brace.
	if i := bytes.IndexByte(raw, '{'); i > 0 {
		raw = raw[i:]
	}
	env, err := official.Parse(raw)
	if err != nil {
		return nil, "", false
	}
	if !env.Success {
		var e struct {
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
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
		// Skipped ([Ignore]/[Explicit]) is not a failure; anything else that is
		// not Passed (Failed, Inconclusive, unknown) must not report PASS.
		failed = failed || r.Status != "Skipped"
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

// rawReportEnv keeps utk's wait (test slot, poll, foreign check) but prints the
// final envelope: --raw would skip the wait and return the async hand-off, and
// the compact view drops the FullName the split needs.
var rawReportEnv = []string{"UTK_RAW_REPORT=1"}

// queueTestTimeout covers `utk run_tests` end to end: up to 10 min waiting
// for a foreign run to clear the slot, then up to 10 min polling the result.
const queueTestTimeout = 25 * time.Minute

func (c *cycle) runTestsMode(mode string, reqs []queueRequest) {
	if queueMergeFilters && len(reqs) > 1 {
		parts := make([]string, len(reqs))
		for i, r := range reqs {
			parts[i] = r.Args.Filter
		}
		raw, code := queueExec(c.cwd, queueTestTimeout, rawReportEnv, utkSelf(), "run_tests", "--mode", utkTestMode(mode), "--filter", strings.Join(parts, ";"))
		if code == 124 {
			// A timeout would just burn another 25 minutes per filter.
			for _, r := range reqs {
				c.finish(r, "TIMEOUT", 124, raw+"\nTIMEOUT: merged test run exceeded 25 min\n")
			}
			return
		}
		results, errCode, ok := decodeTestReport([]byte(raw))
		if ok {
			c.splitResults(reqs, results)
			return
		}
		io.WriteString(c.stderr, "utk queue: merged run failed ("+errCode+"); rerunning each filter alone\n")
	}
	for _, r := range reqs {
		raw, code := queueExec(c.cwd, queueTestTimeout, rawReportEnv, utkSelf(), "run_tests", "--mode", utkTestMode(mode), "--filter", r.Args.Filter)
		if code == 124 {
			c.finish(r, "TIMEOUT", 124, raw+"\nTIMEOUT: test run exceeded 25 min\n")
			continue
		}
		results, errCode, ok := decodeTestReport([]byte(raw))
		if !ok {
			if errCode == "" {
				errCode = "NO_REPORT"
			}
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
		parts := splitTestFilter(r.Args.Filter)
		for _, t := range results {
			name := strings.ToLower(t.FullName)
			for _, part := range parts {
				if strings.Contains(name, part) {
					mine = append(mine, t)
					break // overlapping parts ("Foo;FooBar") must not list a test twice
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

// runSceneJobs is step 5: the only exclusive work. Shortest first — a 5 s
// reference fix must not wait behind a 15 min level build.
func (c *cycle) runSceneJobs() {
	jobs := c.byKind("scene")
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].Args.EstS < jobs[j].Args.EstS })
	for _, r := range jobs {
		out, code := c.runJobCommand(r, []string{"UNITY_LOCK_OWNER=coordinator", "UNITY_QUEUE_REQUEST=" + r.ID})
		c.finishCommand(r, out, code)
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
		if code == 0 && strings.Contains(out, `"playMode":"playing"`) {
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
		out, code := c.runJobCommand(r, []string{"UNITY_LOCK_OWNER=coordinator", "UNITY_QUEUE_REQUEST=" + r.ID})
		c.finishCommand(r, out, code)
	}
}

func (c *cycle) playSession(reqs []queueRequest) {
	if out, code := queueExec(c.cwd, 2*time.Minute, nil, utkSelf(), "editor", "play"); code != 0 || !c.waitPlaying() {
		for _, r := range reqs {
			c.finish(r, "FAIL", 1, "could not enter Play mode\n"+out)
		}
		return
	}
	for i, r := range reqs {
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
			if out, code := queueExec(c.cwd, 2*time.Minute, nil, utkSelf(), "editor", "play"); code != 0 || !c.waitPlaying() {
				// No result would mean the server re-queues them forever.
				for _, rest := range reqs[i+1:] {
					c.finish(rest, "FAIL", 1, "could not re-enter Play mode after a timed-out script\n"+out)
				}
				break
			}
		}
	}
	queueExec(c.cwd, 2*time.Minute, nil, utkSelf(), "editor", "stop")
}
