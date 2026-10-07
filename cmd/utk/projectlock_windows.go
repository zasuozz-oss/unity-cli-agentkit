//go:build windows

package main

// projectLockHeld: Windows has no flock on the lock file here, so it cannot tell; canListProcesses is false there and restart
// refuses to launch (see startStoppedEditor).
func projectLockHeld(project string) bool { return false }

func canListProcesses() bool { return false }
