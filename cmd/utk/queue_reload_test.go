package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mvid-fixture.dll is My-Game' Library/ScriptAssemblies/PPv2URPConverters.dll; the Editor reported this ModuleVersionId for it.
const fixtureMVID = "1cae212c-8d54-40d3-bb64-2dee2bbd9984"

func TestPeMVIDReadsTheModuleVersionId(t *testing.T) {
	got, err := peMVID(filepath.Join("testdata", "mvid-fixture.dll"))
	if err != nil || got != fixtureMVID {
		t.Fatalf("mvid = %q, %v; want %s", got, err, fixtureMVID)
	}
	if _, err := peMVID(filepath.Join("testdata")); err == nil {
		t.Error("a directory is not an assembly")
	}
}

// projectWithAssembly: a cwd whose Library/ScriptAssemblies holds the fixture as Game.dll.
func projectWithAssembly(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sa := filepath.Join(dir, "Library", "ScriptAssemblies")
	if err := os.MkdirAll(sa, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join("testdata", "mvid-fixture.dll"))
	os.WriteFile(filepath.Join(sa, "Game.dll"), b, 0o644)
	return dir
}

// The P6c failure: compile PASS in 0 s, the dll on disk is new, the Editor still runs the old one. The cycle must force a reload
// and only then pass.
func TestCycleForcesAReloadWhenTheEditorRunsOldIL(t *testing.T) {
	cwd := projectWithAssembly(t)
	calls := fakeExec(t, fakeAnswers{
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"up_to_date\n", 0},
	})
	loaded := "Game.dll=00000000-0000-0000-0000-000000000000;" // old module
	inner := queueExec
	queueExec = func(c string, d time.Duration, env []string, name string, args ...string) (string, int) {
		if len(args) > 1 && args[0] == "exec" {
			switch args[1] {
			case loadedMVIDSnippet:
				return "\"" + loaded + "\"", 0
			case reloadSnippet:
				loaded = "Game.dll=" + fixtureMVID + ";" // the reload loads the file on disk
				return "reload requested", 0
			}
		}
		return inner(c, d, env, name, args...)
	}
	t.Cleanup(func() { queueExec = inner })
	c := newCycle(t.TempDir(), []queueRequest{{ID: "1", Kind: "compile", Cwd: cwd}}, io.Discard)
	if !c.runGateAndRefresh() {
		t.Fatal("editor should be OK after the forced reload")
	}
	if r := c.results["1"]; r.Status != "PASS" {
		t.Fatalf("result %+v", r)
	}
	_ = calls
}

func TestCycleFailsWhenTheReloadDoesNotTake(t *testing.T) {
	cwd := projectWithAssembly(t)
	fakeExec(t, fakeAnswers{
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"completed\n", 0},
	})
	inner := queueExec
	queueExec = func(c string, d time.Duration, env []string, name string, args ...string) (string, int) {
		if len(args) > 1 && args[0] == "exec" {
			switch args[1] {
			case loadedMVIDSnippet:
				return "Game.dll=00000000-0000-0000-0000-000000000000;", 0 // never changes
			case reloadSnippet:
				return "reload requested", 0
			}
		}
		return inner(c, d, env, name, args...)
	}
	t.Cleanup(func() { queueExec = inner })
	c := newCycle(t.TempDir(), []queueRequest{{ID: "1", Kind: "compile", Cwd: cwd}, {ID: "2", Kind: "test", Cwd: cwd, Args: queueArgs{Filter: "A", Mode: "editmode"}}}, io.Discard)
	if c.runGateAndRefresh() {
		t.Fatal("stale IL must not count as a good Editor")
	}
	for _, id := range []string{"1", "2"} {
		if r := c.results[id]; r.Status != "FAIL" || !strings.Contains(r.Stdout, "STALE_IL") || !strings.Contains(r.Stdout, "Game.dll") {
			t.Errorf("req %s: %+v", id, r)
		}
	}
}

func TestCycleSkipsTheLoadedCheckWithoutScriptAssemblies(t *testing.T) {
	fakeExec(t, fakeAnswers{ // no answer for the snippet: asking would fail the fake
		"unity-test.sh offline": {"COMPILE OK\n", 0},
		"utk editor refresh":    {"completed\n", 0},
	})
	c := newCycle(t.TempDir(), []queueRequest{{ID: "1", Kind: "compile", Cwd: "/p"}}, io.Discard)
	if !c.runGateAndRefresh() || c.results["1"].Status != "PASS" {
		t.Fatalf("%+v", c.results["1"])
	}
}
