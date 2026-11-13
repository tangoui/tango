//go:build linux && !android

package autostart

import (
	"os"
	"path/filepath"
)

func entryPath(o Options) (string, error) {
	dir := o.Dir
	if dir == "" {
		cfg := os.Getenv("XDG_CONFIG_HOME")
		if cfg == "" || !filepath.IsAbs(cfg) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			cfg = filepath.Join(home, ".config")
		}
		dir = filepath.Join(cfg, "autostart")
	}
	return filepath.Join(dir, o.ID+".desktop"), nil
}

func enable(o Options, cmd []string) error {
	p, err := entryPath(o)
	if err != nil {
		return err
	}
	return writeFile(p, []byte(desktopEntry(o.name(), cmd)))
}

func disable(o Options) error {
	p, err := entryPath(o)
	if err != nil {
		return err
	}
	return removeFile(p)
}

func isEnabled(o Options) (bool, error) {
	p, err := entryPath(o)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return desktopEntryEnabled(string(data)), nil
}
