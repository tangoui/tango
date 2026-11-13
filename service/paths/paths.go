// Package paths returns the per-user directories an application keeps its
// files in, on every platform Tango runs on.
//
// On desktop each directory is the platform's conventional location with
// the app ID appended; the app ID should be a reverse-DNS name such as
// "com.example.notes". Mobile apps are sandboxed, so there the ID is not
// appended.
//
//	           Config                     Data                        Cache                    Logs
//	macOS      ~/Library/Application      ~/Library/Application       ~/Library/Caches/ID      ~/Library/Logs/ID
//	           Support/ID                 Support/ID
//	Windows    %AppData%\ID               %LocalAppData%\ID           %LocalAppData%\ID\Cache  %LocalAppData%\ID\Logs
//	Linux/BSD  $XDG_CONFIG_HOME/ID        $XDG_DATA_HOME/ID           $XDG_CACHE_HOME/ID       $XDG_STATE_HOME/ID/logs
//	iOS        $HOME/Library/Application  $HOME/Library/Application   $HOME/Library/Caches     $HOME/Library/Caches/Logs
//	           Support                    Support/Data
//	Android    $HOME/config               $HOME/data                  $HOME/cache              $HOME/logs
//
// The functions only compute paths; call os.MkdirAll before use.
package paths

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Dirs are an application's directories.
type Dirs struct {
	Config string // settings the user would miss if lost
	Data   string // documents, databases and other durable state
	Cache  string // files that can be recreated; the OS may delete them
	Logs   string // log files
}

// For returns the directories of the app with the given ID on this
// platform.
func For(appID string) (Dirs, error) {
	home, _ := os.UserHomeDir() // checked per platform: some need no home
	return dirsFor(runtime.GOOS, appID, home, os.Getenv)
}

// Config returns For(appID).Config.
func Config(appID string) (string, error) { d, err := For(appID); return d.Config, err }

// Data returns For(appID).Data.
func Data(appID string) (string, error) { d, err := For(appID); return d.Data, err }

// Cache returns For(appID).Cache.
func Cache(appID string) (string, error) { d, err := For(appID); return d.Cache, err }

// Logs returns For(appID).Logs.
func Logs(appID string) (string, error) { d, err := For(appID); return d.Logs, err }

var (
	errNoID   = errors.New("paths: empty app ID")
	errBadID  = errors.New("paths: app ID must be a single path element")
	errNoHome = errors.New("paths: cannot determine the home directory")
)

// dirsFor is For with the platform inputs passed in, so every platform's
// rules can be tested anywhere.
func dirsFor(goos, appID, home string, getenv func(string) string) (Dirs, error) {
	if appID == "" {
		return Dirs{}, errNoID
	}
	if appID == "." || appID == ".." || strings.ContainsAny(appID, `/\`) {
		return Dirs{}, errBadID
	}
	// Paths are built and judged by the target platform's rules, not the
	// host's (the same thing when goos is runtime.GOOS).
	join, isAbs := filepath.Join, filepath.IsAbs
	switch {
	case goos == "windows" && filepath.Separator != '\\':
		join = func(el ...string) string { return strings.Join(el, `\`) }
	case goos != "windows" && filepath.Separator != '/':
		join, isAbs = path.Join, path.IsAbs
	}
	needHome := func() error {
		if home == "" {
			return errNoHome
		}
		return nil
	}

	switch goos {
	case "darwin":
		if err := needHome(); err != nil {
			return Dirs{}, err
		}
		lib := join(home, "Library")
		support := join(lib, "Application Support", appID)
		return Dirs{
			Config: support,
			Data:   support,
			Cache:  join(lib, "Caches", appID),
			Logs:   join(lib, "Logs", appID),
		}, nil

	case "windows":
		roaming, local := getenv("APPDATA"), getenv("LOCALAPPDATA")
		if roaming == "" || local == "" {
			if err := needHome(); err != nil {
				return Dirs{}, err
			}
			if roaming == "" {
				roaming = join(home, "AppData", "Roaming")
			}
			if local == "" {
				local = join(home, "AppData", "Local")
			}
		}
		base := join(local, appID)
		return Dirs{
			Config: join(roaming, appID),
			Data:   base,
			Cache:  join(base, "Cache"),
			Logs:   join(base, "Logs"),
		}, nil

	case "ios":
		// $HOME is the app's sandbox container. Library/Caches is not backed
		// up and may be purged by the system, which suits caches and logs.
		if err := needHome(); err != nil {
			return Dirs{}, err
		}
		lib := join(home, "Library")
		support := join(lib, "Application Support")
		return Dirs{
			Config: support,
			Data:   join(support, "Data"),
			Cache:  join(lib, "Caches"),
			Logs:   join(lib, "Caches", "Logs"),
		}, nil

	case "android":
		// The native host is expected to set $HOME to Context.getFilesDir()
		// (the app's private storage) before Go starts; otherwise Go falls
		// back to /sdcard, which the app usually cannot write.
		if err := needHome(); err != nil {
			return Dirs{}, err
		}
		return Dirs{
			Config: join(home, "config"),
			Data:   join(home, "data"),
			Cache:  join(home, "cache"),
			Logs:   join(home, "logs"),
		}, nil

	default: // Linux and other Unix: XDG Base Directory specification
		xdg := func(env string, rel ...string) (string, error) {
			if d := getenv(env); isAbs(d) {
				return d, nil // relative values must be ignored, per the spec
			}
			if err := needHome(); err != nil {
				return "", err
			}
			return join(append([]string{home}, rel...)...), nil
		}
		var d Dirs
		for _, x := range []struct {
			dst  *string
			env  string
			rel  []string
			tail []string
		}{
			{&d.Config, "XDG_CONFIG_HOME", []string{".config"}, nil},
			{&d.Data, "XDG_DATA_HOME", []string{".local", "share"}, nil},
			{&d.Cache, "XDG_CACHE_HOME", []string{".cache"}, nil},
			{&d.Logs, "XDG_STATE_HOME", []string{".local", "state"}, []string{"logs"}},
		} {
			base, err := xdg(x.env, x.rel...)
			if err != nil {
				return Dirs{}, err
			}
			*x.dst = join(append([]string{base, appID}, x.tail...)...)
		}
		return d, nil
	}
}
