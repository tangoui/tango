package updateui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// stateFile is updateui's file in the updater's state directory, next to
// (and distinct from) the updater's own state files.
const stateFile = "updateui.json"

// downloadsDir, in the state directory, holds downloaded releases, one
// directory per version. Not $TMPDIR: a release kept for "Install on Next
// Launch" must survive a reboot, which may empty the temporary directory.
const downloadsDir = "updateui-downloads"

// persisted is what updateui remembers across launches.
type persisted struct {
	Skipped   string          `json:"skipped,omitempty"`  // "Skip This Version"
	LastCheck time.Time       `json:"lastCheck,omitzero"` // last background or manual check
	Pending   *pendingPayload `json:"pending,omitempty"`  // "Install on Next Launch"
}

// pendingPayload is a downloaded, verified release kept for the next
// launch: enough to rebuild the *updater.Payload for Apply.
type pendingPayload struct {
	Version string           `json:"version"`
	Path    string           `json:"path"`
	Patched bool             `json:"patched,omitempty"`
	Release *updater.Release `json:"release,omitempty"`
	// TreeSHA256 is Release.TreeSHA256, which Release's JSON leaves out
	// (json:"-") but Apply checks a tree release (formats app, dir)
	// against.
	TreeSHA256 string `json:"treeSHA256,omitempty"`
}

// newPending records p for the next launch.
func newPending(p *updater.Payload) *pendingPayload {
	pp := &pendingPayload{Path: p.Path, Patched: p.Patched, Release: p.Release}
	if p.Release != nil {
		pp.Version, pp.TreeSHA256 = p.Release.Version, p.Release.TreeSHA256
	}
	return pp
}

// payload rebuilds the Payload for Apply, restoring the tree digest.
func (pp *pendingPayload) payload() *updater.Payload {
	rel := &updater.Release{Version: pp.Version}
	if pp.Release != nil {
		r := *pp.Release
		rel = &r
	}
	rel.TreeSHA256 = pp.TreeSHA256
	return &updater.Payload{Release: rel, Path: pp.Path, Patched: pp.Patched}
}

// stateDir returns (and creates) the updater's state directory, with the
// same rules as the unexported Updater.stateDir (service/updater
// state.go): Updater.StateDir, else <user config dir>/tango-updater/
// <AppID, else the executable's name without extension>.
func stateDir(u *updater.Updater) (string, error) {
	dir := u.StateDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("updateui: no state directory: %w", err)
		}
		name := u.AppID
		if name == "" {
			exe, err := os.Executable()
			if err != nil {
				return "", fmt.Errorf("updateui: %w", err)
			}
			name = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
		}
		dir = filepath.Join(base, "tango-updater", safeName(name))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("updateui: %w", err)
	}
	return dir, nil
}

// safeName is service/updater's: letters, digits, '.', '-' and '_' are
// kept, anything else becomes '_'.
func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '.' || r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// loadState reads the state file; a missing or unreadable file is the
// zero state.
func loadState(u *updater.Updater) persisted {
	var p persisted
	dir, err := stateDir(u)
	if err != nil {
		return p
	}
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil || json.Unmarshal(b, &p) != nil {
		return persisted{}
	}
	return p
}

// saveState replaces the state file atomically (write, then rename).
func saveState(u *updater.Updater, p persisted) error {
	dir, err := stateDir(u)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, stateFile+".*")
	if err != nil {
		return fmt.Errorf("updateui: %w", err)
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	serr := f.Sync() // on disk before the rename makes it the state file
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		os.Remove(tmp)
		return fmt.Errorf("updateui: writing state: %w", errors.Join(werr, serr, cerr))
	}
	if err := os.Rename(tmp, filepath.Join(dir, stateFile)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("updateui: %w", err)
	}
	return nil
}

// usablePending returns the update kept for this launch when its file is
// still there and it is newer than current; otherwise it forgets it (sets
// Pending to nil) and returns nil.
func (p *persisted) usablePending(current string) *pendingPayload {
	pp := p.Pending
	if pp == nil {
		return nil
	}
	if _, err := os.Stat(pp.Path); err != nil || !newer(pp.Version, current) {
		p.Pending = nil
		return nil
	}
	return pp
}

// newer reports whether semantic version a is newer than b, with
// service/updater's rules (updater.CompareVersions); false when either
// does not parse.
func newer(a, b string) bool {
	c, err := updater.CompareVersions(a, b)
	return err == nil && c > 0
}

// downloadDir returns a fresh (emptied) directory for downloading version.
func downloadDir(u *updater.Updater, version string) (string, error) {
	sd, err := stateDir(u)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(sd, downloadsDir, safeName(version))
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("updateui: %w", err)
	}
	return dir, nil
}

// pruneDownloads removes every download except the version directory
// holding keep ("" keeps none).
func pruneDownloads(u *updater.Updater, keep string) {
	sd, err := stateDir(u)
	if err != nil {
		return
	}
	root := filepath.Join(sd, downloadsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if keep != "" && strings.HasPrefix(keep, dir+string(filepath.Separator)) {
			continue
		}
		os.RemoveAll(dir)
	}
}
