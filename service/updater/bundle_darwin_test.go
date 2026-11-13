package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// signedBundle makes an ad hoc signed .app around a copy of /usr/bin/true.
func signedBundle(t *testing.T, dir, version string) string {
	t.Helper()
	app := filepath.Join(dir, "Notes.app")
	writeTree(t, app, map[string]string{
		"Contents/Info.plist": `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleExecutable</key><string>Notes</string><key>CFBundleIdentifier</key><string>com.example.notes</string><key>CFBundleShortVersionString</key><string>` + version + `</string></dict></plist>`,
		"Contents/Resources/r.txt": version,
	})
	os.MkdirAll(filepath.Join(app, "Contents/MacOS"), 0o755)
	if err := copyFile("/usr/bin/true", filepath.Join(app, "Contents/MacOS/Notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Skipf("codesign: %v %s", err, out)
	}
	return app
}

func TestPrepareBundle(t *testing.T) {
	running := signedBundle(t, t.TempDir(), "1.0.0")
	fresh := signedBundle(t, t.TempDir(), "2.0.0")
	if err := exec.Command("/usr/bin/xattr", "-w", quarantineAttr, "0081;6543a1b2;Safari;", fresh).Run(); err != nil {
		t.Skip("cannot set xattrs here")
	}
	exec.Command("/usr/bin/xattr", "-w", quarantineAttr, "0081;6543a1b2;Safari;", filepath.Join(fresh, "Contents/Info.plist")).Run()
	if !hasQuarantine(fresh) {
		t.Fatal("quarantine not set")
	}
	if err := prepareBundle(fresh, running); err != nil {
		t.Fatal(err)
	}
	if hasQuarantine(fresh) || hasQuarantine(filepath.Join(fresh, "Contents/Info.plist")) {
		t.Fatal("quarantine left")
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", fresh).CombinedOutput(); err != nil {
		t.Fatalf("signature broken: %s", out)
	}

	// A bundle modified after signing is refused while the running app is
	// signed.
	os.WriteFile(filepath.Join(fresh, "Contents/Resources/r.txt"), []byte("tampered"), 0o644)
	if err := prepareBundle(fresh, running); err == nil {
		t.Fatal("tampered bundle accepted")
	}
}
