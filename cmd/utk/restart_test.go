package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeEditor swaps the Editor process for a recorder: every step restartEditor
// takes lands in the returned log, in order.
func fakeEditor(t *testing.T, state string, reachable bool, diesOnQuit bool) *[]string {
	t.Helper()
	var steps []string
	alive := true
	old, oldWait, oldStep := editor, restartQuitWait, restartStep
	restartQuitWait, restartStep = 50*time.Millisecond, time.Millisecond
	t.Setenv("HOME", t.TempDir()) // the marker lives under the kit home
	editor = editorOps{
		find:  func(project string) (int, string) { return 4242, "/Apps/Unity" },
		state: func(project string) (string, bool) { return state, reachable },
		quit: func(project string) {
			steps = append(steps, "quit")
			if diesOnQuit {
				alive = false
			}
		},
		alive: func(pid int) bool { return alive },
		kill:  func(pid int) { steps = append(steps, "kill"); alive = false },
		start: func(bin, project string) error {
			if restartInProgress() {
				steps = append(steps, "marker-on")
			}
			steps = append(steps, "start "+bin+" "+project)
			return nil
		},
		ready: func(project string, budget time.Duration, stderr io.Writer) (string, bool) {
			steps = append(steps, "ready")
			return "", true
		},
	}
	t.Cleanup(func() { editor, restartQuitWait, restartStep = old, oldWait, oldStep })
	return &steps
}

func TestRestartEditor_QuitsStartsAndWaits(t *testing.T) {
	steps := fakeEditor(t, "playing=False dirty=0 busy=False", true, true)
	var errb bytes.Buffer
	if code := restartEditor("/p", false, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if got := strings.Join(*steps, " | "); got != "quit | marker-on | start /Apps/Unity /p | ready" {
		t.Fatalf("steps: %s", got)
	}
	if restartInProgress() {
		t.Fatal("marker left behind: every later connection error would wait on it")
	}
	if !strings.Contains(errb.String(), "restarted") {
		t.Fatalf("stderr: %s", errb.String())
	}
}

// Restarting drops whatever the Editor holds only in memory, so a restart
// nobody forced must never cost somebody's Play session or unsaved scene.
func TestRestartEditor_RefusesWhenWorkWouldBeLost(t *testing.T) {
	for state, want := range map[string]string{
		"playing=True dirty=0 busy=False":  "Play",
		"playing=False dirty=2 busy=False": "unsaved",
		"playing=False dirty=0 busy=True":  "compiling",
	} {
		steps := fakeEditor(t, state, true, true)
		var errb bytes.Buffer
		if code := restartEditor("/p", false, &errb); code != 1 {
			t.Errorf("%s: exit %d, want 1", state, code)
		}
		if len(*steps) != 0 {
			t.Errorf("%s: touched the Editor anyway: %v", state, *steps)
		}
		if !strings.Contains(errb.String(), want) {
			t.Errorf("%s: stderr %q does not say %q", state, errb.String(), want)
		}
	}
}

// An Editor that cannot say whether it holds unsaved work is not restarted
// on a guess; --force is the caller taking that decision.
func TestRestartEditor_HungEditorNeedsForce(t *testing.T) {
	steps := fakeEditor(t, "", false, false)
	var errb bytes.Buffer
	if code := restartEditor("/p", false, &errb); code != 1 || len(*steps) != 0 {
		t.Fatalf("exit %d steps %v", code, *steps)
	}
	if !strings.Contains(errb.String(), "--force") {
		t.Fatalf("stderr: %s", errb.String())
	}
	if code := restartEditor("/p", true, &errb); code != 0 {
		t.Fatalf("--force exit %d: %s", code, errb.String())
	}
	if got := strings.Join(*steps, " | "); got != "quit | kill | marker-on | start /Apps/Unity /p | ready" {
		t.Fatalf("steps: %s", got)
	}
}

func TestRestartEditor_NoEditorIsAnError(t *testing.T) {
	fakeEditor(t, "", false, false)
	editor.find = func(string) (int, string) { return 0, "" }
	var errb bytes.Buffer
	if code := restartEditor("/p", false, &errb); code != 1 || !strings.Contains(errb.String(), "no running Editor") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
}

// A marker whose restart died must not make every connection error wait forever.
func TestRestartInProgress_StaleMarkerIsIgnored(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if restartInProgress() {
		t.Fatal("no marker yet")
	}
	os.MkdirAll(kitHome(), 0o755)
	os.WriteFile(restartMarker(), []byte("/p"), 0o644)
	if !restartInProgress() {
		t.Fatal("fresh marker not seen")
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(restartMarker(), old, old)
	if restartInProgress() {
		t.Fatal("hour-old marker still counts")
	}
}

func TestWaitEditorReady_WaitsOutACompile(t *testing.T) {
	busy := `{"success":true,"data":{"result":{"status":"compiling","compiling":true,"domainReloadInProgress":false}}}`
	ready := `{"success":true,"data":{"result":{"status":"ready","compiling":false,"domainReloadInProgress":false}}}`
	bin, _ := fakeUnity(t, busy, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	fakeUnityNext(t, ready)
	old := waitStep
	waitStep = time.Millisecond
	t.Cleanup(func() { waitStep = old })
	if why, ok := waitEditorReady("/p", 20*time.Second, io.Discard); !ok {
		t.Fatalf("not ready: %s", why)
	}
}

func TestRun_EditorWaitTimesOutWith124AndTheReason(t *testing.T) {
	busy := `{"success":true,"data":{"result":{"status":"compiling","compiling":true,"domainReloadInProgress":false}}}`
	bin, _ := fakeUnity(t, busy, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	old := waitStep
	waitStep = time.Millisecond
	t.Cleanup(func() { waitStep = old })
	var out, errb bytes.Buffer
	code := run([]string{"editor", "wait", "--timeout", "1"}, &out, &errb)
	if code != 124 || !strings.Contains(errb.String(), "compiling") {
		t.Fatalf("exit %d stderr %q", code, errb.String())
	}
}

func TestRun_EditorGcSendsTheCleanupWithABudget(t *testing.T) {
	bin, argvFile := fakeUnity(t, `{"success":true,"data":{"result":"destroyed 25 leaked OS fallback fonts"}}`, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	var out, errb bytes.Buffer
	if code := run([]string{"editor", "gc"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	argv := readArgv(t, argvFile)
	// Without its own budget the snippet dies at the eval tool's 5 s default,
	// exactly when there are thousands of fonts to destroy.
	if !strings.Contains(argv, "command eval") || !strings.Contains(argv, "fallbackOSFontAssets") || !strings.Contains(argv, "-- --timeout 60000") {
		t.Fatalf("argv: %s", argv)
	}
	if !strings.Contains(out.String(), "destroyed 25") {
		t.Fatalf("stdout: %s", out.String())
	}
}

func TestRun_EditorStatusNamesADegradedEditor(t *testing.T) {
	bin, _ := fakeUnity(t, `{"success":true,"data":{"result":{"status":"ready","playMode":"stopped"}}}`, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	old := editorDegraded
	t.Cleanup(func() { editorDegraded = old })
	for _, bad := range []bool{true, false} {
		editorDegraded = func(string) (string, bool) { return "the Editor has degraded: a domain reload now takes 34.0s", bad }
		var out, errb bytes.Buffer
		if code := run([]string{"editor", "status"}, &out, &errb); code != 0 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
		if got := strings.Contains(errb.String(), "utk editor restart"); got != bad {
			t.Errorf("degraded=%v: stderr %q", bad, errb.String())
		}
		if !strings.Contains(out.String(), `"playMode":"stopped"`) {
			t.Errorf("status itself changed: %q", out.String())
		}
	}
}

// A hung call must end at the caller's deadline with coreutils' exit code,
// and take its `unity` child with it: agents wrapped every utk call in
// `perl -e 'alarm …'` because macOS has no timeout(1).
func TestRun_MaxTimeEndsAHungCallWith124(t *testing.T) {
	bin, _ := fakeUnity(t, `{"success":true,"data":{"result":{}}}`, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	t.Setenv("UTK_FAKE_UNITY_SLEEP_MS", "20000")
	got := make(chan int, 1)
	old := exit
	exit = func(code int) { got <- code }
	t.Cleanup(func() { exit = old })
	started := time.Now()
	done := make(chan struct{})
	go func() { run([]string{"--max-time", "1", "exec", "return 1;"}, io.Discard, io.Discard); close(done) }()
	select {
	case code := <-got:
		if code != 124 {
			t.Fatalf("exit %d, want 124", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("--max-time 1 did not fire")
	}
	select {
	case <-done: // the child was killed, so the call itself returned
	case <-time.After(5 * time.Second):
		t.Fatal("the unity child outlived the deadline")
	}
	if time.Since(started) > 8*time.Second {
		t.Fatalf("took %s", time.Since(started))
	}
}
