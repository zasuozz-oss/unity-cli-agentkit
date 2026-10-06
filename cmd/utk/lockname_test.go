package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeProject makes <parent>/<name> look like a Unity project and returns it.
func fakeProject(t *testing.T, parent, name string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	for _, d := range []string{"Assets/Scripts", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The Editor lock belongs to the project: two agents on one project share
// it whatever their shells export, and a worktree Editor gets its own.
func TestEditorLockName_IsTheProjectFolder(t *testing.T) {
	t.Setenv("UNITY_LOCK_NAME", "my-game-ui") // a UI lock leaking in from an agent's env
	t.Setenv("UNITY_PROJECT_PATH", "")
	tmp := t.TempDir()
	for name, want := range map[string]string{
		"My-Game":    "my-game",
		"My-Game-a4": "my-game-a4",
		"My Game_2":    "my-game-2",
	} {
		root := fakeProject(t, tmp, name)
		if got := editorLockName(root); got != want {
			t.Errorf("%s: root -> %q, want %q", name, got, want)
		}
		if got := editorLockName(filepath.Join(root, "Assets", "Scripts")); got != want {
			t.Errorf("%s: subdirectory -> %q, want %q", name, got, want)
		}
	}
	if got := editorLockName(tmp); got != "unity-editor" {
		t.Errorf("outside a project -> %q, want the shared default", got)
	}
}

func TestQueueDir_IsPerProject(t *testing.T) {
	t.Setenv("UTK_QUEUE_DIR", "")
	t.Setenv("UNITY_PROJECT_PATH", "")
	t.Setenv("HOME", t.TempDir())
	root := fakeProject(t, t.TempDir(), "My-Game")
	t.Chdir(filepath.Join(root, "Assets"))
	if got := queueDir(); !strings.HasSuffix(got, filepath.Join("queue", "my-game")) {
		t.Fatalf("queueDir = %q", got)
	}
}

// The coordinator names the lock for the script instead of trusting the
// script's own view of the cwd or whatever UNITY_LOCK_NAME it inherited.
func TestCoordinatorPinsTheProjectLockName(t *testing.T) {
	t.Setenv("UNITY_LOCK_NAME", "my-game-ui")
	t.Setenv("UNITY_PROJECT_PATH", "")
	root := fakeProject(t, t.TempDir(), "My-Game")
	calls := fakeExec(t, fakeAnswers{
		"unity-lock.sh acquire": {"acquired\n", 0},
		"unity-lock.sh release": {"released\n", 0},
	})
	acquireEditorLock(root)
	releaseEditorLock(root)
	for _, c := range *calls {
		if env := strings.Join(c.env, " "); !strings.Contains(env, "UNITY_LOCK_NAME=my-game") || strings.Contains(env, "my-game-ui") {
			t.Errorf("%s %v: env %q", c.name, c.args, env)
		}
	}
	if len(*calls) != 2 {
		t.Fatalf("calls: %d", len(*calls))
	}
}
