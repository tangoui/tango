//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// shellInit is written to the shell's stdin first: it turns job control
// off. bash -i turns it on when it has no terminal at all, and then runs
// every command in a process group of its own, out of reach of killShell.
// (bash echoes this line after its first prompt.)
const shellInit = "set +m\n"

// shellGroup starts the shell in a new session, so it leads a process
// group of its own and has no controlling terminal (an inherited one,
// say from go run, would let it fight the parent shell for the terminal).
// Canceling the command kills that whole group: the shell and, job
// control being off, whatever command it is running.
func shellGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return killShell(cmd) }
}

// killShell kills the shell's process group. A process that moved to
// another group survives; cmd.WaitDelay keeps term from waiting on it.
func killShell(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // -pid: the group
}
