package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const failingReport = `<?xml version="1.0" encoding="utf-8"?>
<test-run total="2" passed="1" failed="1" skipped="0" inconclusive="0" duration="1.5">
  <test-suite type="Assembly" name="A.dll"><test-suite type="TestFixture" name="A">
    <test-case fullname="Ns.A.ok" result="Passed" />
    <test-case fullname="Ns.A.breaks" result="Failed">
      <failure><message>boom</message>
        <stack-trace>at Ns.A.breaks () [0x1] in /x/ATests.cs:12</stack-trace></failure>
    </test-case>
  </test-suite></test-suite>
</test-run>`

// `unity test` exits 0 on a red suite and its JSON names only the report file,
// so without this every `utk test && ship` ships a broken build.
func TestRunBatchTest_RedSuiteExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "test-results.xml")
	if err := os.WriteFile(report, []byte(failingReport), 0o644); err != nil {
		t.Fatal(err)
	}
	env, _ := json.Marshal(map[string]any{
		"success": true, "command": "test",
		"data":     map[string]string{"projectPath": dir, "output": "test-results.xml"},
		"errors":   []any{},
		"warnings": []any{},
	})
	bin, argvFile := fakeUnity(t, string(env), 0)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	code := run([]string{"test", "--mode", "EditMode"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for a failing suite (stderr: %s)", code, stderr.String())
	}
	if argv := readArgv(t, argvFile); !strings.HasPrefix(argv, "test --mode EditMode") {
		t.Fatalf("argv = %q", argv)
	}
	got := stdout.String()
	for _, want := range []string{"2 tests: 1 passed, 1 failed", "mode=EditMode",
		"FAILED Ns.A.breaks", "boom", "at ATests.cs:12"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Ns.A.ok") {
		t.Fatalf("passing names must be dropped:\n%s", got)
	}
}

// --raw must still hand over the runner's own stream untouched.
func TestRunBatchTest_RawStreamsThrough(t *testing.T) {
	bin, argvFile := fakeUnity(t, "batchmode chatter\n", 0)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"test", "--raw"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if argv := readArgv(t, argvFile); argv != "test" {
		t.Fatalf("argv = %q, want %q (no --json)", argv, "test")
	}
	if stdout.String() != "batchmode chatter\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

// beta.6 reports a red suite as exit 8 + a TESTS_FAILED envelope with null
// data, so the report path vanishes from the envelope while the report itself
// is still written. Bailing to the error renderer here loses every failing
// test name — exactly the case an agent needs.
func TestRunBatchTest_Beta6TestsFailedStillRendersReport(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "test-results.xml")
	if err := os.WriteFile(report, []byte(failingReport), 0o644); err != nil {
		t.Fatal(err)
	}
	env, _ := json.Marshal(map[string]any{
		"success": false, "command": "test", "data": nil,
		"errors": []map[string]string{{
			"code": "TESTS_FAILED", "message": "Tests failed: Unity process exited with code 2."}},
		"warnings": []any{},
	})
	bin, _ := fakeUnity(t, string(env), 8)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	code := run([]string{"test", "--mode", "EditMode", "--output", report}, &stdout, &stderr)
	if code != 8 {
		t.Fatalf("exit = %d, want 8 passed through (stderr: %s)", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"2 tests: 1 passed, 1 failed", "FAILED Ns.A.breaks", "boom"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
}

// A run that never reached a verdict has no report to render, so it must keep
// going to the error renderer — that is the whole point of beta.6's 6-vs-8 split.
func TestRunBatchTest_RunErrorStillRendersEnvelope(t *testing.T) {
	env, _ := json.Marshal(map[string]any{
		"success": false, "command": "test", "data": nil,
		"errors": []map[string]string{{
			"code": "TEST_RUN_ERROR", "message": "Unity process stopped with signal SIGABRT."}},
		"warnings": []any{},
	})
	bin, _ := fakeUnity(t, string(env), 6)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"test", "--mode", "EditMode"}, &stdout, &stderr); code != 6 {
		t.Fatalf("exit = %d, want 6", code)
	}
	if !strings.Contains(stderr.String(), "SIGABRT") {
		t.Fatalf("stderr missing the run error:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "tests:") {
		t.Fatalf("must not render a report it never got:\n%s", stdout.String())
	}
}
