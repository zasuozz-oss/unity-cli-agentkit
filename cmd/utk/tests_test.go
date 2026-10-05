package main

import (
	"bytes"
	"strings"
	"testing"
)

// test_status hands its report back as a JSON string, not an object; reading it
// as one makes every poll look unfinished and the run never returns.
func TestTestRunFinished(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{"running", `"{\"status\":\"running\"}"`, false},
		{"completed", `"{\"status\":\"completed\",\"summary\":{}}"`, true},
		{"no run in progress", `"{\"status\":\"no_tests\"}"`, true},
		{"unwrapped object", `{"status":"running"}`, false},
		{"garbage keeps polling", `"nope"`, false},
	}
	for _, c := range cases {
		if got := testRunFinished([]byte(c.payload)); got != c.want {
			t.Errorf("%s: testRunFinished = %v, want %v", c.name, got, c.want)
		}
	}
}

// The framework's own crash never reaches test_status, which then says
// "running" forever; the console entry is the only sign the run is gone.
func TestTestCrashed(t *testing.T) {
	logs := []consoleEntry{
		{Message: "[TestResultCollector] Run started: 3 test(s)"},
		{Message: "An unexpected error happened while running tests.\nstack…"},
	}
	if got := testCrashed(logs); got == "" {
		t.Fatal("crash entry not detected")
	}
	if got := testCrashed(logs[:1]); got != "" {
		t.Fatalf("healthy run flagged as crashed: %q", got)
	}
}

// The report test_status hands back is whichever run finished last on the
// Editor; with agents sharing one, a one-test filter came back with 1041.
func TestForeignTest(t *testing.T) {
	report := `"{\"status\":\"completed\",\"results\":[{\"FullName\":\"G.MineTests.One\"},{\"FullName\":\"G.OtherTests.Two\"}]}"`
	cases := []struct {
		name, filter, filterType, want string
	}{
		{"other run's test", "MineTests", "", "G.OtherTests.Two"},
		{"case-insensitive, all match", "g.", "testName", ""},
		{"no filter: anything is ours", "", "", ""},
		{"assembly: not in the report", "Mine", "assembly", ""},
		{"category: not in the report", "Mine", "category", ""},
		{"filter of only separators: anything is ours", " ; ", "", ""},
		{"merged filter, both ours", "MineTests;OtherTests", "", ""},
		{"merged filter, one foreign", "MineTests;Nope", "", "G.OtherTests.Two"},
	}
	for _, c := range cases {
		if got := foreignTest([]byte(report), c.filter, c.filterType); got != c.want {
			t.Errorf("%s: foreignTest = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSplitTestFilter(t *testing.T) {
	got := splitTestFilter(" A ;b;;C")
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("splitTestFilter = %v, want %v", got, want)
	}
}

// End to end: an idle Editor lets the run start, and a report holding tests
// the filter cannot match fails instead of passing as the caller's own.
func TestRun_RunTestsRejectsAnotherRunsReport(t *testing.T) {
	idle := `{"success":true,"data":{"result":"{\"status\":\"no_tests\"}"}}`
	other := `{"success":true,"data":{"result":"{\"status\":\"completed\",\"summary\":{\"total\":2,\"passed\":2},` +
		`\"results\":[{\"FullName\":\"G.MineTests.One\",\"Status\":\"Passed\"},{\"FullName\":\"G.OtherTests.Two\",\"Status\":\"Passed\"}]}"}}`
	bin, _ := fakeUnity(t, idle, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_NO_FLAG_CHECK", "1")
	fakeUnityNext(t, other)

	var stdout, stderr bytes.Buffer
	code := run([]string{"run_tests", "--mode", "editor", "--filter", "MineTests"}, &stdout, &stderr)
	out := stdout.String() + stderr.String()
	if code != 1 || !strings.Contains(out, "TEST_RESULT_FOREIGN") {
		t.Errorf("exit = %d, want 1 with TEST_RESULT_FOREIGN; output: %s", code, out)
	}
}

// A run that covers the whole suite holds a shared Editor for minutes, so it is
// refused before anything reaches Unity unless UTK_FULL_SUITE=1.
func TestWholeSuite(t *testing.T) {
	cases := []struct {
		args  []string
		whole bool
	}{
		{nil, true},
		{[]string{"--mode", "editor"}, true},
		{[]string{"--filter", "EchoPals.Tests"}, true},
		{[]string{"--filter=EchoPals.Tests.EditMode"}, true},
		{[]string{"--filter_type", "Assembly", "--filter", "EchoPals.Tests.EditMode"}, true},
		{[]string{"--filter", "Game.Tests;Foo"}, true}, // one namespace part drags the suite into a merged run
		{[]string{"--mode", "editor", "--filter", "CatchGameTests"}, false},
		{[]string{"--filter", "EchoPals.Tests.CatchGameTests"}, false},
		{[]string{"--filter_type", "category", "--filter", "Fast"}, false},
	}
	for _, c := range cases {
		if got := wholeSuite(c.args) != ""; got != c.whole {
			t.Errorf("wholeSuite(%q) whole = %v, want %v", c.args, got, c.whole)
		}
	}
	t.Setenv("UTK_FULL_SUITE", "1")
	if why := wholeSuite(nil); why != "" {
		t.Errorf("UTK_FULL_SUITE=1 still refused: %s", why)
	}
}

func TestRun_RunTestsRefusesWholeSuite(t *testing.T) {
	t.Setenv("UTK_UNITY_BIN", "/nonexistent/unity") // must not be reached
	var stdout, stderr bytes.Buffer
	if code := run([]string{"run_tests", "--mode", "editor"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit = %d, want 2; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "UTK_FULL_SUITE=1") {
		t.Errorf("stderr does not name the escape hatch: %s", stderr.String())
	}
}
