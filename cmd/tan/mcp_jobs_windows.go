//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobGroup is a job's process group plus a Windows job object holding the
// process tree, so it can be ended even after the leader exited. job is 0
// when the job object could not be made; kill then falls back to taskkill.
type jobGroup struct {
	pid int
	mu  sync.Mutex // guards job: stop and stopAll may overlap
	job windows.Handle
}

// prepareJobGroup makes cmd, before it starts, lead a new process group,
// which CTRL_BREAK_EVENT can be sent to.
func prepareJobGroup(cmd *exec.Cmd) { newGroup(cmd) }

// attachJobGroup puts the started process into a new job object that kills
// its processes when the last handle closes (also when tan mcp dies).
// Children it spawns join the job; tan starts no child before the few
// microseconds this takes.
func attachJobGroup(cmd *exec.Cmd) *jobGroup {
	g := &jobGroup{pid: cmd.Process.Pid}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return g
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return g
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(g.pid))
	if err != nil {
		windows.CloseHandle(job)
		return g
	}
	err = windows.AssignProcessToJobObject(job, proc)
	windows.CloseHandle(proc)
	if err != nil {
		windows.CloseHandle(job)
		return g
	}
	g.job = job
	return g
}

// terminate sends CTRL_BREAK_EVENT to the process group (a Go program sees
// os.Interrupt). It fails when tan mcp has no console to share.
func (g *jobGroup) terminate() error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(g.pid))
}

// kill ends every process in the job object, or the tree under the leader.
func (g *jobGroup) kill() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job != 0 {
		_ = windows.TerminateJobObject(g.job, 1)
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(g.pid)).Run()
}

// release closes the job object handle, which also ends its processes.
func (g *jobGroup) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job != 0 {
		windows.CloseHandle(g.job)
		g.job = 0
	}
}
