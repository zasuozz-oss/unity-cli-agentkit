package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recompile_status returns the status file's contents, so the payload is a
// JSON string wrapping JSON — reading it as an object sees no status at all
// and would poll until the budget ran out on a compile that had finished.
func TestRecompileState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		status  string
		failed  bool
	}{
		{"string-wrapped", `"{\"status\":\"completed\",\"failed\":false,\"errors\":[]}"`, "completed", false},
		{"failed compile", `"{\"status\":\"completed\",\"failed\":true,\"errors\":[\"CS0103\"]}"`, "completed", true},
		{"plain object", `{"status":"compiling","failed":false}`, "compiling", false},
		{"refresh hand-off", `{"status":"up_to_date","message":"No scripts needed recompilation."}`, "up_to_date", false},
		{"garbage", `not json`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, failed := recompileState([]byte(tc.payload))
			if st != tc.status || failed != tc.failed {
				t.Errorf("got (%q,%v), want (%q,%v)", st, failed, tc.status, tc.failed)
			}
		})
	}
}

// A snippet is retried with `Object.` qualified only when that is what broke
// it; rewriting anything else would change what the caller asked to run.
func TestQualifyObject(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    string
		changed bool
	}{
		{"var x = Object.FindAnyObjectByType<Foo>();", "var x = UnityEngine.Object.FindAnyObjectByType<Foo>();", true},
		{"Object.DestroyImmediate(go);", "UnityEngine.Object.DestroyImmediate(go);", true},
		{"UnityEngine.Object.Destroy(go);", "UnityEngine.Object.Destroy(go);", false},
		{"go.Object.Thing();", "go.Object.Thing();", false},
		{"MyObject.Run();", "MyObject.Run();", false},
		{"var o = new Object();", "var o = new Object();", false},
	} {
		got, changed := qualifyObject(tc.in)
		if got != tc.want || changed != tc.changed {
			t.Errorf("qualifyObject(%q) = (%q,%v), want (%q,%v)", tc.in, got, changed, tc.want, tc.changed)
		}
	}
}

// The eval_file form must retry against a copy: the caller's file is theirs,
// and utk is only guessing at the fix.
func TestQualifyObjectArgs_FileIsNotEdited(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "snippet.cs")
	const body = "return Object.FindAnyObjectByType<Foo>();"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	args, cleanup, ok := qualifyObjectArgs([]string{"command", "eval_file", "--file", src})
	if !ok {
		t.Fatal("want a retry for a snippet with a bare Object.")
	}
	defer cleanup()
	if got := findFlag(args, "--file"); got == src {
		t.Error("retry must not point back at the caller's file")
	} else if b, err := os.ReadFile(got); err != nil || string(b) != "return UnityEngine.Object.FindAnyObjectByType<Foo>();" {
		t.Errorf("copy not qualified: %q (%v)", b, err)
	}
	if b, _ := os.ReadFile(src); string(b) != body {
		t.Errorf("caller's file was rewritten: %q", b)
	}
}

// The composited capture lands a frame later, and the file exists from the
// moment Unity opens it — so "it is there" is not the same as "it is readable".
func TestWaitForPNG_PartialWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := waitForPNG(path, 100*time.Millisecond); ok {
		t.Error("a truncated PNG must not be reported as the capture")
	}
}

// `utk editor refresh` must answer with the compile's outcome, not with the
// hand-off notice — and a compile that produced errors must reach $?, so
// `utk editor refresh && utk run_tests` stops at the broken build.
func TestRun_EditorRefreshWaitsForTheCompile(t *testing.T) {
	// Measured shapes: recompile answers with an object, recompile_status with
	// the status file's contents as a JSON string at data.result.
	handoff := `{"success":true,"data":{"result":{"status":"compiling","message":"Recompilation started."}}}`
	done := `{"success":true,"data":{"result":"{\"status\":\"completed\",\"failed\":true,\"errors\":[\"Foo.cs(3,5): error CS0103\"]}"}}`

	bin, _ := fakeUnity(t, handoff, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	fakeUnityNext(t, done)

	var stdout, stderr bytes.Buffer
	code := run([]string{"editor", "refresh"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a failed compile (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "CS0103") {
		t.Errorf("want the compile errors in stdout, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "Recompilation started") {
		t.Errorf("hand-off notice answered instead of the result: %q", stdout.String())
	}
}

// Nothing to compile is already the final answer: polling for it would add a
// round-trip to the most common refresh of all.
func TestRun_EditorRefreshUpToDateDoesNotPoll(t *testing.T) {
	upToDate := `{"success":true,"data":{"result":{"status":"up_to_date","message":"No scripts needed recompilation."}}}`
	bin, _ := fakeUnity(t, upToDate, 0)
	t.Setenv("UTK_UNITY_BIN", bin)
	fakeUnityNext(t, `{"success":false,"errors":[{"code":"X","message":"polled when it should not have"}]}`)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"editor", "refresh"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "up_to_date") {
		t.Errorf("stdout = %q", stdout.String())
	}
}
