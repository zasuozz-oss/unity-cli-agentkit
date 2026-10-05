//go:build windows

package main

import "os"

// pidAlive: on Windows os.FindProcess opens the process handle and fails when
// the pid is gone, and Signal(0) is unsupported there, so a found process
// counts as alive.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}

// pidIsServe: Windows has no cheap command-line lookup in the stdlib, so a
// live pid counts as the coordinator.
func pidIsServe(pid int) bool { return pidAlive(pid) }
