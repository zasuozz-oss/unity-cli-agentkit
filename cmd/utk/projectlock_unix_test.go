//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Unity holds a flock on Temp/UnityLockfile while the project is open; another open file description with that lock is the same thing.
func TestProjectLockHeld_SeesAFlockedFile(t *testing.T) {
	p := t.TempDir()
	os.MkdirAll(filepath.Join(p, "Temp"), 0o755)
	f, err := os.Create(filepath.Join(p, "Temp", "UnityLockfile"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if !projectLockHeld(p) {
		t.Fatal("a flocked lock file must read as an open project")
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if projectLockHeld(p) {
		t.Fatal("released: not held")
	}
}

// A lock file that exists but cannot be examined is "cannot tell": it counts as held, never as free.
func TestProjectLockHeld_UnreadableFileCountsAsHeld(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	p := t.TempDir()
	os.MkdirAll(filepath.Join(p, "Temp"), 0o755)
	lf := filepath.Join(p, "Temp", "UnityLockfile")
	os.WriteFile(lf, nil, 0o000)
	if !projectLockHeld(p) {
		t.Fatal("an unreadable lock file must not read as free")
	}
	os.Chmod(lf, 0o644)
}

// Unity started from the Hub with a differently spelled path (case, symlink, trailing /) is invisible to the ps match; the lock
// file is the same file whatever the spelling.
func TestProjectLockHeld_SeesTheFileThroughASymlinkedProject(t *testing.T) {
	real := t.TempDir()
	os.MkdirAll(filepath.Join(real, "Temp"), 0o755)
	f, _ := os.Create(filepath.Join(real, "Temp", "UnityLockfile"))
	defer f.Close()
	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	link := filepath.Join(t.TempDir(), "proj")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks")
	}
	if !projectLockHeld(link + "/") {
		t.Fatal("the project opened through a symlink is still held")
	}
}
