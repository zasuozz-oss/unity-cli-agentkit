// Package official executes the official Unity CLI (`unity`) and decodes its
// JSON envelope. It replaces the vendored in-process engine as utk's transport.
package official

import (
	"encoding/json"
	"errors"
)

// ErrNotEnvelope reports JSON that decodes but carries no envelope, so callers
// take their raw-passthrough path instead of reading an invented failure.
var ErrNotEnvelope = errors.New("official: not a CLI envelope")

// Item is one entry of the envelope's errors/warnings arrays.
type Item struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope is the top-level JSON the official CLI emits with --json
// (measured from 1.0.0-beta.3, unchanged through 1.0.0-beta.10; see testdata/).
type Envelope struct {
	Success  bool            `json:"success"`
	Command  string          `json:"command"`
	Data     json.RawMessage `json:"data"`
	Errors   []Item          `json:"errors"`
	Warnings []Item          `json:"warnings"`
}

// Parse decodes an envelope. Any non-envelope input is an error so callers
// can fall back to raw output (loss-safe).
func Parse(raw []byte) (*Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	// encoding/json ignores unknown fields, so a bare payload decodes into a
	// zero Envelope — success=false with no errors to show, which callers read
	// as a failure they cannot describe. `unity command --result-only` (CLI
	// 1.0.0-beta.10) prints exactly that shape. Every real envelope carries
	// `success`, so its absence is what separates the two.
	var probe struct {
		Success *bool `json:"success"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Success == nil {
		return nil, ErrNotEnvelope
	}
	return &e, nil
}

// Payload extracts the real payload. `unity command <tool>` nests it at
// data.result (data also carries a parameters echo and target — noise);
// `unity list` / `unity pipeline list` put it directly in data. Returns nil
// when the expected shape is absent so callers fall back to raw.
func (e *Envelope) Payload(nested bool) json.RawMessage {
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return nil
	}
	if !nested {
		return e.Data
	}
	var d struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(e.Data, &d); err != nil || len(d.Result) == 0 {
		return nil
	}
	return d.Result
}
