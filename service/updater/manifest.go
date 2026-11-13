package updater

import (
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Manifest describes the latest release. It is published inside a signed
// envelope (see SignManifest), so every URL and digest in it is covered by
// the release key:
//
//	{
//	  "manifest": {
//	    "app": "com.example.notes",
//	    "version": "1.4.0",
//	    "pub_date": "2026-11-01T12:00:00Z",
//	    "expires": "2026-12-01T12:00:00Z",
//	    "notes": "Faster startup.",
//	    "platforms": {
//	      "darwin-arm64": {
//	        "format": "app",
//	        "url": "Notes-1.4.0-darwin-arm64.app.tar.gz", "size": 9123456, "sha256": "…",
//	        "tree_sha256": "…",
//	        "patches": [{"from": "1.3.0", "url": "Notes-1.3.0-to-1.4.0-darwin-arm64.patch", "size": 81234, "sha256": "…"}]
//	      },
//	      "windows-amd64": {"format": "exe", "url": "Notes-1.4.0-windows-amd64.exe", "size": 8000000, "sha256": "…"},
//	      "linux-amd64":   {"format": "appimage", "url": "Notes-1.4.0-linux-amd64.AppImage", "size": 9000000, "sha256": "…"}
//	    }
//	  },
//	  "signature": "…"
//	}
//
// Platform keys are GOOS-GOARCH. A relative url is resolved against the
// manifest's URL. sha256 is a file's hex digest; tree_sha256 is the
// TreeDigest of the unpacked directory for the directory formats.
type Manifest struct {
	// App is the application id. When Updater.AppID is set the manifest
	// must name the same app, so one app's manifest cannot be served to
	// another app signed with the same key.
	App     string    `json:"app,omitempty"`
	Version string    `json:"version"`
	PubDate time.Time `json:"pub_date"`
	// Expires, if set, is when the manifest stops being valid (RFC 3339,
	// covered by the signature like every other field). Updater.Check
	// refuses an expired manifest, which bounds how long an attacker can
	// replay an old one to hold the app back. Re-sign the manifest (tan
	// sign-update) before it expires, even without a new release.
	Expires time.Time `json:"expires,omitzero"`
	Notes   string    `json:"notes,omitempty"`
	// Platforms maps GOOS-GOARCH to that platform's files.
	Platforms map[string]Asset `json:"platforms"`
}

// Formats of update files (Asset.Format).
const (
	// FormatApp: a .zip or .tar.gz holding one macOS .app bundle, which
	// replaces the running bundle.
	FormatApp = "app"
	// FormatDir: a .tar.gz or .zip holding one directory, whose contents
	// replace the Linux install directory the executable runs from.
	FormatDir = "dir"
	// FormatAppImage: a Linux AppImage that replaces $APPIMAGE.
	FormatAppImage = "appimage"
	// FormatExe: a Windows executable that replaces the running one.
	FormatExe = "exe"
	// FormatNSIS: a Windows NSIS installer, run silently (/S).
	FormatNSIS = "nsis"
	// FormatMSI: a Windows Installer package, run with msiexec /qn.
	FormatMSI = "msi"
)

// treeFormat reports whether format installs a directory tree.
func treeFormat(format string) bool { return format == FormatApp || format == FormatDir }

// patchable reports whether files of format may have patches: the client
// must hold the old version's file or tree, which installers do not leave.
func patchable(format string) bool {
	return treeFormat(format) || format == FormatExe || format == FormatAppImage
}

// Asset is one platform's update: the full file and patches from earlier
// versions.
type Asset struct {
	Format string `json:"format"`
	File
	// TreeSHA256 is the TreeDigest of the unpacked directory (formats app
	// and dir), which a tree patch must reproduce.
	TreeSHA256 string       `json:"tree_sha256,omitempty"`
	Patches    []PatchAsset `json:"patches,omitempty"`
}

// File is a downloadable file.
type File struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// PatchAsset turns version From's installation into this release: a binary
// patch (Diff) of the file for formats exe and appimage, a tree patch
// (DiffTree) for app and dir.
type PatchAsset struct {
	From string `json:"from"`
	File
}

var formats = []string{FormatApp, FormatDir, FormatAppImage, FormatExe, FormatNSIS, FormatMSI}

// Validate checks that m is complete and well formed.
func (m *Manifest) Validate() error {
	if _, err := parseVersion(m.Version); err != nil {
		return fmt.Errorf("updater: manifest: %w", err)
	}
	if m.PubDate.IsZero() {
		return errors.New("updater: manifest: pub_date is required")
	}
	if !m.Expires.IsZero() && !m.Expires.After(m.PubDate) {
		return errors.New("updater: manifest: expires must be after pub_date")
	}
	if len(m.Platforms) == 0 {
		return errors.New("updater: manifest: no platforms")
	}
	for p, a := range m.Platforms {
		if !slices.Contains(formats, a.Format) {
			return fmt.Errorf("updater: manifest: platform %q has unknown format %q", p, a.Format)
		}
		if err := a.File.validate(); err != nil {
			return fmt.Errorf("updater: manifest: platform %q: %w", p, err)
		}
		if treeFormat(a.Format) && !isHexDigest(a.TreeSHA256) {
			return fmt.Errorf("updater: manifest: platform %q needs tree_sha256", p)
		}
		if len(a.Patches) > 0 && !patchable(a.Format) {
			return fmt.Errorf("updater: manifest: platform %q: format %s cannot have patches", p, a.Format)
		}
		for _, pt := range a.Patches {
			if _, err := parseVersion(pt.From); err != nil {
				return fmt.Errorf("updater: manifest: platform %q patch: %w", p, err)
			}
			if err := pt.File.validate(); err != nil {
				return fmt.Errorf("updater: manifest: platform %q patch from %s: %w", p, pt.From, err)
			}
		}
	}
	return nil
}

func (f File) validate() error {
	if f.URL == "" || !isHexDigest(f.SHA256) || f.Size < 0 {
		return errors.New("needs url, size and sha256")
	}
	return nil
}

func isHexDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32
}

// version is a parsed semantic version (https://semver.org). Missing minor
// or patch numbers count as 0; build metadata is ignored.
type version struct {
	core [3]uint64
	pre  []string
}

func parseVersion(s string) (version, error) {
	var v version
	orig := s
	s = strings.TrimPrefix(s, "v")
	s, _, _ = strings.Cut(s, "+")
	s, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if s == "" || len(parts) > 3 {
		return v, fmt.Errorf("bad version %q", orig)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return v, fmt.Errorf("bad version %q", orig)
		}
		v.core[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return v, fmt.Errorf("bad version %q", orig)
			}
		}
	}
	return v, nil
}

// CompareVersions returns -1, 0 or +1 as semantic version a is older than,
// the same as or newer than b. A leading "v" is allowed.
func CompareVersions(a, b string) (int, error) {
	va, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	return va.compare(vb), nil
}

func (a version) compare(b version) int {
	for i := range a.core {
		if c := cmp.Compare(a.core[i], b.core[i]); c != 0 {
			return c
		}
	}
	// A release is newer than its pre-releases.
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePre(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

// comparePre orders pre-release identifiers: numeric ones numerically and
// before alphanumeric ones, which compare as strings.
func comparePre(a, b string) int {
	na, errA := strconv.ParseUint(a, 10, 64)
	nb, errB := strconv.ParseUint(b, 10, 64)
	switch {
	case errA == nil && errB == nil:
		return cmp.Compare(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}
