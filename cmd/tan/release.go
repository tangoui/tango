package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// tan release-manifest turns the output of tan build / tan package into a
// release for updater.Updater: the update files, binary patches from
// earlier releases, and the signed manifest (latest.json) — everything to
// upload to the update server. See docs/updates.md.

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

type releaseOptions struct {
	buildDir string
	outDir   string
	key      string
	baseURL  string
	notes    string
	prev     []string
	windows  string // exe | nsis
	linux    string // appimage | dir | "" (appimage when built)
	pubDate  time.Time
	expires  time.Duration // manifest validity from pubDate; 0: no expires
}

// defaultManifestValidity is how long a manifest from tan release-manifest
// or tan sign-update stays valid (updater.Manifest.Expires): re-sign it
// with tan sign-update before then, even without a new release.
const defaultManifestValidity = 30 * 24 * time.Hour

func runReleaseManifest(args []string) error {
	fs := newFlags("release-manifest", "[flags]")
	dir := projectFlag(fs)
	buildDir := fs.String("build", "build", "`dir` holding the tan build / tan package output")
	outDir := fs.String("o", "release", "output `dir` for the update files, patches and latest.json")
	keyFile := fs.String("key", "", "private update key `file` from tan keygen (default $"+envUpdateKeyFile+", or the key itself in $"+envUpdateKey+")")
	baseURL := fs.String("url", "", "base `URL` the files are served from (default: next to latest.json, as relative URLs)")
	notes := fs.String("notes", "", "release `notes`, or @file to read them from a file")
	var prev stringList
	fs.Var(&prev, "prev", "`dir` of an earlier release (its latest.json and files) to make patches from; repeatable")
	windows := fs.String("windows", updater.FormatExe, "Windows update `format`: exe (replace the executable) or nsis (run the installer from tan package)")
	linux := fs.String("linux", "", "Linux update `format`: appimage or dir (default appimage when an AppImage was built, else dir)")
	expires := fs.Duration("expires", defaultManifestValidity, "how long the manifest stays valid (its \"expires\"), e.g. 720h; 0 writes none")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	cfg, err := LoadConfig(*dir)
	if err != nil {
		return err
	}
	key, err := updateKey(*keyFile)
	if err != nil {
		return err
	}
	o := releaseOptions{buildDir: *buildDir, outDir: *outDir, key: key, baseURL: *baseURL, prev: prev,
		windows: *windows, linux: *linux, pubDate: time.Now().UTC().Truncate(time.Second), expires: *expires}
	if o.expires < 0 {
		return fmt.Errorf("-expires %s: must not be negative", o.expires)
	}
	if o.notes, err = readNotes(*notes); err != nil {
		return err
	}
	if !filepath.IsAbs(o.buildDir) {
		o.buildDir = cfg.Path(o.buildDir)
	}
	if !filepath.IsAbs(o.outDir) {
		o.outDir = cfg.Path(o.outDir)
	}
	_, err = makeRelease(cfg, o)
	return err
}

func readNotes(s string) (string, error) {
	if f, ok := strings.CutPrefix(s, "@"); ok {
		b, err := os.ReadFile(f)
		return strings.TrimSpace(string(b)), err
	}
	return s, nil
}

// makeRelease writes the release into o.outDir and returns its manifest.
func makeRelease(cfg *Config, o releaseOptions) (*updater.Manifest, error) {
	if o.windows != updater.FormatExe && o.windows != updater.FormatNSIS {
		return nil, fmt.Errorf("-windows %q: want exe or nsis", o.windows)
	}
	if o.linux != "" && o.linux != updater.FormatAppImage && o.linux != updater.FormatDir {
		return nil, fmt.Errorf("-linux %q: want appimage or dir", o.linux)
	}
	if err := os.MkdirAll(o.outDir, 0o755); err != nil {
		return nil, err
	}
	pub, err := updater.PublicKeyOf(o.key)
	if err != nil {
		return nil, err
	}
	m := &updater.Manifest{App: cfg.ID, Version: cfg.Version, PubDate: o.pubDate, Notes: o.notes, Platforms: map[string]updater.Asset{}}
	if o.expires > 0 {
		m.Expires = o.pubDate.Add(o.expires)
	}

	entries, err := os.ReadDir(o.buildDir)
	if err != nil {
		return nil, fmt.Errorf("no build output (run tan build or tan package first): %w", err)
	}
	for _, e := range entries {
		goos, arch, ok := strings.Cut(e.Name(), "-")
		if !e.IsDir() || !ok || !slices.Contains([]string{"darwin", "windows", "linux"}, goos) {
			continue
		}
		t := target{goos, arch}
		a, local, err := releaseFile(cfg, o, t, filepath.Join(o.buildDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		if a == nil {
			continue
		}
		key := goos + "-" + arch
		for _, p := range o.prev {
			pa, err := releasePatch(cfg, o, pub, p, key, *a, local)
			if err != nil {
				return nil, fmt.Errorf("%s: patch from %s: %w", t, p, err)
			}
			if pa != nil {
				a.Patches = append(a.Patches, *pa)
			}
		}
		m.Platforms[key] = *a
		logf("%s: %s %s, %s", key, a.Format, filepath.Base(local), size(a.Size))
		for _, p := range a.Patches {
			logf("  patch from %s: %s (%.1f%% of the full file)", p.From, size(p.Size), 100*float64(p.Size)/float64(a.Size))
		}
	}
	if len(m.Platforms) == 0 {
		return nil, fmt.Errorf("no desktop builds in %s", o.buildDir)
	}
	env, err := updater.SignManifest(o.key, m)
	if err != nil {
		return nil, err
	}
	out := filepath.Join(o.outDir, "latest.json")
	if err := os.WriteFile(out, env, 0o644); err != nil {
		return nil, err
	}
	logf("Wrote %s (version %s, signed; public key %s)", out, m.Version, pub)
	return m, nil
}

func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// releaseFile copies (or packs) one target's update file into the output
// and returns its manifest entry and local path; nil when the target has
// nothing to publish.
func releaseFile(cfg *Config, o releaseOptions, t target, dir string) (*updater.Asset, string, error) {
	base := artifactBase(cfg, t)
	parent := filepath.Dir(dir)
	a := &updater.Asset{}
	var src, name string
	switch t.OS {
	case "darwin":
		apps, _ := filepath.Glob(filepath.Join(dir, "*.app"))
		if len(apps) != 1 {
			return nil, "", fmt.Errorf("want one .app in %s, found %d", dir, len(apps))
		}
		a.Format, name = updater.FormatApp, base+".app.tar.gz"
		if err := updater.PackTree(apps[0], filepath.Join(o.outDir, name)); err != nil {
			return nil, "", err
		}
	case "windows":
		a.Format = o.windows
		if o.windows == updater.FormatNSIS {
			src, name = filepath.Join(parent, base+"-setup.exe"), base+"-setup.exe"
		} else {
			src, name = filepath.Join(dir, cfg.Exe()+".exe"), base+".exe"
		}
	case "linux":
		img := filepath.Join(parent, base+".AppImage")
		format := o.linux
		if format == "" {
			format = updater.FormatDir
			if _, err := os.Stat(img); err == nil {
				format = updater.FormatAppImage
			}
		}
		a.Format = format
		if format == updater.FormatAppImage {
			src, name = img, base+".AppImage"
			break
		}
		name = base + ".tar.gz"
		if tgz := filepath.Join(parent, base+".tar.gz"); fileExists(tgz) {
			src = tgz // tan package's tarball
		} else if err := updater.PackTree(dir, filepath.Join(o.outDir, name)); err != nil {
			return nil, "", err
		}
	default:
		return nil, "", nil
	}
	local := filepath.Join(o.outDir, name)
	if src != "" {
		if !fileExists(src) {
			hint := "run tan build"
			if strings.HasSuffix(src, "-setup.exe") || strings.HasSuffix(src, ".AppImage") || strings.HasSuffix(src, ".tar.gz") {
				hint = "run tan package"
			}
			return nil, "", fmt.Errorf("%s is missing (%s first)", src, hint)
		}
		if err := copyFile(src, local, 0o755); err != nil {
			return nil, "", err
		}
	}
	f, err := updater.FileInfo(local, fileURL(o.baseURL, name))
	if err != nil {
		return nil, "", err
	}
	a.File = f
	if a.Format == updater.FormatApp || a.Format == updater.FormatDir {
		tmp, err := os.MkdirTemp("", "tango-release-*")
		if err != nil {
			return nil, "", err
		}
		defer os.RemoveAll(tmp)
		top, err := updater.Unpack(local, tmp)
		if err != nil {
			return nil, "", err
		}
		if a.TreeSHA256, err = updater.TreeDigest(top); err != nil {
			return nil, "", err
		}
	}
	return a, local, nil
}

// releasePatch makes a patch from the release in prevDir to the new file
// for platform key; nil when the earlier release has no usable file for it
// or the patch would not save much.
func releasePatch(cfg *Config, o releaseOptions, pub, prevDir, key string, a updater.Asset, local string) (*updater.PatchAsset, error) {
	if a.Format != updater.FormatApp && a.Format != updater.FormatDir && a.Format != updater.FormatExe && a.Format != updater.FormatAppImage {
		return nil, nil
	}
	env, err := os.ReadFile(filepath.Join(prevDir, "latest.json"))
	if err != nil {
		return nil, err
	}
	prev, err := updater.VerifyManifest(env, pub)
	if err != nil {
		return nil, fmt.Errorf("its latest.json: %w (signed with another key?)", err)
	}
	if c, err := updater.CompareVersions(prev.Version, cfg.Version); err != nil || c >= 0 {
		return nil, fmt.Errorf("its version %s is not older than %s", prev.Version, cfg.Version)
	}
	pa, ok := prev.Platforms[key]
	if !ok || pa.Format != a.Format {
		warnf("%s: release %s has no %s file; no patch", key, prev.Version, a.Format)
		return nil, nil
	}
	old := filepath.Join(prevDir, filepath.Base(pa.URL))
	if f, err := updater.FileInfo(old, ""); err != nil || f.SHA256 != pa.SHA256 {
		return nil, fmt.Errorf("%s is missing or differs from its manifest", old)
	}
	name := fmt.Sprintf("%s-%s-to-%s-%s.patch", cfg.Exe(), prev.Version, cfg.Version, key)
	out := filepath.Join(o.outDir, name)
	if a.Format == updater.FormatApp || a.Format == updater.FormatDir {
		err = treePatch(old, local, out)
	} else {
		err = filePatch(old, local, out)
	}
	if err != nil {
		return nil, err
	}
	f, err := updater.FileInfo(out, fileURL(o.baseURL, name))
	if err != nil {
		return nil, err
	}
	if f.Size >= a.Size*4/5 {
		os.Remove(out)
		warnf("%s: the patch from %s is %s, close to the full %s; not publishing it", key, prev.Version, size(f.Size), size(a.Size))
		return nil, nil
	}
	return &updater.PatchAsset{From: prev.Version, File: f}, nil
}

func filePatch(oldFile, newFile, out string) error {
	old, err := os.ReadFile(oldFile)
	if err != nil {
		return err
	}
	nw, err := os.ReadFile(newFile)
	if err != nil {
		return err
	}
	p, err := updater.Diff(old, nw)
	if err != nil {
		return err
	}
	return os.WriteFile(out, p, 0o644)
}

// treePatch diffs the trees of two archives, unpacked as the updater will
// find them installed.
func treePatch(oldArchive, newArchive, out string) error {
	tmp, err := os.MkdirTemp("", "tango-release-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var tops [2]string
	for i, a := range []string{oldArchive, newArchive} {
		d := filepath.Join(tmp, fmt.Sprint(i))
		if err := os.Mkdir(d, 0o755); err != nil {
			return err
		}
		if tops[i], err = updater.Unpack(a, d); err != nil {
			return err
		}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	err = updater.DiffTree(tops[0], tops[1], f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func fileURL(base, name string) string {
	if base == "" {
		return name
	}
	return strings.TrimSuffix(base, "/") + "/" + name
}
