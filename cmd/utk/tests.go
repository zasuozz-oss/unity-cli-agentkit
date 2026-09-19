package main

import (
	"encoding/json"
	"fmt"
	"io"
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
)

// pollTests waits out an async run_tests and returns test_status's report in
// its place, so `utk run_tests` still answers with results rather than an
// acknowledgement. initial is the hand-off response: anything but a successful
// one is handed straight back for the caller's normal error path.
func pollTests(initial []byte, stderr io.Writer) ([]byte, int) {
	if env, err := official.Parse(initial); err != nil || !env.Success {
		return initial, 0
	}
	args := []string{"command", "test_status", "--json", "--no-banner"}
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
	}
	fmt.Fprintf(stderr, "utk: tests still running after %s — poll `utk test_status` for the result\n",
		testPollBudget)
	return initial, 1
}

// testRunFinished reads test_status's own status field. The payload arrives as
// a JSON *string* rather than an object, so it needs unwrapping first — the
// same extra layer internal/filter's parseTests strips.
func testRunFinished(payload []byte) bool {
	if len(payload) > 0 && payload[0] == '"' {
		var s string
		if json.Unmarshal(payload, &s) != nil {
			return false
		}
		payload = []byte(s)
	}
	var d struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(payload, &d) != nil {
		return false
	}
	return d.Status != "running"
}
