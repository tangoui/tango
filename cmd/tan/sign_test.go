package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/service/updater"
)

func TestKeygenOverwrite(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "k")
	// A loose-mode key from before: -f replaces it with a 0600 one.
	os.WriteFile(key+".key", []byte("old\n"), 0o644)
	os.Chmod(key+".key", 0o644)
	os.WriteFile(key+".pub", []byte("old\n"), 0o644)
	if err := runKeygen([]string{"-o", key}); err == nil {
		t.Error("existing key overwritten without -f")
	}
	if err := runKeygen([]string{"-o", key, "-f"}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(key + ".key")
	if err != nil || !st.Mode().IsRegular() || runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("private key after -f: %v %v", st, err)
	}
	if b, _ := os.ReadFile(key + ".key"); string(b) == "old\n" {
		t.Error("key not replaced")
	}
	if st, _ := os.Stat(key + ".pub"); st == nil || runtime.GOOS != "windows" && st.Mode().Perm() != 0o644 {
		t.Errorf("public key mode: %v", st)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*tmp*")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}

	// A symbolic link planted where the key goes is refused, with -f too,
	// and its target is untouched.
	victim := filepath.Join(dir, "victim")
	os.WriteFile(victim, []byte("keep"), 0o644)
	link := filepath.Join(dir, "l")
	if err := os.Symlink(victim, link+".key"); err != nil {
		t.Skip(err)
	}
	if err := runKeygen([]string{"-o", link}); err == nil {
		t.Error("symlink treated as absent without -f")
	}
	if err := runKeygen([]string{"-o", link, "-f"}); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Errorf("symlink with -f: %v", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Errorf("symlink target written: %q", b)
	}
	// A dangling link is refused as well.
	os.Remove(victim)
	if err := runKeygen([]string{"-o", link, "-f"}); err == nil {
		t.Error("dangling symlink followed")
	}
	if _, err := os.Lstat(victim); err == nil {
		t.Error("dangling symlink target created")
	}
}

func TestKeygenAndSignUpdate(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "k")
	if err := runKeygen([]string{"-o", key}); err != nil {
		t.Fatal(err)
	}
	if err := runKeygen([]string{"-o", key}); err == nil {
		t.Error("existing key overwritten without -f")
	}
	if st, _ := os.Stat(key + ".key"); st == nil || runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("private key mode: %v", st)
	}
	manifest := filepath.Join(dir, "latest.json")
	os.WriteFile(manifest, []byte(`{"app":"com.example.notes","version":"1.1.0","pub_date":"2026-11-01T12:00:00Z",
		"platforms":{"windows-amd64":{"format":"exe","url":"Notes.exe","size":3,"sha256":"`+strings.Repeat("ab", 32)+`"}}}`), 0o644)
	if err := runSignUpdate([]string{"-key", key + ".key", manifest}); err != nil {
		t.Fatal(err)
	}
	pub, _ := os.ReadFile(key + ".pub")
	env, _ := os.ReadFile(manifest)
	m, err := updater.VerifyManifest(env, string(pub))
	if err != nil || m.Version != "1.1.0" {
		t.Fatalf("signed manifest: %+v, %v", m, err)
	}
	// Signing writes "expires", 30 days from now by default.
	if d := time.Until(m.Expires); d < defaultManifestValidity-time.Hour || d > defaultManifestValidity {
		t.Fatalf("expires %v (in %v)", m.Expires, d)
	}
	// -expires 0 keeps the manifest's own.
	if err := runSignUpdate([]string{"-key", key + ".key", "-expires", "0", "-o", manifest + ".keep", manifest}); err != nil {
		t.Fatal(err)
	}
	envKeep, _ := os.ReadFile(manifest + ".keep")
	if mk, err := updater.VerifyManifest(envKeep, string(pub)); err != nil || !mk.Expires.Equal(m.Expires) {
		t.Fatalf("-expires 0: %+v, %v", mk, err)
	}
	// Signing again (an envelope as input) keeps it valid.
	if err := runSignUpdate([]string{"-key", key + ".key", "-o", manifest + ".2", manifest}); err != nil {
		t.Fatal(err)
	}
	env2, _ := os.ReadFile(manifest + ".2")
	if _, err := updater.VerifyManifest(env2, string(pub)); err != nil {
		t.Fatal(err)
	}
	// Invalid manifests are refused.
	os.WriteFile(manifest, []byte(`{"version":"1.1.0","platforms":{}}`), 0o644)
	if err := runSignUpdate([]string{"-key", key + ".key", manifest}); err == nil {
		t.Error("invalid manifest signed")
	}
	t.Setenv(envUpdateKey, "")
	t.Setenv(envUpdateKeyFile, "")
	if err := runSignUpdate([]string{manifest}); err == nil || !strings.Contains(err.Error(), "no update key") {
		t.Errorf("missing key: %v", err)
	}
}

func TestCodesignAndNotaryArgs(t *testing.T) {
	got := strings.Join(codesignArgs("Developer ID Application: X", "e.plist", "A.app"), " ")
	if got != "--force --sign Developer ID Application: X --options runtime --timestamp --entitlements e.plist A.app" {
		t.Errorf("codesign args %q", got)
	}
	if a, _ := notaryAuth("prof"); strings.Join(a, " ") != "--keychain-profile prof" {
		t.Errorf("profile auth %v", a)
	}
	for _, k := range []string{envAppleID, envAppleTeamID, envApplePassword} {
		t.Setenv(k, "")
	}
	if _, err := notaryAuth(""); err == nil || !strings.Contains(err.Error(), "store-credentials") {
		t.Errorf("missing credentials: %v", err)
	}
	t.Setenv(envAppleID, "a@b.c")
	t.Setenv(envAppleTeamID, "TEAM")
	t.Setenv(envApplePassword, "pw")
	if a, err := notaryAuth(""); err != nil || len(a) != 6 {
		t.Errorf("env auth %v %v", a, err)
	}
}
