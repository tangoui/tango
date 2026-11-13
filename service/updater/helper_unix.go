//go:build unix

package updater

import (
	"os/exec"
	"syscall"
)

// startHelper starts helperScript in its own session, so that it outlives
// the app and is not killed with the app's process group.
func startHelper(j *helperJob, _ *pending) error {
	switch j.Format {
	case FormatApp, FormatDir, FormatAppImage:
	default:
		return ErrUnsupported
	}
	cmd := exec.Command("/bin/sh", helperArgs(j)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
