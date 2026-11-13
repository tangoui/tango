package updater

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---- M5: AppID requires a matching "app" ----

func TestCheckAppIDRequiresApp(t *testing.T) {
	ctx := context.Background()
	s := newServer(t)
	full := s.put("App-1.2.0.exe", []byte("v1.2.0"))
	m := manifest("1.2.0", FormatExe, full)
	m.App = "" // signed by hand without "app", same key as another app
	s.publish(m)

	u := s.updater("1.0.0")
	u.AppID = "com.example.notes"
	if _, err := u.Check(ctx); err == nil || !strings.Contains(err.Error(), "names no app") {
		t.Fatalf("manifest without app accepted: %v", err)
	}
	// Without AppID it is still accepted (as before).
	if rel, err := s.updater("1.0.0").Check(ctx); err != nil || rel == nil {
		t.Fatalf("no AppID: %v, %v", rel, err)
	}
	m.App = "com.example.notes"
	s.publish(m)
	if rel, err := u.Check(ctx); err != nil || rel == nil {
		t.Fatalf("matching app: %v, %v", rel, err)
	}
}

// ---- M6: expires ----

func setNow(t *testing.T, at time.Time) {
	t.Helper()
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
}

func TestCheckExpiry(t *testing.T) {
	ctx := context.Background()
	s := newServer(t)
	full := s.put("App-1.2.0.exe", []byte("v1.2.0"))
	m := manifest("1.2.0", FormatExe, full)
	m.Expires = pubDate.Add(30 * 24 * time.Hour)
	s.publish(m)

	setNow(t, pubDate.Add(24*time.Hour))
	if rel, err := s.updater("1.0.0").Check(ctx); err != nil || rel == nil {
		t.Fatalf("valid manifest: %v, %v", rel, err)
	}
	// Replayed after it expired (same version as the running one: a
	// freeze attempt), even with no StateDir.
	setNow(t, m.Expires.Add(DefaultClockSkew+time.Second))
	u := s.updater("1.2.0")
	u.StateDir = ""
	if _, err := u.Check(ctx); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired manifest: %v", err)
	}
	// Within the clock skew allowance it still passes; ClockSkew tunes it.
	setNow(t, m.Expires.Add(DefaultClockSkew-time.Second))
	if _, err := s.updater("1.0.0").Check(ctx); err != nil {
		t.Fatalf("within skew: %v", err)
	}
	u = s.updater("1.0.0")
	u.ClockSkew = -1
	if _, err := u.Check(ctx); !errors.Is(err, ErrExpired) {
		t.Fatalf("no skew: %v", err)
	}
	u.ClockSkew = time.Hour
	if _, err := u.Check(ctx); err != nil {
		t.Fatalf("hour of skew: %v", err)
	}

	// No expires: accepted by default, refused with RequireExpiry...
	setNow(t, pubDate.Add(time.Hour))
	noExp := manifest("1.3.0", FormatExe, full)
	s.publish(noExp)
	if _, err := s.updater("1.0.0").Check(ctx); err != nil {
		t.Fatalf("no expires, default: %v", err)
	}
	u = s.updater("1.0.0")
	u.RequireExpiry = true
	if _, err := u.Check(ctx); !errors.Is(err, ErrExpired) {
		t.Fatalf("no expires, RequireExpiry: %v", err)
	}
	// ...and refused once an expiring manifest was accepted (a replay of a
	// manifest signed before expiry existed).
	u = s.updater("1.0.0")
	s.publish(m)
	if _, err := u.Check(ctx); err != nil {
		t.Fatal(err)
	}
	newer := manifest("1.4.0", FormatExe, full)
	s.publish(newer)
	if _, err := u.Check(ctx); !errors.Is(err, ErrExpired) {
		t.Fatalf("no expires after an expiring one: %v", err)
	}

	// expires must follow pub_date.
	bad := manifest("1.2.0", FormatExe, full)
	bad.Expires = pubDate
	if _, err := SignManifest(s.priv, bad); err == nil {
		t.Fatal("expires == pub_date signed")
	}
}

func TestExpiresIsSigned(t *testing.T) {
	pub, priv, _ := GenerateKey()
	m := manifest("1.0.0", FormatExe, File{URL: "a.exe", Size: 1, SHA256: strings.Repeat("ab", 32)})
	m.Expires = pubDate.Add(time.Hour)
	env, err := SignManifest(priv, m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(env, []byte(`"expires": "2026-11-01T13:00:00Z"`)) {
		t.Fatalf("expires not written as RFC 3339:\n%s", env)
	}
	tampered := bytes.Replace(env, []byte("2026-11-01T13:00:00Z"), []byte("2099-11-01T13:00:00Z"), 1)
	if _, err := VerifyManifest(tampered, pub); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("extended expires verified: %v", err)
	}
	got, err := VerifyManifest(env, pub)
	if err != nil || !got.Expires.Equal(m.Expires) {
		t.Fatalf("round trip: %v, %v", got, err)
	}
	// Without Expires nothing is written (manifests stay as before).
	m.Expires = time.Time{}
	env, _ = SignManifest(priv, m)
	if bytes.Contains(env, []byte("expires")) {
		t.Fatalf("zero expires written:\n%s", env)
	}
}

// ---- symlink chains ----

func TestExtractRejectsSymlinkChains(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	cases := map[string][]entry{
		// "a" → "." puts a/b at the top, where ".." leaves the root.
		"stacked": {
			{"a", fs.ModeSymlink | 0o777, "."},
			{"a/b", fs.ModeSymlink | 0o777, ".."},
		},
		// Deeper: each level's ".." looks local on its own.
		"stacked deep": {
			{"x/", fs.ModeDir | 0o755, ""},
			{"x/up", fs.ModeSymlink | 0o777, ".."},
			{"x/up/y", fs.ModeSymlink | 0o777, "."},
			{"x/up/y/z", fs.ModeSymlink | 0o777, "../.."},
		},
		// s/l is fine when written; d → "." written later redirects it.
		"redirected later": {
			{"s/", fs.ModeDir | 0o755, ""},
			{"s/l", fs.ModeSymlink | 0o777, "../d/.."},
			{"d", fs.ModeSymlink | 0o777, "."},
		},
		"loop": {
			{"p", fs.ModeSymlink | 0o777, "q"},
			{"q", fs.ModeSymlink | 0o777, "p/.."},
		},
	}
	for name, entries := range cases {
		for _, kind := range []string{"zip", "tar.gz"} {
			dir := t.TempDir()
			archive := filepath.Join(dir, "a."+kind)
			if kind == "zip" {
				makeZip(t, archive, entries)
			} else {
				makeTarGz(t, archive, entries)
			}
			dst := filepath.Join(dir, "out", "inner")
			os.MkdirAll(dst, 0o755)
			if err := extract(archive, dst); err == nil {
				t.Errorf("%s (%s): extracted", name, kind)
			}
		}
	}
	// Chains that stay inside are fine.
	dir := t.TempDir()
	archive := filepath.Join(dir, "ok.tar.gz")
	makeTarGz(t, archive, []entry{
		{"top/lib/real", 0o644, "x"},
		{"top/cur", fs.ModeSymlink | 0o777, "lib"},
		{"top/bin/l", fs.ModeSymlink | 0o777, "../cur/real"},
		{"top/bin/self", fs.ModeSymlink | 0o777, "."},
		{"top/bin/self/up", fs.ModeSymlink | 0o777, "../lib"},
	})
	dst := filepath.Join(dir, "out")
	os.Mkdir(dst, 0o755)
	if err := extract(archive, dst); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "top/bin/l")); err != nil || string(b) != "x" {
		t.Fatalf("inside chain: %q, %v", b, err)
	}
}

func TestPatchTreeRejectsSymlinkChains(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	old := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	os.Mkdir(outDir, 0o755)
	oldRoot, _ := os.OpenRoot(old)
	defer oldRoot.Close()
	root, _ := os.OpenRoot(outDir)
	defer root.Close()
	if err := patchEntry(oldRoot, root, treeEntry{Path: "a", Type: "symlink", Link: "."}, nil); err != nil {
		t.Fatal(err)
	}
	if err := patchEntry(oldRoot, root, treeEntry{Path: "a/b", Type: "symlink", Link: ".."}, nil); err == nil {
		t.Fatal("stacked symlink accepted")
	}
	// The whole-tree check catches a link redirected after it was written.
	os.Remove(filepath.Join(outDir, "a"))
	os.Mkdir(filepath.Join(outDir, "s"), 0o755)
	os.Symlink("../d/..", filepath.Join(outDir, "s", "l"))
	os.Symlink(".", filepath.Join(outDir, "d"))
	if err := checkSymlinks(outDir, root); err == nil {
		t.Fatal("redirected symlink accepted")
	}
}

// ---- malformed binary patches ----

// rawPatch assembles a patch from uncompressed streams.
func rawPatch(old []byte, newSize uint64, ctrl, dif, extra []byte) []byte {
	b := make([]byte, patchHeader)
	copy(b, patchMagic)
	binary.LittleEndian.PutUint64(b[8:], uint64(len(old)))
	binary.LittleEndian.PutUint64(b[16:], newSize)
	sum := sha256.Sum256(old)
	copy(b[24:], sum[:])
	for i, s := range [][]byte{ctrl, dif, extra} {
		var z bytes.Buffer
		zw, _ := flate.NewWriter(&z, flate.BestSpeed)
		zw.Write(s)
		zw.Close()
		binary.LittleEndian.PutUint64(b[88+8*i:], uint64(z.Len()))
		b = append(b, z.Bytes()...)
	}
	return b
}

func triples(t ...int64) []byte {
	var b []byte
	for _, v := range t {
		b = binary.AppendVarint(b, v)
	}
	return b
}

func TestPatchMalformedNoPanic(t *testing.T) {
	old := []byte("0123456789abcdef")
	cases := map[string][]byte{
		// oldPos += MaxInt64, then add 1: oldPos+add wrapped negative and
		// passed the old bounds check, then indexed old out of range.
		"seek overflow": rawPatch(old, 1, triples(0, 0, math.MaxInt64, 1, 0, 0), []byte{1}, nil),
		"seek negative": rawPatch(old, 1, triples(0, 0, -1, 1, 0, 0), []byte{1}, nil),
		"seek past old": rawPatch(old, 1, triples(0, 0, 17, 1, 0, 0), []byte{1}, nil),
		"add past old":  rawPatch(old, 17, triples(17, 0, 0), make([]byte, 17), nil),
		"min add":       rawPatch(old, 1, triples(math.MinInt64, 1, 0), nil, []byte{1}),
		"copy overflow": rawPatch(old, 1, triples(0, math.MaxInt64, 0), nil, []byte{1}),
		"short triple":  rawPatch(old, 1, triples(0, 1), nil, []byte{1}),
		"short output":  rawPatch(old, 2, triples(0, 1, 0), nil, []byte{1, 2}),
		"extra stream":  rawPatch(old, 1, triples(0, 1, 0), nil, []byte{1, 2}),
		"empty triples": rawPatch(old, 0, bytes.Repeat(triples(0, 0, 0), 2<<20), nil, nil),
		// 2 GiB claimed with nothing behind it: refused before allocating.
		"huge output": rawPatch(old, 2<<30, nil, nil, nil),
	}
	for name, p := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic %v", name, r)
				}
			}()
			if _, err := Patch(old, p); err == nil {
				t.Errorf("%s: applied", name)
			}
		}()
	}
}

func TestPatchMax(t *testing.T) {
	old := bytes.Repeat([]byte("old file "), 500)
	newer := append(bytes.Repeat([]byte("old file "), 400), bytes.Repeat([]byte("new! "), 300)...)
	p, err := Diff(old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := PatchMax(old, p, int64(len(newer))); err != nil || !bytes.Equal(got, newer) {
		t.Fatalf("PatchMax(exact) = %v", err)
	}
	if _, err := PatchMax(old, p, int64(len(newer))-1); err == nil {
		t.Fatal("output above maxSize accepted")
	}
	if _, err := PatchMax(old, rawPatch(old, 1<<40, nil, nil, nil), 1<<41); err == nil {
		t.Fatal("output above the hard limit accepted")
	}
}

// ---- stage re-verifies what it installs ----

func TestStageReverifies(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.exe")
	os.WriteFile(good, []byte("the release"), 0o755)
	full, _ := FileInfo(good, "App.exe")

	newTarget := func() string {
		d := t.TempDir()
		p := filepath.Join(d, "App.exe")
		os.WriteFile(p, []byte("old"), 0o755)
		return p
	}
	// A file swapped in the (shared) download directory after Download.
	for _, format := range []string{FormatExe, FormatAppImage, FormatNSIS} {
		swapped := filepath.Join(dir, "swapped-"+format)
		os.WriteFile(swapped, []byte("evil release"), 0o755)
		target := newTarget()
		if _, err := stage(&Payload{Release: &Release{Format: format, Full: full}, Path: swapped}, target); err == nil {
			t.Errorf("%s: swapped file staged", format)
		}
		if left, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".tango-update-*")); len(left) != 0 {
			t.Errorf("%s: work directory left: %v", format, left)
		}
	}
	st, err := stage(&Payload{Release: &Release{Format: FormatExe, Full: full}, Path: good}, newTarget())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(st.repl); string(b) != "the release" {
		t.Fatalf("staged %q", b)
	}
	os.RemoveAll(st.work)

	// A patched tree changed after PatchTree checked it.
	src := filepath.Join(t.TempDir(), "notes")
	writeTree(t, src, map[string]string{"notes": "#!/bin/sh\n"})
	digest, _ := TreeDigest(src)
	os.WriteFile(filepath.Join(src, "notes"), []byte("#!/bin/sh\nevil\n"), 0o755)
	archive := filepath.Join(t.TempDir(), "notes.tar.gz")
	if err := PackTree(src, archive); err != nil {
		t.Fatal(err)
	}
	tdir := filepath.Join(t.TempDir(), "notes")
	os.Mkdir(tdir, 0o755)
	rel := &Release{Format: FormatDir, TreeSHA256: digest}
	if _, err := stage(&Payload{Release: rel, Path: src, Patched: true}, tdir); err == nil {
		t.Fatal("changed patched tree staged")
	}
	// A full archive swapped after Download.
	rel.Full = full // digest of another file
	if _, err := stage(&Payload{Release: rel, Path: archive}, tdir); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("swapped archive: %v", err)
	}
}

// ---- Linux dir installs ----

func TestCheckDirTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix paths")
	}
	home := "/home/u"
	for _, bad := range []string{
		"/", "/usr", "/opt", "/tmp", "/home", "/usr/bin", "/usr/local/bin", "/usr/local", "/home/other",
		"/Users/u", "/home/u", "/home/u/bin", "/home/u/.local/bin", "/home/u/Downloads", "/home/u/Desktop",
		"relative/notes", "/usr/bin/",
	} {
		if err := checkDirTarget(bad, home); !errors.Is(err, ErrUnsupported) {
			t.Errorf("checkDirTarget(%q) = %v", bad, err)
		}
	}
	for _, ok := range []string{"/opt/notes", "/home/u/.local/opt/notes", "/home/u/Apps/notes", "/usr/lib/notes", "/tmp/x/notes", "/srv/notes"} {
		if err := checkDirTarget(ok, home); err != nil {
			t.Errorf("checkDirTarget(%q) = %v", ok, err)
		}
	}
	// An app run from /usr/local/bin is not updated as a directory.
	target, err := targetFor("linux", FormatDir, "/usr/local/bin/notes", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage(&Payload{Release: &Release{Format: FormatDir}}, target); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("stage(/usr/local/bin) = %v", err)
	}
}

func TestCheckDirLaunch(t *testing.T) {
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"bin/notes": "#!/bin/sh\n", "data": "x"})
	os.Chmod(filepath.Join(tree, "bin/notes"), 0o755)
	target := t.TempDir()
	target, _ = filepath.EvalSymlinks(target)
	if err := checkDirLaunch(tree, target, filepath.Join(target, "bin", "notes")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := checkDirLaunch(tree, target, filepath.Join(target, "data")); err == nil {
			t.Error("non-executable accepted")
		}
	}
	if err := checkDirLaunch(tree, target, filepath.Join(target, "missing")); err == nil {
		t.Error("missing executable accepted")
	}
	// The app does not run from an explicit Updater.Target: not checked.
	if err := checkDirLaunch(tree, target, "/elsewhere/notes"); err != nil {
		t.Error(err)
	}
}
