package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zasuo/unity-cli-agentkit/internal/official"
)

// editorOps is everything a restart does to the Editor process. A struct so
// tests swap the whole Editor for a recorder.
type editorOps struct {
	find  func(project string) (pid int, bin string)
	state func(project string) (string, bool) // guardSnippet's answer; false = the Editor did not answer
	quit  func(project string)
	alive func(pid int) bool
	kill  func(pid int)
	start func(bin, project string) error
	ready func(project string, budget time.Duration, stderr io.Writer) (string, bool)
}

var editor editorOps

// Set in init: queryEditorState goes through run, which dispatches back here.
func init() {
	editor = editorOps{
		find:  findEditor,
		state: queryEditorState,
		quit:  quitEditor,
		alive: pidAlive,
		kill: func(pid int) {
			if p, err := os.FindProcess(pid); err == nil {
				p.Kill()
			}
		},
		start: startEditor,
		ready: waitEditorReady,
	}
	official.Restarting = restartInProgress
}

var (
	restartQuitWait  = 60 * time.Second
	restartStep      = 500 * time.Millisecond
	restartReadyWait = 10 * time.Minute // a cold start reimports and recompiles
	waitStep         = time.Second
)

// guardSnippet answers what a restart would destroy. isDirty covers scenes
// only: a prefab stage with unsaved edits also shows as a dirty stage scene.
const guardSnippet = `int dirty = 0; for (int i = 0; i < UnityEngine.SceneManagement.SceneManager.sceneCount; i++) if (UnityEngine.SceneManagement.SceneManager.GetSceneAt(i).isDirty) dirty++; return "playing=" + UnityEditor.EditorApplication.isPlayingOrWillChangePlaymode + " dirty=" + dirty + " busy=" + (UnityEditor.EditorApplication.isCompiling || UnityEditor.EditorApplication.isUpdating);`

const quitSnippet = `UnityEditor.EditorApplication.Exit(0); return "bye";`

func queryEditorState(project string) (string, bool) {
	var out bytes.Buffer
	code := run([]string{"exec", guardSnippet, "--project-path", project, "--timeout", "15000"}, &out, io.Discard)
	return out.String(), code == 0
}

// quitEditor does not wait for an answer: Exit(0) takes the HTTP server down
// with the process, so the reply is a connection error by design — and going
// through CaptureRetry would resend the quit for as long as the marker stands.
func quitEditor(project string) {
	go official.Capture([]string{"command", "eval", quitSnippet, "--project-path", project, "--json", "--no-banner"}, io.Discard)
}

// startEditor launches the same binary on the same project and lets go of it.
// The Hub's own flags (-useHub -hubIPC …) are session tokens of the launch
// that just ended, so they are not replayed.
func startEditor(bin, project string) error {
	cmd := exec.Command(bin, "-projectPath", project)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// restartMarker tells every other utk on the machine that the Editor is down
// on purpose, so a connection error is worth waiting out (see official.Restarting).
func restartMarker() string { return filepath.Join(kitHome(), "restarting") }

func restartInProgress() bool {
	info, err := os.Stat(restartMarker())
	return err == nil && time.Since(info.ModTime()) < restartReadyWait+2*time.Minute
}

// restartEditor quits the project's Editor, starts it again and waits until it
// answers. Unforced, it refuses whenever the Editor holds something a restart
// would lose.
func restartEditor(project string, force bool, stderr io.Writer) int {
	pid, bin := editor.find(project)
	if pid == 0 {
		fmt.Fprintf(stderr, "utk: no running Editor for %s — nothing to restart (open the project in Unity first)\n", project)
		return 1
	}
	if !force {
		st, ok := editor.state(project)
		why := ""
		switch {
		case !ok:
			why = "it does not answer, so utk cannot tell whether it holds unsaved work"
		case strings.Contains(st, "playing=True"):
			why = "it is in Play mode (`utk editor stop` first)"
		case !strings.Contains(st, "dirty=0"):
			why = "it has unsaved scene changes (save or discard them first)"
		case strings.Contains(st, "busy=True"):
			why = "it is compiling or importing (`utk editor wait` first)"
		}
		if why != "" {
			fmt.Fprintf(stderr, "utk: not restarting the Editor: %s\n  --force restarts anyway and loses whatever is unsaved\n", why)
			return 1
		}
	}
	started := time.Now()
	os.MkdirAll(kitHome(), 0o755)
	os.WriteFile(restartMarker(), []byte(project), 0o644)
	defer os.Remove(restartMarker())

	editor.quit(project)
	if !waitGone(pid, restartQuitWait) {
		// Either --force on a hung Editor, or one that passed the guard and
		// still would not quit: nothing unsaved is at stake in both cases.
		editor.kill(pid)
		if !waitGone(pid, 10*time.Second) {
			fmt.Fprintf(stderr, "utk: Editor pid %d survived a kill; restart it by hand\n", pid)
			return 1
		}
	}
	if err := editor.start(bin, project); err != nil {
		fmt.Fprintln(stderr, "utk: could not start the Editor:", err)
		return 1
	}
	if why, ok := editor.ready(project, restartReadyWait, stderr); !ok {
		fmt.Fprintf(stderr, "utk: the Editor was started but is not ready after %s (%s)\n", restartReadyWait, why)
		return 1
	}
	fmt.Fprintf(stderr, "utk: Editor restarted in %ds\n", int(time.Since(started).Seconds()))
	return 0
}

func waitGone(pid int, budget time.Duration) bool {
	for deadline := time.Now().Add(budget); editor.alive(pid); time.Sleep(restartStep) {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
}

// editorBusy reads one editor_status answer: "" when the Editor can take a
// command now, else the reason it cannot.
func editorBusy(raw []byte) string {
	env, err := official.Parse(raw)
	if err != nil {
		return "unreachable"
	}
	if !env.Success {
		if len(env.Errors) > 0 {
			return strings.SplitN(env.Errors[0].Message, "\n", 2)[0]
		}
		return "unreachable"
	}
	var d struct {
		Status                 string `json:"status"`
		Compiling              bool   `json:"compiling"`
		DomainReloadInProgress bool   `json:"domainReloadInProgress"`
	}
	json.Unmarshal(env.Payload(true), &d)
	switch {
	case d.DomainReloadInProgress:
		return "reloading the domain"
	case d.Compiling:
		return "compiling"
	case d.Status != "" && d.Status != "ready":
		return d.Status
	}
	return ""
}

// waitEditorReady blocks until the Editor answers and is neither compiling nor
// reloading. It replaces the `until utk editor status …; do sleep 5; done`
// loop every caller wrote for itself, with a guessed budget each time.
func waitEditorReady(project string, budget time.Duration, stderr io.Writer) (why string, ok bool) {
	args := []string{"command", "editor_status", "--json", "--no-banner"}
	if project != "" {
		args = append(args, "--project-path", project)
	}
	deadline, lastModal := time.Now().Add(budget), time.Now()
	for {
		raw, _ := official.Capture(args, io.Discard)
		if why = editorBusy(raw); why == "" {
			return "", true
		}
		if time.Now().After(deadline) {
			return why, false
		}
		// A startup dialog (Safe Mode) keeps the server down until answered.
		if time.Since(lastModal) > 15*time.Second {
			answerModal(stderr)
			lastModal = time.Now()
		}
		time.Sleep(waitStep)
	}
}

func editorProject(args []string, stderr io.Writer) string {
	if p := findFlag(args, "--project-path"); p != "" {
		return p
	}
	p := localProjectRoot()
	if p == "" {
		fmt.Fprintln(stderr, "utk: run this inside a Unity project, or pass --project-path")
	}
	return p
}

// runEditorWait is `utk editor wait [--timeout S]`. Exit 124 marks a timeout,
// like coreutils timeout and the queue.
func runEditorWait(args []string, stdout, stderr io.Writer) int {
	// Outside a project the CLI picks the one reachable Editor, as for any verb.
	project := findFlag(args, "--project-path")
	if project == "" {
		project = localProjectRoot()
	}
	budget := 300
	if v := findFlag(args, "--timeout"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			fmt.Fprintf(stderr, "utk: editor wait --timeout must be a positive number of seconds, got %q\n", v)
			return 2
		}
		budget = n
	}
	started := time.Now()
	why, ok := waitEditorReady(project, time.Duration(budget)*time.Second, stderr)
	if !ok {
		fmt.Fprintf(stderr, "utk: TIMEOUT: the Editor is not ready after %ds (%s)\n", budget, why)
		return 124
	}
	fmt.Fprintf(stdout, "ready (waited %ds)\n", int(time.Since(started).Seconds()))
	return 0
}

// runEditorRestart is `utk editor restart [--force]`.
func runEditorRestart(args []string, stderr io.Writer) int {
	project := editorProject(args, stderr)
	if project == "" {
		return 2
	}
	return restartEditor(project, hasFlag(args, "--force"), stderr)
}
