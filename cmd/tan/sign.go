package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tangoui/tango/service/updater"
)

// Environment variables tan sign reads when flags are absent, so CI can
// keep credentials out of command lines.
const (
	envSignIdentity   = "TANGO_SIGN_IDENTITY"   // codesign identity
	envNotaryProfile  = "TANGO_NOTARY_PROFILE"  // notarytool keychain profile
	envAppleID        = "TANGO_APPLE_ID"        // with team id and app password
	envAppleTeamID    = "TANGO_APPLE_TEAM_ID"   //
	envApplePassword  = "TANGO_APPLE_PASSWORD"  // app-specific password
	envUpdateKey      = "TANGO_UPDATE_KEY"      // private update key, base64
	envUpdateKeyFile  = "TANGO_UPDATE_KEY_FILE" // or a file holding it
	notarizeAppSuffix = ".app"
)

func runSign(args []string) error {
	fs := newFlags("sign", "[flags] <App.app|image.dmg> [-- notarytool args]\n       tan sign [flags] <app.exe|setup.msi|app.msix|app.appx>...")
	dir := projectFlag(fs)
	identity := fs.String("identity", "", "macOS: codesign `identity`, e.g. \"Developer ID Application: Name (TEAMID)\" (default $"+envSignIdentity+")")
	entitlements := fs.String("entitlements", "", "entitlements `plist` (default macos.entitlements from tan.json)")
	notarize := fs.Bool("notarize", false, "submit to Apple's notary service and staple the ticket")
	profile := fs.String("profile", "", "notarytool keychain `profile` (default $"+envNotaryProfile+"; else $"+envAppleID+", $"+envAppleTeamID+", $"+envApplePassword+")")
	win := addWinSignFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return errors.New("sign needs the app, disk image, executable or package to sign")
	}
	if isWindowsSignable(fs.Arg(0)) { // winsign.go
		if *notarize {
			return errors.New("-notarize is for macOS apps and disk images")
		}
		return signWindows(*dir, win, fs.Args())
	}
	if runtime.GOOS != "darwin" {
		return errors.New("code signing and notarization need macOS (codesign, notarytool)")
	}
	target, extra := fs.Arg(0), fs.Args()[1:]
	ent := *entitlements
	if ent == "" {
		if cfg, err := LoadConfig(*dir); err == nil && cfg.MacOS.Entitlements != "" {
			ent = cfg.Path(cfg.MacOS.Entitlements)
		}
	}
	id := firstNonEmpty(*identity, os.Getenv(envSignIdentity))
	if id == "" {
		return fmt.Errorf("no signing identity: pass -identity or set %s (list yours with: security find-identity -v -p codesigning)", envSignIdentity)
	}
	ctx := context.Background()
	if err := runIn(ctx, "", nil, "codesign", codesignArgs(id, ent, target)...); err != nil {
		return err
	}
	if err := runIn(ctx, "", nil, "codesign", "--verify", "--strict", "--verbose=2", target); err != nil {
		return err
	}
	logf("Signed %s", target)
	if !*notarize {
		return nil
	}
	auth, err := notaryAuth(firstNonEmpty(*profile, os.Getenv(envNotaryProfile)))
	if err != nil {
		return err
	}
	return notarizeAndStaple(ctx, target, auth, extra)
}

// codesignArgs signs with the hardened runtime and a secure timestamp, as
// notarization requires.
func codesignArgs(identity, entitlements, target string) []string {
	args := []string{"--force", "--sign", identity, "--options", "runtime", "--timestamp"}
	if entitlements != "" {
		args = append(args, "--entitlements", entitlements)
	}
	return append(args, target)
}

// notaryAuth returns the notarytool credential flags.
func notaryAuth(profile string) ([]string, error) {
	if profile != "" {
		return []string{"--keychain-profile", profile}, nil
	}
	id, team, pw := os.Getenv(envAppleID), os.Getenv(envAppleTeamID), os.Getenv(envApplePassword)
	if id == "" || team == "" || pw == "" {
		return nil, fmt.Errorf("no notary credentials: pass -profile (create one with: xcrun notarytool store-credentials), or set %s, %s and %s", envAppleID, envAppleTeamID, envApplePassword)
	}
	return []string{"--apple-id", id, "--team-id", team, "--password", pw}, nil
}

func notarizeAndStaple(ctx context.Context, target string, auth, extra []string) error {
	submit := target
	if strings.HasSuffix(target, notarizeAppSuffix) {
		// notarytool takes a zip, dmg or pkg; an app is zipped with ditto
		// so its bundle structure survives.
		tmp, err := os.MkdirTemp("", "tango-notary-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		submit = filepath.Join(tmp, strings.TrimSuffix(filepath.Base(target), notarizeAppSuffix)+".zip")
		if err := runIn(ctx, "", nil, "ditto", "-c", "-k", "--keepParent", target, submit); err != nil {
			return err
		}
	}
	logf("Submitting %s for notarization (this can take minutes)", filepath.Base(target))
	args := append([]string{"notarytool", "submit", submit, "--wait"}, auth...)
	args = append(args, extra...)
	if err := runIn(ctx, "", nil, "xcrun", args...); err != nil {
		return fmt.Errorf("%w (see the log with: xcrun notarytool log <submission id> ...)", err)
	}
	if err := runIn(ctx, "", nil, "xcrun", "stapler", "staple", target); err != nil {
		return err
	}
	logf("Notarized and stapled %s", target)
	return nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// Update signing uses the updater package, so keys and signatures are
// exactly what updater.Updater verifies: standard base64 Ed25519 keys and
// a signed manifest envelope (see updater.SignManifest).

func runKeygen(args []string) error {
	fs := newFlags("keygen", "[flags]")
	out := fs.String("o", "update", "write `name`.key (private) and name.pub (public)")
	force := fs.Bool("f", false, "overwrite existing key files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	priv, pub := *out+".key", *out+".pub"
	if !*force {
		for _, p := range []string{priv, pub} {
			if _, err := os.Lstat(p); err == nil { // a dangling symlink counts
				return fmt.Errorf("%s exists (pass -f to overwrite)", p)
			}
		}
	}
	pk, sk, err := updater.GenerateKey()
	if err != nil {
		return err
	}
	if err := writeKeyFile(priv, []byte(sk+"\n"), 0o600); err != nil {
		return err
	}
	if err := writeKeyFile(pub, []byte(pk+"\n"), 0o644); err != nil {
		return err
	}
	logf("Wrote %s (keep it secret, out of the repository) and %s; the public key, for Updater.PublicKey:", priv, pub)
	fmt.Println(pk)
	return nil
}

// writeKeyFile writes a key file without trusting what is at path: the
// data goes to a new file in the same directory (os.CreateTemp: O_EXCL,
// mode 0600), which then replaces path with a rename. A replaced private
// key so never keeps an old, looser mode, and a symbolic link planted at
// path is refused rather than followed (rename would replace the link
// itself, but refusing makes the surprise visible).
func writeKeyFile(path string, data []byte, mode os.FileMode) error {
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file (a symbolic link?); remove it first", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil && mode != 0o600 {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// runSignUpdate signs an update manifest: a manifest JSON file written by
// hand or a script (or an already signed one, to sign it again), checked
// and wrapped in the signed envelope updater.Updater fetches. tan
// release-manifest does this as part of building a release.
func runSignUpdate(args []string) error {
	fs := newFlags("sign-update", "[flags] <manifest.json>")
	keyFile := fs.String("key", "", "private key `file` from tan keygen (default $"+envUpdateKeyFile+", or the key itself in $"+envUpdateKey+")")
	out := fs.String("o", "", "write the signed manifest to `file` (default: replace the input)")
	expires := fs.Duration("expires", defaultManifestValidity, "set the manifest's \"expires\" this long from now, e.g. 720h; 0 keeps the manifest's own")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("sign-update needs one manifest file (tan release-manifest builds one from a build)")
	}
	if *expires < 0 {
		return fmt.Errorf("-expires %s: must not be negative", *expires)
	}
	key, err := updateKey(*keyFile)
	if err != nil {
		return err
	}
	in := fs.Arg(0)
	b, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	m, err := manifestToSign(b)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	if *expires > 0 {
		m.Expires = time.Now().UTC().Truncate(time.Second).Add(*expires)
	}
	if m.App == "" {
		logf("warning: %s has no \"app\"; apps that set Updater.AppID refuse it", in)
	}
	env, err := updater.SignManifest(key, m)
	if err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	dst := firstNonEmpty(*out, in)
	if err := os.WriteFile(dst, env, 0o644); err != nil {
		return err
	}
	pub, _ := updater.PublicKeyOf(key)
	logf("Signed %s %s (%d platforms) → %s; verifies with public key %s", m.Version, m.App, len(m.Platforms), dst, pub)
	return nil
}

// manifestToSign decodes a bare manifest, or the manifest inside a signed
// envelope.
func manifestToSign(b []byte) (*updater.Manifest, error) {
	var env updater.SignedManifest
	if json.Unmarshal(b, &env) == nil && len(env.Manifest) > 0 {
		b = env.Manifest
	}
	return updater.ParseManifest(b)
}

// updateKey returns the private update key from -key, $TANGO_UPDATE_KEY_FILE
// or $TANGO_UPDATE_KEY.
func updateKey(keyFile string) (string, error) {
	key := os.Getenv(envUpdateKey)
	if f := firstNonEmpty(keyFile, os.Getenv(envUpdateKeyFile)); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		key = string(b)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("no update key: pass -key, or set %s or %s (create one with tan keygen)", envUpdateKeyFile, envUpdateKey)
	}
	if _, err := updater.PublicKeyOf(key); err != nil {
		return "", err
	}
	return key, nil
}

// lookTool reports whether name is on PATH.
func lookTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
