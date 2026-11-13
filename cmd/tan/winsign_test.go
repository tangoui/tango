package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsWindowsSignable(t *testing.T) {
	for _, f := range []string{"a.exe", `C:\b\Setup.EXE`, "x.msi", "x.msix", "x.appx"} {
		if !isWindowsSignable(f) {
			t.Errorf("%s not signable", f)
		}
	}
	for _, f := range []string{"A.app", "a.dmg", "a.zip", "exe"} {
		if isWindowsSignable(f) {
			t.Errorf("%s signable", f)
		}
	}
}

func TestWindowsSignArgs(t *testing.T) {
	s := winSigner{cert: "c.pfx", password: "pw", timestamp: "http://ts"}
	if got := strings.Join(signtoolArgs(s, "a.exe"), " "); got != "sign /fd sha256 /f c.pfx /p pw /tr http://ts /td sha256 a.exe" {
		t.Errorf("signtool %q", got)
	}
	if got := strings.Join(osslsigncodeArgs(s, "/tmp/pw", "a.exe", "a.tmp"), " "); got != "sign -pkcs12 c.pfx -readpass /tmp/pw -h sha256 -ts http://ts -in a.exe -out a.tmp" {
		t.Errorf("osslsigncode %q", got)
	}
	if strings.Contains(s.String(), "pw") {
		t.Errorf("String shows the password: %s", s)
	}
	if got := redactArgs([]string{"x", "-pass", "pw"}, "pw"); got != "x -pass ******" {
		t.Errorf("redacted %q", got)
	}
	for _, goos := range []string{"windows", "darwin", "linux"} {
		if h := winSignToolHint(goos); !strings.Contains(h, "install") {
			t.Errorf("%s hint %q", goos, h)
		}
	}
}

func TestWritePassFile(t *testing.T) {
	p, err := writePassFile("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	b, _ := os.ReadFile(p)
	st, _ := os.Stat(p)
	if string(b) != "s3cret" {
		t.Errorf("contents %q", b)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
}

func TestResolveWinSigner(t *testing.T) {
	cfg := testConfig(t)
	t.Setenv(envWindowsCert, "")
	t.Setenv(envWindowsCertPassword, "")
	if _, err := resolveWinSigner(cfg, "", "", ""); err == nil || !strings.Contains(err.Error(), envWindowsCert) {
		t.Errorf("no certificate: %v", err)
	}
	cfg.Windows.Certificate = "cfg.pfx"
	os.WriteFile(cfg.Path("cfg.pfx"), []byte("x"), 0o600)
	if _, err := resolveWinSigner(cfg, "", "", ""); err == nil || !strings.Contains(err.Error(), envWindowsCertPassword) {
		t.Errorf("no password: %v", err)
	}
	t.Setenv(envWindowsCertPassword, "envpw")
	s, err := resolveWinSigner(cfg, "", "", "")
	if err != nil || s.cert != cfg.Path("cfg.pfx") || s.password != "envpw" || s.timestamp != defaultTimestampURL {
		t.Errorf("from tan.json: %+v %v", s, err)
	}
	env := filepath.Join(cfg.dir, "env.pfx")
	os.WriteFile(env, []byte("x"), 0o600)
	t.Setenv(envWindowsCert, env)
	cfg.Windows.TimestampURL = "http://cfg-ts"
	if s, _ := resolveWinSigner(cfg, "", "", ""); s.cert != env || s.timestamp != "http://cfg-ts" {
		t.Errorf("env over tan.json: %+v", s)
	}
	flagCert := filepath.Join(cfg.dir, "flag.pfx")
	os.WriteFile(flagCert, []byte("x"), 0o600)
	if s, _ := resolveWinSigner(cfg, flagCert, "flagpw", "http://flag-ts"); s.cert != flagCert || s.password != "flagpw" || s.timestamp != "http://flag-ts" {
		t.Errorf("flags first: %+v", s)
	}
	if _, err := resolveWinSigner(cfg, filepath.Join(cfg.dir, "missing.pfx"), "", ""); err == nil {
		t.Error("missing certificate file accepted")
	}
	if _, err := resolveWinSigner(nil, flagCert, "", ""); err != nil {
		t.Errorf("without tan.json: %v", err)
	}
}

// fakeSignTools puts an osslsigncode (appends SIGNED, fails unless the
// password is "good") and a makensis (writes the OutFile) on PATH.
func fakeSignTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fakes")
	}
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "osslsigncode"), []byte(`#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in -in) in=$2; shift;; -out) out=$2; shift;; -readpass) pass=$(cat "$2"); shift;; -pass|good) echo "password on the command line" >&2; exit 5;; esac
  shift
done
[ "$pass" = good ] || { echo "bad password" >&2; exit 3; }
[ -e "$out" ] && exit 4
{ cat "$in"; printf SIGNED; } > "$out"
`), 0o755)
	os.WriteFile(filepath.Join(bin, "makensis"), []byte(`#!/bin/sh
out=$(sed -n 's/^OutFile "\(.*\)"$/\1/p' "$2")
printf SETUP > "$out"
`), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
}

func TestSignWindowsFileOsslsigncode(t *testing.T) {
	fakeSignTools(t)
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir) // where the password file goes
	dir := t.TempDir()
	exe := filepath.Join(dir, "a.exe")
	os.WriteFile(exe, []byte("MZ"), 0o755)
	cert := filepath.Join(dir, "c.pfx")
	os.WriteFile(cert, nil, 0o600)
	ctx := context.Background()
	if err := signWindowsFile(ctx, winSigner{cert: cert, password: "good", timestamp: "http://ts"}, exe); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmpDir); len(left) > 0 {
		t.Errorf("password file left behind: %v", left)
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZSIGNED" {
		t.Errorf("signed file %q", b)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", st.Mode())
	}
	err := signWindowsFile(ctx, winSigner{cert: cert, password: "s3cret", timestamp: "http://ts"}, exe)
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("failure: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZSIGNED" {
		t.Errorf("failed signing changed the file: %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) > 0 {
		t.Errorf("left %v", left)
	}
}

func TestRunSignWindows(t *testing.T) {
	fakeSignTools(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "a.exe")
	os.WriteFile(exe, []byte("MZ"), 0o644)
	cert := filepath.Join(dir, "c.pfx")
	os.WriteFile(cert, nil, 0o600)
	t.Setenv(envWindowsCert, cert)
	t.Setenv(envWindowsCertPassword, "good")
	if err := runSign([]string{"-C", dir, exe}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZSIGNED" {
		t.Errorf("signed file %q", b)
	}
	if err := runSign([]string{"-C", dir, "-notarize", exe}); err == nil {
		t.Error("-notarize accepted for an exe")
	}
	t.Setenv(envWindowsCertPassword, "")
	if err := runSign([]string{"-C", dir, exe}); err == nil || !strings.Contains(err.Error(), envWindowsCertPassword) {
		t.Errorf("no password: %v", err)
	}
}

func TestPackageWindowsSigning(t *testing.T) {
	fakeSignTools(t)
	cfg := testConfig(t)
	a := artifact{target: target{"windows", "amd64"}, dir: filepath.Join(cfg.dir, "build", "windows-amd64")}
	os.MkdirAll(a.dir, 0o755)
	a.exe = filepath.Join(a.dir, "HelloApp.exe")
	a.path = a.exe
	os.WriteFile(a.exe, []byte("MZ"), 0o755)
	setup := filepath.Join(cfg.dir, "build", "HelloApp-1.0.0-windows-amd64-setup.exe")

	// No certificate: unsigned, with a single warning.
	t.Setenv(envWindowsCert, "")
	warnedUnsigned = false
	made, err := packageWindows(context.Background(), cfg, a)
	if err != nil || len(made) != 2 || made[1] != setup {
		t.Fatalf("made %v, %v", made, err)
	}
	if !warnedUnsigned {
		t.Error("no warning")
	}
	if b, _ := os.ReadFile(a.exe); !bytes.Equal(b, []byte("MZ")) {
		t.Errorf("exe changed: %q", b)
	}

	// With one: the exe before NSIS, the installer after.
	cfg.Windows.Certificate = "c.pfx"
	os.WriteFile(cfg.Path("c.pfx"), nil, 0o600)
	t.Setenv(envWindowsCertPassword, "good")
	if _, err := packageWindows(context.Background(), cfg, a); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(a.exe); string(b) != "MZSIGNED" {
		t.Errorf("exe %q", b)
	}
	if b, _ := os.ReadFile(setup); string(b) != "SETUPSIGNED" {
		t.Errorf("setup %q", b)
	}
	t.Setenv(envWindowsCertPassword, "")
	if _, err := packageWindows(context.Background(), cfg, a); err == nil {
		t.Error("certificate without a password packaged")
	}
}
