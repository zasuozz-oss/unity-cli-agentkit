package official

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseEvalInt(t *testing.T) {
	env, err := Parse(load(t, "eval_int.json"))
	if err != nil || !env.Success {
		t.Fatalf("Parse: %v success=%v", err, env.Success)
	}
	p := env.Payload(true) // data.result
	var r struct {
		Result json.Number `json:"result"`
	}
	if err := json.Unmarshal(p, &r); err != nil || r.Result != "42" {
		t.Fatalf("payload result = %s (err %v), want 42", r.Result, err)
	}
}

func TestParseListPayloadNotNested(t *testing.T) {
	env, _ := Parse(load(t, "list_full.json"))
	var d struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(env.Payload(false), &d); err != nil || d.Count != 140 {
		t.Fatalf("count = %d (err %v), want 140", d.Count, err)
	}
}

func TestParseFailureEnvelope(t *testing.T) {
	env, err := Parse(load(t, "eval_compile_error.json"))
	if err != nil {
		t.Fatal(err)
	}
	if env.Success || len(env.Errors) == 0 || env.Errors[0].Code != "COMMAND_FAILED" {
		t.Fatalf("want failure envelope with COMMAND_FAILED, got %+v", env)
	}
	if env.Payload(true) != nil { // data is null
		t.Fatal("Payload on null data must be nil")
	}
}

func TestParseNotJSON(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("want error on non-JSON")
	}
}

// A bare payload is what `unity command --result-only` prints. Decoding it as
// an envelope invents success=false with no errors, which reaches the caller as
// an exit code and nothing on either stream.
func TestParseBarePayloadIsNotAnEnvelope(t *testing.T) {
	bare := []byte(`{"status":"completed","failed":false,"errors":[],"compilationFailed":false}`)
	if _, err := Parse(bare); !errors.Is(err, ErrNotEnvelope) {
		t.Fatalf("Parse(bare payload) err = %v, want ErrNotEnvelope", err)
	}
}

func TestPayloadMissingResultKey(t *testing.T) {
	env, _ := Parse(load(t, "pipeline_list.json")) // data has no "result" key
	if env.Payload(true) != nil {
		t.Fatal("nested Payload on data without result must be nil")
	}
	if !bytes.Contains(env.Payload(false), []byte("instances")) {
		t.Fatal("non-nested Payload must return data verbatim")
	}
}
