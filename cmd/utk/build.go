package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

const (
	// An Android IL2CPP build of a mid-size project runs 5–20 minutes; the
	// loops agents hand-rolled around build_status gave up at 4–7 minutes.
	buildPollBudget = 30 * time.Minute
	// build_status answers off a status object, not the main thread, so a
	// longer step buys nothing but latency at the end; a shorter one only
	// stacks `unity` startups.
	buildPollStep = 5 * time.Second
	// A failed build repeats one root cause per scene/assembly; the first few
	// lines name it.
	buildErrorCap = 12
)

// pollBuild waits out the build `utk build` just queued and answers with
// build_status's final report instead of the "queued" hand-off. Every caller
// otherwise wrote the same `until build_status | grep Succeeded; sleep 15`
// loop with a budget guessed per call. Run it in the background and the
// harness notifies when it exits — no poll round-trips at all.
//
// initial is the hand-off response: anything but a successful envelope, or a
// call that queued nothing (dry_run, refused --confirm), is handed straight back.
func pollBuild(initial []byte, projectPath string, stderr io.Writer) ([]byte, int) {
	env, err := official.Parse(initial)
	if err != nil || !env.Success {
		return initial, 0
	}
	if st, _ := buildState(env.Payload(true)); st != "queued" && st != "building" {
		return initial, 0
	}
	args := []string{"command", "build_status", "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	deadline := time.Now().Add(buildPollBudget)
	for time.Now().Before(deadline) {
		time.Sleep(buildPollStep)
		raw, code := official.CaptureRetry(args, stderr)
		env, err := official.Parse(raw)
		if err != nil || !env.Success {
			return raw, code
		}
		st, result := buildState(env.Payload(true))
		if st == "queued" || st == "building" {
			continue
		}
		if st == "" {
			fmt.Fprintln(stderr, "utk: could not read build_status; reporting it as-is")
		}
		if st == "completed" && result != "Succeeded" && code == 0 {
			code = 1
		}
		return raw, code
	}
	fmt.Fprintf(stderr, "utk: still building after %s — poll `utk build_status` for the result\n", buildPollBudget)
	return initial, 1
}

// buildState reads build_status's status (idle | queued | building |
// completed) and, once completed, the BuildReport result (Succeeded | Failed |
// Cancelled).
func buildState(payload []byte) (status, result string) {
	var d struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if json.Unmarshal(payload, &d) != nil {
		return "", ""
	}
	return d.Status, d.Result
}

// trimBuildReport drops the per-file inventory from a completed build_status
// payload. A 1.2 GB Android build lists thousands of {path, role, sizeBytes}
// entries — hundreds of KB nobody reads to learn whether the build passed —
// while the answer is the dozen scalar fields plus the errors. Anything
// else, including an in-flight status, passes through untouched.
func trimBuildReport(payload []byte) []byte {
	var d map[string]json.RawMessage
	if json.Unmarshal(payload, &d) != nil || string(d["status"]) != `"completed"` {
		return payload
	}
	dropped := 0
	for _, k := range []string{"files", "packedAssets", "buildSteps", "strippingInfo"} {
		if _, ok := d[k]; ok {
			delete(d, k)
			dropped++
		}
	}
	if dropped == 0 {
		return payload
	}
	if errs, ok := d["errors"]; ok {
		var list []json.RawMessage
		if json.Unmarshal(errs, &list) == nil && len(list) > buildErrorCap {
			list = append(list[:buildErrorCap], json.RawMessage(strconv.Quote(
				"…+"+strconv.Itoa(len(list)-buildErrorCap)+" more errors (--raw for all)")))
			d["errors"], _ = json.Marshal(list)
		}
	}
	d["trimmed"] = json.RawMessage(`"files/packedAssets/buildSteps omitted (--raw for the full BuildReport)"`)
	out, err := json.Marshal(d)
	if err != nil {
		return payload
	}
	return out
}
