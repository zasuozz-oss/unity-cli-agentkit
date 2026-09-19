package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// consoleBody builds the `console` envelope utk's follow-up call sees.
func consoleBody(entries ...string) string {
	return `{"success":true,"command":"command console","data":{"command":"console",` +
		`"result":{"returned":` + itoa(len(entries)) + `,"dropped":false` +
		`,"entries":[` + strings.Join(entries, ",") + `]}},"errors":[],"warnings":[]}`
}

// logEntry writes an entry the way com.unity.pipeline 0.7.0 does: the exact
// logType beside the level it collapses to.
func logEntry(kind, msg string, at time.Time) string {
	return `{"logType":"` + kind + `","level":"` + level(kind) + `","message":"` + msg +
		`","stackTrace":"","timestampUtc":"` + at.UTC().Format(time.RFC3339Nano) + `"}`
}

func level(kind string) string {
	switch kind {
	case "Warning":
		return "warn"
	case "Log":
		return "log"
	default:
		return "error"
	}
}

func itoa(n int) string { return string(rune('0' + n)) }

// `eval` drops Debug.Log — its `output` field is null on every call — so a
// snippet whose answer is what it logged comes back empty and costs a second
// `utk console` to read at all.
func TestRun_ExecMergesItsOwnLogs(t *testing.T) {
	now := time.Now()
	bin, _ := fakeUnity(t, readFixture(t, "eval_int.json"), 0)
	fakeUnityNext(t, consoleBody(
		logEntry("Log", "stale entry from an earlier run", now.Add(-time.Hour)),
		logEntry("Log", "hello from the snippet", now.Add(time.Minute)),
	))
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"exec", "return 42;"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "42") {
		t.Errorf("the result itself must survive: %q", out)
	}
	if !strings.Contains(out, "[Log] hello from the snippet") {
		t.Errorf("the snippet's own log must be folded in: %q", out)
	}
	// Anything logged before the call belongs to some earlier command.
	if strings.Contains(out, "stale entry") {
		t.Errorf("pre-existing console entries must not be attributed here: %q", out)
	}
}

func TestRun_ExecLogsCanBeDisabled(t *testing.T) {
	bin, argvFile := fakeUnity(t, readFixture(t, "eval_int.json"), 0)
	fakeUnityNext(t, consoleBody(logEntry("Log", "hello", time.Now().Add(time.Minute))))
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_NO_EXEC_LOGS", "1")

	var stdout, stderr bytes.Buffer
	run([]string{"exec", "return 42;"}, &stdout, &stderr)
	if strings.Contains(stdout.String(), "hello") {
		t.Errorf("UTK_NO_EXEC_LOGS must skip the fetch: %q", stdout.String())
	}
	if got := readArgv(t, argvFile); !strings.Contains(got, "eval") {
		t.Errorf("only the eval call should have been made, got %q", got)
	}
}

// A file long enough to be worth `--file` is long enough that "$(cat f.cs)"
// mangles it; the CLI has a tool that reads the file itself.
func TestRun_ExecFileUsesEvalFile(t *testing.T) {
	bin, argvFile := fakeUnity(t, readFixture(t, "eval_int.json"), 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_NO_EXEC_LOGS", "1")

	var stdout, stderr bytes.Buffer
	run([]string{"exec", "--file", "snippet.cs"}, &stdout, &stderr)
	if got := readArgv(t, argvFile); !strings.HasPrefix(got, "command eval_file --file snippet.cs") {
		t.Fatalf("argv = %q, want the eval_file tool", got)
	}
}
