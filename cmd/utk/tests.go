package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// The official CLI polls its own async runs for 10 minutes; match it rather
// than invent a second number a suite could sit between. A var for tests.
var testPollBudget = 10 * time.Minute

const (
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
	// Not the hand-off envelope: it is success with no results, and the queue
	// read it as an empty report (NO_TESTS_MATCHED) while the run went on.
	env, _ := json.Marshal(map[string]any{
		"success": false,
		"errors":  []map[string]string{{"code": "TEST_RUN_TIMEOUT", "message": "tests still running after " + testPollBudget.String()}},
	})
	return env, 1
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

// namespaceFilter matches a filter that names the test namespace or assembly
// (`Game.Tests`, `Game.Tests.EditMode`) rather than a class: it hits every test.
var namespaceFilter = regexp.MustCompile(`\.Tests(\.[A-Za-z]+Mode)?\.?$`)

// wholeSuite says why a run_tests would run the whole suite, or "" when it is
// scoped. 1000+ tests hold a shared Editor for minutes and every other agent
// on it waits behind them; agents ran it anyway despite the skill's rule.
// UTK_FULL_SUITE=1 lets it through when the user asked for the full suite.
func wholeSuite(args []string) string {
	if os.Getenv("UTK_FULL_SUITE") == "1" {
		return ""
	}
	filter := findFlag(args, "--filter")
	switch {
	case filter == "":
		return "no --filter: that is the whole suite"
	case strings.EqualFold(findFlag(args, "--filter_type"), "assembly"):
		return "--filter_type assembly runs the whole test assembly"
	}
	// Each "A;B" part runs on its own, so one namespace part is the whole
	// suite — and through the queue it lands in everyone's merged run. Split
	// raw, not with splitTestFilter: namespaceFilter is case-sensitive.
	for _, part := range strings.Split(filter, ";") {
		if part = strings.TrimSpace(part); namespaceFilter.MatchString(part) {
			return "--filter " + part + " is a namespace and matches every test"
		}
	}
	return ""
}

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
	parts := splitTestFilter(filter)
	if len(parts) == 0 {
		return ""
	}
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

// toolTestsSnippet lists test methods in Category("Tool") — on the method or
// its fixture — that carry no [Explicit] on either. The pipeline's full run
// skips [Explicit] but cannot exclude a category, so [Explicit] is the guard.
const toolTestsSnippet = `var bad = new System.Collections.Generic.List<string>();
foreach (var asm in System.AppDomain.CurrentDomain.GetAssemblies()) {
  bool nunit = false; foreach (var r in asm.GetReferencedAssemblies()) if (r.Name == "nunit.framework") nunit = true;
  if (!nunit) continue;
  System.Type[] ts; try { ts = asm.GetTypes(); } catch { continue; }
  foreach (var t in ts) {
    bool tTool = false, tExp = false;
    foreach (var a in t.GetCustomAttributes(true)) { var n = a.GetType().Name; if (n == "ExplicitAttribute") tExp = true; if (n == "CategoryAttribute" && (string)a.GetType().GetProperty("Name").GetValue(a) == "Tool") tTool = true; }
    foreach (var m in t.GetMethods(System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.Static | System.Reflection.BindingFlags.DeclaredOnly)) {
      bool test = false, tool = tTool, exp = tExp;
      foreach (var a in m.GetCustomAttributes(true)) { var n = a.GetType().Name; if (n == "TestAttribute" || n == "TestCaseAttribute" || n == "TestCaseSourceAttribute" || n == "UnityTestAttribute") test = true; if (n == "ExplicitAttribute") exp = true; if (n == "CategoryAttribute" && (string)a.GetType().GetProperty("Name").GetValue(a) == "Tool") tool = true; }
      if (test && tool && !exp) bad.Add(t.Name + "." + m.Name);
    }
  }
}
return "TOOLTESTS:" + string.Join(",", bad.ToArray());`

// toolTestsNotExplicit returns the offending names, a "(could not scan …)"
// reason, or "" when the suite is clear.
func toolTestsNotExplicit(project string) string {
	var out bytes.Buffer
	args := []string{"exec", toolTestsSnippet, "--timeout", "15000"}
	if project != "" {
		args = append(args, "--project-path", project)
	}
	// Fails closed: a scan that did not run cannot clear a suite that may hold
	// the Editor for minutes.
	if run(args, &out, io.Discard) != 0 {
		return "(could not scan: the Editor did not run the check)"
	}
	s := out.String()
	i := strings.Index(s, "TOOLTESTS:")
	if i < 0 {
		return "(could not scan: no TOOLTESTS line in the answer)"
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(s[i+len("TOOLTESTS:"):]), `"`))
}

func editorProjectOf(args []string) string {
	if p := findFlag(args, "--project-path"); p != "" {
		return p
	}
	return localProjectRoot()
}
