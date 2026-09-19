package filter

import (
	"strings"
	"testing"
)

// Shape copied from a real `unity test` report (NUnit3): cases sit several
// <test-suite> levels down, ignored ones carry <reason>, failures <failure>.
const nunitReport = `<?xml version="1.0" encoding="utf-8"?>
<test-run total="3" passed="1" failed="1" skipped="1" inconclusive="0" duration="0.134">
  <test-suite type="TestSuite" name="root">
    <test-suite type="Assembly" name="A.dll">
      <test-suite type="TestSuite" name="Ns">
        <test-suite type="TestFixture" name="A">
          <test-case fullname="Ns.A.ok" result="Passed" duration="0.007" />
          <test-case fullname="Ns.A.ignored" result="Skipped" label="Ignored">
            <reason><message>needs a saved scene</message></reason>
          </test-case>
          <test-case fullname="Ns.A.breaks" result="Failed">
            <failure>
              <message>  Expected: True&#13;&#10;  But was:  False</message>
              <stack-trace>at Ns.A.breaks () [0x0000a] in C:\Assets\ATests.cs:79</stack-trace>
            </failure>
          </test-case>
        </test-suite>
      </test-suite>
    </test-suite>
  </test-suite>
</test-run>`

func TestNUnit(t *testing.T) {
	out, failed, ok := NUnit([]byte(nunitReport), "EditMode")
	if !ok {
		t.Fatal("a real report must parse")
	}
	if !failed {
		t.Fatal("failed=1 must reach the exit code")
	}
	got := string(out)
	for _, want := range []string{
		"3 tests: 1 passed, 1 failed, 1 skipped",
		"0.1s", "mode=EditMode",
		"FAILED Ns.A.breaks", "Expected: True", "But was:  False", "at ATests.cs:79",
		"SKIPPED Ns.A.ignored", "needs a saved scene",
		"(1 passing test names omitted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("NUnit output missing %q:\n%s", want, got)
		}
	}
	// The whole point: passing test names never reach the reader.
	if strings.Contains(got, "Ns.A.ok") {
		t.Fatalf("passing names must be dropped:\n%s", got)
	}
	if len(out) >= len(nunitReport) {
		t.Fatalf("report should shrink: %d >= %d", len(out), len(nunitReport))
	}
}

func TestNUnit_NotAReport(t *testing.T) {
	for _, in := range []string{"", "not xml", "<test-run total=\"0\"/>"} {
		if _, _, ok := NUnit([]byte(in), ""); ok {
			t.Fatalf("%q must not parse as a report", in)
		}
	}
}
