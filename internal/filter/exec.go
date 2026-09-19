package filter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

// Exec compacts exec output. Default (truncate=0) is loss-safe: valid JSON has
// its insignificant whitespace removed; anything that is not valid JSON is
// returned byte-for-byte unchanged. When truncate>0, JSON arrays longer than
// truncate are shortened with a "…+N more" marker (opt-in; the only lossy mode).
func Exec(raw []byte, truncate int) []byte {
	// UseNumber keeps integers/decimals as their exact source text so large
	// instanceIDs/entity IDs and high-precision values survive the round-trip
	// (a plain any decode would coerce every number to float64 and corrupt them).
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw // not JSON → never touch it (spec §3.2 safety)
	}
	// Decoder stops after one value; reject trailing non-whitespace so output
	// like "{...}\n<log text>" or multiple values still passes through raw.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return raw
	}
	if m, ok := v.(map[string]any); ok {
		v = stripToolEnvelope(m)
	}
	// A bare string result ("reserialized", "hi") is the whole answer; quoting
	// and escaping it back into JSON only adds noise for the reader.
	if s, ok := v.(string); ok {
		return []byte(s)
	}
	if truncate > 0 {
		v = truncateArrays(v, truncate)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return raw // re-encode failed → fall back to raw, lose nothing
	}
	// json.Encoder appends a newline; trim it to match compact behavior.
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// envelopeNoise are the tool-envelope fields worth dropping when they carry
// nothing. Every `unity command <tool>` response nests its payload in this
// wrapper, so the nulls are paid for on every single call.
var envelopeNoise = []string{"result", "output", "message", "error", "errorDetails", "diagnostics", "success"}

// wrapperFields are the per-tool result wrapper's own keys. Recognising the
// wrapper needs one of these beside a boolean success: com.unity.pipeline
// 0.6.0 drops null keys from a response, so executedAt — which every wrapper
// used to carry — is simply absent from a lean one (run_script, batch). Gating
// on it alone left those payloads unstripped.
var wrapperFields = []string{
	"executedAt", "executionTimeMs", "command", "diagnostics",
}

// stripToolEnvelope removes the dead weight from the per-tool result wrapper
// the official CLI nests under data.result:
//
//	{"command":null,"diagnostics":[],"error":null,"errorDetails":null,
//	 "executedAt":"0001-01-01T00:00:00","executionTimeMs":483,"message":null,
//	 "output":null,"result":"reserialized","success":true}
//
// Only empty fields are dropped, so nothing informative is lost. Payloads that
// are not this envelope are returned untouched — some tools merge their own
// fields into the wrapper, and a whitelist would silently eat them.
func stripToolEnvelope(m map[string]any) any {
	if _, ok := m["success"].(bool); !ok {
		return m
	}
	if !hasAny(m, wrapperFields) {
		return m
	}
	// Always noise: a zero date, timings the caller did not ask for, and
	// run_script's generated assembly name — an internal handle that means
	// nothing outside the Editor that produced it. `--raw` still has them all.
	for _, k := range []string{
		"executedAt", "executionTimeMs", "compileMs", "executeMs", "assemblyName",
	} {
		delete(m, k)
	}
	// The tool name is already in the command the caller typed.
	delete(m, "command")
	for _, k := range envelopeNoise {
		if v, ok := m[k]; ok && isEmpty(v) {
			delete(m, k)
		}
	}
	// success:true is implied by the absence of an error; a false one stays.
	if v, ok := m["success"]; ok && v == true {
		delete(m, "success")
	}
	if len(m) == 1 {
		if r, ok := m["result"]; ok {
			return r
		}
	}
	return m
}

// hasAny reports whether m carries at least one of keys.
func hasAny(m map[string]any, keys []string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// isEmpty reports whether v carries no information: null, "", [] or {}.
func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

func truncateArrays(v any, n int) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, 0, len(t))
		for i, e := range t {
			if i >= n {
				out = append(out, "…+"+strconv.Itoa(len(t)-n)+" more")
				break
			}
			out = append(out, truncateArrays(e, n))
		}
		return out
	case map[string]any:
		for k, e := range t {
			t[k] = truncateArrays(e, n)
		}
		return t
	default:
		return v
	}
}
