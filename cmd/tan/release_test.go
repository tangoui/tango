package main

import (
	"bytes"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// fakeBuild writes what tan build would leave for darwin, windows and
// linux, with exe as the program's content.
func fakeBuild(t *testing.T, cfg *Config, exe []byte) string {
	t.Helper()
	build := filepath.Join(t.TempDir(), "build")
	app := filepath.Join(build, "darwin-arm64", cfg.Name+".app", "Contents")
	write := func(p string, b []byte, mode os.FileMode) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, b, mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(app, "MacOS", cfg.Exe()), exe, 0o755)
	write(filepath.Join(app, "Info.plist"), []byte("<plist>"+cfg.Version+"</plist>"), 0o644)
	write(filepath.Join(app, "Resources", "icon.icns"), bytes.Repeat([]byte("icon"), 4000), 0o644)
	write(filepath.Join(build, "windows-amd64", cfg.Exe()+".exe"), exe, 0o755)
	write(filepath.Join(build, "linux-amd64", cfg.Exe()), exe, 0o755)
	return build
}

func releaseConfig(t *testing.T, version string) *Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(`{"name":"Notes","id":"com.example.notes","version":"` + version + `","frontend":{"dir":"frontend","dist":"dist"}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.dir = t.TempDir()
	return cfg
}

func TestReleaseManifest(t *testing.T) {
	pub, priv, _ := updater.GenerateKey()
	r := rand.New(rand.NewPCG(1, 2))
	exe1 := make([]byte, 400000)
	for i := range exe1 {
		exe1[i] = byte(r.Uint32())
	}
	exe2 := slices.Clone(exe1)
	for range 100 {
		exe2[r.IntN(len(exe2))]++
	}

	// 1.0.0: no earlier release.
	cfg1 := releaseConfig(t, "1.0.0")
	rel1 := filepath.Join(t.TempDir(), "release-1.0.0")
	o := releaseOptions{buildDir: fakeBuild(t, cfg1, exe1), outDir: rel1, key: priv, windows: "exe", pubDate: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	m1, err := makeRelease(cfg1, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(m1.Platforms) != 3 || m1.App != "com.example.notes" {
		t.Fatalf("manifest %+v", m1)
	}

	// 1.1.0 with patches from 1.0.0.
	cfg2 := releaseConfig(t, "1.1.0")
	rel2 := filepath.Join(t.TempDir(), "release-1.1.0")
	o = releaseOptions{buildDir: fakeBuild(t, cfg2, exe2), outDir: rel2, key: priv, windows: "exe", prev: []string{rel1},
		baseURL: "https://dl.example.com/notes/", notes: "Fixes", pubDate: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		expires: defaultManifestValidity}
	if _, err := makeRelease(cfg2, o); err != nil {
		t.Fatal(err)
	}
	env, _ := os.ReadFile(filepath.Join(rel2, "latest.json"))
	m2, err := updater.VerifyManifest(env, pub)
	if err != nil {
		t.Fatal(err)
	}
	if !m2.Expires.Equal(o.pubDate.Add(defaultManifestValidity)) || !m1.Expires.IsZero() {
		t.Fatalf("expires: %v, %v", m2.Expires, m1.Expires)
	}
	formats := map[string]string{"darwin-arm64": "app", "windows-amd64": "exe", "linux-amd64": "dir"}
	for key, format := range formats {
		a := m2.Platforms[key]
		if a.Format != format || !strings.HasPrefix(a.URL, "https://dl.example.com/notes/Notes-1.1.0-") || len(a.Patches) != 1 || a.Patches[0].From != "1.0.0" {
			t.Fatalf("%s: %+v", key, a)
		}
		t.Logf("%s (%s): full %d bytes, patch %d bytes", key, format, a.Size, a.Patches[0].Size)
	}

	// The windows patch turns the 1.0.0 executable into 1.1.0's.
	w := m2.Platforms["windows-amd64"]
	patch, _ := os.ReadFile(filepath.Join(rel2, filepath.Base(w.Patches[0].URL)))
	got, err := updater.Patch(exe1, patch)
	if err != nil || !bytes.Equal(got, exe2) {
		t.Fatalf("exe patch: %v", err)
	}

	// The app patch turns the unpacked 1.0.0 bundle into 1.1.0's.
	d := m2.Platforms["darwin-arm64"]
	oldTop, err := updater.Unpack(filepath.Join(rel1, filepath.Base(m1.Platforms["darwin-arm64"].URL)), t.TempDir())
	if err != nil || filepath.Base(oldTop) != "Notes.app" {
		t.Fatalf("unpack: %s %v", oldTop, err)
	}
	f, _ := os.Open(filepath.Join(rel2, filepath.Base(d.Patches[0].URL)))
	defer f.Close()
	digest, err := updater.PatchTree(oldTop, f, filepath.Join(t.TempDir(), "Notes.app"))
	if err != nil || digest != d.TreeSHA256 {
		t.Fatalf("tree patch: %s %v (want %s)", digest, err, d.TreeSHA256)
	}

	// Errors: a previous release signed with another key, a bad format.
	_, other, _ := updater.GenerateKey()
	o.key, o.outDir = other, t.TempDir()
	if _, err := makeRelease(cfg2, o); err == nil || !strings.Contains(err.Error(), "another key") {
		t.Fatalf("foreign previous release: %v", err)
	}
	o.key, o.windows = priv, "zip"
	if _, err := makeRelease(cfg2, o); err == nil {
		t.Fatal("bad -windows accepted")
	}
	o.windows, o.buildDir = "nsis", fakeBuild(t, cfg2, exe2)
	if _, err := makeRelease(cfg2, o); err == nil || !strings.Contains(err.Error(), "tan package") {
		t.Fatalf("missing installer: %v", err)
	}
}
