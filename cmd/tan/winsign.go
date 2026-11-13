package main

// Authenticode signing for Windows executables, installers and packages.
// On Windows tan uses signtool (Windows SDK); elsewhere osslsigncode. The
// certificate is a PKCS#12 (.pfx) file; its password comes from a flag or
// the environment, never from tan.json, and never appears in tan's output.
//
// (The file is not named sign_windows.go: that suffix would limit it to
// GOOS=windows builds.)

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	envWindowsCert         = "TANGO_WINDOWS_CERT"          // .pfx path
	envWindowsCertPassword = "TANGO_WINDOWS_CERT_PASSWORD" // its password
	defaultTimestampURL    = "http://timestamp.digicert.com"
)

// windowsSignExts are the files tan sign hands to Authenticode.
var windowsSignExts = []string{".exe", ".msi", ".msix", ".appx"}

// isWindowsSignable reports whether path is a Windows file tan sign signs.
func isWindowsSignable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range windowsSignExts {
		if ext == e {
			return true
		}
	}
	return false
}

// winSigner holds resolved signing settings. String hides the password.
type winSigner struct {
	cert, password, timestamp string
}

func (s winSigner) String() string {
	return fmt.Sprintf("certificate %s, timestamp %s", s.cert, s.timestamp)
}

// winSignFlags are tan sign's Windows flags.
type winSignFlags struct {
	cert, password, timestamp *string
}

func addWinSignFlags(fs *flag.FlagSet) winSignFlags {
	return winSignFlags{
		cert:      fs.String("cert", "", "Windows: PKCS#12 certificate `file` (default $"+envWindowsCert+", else windows.certificate in tan.json)"),
		password:  fs.String("password", "", "Windows: certificate `password` (prefer $"+envWindowsCertPassword+": flags show up in process lists)"),
		timestamp: fs.String("timestamp", "", "Windows: RFC 3161 timestamp `url` (default windows.timestampURL, else "+defaultTimestampURL+")"),
	}
}

// windowsCertPath is the certificate from the flag, the environment or
// tan.json, in that order; "" when none is set. cfg may be nil.
func windowsCertPath(cfg *Config, flagCert string) string {
	if p := firstNonEmpty(flagCert, os.Getenv(envWindowsCert)); p != "" {
		return p
	}
	if cfg != nil && cfg.Windows.Certificate != "" {
		return cfg.Path(cfg.Windows.Certificate)
	}
	return ""
}

// resolveWinSigner completes the signing settings or says what is missing.
func resolveWinSigner(cfg *Config, flagCert, flagPassword, flagTimestamp string) (winSigner, error) {
	s := winSigner{cert: windowsCertPath(cfg, flagCert)}
	if s.cert == "" {
		return s, fmt.Errorf("no Windows signing certificate: pass -cert, set %s, or set \"windows.certificate\" in %s", envWindowsCert, ConfigFile)
	}
	if _, err := os.Stat(s.cert); err != nil {
		return s, fmt.Errorf("Windows signing certificate: %w", err)
	}
	s.password = firstNonEmpty(flagPassword, os.Getenv(envWindowsCertPassword))
	if s.password == "" {
		return s, fmt.Errorf("no password for %s: set %s (or pass -password)", filepath.Base(s.cert), envWindowsCertPassword)
	}
	ts := ""
	if cfg != nil {
		ts = cfg.Windows.TimestampURL
	}
	s.timestamp = firstNonEmpty(flagTimestamp, ts, defaultTimestampURL)
	return s, nil
}

// signtoolArgs signs file with SHA-256 digests and an RFC 3161 timestamp.
// signtool takes a .pfx password only as /p on its command line (the
// SignTool documentation lists no file, environment or stdin option), so
// while it runs the password is visible to other processes of this user
// and administrators; osslsigncode reads it from a file instead, see
// osslsigncodeArgs.
func signtoolArgs(s winSigner, file string) []string {
	return []string{"sign", "/fd", "sha256", "/f", s.cert, "/p", s.password, "/tr", s.timestamp, "/td", "sha256", file}
}

// osslsigncodeArgs signs in into out (osslsigncode does not sign in place).
// The password is read from passFile (-readpass), not given on the command
// line where process lists show it.
func osslsigncodeArgs(s winSigner, passFile, in, out string) []string {
	return []string{"sign", "-pkcs12", s.cert, "-readpass", passFile, "-h", "sha256", "-ts", s.timestamp, "-in", in, "-out", out}
}

// writePassFile writes password to a new file only this user can read
// (os.CreateTemp: O_EXCL, mode 0600), for osslsigncode -readpass. The
// caller removes it. osslsigncode strips a trailing newline, so none is
// written.
func writePassFile(password string) (string, error) {
	f, err := os.CreateTemp("", "tango-sign-*")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(password)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// winSignTool finds the signing tool: signtool on Windows (osslsigncode
// also works there), osslsigncode elsewhere.
func winSignTool() (path, kind string, err error) {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("signtool"); err == nil {
			return p, "signtool", nil
		}
	}
	if p, err := exec.LookPath("osslsigncode"); err == nil {
		return p, "osslsigncode", nil
	}
	return "", "", errors.New(winSignToolHint(runtime.GOOS))
}

func winSignToolHint(goos string) string {
	switch goos {
	case "windows":
		return "no signtool.exe on PATH: install the Windows SDK (Signing Tools for Desktop Apps) and add its bin\\<version>\\x64 directory to PATH, or install osslsigncode"
	case "darwin":
		return "no osslsigncode on PATH: install it with brew install osslsigncode (Windows signing on macOS uses osslsigncode)"
	default:
		return "no osslsigncode on PATH: install it with your package manager (apt install osslsigncode, dnf install osslsigncode, pacman -S osslsigncode)"
	}
}

// signWindowsFile signs file in place.
func signWindowsFile(ctx context.Context, s winSigner, file string) error {
	tool, kind, err := winSignTool()
	if err != nil {
		return err
	}
	if kind == "signtool" {
		if !warnedSigntoolPassword {
			warnedSigntoolPassword = true
			warnf("signtool takes the certificate password on its command line (/p), where other programs can read it while it runs; with osslsigncode on PATH instead, tan passes it in a private temporary file")
		}
		if err := runSecret(ctx, s.password, tool, signtoolArgs(s, file)...); err != nil {
			return err
		}
		logf("Signed %s", file)
		return nil
	}
	st, err := os.Stat(file)
	if err != nil {
		return err
	}
	passFile, err := writePassFile(s.password)
	if err != nil {
		return err
	}
	defer os.Remove(passFile)
	tmp := file + ".signing.tmp"
	os.Remove(tmp) // osslsigncode refuses to overwrite
	if err := runSecret(ctx, s.password, tool, osslsigncodeArgs(s, passFile, file, tmp)...); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, st.Mode().Perm()); err != nil {
		os.Remove(tmp)
		return err
	}
	// Same directory, so the rename replaces the file atomically.
	if err := os.Rename(tmp, file); err != nil {
		os.Remove(tmp)
		return err
	}
	logf("Signed %s", file)
	return nil
}

// runSecret runs a command whose arguments include secret; an error
// message shows the command with secret masked.
func runSecret(ctx context.Context, secret, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", redactArgs(cmd.Args, secret), err)
	}
	return nil
}

// redactArgs joins args with every occurrence of secret replaced.
func redactArgs(args []string, secret string) string {
	s := strings.Join(args, " ")
	if secret != "" {
		s = strings.ReplaceAll(s, secret, "******")
	}
	return s
}

// signWindows is tan sign for .exe, .msi, .msix and .appx files.
func signWindows(dir string, f winSignFlags, files []string) error {
	cfg, _ := LoadConfig(dir) // tan.json is optional here
	s, err := resolveWinSigner(cfg, *f.cert, *f.password, *f.timestamp)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, file := range files {
		if !isWindowsSignable(file) {
			return fmt.Errorf("%s: tan sign signs .exe, .msi, .msix and .appx files on Windows", file)
		}
		if err := signWindowsFile(ctx, s, file); err != nil {
			return err
		}
	}
	return nil
}

// packageSigner returns the signer tan package uses, or ok false when no
// certificate is configured; then it warns once per run.
func packageSigner(cfg *Config) (s winSigner, ok bool, err error) {
	if windowsCertPath(cfg, "") == "" {
		if !warnedUnsigned {
			warnedUnsigned = true
			warnf("No Windows signing certificate (%s or \"windows.certificate\"): the executable and installer are unsigned, and SmartScreen will warn users", envWindowsCert)
		}
		return s, false, nil
	}
	s, err = resolveWinSigner(cfg, "", "", "")
	return s, err == nil, err
}

var warnedUnsigned, warnedSigntoolPassword bool
