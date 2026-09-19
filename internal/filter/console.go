package filter

import (
	"encoding/json"
	"strconv"
	"strings"
)

type consoleLog struct {
	// LogType is Unity's exact LogType, added by com.unity.pipeline 0.7.0; it
	// keeps the Exception/Assert distinction that Level collapses into "error".
	// Absent on 0.6.0, where Level is all there is.
	LogType    string `json:"logType"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	StackTrace string `json:"stackTrace"`
}

// severity is what the entry is labelled with: the exact LogType when the
// package reports one, else the collapsed level.
func (l consoleLog) severity() string {
	if l.LogType != "" {
		return l.LogType
	}
	return l.Level
}

type consoleResult struct {
	// Entries is a pointer so an absent key (the wrong schema) is told apart
	// from a present-but-empty list (a genuinely quiet console).
	Entries  *[]consoleLog `json:"entries"`
	Returned int           `json:"returned"`
	Dropped  bool          `json:"dropped"`
}

// groupCap bounds how many distinct entries are rendered. Root-cause grouping
// already collapses a compile cascade to its handful of causes, so this only
// catches genuine floods (a log in Update, a per-frame exception) — where the
// tail is repetition and the head is the thing to read.
const groupCap = 40

// Console compacts the `console` tool's JSON (official CLI). Compiler
// diagnostics are grouped by root cause — severity+code+text, with the
// locations rolled up — and emitted first in compiler order, because one bad
// type name reports at every site that used it and the tool hands them back
// newest-first, so a plain truncation drops the error that caused all the
// others. Remaining entries are deduped verbatim, stacktraces trimmed to
// first+last frame.
//
// only narrows the result to one exact severity, because the tool's --level is
// a *minimum*: utk's `--type warning` means warning alone, and asking for
// --level warn also brings back every error. Empty means keep everything.
//
// Input that is not the expected shape passes through unchanged (loss-safe).
func Console(result []byte, only string) []byte {
	var r consoleResult
	if err := json.Unmarshal(result, &r); err != nil || r.Entries == nil {
		return result // not the expected schema → never touch it
	}

	type group struct {
		log   consoleLog
		diag  Diag
		isDia bool
		sites []string
		count int
	}
	var diagOrder, plainOrder []string
	groups := map[string]*group{}
	add := func(key string, g *group) {
		if prev, ok := groups[key]; ok {
			prev.count++
			if g.isDia {
				prev.sites = append(prev.sites, g.sites...)
			}
			return
		}
		groups[key] = g
		if g.isDia {
			diagOrder = append(diagOrder, key)
		} else {
			plainOrder = append(plainOrder, key)
		}
	}
	errCount, warnCount := 0, 0
	kept := 0
	for _, l := range *r.Entries {
		if only != "" && !matchesSeverity(l, only) {
			continue
		}
		kept++
		if d, ok := ParseDiag(l.Message); ok {
			if d.Severity == "warning" {
				warnCount++
			} else {
				errCount++
			}
			add(d.Key(), &group{diag: d, isDia: true, sites: []string{d.Site()}, count: 1})
			continue
		}
		add(l.severity()+"\x00"+l.Message+"\x00"+l.StackTrace, &group{log: l, count: 1})
	}
	// The tool answers newest-first; the compiler emitted these oldest first,
	// and the first error is the one worth reading.
	reverse(diagOrder)

	returned := r.Returned
	if only != "" {
		returned = kept
	}
	var b strings.Builder
	// No buffer total here: the `console` tool reports none on 0.6.0. 0.7.0
	// adds a `counts` object that could restore one — left alone until there
	// is a 0.7.0 Editor to measure it against.
	b.WriteString("console: " + strconv.Itoa(returned) + " returned")
	if r.Dropped {
		b.WriteString(", some dropped")
	}
	if errCount+warnCount > 0 {
		b.WriteString("; compile: " + count(errCount, "error") + ", " + count(warnCount, "warning"))
	}
	b.WriteString("\n")

	order := append(diagOrder, plainOrder...)
	shown := order
	if len(shown) > groupCap {
		shown = shown[:groupCap]
	}
	for _, key := range shown {
		g := groups[key]
		times := ""
		if g.count > 1 {
			times = " ×" + strconv.Itoa(g.count)
		}
		if g.isDia {
			b.WriteString("[" + g.diag.Severity + " " + g.diag.Code + times + "] " + g.diag.Text + "\n")
			b.WriteString(" " + g.sites[0])
			if n := len(g.sites) - 1; n > 0 {
				b.WriteString(" +" + strconv.Itoa(n) + " more sites")
			}
			b.WriteString("\n")
			continue
		}
		b.WriteString("[" + g.log.severity() + times + "] " + g.log.Message + "\n")
		for _, fr := range trimFrames(g.log.StackTrace) {
			b.WriteString(" " + fr + "\n")
		}
	}
	if n := len(order) - len(shown); n > 0 {
		b.WriteString("…+" + strconv.Itoa(n) + " more distinct entries (raise --limit or use --raw)\n")
	}
	return []byte(b.String())
}

// matchesSeverity reports whether an entry is exactly the wanted severity.
// Entries carry either the collapsed level ("log"/"warn"/"error") or 0.7.0's
// exact LogType ("Log"/"Warning"/"Error"/"Exception"/"Assert"), so both
// spellings are accepted for the one utk name.
func matchesSeverity(l consoleLog, want string) bool {
	switch want {
	case "error":
		return l.Level == "error" || isAnyOf(l.LogType, "Error", "Exception", "Assert")
	case "warning":
		return l.Level == "warn" || isAnyOf(l.LogType, "Warning")
	case "log":
		return l.Level == "log" || isAnyOf(l.LogType, "Log")
	}
	return true
}

func isAnyOf(v string, names ...string) bool {
	for _, n := range names {
		if v == n {
			return true
		}
	}
	return false
}

func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// trimFrames keeps the first and last stack frame, replacing the middle with
// an explicit omission marker.
func trimFrames(stack string) []string {
	stack = strings.TrimRight(stack, "\n")
	if stack == "" {
		return nil
	}
	frames := strings.Split(stack, "\n")
	if len(frames) <= 2 {
		return frames
	}
	return []string{
		frames[0],
		"… " + strconv.Itoa(len(frames)-2) + " frames omitted",
		frames[len(frames)-1],
	}
}
