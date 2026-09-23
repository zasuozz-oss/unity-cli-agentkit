package official

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// execUnity is swappable in tests to avoid needing a real `unity` binary.
var execUnity = func(args []string, stdout, stderr io.Writer) int {
	bin, err := Bin()
	if err != nil {
		fmt.Fprintln(stderr, "utk:", err)
		return 1
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(stderr, "utk:", err)
		return 1
	}
	return 0
}

// Run streams the official CLI straight through (the --raw path).
func Run(args []string, stdout, stderr io.Writer) int {
	return execUnity(args, stdout, stderr)
}

// Capture runs the official CLI buffering stdout (the filtered path).
// stderr passes straight through so warnings stay visible.
func Capture(args []string, stderr io.Writer) ([]byte, int) {
	var buf bytes.Buffer
	code := execUnity(args, &buf, stderr)
	return buf.Bytes(), code
}

// reloadErrors are the messages the Editor produces while it is reloading its
// domain. All are connection-level: the request either never left utk or died
// with the AppDomain that would have applied it, so replaying it cannot apply
// the same change twice.
var reloadErrors = []string{
	"No Unity Editor instances found with reachable Pipeline servers",
	"Connection reset by server",
	// With --project-path the CLI dials the project's port directly, and a
	// reload in progress answers with a refused connect instead of the above.
	"Cannot connect to Unity Editor Pipeline server",
}

const (
	retryBudget = 8 * time.Second
	retryStep   = 300 * time.Millisecond
)

// CaptureRetry is Capture that rides out a domain reload. Anything recompiling
// scripts — create_script, recompile, editor_play, package add/remove — takes
// the pipeline server down for 0.5-2s, and every call landing in that window
// fails with a connection error that says nothing about the request. Callers
// had to poll editor_status themselves; worse, editor_status answers while the
// Editor is still compiling, so the obvious poll does not actually work.
//
// The happy path pays nothing: only a failing call whose stderr carries a
// reload signature waits at all. Before waiting it asks once whether an Editor
// is up — with none the error is permanent, and stalling for the full budget
// would only make "Unity is not running" take eight seconds to say.
func CaptureRetry(args []string, stderr io.Writer) ([]byte, int) {
	out, errText, code := capture(args)
	if code == 0 || !reloading(out, errText) || !editorRunning() {
		io.WriteString(stderr, errText)
		return out, code
	}
	deadline := time.Now().Add(retryBudget)
	for time.Now().Before(deadline) {
		time.Sleep(retryStep)
		// Only the last attempt's stderr is forwarded: the retried-away errors
		// describe a window that has since closed.
		if out, errText, code = capture(args); code == 0 || !reloading(out, errText) {
			break
		}
	}
	io.WriteString(stderr, errText)
	return out, code
}

func capture(args []string) (stdout []byte, stderr string, code int) {
	var o, e bytes.Buffer
	code = execUnity(args, &o, &e)
	return o.Bytes(), e.String(), code
}

// reloading checks both streams. On the filtered path --json puts the failure
// in the envelope on stdout and leaves stderr empty, so watching stderr alone
// misses every reload the filtered path hits — which is all of them.
func reloading(stdout []byte, stderr string) bool {
	for _, s := range reloadErrors {
		if strings.Contains(stderr, s) || bytes.Contains(stdout, []byte(s)) {
			return true
		}
	}
	return false
}

// editorRunning reports whether any Editor process is up, reachable or not —
// an unreachable one is exactly the case worth waiting for.
func editorRunning() bool {
	raw, code := Capture([]string{"pipeline", "list", "--json", "--no-banner"}, io.Discard)
	if code != 0 {
		return false
	}
	env, err := Parse(raw)
	if err != nil || !env.Success {
		return false
	}
	var d struct {
		Instances []struct {
			IsRunning bool `json:"isRunning"`
		} `json:"instances"`
	}
	if json.Unmarshal(env.Payload(false), &d) != nil {
		return false
	}
	for _, in := range d.Instances {
		if in.IsRunning {
			return true
		}
	}
	return false
}
