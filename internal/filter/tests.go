package filter

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

type testSummary struct {
	Total        *int `json:"Total"`
	Passed       int  `json:"Passed"`
	Failed       int  `json:"Failed"`
	Skipped      int  `json:"Skipped"`
	Inconclusive int  `json:"Inconclusive"`
}

type testResult struct {
	FullName   string `json:"FullName"`
	Status     string `json:"Status"`
	Message    string `json:"Message"`
	StackTrace string `json:"StackTrace"`
}

type testData struct {
	Summary  *testSummary `json:"Summary"`
	Results  []testResult `json:"Results"`
	Duration float64      `json:"Duration"`
	Mode     string       `json:"Mode"`
	// Status is test_status's own field ("running"/"completed"); it is the
	// answer to the question that command is asked, so it leads the header.
	Status string `json:"status"`
	// Message is the tool envelope's own note, not a test's. It carries the
	// "PlayMode started asynchronously — poll test_status" hand-off, so dropping
	// it would strand the agent waiting for results that never print.
	Message string `json:"message"`
}

// frameFile pulls "File.cs:123" out of a C# stack frame, discarding the
// absolute path and the IL offset that make up most of its length.
var frameFile = regexp.MustCompile(`[^\\/]+\.cs:\d+`)

// parseTests decodes a run_tests/test_status payload. ok is false for anything
// that is not one, so callers can fall back to passing the bytes through.
func parseTests(data []byte) (testData, bool) {
	// test_status hands back its report as a JSON *string* rather than an
	// object, and with lowercase keys. Unwrap the extra encoding layer so both
	// tools reach the same renderer — encoding/json matches field names
	// case-insensitively, so the casing difference needs nothing.
	if len(data) > 0 && data[0] == '"' {
		var s string
		if json.Unmarshal(data, &s) == nil {
			data = []byte(s)
		}
	}
	var d testData
	if err := json.Unmarshal(data, &d); err != nil || d.Summary == nil || d.Summary.Total == nil {
		return d, false
	}
	return d, true
}

// TestsFailed reports whether a test payload contains failures, so a red run
// can set a non-zero exit code. A successful *command* that reports failing
// *tests* still exits 0 upstream, which reads as "tests passed" to every
// script, CI step and retry loop that checks $?.
func TestsFailed(data []byte) bool {
	d, ok := parseTests(data)
	return ok && (d.Summary.Failed > 0 || d.Summary.Inconclusive > 0)
}

// Tests renders `run_tests` output as a summary plus the failures only. Passing
// tests are the bulk of the payload (57 entries of null Message/StackTrace here)
// and carry no information a summary count does not, so they are dropped — the
// one deliberately lossy filter, announced in the output and bypassable with
// --raw. Unexpected input passes through unchanged (loss-safe).
func Tests(data []byte) []byte {
	d, ok := parseTests(data)
	if !ok {
		// test_status answers "no run in progress" with status/message and no
		// Summary. Returning the raw bytes there prints the escaped JSON string
		// its extra encoding layer arrived in, so render the two fields instead.
		if line := strings.Trim(d.Status+": "+d.Message, ": "); line != "" {
			return []byte(line + "\n")
		}
		return data
	}
	return render(d)
}

// render turns a decoded test payload into the summary + failures view. Split
// out of Tests so the batchmode runner, whose results only exist as NUnit XML,
// reports in the same shape as a pipeline run.
func render(d testData) []byte {
	s := d.Summary

	var b strings.Builder
	if d.Status != "" {
		b.WriteString(d.Status + ": ")
	}
	b.WriteString(strconv.Itoa(*s.Total) + " tests: " + strconv.Itoa(s.Passed) + " passed")
	for _, part := range []struct {
		label string
		n     int
	}{{"failed", s.Failed}, {"skipped", s.Skipped}, {"inconclusive", s.Inconclusive}} {
		if part.n > 0 {
			b.WriteString(", " + strconv.Itoa(part.n) + " " + part.label)
		}
	}
	if d.Duration > 0 {
		b.WriteString("  " + strconv.FormatFloat(d.Duration, 'f', 1, 64) + "s")
	}
	if d.Mode != "" {
		b.WriteString("  mode=" + d.Mode)
	}
	b.WriteString("\n")
	if d.Message != "" {
		b.WriteString("note: " + d.Message + "\n")
	}

	shown := 0
	for _, r := range d.Results {
		if r.Status == "Passed" {
			continue
		}
		shown++
		b.WriteString(strings.ToUpper(r.Status) + " " + r.FullName + "\n")
		for _, line := range strings.Split(strings.ReplaceAll(r.Message, "\r\n", "\n"), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				b.WriteString("  " + line + "\n")
			}
		}
		if at := frameFile.FindString(r.StackTrace); at != "" {
			b.WriteString("  at " + at + "\n")
		}
	}
	if omitted := len(d.Results) - shown; omitted > 0 {
		b.WriteString("(" + strconv.Itoa(omitted) + " passing test names omitted; --raw for all)\n")
	}
	return []byte(b.String())
}
