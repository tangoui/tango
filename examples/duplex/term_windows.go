package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000 // CREATE_NO_WINDOW

// shellInit is written to the shell's stdin first; cmd.exe needs nothing.
const shellInit = ""

// shellGroup keeps cmd.exe from opening a console window when the app is
// built as a GUI program (-H windowsgui), and makes canceling the command
// kill its whole process tree.
func shellGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	cmd.Cancel = func() error { return killShell(cmd) }
}

// killShell kills cmd.exe and every process it started (taskkill /T). A
// process that left the tree survives; cmd.WaitDelay keeps term from
// waiting on it.
func killShell(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := tk.Run(); err != nil {
		return cmd.Process.Kill() // at least the shell
	}
	return nil
}
