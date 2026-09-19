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

	t.Run("no editor gives up at once", func(t *testing.T) {
		calls := fakeCLI(t, false, reloadErr)
		out, code := CaptureRetry([]string{"command", "x"}, io.Discard)
		if code != 6 {
			t.Fatalf("code = %d, want 6", code)
		}
		// Waiting 8s here would only make "Unity is not running" slow to say.
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
