package filter

import (
	"encoding/json"
	"strings"
	"testing"
)

// Shape copied from a real `unity command run_tests` payload (beta.3): the tool
// merges its own fields into the result envelope, and every passing entry
// carries null Message/StackTrace.
const runTestsPayload = `{
  "Summary": {"Total": 4, "Passed": 2, "Failed": 1, "Skipped": 1, "Inconclusive": 0},
  "Results": [
    {"FullName": "Ns.A.passes_one", "Status": "Passed", "Duration": 0.0001, "Message": null, "StackTrace": null},
    {"FullName": "Ns.A.passes_two", "Status": "Passed", "Duration": 0.0002, "Message": null, "StackTrace": null},
    {"FullName": "Ns.A.skipped_one", "Status": "Skipped", "Duration": 0.0, "Message": null, "StackTrace": null},
    {"FullName": "Ns.B.breaks", "Status": "Failed", "Duration": 0.002,
     "Message": "  Expected: True\r\n  But was:  False\r\n",
     "StackTrace": "at Ns.B.breaks () [0x0000a] in C:\\Users\\me\\Assets\\Tests\\BTests.cs:79\r\n"}
  ],
  "Duration": 4.24,
  "Mode": "All",
  "success": true,
  "command": "run_tests",
  "result": "playmode_running",
  "executionTimeMs": 4237,
  "message": "EditMode complete: 3/4 passed. PlayMode tests started asynchronously — poll the test_status command for their results.",
  "error": null,
  "errorDetails": null,
  "executedAt": "0001-01-01T00:00:00"
}`

func TestTests(t *testing.T) {
	out := string(Tests([]byte(runTestsPayload)))

	for _, want := range []string{
		"4 tests: 2 passed, 1 failed, 1 skipped",
		"4.2s",
		"mode=All",
		"FAILED Ns.B.breaks",
		"Expected: True",
		"at BTests.cs:79",
		// The async hand-off must survive: without it an agent waits forever
		// for PlayMode results that are never printed.
		"poll the test_status command",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Passing names are the payload's bulk and its only deliberate loss.
	if strings.Contains(out, "passes_one") {
		t.Errorf("passing test names must be dropped:\n%s", out)
	}
	if !strings.Contains(out, "2 passing test names omitted") {
		t.Errorf("the omission must be announced:\n%s", out)
	}
	// The point of the filter.
	if len(out) > len(runTestsPayload)/3 {
		t.Errorf("compression too weak: %d -> %d bytes", len(runTestsPayload), len(out))
	}
}

// test_status wraps the same report in a JSON string and lowercases every key.
// Without the unwrap it printed as one escaped \r\n-riddled blob.
func TestTests_UnwrapsTestStatusStringPayload(t *testing.T) {
	inner := `{"status":"completed","duration":1.5,"summary":{"total":2,"passed":1,"failed":1},` +
		`"results":[{"fullName":"Ns.B.breaks","status":"Failed","message":"boom","stackTrace":""}]}`
	quoted, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	out := string(Tests(quoted))
	for _, want := range []string{"completed: 2 tests: 1 passed, 1 failed", "FAILED Ns.B.breaks", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `\"`) {
		t.Errorf("the escaped JSON string must not survive:\n%s", out)
	}
}

func TestTests_PassesThroughUnexpectedShape(t *testing.T) {
	raw := []byte(`{"something":"else"}`)
	if got := Tests(raw); string(got) != string(raw) {
		t.Fatalf("missing Summary must pass through verbatim, got %q", got)
	}
}

func TestExec_StripsToolEnvelope(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"a bare string result loses the wrapper and the quotes",
			`{"command":null,"diagnostics":[],"error":null,"errorDetails":null,
			  "executedAt":"0001-01-01T00:00:00","executionTimeMs":483,"message":null,
			  "output":null,"result":"reserialized","success":true}`,
			"reserialized",
		},
		{
			"a failure keeps everything that says why",
			`{"command":null,"diagnostics":[],"error":"CS0103","errorDetails":null,
			  "executedAt":"0001-01-01T00:00:00","executionTimeMs":12,"message":null,
			  "output":null,"result":null,"success":false}`,
			`{"error":"CS0103","success":false}`,
		},
		{
			"a payload that is not the envelope is left alone",
			`{"result":"x","success":true}`,
			`{"result":"x","success":true}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(Exec([]byte(c.in), 0)); got != c.want {
				t.Fatalf("Exec = %q, want %q", got, c.want)
			}
		})
	}
}

// test_status with no run in progress: no Summary, and the report arrives as a
// JSON *string*, so passing it through printed the escaped encoding verbatim.
func TestTests_StatusOnlyPayload(t *testing.T) {
	in := []byte(`"{\"status\":\"no_tests\",\"message\":\"No test run in progress\"}"`)
	if got := string(Tests(in)); got != "no_tests: No test run in progress\n" {
		t.Fatalf("Tests = %q", got)
	}
	if got := string(Tests([]byte(`{"status":"running"}`))); got != "running\n" {
		t.Fatalf("status alone = %q", got)
	}
	// Still loss-safe for anything that is neither.
	if got := string(Tests([]byte(`{"foo":1}`))); got != `{"foo":1}` {
		t.Fatalf("unknown payload must pass through, got %q", got)
	}
}
