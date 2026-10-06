package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// An EditMode test, an exec snippet and a modal dialog all run on (or block)
// the Editor's main thread. /api/status answers from the HTTP thread whatever
// the main thread does, so `utk status` said "alive" through every hang. These
// helpers ask the main thread itself, stop what a dead job left running, and
// restart an Editor that stays silent (the 5-minute Hung Editor rule).

var (
	probeTimeout      = 20 * time.Second
	freeWait          = 60 * time.Second
	watchdogHang      = 5 * time.Minute
	watchdogGiveUp    = 30 * time.Minute
	watchdogTickEvery = 30 * time.Second
)

const probeSnippet = `return "alive";`

// probeMainThread is "ok" when the main thread ran a trivial snippet, "hung"
// when it did not answer in time, "down" for anything else (no Editor, a
// refused connection, a compile error): a restart does not fix those.
func probeMainThread(cwd string) string {
	out, code := queueExec(cwd, probeTimeout+15*time.Second, nil, utkSelf(), "exec", probeSnippet,
		"--timeout", strconv.Itoa(int(probeTimeout/time.Millisecond)))
	switch {
	case code == 0:
		return "ok"
	case code == 124, strings.Contains(out, "timed out"):
		return "hung"
	}
	return "down"
}

// modalMarkers are the frames a main thread shows while a native modal sits on it
// (an NSAlert such as EditorSceneManager::HandleOpenScenesChangeOnDisk, a runModal
// loop, Unity's own DisplayDialog). A hung Editor in a modal must be answered, not restarted.
var modalMarkers = regexp.MustCompile(`HandleOpenScenesChangeOnDisk|runModal|NSAlert|beginModalSession|DisplayDialog`)

// dialogCmd is $UTK_DIALOG_CMD, else the one line of ~/.unity-cli-agentkit/dialog-cmd: the coordinator is spawned by
// whichever tab submits first, so its environment is not something a role can count on.
func dialogCmd() string {
	if c := os.Getenv("UTK_DIALOG_CMD"); c != "" {
		return c
	}
	b, _ := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".unity-cli-agentkit", "dialog-cmd"))
	return strings.TrimSpace(string(b))
}

// modalDialog returns "" when no Unity main thread is inside a modal, else a
// `DIALOG: ...` line. UTK_DIALOG_CMD (e.g. game-studio's tools/unity-dialog.sh) adds
// the dialog text and buttons (`list`) and, for the known scene-changed-on-disk alert,
// presses Reload (disk is git, the truth: coordination-rules "Editor dialogs").
func modalDialog(cwd, dir string) string {
	out, code := queueExec(cwd, 10*time.Second, nil, "pgrep", "-x", "Unity")
	if code != 0 {
		return ""
	}
	for _, pid := range strings.Fields(out) {
		f := filepath.Join(dir, "sample-"+pid+".txt")
		if _, c := queueExec(cwd, 30*time.Second, nil, "sample", pid, "1", "-file", f); c != 0 {
			continue
		}
		b, _ := os.ReadFile(f)
		os.Remove(f)
		m := modalMarkers.FindString(string(b))
		if strings.Contains(string(b), "HandleOpenScenesChangeOnDisk") {
			m = "HandleOpenScenesChangeOnDisk" // the specific alert wins over the generic frames above it
		}
		if m == "" {
			continue
		}
		line := "DIALOG: a native modal is up on the Editor main thread (pid " + pid + ", " + m + ")"
		if cmd := dialogCmd(); cmd != "" {
			l, _ := queueExec(cwd, 30*time.Second, nil, "bash", cmd, "list")
			line += "; " + strings.Join(strings.Fields(l), " ")
			if m == "HandleOpenScenesChangeOnDisk" {
				r, _ := queueExec(cwd, 30*time.Second, nil, "bash", cmd, "click", "Reload")
				line += "; pressed Reload: " + strings.TrimSpace(r)
			}
		}
		return line
	}
	return ""
}

func strconvItoa(n int) string { return strconv.Itoa(n) }

// freeEditor runs after a job was stopped from the client side (its timeout,
// its submitter gone): the Editor-side run it started goes on otherwise. Tests
// get cancel_tests first; a main thread still silent after freeWait means a
// single test or snippet that never yields, and only a restart ends that.
// Returns a line for the job's output.
func (c *cycle) freeEditor(cancelTests bool, why string) string {
	if cancelTests {
		queueExec(c.cwd, 45*time.Second, nil, utkSelf(), "cancel_tests")
	}
	for deadline := time.Now().Add(freeWait); ; {
		if probeMainThread(c.cwd) != "hung" {
			return "editor: freed (the Editor-side run was stopped)\n"
		}
		if time.Now().After(deadline) {
			break
		}
	}
	return "editor: " + restartHung(c.dir, c.cwd, why, c.stderr) + "\n"
}

// restartHung force-restarts a silent Editor, waits until it is ready and turns
// autotick back on. One line of telemetry either way.
func restartHung(dir, cwd, why string, stderr io.Writer) string {
	if restartInProgress() {
		return "restart already in progress elsewhere; not starting another"
	}
	fmt.Fprintf(stderr, "%s utk queue: the Editor main thread does not answer (%s); restart --force\n", stamp(), why)
	out, code := queueExec(cwd, restartReadyWait+3*time.Minute, nil, utkSelf(), "editor", "restart", "--force")
	io.WriteString(stderr, out)
	result := "RESTARTED"
	if code != 0 {
		result = "RESTART_FAILED"
	} else {
		queueExec(cwd, 6*time.Minute, nil, utkSelf(), "editor", "wait", "--timeout", "300")
		queueExec(cwd, time.Minute, nil, utkSelf(), "set_autotick", "--enable", "true")
	}
	appendTelemetry(telemetryEntry{TS: time.Now().UTC().Format(time.RFC3339), Owner: "coordinator", Task: why,
		Kind: "restart", Result: result, Via: "queue"})
	return strings.ToLower(result) + " (" + why + ")"
}

func stamp() string { return time.Now().Format("15:04:05") }

// setRunning records the job holding the Editor, so `utk queue status` can say
// "busy with X until T" instead of leaving everyone to guess it is a hang.
func (c *cycle) setRunning(r queueRequest) {
	until := time.Now().Add(jobTimeout(r))
	line := fmt.Sprintf("%s %s %s %s until %s", r.ID, r.Agent, r.Kind, describeArgs(r), until.Format("15:04:05"))
	os.WriteFile(filepath.Join(c.dir, "running"), []byte(line+"\n"), 0o644)
}

func clearRunning(dir string) { os.Remove(filepath.Join(dir, "running")) }

// hungNotice is the producer-visible line the watchdog leaves when it gives up.
func hungNotice(dir string) string {
	b, _ := os.ReadFile(filepath.Join(dir, "HUNG"))
	return strings.TrimSpace(string(b))
}

// lockHolder reads unity-lock.sh's lock: owner name and holder pid (0 when the
// lock is free or was taken by a script that does not record its pid).
func lockHolder() (owner string, pid int) {
	d := os.Getenv("UNITY_LOCK_DIR")
	if d == "" {
		d = filepath.Join(os.Getenv("HOME"), ".unity-cli-agentkit", "locks")
	}
	name := os.Getenv("UNITY_LOCK_NAME")
	if name == "" {
		name = "unity-editor"
	}
	b, err := os.ReadFile(filepath.Join(d, name+".lock.d", "owner"))
	if err != nil {
		return "", 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return "", 0
	}
	p, _ := os.ReadFile(filepath.Join(d, name+".lock.d", "pid"))
	return f[0], atoi(string(p))
}

// busyElsewhere: a live process other than this coordinator holds the Editor
// lock (a unity-job inside its own timeout). Its silence is work, not a hang;
// unity-job.sh stops its own command at its timeout.
func busyElsewhere() bool {
	owner, pid := lockHolder()
	return owner != "" && owner != "coordinator" && (pid == 0 || pidAlive(pid))
}

// watchdog applies the Hung Editor rule between cycles: no job of the queue is
// running then, so a main thread silent for watchdogHang is hung. It restarts
// once; a second hang within watchdogGiveUp of that restart is left to a human,
// with a HUNG line that `utk queue status` prints.
type watchdog struct {
	dir         string
	hungSince   time.Time
	lastTick    time.Time
	lastRestart time.Time // kept in <dir>/watchdog-restart: a respawned coordinator still knows it
	now         func() time.Time
}

func newWatchdog(dir string) *watchdog {
	w := &watchdog{dir: dir, now: time.Now}
	if b, err := os.ReadFile(filepath.Join(dir, "watchdog-restart")); err == nil {
		w.lastRestart, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	}
	return w
}

// watching: the Editor is silent and the 5 minutes run; the coordinator must
// not idle out before they end.
func (w *watchdog) watching() bool { return !w.hungSince.IsZero() }

// tick returns true when it judged the Editor hung and gave up on it.
func (w *watchdog) tick(cwd string, stderr io.Writer) bool {
	now := w.now()
	if cwd == "" || os.Getenv("UTK_NO_WATCHDOG") != "" || now.Sub(w.lastTick) < watchdogTickEvery {
		return hungNotice(w.dir) != ""
	}
	w.lastTick = now
	if busyElsewhere() {
		w.hungSince = time.Time{}
		return hungNotice(w.dir) != ""
	}
	if probeMainThread(cwd) != "hung" {
		w.hungSince = time.Time{}
		os.Remove(filepath.Join(w.dir, "HUNG")) // it answers again: whoever fixed it, the notice is stale
		return false
	}
	if d := modalDialog(cwd, w.dir); d != "" { // a dialog is not a hang: say so, never restart under it
		w.hungSince = time.Time{}
		if hungNotice(w.dir) != d {
			os.WriteFile(filepath.Join(w.dir, "HUNG"), []byte(d+"\n"), 0o644)
			fmt.Fprintf(stderr, "%s utk queue: watchdog: %s\n", stamp(), d)
		}
		return true
	}
	if w.hungSince.IsZero() {
		w.hungSince = now
		fmt.Fprintf(stderr, "%s utk queue: watchdog: the Editor main thread does not answer; restart if it stays silent for %s\n", stamp(), watchdogHang)
	}
	if now.Sub(w.hungSince) < watchdogHang {
		return false
	}
	if !w.lastRestart.IsZero() && now.Sub(w.lastRestart) < watchdogGiveUp {
		if hungNotice(w.dir) == "" {
			msg := fmt.Sprintf("%s the Editor hung again within %s of the watchdog restart at %s; not restarting. Producer: find the cause, then `utk editor restart --force`.",
				now.Format("2006-01-02 15:04"), watchdogGiveUp, w.lastRestart.Format("15:04"))
			os.WriteFile(filepath.Join(w.dir, "HUNG"), []byte(msg+"\n"), 0o644)
			fmt.Fprintln(stderr, "utk queue: watchdog:", msg)
			appendTelemetry(telemetryEntry{TS: now.UTC().Format(time.RFC3339), Owner: "coordinator", Task: "watchdog",
				Kind: "restart", Result: "GAVE_UP", Via: "queue"})
		}
		return true
	}
	if _, code := queueExec(cwd, time.Minute, nil, "bash", lockScript(), "acquire", "coordinator", "5"); code != 0 {
		return false // somebody took the Editor in between; look again next tick
	}
	restartHung(w.dir, cwd, "watchdog: silent for "+now.Sub(w.hungSince).Round(time.Second).String(), stderr)
	releaseEditorLock(cwd)
	w.lastRestart = w.now()
	os.WriteFile(filepath.Join(w.dir, "watchdog-restart"), []byte(w.lastRestart.Format(time.RFC3339)+"\n"), 0o644)
	w.hungSince = time.Time{}
	return false
}
