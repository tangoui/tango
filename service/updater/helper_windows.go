package updater

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// envHelper names the job file when a copy of the app's executable runs
// as the update helper.
const envHelper = "TANGO_UPDATE_HELPER"

// A copy of the app's own executable is the helper on Windows, where no
// shell can be relied on: Apply copies the executable to a temporary
// folder and starts it with envHelper set, and this init runs the job
// before the app's main ever starts. Package initialisation of the app's
// other imports still runs, so it must not open windows.
func init() {
	job := os.Getenv(envHelper)
	if job == "" {
		return
	}
	os.Unsetenv(envHelper)
	var j helperJob
	b, err := os.ReadFile(job)
	if err == nil {
		err = json.Unmarshal(b, &j)
	}
	if err != nil {
		os.Exit(2)
	}
	if err := runJob(&j, windowsOps()); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGroup, HideWindow: true}
}

func windowsOps() helperOps {
	return helperOps{
		wait: func(pid int) {
			for processAlive(pid) {
				time.Sleep(100 * time.Millisecond)
			}
		},
		start: func(c []string) error {
			cmd := exec.Command(c[0], c[1:]...)
			detach(cmd)
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		},
		run: func(c []string) error {
			cmd := exec.Command(c[0], c[1:]...)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			return cmd.Run()
		},
		alive: processAlive,
		sleep: time.Sleep,
	}
}

func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false // gone (or not ours to wait for)
	}
	defer syscall.CloseHandle(h)
	ev, _ := syscall.WaitForSingleObject(h, 0)
	return ev == syscall.WAIT_TIMEOUT
}

// startHelper copies the running executable to a temporary folder and
// starts it as the helper for j. The copy cannot delete itself; Confirm
// removes its folder (pending.HelperDir) on the next launch.
func startHelper(j *helperJob, p *pending) error {
	switch j.Format {
	case FormatExe, FormatNSIS, FormatMSI:
	default:
		return ErrUnsupported
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "tango-update-helper-")
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, "update-helper.exe")
	if err := copyFile(exe, helper, 0o755); err != nil {
		os.RemoveAll(dir)
		return err
	}
	job := filepath.Join(dir, "job.json")
	if err := writeJSON(job, j); err != nil {
		os.RemoveAll(dir)
		return err
	}
	cmd := exec.Command(helper)
	cmd.Env = append(os.Environ(), envHelper+"="+job)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return err
	}
	p.HelperDir = dir
	return cmd.Process.Release()
}
