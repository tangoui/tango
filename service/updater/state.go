package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// State files, in Updater.StateDir:
//
//	pending.json   written by Apply: the update being installed
//	confirmed      written by Confirm in the new version; the helper then
//	               removes the previous version
//	rollback       written by Rollback (holding the caller's pid): the
//	               helper restores the previous version once it exits
//	rolledback     written by the helper when it restored the previous
//	               version (holding the reason); Confirm reports it
//	seen.json      the newest manifest Check has accepted (anti-replay)
const (
	statePending    = "pending.json"
	stateConfirmed  = "confirmed"
	stateRollback   = "rollback"
	stateRolledBack = "rolledback"
	stateSeen       = "seen.json"
)

// pending describes an update Apply has handed to the helper.
type pending struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Target    string    `json:"target"`
	Work      string    `json:"work"`             // holds the previous version until confirmed
	HelperDir string    `json:"helper,omitempty"` // Windows: the helper's copy of the executable
	Time      time.Time `json:"time"`
	Timeout   int       `json:"timeout"` // seconds the helper waits for Confirm
}

// Status is the outcome Confirm reports for the last update.
type Status string

const (
	// StatusNone: no update was pending.
	StatusNone Status = "none"
	// StatusUpdated: this process is the new version; the update is
	// confirmed and the previous version will be removed.
	StatusUpdated Status = "updated"
	// StatusRolledBack: the new version did not confirm in time (or asked
	// for a rollback, or could not be installed) and the previous version,
	// this process, was restored.
	StatusRolledBack Status = "rolled_back"
	// StatusPending: an update is still being installed (this process is
	// the old version and the helper has not swapped yet).
	StatusPending Status = "pending"
)

// Outcome is what Confirm found.
type Outcome struct {
	Status Status `json:"status"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Reason string `json:"reason,omitempty"` // for StatusRolledBack
}

// stateDir returns (and creates) the directory for state files.
func (u *Updater) stateDir() (string, error) {
	dir := u.StateDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("updater: no state directory: %w", err)
		}
		name := u.AppID
		if name == "" {
			exe, err := os.Executable()
			if err != nil {
				return "", fmt.Errorf("updater: %w", err)
			}
			name = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
		}
		dir = filepath.Join(base, "tango-updater", safeName(name))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	return dir, nil
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '.' || r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || strings.Trim(b.String(), ".") == "" {
		return "app"
	}
	return b.String()
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON writes v to path atomically.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Confirm reports how the last update went. Call it early in every
// launch, once the app has started well enough to count as working: in
// the new version it confirms the update, so the helper keeps it and
// deletes the previous version; if it is not called within
// Updater.ConfirmTimeout of the relaunch, the helper puts the previous
// version back and starts it. In the restored previous version it
// returns StatusRolledBack once.
func (u *Updater) Confirm() (Outcome, error) {
	dir, err := u.stateDir()
	if err != nil {
		return Outcome{}, err
	}
	var p pending
	if err := readJSON(filepath.Join(dir, statePending), &p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			os.Remove(filepath.Join(dir, stateRolledBack))
			return Outcome{Status: StatusNone}, nil
		}
		// Unreadable state: start over rather than fail every launch.
		u.clearState(dir)
		return Outcome{Status: StatusNone}, nil
	}
	out := Outcome{From: p.From, To: p.To}
	if reason, err := os.ReadFile(filepath.Join(dir, stateRolledBack)); err == nil {
		out.Status, out.Reason = StatusRolledBack, strings.TrimSpace(string(reason))
		u.clearState(dir)
		if p.HelperDir != "" {
			os.RemoveAll(p.HelperDir)
		}
		return out, nil
	}
	if c, err := CompareVersions(u.Version, p.To); err == nil && c == 0 {
		out.Status = StatusUpdated
		if err := os.WriteFile(filepath.Join(dir, stateConfirmed), nil, 0o600); err != nil {
			return out, fmt.Errorf("updater: %w", err)
		}
		// If the helper is gone (the machine restarted while it waited),
		// finish its job here.
		if time.Since(p.Time) > time.Duration(p.Timeout)*time.Second+2*time.Minute {
			if p.Work != "" {
				os.RemoveAll(p.Work)
			}
			u.clearState(dir)
		}
		if p.HelperDir != "" {
			os.RemoveAll(p.HelperDir)
		}
		return out, nil
	}
	// Still the old version: the helper has not run yet, or died.
	if time.Since(p.Time) > time.Duration(p.Timeout)*time.Second+2*time.Minute {
		out.Status, out.Reason = StatusRolledBack, "the update was not installed"
		if p.Work != "" {
			os.RemoveAll(p.Work)
		}
		u.clearState(dir)
		return out, nil
	}
	out.Status = StatusPending
	return out, nil
}

// Rollback asks for the previous version to be restored, from a new
// version that has not called Confirm yet (say, a data migration failed).
// Quit after it returns nil: the helper waits for this process to exit,
// puts the previous version back and starts it.
func (u *Updater) Rollback() error {
	dir, err := u.stateDir()
	if err != nil {
		return err
	}
	var p pending
	if err := readJSON(filepath.Join(dir, statePending), &p); err != nil {
		return errors.New("updater: no update to roll back")
	}
	if _, err := os.Stat(filepath.Join(dir, stateConfirmed)); err == nil {
		return errors.New("updater: the update is already confirmed")
	}
	return os.WriteFile(filepath.Join(dir, stateRollback), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func (u *Updater) clearState(dir string) {
	for _, f := range []string{statePending, stateConfirmed, stateRollback, stateRolledBack} {
		os.Remove(filepath.Join(dir, f))
	}
}

// seen is the newest manifest accepted so far.
type seen struct {
	Version string    `json:"version"`
	PubDate time.Time `json:"pub_date"`
	// Expiring records that the manifest had "expires": from then on
	// manifests without it are refused (see Updater.checkExpiry).
	Expiring bool `json:"expiring,omitempty"`
}

// lastSeen returns the newest manifest accepted before, if remembered.
func (u *Updater) lastSeen() (seen, bool) {
	var s seen
	if u.StateDir == "" && u.AppID == "" {
		return s, false
	}
	dir, err := u.stateDir()
	if err != nil {
		return s, false
	}
	if readJSON(filepath.Join(dir, stateSeen), &s) != nil {
		return s, false
	}
	return s, true
}

// checkReplay refuses a manifest older than one accepted before: an
// attacker who serves an old (validly signed) manifest must not be able to
// move the app back to a release it has already moved past.
func (u *Updater) checkReplay(m *Manifest) error {
	// Without a stable place to remember (no StateDir or AppID) only
	// Version, MinVersion and the manifest's expiry apply.
	s, ok := u.lastSeen()
	if !ok {
		return nil
	}
	c, err := CompareVersions(m.Version, s.Version)
	if err != nil {
		return nil
	}
	if c < 0 || c == 0 && m.PubDate.Before(s.PubDate) {
		return fmt.Errorf("%w: manifest %s (%s) is older than %s (%s) seen before", ErrDowngrade,
			m.Version, m.PubDate.Format(time.RFC3339), s.Version, s.PubDate.Format(time.RFC3339))
	}
	return nil
}

func (u *Updater) recordSeen(m *Manifest) {
	if u.StateDir == "" && u.AppID == "" {
		return
	}
	if dir, err := u.stateDir(); err == nil {
		_ = writeJSON(filepath.Join(dir, stateSeen), seen{m.Version, m.PubDate, !m.Expires.IsZero()})
	}
}
