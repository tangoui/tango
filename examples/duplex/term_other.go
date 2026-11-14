//go:build !unix && !windows

package main

import "os/exec"

// shellInit: nothing to send first.
const shellInit = ""

// shellGroup: no process groups here; canceling kills the shell only.
func shellGroup(*exec.Cmd) {}

func killShell(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
