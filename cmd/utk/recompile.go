package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

const (
	// A cold full recompile of a large project runs a few minutes; the loops
	// agents hand-rolled around this call gave up between 60s and 120s and then
	// reported a compile that was still running as if it had stalled.
	recompilePollBudget = 5 * time.Minute
	// Each poll spawns a `unity` process (~1s on its own), so a shorter interval
	// only stacks startups back to back. recompile_status reads a status file
	// off the main thread, so polling never queues behind the compile itself.
	recompilePollStep = time.Second
)

// pollRecompile waits out the compile `utk editor refresh` just triggered and
// answers with recompile_status's final report instead of the "started" notice.
//
// The tool hands off because a successful compile tears down the domain — and
// with it the HTTP server that would have replied. Every caller therefore had
// to write the same shell loop (refresh → poll recompile_status → console),
// three round-trips deep, with a sleep budget guessed per call. Doing it here
// costs one command and cannot guess wrong.
//
// initial is the hand-off response: anything but a successful envelope, or a
// refresh that found nothing to compile, is handed straight back.
func pollRecompile(initial []byte, projectPath string, stderr io.Writer) ([]byte, int) {
	env, err := official.Parse(initial)
	if err != nil || !env.Success {
		return initial, 0
	}
	if st, _ := recompileState(env.Payload(true)); st == "up_to_date" || st == "completed" {
		return initial, 0
	}
	args := []string{"command", "recompile_status", "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	deadline := time.Now().Add(recompilePollBudget)
	for time.Now().Before(deadline) {
		time.Sleep(recompilePollStep)
		raw, code := official.CaptureRetry(args, stderr)
		env, err := official.Parse(raw)
		if err != nil || !env.Success {
			return raw, code
		}
		st, failed := recompileState(env.Payload(true))
		if st == "triggered" || st == "compiling" {
			continue
		}
		// Anything else is as final as this is going to get: `idle` means the
		// status file is gone (a cleared Temp/), and an unreadable status is
		// not going to become readable. Hand it back rather than spend the
		// whole budget waiting for a word that is never coming.
		if st == "" {
			fmt.Fprintln(stderr, "utk: could not read recompile_status; reporting it as-is")
		}
		// The compile errors are in the payload the caller is about to read;
		// only the exit code needs to agree, so `utk editor refresh && …` stops.
		if failed && code == 0 {
			code = 1
		}
		return raw, code
	}
	fmt.Fprintf(stderr, "utk: still compiling after %s — poll `utk recompile_status` for the result\n",
		recompilePollBudget)
	fmt.Fprintln(stderr, "  (stuck at `triggered` means the import never started: focus the Unity window)")
	return initial, 1
}

// recompileState reads recompile_status's own report: one of idle, triggered,
// compiling, completed, up_to_date, plus whether the compile produced errors.
// The payload arrives as a JSON *string* rather than an object — the tool
// returns the status file's contents verbatim — so it needs unwrapping first.
func recompileState(payload []byte) (status string, failed bool) {
	if len(payload) > 0 && payload[0] == '"' {
		var s string
		if json.Unmarshal(payload, &s) != nil {
			return "", false
		}
		payload = []byte(s)
	}
	var d struct {
		Status string `json:"status"`
		Failed bool   `json:"failed"`
	}
	if json.Unmarshal(payload, &d) != nil {
		return "", false
	}
	return d.Status, d.Failed
}
