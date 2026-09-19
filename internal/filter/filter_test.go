package filter

import (
	"bytes"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(400); got != 100 { // ~4 chars/token
		t.Fatalf("EstimateTokens(400) = %d, want 100", got)
	}
	if got := EstimateTokens(0); got != 0 {
		t.Fatalf("EstimateTokens(0) = %d, want 0", got)
	}
}

func TestSplitUtkFlags(t *testing.T) {
	rest, opts := SplitUtkFlags([]string{"--type", "error", "--raw", "--truncate", "5"})
	if !opts.Raw {
		t.Fatal("expected Raw=true")
	}
	if opts.Truncate != 5 {
		t.Fatalf("Truncate = %d, want 5", opts.Truncate)
	}
	want := []string{"--type", "error"}
	if len(rest) != 2 || rest[0] != want[0] || rest[1] != want[1] {
		t.Fatalf("rest = %v, want %v (utk-only flags must be stripped)", rest, want)
	}
}

func TestApply_DispatchesConsole(t *testing.T) {
	raw := []byte(`{"returned":3,"dropped":false,"entries":[{"logType":"Log","level":"log","message":"a"},{"logType":"Log","level":"log","message":"a"},{"logType":"Log","level":"log","message":"b"}]}`)
	out := Apply("console", raw, Options{})
	if !bytes.Contains(out, []byte("×2")) || !bytes.Contains(out, []byte("] b")) {
		t.Fatalf("console dispatch wrong: %q", out)
	}
}

func TestApplyKinds(t *testing.T) {
	if got := Apply("exec", []byte(`{ "a" : 1 }`), Options{}); string(got) != `{"a":1}` {
		t.Fatalf("exec must compact JSON, got %q", got)
	}
	raw := []byte("anything")
	if got := Apply("unknown-kind", raw, Options{}); !bytes.Equal(got, raw) {
		t.Fatal("unknown kind must be identity")
	}
}
