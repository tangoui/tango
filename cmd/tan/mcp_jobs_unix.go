//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// jobGroup is a job's process group; its id is the leader's pid.
type jobGroup struct{ pgid int }

// prepareJobGroup makes cmd, before it starts, lead a new process group.
func prepareJobGroup(cmd *exec.Cmd) { newGroup(cmd) }

func attachJobGroup(cmd *exec.Cmd) *jobGroup { return &jobGroup{pgid: cmd.Process.Pid} }

// terminate sends SIGTERM to the group.
func (g *jobGroup) terminate() error { return syscall.Kill(-g.pgid, syscall.SIGTERM) }

// kill sends SIGKILL to the group; the group outlives its leader, so this
// also reaches what an ended job left behind.
func (g *jobGroup) kill() { _ = syscall.Kill(-g.pgid, syscall.SIGKILL) }

func (g *jobGroup) release() {}
