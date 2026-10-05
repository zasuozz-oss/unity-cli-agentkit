package official

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(stderr, "utk:", err)
		return 1
	}
	childMu.Lock()
	children[cmd] = struct{}{}
	childMu.Unlock()
	err = cmd.Wait()
	childMu.Lock()
	delete(children, cmd)
	childMu.Unlock()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(stderr, "utk:", err)
		return 1
	}
	return 0
}

var (
	childMu  sync.Mutex
	children = map[*exec.Cmd]struct{}{}
)

// KillChildren ends every `unity` call in flight. utk exiting on its own
// deadline would otherwise leave them talking to the Editor.
func KillChildren() {
	childMu.Lock()
	defer childMu.Unlock()
	for c := range children {
		c.Process.Kill()
	}
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
	// The first call after `editor play`/`refresh` reaches a socket the
	// reloading domain is tearing down; the CLI's HttpClient reports that as
	// a send failure, not a refused connect.
	"Network error: An error occurred while sending the request",
}

// Restarting reports that `utk editor restart` has the Editor down on purpose.
// Without it a call landing in that minute finds no Editor process at all and
// fails at once with "not running" — the one moment waiting is certain to work.
// It must turn false by itself (utk keys it on a marker file's age).
var Restarting = func() bool { return false }

const (
	// A play-mode or post-import reload in a mid-size project keeps the server
	// down 10-30s; at 8s the agents' own `sleep 5` retry loops were still the
	// norm after every `editor play`.
	retryBudget = 30 * time.Second
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
// would only make "Unity is not running" take the whole retry budget to say.
func CaptureRetry(args []string, stderr io.Writer) ([]byte, int) {
	out, errText, code := capture(args)
	// Between the old Editor's exit and the new one's first heartbeat the CLI
	// knows no Editor for the project at all; mid-restart that too is a window.
	away := func() bool {
		return reloading(out, errText) || Restarting() && (strings.Contains(errText, noInstance) || bytes.Contains(out, []byte(noInstance)))
	}
	if code == 0 || !away() || !(editorRunning() || Restarting()) {
		io.WriteString(stderr, errText)
		return out, code
	}
	deadline := time.Now().Add(retryBudget)
	for time.Now().Before(deadline) || Restarting() {
		time.Sleep(retryStep)
		// Only the last attempt's stderr is forwarded: the retried-away errors
		// describe a window that has since closed.
		if out, errText, code = capture(args); code == 0 || !away() {
			break
		}
	}
	io.WriteString(stderr, errText)
	return out, code
}

const noInstance = "No Pipeline instance found"

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
