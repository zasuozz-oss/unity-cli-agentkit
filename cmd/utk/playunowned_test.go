package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const playingJSON = `{"playMode":"playing","status":"playing"}`

// (a) PLAY-UNOWNED shows after 30 s of Play with no lock holder and no job, never during a job's own Play.
func TestPlayUnownedIsReportedAfterThirtySeconds(t *testing.T) {
	dir, locks := t.TempDir(), t.TempDir()
	t.Setenv("UNITY_LOCK_DIR", locks)
	fakeExec(t, fakeAnswers{"utk editor status": {playingJSON, 0}})
	t0 := time.Date(2026, 10, 6, 20, 19, 0, 0, time.Local)
	if n, pending := playUnowned(dir, "/p", t0); n != "" || !pending {
		t.Fatalf("first sight: %q pending=%v", n, pending)
	}
	if n, _ := playUnowned(dir, "/p", t0.Add(29*time.Second)); n != "" {
		t.Fatalf("reported before 30 s: %q", n)
	}
	n, _ := playUnowned(dir, "/p", t0.Add(31*time.Second))
	if !strings.Contains(n, "PLAY-UNOWNED since 20:19:00 local") || !strings.Contains(n, "never stop it yourself") {
		t.Fatalf("notice = %q", n)
	}
	// a running queue job owns the Play
	os.WriteFile(filepath.Join(dir, "running"), []byte("shot job\n"), 0o644)
	if n, _ := playUnowned(dir, "/p", t0.Add(60*time.Second)); n != "" {
		t.Fatalf("reported during a job: %q", n)
	}
	os.Remove(filepath.Join(dir, "running"))
	// and so does a live lock holder (a unity-job shot)
	os.MkdirAll(filepath.Join(locks, "unity-editor.lock.d"), 0o755)
	os.WriteFile(filepath.Join(locks, "unity-editor.lock.d", "owner"), []byte("senior 1\n"), 0o644)
	os.WriteFile(filepath.Join(locks, "unity-editor.lock.d", "pid"), []byte(strconvItoa(os.Getpid())), 0o644)
	if n, _ := playUnowned(dir, "/p", t0.Add(90*time.Second)); n != "" {
		t.Fatalf("reported under a lock holder: %q", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "play-unowned")); err == nil {
		t.Error("first-sight file not cleared once the Play is owned")
	}
}

// (b) a job submitted during unowned Play does not run; it ends BLOCKED with exit 75 once its wait budget is spent.
func TestJobDuringUnownedPlayEndsBlocked(t *testing.T) {
	dir, locks := t.TempDir(), t.TempDir()
	t.Setenv("UNITY_LOCK_DIR", locks)
	t.Setenv("UTK_TELEMETRY", dir+"/t.jsonl")
	calls := fakeExec(t, fakeAnswers{
		"utk status":        {"ok\n", 0},
		"utk editor status": {playingJSON, 0},
	})
	old := playUnownedGrace
	playUnownedGrace = 0
	t.Cleanup(func() { playUnownedGrace = old })
	writeRequest(dir, queueRequest{ID: "j1", Kind: "compile", Agent: "senior", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now(), WaitS: 3600})
	if n := serveOnce(dir, io.Discard); n != 0 {
		t.Fatalf("answered %d before the budget ran out", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", "j1.json")); err != nil {
		t.Fatal("the request must stay queued while Play ends")
	}
	writeRequest(dir, queueRequest{ID: "j2", Kind: "compile", Agent: "senior", PID: os.Getpid(), Cwd: "/p", Submitted: time.Now().Add(-playBlockWait - time.Second), WaitS: 3600})
	if n := serveOnce(dir, io.Discard); n != 1 {
		t.Fatalf("answered %d, want 1 (j2)", n)
	}
	res, ok := readResult(dir, "j2")
	if !ok || res.Status != "BLOCKED" || res.Exit != exitBlocked || !strings.Contains(res.Stdout, "RESULT: BLOCKED (PLAY-UNOWNED since") {
		t.Fatalf("j2 = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", "j1.json")); err != nil {
		t.Error("j1 (still inside its budget) was dropped")
	}
	for _, c := range *calls {
		if strings.HasPrefix(c.name, "unity-lock") || strings.Contains(strings.Join(c.args, " "), "play") && c.args[0] != "editor" {
			t.Fatalf("a command ran under unowned Play: %v", c)
		}
		if len(c.args) > 0 && ((c.args[0] == "exec" && !strings.Contains(strings.Join(c.args, " "), "alive")) || c.name == "unity-test.sh") { // the main-thread probe is not the job
			t.Fatalf("the job's work ran: %v", c)
		}
	}
	if exitBlocked == 0 || exitBlocked == 1 || exitBlocked == 2 || exitBlocked == 124 || exitBlocked == 130 {
		t.Fatal("exitBlocked must be distinct")
	}
}

// (c) `utk editor play` needs the lock (or a queue job); --force passes; stop only warns.
func TestEditorPlayNeedsTheLock(t *testing.T) {
	locks := t.TempDir()
	t.Setenv("UNITY_LOCK_DIR", locks)
	t.Setenv("UNITY_LOCK_OWNER", "")
	t.Setenv("UNITY_QUEUE_REQUEST", "")
	fakeExec(t, fakeAnswers{"utk editor status": {playingJSON, 0}})
	var stderr bytes.Buffer
	if _, code := guardEditorPlay("play", []string{"play"}, &stderr); code != 3 || !strings.Contains(stderr.String(), "needs the Editor lock") {
		t.Fatalf("no lock: code=%d %q", code, stderr.String())
	}
	if rest, code := guardEditorPlay("play", []string{"play", "--force"}, io.Discard); code != -1 || strings.Join(rest, " ") != "play" {
		t.Fatalf("--force: code=%d rest=%v (the flag must not reach Unity)", code, rest)
	}
	t.Setenv("UNITY_QUEUE_REQUEST", "r1")
	if _, code := guardEditorPlay("play", []string{"play"}, io.Discard); code != -1 {
		t.Fatalf("inside a queue job: code=%d", code)
	}
	t.Setenv("UNITY_QUEUE_REQUEST", "")
	t.Setenv("UNITY_LOCK_OWNER", "ui-dev")
	os.MkdirAll(filepath.Join(locks, "unity-editor.lock.d"), 0o755)
	os.WriteFile(filepath.Join(locks, "unity-editor.lock.d", "owner"), []byte("ui-dev 1\n"), 0o644)
	if _, code := guardEditorPlay("play", []string{"play"}, io.Discard); code != -1 {
		t.Fatalf("lock holder: code=%d", code)
	}
	// stop under unowned Play without the lock: proceeds, with a warning
	t.Setenv("UNITY_LOCK_OWNER", "other")
	os.RemoveAll(filepath.Join(locks, "unity-editor.lock.d"))
	stderr.Reset()
	if _, code := guardEditorPlay("stop", []string{"stop"}, &stderr); code != -1 || !strings.Contains(stderr.String(), "warning: stopping a Play nobody owns") {
		t.Fatalf("stop: code=%d %q", code, stderr.String())
	}
}
