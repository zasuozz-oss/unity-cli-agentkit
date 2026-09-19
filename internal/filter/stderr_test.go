package filter

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoiseFilter(t *testing.T) {
	const noise = `[Experiment] Fetch failed:  [
  Error: connect ECONNREFUSED 127.0.0.1:443
      at _afterConnectImpl (ext:deno_node/net.ts:1:7280) {
    errno: -61,
    code: "ECONNREFUSED",
  }
]
`
	in := "real warning before\n" + noise + "real error after\n"

	// One Write, then split across chunk boundaries: the block must vanish
	// either way, since a pipe splits wherever it likes.
	for _, chunk := range []int{len(in), 7, 1} {
		var out bytes.Buffer
		f := &NoiseFilter{W: &out}
		for i := 0; i < len(in); i += chunk {
			end := i + chunk
			if end > len(in) {
				end = len(in)
			}
			f.Write([]byte(in[i:end]))
		}
		f.Close()
		got := out.String()
		if strings.Contains(got, "Experiment") || strings.Contains(got, "ECONNREFUSED") {
			t.Fatalf("chunk=%d: noise survived: %q", chunk, got)
		}
		if got != "real warning before\nreal error after\n" {
			t.Fatalf("chunk=%d: real diagnostics must survive verbatim: %q", chunk, got)
		}
	}
}

// A trailing line with no newline must still be flushed, not dropped.
func TestNoiseFilter_PartialLine(t *testing.T) {
	var out bytes.Buffer
	f := &NoiseFilter{W: &out}
	f.Write([]byte("no trailing newline"))
	f.Close()
	if out.String() != "no trailing newline" {
		t.Fatalf("got %q", out.String())
	}
}
