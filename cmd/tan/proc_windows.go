package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

// groupOutlivesLeader: taskkill /T walks the tree from the leader's pid,
// so once the leader exited there is nothing left to reach.
const groupOutlivesLeader = false

// shellCommand runs line with cmd.exe. The command line is passed as is:
// os/exec would quote line for CommandLineToArgvW, turning its " into \",
// which cmd.exe does not unescape. /S makes cmd strip only the outer pair
// of quotes added here.
func shellCommand(line string) *exec.Cmd {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd /S /C "` + line + `"`}
	return cmd
}

// newGroup gives cmd a new process group; Windows has no group signals, so
// signalGroup ends the process tree with taskkill instead.
func newGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

func signalGroup(cmd *exec.Cmd, force bool) {
	if cmd.Process == nil {
		return
	}
	args := []string{"/T", "/PID", strconv.Itoa(cmd.Process.Pid)}
	if !force && exec.Command("taskkill", args...).Run() == nil {
		return
	}
	// Without /F taskkill only asks windows to close; it refuses console
	// processes (a dev server, its shell), which have none. Waiting out the
	// grace period would change nothing, so end the tree now.
	_ = exec.Command("taskkill", append([]string{"/F"}, args...)...).Run()
}
