package official

import (
	"bytes"
	"io"
	"testing"
)

func TestCaptureUsesExecUnity(t *testing.T) {
	old := execUnity
	defer func() { execUnity = old }()
	execUnity = func(args []string, stdout, stderr io.Writer) int {
		stdout.Write([]byte(`{"ok":1}`))
		return 6
	}
	out, code := Capture([]string{"command", "x"}, io.Discard)
	if code != 6 || !bytes.Equal(out, []byte(`{"ok":1}`)) {
		t.Fatalf("got %q code %d", out, code)
	}
}

func TestBinPrefersEnv(t *testing.T) {
	t.Setenv("UTK_UNITY_BIN", "/tmp/fake-unity")
	p, err := Bin()
	if err != nil || p != "/tmp/fake-unity" {
		t.Fatalf("got %q, %v", p, err)
	}
}

// fakeCLI answers `pipeline list` with editorUp, and every other argv from
// replies (one per attempt, the last repeating). A reply is an error message,
// delivered the way the CLI actually delivers it: inside the envelope on
// stdout. "" means success.
func fakeCLI(t *testing.T, editorUp bool, replies ...string) *int {
	t.Helper()
	calls := 0
	old := execUnity
	t.Cleanup(func() { execUnity = old })
	execUnity = func(args []string, stdout, stderr io.Writer) int {
		if args[0] == "pipeline" {
			running := "false"
			if editorUp {
				running = "true"
			}
			stdout.Write([]byte(`{"success":true,"data":{"instances":[{"isRunning":` + running + `}]}}`))
			return 0
		}
		calls++
		msg := replies[min(calls, len(replies))-1]
		if msg == "" {
			stdout.Write([]byte(`{"success":true,"data":{"result":{}}}`))
			return 0
		}
		stdout.Write([]byte(`{"success":false,"errors":[{"code":"COMMAND_FAILED","message":"` + msg + `"}]}`))
		return 6
	}
	return &calls
}

const reloadErr = `Failed to execute command 'x': Connection reset by server`

func TestCaptureRetry(t *testing.T) {
	t.Run("success is not retried", func(t *testing.T) {
		calls := fakeCLI(t, true, "")
		if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 0 {
			t.Fatalf("code = %d", code)
		}
		if *calls != 1 {
			t.Fatalf("calls = %d, want 1", *calls)
		}
	})

	t.Run("reload error retries until it clears", func(t *testing.T) {
		calls := fakeCLI(t, true, reloadErr, reloadErr, "")
		out, code := CaptureRetry([]string{"command", "x"}, io.Discard)
		if code != 0 {
			t.Fatalf("code = %d out = %s, want 0", code, out)
		}
		if *calls != 3 {
			t.Fatalf("calls = %d, want 3", *calls)
		}
	})

	// utk always passes --project-path, and then a reload surfaces as a refused
	// connect to the project's port — the exit 6 a post-import refresh hit.
	t.Run("refused connect during reload retries", func(t *testing.T) {
		calls := fakeCLI(t, true, "Cannot connect to Unity Editor Pipeline server at 127.0.0.1:7800.", "")
		if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 0 {
			t.Fatalf("code = %d, want 0", code)
		}
		if *calls != 2 {
			t.Fatalf("calls = %d, want 2", *calls)
		}
	})

	// The first call after `editor play`/`refresh` lands while the reloading
	// domain drops the socket; the CLI reports that as an HTTP send failure.
	t.Run("network error during reload retries", func(t *testing.T) {
		calls := fakeCLI(t, true, "Failed to execute command 'set_autotick': Network error: An error occurred while sending the request.", "")
		if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 0 {
			t.Fatalf("code = %d, want 0", code)
		}
		if *calls != 2 {
			t.Fatalf("calls = %d, want 2", *calls)
		}
	})

	t.Run("no editor gives up at once", func(t *testing.T) {
		calls := fakeCLI(t, false, reloadErr)
		out, code := CaptureRetry([]string{"command", "x"}, io.Discard)
		if code != 6 {
			t.Fatalf("code = %d, want 6", code)
		}
		// Waiting out the retry budget here would only make "Unity is not running" slow to say.
		if *calls != 1 {
			t.Fatalf("calls = %d, want 1", *calls)
		}
		if !bytes.Contains(out, []byte("Connection reset")) {
			t.Fatalf("out = %s, want the envelope forwarded", out)
		}
	})

	t.Run("other failures are not retried", func(t *testing.T) {
		calls := fakeCLI(t, true, "path is required")
		if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 6 {
			t.Fatalf("code = %d, want 6", code)
		}
		if *calls != 1 {
			t.Fatalf("calls = %d, want 1", *calls)
		}
	})

	// The unfiltered path has no envelope, so the same signature has to be
	// recognised on stderr as well.
	t.Run("reload error on stderr also retries", func(t *testing.T) {
		calls := 0
		old := execUnity
		t.Cleanup(func() { execUnity = old })
		execUnity = func(args []string, stdout, stderr io.Writer) int {
			if args[0] == "pipeline" {
				stdout.Write([]byte(`{"success":true,"data":{"instances":[{"isRunning":true}]}}`))
				return 0
			}
			calls++
			if calls == 1 {
				io.WriteString(stderr, "utk: "+reloadErr)
				return 6
			}
			return 0
		}
		if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 0 {
			t.Fatalf("code = %d, want 0", code)
		}
		if calls != 2 {
			t.Fatalf("calls = %d, want 2", calls)
		}
	})
}

// During `utk editor restart` no Editor process exists for a minute. A call
// landing there must wait for the new one instead of failing with "not running".
func TestCaptureRetry_WaitsOutARestart(t *testing.T) {
	calls := fakeCLI(t, false, "Cannot connect to Unity Editor Pipeline server at 127.0.0.1:7800", "Cannot connect to Unity Editor Pipeline server at 127.0.0.1:7800", "")
	old := Restarting
	t.Cleanup(func() { Restarting = old })
	Restarting = func() bool { return *calls < 3 }
	if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 0 {
		t.Fatalf("code = %d, want the call to succeed once the Editor is back", code)
	}
	if *calls != 3 {
		t.Fatalf("calls = %d, want 3", *calls)
	}
}

// While the Editor process is gone the CLI stops saying "cannot connect" and
// says it knows no such Editor (seen live, 6 s into `utk editor restart`).
// That is only worth waiting out during a restart: otherwise it is the truth.
func TestCaptureRetry_NoInstanceIsRetriedOnlyDuringARestart(t *testing.T) {
	const gone = "No Pipeline instance found for project: /p. Make sure Unity Editor is running with the Pipeline package installed."
	old := Restarting
	t.Cleanup(func() { Restarting = old })

	calls := fakeCLI(t, false, gone, gone, "")
	Restarting = func() bool { return *calls < 3 }
	if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code != 0 || *calls != 3 {
		t.Fatalf("during a restart: code %d after %d calls, want success on the 3rd", code, *calls)
	}

	calls = fakeCLI(t, false, gone, "")
	Restarting = func() bool { return false }
	if _, code := CaptureRetry([]string{"command", "x"}, io.Discard); code == 0 || *calls != 1 {
		t.Fatalf("no restart: code %d after %d calls, want the failure at once", code, *calls)
	}
}
