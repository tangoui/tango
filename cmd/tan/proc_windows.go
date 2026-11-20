package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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

func startDevGroup(cmd *exec.Cmd) (*jobGroup, error) {
	newGroup(cmd)
	// Assign the job before the command can spawn children. A job still
	// holds the tree if its leader exits before a cancellation is handled.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	group := attachJobGroup(cmd)
	var err error
	if group.job == 0 {
		err = errors.New("could not assign the process to a Windows job object")
	} else {
		err = resumeDevCommand(uint32(cmd.Process.Pid))
	}
	if err != nil {
		// The command is still suspended and cannot have descendants.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		group.release()
		return nil, fmt.Errorf("starting %s: %w", cmd.Path, err)
	}
	return group, nil
}

func resumeDevCommand(pid uint32) error {
	// os/exec closes the primary thread handle. At this point the
	// suspended process has only that thread, so reopen it by PID.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(thread)
		_, err = windows.ResumeThread(thread)
		return err
	}
	return errors.New("could not find the suspended process's primary thread")
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
