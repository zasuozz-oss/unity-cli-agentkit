//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// pidAlive probes with signal 0: no signal is sent, only existence is checked.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// pidIsServe is pidAlive plus "and it is a coordinator": after a crash or a
// reboot serve.pid stays behind, and a reused pid would otherwise read as a
// live server and block every submit for its whole --wait. Without ps, fall
// back to pidAlive rather than spawn a second server on every submit.
func pidIsServe(pid int) bool {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return pidAlive(pid)
	}
	return strings.Contains(string(out), "queue serve")
}
