//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// groupOutlivesLeader: the process group can still be signalled after its
// leader exited, reaching the children it left behind.
const groupOutlivesLeader = true

// shellCommand runs line through the system shell.
func shellCommand(line string) *exec.Cmd { return exec.Command("/bin/sh", "-c", line) }

// newGroup starts cmd in its own process group so stopping it also stops
// whatever it spawned (npm → node → esbuild).
func newGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalGroup sends SIGTERM (or SIGKILL when force) to cmd's group.
func signalGroup(cmd *exec.Cmd, force bool) {
	if cmd.Process == nil {
		return
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}
