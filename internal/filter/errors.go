package filter

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// PayloadFailed reports whether a tool announced its own failure inside the
// payload. The official CLI wraps that payload in an envelope whose success
// flag describes the *transport*, so a tool that refuses the request
// (switch_build_target without confirm), cannot find its input (reload_file on
// a missing path) or rejects the arguments (set_quality_settings with no
// settings object) still comes back as envelope-success and exit 0. Every
// `utk <tool> && next` then runs `next` on a failure.
//
// Only the top level is inspected: a nested success flag belongs to some
// element of the answer, not to the call. Payloads carrying neither field —
// the large majority — are never affected.
func PayloadFailed(payload []byte) bool {
	var d struct {
		Success *bool  `json:"success"`
		Status  string `json:"status"`
	}
	if json.Unmarshal(payload, &d) != nil {
		return false
	}
	return (d.Success != nil && !*d.Success) || d.Status == "error"
}

var (
	// unknownCmd captures the name that was actually typed.
	unknownCmd = regexp.MustCompile(`No command named '([^']+)' is available`)
	// availableList matches the tail the pipeline server appends to that error:
	// all ~140 tool names, unsorted, in one bracket. It costs ~700 tokens and
	// is emitted on every typo.
	availableList = regexp.MustCompile(`Available: \[([^\]]*)\]`)
)

// noPipeline is the first line of the "no reachable Editor" error.
const noPipeline = "No Unity Editor instances found with reachable Pipeline servers."

// Error trims an upstream error message down to what a reader can act on. The
// unknown-command error carries the entire tool catalogue; the useful part is
// the handful of names close to the typo, and the rest is one `utk list` away.
// Messages without that list are returned unchanged.
//
// fatal reports whether the message describes a real failure. It is false only
// for a compilation blob that turned out to hold nothing but warnings — see
// compileFailure — so every other message keeps failing exactly as before.
func Error(msg string) (text string, fatal bool) {
	if s, f, ok := compileFailure(msg); ok {
		return s, f
	}
	// The unreachable-pipeline error ships a 6-line "Make sure:" checklist on
	// every occurrence — including the common transient one, where the Editor is
	// simply mid domain-reload and the checklist is all false alarms.
	if strings.HasPrefix(msg, noPipeline) {
		return noPipeline + " (Editor busy/reloading, or pipeline not installed — check: utk status)", true
	}
	loc := availableList.FindStringSubmatchIndex(msg)
	if loc == nil {
		return msg, true
	}
	var names []string
	for _, n := range strings.Split(msg[loc[2]:loc[3]], ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	typo := ""
	if m := unknownCmd.FindStringSubmatch(msg); m != nil {
		typo = m[1]
	}
	var near []string
	if typo != "" {
		for _, n := range names {
			if strings.Contains(n, typo) || strings.Contains(typo, n) {
				near = append(near, n)
			}
		}
	}
	tail := strconv.Itoa(len(names)) + " tools available; see: utk list"
	if len(near) > 0 {
		tail = "close: " + strings.Join(near, ", ") + " (" + tail + ")"
	}
	return strings.TrimRight(msg[:loc[0]], " ") + " " + tail, true
}

// compileHead opens every failed `eval` / `eval_file` compilation.
const compileHead = "Compilation Failed"

// errorCap bounds the error list. One bad type name in a 370-line snippet
// produces dozens of follow-on diagnostics; past the first handful they all
// describe the same root cause and none of them is the one to fix.
const errorCap = 12

// scopeHint fires on the diagnostics a snippet gets for being written like a
// file instead of a method body. It is the single most common first-attempt
// failure and the message alone does not hint at the cause, so finding it costs
// a round-trip of guessing.
var scopeHint = regexp.MustCompile(`(?i)using directive|could not be found|does not exist in the current context|is a namespace but is used like a type`)

const execContract = "hint: an eval snippet is a *method body* — `using` is invalid there, and only " +
	"UnityEngine/UnityEditor are in scope; write other namespaces in full (UnityEngine.UI.Image, TMPro.TextMeshProUGUI)"

// compileFailure reformats the blob `eval` returns for a failed compilation:
//
//	Compilation Failed
//	  Identifier expected (line 1, col 55)
//	  Unreachable code detected (line 2, col 12)
//	  …
//
// Every diagnostic arrives at the same indent with its CS code stripped, so
// warnings are indistinguishable from the errors that actually stopped the
// compile, and a cascade repeats one root cause until it fills the screen.
// Errors come first and deduped, warnings collapse to a single trailing line,
// and fatal is false when there were no errors at all.
//
// ok is false for anything that is not this blob, leaving Error's other paths
// untouched.
func compileFailure(msg string) (text string, fatal bool, ok bool) {
	if !strings.HasPrefix(msg, compileHead) {
		return "", false, false
	}
	var errs, warns []string
	seen := map[string]bool{}
	for _, line := range strings.Split(msg, "\n")[1:] {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		if IsWarningText(line) {
			// eval wraps the snippet in a method and appends its own `return`
			// tail, so *every* failed compile reports unreachable code one line
			// past the caller's last one. It is never about the snippet and
			// there is nothing to change in response.
			if strings.Contains(strings.ToLower(line), "unreachable code detected") {
				continue
			}
			warns = append(warns, line)
		} else {
			errs = append(errs, line)
		}
	}
	var b strings.Builder
	b.WriteString(compileHead + " — " + count(len(errs), "error") + ", " + count(len(warns), "warning"))
	shown := errs
	if len(shown) > errorCap {
		shown = shown[:errorCap]
	}
	for _, e := range shown {
		b.WriteString("\n  " + e)
	}
	if n := len(errs) - len(shown); n > 0 {
		b.WriteString("\n  …+" + strconv.Itoa(n) + " more errors (fix the first ones and recompile)")
	}
	if len(warns) > 0 {
		b.WriteString("\n  warnings (did not fail the compile): " + strings.Join(warns, "; "))
	}
	if len(errs) > 0 && scopeHint.MatchString(strings.Join(errs, "\n")) {
		b.WriteString("\n  " + execContract)
	}
	return b.String(), len(errs) > 0, true
}

// count renders "1 error" / "3 errors".
func count(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}
