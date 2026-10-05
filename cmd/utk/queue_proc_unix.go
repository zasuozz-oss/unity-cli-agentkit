//go:build !windows

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// runWithTimeout runs name args… in cwd inside its own process group; on
// timeout the whole group gets TERM, then KILL. A hung utk child must not keep
// driving the Editor after the coordinator moved on (unity-job.sh learned this
// the hard way with perl setpgrp). Exit 124 marks a timeout, like coreutils.
func runWithTimeout(cwd string, timeout time.Duration, env []string, name string, args ...string) (string, int) {
	return runWithAbort(cwd, timeout, env, nil, name, args...)
}

// runWithAbort is runWithTimeout that also stops the command (exit 130) once
// gone() reports that nobody waits for its result. gone is polled every second.
func runWithAbort(cwd string, timeout time.Duration, env []string, gone func() bool, name string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.Command(name, args...)
	// Pipe holders outside the process group (setsid/daemonised helpers) would make Wait block forever.
	cmd.WaitDelay = 3 * time.Second
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return err.Error(), 127
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stop := func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}
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
		case <-ctx.Done():
			stop()
			return buf.String() + "\nTIMEOUT after " + timeout.String() + "\n", 124
		case <-tick.C:
			if gone != nil && gone() {
				stop()
				return buf.String() + "\nCANCELLED: nobody is waiting for this job\n", 130
			}
		}
	}
}

// detach lets the spawned coordinator outlive the submitting agent's session.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
