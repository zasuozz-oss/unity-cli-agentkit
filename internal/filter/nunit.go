package filter

import (
	"encoding/xml"
	"strings"
)

// nunitCase mirrors one <test-case> of an NUnit3 report. A failure carries
// <failure><message>/<stack-trace>; an ignored test carries <reason><message>.
type nunitCase struct {
	FullName string `xml:"fullname,attr"`
	Result   string `xml:"result,attr"`
	Failure  struct {
		Message    string `xml:"message"`
		StackTrace string `xml:"stack-trace"`
	} `xml:"failure"`
	Reason struct {
		Message string `xml:"message"`
	} `xml:"reason"`
}

// nunitSuite nests to whatever depth the namespaces do — assembly, then one
// level per namespace segment, then the fixture — so it recurses rather than
// naming a fixed path.
type nunitSuite struct {
	Suites []nunitSuite `xml:"test-suite"`
	Cases  []nunitCase  `xml:"test-case"`
}

// nunitRun is the <test-run> root.
type nunitRun struct {
	Total        int          `xml:"total,attr"`
	Passed       int          `xml:"passed,attr"`
	Failed       int          `xml:"failed,attr"`
	Skipped      int          `xml:"skipped,attr"`
	Inconclusive int          `xml:"inconclusive,attr"`
	Duration     float64      `xml:"duration,attr"`
	Suites       []nunitSuite `xml:"test-suite"`
}

// cases flattens the suite tree depth-first.
func (s nunitSuite) cases() []nunitCase {
	out := s.Cases
	for _, sub := range s.Suites {
		out = append(out, sub.cases()...)
	}
	return out
}

// NUnit renders an NUnit3 report the way Tests renders a pipeline payload.
// `unity test`'s own JSON carries only the path to this file — no counts, no
// failures — so without it a batchmode run reports nothing at all. ok is false
// for anything that is not a report, so callers stay loss-safe.
func NUnit(data []byte, mode string) (out []byte, failed, ok bool) {
	var r nunitRun
	if err := xml.Unmarshal(data, &r); err != nil || r.Total == 0 {
		return nil, false, false
	}
	d := testData{
		Summary: &testSummary{
			Total: &r.Total, Passed: r.Passed, Failed: r.Failed,
			Skipped: r.Skipped, Inconclusive: r.Inconclusive,
		},
		Duration: r.Duration,
		Mode:     mode,
	}
	for _, s := range r.Suites {
		for _, c := range s.cases() {
			msg := c.Failure.Message
			if msg == "" {
				msg = c.Reason.Message
			}
			d.Results = append(d.Results, testResult{
				FullName:   c.FullName,
				Status:     strings.TrimSpace(c.Result),
				Message:    msg,
				StackTrace: c.Failure.StackTrace,
			})
		}
	}
	return render(d), r.Failed > 0 || r.Inconclusive > 0, true
}
