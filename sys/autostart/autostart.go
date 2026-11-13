// Package autostart starts the app when the user logs in.
//
//	o := autostart.Options{ID: "com.example.notes", Name: "Notes"}
//	err := autostart.Enable(o)
//
// Per platform:
//
//   - macOS: a launch agent, ~/Library/LaunchAgents/<ID>.plist. It takes
//     effect at the next login; System Settings lists it under Login Items
//     ("Allow in the Background").
//   - Windows: a value named <ID> under
//     HKCU\Software\Microsoft\Windows\CurrentVersion\Run. Task Manager's
//     Startup tab can disable it without removing it; IsEnabled then
//     reports false.
//   - Linux: an XDG autostart entry, ~/.config/autostart/<ID>.desktop
//     ($XDG_CONFIG_HOME is honoured). A desktop that turned it off
//     (Hidden=true, X-GNOME-Autostart-enabled=false) reads as disabled.
//
// The login command runs the executable with Args and LoginArg, which
// OpenedAtLogin detects, so the app can start hidden in the tray.
//
// The package needs no running app and does no work on the main thread.
// Other platforms return ErrUnsupported.
package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported, re-exported for callers
// outside the module.
var ErrUnsupported = platform.ErrUnsupported

// LoginArg is added to the command line of an app started at login.
const LoginArg = "--tango-login"

// Options identifies the login item.
type Options struct {
	// ID names the entry: a reverse-DNS identifier such as
	// "com.example.notes". Required; it must not contain path separators.
	ID string
	// Name is shown by the desktop (Linux entry Name). Defaults to ID.
	Name string
	// Exec is the program to start; defaults to os.Executable().
	Exec string
	// Args are extra arguments, before LoginArg.
	Args []string
	// Dir overrides where the entry lives, for tests and portable
	// installs: the LaunchAgents directory (macOS), the autostart
	// directory (Linux) or the registry key under HKEY_CURRENT_USER
	// (Windows).
	Dir string
}

func (o Options) check() error {
	if o.ID == "" {
		return errors.New("tango/autostart: Options.ID is required")
	}
	if strings.ContainsAny(o.ID, `/\`) || o.ID == "." || o.ID == ".." {
		return errors.New("tango/autostart: Options.ID must not contain path separators")
	}
	return nil
}

func (o Options) name() string {
	if o.Name != "" {
		return o.Name
	}
	return o.ID
}

// command returns the program and arguments the login item runs.
func (o Options) command() ([]string, error) {
	exe := o.Exec
	if exe == "" {
		p, err := os.Executable()
		if err != nil {
			return nil, err
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		exe = p
	}
	args := append([]string{exe}, o.Args...)
	return append(args, LoginArg), nil
}

// Enable registers the login item, replacing an existing one with the
// same ID.
func Enable(o Options) error {
	if err := o.check(); err != nil {
		return err
	}
	cmd, err := o.command()
	if err != nil {
		return err
	}
	return enable(o, cmd)
}

// Disable removes the login item. Removing one that does not exist is not
// an error.
func Disable(o Options) error {
	if err := o.check(); err != nil {
		return err
	}
	return disable(o)
}

// IsEnabled reports whether the login item exists and is not turned off.
func IsEnabled(o Options) (bool, error) {
	if err := o.check(); err != nil {
		return false, err
	}
	return isEnabled(o)
}

// OpenedAtLogin reports whether this process was started by the login
// item (its command line has LoginArg).
func OpenedAtLogin() bool { return slices.Contains(os.Args[1:], LoginArg) }

// writeFile writes data to path atomically, creating the directory.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tango-autostart-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
