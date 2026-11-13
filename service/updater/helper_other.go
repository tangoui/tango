//go:build !unix && !windows

package updater

func startHelper(*helperJob, *pending) error { return ErrUnsupported }
