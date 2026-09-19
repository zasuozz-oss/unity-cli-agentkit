package filter

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func consolePayload(t *testing.T, fixture string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../official/testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data struct {
			Result json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data.Result
}

func TestConsoleDedupsAndTrims(t *testing.T) {
	out := string(Console(consolePayload(t, "console_v2.json"), ""))
	if !strings.HasPrefix(out, "console: 27 returned\n") {
		t.Fatalf("missing header, got %q", out[:60])
	}
	if !strings.Contains(out, "×") {
		t.Fatal("expected at least one ×N dedup marker (fixture has duplicate errors)")
	}
	if !strings.Contains(out, "frames omitted") {
		t.Fatal("expected stack trimming marker")
	}
	if strings.Contains(out, "timestampUtc") {
		t.Fatal("timestamps must be dropped")
	}
}

func TestConsoleSavesEnoughBytes(t *testing.T) {
	p := consolePayload(t, "console_v2.json")
	out := Console(p, "")
	// Spec §6: 57.6KB official JSON → ≤ 6000B filtered.
	if len(out) > 6000 {
		t.Fatalf("filtered = %d bytes, want ≤ 6000", len(out))
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
}

func TestConsoleKeepsEveryUniqueMessage(t *testing.T) {
	p := consolePayload(t, "console_v2.json")
	var r struct {
		Entries []struct{ Message string } `json:"entries"`
	}
	json.Unmarshal(p, &r)
	// Guard against the assertion below going vacuous if the schema moves
	// again: an empty decode would make this test pass without checking a
	// single message.
	if len(r.Entries) == 0 {
		t.Fatal("fixture decoded to zero entries — the schema moved")
	}
	out := Console(p, "")
	for _, l := range r.Entries {
		firstLine, _, _ := strings.Cut(l.Message, "\n")
		if !bytes.Contains(out, []byte(firstLine)) {
			t.Fatalf("lost message %q", firstLine)
		}
	}
}

func TestConsoleFallbackRawOnBadSchema(t *testing.T) {
	bad := []byte("plain text, not the expected JSON")
	if got := Console(bad, ""); !bytes.Equal(got, bad) {
		t.Fatal("non-JSON input must pass through unchanged")
	}
}

func TestConsoleEmptyLogs(t *testing.T) {
	out := string(Console([]byte(`{"returned":0,"dropped":false,"entries":[]}`), ""))
	if !strings.Contains(out, "0 returned") {
		t.Fatalf("got %q", out)
	}
}

// A single bad type name reports at every site that used it. Ungrouped that is
// dozens of near-identical lines, and `get_console_logs` hands them back
// newest-first, so truncating from the front drops the error that caused them.
func TestConsoleGroupsCompileErrorsByRootCause(t *testing.T) {
	logs := []string{
		`{"logType":"Error","level":"error","message":"Assets/Late.cs(9,1): error CS0246: The type or namespace name 'Missing' could not be found","stackTrace":""}`,
		`{"logType":"Error","level":"error","message":"Assets/Mid.cs(5,3): error CS0246: The type or namespace name 'Missing' could not be found","stackTrace":""}`,
		`{"logType":"Warning","level":"warn","message":"Assets/Ui.cs(3,9): warning CS0618: 'TMP_Text.enableWordWrapping' is obsolete","stackTrace":""}`,
		`{"logType":"Error","level":"error","message":"Assets/First.cs(1,1): error CS0103: The name 'Foo' does not exist in the current context","stackTrace":""}`,
	}
	in := []byte(`{"returned":4,"dropped":false,"entries":[` + strings.Join(logs, ",") + `]}`)
	out := string(Console(in, ""))

	if !strings.Contains(out, "compile: 3 errors, 1 warning") {
		t.Errorf("want a severity split in the header:\n%s", out)
	}
	if !strings.Contains(out, "[error CS0246 ×2]") {
		t.Errorf("the two sites of one cause must collapse:\n%s", out)
	}
	if !strings.Contains(out, "+1 more sites") {
		t.Errorf("the rolled-up site count must survive:\n%s", out)
	}
	// The compiler emitted CS0103 first; the tool returns it last.
	if strings.Index(out, "CS0103") > strings.Index(out, "CS0246") {
		t.Errorf("the first error must be reported first:\n%s", out)
	}
	if strings.Count(out, "\n[") > 3 {
		t.Errorf("4 lines from 3 causes must render as 3 groups:\n%s", out)
	}
}

// Grouping must not reach runtime logs, whose message is the whole signal.
func TestConsoleLeavesRuntimeLogsAlone(t *testing.T) {
	in := []byte(`{"returned":1,"dropped":false,"entries":[{"logType":"Error","level":"error","message":"NullReferenceException: Object reference not set","stackTrace":""}]}`)
	out := string(Console(in, ""))
	if !strings.Contains(out, "[Error] NullReferenceException") {
		t.Fatalf("runtime log reshaped: %q", out)
	}
	if strings.Contains(out, "compile:") {
		t.Fatalf("no compile summary belongs here: %q", out)
	}
}

func TestConsoleNarrowsToExactSeverity(t *testing.T) {
	// The tool's --level is a minimum, so asking it for warnings also hands
	// back every error. utk's --type warning means warnings alone. The fixture
	// carries only `level` (no 0.7.0 logType), so this covers that path too.
	p := consolePayload(t, "console_v2.json")
	all := string(Console(p, ""))
	for _, want := range []string{"[error]", "[warn", "[log"} {
		if !strings.Contains(all, want) {
			t.Fatalf("fixture must carry %s for this to mean anything:\n%s", want, all)
		}
	}
	warn := string(Console(p, "warning"))
	for _, gone := range []string{"[error]", "[log"} {
		if strings.Contains(warn, gone) {
			t.Fatalf("%s leaked into --type warning:\n%s", gone, warn)
		}
	}
	if !strings.Contains(warn, "[warn") {
		t.Fatalf("warnings must survive:\n%s", warn)
	}
	// The header counts what was kept, not what the tool sent.
	if strings.HasPrefix(warn, "console: 27 returned") {
		t.Fatalf("header must report the narrowed count:\n%s", warn)
	}
}
