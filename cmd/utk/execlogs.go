package main

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// logCap bounds the merged log block. A snippet that logs in a loop would
// otherwise turn a one-value answer into a wall of text.
const logCap = 20

// snippetLogs returns the console entries the Editor recorded at or after
// `since` — the ones the snippet just evaluated wrote itself.
//
// `eval` discards Debug.Log: its result envelope carries an `output` field that
// is null on every call, measured, including for snippets whose entire answer
// is what they logged. Those snippets come back as `{}` and the log is one
// `utk console` away — a second round-trip for output the first call already
// produced. Fetching it here costs one extra local call and no agent turn.
//
// Entries are matched by timestamp rather than by diffing the log against a
// before-state, so it stays a single call; Editor and utk read the same wall
// clock. Set UTK_NO_EXEC_LOGS=1 to skip it.
func snippetLogs(since time.Time, projectPath string) []byte {
	if os.Getenv("UTK_NO_EXEC_LOGS") != "" {
		return nil
	}
	args := []string{"command", "console", "--tail", "50", "--json", "--no-banner"}
	if projectPath != "" {
		args = append(args, "--project-path", projectPath)
	}
	raw, code := official.Capture(args, io.Discard)
	if code != 0 {
		return nil
	}
	env, err := official.Parse(raw)
	if err != nil || !env.Success {
		return nil
	}
	var d struct {
		Logs []struct {
			// logType is 0.7.0's exact LogType; level is the collapsed
			// severity every version reports. Prefer the former when present.
			LogType   string `json:"logType"`
			Level     string `json:"level"`
			Message   string `json:"message"`
			Timestamp string `json:"timestampUtc"`
		} `json:"entries"`
	}
	if json.Unmarshal(env.Payload(true), &d) != nil {
		return nil
	}
	// The tool answers newest-first; a snippet's own logs read in the order it
	// wrote them.
	var b strings.Builder
	kept := 0
	for i := len(d.Logs) - 1; i >= 0; i-- {
		l := d.Logs[i]
		t, terr := time.Parse(time.RFC3339Nano, l.Timestamp)
		if terr != nil || t.Before(since) {
			continue
		}
		kept++
		if kept > logCap {
			continue
		}
		sev := l.LogType
		if sev == "" {
			sev = l.Level
		}
		b.WriteString("[" + sev + "] " + firstLine(l.Message) + "\n")
	}
	if kept > logCap {
		b.WriteString("…+" + strconv.Itoa(kept-logCap) + " more (utk console)\n")
	}
	return []byte(b.String())
}

// firstLine drops a log's trailing detail; the message itself is the signal and
// the rest is one `utk console` away.
func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}
