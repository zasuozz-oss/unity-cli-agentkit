//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// projectLockHeld reports whether a Unity process holds <project>/Temp/UnityLockfile (Unity keeps a flock on it for as long as the
// project is open, whatever the command line looked like). A leftover file after a crash is not locked. Anything unclear (unreadable file, flock error) counts as held.
func projectLockHeld(project string) bool {
	f, err := os.Open(filepath.Join(project, "Temp", "UnityLockfile"))
	if err != nil {
		return !os.IsNotExist(err) // exists but unreadable: cannot tell, so it counts as held
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return true // EWOULDBLOCK = held; any other error = cannot tell: both mean "do not launch"
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

// canListProcesses: `ps` answered, so "no Unity process matched" means no Unity process, not "could not look".
func canListProcesses() bool { return exec.Command("ps", "-axo", "pid=").Run() == nil }
