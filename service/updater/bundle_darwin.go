package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// prepareBundle readies a staged .app bundle: it removes the quarantine
// attribute, so Gatekeeper does not stop the relaunch with a "downloaded
// from the Internet" prompt (the update was verified against the signed
// manifest instead), and checks the bundle's code signature: when the
// running app is signed, the new bundle must verify, and when the running
// app is signed with a Developer ID (a Team ID), the new one must carry
// the same Team ID. The bundle's files, including _CodeSignature and the
// signed executables, are moved, never rewritten, so its signature stays
// valid.
func prepareBundle(bundle, running string) error {
	stripQuarantine(bundle)
	return checkSignature(bundle, running)
}

const quarantineAttr = "com.apple.quarantine"

func stripQuarantine(path string) {
	if _, err := os.Stat("/usr/bin/xattr"); err != nil {
		return
	}
	// xattr exits non-zero when some file lacks the attribute; the result
	// is checked by hasQuarantine in tests instead.
	_ = exec.Command("/usr/bin/xattr", "-r", "-d", quarantineAttr, path).Run()
}

// hasQuarantine reports whether path carries the quarantine attribute.
func hasQuarantine(path string) bool {
	return exec.Command("/usr/bin/xattr", "-p", quarantineAttr, path).Run() == nil
}

type signature struct {
	signed bool
	team   string // "" for ad hoc or no Team ID
}

func codesignInfo(path string) (signature, error) {
	out, err := exec.Command("/usr/bin/codesign", "-d", "--verbose=2", path).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "not signed") {
			return signature{}, nil
		}
		return signature{}, fmt.Errorf("codesign: %v: %s", err, strings.TrimSpace(string(out)))
	}
	s := signature{signed: true}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "TeamIdentifier="); ok && v != "not set" {
			s.team = strings.TrimSpace(v)
		}
	}
	return s, nil
}

func checkSignature(bundle, running string) error {
	if _, err := os.Stat("/usr/bin/codesign"); err != nil {
		return nil
	}
	old, err := codesignInfo(running)
	if err != nil || !old.signed {
		return nil // nothing to hold the new version to
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", bundle).CombinedOutput(); err != nil {
		return fmt.Errorf("updater: the new app's code signature does not verify: %s", strings.TrimSpace(string(out)))
	}
	if old.team == "" {
		return nil
	}
	nw, err := codesignInfo(bundle)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	if nw.team != old.team {
		return errors.New("updater: the new app is signed by Team ID " + orNone(nw.team) + ", not " + old.team)
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
