package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The Editor in Play with nobody holding the Editor lock and no queue job running
// (2026-10-06: Play pressed in the GUI over a remote desktop; the next job failed with
// "This cannot be used during play mode" and nobody knew for 8 minutes). A Play a
// utk job or lock holder did not start is "unowned": it is named, a job that meets it
// ends BLOCKED, and it is never stopped from here.

// exitBlocked is the exit code of a job that ended BLOCKED (PLAY-UNOWNED). Unused by
// utk elsewhere (0 ok, 1 fail, 2 usage, 124 timeout, 130 cancelled); EX_TEMPFAIL.
const exitBlocked = 75

var (
	playUnownedGrace = 30 * time.Second // unowned this long before it is reported
	playBlockWait    = 5 * time.Minute  // a queued job waits this long for Play to end, then BLOCKED
)

func editorPlaying(cwd string) bool {
	out, code := queueExec(cwd, 30*time.Second, nil, utkSelf(), "editor", "status")
	return code == 0 && strings.Contains(out, `"playMode":"playing"`)
}

// playOwned: a live lock holder or a running queue job owns whatever Play there is.
func playOwned(dir string) bool {
	if owner, pid := lockHolder(); owner != "" && (pid == 0 || pidAlive(pid)) {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, "running"))
	return err == nil
}

// playUnowned returns the PLAY-UNOWNED notice once Play has been unowned for
// playUnownedGrace, else "". The first sight is kept in <dir>/play-unowned (the time
// the line says "since"); an owned or absent Play clears it. pending says "seen, not
// yet confirmed" for `queue status`.
func playUnowned(dir, cwd string, now time.Time) (notice string, pending bool) {
	f := filepath.Join(dir, "play-unowned")
	if playOwned(dir) || !editorPlaying(cwd) { // the cheap local check first: a lock holder costs no Editor call
		os.Remove(f)
		return "", false
	}
	since := now
	if b, err := os.ReadFile(f); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b))); err == nil {
			since = t
		}
	} else {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(f, []byte(now.Format(time.RFC3339)+"\n"), 0o644)
	}
	if now.Sub(since) < playUnownedGrace {
		return "", true
	}
	return fmt.Sprintf("PLAY-UNOWNED since %s local (not a utk job: GUI, remote desktop or a direct call) — ask producer/user, never stop it yourself",
		since.Local().Format("15:04:05")), false
}

// holdsEditorLock: the caller is the lock holder (UNITY_LOCK_OWNER names it) or runs inside a queue job.
func holdsEditorLock() bool {
	if os.Getenv("UNITY_QUEUE_REQUEST") != "" {
		return true
	}
	me := os.Getenv("UNITY_LOCK_OWNER")
	owner, _ := lockHolder()
	return me != "" && owner == me
}

// guardEditorPlay is `utk editor play|stop`'s check, before anything reaches Unity.
// play without the lock is refused unless --force (user / manual only); stop is
// unchanged but warns when it ends a Play nobody owns. Returns args without --force
// and the exit code (-1 = go on).
func guardEditorPlay(sub string, args []string, stderr io.Writer) ([]string, int) {
	force := false
	var rest []string
	for _, a := range args {
		if a == "--force" {
			force = true
			continue
		}
		rest = append(rest, a)
	}
	switch {
	case sub == "play" && !force && !holdsEditorLock():
		fmt.Fprintln(stderr, "utk: `editor play` needs the Editor lock: run it inside a queue job (`utk queue submit shot|scene …`), under unity-job.sh, or with UNITY_LOCK_OWNER = the lock holder. `--force` is for the user / manual use only: a Play outside the lock breaks every job queued behind it.")
		return nil, 3
	case sub == "stop" && !holdsEditorLock() && editorPlaying(localProjectRoot()) && !playOwned(queueDir()):
		fmt.Fprintln(stderr, "utk: warning: stopping a Play nobody owns (no lock, no job) and you hold no lock; if it is the user's or another role's session, leave it and tell the producer")
	}
	return rest, -1
}

func reqCwd(reqs []queueRequest) string {
	if len(reqs) > 0 {
		return reqs[0].Cwd
	}
	return localProjectRoot()
}

func playSince(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "play-unowned"))
	if err != nil {
		return "now"
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil {
		return "now"
	}
	return t.Local().Format("15:04:05")
}

// blockedByPlay ends the requests that waited out their budget (the submitter's --wait, at most
// playBlockWait) with BLOCKED; the others stay queued.
func blockedByPlay(reqs []queueRequest, notice string, results *map[string]queueResult, now time.Time) {
	out := map[string]queueResult{}
	for _, r := range reqs {
		budget := playBlockWait
		if w := time.Duration(r.WaitS) * time.Second; w > 0 && w-5*time.Second < budget {
			budget = w - 5*time.Second
		}
		if now.Sub(r.Submitted) >= budget {
			out[r.ID] = queueResult{ID: r.ID, Status: "BLOCKED", Exit: exitBlocked,
				Stdout: "RESULT: BLOCKED (" + notice + ")\nnothing was run; the Editor is in a Play no utk job owns. Never stop it yourself: ask producer/user, then resubmit.\n",
				WaitS:  int(now.Sub(r.Submitted).Seconds())}
		}
	}
	*results = out
}
