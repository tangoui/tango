package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNSISLanguageMapping(t *testing.T) {
	for tag, want := range map[string]string{
		"en": "English", "en-US": "English", "zh-Hans": "SimpChinese", "zh_CN": "SimpChinese", "zh-Hans-CN": "SimpChinese",
		"zh-Hant": "TradChinese", "zh-TW": "TradChinese", "ja": "Japanese", "pt-BR": "PortugueseBR", "pt": "Portuguese",
		"fr-CA": "French", "xx": "", "": "",
	} {
		if got := nsisLanguageName(tag); got != want {
			t.Errorf("%q → %q, want %q", tag, got, want)
		}
	}
	cfg := testConfig(t)
	langs, unknown := installerLanguages(cfg)
	var names []string
	for _, l := range langs {
		names = append(names, l.Name)
	}
	if !slices.Equal(names, []string{"English", "SimpChinese", "TradChinese", "Japanese"}) || unknown != nil {
		t.Errorf("default languages %v %v", names, unknown)
	}
	cfg.Localizations = []string{"zh-Hans", "zh-CN", "ko", "tlh"}
	langs, unknown = installerLanguages(cfg)
	names = nil
	for _, l := range langs {
		names = append(names, l.Name)
	}
	if !slices.Equal(names, []string{"SimpChinese", "Korean", "English"}) || !slices.Equal(unknown, []string{"tlh"}) {
		t.Errorf("languages %v unknown %v", names, unknown)
	}
}

func TestInstallerConfig(t *testing.T) {
	base := `{"name":"A","id":"com.example.a","version":"1.0.0",`
	ok := base + `"installer":{"windows":{"scope":"machine","license":"LICENSE","welcome":false,"sidebarImage":"s.png","headerImage":"h.bmp",
		"runAfterInstall":false,"desktopShortcut":"always","languageDialog":true,"include":"i.nsh","template":"t.nsi"},
		"macos":{"dmg":{"background":"bg.png","windowSize":[600,400],"iconSize":96,"appPosition":[150,200],"applicationsPosition":[450,200],"title":"A Installer"}}}}`
	cfg, err := ParseConfig([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	w, d := cfg.Installer.Windows, cfg.Installer.MacOS.DMG
	if w.Scope != "machine" || *w.Welcome || *w.RunAfterInstall || w.DesktopShortcut != "always" || !w.LanguageDialog || w.Template != "t.nsi" {
		t.Errorf("windows %+v", w)
	}
	if d.IconSize != 96 || !slices.Equal(d.WindowSize, []int{600, 400}) || d.Title != "A Installer" {
		t.Errorf("dmg %+v", d)
	}
	for _, c := range []struct{ json, want string }{
		{`"installer":{"windows":{"scope":"global"}}}`, `"installer.windows.scope"`},
		{`"installer":{"windows":{"desktopShortcut":"yes"}}}`, `"installer.windows.desktopShortcut"`},
		{`"installer":{"windows":{"sidebarImage":"s.jpg"}}}`, `"installer.windows.sidebarImage"`},
		{`"installer":{"windows":{"license":"l.pdf"}}}`, `"installer.windows.license"`},
		{`"installer":{"windows":{"include":"/abs/i.nsh"}}}`, `must be relative`},
		{`"installer":{"windows":{"bogus":1}}}`, `unknown field "bogus"`},
		{`"installer":{"macos":{"dmg":{"windowSize":[600]}}}}`, `"installer.macos.dmg.windowSize"`},
		{`"installer":{"macos":{"dmg":{"appPosition":[-1,2]}}}}`, `"installer.macos.dmg.appPosition"`},
		{`"installer":{"macos":{"dmg":{"iconSize":1000}}}}`, `"installer.macos.dmg.iconSize"`},
		{`"installer":{"macos":{"dmg":{"background":"bg.gif"}}}}`, `"installer.macos.dmg.background"`},
		{`"installer":{"macos":{"dmg":{"title":"a:b"}}}}`, `"installer.macos.dmg.title"`},
	} {
		_, err := ParseConfig([]byte(base + c.json))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %s", c.json, err, c.want)
		}
	}
	// Zero config marshals without an installer section.
	plain, _ := ParseConfig([]byte(base + `"icon":"i.png"}`))
	if strings.Contains(string(plain.Marshal()), "installer") {
		t.Error("empty installer section marshalled")
	}
}

func boolp(b bool) *bool { return &b }

func nsisFor(t *testing.T, cfg *Config) string {
	t.Helper()
	a := artifact{target: target{"windows", "amd64"}, exe: `C:\b\HelloApp.exe`}
	return nsisScript(cfg, a, `C:\b\setup.exe`)
}

func TestNSISScriptOptions(t *testing.T) {
	has := func(t *testing.T, s string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(s, w) {
				t.Errorf("script lacks %q", w)
			}
		}
	}
	hasNot := func(t *testing.T, s string, unwanted ...string) {
		t.Helper()
		for _, w := range unwanted {
			if strings.Contains(s, w) {
				t.Errorf("script has %q", w)
			}
		}
	}
	t.Run("default", func(t *testing.T) {
		s := nsisFor(t, testConfig(t))
		if !strings.HasPrefix(s, "\uFEFF") {
			t.Error("no UTF-8 BOM")
		}
		has(t, s, "RequestExecutionLevel user", `InstallDir "$LOCALAPPDATA\Programs\Hello App"`,
			`InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.example.hello" "InstallLocation"`,
			"!insertmacro MUI_PAGE_WELCOME", "!insertmacro MUI_PAGE_DIRECTORY", "!insertmacro MUI_PAGE_FINISH",
			`!define MUI_FINISHPAGE_RUN "$INSTDIR\HelloApp.exe"`, `!define MUI_FINISHPAGE_SHOWREADME_FUNCTION TanDesktopShortcut`,
			`!insertmacro MUI_LANGUAGE "English"`, `!insertmacro MUI_LANGUAGE "SimpChinese"`, `!insertmacro MUI_LANGUAGE "TradChinese"`, `!insertmacro MUI_LANGUAGE "Japanese"`,
			`LangString TanDesktopShortcut ${LANG_SIMPCHINESE} "创建桌面快捷方式"`, `LangString TanDesktopShortcut ${LANG_JAPANESE} "デスクトップにショートカットを作成する"`,
			`WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.example.hello" "DisplayName" "Hello App"`,
			`Delete "$DESKTOP\Hello App.lnk"`, `DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.example.hello"`,
			"!ifmacrodef customInit", "!ifmacrodef customInstall", "!ifmacrodef customUnInstall", `VIProductVersion "1.0.0.0"`)
		hasNot(t, s, "MUI_PAGE_LICENSE", "SetShellVarContext all", "HKLM", "MUI_LANGDLL_DISPLAY", "MUI_HEADERIMAGE", "MUI_ICON", "!include \"/")
		if strings.Index(s, `MUI_LANGUAGE "English"`) > strings.Index(s, `MUI_LANGUAGE "SimpChinese"`) {
			t.Error("English should come first: it is first in the localizations")
		}
	})
	t.Run("machine", func(t *testing.T) {
		cfg := assocConfig(t)
		cfg.Installer.Windows = NSISInstaller{Scope: "machine", DesktopShortcut: "always", Welcome: boolp(false), RunAfterInstall: boolp(false), License: "LICENSE", Include: "build/installer.nsh", LanguageDialog: true}
		s := nsisFor(t, cfg)
		has(t, s, "RequestExecutionLevel admin", `InstallDir "$PROGRAMFILES64\Hello App"`, "SetShellVarContext all",
			`WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\com.example.hello" "InstallLocation" "$INSTDIR"`,
			`WriteRegStr HKLM "Software\Classes\hello" "URL Protocol" ""`, `DeleteRegKey HKLM "Software\Classes\hello"`,
			`CreateShortcut "$DESKTOP\Hello App.lnk" "$INSTDIR\HelloApp.exe"`,
			"!insertmacro MUI_PAGE_LICENSE "+nsisQuote(cfg.Path("LICENSE")), "!include "+nsisQuote(cfg.Path("build/installer.nsh")),
			"!insertmacro MUI_LANGDLL_DISPLAY", "!insertmacro MUI_UNGETLANGUAGE", "IfSilent tan_lang_done", `!define MUI_LANGDLL_REGISTRY_ROOT HKLM`)
		hasNot(t, s, "MUI_PAGE_WELCOME", "MUI_FINISHPAGE_RUN", "MUI_FINISHPAGE_SHOWREADME", "Function TanDesktopShortcut", "HKCU", "$LOCALAPPDATA")
		if strings.Count(s, "SetShellVarContext all") != 2 {
			t.Error("the uninstaller must also use the all-users shell folders")
		}
	})
	t.Run("never", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Installer.Windows.DesktopShortcut = "never"
		cfg.Localizations = []string{"ja"}
		s := nsisFor(t, cfg)
		hasNot(t, s, `CreateShortcut "$DESKTOP`, "SHOWREADME", `MUI_LANGUAGE "SimpChinese"`)
		has(t, s, `Delete "$DESKTOP\Hello App.lnk"`, `MUI_LANGUAGE "Japanese"`, `MUI_LANGUAGE "English"`, `LangString TanDesktopShortcut ${LANG_ENGLISH} "Create a desktop shortcut"`)
	})
	t.Run("386", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Installer.Windows.Scope = "machine"
		s := nsisScript(cfg, artifact{target: target{"windows", "386"}, exe: `C:\b\x.exe`}, `C:\b\s.exe`)
		has(t, s, `InstallDir "$PROGRAMFILES\Hello App"`)
	})
	t.Run("escaping", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Name = `Cost$ "Calc"`
		s := nsisFor(t, cfg)
		has(t, s, `Name "Cost$$ $\"Calc$\""`, `InstallDir "$LOCALAPPDATA\Programs\Cost$$ Calc"`,
			`CreateShortcut "$SMPROGRAMS\Cost$$ Calc.lnk"`, `Delete "$SMPROGRAMS\Cost$$ Calc.lnk"`, `Delete "$DESKTOP\Cost$$ Calc.lnk"`,
			`CreateShortcut "$DESKTOP\Cost$$ Calc.lnk"`)
		hasNot(t, s, `\Cost$ `)
	})
	t.Run("machine default has no run", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Installer.Windows.Scope = "machine"
		s := nsisFor(t, cfg)
		hasNot(t, s, "MUI_FINISHPAGE_RUN", "TanRunApp")
		has(t, s, "RequestExecutionLevel admin")
	})
	t.Run("machine run through explorer", func(t *testing.T) {
		cfg := testConfig(t)
		cfg.Installer.Windows.Scope = "machine"
		cfg.Installer.Windows.RunAfterInstall = boolp(true)
		s := nsisFor(t, cfg)
		has(t, s, "!define MUI_FINISHPAGE_RUN\n", "!define MUI_FINISHPAGE_RUN_FUNCTION TanRunApp",
			"Function TanRunApp\n  Exec '\"$WINDIR\\explorer.exe\" \"$INSTDIR\\HelloApp.exe\"'\nFunctionEnd")
		hasNot(t, s, `!define MUI_FINISHPAGE_RUN "`)
	})
	t.Run("user run direct", func(t *testing.T) {
		s := nsisFor(t, testConfig(t))
		has(t, s, `!define MUI_FINISHPAGE_RUN "$INSTDIR\HelloApp.exe"`)
		hasNot(t, s, "TanRunApp")
	})
}

func TestNSISTemplate(t *testing.T) {
	cfg := testConfig(t)
	cfg.Installer.Windows.Template = "build/windows/installer.nsi"
	os.MkdirAll(cfg.Path("build/windows"), 0o755)
	os.WriteFile(cfg.Path("build/windows/installer.nsi"), []byte(`Name {{q .Name}}
OutFile {{q .OutFile}}
InstallDir {{qv .InstallDir}}
; {{.Exe}} {{.Version}} {{.Scope}} {{.RegRoot}}{{range .Languages}} {{.Name}}{{end}}
`), 0o644)
	a := artifact{target: target{"windows", "amd64"}, dir: cfg.Path("build/windows-amd64"), exe: cfg.Path("build/windows-amd64/HelloApp.exe")}
	os.MkdirAll(a.dir, 0o755)
	nsi, err := writeNSISScript(cfg, a, "setup.exe")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(nsi)
	want := "\uFEFFName \"Hello App\"\nOutFile \"setup.exe\"\nInstallDir \"$LOCALAPPDATA\\Programs\\Hello App\"\n; HelloApp.exe 1.0.0 user HKCU English SimpChinese TradChinese Japanese\n"
	if string(b) != want {
		t.Errorf("template output\n%q\nwant\n%q", b, want)
	}
	os.WriteFile(cfg.Path("build/windows/installer.nsi"), []byte(`{{.Nope}}`), 0o644)
	if _, err := writeNSISScript(cfg, a, "setup.exe"); err == nil || !strings.Contains(err.Error(), "installer.windows.template") {
		t.Errorf("unknown variable: %v", err)
	}
	cfg.Installer.Windows.Template = "missing.nsi"
	if _, err := writeNSISScript(cfg, a, "setup.exe"); err == nil {
		t.Error("missing template accepted")
	}
	cfg.Installer.Windows = NSISInstaller{License: "LICENSE"}
	if _, err := writeNSISScript(cfg, a, "setup.exe"); err == nil || !strings.Contains(err.Error(), "installer.windows.license") {
		t.Errorf("missing licence: %v", err)
	}
}

func testPNG(w, h int, c color.Color) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestEncodeBMP(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255}) // top left red
	img.Set(2, 1, color.NRGBA{0, 0, 255, 255}) // bottom right blue
	img.Set(1, 0, color.NRGBA{0, 0, 0, 0})     // transparent → white
	b := encodeBMP(img)
	stride := 12 // 3*3 rounded up to 4
	if len(b) != 54+stride*2 || string(b[:2]) != "BM" {
		t.Fatalf("size %d", len(b))
	}
	// Bottom-up rows of BGR: the first row in the file is the bottom one.
	if px := b[54+6 : 54+9]; !bytes.Equal(px, []byte{255, 0, 0}) {
		t.Errorf("bottom right %v, want blue", px)
	}
	if px := b[54+stride : 54+stride+3]; !bytes.Equal(px, []byte{0, 0, 255}) {
		t.Errorf("top left %v, want red", px)
	}
	if px := b[54+stride+3 : 54+stride+6]; !bytes.Equal(px, []byte{255, 255, 255}) {
		t.Errorf("transparent %v, want white", px)
	}
}

// TestNSISCompile builds installers with makensis, when it is installed,
// for the option combinations: icon, converted images, licence, hooks.
func TestNSISCompile(t *testing.T) {
	makensis, err := exec.LookPath("makensis")
	if err != nil {
		t.Skip("makensis is not installed")
	}
	cfg := assocConfig(t)
	cfg.Icon = "icon.png"
	os.WriteFile(cfg.Path("icon.png"), defaultIcon("x"), 0o644)
	os.MkdirAll(cfg.Path("build/installer"), 0o755)
	os.WriteFile(cfg.Path("build/installer/sidebar.png"), testPNG(164, 314, color.NRGBA{20, 90, 200, 255}), 0o644)
	os.WriteFile(cfg.Path("build/installer/header.png"), testPNG(150, 57, color.NRGBA{200, 90, 20, 255}), 0o644)
	os.WriteFile(cfg.Path("LICENSE"), []byte("MIT License\n\nPermission is hereby granted…\n"), 0o644)
	os.WriteFile(cfg.Path("build/installer.nsh"), []byte("!macro customInstall\n  DetailPrint \"custom install\"\n!macroend\n!macro customInit\n!macroend\n"), 0o644)
	for _, w := range []NSISInstaller{
		{},
		{Scope: "machine", License: "LICENSE", SidebarImage: "build/installer/sidebar.png", HeaderImage: "build/installer/header.png", Include: "build/installer.nsh", LanguageDialog: true, DesktopShortcut: "always"},
		{Welcome: boolp(false), RunAfterInstall: boolp(false), DesktopShortcut: "never", Include: "build/installer.nsh"},
	} {
		cfg.Installer.Windows = w
		dir := t.TempDir()
		a := artifact{target: target{"windows", "amd64"}, dir: dir, exe: filepath.Join(dir, "HelloApp.exe")}
		os.WriteFile(a.exe, []byte("MZ"), 0o644)
		setup := filepath.Join(dir, "HelloApp-1.0.0-windows-amd64-setup.exe")
		nsi, err := writeNSISScript(cfg, a, setup)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(makensis, "-V2", "-WX", nsi).CombinedOutput(); err != nil {
			s, _ := os.ReadFile(nsi)
			t.Fatalf("%+v: makensis: %v\n%s\n%s", w, err, out, s)
		}
		if st, err := os.Stat(setup); err != nil || st.Size() < 10000 {
			t.Errorf("%+v: setup %v", w, err)
		}
		if w.SidebarImage != "" {
			for _, f := range []string{"installer.ico", "installer-sidebar.bmp", "installer-header.bmp"} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Error(err)
				}
			}
		}
	}
}

// nsisTestZip is a zip shaped like the NSIS release.
func nsisTestZip(t *testing.T, entries map[string]string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, name := range sortedKeys(entries) {
		w, _ := zw.Create(name)
		w.Write([]byte(entries[name]))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCachedMakensis(t *testing.T) {
	good := nsisTestZip(t, map[string]string{
		"nsis-" + nsisVersion + "/makensis.exe":     "MZstub",
		"nsis-" + nsisVersion + "/Bin/makensis.exe": "MZ",
	})
	evil := nsisTestZip(t, map[string]string{"../evil.exe": "x"})
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/nsis.zip":
			w.Write(good)
		case "/evil.zip":
			w.Write(evil)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	sum := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	set := func(url, s string) {
		oldURL, oldSum := nsisURL, nsisSum
		nsisURL, nsisSum = url, s
		t.Cleanup(func() { nsisURL, nsisSum = oldURL, oldSum })
	}
	ctx := context.Background()

	dir := t.TempDir()
	set(srv.URL+"/nsis.zip", strings.Repeat("0", 64))
	if _, err := cachedMakensis(ctx, srv.Client(), dir); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("bad checksum: %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("failed download left %v", ents)
	}

	set(srv.URL+"/nsis.zip", sum(good))
	exe, err := cachedMakensis(ctx, srv.Client(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "MZstub" || exe != filepath.Join(dir, "nsis-"+nsisVersion, "makensis.exe") {
		t.Errorf("makensis %s %q", exe, b)
	}
	before := hits
	if again, err := cachedMakensis(ctx, srv.Client(), dir); err != nil || again != exe || hits != before {
		t.Errorf("cache not reused: %v, %d downloads", err, hits-before)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("cache holds %v", ents)
	}

	set(srv.URL+"/missing.zip", sum(good))
	if _, err := cachedMakensis(ctx, srv.Client(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("404: %v", err)
	}
	set(srv.URL+"/evil.zip", sum(evil))
	if _, err := cachedMakensis(ctx, srv.Client(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "leaves") {
		t.Errorf("zip slip: %v", err)
	}
	// A server that stalls is cut off by the context.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	set(slow.URL, sum(good))
	cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := cachedMakensis(cctx, slow.Client(), t.TempDir()); err == nil {
		t.Error("stalled download succeeded")
	}
	if !strings.Contains(makensisHint("darwin"), "brew install makensis") || !strings.Contains(makensisHint("linux"), "apt install nsis") {
		t.Error("install hints")
	}
}
