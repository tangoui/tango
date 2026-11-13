// Package updater keeps a Tango desktop app up to date: it fetches a
// signed update manifest, refuses anything older than what it runs (or
// has seen), downloads a binary patch or the full release, verifies it,
// installs it once the app quits, and rolls back if the new version does
// not confirm that it starts.
//
//	u := &updater.Updater{
//		ManifestURL: "https://example.com/notes/latest.json",
//		PublicKey:   "…", // from tan keygen
//		Version:     "1.3.2",
//		AppID:       "com.example.notes",
//		OnEvent:     func(e updater.Event) { … }, // drive your own UI
//	}
//	outcome, _ := u.Confirm()             // early in every launch
//	rel, err := u.Check(ctx)              // nil, nil: up to date
//	p, err := u.Download(ctx, rel, "")    // patch first, full file as fallback
//	err = u.Apply(p)                      // then quit; the new version starts
//
// See docs/updates.md for the manifest format, the security model and
// how each platform installs an update. tan release-manifest builds and
// signs the manifest and patches.
package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"
)

// ErrUnsupported is returned where the app cannot be updated in place
// (unsupported platform, not running from a bundle, AppImage or install
// directory of the manifest's format).
var ErrUnsupported = errors.New("updater: cannot update this installation")

// ErrDowngrade is returned by Check for a manifest older than the running
// version, older than Updater.MinVersion, or older than a manifest it
// accepted before.
var ErrDowngrade = errors.New("updater: refusing to downgrade")

// maxManifest bounds the manifest download.
const maxManifest = 1 << 20

// DefaultConfirmTimeout is how long the helper waits for the new version
// to call Confirm when Updater.ConfirmTimeout is zero.
const DefaultConfirmTimeout = 60 * time.Second

// DefaultClockSkew is how long after its "expires" a manifest is still
// accepted when Updater.ClockSkew is zero.
const DefaultClockSkew = 10 * time.Minute

// ErrExpired is returned by Check for a manifest past its "expires" (or
// without one where Updater.RequireExpiry, or an earlier manifest, asks
// for it).
var ErrExpired = errors.New("updater: manifest expired")

// now is the clock Check compares "expires" with (replaced in tests).
var now = time.Now

// Updater checks for, downloads and installs releases. Set the fields
// before use; an Updater is not safe for concurrent use.
type Updater struct {
	// ManifestURL is where the signed manifest is published.
	ManifestURL string
	// PublicKey verifies the manifest's signature (tan keygen's .pub).
	PublicKey string
	// PublicKeys are further accepted keys, for rotating keys: ship the
	// new key alongside the old one before signing with it.
	PublicKeys []string
	// Version is the running app's semantic version.
	Version string
	// MinVersion, if set, is the oldest release the app accepts: older
	// manifests are refused even if newer than Version (say, releases
	// with a known flaw).
	MinVersion string
	// AppID is the app's id. When set, the manifest's "app" must be
	// present and match, and state is kept under it (see StateDir). Set
	// it whenever more than one app shares a signing key.
	AppID string
	// RequireExpiry refuses manifests without "expires". When false (the
	// default, so manifests signed before expiry existed keep working), a
	// manifest without it is accepted until the app has once accepted one
	// with it (remembered with StateDir or AppID); an expired manifest is
	// always refused. Turn it on once every published manifest carries
	// expires (tan release-manifest and tan sign-update write it).
	RequireExpiry bool
	// ClockSkew is how far past "expires" a manifest is still accepted,
	// for clocks running fast; DefaultClockSkew if zero, none if negative.
	ClockSkew time.Duration
	// Platform selects the manifest entry; runtime.GOOS-runtime.GOARCH if
	// empty.
	Platform string
	// Client makes the requests; http.DefaultClient if nil.
	Client *http.Client

	// Target is the installation to update: the .app bundle (macOS), the
	// executable (Windows), the AppImage or the install directory (Linux).
	// If empty it is found from the running executable (and $APPIMAGE).
	Target string
	// StateDir keeps the update state (pending update, confirmation, the
	// newest manifest seen). Default: <user config dir>/tango-updater/
	// <AppID or executable name>. Remembering the newest manifest (which
	// stops replays of older ones) needs StateDir or AppID set.
	StateDir string
	// ConfirmTimeout is how long after relaunching the helper waits for
	// the new version's Confirm before rolling back; DefaultConfirmTimeout
	// if zero.
	ConfirmTimeout time.Duration
	// InstallerArgs are extra arguments for FormatNSIS and FormatMSI
	// installers (after /S, or after /i <file> /qn).
	InstallerArgs []string

	// OnEvent, if set, receives progress events for the app's own UI. It
	// is called on the goroutine running Check, Download or Apply.
	OnEvent func(Event)
}

// EventKind names an Event.
type EventKind string

const (
	EventChecking    EventKind = "checking"     // Check started
	EventAvailable   EventKind = "available"    // a newer release exists (Release set)
	EventUpToDate    EventKind = "up-to-date"   // no newer release
	EventDownloading EventKind = "downloading"  // a download started (Patch: is it the patch)
	EventProgress    EventKind = "progress"     // Done of Total bytes (Total -1 if unknown)
	EventPatchFailed EventKind = "patch-failed" // the patch failed (Error); falling back to the full file
	EventVerified    EventKind = "verified"     // the release is downloaded and verified
	EventReady       EventKind = "ready"        // Apply staged the update: quit to restart into it
	EventError       EventKind = "error"        // a step failed (Error)
)

// Event reports progress to the app's update UI.
type Event struct {
	Kind    EventKind `json:"kind"`
	Release *Release  `json:"release,omitempty"`
	Patch   bool      `json:"patch,omitempty"`
	Done    int64     `json:"done,omitempty"`
	Total   int64     `json:"total,omitempty"`
	Error   string    `json:"error,omitempty"`
}

func (u *Updater) emit(e Event) {
	if u.OnEvent != nil {
		u.OnEvent(e)
	}
}

func (u *Updater) fail(err error) error {
	if err != nil {
		u.emit(Event{Kind: EventError, Error: err.Error()})
	}
	return err
}

// Release is a newer release found by Check, with this platform's files.
type Release struct {
	Version    string    `json:"version"`
	Notes      string    `json:"notes,omitempty"`
	PubDate    time.Time `json:"pubDate"`
	Format     string    `json:"format"`
	Full       File      `json:"full"`            // URL absolute
	TreeSHA256 string    `json:"-"`               // formats app and dir
	Patch      *File     `json:"patch,omitempty"` // from the running version, if published
}

// Payload is a downloaded, verified release ready for Apply.
type Payload struct {
	Release *Release
	// Path is the full file (an archive, AppImage, executable or
	// installer) or, after a tree patch, the patched directory.
	Path string
	// Patched reports whether Path was made from a patch.
	Patched bool
}

func (u *Updater) keys() []string {
	return append([]string{u.PublicKey}, u.PublicKeys...)
}

// Check fetches the manifest, verifies its signature and returns the
// release it describes if it is newer than u.Version, or nil, nil if it
// is the same version. Manifests older than the running version, older
// than MinVersion or older than one accepted before fail with
// ErrDowngrade; expired manifests (see Updater.RequireExpiry) fail with
// ErrExpired; a newer release without a file for this platform is an
// error.
func (u *Updater) Check(ctx context.Context) (*Release, error) {
	u.emit(Event{Kind: EventChecking})
	rel, err := u.check(ctx)
	if err != nil {
		return nil, u.fail(err)
	}
	if rel == nil {
		u.emit(Event{Kind: EventUpToDate})
	} else {
		u.emit(Event{Kind: EventAvailable, Release: rel})
	}
	return rel, nil
}

func (u *Updater) check(ctx context.Context) (*Release, error) {
	if _, err := parsePublicKey(u.PublicKey); err != nil {
		return nil, err
	}
	if _, err := parseVersion(u.Version); err != nil {
		return nil, fmt.Errorf("updater: current version: %w", err)
	}
	base, err := url.Parse(u.ManifestURL)
	if err != nil {
		return nil, fmt.Errorf("updater: manifest URL: %w", err)
	}
	body, _, err := u.open(ctx, u.ManifestURL)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(body, maxManifest+1))
	body.Close()
	if err != nil {
		return nil, fmt.Errorf("updater: manifest: %w", err)
	}
	if len(b) > maxManifest {
		return nil, errors.New("updater: manifest too large")
	}
	m, err := VerifyManifest(b, u.keys()...)
	if err != nil {
		return nil, err
	}
	if u.AppID != "" && m.App != u.AppID {
		if m.App == "" {
			return nil, fmt.Errorf("updater: manifest names no app; want %q", u.AppID)
		}
		return nil, fmt.Errorf("updater: manifest is for %q, not %q", m.App, u.AppID)
	}
	if err := u.checkExpiry(m); err != nil {
		return nil, err
	}
	if u.MinVersion != "" {
		c, err := CompareVersions(m.Version, u.MinVersion)
		if err != nil {
			return nil, fmt.Errorf("updater: MinVersion: %w", err)
		}
		if c < 0 {
			return nil, fmt.Errorf("%w: manifest %s is older than the minimum %s", ErrDowngrade, m.Version, u.MinVersion)
		}
	}
	if err := u.checkReplay(m); err != nil {
		return nil, err
	}
	newer, _ := CompareVersions(m.Version, u.Version)
	if newer < 0 {
		return nil, fmt.Errorf("%w: manifest %s is older than the running %s", ErrDowngrade, m.Version, u.Version)
	}
	u.recordSeen(m)
	if newer == 0 {
		return nil, nil
	}
	platform := u.platform()
	a, ok := m.Platforms[platform]
	if !ok {
		return nil, fmt.Errorf("updater: release %s has no file for %s", m.Version, platform)
	}
	rel := &Release{Version: m.Version, Notes: m.Notes, PubDate: m.PubDate, Format: a.Format, TreeSHA256: a.TreeSHA256}
	if rel.Full, err = resolve(base, a.File); err != nil {
		return nil, err
	}
	for _, p := range a.Patches {
		if c, _ := CompareVersions(p.From, u.Version); c == 0 {
			f, err := resolve(base, p.File)
			if err != nil {
				return nil, err
			}
			rel.Patch = &f
			break
		}
	}
	return rel, nil
}

func resolve(base *url.URL, f File) (File, error) {
	ref, err := url.Parse(f.URL)
	if err != nil {
		return f, fmt.Errorf("updater: file URL: %w", err)
	}
	abs := base.ResolveReference(ref)
	if abs.Scheme != "https" && abs.Scheme != "http" {
		return f, fmt.Errorf("updater: file URL %q is not http(s)", abs)
	}
	f.URL = abs.String()
	return f, nil
}

// Download fetches rel into dir (a new temporary directory if empty) and
// verifies it. If rel has a patch from the running version and the
// installation is intact, the patch is downloaded and applied to it, and
// the result checked against the full release's digest; if any of that
// fails (EventPatchFailed), the full file is downloaded instead.
func (u *Updater) Download(ctx context.Context, rel *Release, dir string) (*Payload, error) {
	p, err := u.download(ctx, rel, dir)
	if err != nil {
		return nil, u.fail(err)
	}
	u.emit(Event{Kind: EventVerified, Release: rel, Patch: p.Patched})
	return p, nil
}

func (u *Updater) download(ctx context.Context, rel *Release, dir string) (*Payload, error) {
	if rel == nil {
		return nil, errors.New("updater: nil release")
	}
	var err error
	if dir == "" {
		if dir, err = os.MkdirTemp("", "tango-update-"); err != nil {
			return nil, fmt.Errorf("updater: %w", err)
		}
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("updater: %w", err)
	}
	if rel.Patch != nil && patchable(rel.Format) {
		p, err := u.downloadPatch(ctx, rel, dir)
		if err == nil {
			return p, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		u.emit(Event{Kind: EventPatchFailed, Release: rel, Error: err.Error()})
	}
	u.emit(Event{Kind: EventDownloading, Release: rel})
	path, err := u.fetch(ctx, rel.Full, dir)
	if err != nil {
		return nil, err
	}
	return &Payload{Release: rel, Path: path}, nil
}

// downloadPatch fetches rel's patch and applies it to the installation.
func (u *Updater) downloadPatch(ctx context.Context, rel *Release, dir string) (*Payload, error) {
	target, err := u.target(rel.Format)
	if err != nil {
		return nil, err
	}
	u.emit(Event{Kind: EventDownloading, Release: rel, Patch: true})
	pf, err := u.fetch(ctx, *rel.Patch, dir)
	if err != nil {
		return nil, err
	}
	defer os.Remove(pf)
	if treeFormat(rel.Format) {
		f, err := os.Open(pf)
		if err != nil {
			return nil, fmt.Errorf("updater: %w", err)
		}
		defer f.Close()
		out := filepath.Join(dir, filepath.Base(target))
		os.RemoveAll(out)
		digest, err := PatchTree(target, f, out)
		if err != nil {
			return nil, err
		}
		if digest != rel.TreeSHA256 {
			os.RemoveAll(out)
			return nil, errors.New("updater: patched tree does not match the release")
		}
		return &Payload{Release: rel, Path: out, Patched: true}, nil
	}
	old, err := readAllFile(target)
	if err != nil {
		return nil, err
	}
	patch, err := readAllFile(pf)
	if err != nil {
		return nil, err
	}
	b, err := PatchMax(old, patch, rel.Full.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != rel.Full.Size {
		return nil, errors.New("updater: patched file does not match the release")
	}
	if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != rel.Full.SHA256 {
		return nil, errors.New("updater: patched file does not match the release")
	}
	name, err := fileName(rel.Full.URL)
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, name)
	if err := os.WriteFile(out, b, 0o755); err != nil {
		return nil, fmt.Errorf("updater: %w", err)
	}
	return &Payload{Release: rel, Path: out, Patched: true}, nil
}

// fetch downloads f into dir under its URL's file name, checking its size
// and digest.
func (u *Updater) fetch(ctx context.Context, f File, dir string) (string, error) {
	want, err := hex.DecodeString(f.SHA256)
	if err != nil || len(want) != sha256.Size {
		return "", errors.New("updater: malformed sha256")
	}
	name, err := fileName(f.URL)
	if err != nil {
		return "", err
	}
	body, total, err := u.open(ctx, f.URL)
	if err != nil {
		return "", err
	}
	defer body.Close()
	if total < 0 {
		total = f.Size
	}
	out, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	tmp := out.Name()
	ok := false
	defer func() {
		if !ok {
			out.Close()
			os.Remove(tmp)
		}
	}()
	h := sha256.New()
	// Read at most one byte more than announced, to catch an endless or
	// oversized response without trusting the server.
	src := io.LimitReader(body, f.Size+1)
	pr := &progressReader{r: src, total: total, fn: func(done, total int64) {
		u.emit(Event{Kind: EventProgress, Done: done, Total: total})
	}}
	n, err := io.Copy(io.MultiWriter(out, h), pr)
	if err != nil {
		return "", fmt.Errorf("updater: download: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	if n != f.Size {
		return "", fmt.Errorf("updater: %s: got %d bytes, want %d", name, n, f.Size)
	}
	if digest := h.Sum(nil); !bytes.Equal(digest, want) {
		return "", fmt.Errorf("updater: %s: sha256 mismatch: got %x", name, digest)
	}
	final := filepath.Join(dir, name)
	if err := os.Rename(tmp, final); err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	ok = true
	return final, nil
}

// Update checks, downloads and applies in one go, and returns the release
// it staged (nil if up to date). Quit the app after it returns a release.
func (u *Updater) Update(ctx context.Context) (*Release, error) {
	rel, err := u.Check(ctx)
	if err != nil || rel == nil {
		return nil, err
	}
	p, err := u.Download(ctx, rel, "")
	if err != nil {
		return nil, err
	}
	if err := u.Apply(p); err != nil {
		return nil, err
	}
	return rel, nil
}

func (u *Updater) platform() string {
	if u.Platform != "" {
		return u.Platform
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

func (u *Updater) confirmTimeout() time.Duration {
	if u.ConfirmTimeout > 0 {
		return u.ConfirmTimeout
	}
	return DefaultConfirmTimeout
}

func (u *Updater) clockSkew() time.Duration {
	switch {
	case u.ClockSkew > 0:
		return u.ClockSkew
	case u.ClockSkew < 0:
		return 0
	}
	return DefaultClockSkew
}

// checkExpiry refuses an expired manifest, and one without "expires" when
// RequireExpiry is set or an expiring manifest was accepted before (so a
// replayed manifest from before expiry existed cannot sidestep it).
func (u *Updater) checkExpiry(m *Manifest) error {
	if m.Expires.IsZero() {
		if u.RequireExpiry {
			return fmt.Errorf("%w: manifest %s has no expiry (Updater.RequireExpiry)", ErrExpired, m.Version)
		}
		if s, ok := u.lastSeen(); ok && s.Expiring {
			return fmt.Errorf("%w: manifest %s has no expiry, but earlier ones had", ErrExpired, m.Version)
		}
		return nil
	}
	if t := now(); t.After(m.Expires.Add(u.clockSkew())) {
		return fmt.Errorf("%w: manifest %s expired at %s (now %s)", ErrExpired, m.Version,
			m.Expires.UTC().Format(time.RFC3339), t.UTC().Format(time.RFC3339))
	}
	return nil
}

func (u *Updater) client() *http.Client {
	if u.Client != nil {
		return u.Client
	}
	return http.DefaultClient
}

func (u *Updater) open(ctx context.Context, rawURL string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("updater: %w", err)
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("updater: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, 0, fmt.Errorf("updater: GET %s: %s", rawURL, resp.Status)
	}
	return resp.Body, resp.ContentLength, nil
}

// fileName returns a safe local name for the file at rawURL.
func fileName(rawURL string) (string, error) {
	p, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	name := path.Base(p.Path)
	if name == "." || name == "/" || name == ".." || !filepath.IsLocal(name) {
		return "", fmt.Errorf("updater: no file name in %q", rawURL)
	}
	return name, nil
}

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	last  time.Time
	fn    func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		// At most ~20 events a second, plus the last one.
		if now := time.Now(); now.Sub(p.last) >= 50*time.Millisecond || p.done == p.total {
			p.last = now
			p.fn(p.done, p.total)
		}
	}
	return n, err
}
