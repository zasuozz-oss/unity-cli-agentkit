package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/filter"
	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run holds all routing/exec/print logic, taking argv (without the program
// name) and injectable stdout/stderr so it is testable without a real `unity`
// binary on PATH driving os.Stdout directly.
func run(args []string, stdout, stderr io.Writer) int {
	// Split utk's own flags across the whole argv, not just the part after the
	// verb: `utk --raw console` otherwise took --raw as the command name and ran
	// `unity command --raw console`, which fails with an unrelated error.
	args, opts := filter.SplitUtkFlags(args)
	if len(args) < 1 {
		fmt.Fprintln(stderr, "utk: missing command (try: utk status | utk list | utk init)")
		return 2
	}
	cmd := args[0]
	rest := args[1:]

	if cmd == "init" {
		return runInit(rest) // implemented in init.go
	}
	// `utk --help` used to map onto `unity command --help`, which documents the
	// official CLI and never mentions a single utk verb — an agent that runs it
	// to orient itself learns nothing about the tool it is holding.
	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	// Unity's ForceReserializeAssets ignores paths that do not exist, and the
	// snippet we generate returns a hardcoded "reserialized" either way — so a
	// typo'd path reports a successful validation that never happened.
	if cmd == "reserialize" {
		if code := checkReserializePaths(rest, stderr); code != 0 {
			return code
		}
	}
	// import_asset wants an absolute --source and only reports a missing file
	// after the Editor round-trip; rewrite `utk import <file> [dest]` locally
	// and refuse a path that is not on disk before anything is sent.
	if cmd == "import" {
		var code int
		if rest, code = prepImportArgs(rest, stderr); code != 0 {
			return code
		}
	}

	// `utk list <tool>` = detail mode: same unity call, different rendering.
	// A tool name only means something on the filtered path, and only one at a
	// time — `unity list` always returns all 140. Refuse the other forms instead
	// of dropping the argument and answering a question nobody asked.
	detailTool := ""
	if cmd == "list" && len(rest) > 0 {
		if len(rest) > 1 {
			fmt.Fprintln(stderr, "utk: list takes at most one tool name, got:", strings.Join(rest, " "))
			return 2
		}
		if opts.Raw {
			fmt.Fprintln(stderr, "utk: --raw has no per-tool mode (unity list always returns all tools)")
			fmt.Fprintln(stderr, "  drop --raw for the tool's schema, or drop the tool name for the raw listing")
			return 2
		}
		// Look the tool up under the name it actually has upstream: `utk exec`
		// runs the `eval` tool, so `utk list exec` must show eval's schema
		// rather than claim no such tool exists. mapVerb is the one place that
		// knows the aliases, so ask it instead of keeping a second table.
		detailTool = rest[0]
		if a, _, _ := mapVerb(detailTool, nil); len(a) >= 2 && a[0] == "command" {
			detailTool = a[1]
		}
		rest = nil
	}

	// com.unity.pipeline 0.5.0 starts enforcing eval/eval_file's own --timeout
	// (milliseconds, default 5000), and the CLI keeps any --timeout before `--`
	// for itself (seconds, transport) — so the tool's budget was unreachable and
	// every snippet past 5s died no matter what the caller raised. Take one
	// budget in ms (the unit `utk list eval` documents) and route it to both
	// sides below, once utk's other flags are in place. Default restores the
	// ~60s headroom 0.4.0 gave in practice; reserialize rides eval, so a large
	// asset batch gets the same room.
	execMS := 0
	if !opts.Raw && (cmd == "exec" || cmd == "reserialize") {
		execMS = 60000
		if v := findFlag(rest, "--timeout"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "utk: --timeout must be a positive integer (milliseconds), got %q\n", v)
				return 2
			}
			execMS = n
			rest = dropFlag(rest, "--timeout")
		}
	}

	// `utk build` waits for the build it queues (pollBuild); --wait false keeps
	// the tool's own hand-off. The flag is utk's, so it goes before the schema
	// check would refuse it as unknown to the build tool.
	buildWait := true
	if cmd == "build" {
		if v := findFlag(rest, "--wait"); v != "" {
			buildWait = v != "false"
			rest = dropFlag(rest, "--wait")
		}
	}

	unityArgs, kind, only := mapVerb(cmd, rest)
	opts.ConsoleOnly = only

	// With two Editors open the CLI picks a reachable instance by itself: it
	// does not read cwd. A command run inside project A is then answered by
	// project B — silently, and destructively for anything that writes. Name the
	// project cwd is in, so a wrong or unreachable target fails instead.
	if len(unityArgs) > 0 && unityArgs[0] == "command" && !hasFlag(rest, "--project-path") {
		if root := localProjectRoot(); root != "" {
			unityArgs = append(unityArgs, "--project-path", root)
		}
	}

	// The official CLI drops parameters it does not recognise instead of
	// refusing them, so a typo silently changes what the call means:
	// `open_scene --mode Additive` loses --mode and replaces the open scene.
	// Catch it here, against a local schema cache, before anything is sent.
	if code := checkFlags(unityArgs, stderr); code != 0 {
		return code
	}

	// Raw or unfiltered verbs stream straight through — no --json, no capture,
	// byte-identical to calling `unity` with the mapped argv.
	if opts.Raw || kind == "" {
		return official.Run(unityArgs, stdout, stderr)
	}

	// The batchmode runner reports through a results file, not the envelope,
	// so it needs its own path rather than one of the payload filters.
	if kind == "batchtest" {
		return runBatchTest(unityArgs, findFlag(rest, "--mode"), stdout, stderr)
	}

	// Filtered path: always machine output (human format + --quiet swallows
	// eval results — measured on beta.3), captured then compacted.
	unityArgs = append(unityArgs, "--json", "--no-banner")
	// The `--` chunk must come last: everything after it goes to the tool as
	// parameters, so a CLI flag appended behind it would be silently eaten.
	// Transport gets 10s on top of the tool budget so the tool's own timeout
	// error wins the race and reaches the caller.
	baseArgs := unityArgs
	if execMS > 0 {
		unityArgs = append(unityArgs, execTimeoutArgs(execMS)...)
	}
	// Stamped before the call so every entry the snippet logs falls after it.
	execStart := time.Now().UTC()
	raw, code := official.CaptureRetry(unityArgs, stderr)
	// com.unity.pipeline < 0.5.0 refuses a tool --timeout above its own cap
	// ("Timeout must be between 1ms and 30000ms"), so the 60000ms default
	// kills every plain `utk exec` on a project that has not upgraded yet.
	// The refusal names the cap; retry once clamped to it instead of failing.
	if execMS > 0 {
		if cap := evalTimeoutCap(raw); cap > 0 && execMS > cap {
			fmt.Fprintf(stderr, "utk: this project's pipeline caps eval timeout at %dms; retrying clamped\n", cap)
			fmt.Fprintln(stderr, "  (upgrade for longer budgets: unity pipeline install --force)")
			unityArgs = append(baseArgs, execTimeoutArgs(cap)...)
			execStart = time.Now().UTC()
			raw, code = official.CaptureRetry(unityArgs, stderr)
		}
	}
	// A main-thread timeout means a modal dialog is almost certainly pumping
	// its own event loop, and the request never ran at all. Answering the
	// dialog is the fix; only then is there anything to retry.
	if mainThreadTimeout(string(raw)) && answerModal(stderr) {
		execStart = time.Now().UTC()
		raw, code = official.CaptureRetry(unityArgs, stderr)
	}
	// A snippet written the way the surrounding project is written — bare
	// `Object.FindAnyObjectByType<T>()` — cannot compile inside eval, where
	// `System.object` is equally in scope. It is the single most common exec
	// failure, and the round-trip that fixes it only ever qualifies the name.
	if cmd == "exec" && ambiguousObject(raw) {
		if retryArgs, cleanup, ok := qualifyObjectArgs(unityArgs); ok {
			fmt.Fprintln(stderr, "utk: bare `Object.` is ambiguous with `object` inside a snippet; retried as `UnityEngine.Object.`")
			execStart = time.Now().UTC()
			raw, code = official.CaptureRetry(retryArgs, stderr)
			cleanup()
		}
	}
	// mapVerb sent run_tests off asynchronously to get out from under the CLI's
	// 30s ceiling; the hand-off answers "running", not results. Wait here so the
	// verb keeps its meaning. An explicit --async_tests is left alone: asking for
	// the hand-off and getting a block instead would be the surprise.
	if cmd == "run_tests" && code == 0 && !hasFlag(rest, "--async_tests") {
		raw, code = pollTests(raw, execStart, findFlag(unityArgs, "--project-path"), stderr)
	}
	// `recompile` hands off the same way and for the same reason (the domain
	// reload kills the reply), so `utk editor refresh` answered "started" and
	// left the caller to poll. Wait here too: the verb's whole purpose is
	// knowing whether the edited C# compiles.
	if cmd == "editor" && len(rest) > 0 && rest[0] == "refresh" && code == 0 {
		raw, code = pollRecompile(raw, findFlag(unityArgs, "--project-path"), stderr)
	}
	// `build` queues and returns; the 30-minute wait behind it was every
	// caller's own `until build_status …; sleep 15` loop.
	if cmd == "build" && buildWait && code == 0 {
		raw, code = pollBuild(raw, findFlag(unityArgs, "--project-path"), stderr)
	}

	env, err := official.Parse(raw)
	if err != nil {
		// Not an envelope (older CLI? plain text?) → loss-safe passthrough.
		stdout.Write(raw)
		return code
	}
	for _, w := range env.Warnings {
		fmt.Fprintln(stderr, "utk: warning:", w.Message)
	}
	if !env.Success {
		// Asking for the scene that is already active is a failure upstream.
		// Report the state the caller wanted rather than an error about it.
		if path := findFlag(rest, "--path"); len(unityArgs) > 1 &&
			unityArgs[1] == "set_active_scene" && activeSceneIs(path) {
			// Stay JSON like every other filtered answer, so a caller parsing
			// stdout does not have to special-case this one.
			fmt.Fprintf(stdout, "{\"activeScene\":%q,\"alreadyActive\":true}\n", path)
			return 0
		}
		return renderEnvelopeErrors(env, code, stderr)
	}

	// `unity command <tool>` nests its payload under data.result; the top-level
	// verbs behind `list`/`status` put theirs directly at data.
	payload := env.Payload(kind != "list" && kind != "status")
	if payload == nil {
		stdout.Write(raw) // unexpected shape → emit everything we got
		return code
	}

	// Both of the screenshot tool's hazards — a capture that silently missed the
	// overlay UI, and a native-resolution PNG nobody needs at that size — can
	// only be addressed once it has answered with a path.
	if cmd == "screenshot" {
		// An explicit size was asked of the tool, and the overlay re-capture
		// cannot honour it (ScreenCapture has no width/height) — such a call
		// keeps the camera render, and only hears about what is missing.
		sized := hasFlag(rest, "--width") || hasFlag(rest, "--height")
		payload = fixOverlayShot(payload, findFlag(rest, "--view"), findFlag(unityArgs, "--project-path"), sized, stderr)
		if !sized {
			payload = shrinkShot(payload, opts.Max, stderr)
		}
	}

	// The completed BuildReport inventories every output file; the verdict is
	// a dozen scalars and the errors.
	if !opts.Raw && (cmd == "build" || cmd == "build_status") {
		payload = trimBuildReport(payload)
	}

	var out []byte
	if detailTool != "" {
		out = filter.ListDetail(payload, detailTool)
	} else {
		out = filter.Apply(kind, payload, opts)
	}
	// eval discards Debug.Log, so a snippet whose whole answer is what it logged
	// comes back as `{}` and costs a second `utk console` to read. Fold the
	// snippet's own log into the answer it belongs to.
	if cmd == "exec" && code == 0 {
		if logs := snippetLogs(execStart, findFlag(unityArgs, "--project-path")); len(logs) > 0 {
			merged := make([]byte, 0, len(out)+len(logs)+1)
			merged = append(merged, out...)
			if len(merged) > 0 && merged[len(merged)-1] != '\n' {
				merged = append(merged, '\n')
			}
			out = append(merged, logs...)
		}
	}
	stdout.Write(out)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		stdout.Write([]byte("\n"))
	}
	// A command that succeeded while its tests failed still exits 0 upstream.
	// Every `utk run_tests && ship` reads that as green.
	if kind == "tests" && code == 0 && filter.TestsFailed(payload) {
		code = 1
	}
	// Same hazard one level down: the envelope reports the transport, so a tool
	// that refused the request or could not find its input also arrives as
	// exit 0. The payload printed above says why; this makes the exit code agree.
	if code == 0 && filter.PayloadFailed(payload) {
		code = 1
	}
	if cmd == "clear_console" && code == 0 {
		fmt.Fprintln(stderr, "utk: WARNING: com.unity.pipeline <= 0.4.0-exp.1 stops capturing logs after a clear (fixed in 0.5.0).")
		fmt.Fprintln(stderr, "  `utk console` will report 0 entries — including real errors — until the next")
		fmt.Fprintln(stderr, "  domain reload. Run `utk editor refresh` (or play/stop) before trusting it.")
	}
	// No savings line for `utk list <tool>`: the baseline would be the full
	// 140-tool listing, which is not what was asked for, so the number claims a
	// saving against output nobody wanted.
	if saved := len(raw) - len(out); saved > 0 && detailTool == "" {
		fmt.Fprintf(stderr, "utk: %s saved %d bytes (~%d tokens)\n",
			cmd, saved, filter.EstimateTokens(saved))
	}
	return code
}

// renderEnvelopeErrors reports a failed envelope's errors and returns the exit
// code. A failed compile lists warnings alongside the errors that stopped it,
// under the same heading and with the CS code stripped. Reported as one blob
// an obsolete-API note reads as a build breaker; filter.Error tells them
// apart, and a blob holding only warnings is not a failure at all.
func renderEnvelopeErrors(env *official.Envelope, code int, stderr io.Writer) int {
	fatal, blocked := false, false
	for _, e := range env.Errors {
		text, isFatal := filter.Error(e.Message)
		if isFatal {
			fatal = true
			blocked = blocked || mainThreadTimeout(e.Message)
			fmt.Fprintln(stderr, "utk:", e.Code+":", text)
			continue
		}
		fmt.Fprintln(stderr, "utk: warning:", text)
	}
	if blocked {
		answerModal(stderr)
	}
	if !fatal && len(env.Errors) > 0 {
		return 0
	}
	if code == 0 {
		code = 1
	}
	return code
}

// execTimeoutArgs routes one budget (ms) to both sides of the call: the CLI
// transport (seconds, before `--`, with 10s headroom so the tool's own
// timeout error wins the race) and the eval tool itself (ms, after `--`).
func execTimeoutArgs(ms int) []string {
	return []string{"--timeout", strconv.Itoa(ms/1000 + 10), "--", "--timeout", strconv.Itoa(ms)}
}

var timeoutCapRe = regexp.MustCompile(`Timeout must be between 1ms and (\d+)ms`)

// evalTimeoutCap extracts the pipeline's eval-timeout ceiling from a refusal,
// or 0 when the output is not that refusal.
func evalTimeoutCap(raw []byte) int {
	m := timeoutCapRe.FindSubmatch(raw)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0
	}
	return n
}
