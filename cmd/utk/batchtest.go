package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zasuo/unity-cli-agentkit/internal/filter"
	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// dropFlag returns args without the named `--flag v` / `--flag=v` pair.
func dropFlag(args []string, name string) []string {
	out := args[:0:0]
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			i++
			continue
		}
		if strings.HasPrefix(args[i], name+"=") {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// findFlag returns the value of a `--flag v` / `--flag=v` pair, or "".
func findFlag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return ""
}

// runBatchTest drives `unity test`, whose JSON says only where it wrote its
// NUnit report — no counts, no failures. Streamed unfiltered it prints nothing
// an agent can act on, buried in the CLI's telemetry stack traces. So: read the
// report it names, render it like `run_tests`, and let failures reach $?.
func runBatchTest(unityArgs []string, mode string, stdout, stderr io.Writer) int {
	noise := &filter.NoiseFilter{W: stderr}
	raw, code := official.Capture(append(unityArgs, "--json", "--no-banner"), noise)
	noise.Close()

	env, err := official.Parse(raw)
	if err != nil {
		stdout.Write(raw) // not an envelope → loss-safe passthrough
		return code
	}
	for _, w := range env.Warnings {
		fmt.Fprintln(stderr, "utk: warning:", w.Message)
	}
	if !env.Success {
		// beta.6 split this command's exits: a red suite is now exit 8 with a
		// TESTS_FAILED envelope whose data is null, so the report path the
		// success path reads is simply absent — while the report itself is
		// still written. Every other code (TEST_RUN_ERROR, TEST_TIMED_OUT)
		// means the run never reached a verdict, so there is nothing to render.
		if len(env.Errors) == 0 || env.Errors[0].Code != "TESTS_FAILED" {
			return renderEnvelopeErrors(env, code, stderr)
		}
		// Recover the path from the flag we sent. The CLI resolves a relative
		// --output against the working directory, not the project, which is
		// what os.ReadFile does with it too; unset, its default is this name.
		report := findFlag(unityArgs, "--output")
		if report == "" {
			report = "test-results.xml"
		}
		return renderNUnit(report, raw, mode, code, stdout, stderr)
	}

	var d struct {
		ProjectPath string `json:"projectPath"`
		Output      string `json:"output"`
	}
	if json.Unmarshal(env.Data, &d) != nil || d.Output == "" {
		stdout.Write(raw)
		return code
	}
	report := d.Output
	if !filepath.IsAbs(report) {
		report = filepath.Join(d.ProjectPath, report)
	}
	return renderNUnit(report, raw, mode, code, stdout, stderr)
}

// renderNUnit compacts the report at path and returns the exit code to use.
// raw is the envelope to fall back to when the report cannot be used.
func renderNUnit(report string, raw []byte, mode string, code int, stdout, stderr io.Writer) int {
	xmlBytes, rerr := os.ReadFile(report)
	if rerr != nil {
		fmt.Fprintln(stderr, "utk: test: cannot read results:", rerr)
		stdout.Write(raw)
		if code == 0 {
			code = 1
		}
		return code
	}
	out, failed, ok := filter.NUnit(xmlBytes, mode)
	if !ok {
		stdout.Write(xmlBytes) // unrecognised report → hand over everything
		return code
	}
	stdout.Write(out)
	fmt.Fprintln(stderr, "utk: test: full report at", report)
	if saved := len(xmlBytes) - len(out); saved > 0 {
		fmt.Fprintf(stderr, "utk: test saved %d bytes (~%d tokens)\n",
			saved, filter.EstimateTokens(saved))
	}
	// beta.6 exits 8 on a red suite, but a CLI that still exits 0 on one would
	// have every `utk test && ship` read that as green.
	if failed && code == 0 {
		code = 1
	}
	return code
}
