package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// mainThreadTimeout reports whether msg is the Editor failing to run a request
// on its main thread. An Editor stuck this way still answers `utk status`
// (the pipeline server's socket is served off the main thread), so the timeout
// is the only symptom, and on its own it reads as "Unity is busy compiling" —
// which is exactly what it is not.
func mainThreadTimeout(msg string) bool {
	return strings.Contains(msg, "Main thread operation timed out") ||
		(strings.Contains(msg, "Pipeline command") && strings.Contains(msg, "timed out"))
}

// dialog is one modal window open on an Editor.
type dialog struct {
	pid, index string // how to address the window again, to click it
	text       string
	buttons    []string
}

// knownDialogs are the modals utk may answer by itself, and only those: a
// dialog that is not on this list is reported, never clicked.
//
// Deliberately NOT here, because the answer changes files the caller did not
// ask utk to change: the API updater ("I Made a Backup. Go Ahead!") rewrites
// .cs sources.
//
// retry says whether the request the dialog ate is worth sending again. It is
// not after a Cancel: whatever raised the dialog would raise it again.
type knownDialog struct {
	match, button, why string
	retry              bool
	next               string // what the caller has to do itself, if anything
}

var knownDialogs = []knownDialog{
	{
		match:  "have been modified externally",
		button: "Reload",
		retry:  true,
		// The on-disk version is the one somebody just asked for — a git pull
		// or a deliberate text edit — and it is what every later command will
		// read. Ignore keeps the Editor's stale copy and silently writes it
		// back over that change at the next save. Reload costs unsaved
		// in-Editor edits, which an agent-driven session rarely has.
		why: "the on-disk scene is the change that was just made; Ignore would overwrite it at the next save",
	},
	{
		// "Scene(s) Have Been Modified — Do you want to save the changes you
		// made in the scenes: … Save / Don't Save / Cancel". Raised when
		// something closes a dirty scene the interactive way (a File/ menu
		// item, SaveCurrentModifiedScenesIfUserWantsTo, quitting). Save writes
		// the Editor's copy over whatever is on disk, possibly a pull; Don't
		// Save drops the work. Which is right depends on whose edits those
		// are, and only the caller knows that. Cancel writes nothing, only
		// aborts the action that asked, and hands the choice back.
		match:  "want to save the changes you made in the scene",
		button: "Cancel",
		why:    "Save could overwrite the file on disk and Don't Save drops unsaved work; Cancel changes nothing",
		next: "the scene is still dirty and the action that raised the prompt did not happen. Decide explicitly: " +
			"EditorSceneManager.SaveScene(scene) if the edits are yours, ask the user if they are not, then redo the action",
	},
}

// autoAnswerOff lets a human keep the Editor's dialogs to themselves.
const autoAnswerOff = "UTK_NO_AUTO_DIALOG"

// listScript emits one line per modal dialog: pid, window index, text, buttons.
const listScript = `tell application "System Events"
	set out to {}
	repeat with p in (every process whose name is "Unity")
		set n to 0
		repeat with w in (every window of p)
			set n to n + 1
			if subrole of w is "AXDialog" then
				set AppleScript's text item delimiters to " "
				set txt to ((value of every static text of w) as text)
				set AppleScript's text item delimiters to "|"
				set btn to ((name of every button of w) as text)
				set end of out to ((unix id of p) as text) & tab & (n as text) & tab & txt & tab & btn
			end if
		end repeat
	end repeat
	set AppleScript's text item delimiters to linefeed
	return out as text
end tell`

// modalDialogs returns every modal dialog blocking an Editor.
//
// A dialog is the usual reason a main-thread call times out: Unity shows it
// from the main thread and pumps its own event loop until it is answered, so
// every queued request expires while the process sits at ~0% CPU. "The open
// scene(s) have been modified externally", raised when a git pull or a text
// edit rewrites the scene that is currently open, is the one that keeps
// costing sessions.
//
// macOS only, and the query needs Accessibility permission for the terminal
// running utk (there is no permission-free way to read another app's window
// text: the Quartz window list wants Screen Recording instead). Anywhere else,
// or unpermitted, it returns nothing and the caller falls back to reporting
// the bare timeout — no worse than before, and never a wrong answer.
func modalDialogs() []dialog {
	if runtime.GOOS != "darwin" {
		return nil
	}
	out, err := osascript(listScript)
	if err != nil {
		return nil
	}
	var ds []dialog
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), "\t")
		if len(f) != 4 {
			continue
		}
		ds = append(ds, dialog{pid: f[0], index: f[1], text: f[2], buttons: strings.Split(f[3], "|")})
	}
	return ds
}

// answer clicks one of the dialog's buttons.
func (d dialog) answer(button string) error {
	_, err := osascript(fmt.Sprintf(
		`tell application "System Events" to tell (first process whose unix id is %s) to click button %q of window %s`,
		d.pid, button, d.index))
	return err
}

// known finds the entry authorising an automatic answer, if there is one. A
// button utk does not actually see on the dialog is not clicked: the wording
// varies by Editor version, and clicking blind could hit the wrong option.
func (d dialog) known() (knownDialog, bool) {
	for _, k := range knownDialogs {
		if !strings.Contains(d.text, k.match) {
			continue
		}
		for _, b := range d.buttons {
			if b == k.button {
				return k, true
			}
		}
	}
	return knownDialog{}, false
}

// answerModal deals with whatever is blocking the Editor's main thread: it
// clicks the dialogs it knows the right answer to and reports the rest for a
// human to answer. It returns true when it clicked something worth retrying
// the request the dialog ate for.
func answerModal(stderr io.Writer) bool {
	dialogs := modalDialogs()
	if len(dialogs) == 0 {
		return false
	}
	retry := false
	for _, d := range dialogs {
		k, ok := d.known()
		if ok && os.Getenv(autoAnswerOff) == "" {
			if err := d.answer(k.button); err != nil {
				ok = false // fall through and report it instead
			} else {
				fmt.Fprintf(stderr, "utk: auto-answered modal %q → %s (%s)\n", d.text, k.button, k.why)
				if k.next != "" {
					fmt.Fprintf(stderr, "  %s\n", k.next)
				}
				retry = retry || k.retry
				continue
			}
		}
		fmt.Fprintf(stderr, "utk: the Editor main thread is blocked by a modal dialog — answer it in the Unity window:\n  %s [buttons: %s]\n",
			d.text, strings.Join(d.buttons, ", "))
		if ok {
			fmt.Fprintf(stderr, "  (%s=1 is set, so utk left it alone; the answer it would pick is %s)\n", autoAnswerOff, k.button)
		}
	}
	return retry
}

func osascript(script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	return string(out), err
}
