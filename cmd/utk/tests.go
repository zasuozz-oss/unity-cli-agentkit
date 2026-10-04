package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

const (
	// The official CLI polls its own async runs for 10 minutes; match it rather
	// than invent a second number a suite could sit between.
	testPollBudget = 10 * time.Minute
	// Each poll spawns a `unity` process (~5s on its own), so a short interval
	// buys nothing — it only stacks startups back to back.
	testPollStep = 2 * time.Second
	// Long enough that a normal suite never sees the hint, short enough to
	// land before the 120s default tool timeout it is about.
	testBackgroundHint = 90 * time.Second
	// A console read is one more `unity` spawn, so look for a crashed run only
	// this often rather than on every poll.
	testCrashCheck = 15 * time.Second
)

// testRunnerCrash is what the Unity Test Framework logs when a run dies inside
// the framework itself (e.g. "Test tree is not available for
// PostbuildCleanupTask"). RunFinished never fires after it, so the pipeline's
// request file stays put and test_status answers "running" until utk gives up.
const testRunnerCrash = "An unexpected error happened while running tests"

// pollTests waits out an async run_tests and returns test_status's report in
// its place, so `utk run_tests` still answers with results rather than an
// acknowledgement. initial is the hand-off response: anything but a successful
// one is handed straight back for the caller's normal error path. start is
// when run_tests was sent; console entries before it belong to earlier runs.
func pollTests(initial []byte, start time.Time, projectPath string, stderr io.Writer) ([]byte, int) {
	if env, err := official.Parse(initial); err != nil || !env.Success {
		return initial, 0
	}
	args := []string{"command", "test_status", "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	crashAt := time.Now().Add(testCrashCheck)
	deadline := time.Now().Add(testPollBudget)
	// The Bash tool most agents call this from waits 120s by default and then
	// backgrounds the command — after which they wrote grep-and-sleep loops
	// over its output file. Say so once, while there is still time to act.
	hintAt := time.Now().Add(testBackgroundHint)
	for time.Now().Before(deadline) {
		time.Sleep(testPollStep)
		if !hintAt.IsZero() && time.Now().After(hintAt) {
			fmt.Fprintln(stderr, "utk: this run is taking a while — for long suites run `utk run_tests` with run_in_background so you are notified when it ends, instead of polling")
			hintAt = time.Time{}
		}
		raw, code := official.CaptureRetry(args, stderr)
		env, err := official.Parse(raw)
		if err != nil || !env.Success {
			return raw, code
		}
		if testRunFinished(env.Payload(true)) {
			return raw, code
		}
		if time.Now().After(crashAt) {
			crashAt = time.Now().Add(testCrashCheck)
			if msg := testCrashed(consoleSince(start, projectPath)); msg != "" {
				return abandonTests(msg, projectPath, stderr)
			}
		}
	}
	fmt.Fprintf(stderr, "utk: tests still running after %s — poll `utk test_status` for the result\n",
		testPollBudget)
	return initial, 1
}

// testRunFinished reads test_status's own status field.
func testRunFinished(payload []byte) bool {
	d, ok := testReport(payload)
	return ok && d.Status != "running"
}

// testReport decodes a test_status payload. It arrives as a JSON *string*
// rather than an object, so it needs unwrapping first — the same extra layer
// internal/filter's parseTests strips.
func testReport(payload []byte) (d struct {
	Status  string `json:"status"`
	Results []struct {
		FullName string `json:"FullName"`
	} `json:"results"`
}, ok bool) {
	if len(payload) > 0 && payload[0] == '"' {
		var s string
		if json.Unmarshal(payload, &s) != nil {
			return d, false
		}
		payload = []byte(s)
	}
	return d, json.Unmarshal(payload, &d) == nil
}

// waitTestSlot holds a run_tests back while another async run owns the Editor.
// The pipeline keeps one run per Editor and every run_tests cancels the one in
// flight, so agents sharing an Editor kept killing each other's runs and then
// rerunning them. Anything but a clear "running" — no run, a finished report,
// an unreadable answer — lets the caller go ahead.
// ponytail: check-then-start races when two agents check in the same instant;
// unity-job.sh's lock closes that window if it matters.
func waitTestSlot(projectPath string, stderr io.Writer) int {
	args := []string{"command", "test_status", "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	deadline := time.Now().Add(testPollBudget)
	for waited := false; ; waited = true {
		raw, _ := official.CaptureRetry(args, io.Discard)
		env, err := official.Parse(raw)
		if err != nil || !env.Success || testRunFinished(env.Payload(true)) {
			return 0
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(stderr, "utk: another test run still holds this Editor after %s; not starting yours over it.\n", testPollBudget)
			fmt.Fprintln(stderr, "  If nothing is really running (a crashed run never reports): `utk cancel_tests`, then retry.")
			return 1
		}
		if !waited {
			fmt.Fprintln(stderr, "utk: another test run (another agent?) is in progress on this Editor; waiting for it instead of cancelling it")
		}
		time.Sleep(testPollStep)
	}
}

// foreignTest returns a test in the report that the run's own testName filter
// could not have selected, or "". The report test_status hands back is
// whatever run finished last on the Editor, so an agent whose run another
// agent replaced got that agent's results — 1041 tests for a one-test filter —
// and took them for its own. Only the testName filter can be checked: the
// report carries neither assembly nor category.
func foreignTest(payload []byte, filter, filterType string) string {
	if filter == "" || filterType != "" && !strings.EqualFold(filterType, "testName") {
		return ""
	}
	d, ok := testReport(payload)
	if !ok {
		return ""
	}
	f := strings.ToLower(filter)
	for _, r := range d.Results {
		if !strings.Contains(strings.ToLower(r.FullName), f) {
			return r.FullName
		}
	}
	return ""
}

// foreignReport is the failed envelope for a report that belongs to another run.
func foreignReport(name string, stderr io.Writer) ([]byte, int) {
	fmt.Fprintln(stderr, "utk: these results are not your run's — another run_tests on this Editor replaced it.")
	fmt.Fprintln(stderr, "  Rerun yours; for a shared Editor queue through unity-job.sh, or give each agent its own Editor.")
	env, _ := json.Marshal(map[string]any{
		"success": false,
		"errors":  []map[string]string{{"code": "TEST_RESULT_FOREIGN", "message": "report contains " + name + ", which your --filter does not match"}},
	})
	return env, 1
}

// testCrashed returns the framework's crash entry among logs, or "".
func testCrashed(logs []consoleEntry) string {
	for _, l := range logs {
		if strings.Contains(l.Message, testRunnerCrash) {
			return l.Message
		}
	}
	return ""
}

// abandonTests cancels a run the framework has already lost — clearing the
// request file, so test_status stops claiming "running" — and reports it as a
// failed envelope for the caller's normal error path.
func abandonTests(msg, projectPath string, stderr io.Writer) ([]byte, int) {
	args := []string{"command", "cancel_tests", "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	official.CaptureRetry(args, io.Discard)
	fmt.Fprintln(stderr, "utk: the Test Runner crashed mid-run and would never report; cancelled it.")
	fmt.Fprintln(stderr, "  Often a stale state after a domain reload or a test left play mode on: run")
	fmt.Fprintln(stderr, "  `utk editor status`, then retry. Full error: `utk console --type error`.")
	env, _ := json.Marshal(map[string]any{
		"success": false,
		"errors":  []map[string]string{{"code": "TEST_RUN_CRASHED", "message": firstLine(msg)}},
	})
	return env, 1
}
