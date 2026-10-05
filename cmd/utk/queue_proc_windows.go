//go:build windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"time"
)

// runWithTimeout on Windows kills only the direct child: there is no process
// group to signal. Good enough for the Windows agents, who run one job at a time.
func runWithTimeout(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
	return runWithAbort(cwd, timeout, env, nil, name, args...)
}

// runWithAbort is runWithTimeout that also stops the command (exit 130) once
// gone() reports that nobody waits for its result. gone is polled every second.
func runWithAbort(cwd string, timeout time.Duration, env []string, gone func() bool, name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	// Pipe holders outside the process group (setsid/daemonised helpers) would make Wait block forever.
	cmd.WaitDelay = 3 * time.Second
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return err.Error(), 127
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.After(timeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if ee, ok := err.(*exec.ExitError); ok {
				return buf.String(), ee.ExitCode()
			}
			if err != nil {
				return buf.String() + err.Error(), 1
			}
			return buf.String(), 0
		case <-deadline:
			cmd.Process.Kill()
			<-done
			return buf.String() + "\nTIMEOUT after " + timeout.String() + "\n", 124
		case <-tick.C:
			if gone != nil && gone() {
				cmd.Process.Kill()
				<-done
				return buf.String() + "\nCANCELLED: nobody is waiting for this job\n", 130
			}
		}
	}
}

// detach lets the spawned coordinator outlive the submitting agent's session.
func detach(cmd *exec.Cmd) {}
