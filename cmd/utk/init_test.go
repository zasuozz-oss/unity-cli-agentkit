package main

import (
	"os"
	"testing"
)

// `utk init --help` used to run a real init, replacing the installed skills
// and the CLAUDE.md/AGENTS.md block of whatever project it was probed in.
func TestInitHelpAndUnknownArgsTouchNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	if code := runInit([]string{"--help"}); code != 0 {
		t.Fatalf("init --help exit = %d, want 0", code)
	}
	if code := runInit([]string{"--frobnicate"}); code != 2 {
		t.Fatalf("init with unknown arg exit = %d, want 2", code)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Fatalf("init wrote %d entries for a help/unknown-arg call", len(entries))
	}
}
