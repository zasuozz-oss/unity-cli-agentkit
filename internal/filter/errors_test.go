package filter

import (
	"strings"
	"testing"
)

func TestError(t *testing.T) {
	const catalogue = "Available: [get_quality_settings, delete_asset, editor_play, " +
		"get_console_logs, console_clear, search, eval]"

	cases := []struct {
		name     string
		in       string
		want     []string
		absent   []string
		verbatim bool
	}{
		{
			name: "a typo with no near match points at utk list",
			in:   "Pipeline server returned 400: No command named 'definitely_not_a_tool' is available. " + catalogue,
			want: []string{"definitely_not_a_tool", "7 tools available; see: utk list"},
			// The whole point: the catalogue must not reach the reader.
			absent: []string{"get_quality_settings", "delete_asset", "["},
		},
		{
			name:   "a near miss suggests the real names",
			in:     "No command named 'console' is available. " + catalogue,
			want:   []string{"close: get_console_logs, console_clear"},
			absent: []string{"delete_asset"},
		},
		{
			name:     "an unrelated error is untouched",
			in:       "Asset at path Assets/Missing.prefab could not be loaded",
			verbatim: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, fatal := Error(c.in)
			if !fatal {
				t.Fatal("a non-compile error is always fatal")
			}
			if c.verbatim {
				if got != c.in {
					t.Fatalf("must pass through verbatim, got %q", got)
				}
				return
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %q", w, got)
				}
			}
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Errorf("must not contain %q: %q", a, got)
				}
			}
			if len(got) >= len(c.in) {
				t.Errorf("no compression: %d -> %d", len(c.in), len(got))
			}
		})
	}
}

func TestError_UnreachablePipeline(t *testing.T) {
	in := noPipeline + "\n\nMake sure:\n• Unity Editor is running with a project open\n" +
		"• The Pipeline package is installed in the project\n• The Pipeline HTTP server is running\n\n" +
		"You can check available instances with: unity pipeline list"
	got, _ := Error(in)
	if len(got) >= len(in) {
		t.Fatalf("checklist should be dropped: %q", got)
	}
	for _, want := range []string{noPipeline, "utk status"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Error = %q, missing %q", got, want)
		}
	}
}

func TestPayloadFailed(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		// The three shapes an in-handler refusal actually arrives in.
		{"success false", `{"success":false,"error":"confirm required"}`, true},
		{"status error", `{"status":"error","message":"file not found"}`, true},
		{"success false with data", `{"success":false,"changed":0}`, true},

		{"success true", `{"success":true,"path":"Assets/x.mat"}`, false},
		{"no signal", `{"count":3,"tools":[]}`, false},
		// A nested flag describes one element of the answer, not the call: a
		// listing where one item failed to load is still a successful listing.
		{"nested success false", `{"count":1,"results":[{"success":false}]}`, false},
		{"array payload", `[{"success":false}]`, false},
		{"not json", `plain text output`, false},
		{"empty", ``, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PayloadFailed([]byte(c.payload)); got != c.want {
				t.Fatalf("PayloadFailed(%s) = %v, want %v", c.payload, got, c.want)
			}
		})
	}
}
