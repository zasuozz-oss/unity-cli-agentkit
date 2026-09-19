package filter

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStatusCompact(t *testing.T) {
	raw, err := os.ReadFile("../official/testdata/pipeline_list.json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	json.Unmarshal(raw, &env)
	out := string(Status(env.Data))
	for _, want := range []string{"Unity-AI", "7800", "reachable", "0.4.0-exp.1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if len(out) > 400 {
		t.Fatalf("status too long: %d bytes", len(out))
	}
}

func TestStatusSafeMode(t *testing.T) {
	data := []byte(`{"instances":[{"projectName":"SDU","pid":1,"isRunning":true,
		"pipelineVersion":"0.5.0-exp.1",
		"pipelineServer":{"port":7800,"isReachable":false},
		"safeMode":{"detected":true}}]}`)
	out := string(Status(data))
	if !strings.Contains(out, "SAFE MODE") {
		t.Fatalf("safe mode not surfaced: %q", out)
	}
	if strings.Contains(out, "UNREACHABLE") {
		t.Fatalf("SAFE MODE must replace UNREACHABLE: %q", out)
	}
}

func TestStatusNoInstances(t *testing.T) {
	out := string(Status([]byte(`{"instances":[],"summary":{}}`)))
	if !strings.Contains(out, "no editor instances") {
		t.Fatalf("got %q", out)
	}
}

func TestStatusFallbackRaw(t *testing.T) {
	bad := []byte("nope")
	if !bytes.Equal(Status(bad), bad) {
		t.Fatal("bad input must pass through")
	}
}

func TestStatusOutdatedPipeline(t *testing.T) {
	data := []byte(`{"instances":[{"projectName":"SDU","pid":1,"isRunning":true,
		"pipelineVersion":"0.5.0-exp.1","updateAvailable":true,
		"pipelineServer":{"port":7800,"isReachable":true}}],
		"latestVersion":"0.6.0-exp.1"}`)
	out := string(Status(data))
	for _, want := range []string{"OUTDATED", "0.6.0-exp.1",
		"unity pipeline install --package-version 0.6.0-exp.1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	// An up-to-date instance must stay on the short line.
	ok := string(Status(bytes.Replace(data, []byte(`"updateAvailable":true`),
		[]byte(`"updateAvailable":false`), 1)))
	if strings.Contains(ok, "OUTDATED") {
		t.Fatalf("false positive: %q", ok)
	}
}
