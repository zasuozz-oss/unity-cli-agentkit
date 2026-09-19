package filter

import (
	"strings"
	"testing"
)

func TestExec_CompactsJSONWhitespace(t *testing.T) {
	in := []byte("{\n  \"a\": 1,\n  \"b\": [1, 2, 3]\n}")
	out := Exec(in, 0)
	want := `{"a":1,"b":[1,2,3]}`
	if string(out) != want {
		t.Fatalf("Exec compact = %q, want %q", out, want)
	}
}

func TestExec_PreservesLargeAndPreciseNumbers(t *testing.T) {
	// Large integers (instanceIDs/entity IDs) and high-precision decimals must
	// survive exact; a float64 decode would silently corrupt these. An array is
	// used so element order is preserved (object key order is not guaranteed).
	in := []byte(`{"nums":[12345678901234567890,9999999999999999,3.141592653589793238]}`)
	out := Exec(in, 0)
	if string(out) != string(in) {
		t.Fatalf("Exec must preserve number precision; got %q want %q", out, in)
	}
}

func TestExec_TrailingNonJSONPassesThroughRaw(t *testing.T) {
	// exec output like "{...}\n<log line>" is not pure JSON → keep it verbatim.
	in := []byte("{\"a\":1}\nlog: done\n")
	out := Exec(in, 0)
	if string(out) != string(in) {
		t.Fatalf("trailing non-JSON must pass through; got %q", out)
	}
}

func TestExec_NonJSONPassesThroughUnchanged(t *testing.T) {
	in := []byte("/Users/me/project/Assets") // exec can return a bare string
	out := Exec(in, 0)
	if string(out) != string(in) {
		t.Fatalf("non-JSON must pass through verbatim; got %q", out)
	}
}

func TestExec_TruncateArraysWhenOptIn(t *testing.T) {
	in := []byte(`{"items":[1,2,3,4,5]}`)
	out := Exec(in, 2)
	want := `{"items":[1,2,"…+3 more"]}`
	if string(out) != want {
		t.Fatalf("Exec truncate = %q, want %q", out, want)
	}
}

func TestExec_TruncateDisabledKeepsAllElements(t *testing.T) {
	in := []byte(`{"items":[1,2,3,4,5]}`)
	out := Exec(in, 0)
	want := `{"items":[1,2,3,4,5]}`
	if string(out) != want {
		t.Fatalf("Exec with Truncate=0 must keep all data; got %q", out)
	}
}

func TestExec_RunScriptSuccessUnwrapsToResult(t *testing.T) {
	// com.unity.pipeline 0.6.0's run_script wrapper: no executedAt (lean
	// responses drop null keys), so the strip must recognise it without one.
	in := []byte(`{"assemblyName":"PipelineRunScript__probe_ed7688ab","compileMs":976,` +
		`"diagnostics":[],"executeMs":4,"result":{"doubled":42},"success":true}`)
	out := Exec(in, 0)
	want := `{"doubled":42}`
	if string(out) != want {
		t.Fatalf("run_script strip = %q, want %q", out, want)
	}
}

func TestExec_RunScriptFailureKeepsDiagnostics(t *testing.T) {
	// A failed compile is the whole reason to call this: the diagnostics and
	// the false success must survive, only the timings and handle go.
	in := []byte(`{"assemblyName":"PipelineRunScript__broken_da6b39a3","compileMs":34,` +
		`"diagnostics":[{"column":20,"id":"CS0029","line":4,"message":"Cannot implicitly ` +
		`convert type 'string' to 'int'","severity":"error"}],"error":"Compilation Failed",` +
		`"executeMs":0,"success":false}`)
	out := string(Exec(in, 0))
	for _, want := range []string{"CS0029", `"line":4`, "Compilation Failed", `"success":false`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	for _, gone := range []string{"assemblyName", "compileMs", "executeMs"} {
		if strings.Contains(out, gone) {
			t.Fatalf("%q should have been stripped: %q", gone, out)
		}
	}
}

func TestExec_NonWrapperWithSuccessIsUntouched(t *testing.T) {
	// A tool payload that merely has a success field is not the wrapper; the
	// strip must not eat its fields. (Compare fields, not bytes: re-encoding
	// sorts object keys, as TestExec_PreservesLargeAndPreciseNumbers notes.)
	in := []byte(`{"success":true,"myOwnField":1}`)
	out := string(Exec(in, 0))
	for _, want := range []string{`"myOwnField":1`, `"success":true`} {
		if !strings.Contains(out, want) {
			t.Fatalf("non-wrapper lost %q: %q", want, out)
		}
	}
}
