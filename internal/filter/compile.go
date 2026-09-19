package filter

import (
	"regexp"
	"strings"
)

// diagLine matches a compiler diagnostic as the Editor console records it:
//
//	Assets/Scripts/Foo.cs(12,20): error CS0246: The type or namespace name 'Bar' …
//
// The location changes on every occurrence while severity+code+text identify
// the cause, which is what makes a 60-line cascade collapsible to its handful
// of roots.
var diagLine = regexp.MustCompile(`^(.+?)\((\d+,\d+)\): (error|warning) (CS\d+): (.*)$`)

// Diag is one compiler diagnostic split into the part that names its root cause
// (Severity+Code+Text) and the part that only says where it surfaced.
type Diag struct {
	File     string
	Pos      string
	Severity string
	Code     string
	Text     string
}

// Key identifies the root cause: every site reporting the same code with the
// same text is the same mistake, however many files it broke.
func (d Diag) Key() string { return d.Severity + " " + d.Code + " " + d.Text }

// Site renders the location alone, for the roll-up under a grouped diagnostic.
func (d Diag) Site() string { return d.File + "(" + d.Pos + ")" }

// ParseDiag splits a compiler diagnostic. ok is false for anything else, so a
// runtime log or an exception is never reshaped by the compile-error path.
func ParseDiag(msg string) (Diag, bool) {
	line, _, _ := strings.Cut(msg, "\n")
	m := diagLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
	if m == nil {
		return Diag{}, false
	}
	return Diag{File: m[1], Pos: m[2], Severity: m[3], Code: m[4], Text: m[5]}, true
}

// warningPhrases identify Roslyn messages that are warnings even when nothing
// says so. `unity command eval` lists every diagnostic under one "Compilation
// Failed" heading with the CS code stripped, so an obsolete-API note and a
// missing type read exactly alike — and a caller reasonably treats both as the
// reason the build is red.
//
// Matching is deliberately one-way: only a confident match is downgraded, so an
// unrecognised diagnostic keeps being reported as an error, as it is today.
var warningPhrases = []string{
	"unreachable code detected",
	"is obsolete",
	"is declared but never used",
	"but its value is never used",
	"is never used",
	"hides inherited member",
	"possible unintended reference comparison",
	"is assigned but",
}

// IsWarningText reports whether an unlabelled Roslyn message is a warning.
func IsWarningText(msg string) bool {
	l := strings.ToLower(msg)
	for _, p := range warningPhrases {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}
