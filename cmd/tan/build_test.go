package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInfoPlist(t *testing.T) {
	c := &Config{Name: "R&D Tool", ID: "com.example.rd", Version: "2.0.1", Copyright: "© <Us>"}
	got := string(infoPlist(c, "icon"))
	for _, want := range []string{
		"<key>CFBundleExecutable</key>\n\t<string>RDTool</string>",
		"<key>CFBundleIdentifier</key>\n\t<string>com.example.rd</string>",
		"<key>CFBundleName</key>\n\t<string>R&amp;D Tool</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>2.0.1</string>",
		"<key>CFBundleIconFile</key>\n\t<string>icon</string>",
		"<key>LSMinimumSystemVersion</key>\n\t<string>11.0</string>",
		"<string>© &lt;Us&gt;</string>",
		"<key>NSHighResolutionCapable</key>\n\t<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(string(infoPlist(c, "")), "CFBundleIconFile") {
		t.Error("icon key written without an icon")
	}
	if plutil, err := exec.LookPath("plutil"); err == nil {
		p := filepath.Join(t.TempDir(), "Info.plist")
		os.WriteFile(p, []byte(got), 0o644)
		if out, err := exec.Command(plutil, "-lint", p).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v: %s", err, out)
		}
	}
}

func TestInfoPlistAssociations(t *testing.T) {
	cfg := assocConfig(t)
	got := string(infoPlist(cfg, ""))
	for _, want := range []string{
		"<key>CFBundleURLTypes</key>",
		"<key>CFBundleURLName</key>\n\t\t\t<string>com.example.hello</string>",
		"<key>CFBundleURLSchemes</key>\n\t\t\t<array>\n\t\t\t\t<string>hello</string>\n\t\t\t\t<string>hello-dev</string>",
		"<key>CFBundleDocumentTypes</key>",
		"<key>CFBundleTypeName</key>\n\t\t\t<string>Hello document</string>\n\t\t\t<key>CFBundleTypeRole</key>\n\t\t\t<string>Editor</string>\n\t\t\t<key>LSHandlerRank</key>\n\t\t\t<string>Default</string>",
		"<key>CFBundleTypeRole</key>\n\t\t\t<string>Viewer</string>\n\t\t\t<key>LSHandlerRank</key>\n\t\t\t<string>Alternate</string>",
		"<key>CFBundleTypeExtensions</key>\n\t\t\t<array>\n\t\t\t\t<string>hlo</string>",
		"<string>text/markdown</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, got)
		}
	}
	if plutil, err := exec.LookPath("plutil"); err == nil {
		p := filepath.Join(t.TempDir(), "Info.plist")
		os.WriteFile(p, []byte(got), 0o644)
		if out, err := exec.Command(plutil, "-lint", p).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v: %s", err, out)
		}
	}
}

func TestParseTargets(t *testing.T) {
	ts, err := parseTargets("darwin/arm64, windows/amd64,darwin/arm64,linux/arm64,darwin/universal")
	if err != nil {
		t.Fatal(err)
	}
	want := []target{{"darwin", "arm64"}, {"windows", "amd64"}, {"linux", "arm64"}, {"darwin", "universal"}}
	if !slices.Equal(ts, want) {
		t.Fatalf("got %v", ts)
	}
	if ts, _ := parseTargets(""); !slices.Equal(ts, []target{hostTarget}) {
		t.Errorf("default = %v", ts)
	}
	for _, bad := range []string{"plan9/amd64", "windows/universal", "linux/386", "ios/amd64", "android/mips"} {
		if _, err := parseTargets(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestGoBuildArgs(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	rel := strings.Join(goBuildArgs(target{"windows", "amd64"}, "out.exe", false), " ")
	if rel != "build -tags tango_release -trimpath -ldflags -s -w -H windowsgui -o out.exe ." {
		t.Errorf("release = %q", rel)
	}
	dbg := strings.Join(goBuildArgs(target{"linux", "amd64"}, "out", true), " ")
	if dbg != "build -tags tango_release -o out ." {
		t.Errorf("debug = %q", dbg)
	}
}
