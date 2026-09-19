package filter

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func listPayload(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../official/testdata/list_full.json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func TestListOneLinePerTool(t *testing.T) {
	out := string(List(listPayload(t)))
	if !strings.HasPrefix(out, "140 tools.") {
		t.Fatalf("missing count header: %q", out[:40])
	}
	for _, name := range []string{"eval", "get_console_logs", "get_scene_hierarchy", "screenshot"} {
		if !strings.Contains(out, "\n"+name+" — ") {
			t.Fatalf("tool %q missing from listing", name)
		}
	}
}

func TestListSizeBudget(t *testing.T) {
	out := List(listPayload(t))
	// Spec §6: 115KB official JSON → ≤ 8000B. Adjust descCut down if this fails.
	if len(out) > 8000 {
		t.Fatalf("filtered = %d bytes, want ≤ 8000", len(out))
	}
}

func TestListDetailShowsParameters(t *testing.T) {
	out := string(ListDetail(listPayload(t), "get_console_logs"))
	for _, want := range []string{"get_console_logs", "severity", "limit", "all | log | warning | error"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail missing %q in %q", want, out)
		}
	}
}

func TestListDetailUnknownTool(t *testing.T) {
	out := string(ListDetail(listPayload(t), "console_logs"))
	if !strings.Contains(out, "get_console_logs") { // suggestion by substring
		t.Fatalf("want suggestions containing get_console_logs, got %q", out)
	}
}

func TestListFallbackRaw(t *testing.T) {
	bad := []byte("not json")
	if !bytes.Equal(List(bad), bad) || !bytes.Equal(ListDetail(bad, "x"), bad) {
		t.Fatal("bad input must pass through unchanged")
	}
}

// The listing is ~7.5KB; finding one tool by piping it through grep pays for
// every byte first. --grep never emits them.
func TestListGrep(t *testing.T) {
	full := List(listPayload(t))
	out := string(Grep(full, "scene"))
	if !strings.Contains(out, "get_scene_hierarchy") {
		t.Fatalf("want the matching tools, got %q", out)
	}
	if strings.Contains(out, "\neval — ") {
		t.Fatalf("non-matching tools must be dropped: %q", out)
	}
	if len(out) >= len(full)/2 {
		t.Fatalf("no saving: %d of %d bytes", len(out), len(full))
	}
	if !strings.HasPrefix(out, "1") && !strings.Contains(strings.SplitN(out, "\n", 2)[0], "tools match") {
		t.Fatalf("want a match count header, got %q", strings.SplitN(out, "\n", 2)[0])
	}
}

func TestListGrepNoMatch(t *testing.T) {
	out := string(Grep(List(listPayload(t)), "definitely_not_a_tool"))
	if !strings.Contains(out, "no tool matches") {
		t.Fatalf("want an explicit miss, got %q", out)
	}
}

// A stray bracket is a search, not a crash.
func TestListGrepInvalidRegexp(t *testing.T) {
	out := string(Grep(List(listPayload(t)), "scene["))
	if !strings.Contains(out, "no tool matches") {
		t.Fatalf("want a literal search, got %.80q", out)
	}
}
