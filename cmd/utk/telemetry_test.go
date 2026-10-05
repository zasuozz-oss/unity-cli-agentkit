package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestTelemetryStats(t *testing.T) {
	lines := `{"ts":"2026-10-05T01:00:00Z","owner":"a","task":"","kind":"test","wait_s":10,"hold_s":100,"result":"PASS","via":"job"}
{"ts":"2026-10-05T01:05:00Z","owner":"b","task":"","kind":"test","wait_s":30,"hold_s":120,"result":"FAIL","via":"queue"}
{"ts":"2026-10-05T01:06:00Z","owner":"c","task":"","kind":"shot","wait_s":600,"hold_s":20,"result":"PASS","via":"job"}
{"ts":"2026-10-05T01:07:00Z","owner":"d","task":"","kind":"compile","wait_s":0,"hold_s":0,"result":"GATE_FAILED","via":"queue"}
{"ts":"2026-09-01T00:00:00Z","owner":"old","task":"","kind":"test","wait_s":9999,"hold_s":9999,"result":"PASS","via":"job"}
not json`
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out := telemetryStats(strings.NewReader(lines), since)
	for _, want := range []string{"test", "n=2", "shot", "wait p50=600", "gate blocked: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "wait p50=10s p95=30s") {
		t.Errorf("test-kind percentiles wrong:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "compile") {
			t.Errorf("GATE_FAILED entry leaked into per-kind stats:\n%s", out)
		}
	}
	if strings.Contains(out, "9999") {
		t.Errorf("entry before --since counted:\n%s", out)
	}
}

func TestAppendTelemetryRoundTrip(t *testing.T) {
	t.Setenv("UTK_TELEMETRY", t.TempDir()+"/sub/t.jsonl")
	if err := appendTelemetry(telemetryEntry{TS: "2026-10-05T01:00:00Z", Kind: "scene", HoldS: 5, Result: "PASS", Via: "queue"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(telemetryPath())
	if !strings.Contains(string(b), `"kind":"scene"`) || !strings.HasSuffix(string(b), "\n") {
		t.Fatalf("bad line: %q", b)
	}
}
