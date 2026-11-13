//go:build darwin && !ios

package autostart

import (
	"os"
	"path/filepath"
)

func plistPath(o Options) (string, error) {
	dir := o.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Library", "LaunchAgents")
	}
	return filepath.Join(dir, o.ID+".plist"), nil
}

func enable(o Options, cmd []string) error {
	p, err := plistPath(o)
	if err != nil {
		return err
	}
	return writeFile(p, []byte(launchAgentPlist(o.ID, cmd)))
}

func disable(o Options) error {
	p, err := plistPath(o)
	if err != nil {
		return err
	}
	return removeFile(p)
}

func isEnabled(o Options) (bool, error) {
	p, err := plistPath(o)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
