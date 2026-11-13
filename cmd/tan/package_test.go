package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func testConfig(t *testing.T) *Config {
	dir := t.TempDir()
	return &Config{Name: "Hello App", ID: "com.example.hello", Version: "1.0.0", Frontend: Frontend{Dir: ".", Dist: "."}, dir: dir}
}

func TestPackageLinux(t *testing.T) {
	cfg := testConfig(t)
	cfg.Icon = "icon.png"
	os.WriteFile(cfg.Path("icon.png"), defaultIcon("x"), 0o644)
	cfg.Icons.Linux.Symbolic = "symbolic.svg"
	os.WriteFile(cfg.Path("symbolic.svg"), []byte("<svg/>"), 0o644)
	a := artifact{target: target{"linux", "amd64"}, dir: filepath.Join(cfg.dir, "build", "linux-amd64")}
	os.MkdirAll(a.dir, 0o755)
	a.exe = filepath.Join(a.dir, "HelloApp")
	a.path = a.exe
	os.WriteFile(a.exe, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", t.TempDir()) // no appimagetool
	made, err := packageLinux(cfg, a)
	if err != nil {
		t.Fatal(err)
	}
	tgz := filepath.Join(cfg.dir, "build", "HelloApp-1.0.0-linux-amd64.tar.gz")
	if len(made) != 2 || made[0] != tgz {
		t.Fatalf("made %v", made)
	}
	f, _ := os.Open(tgz)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if h.Name == "HelloApp-1.0.0-linux-amd64/HelloApp" && h.Mode != 0o755 {
			t.Errorf("executable mode %o", h.Mode)
		}
	}
	want := []string{"HelloApp-1.0.0-linux-amd64/", "HelloApp-1.0.0-linux-amd64/HelloApp", "HelloApp-1.0.0-linux-amd64/HelloApp.desktop", "HelloApp-1.0.0-linux-amd64/install.sh"}
	var hicolor []string
	for _, n := range linuxIconSizes {
		p := "share/" + hicolorPath("HelloApp", n)
		want = append(want, "HelloApp-1.0.0-linux-amd64/"+p)
		hicolor = append(hicolor, "usr/"+p)
	}
	want = append(want, "HelloApp-1.0.0-linux-amd64/share/"+hicolorSymbolicPath("HelloApp"))
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("tar entries\n%v\nwant\n%v", names, want)
	}
	appDir := made[1]
	hicolor = append(hicolor, "usr/share/"+hicolorSymbolicPath("HelloApp"))
	for _, p := range append([]string{"AppRun", "HelloApp.desktop", "HelloApp.png", ".DirIcon", "usr/bin/HelloApp"}, hicolor...) {
		if _, err := os.Stat(filepath.Join(appDir, p)); err != nil {
			t.Error(err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(appDir, "HelloApp.png")); err != nil || !strings.Contains(string(b[:32]), "PNG") {
		t.Errorf("AppDir icon: %v", err)
	} else if cfgPNG, _ := png.DecodeConfig(bytes.NewReader(b)); cfgPNG.Width != 256 {
		t.Errorf("AppDir icon is %d px wide, want 256", cfgPNG.Width)
	}
	// Windows file systems keep no execute bit; appimagetool, which needs
	// it, runs on Linux.
	if st, _ := os.Stat(filepath.Join(appDir, "AppRun")); st == nil || runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
		t.Error("AppRun not executable")
	}
}

func TestDesktopEntryAndNSIS(t *testing.T) {
	cfg := testConfig(t)
	d := desktopEntry(cfg)
	for _, want := range []string{"Name=Hello App\n", "Exec=HelloApp %U\n", "Icon=HelloApp\n"} {
		if !strings.Contains(d, want) {
			t.Errorf(".desktop lacks %q", want)
		}
	}
	a := artifact{target: target{"windows", "amd64"}, exe: `C:\b\HelloApp.exe`}
	n := nsisScript(cfg, a, `C:\b\setup.exe`)
	for _, want := range []string{`Name "Hello App"`, `OutFile "C:\b\setup.exe"`, `File "C:\b\HelloApp.exe"`, `Uninstall\com.example.hello`, `"DisplayVersion" "1.0.0"`} {
		if !strings.Contains(n, want) {
			t.Errorf("nsis lacks %q", want)
		}
	}
}

// Names validate refuses are still kept on one line by the generators.
func TestPackageFieldsStayOnOneLine(t *testing.T) {
	cfg := testConfig(t)
	cfg.Name = "Evil\nExec=/bin/sh -c id\r\\x"
	cfg.Linux.Maintainer = "Jane\nDepends: evil"
	d := desktopEntry(cfg)
	if strings.Contains(d, "\nExec=/bin/sh") || !strings.Contains(d, `Name=Evil\nExec=/bin/sh -c id\r\\x`+"\n") {
		t.Errorf(".desktop:\n%s", d)
	}
	c := debControl(cfg, "amd64", 1)
	for _, line := range strings.Split(strings.TrimSuffix(c, "\n"), "\n") {
		if strings.HasPrefix(line, "Depends: evil") || strings.HasPrefix(line, "Exec=") {
			t.Errorf("control gained a line %q:\n%s", line, c)
		}
	}
	if b := rpmCString("a\x00b"); string(b) != "ab\x00" {
		t.Errorf("rpm string %q", b)
	}
	if got := oneLine("a\nb\tc"); got != "a b c" {
		t.Errorf("oneLine %q", got)
	}
}

func assocConfig(t *testing.T) *Config {
	cfg := testConfig(t)
	cfg.URLSchemes = []string{"hello", "hello-dev"}
	cfg.FileAssociations = []FileAssociation{
		{Ext: "hlo", Name: "Hello document"},
		{Ext: "md", Name: "Markdown", Role: "viewer", MIME: "text/markdown"},
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestLinuxAssociations(t *testing.T) {
	cfg := assocConfig(t)
	d := desktopEntry(cfg)
	if !strings.Contains(d, "\nMimeType=x-scheme-handler/hello;x-scheme-handler/hello-dev;application/x-helloapp-hlo;text/markdown;\n") {
		t.Errorf(".desktop:\n%s", d)
	}
	x := string(mimePackage(cfg))
	if !strings.Contains(x, `<mime-type type="application/x-helloapp-hlo">`) || !strings.Contains(x, `<glob pattern="*.hlo"/>`) || strings.Contains(x, "markdown") {
		t.Errorf("mime package:\n%s", x)
	}
	if mimePackage(testConfig(t)) != nil {
		t.Error("mime package without private types")
	}
	sh := installScript(cfg, true, true)
	for _, want := range []string{"update-desktop-database", "update-mime-database", `share/mime/packages/com.example.hello.xml`, `s|^Exec=HelloApp |Exec=$PREFIX/bin/HelloApp |`,
		"for d in share/icons/hicolor/*/apps; do", `gtk-update-icon-cache -q -t -f "$PREFIX/share/icons/hicolor"`} {
		if !strings.Contains(sh, want) {
			t.Errorf("install.sh lacks %q:\n%s", want, sh)
		}
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		p := filepath.Join(t.TempDir(), "install.sh")
		os.WriteFile(p, []byte(installScript(cfg, true, true)), 0o755)
		if out, err := exec.Command(sh, "-n", p).CombinedOutput(); err != nil {
			t.Errorf("sh -n: %v %s", err, out)
		}
	}
}

func TestNSISAssociations(t *testing.T) {
	cfg := assocConfig(t)
	a := artifact{target: target{"windows", "amd64"}, exe: `C:\b\HelloApp.exe`}
	n := nsisScript(cfg, a, `C:\b\setup.exe`)
	for _, want := range []string{
		`WriteRegStr HKCU "Software\Classes\hello" "URL Protocol" ""`,
		`WriteRegStr HKCU "Software\Classes\hello\shell\open\command" "" '"$INSTDIR\HelloApp.exe" -- "%1"'`,
		`WriteRegStr HKCU "Software\Classes\com.example.hello.hlo\shell\open\command" "" '"$INSTDIR\HelloApp.exe" -- "%1"'`,
		`WriteRegStr HKCU "Software\Classes\.hlo\OpenWithProgids" "com.example.hello.hlo" ""`,
		`WriteRegStr HKCU "Software\Classes\.md" "Content Type" "text/markdown"`,
		`DeleteRegKey HKCU "Software\Classes\hello-dev"`,
		`SHChangeNotify(i 0x08000000`,
	} {
		if !strings.Contains(n, want) {
			t.Errorf("nsis lacks %q", want)
		}
	}
	makensis, err := exec.LookPath("makensis")
	if err != nil {
		return
	}
	// Compile for real against a dummy executable.
	dir := t.TempDir()
	a.exe = filepath.Join(dir, "HelloApp.exe")
	os.WriteFile(a.exe, []byte("MZ"), 0o644)
	nsi := filepath.Join(dir, "installer.nsi")
	os.WriteFile(nsi, []byte(nsisScript(cfg, a, filepath.Join(dir, "setup.exe"))), 0o644)
	if out, err := exec.Command(makensis, "-V2", nsi).CombinedOutput(); err != nil {
		t.Fatalf("makensis: %v\n%s", err, out)
	}
}
