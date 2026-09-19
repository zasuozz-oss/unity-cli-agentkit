package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMain lets this test binary impersonate the official `unity` binary when
// the fake-unity env vars are set: it records its argv, replays a canned body
// and exits with a chosen code. Re-executing ourselves keeps the stand-in
// portable (a shebang script is not executable on Windows).
func TestMain(m *testing.M) {
	if argvFile := os.Getenv("UTK_FAKE_UNITY_ARGV"); argvFile != "" {
		// The primary body answers the first call; a test that drives one of
		// utk's own follow-up calls (the console fetch behind an exec, the
		// overlay probe behind a screenshot) sets a second one for the rest.
		bodyVar := "UTK_FAKE_UNITY_BODY"
		if _, err := os.Stat(argvFile); err == nil && os.Getenv("UTK_FAKE_UNITY_BODY2") != "" {
			bodyVar = "UTK_FAKE_UNITY_BODY2"
		}
		os.WriteFile(argvFile, []byte(strings.Join(os.Args[1:], " ")), 0o644)
		body, _ := os.ReadFile(os.Getenv(bodyVar))
		os.Stdout.Write(body)
		code, _ := strconv.Atoi(os.Getenv("UTK_FAKE_UNITY_EXIT"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// fakeUnity points UTK_UNITY_BIN at this test binary running in the
// impersonation mode above, so official.Bin() resolves to it instead of a real
// install. Returns the argv file the fake writes for assertions.
func fakeUnity(t *testing.T, body string, exitCode int) (binPath, argvFile string) {
	t.Helper()
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "body")
	if err := os.WriteFile(bodyFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	argvFile = filepath.Join(dir, "argv")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("UTK_FAKE_UNITY_ARGV", argvFile)
	t.Setenv("UTK_FAKE_UNITY_BODY", bodyFile)
	t.Setenv("UTK_FAKE_UNITY_EXIT", strconv.Itoa(exitCode))
	return self, argvFile
}

// fakeUnityNext gives the fake a second body to serve from the second call on,
// so a test can drive utk's own follow-up calls.
func fakeUnityNext(t *testing.T, body string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "body2")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UTK_FAKE_UNITY_BODY2", f)
}

func readArgv(t *testing.T, argvFile string) string {
	t.Helper()
	b, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "internal", "official", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRun_MissingCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "missing command") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

// `unity list` has no per-tool mode, so the only form that can honour a tool
// name is the filtered one with exactly one name. The rejected forms must say
// so and never reach the official CLI — silently listing all 140 tools is the
// opposite of what was asked.
func TestRun_ListToolArgument(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr string
		wantCalled bool
	}{
		{"one tool name renders the detail view", []string{"list", "eval"}, 0, "", true},
		{"two tool names are refused", []string{"list", "eval", "recompile"}, 2, "at most one tool name", false},
		{"a tool name with --raw is refused", []string{"list", "eval", "--raw"}, 2, "no per-tool mode", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bin, argvFile := fakeUnity(t, readFixture(t, "list_full.json"), 0)
			t.Setenv("UTK_UNITY_BIN", bin)

			var stdout, stderr bytes.Buffer
			code := run(c.args, &stdout, &stderr)
			if code != c.wantExit {
				t.Fatalf("exit = %d, want %d (stderr: %s)", code, c.wantExit, stderr.String())
			}
			if c.wantStderr != "" && !strings.Contains(stderr.String(), c.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), c.wantStderr)
			}
			if _, err := os.Stat(argvFile); (err == nil) != c.wantCalled {
				t.Fatalf("official CLI called = %v, want %v", err == nil, c.wantCalled)
			}
			if c.wantCalled && !strings.HasPrefix(stdout.String(), "eval (") {
				t.Fatalf("want the detail view for one tool, got %.60q", stdout.String())
			}
		})
	}
}

func TestRun(t *testing.T) {
	evalInt := readFixture(t, "eval_int.json")           // success:true, data.result.result=42
	evalErr := readFixture(t, "eval_compile_error.json") // success:false, errors[0].code=COMMAND_FAILED

	cases := []struct {
		name     string
		args     []string
		body     string
		exitCode int

		wantExit               int
		wantArgv               string // exact match; "" = skip
		wantStdout             string // substring unless exactStdout
		exactStdout            bool
		wantStderr             string // substring; "" = skip
		wantOneTrailingNewline bool
	}{
		{
			name:     "filtered path appends --json --no-banner and compacts the payload",
			args:     []string{"exec", "return 42;"},
			body:     evalInt,
			exitCode: 0,
			wantExit: 0,
			// The exec budget rides along twice: seconds before `--` for the CLI
			// transport, milliseconds after it for the eval tool (which enforces
			// its own 5000ms default since com.unity.pipeline 0.5.0).
			wantArgv:   "command eval return 42; --json --no-banner --timeout 70 -- --timeout 60000",
			wantStdout: "42\n",
			// The whole envelope collapses to the value: nine of its ten fields
			// are null/empty boilerplate paid for on every command.
			exactStdout: true,
		},
		{
			name:     "exec --timeout is milliseconds, split across transport and tool",
			args:     []string{"exec", "--timeout", "3000", "return 42;"},
			body:     evalInt,
			exitCode: 0,
			wantExit: 0,
			wantArgv: "command eval return 42; --json --no-banner --timeout 13 -- --timeout 3000",
		},
		{
			name:       "exec --timeout rejects a non-positive or non-numeric value",
			args:       []string{"exec", "--timeout", "soon", "return 42;"},
			body:       evalInt,
			exitCode:   0,
			wantExit:   2,
			wantStderr: "must be a positive integer",
		},
		{
			name:        "--raw streams the mapped argv through unchanged, no --json",
			args:        []string{"console", "--type", "error", "--raw"},
			body:        "raw passthrough output\n",
			exitCode:    0,
			wantExit:    0,
			wantArgv:    "command console --level error",
			wantStdout:  "raw passthrough output\n",
			exactStdout: true,
		},
		{
			name:       "success:false with exit 0 becomes exit 1, error on stderr",
			args:       []string{"exec", "bad code"},
			body:       evalErr,
			exitCode:   0,
			wantExit:   1,
			wantStderr: "COMMAND_FAILED",
		},
		{
			name:     "non-zero exit code from the official CLI is preserved",
			args:     []string{"exec", "return 42;"},
			body:     evalInt,
			exitCode: 6,
			wantExit: 6,
		},
		{
			name:        "non-envelope stdout passes through verbatim (loss-safe)",
			args:        []string{"exec", "whatever"},
			body:        "/Users/me/project/Assets",
			exitCode:    0,
			wantExit:    0,
			wantStdout:  "/Users/me/project/Assets",
			exactStdout: true,
		},
		{
			name:       "savings line reported on stderr for the filtered path",
			args:       []string{"exec", "return 42;"},
			body:       evalInt,
			exitCode:   0,
			wantExit:   0,
			wantStderr: "saved",
		},
		{
			name:                   "filtered output ends with exactly one trailing newline",
			args:                   []string{"exec", "return 42;"},
			body:                   evalInt,
			exitCode:               0,
			wantExit:               0,
			wantOneTrailingNewline: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bin, argvFile := fakeUnity(t, c.body, c.exitCode)
			t.Setenv("UTK_UNITY_BIN", bin)
			// These cases assert on the one call utk makes for the verb; the
			// log merge adds a second one of its own and has its own test.
			t.Setenv("UTK_NO_EXEC_LOGS", "1")

			var stdout, stderr bytes.Buffer
			code := run(c.args, &stdout, &stderr)
			if code != c.wantExit {
				t.Fatalf("exit = %d, want %d (stderr: %s)", code, c.wantExit, stderr.String())
			}
			if c.wantArgv != "" {
				if got := readArgv(t, argvFile); got != c.wantArgv {
					t.Fatalf("argv = %q, want %q", got, c.wantArgv)
				}
			}
			if c.exactStdout {
				if stdout.String() != c.wantStdout {
					t.Fatalf("stdout = %q, want %q", stdout.String(), c.wantStdout)
				}
			} else if c.wantStdout != "" && !strings.Contains(stdout.String(), c.wantStdout) {
				t.Fatalf("stdout = %q, want substring %q", stdout.String(), c.wantStdout)
			}
			if c.wantStderr != "" && !strings.Contains(stderr.String(), c.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr.String(), c.wantStderr)
			}
			if c.wantOneTrailingNewline {
				out := stdout.String()
				if !strings.HasSuffix(out, "\n") || strings.HasSuffix(out, "\n\n") {
					t.Fatalf("stdout = %q, want exactly one trailing newline", out)
				}
			}
		})
	}
}

// `utk --raw console` used to read --raw as the command name and shell out to
// `unity command --raw console`.
func TestRun_RawBeforeVerb(t *testing.T) {
	bin, argvFile := fakeUnity(t, "logs", 0)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--raw", "console"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr.String())
	}
	argv := readArgv(t, argvFile)
	if want := "command console"; argv != want {
		t.Fatalf("argv = %q, want %q", argv, want)
	}
	if stdout.String() != "logs" {
		t.Fatalf("--raw must stream through, got %q", stdout.String())
	}
}

// Asset paths are project-root relative — the base the Editor resolves them
// against — so utk must not reject a valid path just because cwd is elsewhere.
func TestRun_ReserializeResolvesFromProjectRoot(t *testing.T) {
	proj := t.TempDir()
	for _, d := range []string{"Assets", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(proj, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "Assets", "S.unity"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNITY_PROJECT_PATH", proj) // cwd stays outside the project

	bin, _ := fakeUnity(t, `{"success":true,"data":{"result":{"result":"reserialized","success":true}}}`, 0)
	t.Setenv("UTK_UNITY_BIN", bin)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"reserialize", "Assets/S.unity"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"reserialize", "Assets/Nope.unity"}, &stdout, &stderr); code != 2 {
		t.Fatalf("missing path exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no such file") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
