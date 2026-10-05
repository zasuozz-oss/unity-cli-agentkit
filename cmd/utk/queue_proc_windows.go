//go:build windows

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

// runWithTimeout on Windows kills only the direct child: there is no process
// group to signal. Good enough for the Windows agents, who run one job at a time.
func runWithTimeout(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// Pipe holders outside the process group (setsid/daemonised helpers) would make Wait block forever.
	cmd.WaitDelay = 3 * time.Second
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if ctx.Err() != nil {
		return buf.String() + "\nTIMEOUT after " + timeout.String() + "\n", 124
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return buf.String(), ee.ExitCode()
	}
	if err != nil {
		return buf.String() + err.Error(), 1
	}
	return buf.String(), 0
}

// detach lets the spawned coordinator outlive the submitting agent's session.
func detach(cmd *exec.Cmd) {}
