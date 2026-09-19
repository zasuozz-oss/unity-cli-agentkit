package filter

import (
	"bytes"
	"testing"
)

// Apply on a pass-through (unknown) kind must be byte-identical to its input
// (spec §11 "byte-identical"). Guards against accidental mutation.
func TestApply_PassthroughIsByteIdentical(t *testing.T) {
	for _, kind := range []string{"editor", "menu", "screenshot", "profiler", "test"} {
		raw := []byte("arbitrary\x00binary\xff payload\n\n")
		out := Apply(kind, raw, Options{})
		if !bytes.Equal(out, raw) {
			t.Fatalf("Apply(%q) mutated pass-through output", kind)
		}
	}
}
