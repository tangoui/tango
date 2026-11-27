package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInfoPlistInputMethod(t *testing.T) {
	cfg := &Config{Name: "Toy", ID: "com.example.inputmethod.toy", Version: "1.0.0"}
	cfg.MacOS.InputMethod = &InputMethod{Language: "zh-Hans"}
	data := infoPlist(cfg, "icon")
	for _, want := range []string{
		"<key>InputMethodConnectionName</key>\n\t<string>com.example.inputmethod.toy_Connection</string>",
		"<key>InputMethodServerControllerClass</key>\n\t<string>TangoInputController</string>",
		"<key>LSBackgroundOnly</key>\n\t<true/>",
		"<key>TISIntendedLanguage</key>\n\t<string>zh-Hans</string>",
		"<key>tsInputMethodCharacterRepertoireKey</key>\n\t<array>\n\t\t<string>Hans</string>\n\t</array>",
		"<key>tsInputMethodIconFileKey</key>\n\t<string>icon.icns</string>",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("Info.plist lacks %q", want)
		}
	}
	if plutil, err := exec.LookPath("plutil"); err == nil {
		p := filepath.Join(t.TempDir(), "Info.plist")
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(plutil, "-lint", p).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v: %s", err, out)
		}
	}
}

func TestInfoPlistNoInputMethod(t *testing.T) {
	cfg := &Config{Name: "Toy", ID: "com.example.toy", Version: "1.0.0"}
	got := string(infoPlist(cfg, ""))
	for _, key := range []string{"InputMethodConnectionName", "InputMethodServerControllerClass",
		"LSBackgroundOnly", "TISIntendedLanguage", "tsInputMethodCharacterRepertoireKey", "tsInputMethodIconFileKey"} {
		if strings.Contains(got, "<key>"+key+"</key>") {
			t.Errorf("Info.plist unexpectedly contains %s", key)
		}
	}
}

func TestInputMethodIDCheck(t *testing.T) {
	cfg := &Config{Name: "Toy", ID: "com.example.toy", Version: "1.0.0"}
	cfg.MacOS.InputMethod = &InputMethod{}
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), ".inputmethod.") {
		t.Fatalf("validate() = %v, want an error requiring .inputmethod.", err)
	}
	cfg.ID = "com.example.inputmethod.toy"
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInputMethodBundleIcon(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Name: "Toy", ID: "com.example.inputmethod.toy", Version: "1.0.0", dir: dir}
	cfg.MacOS.InputMethod = &InputMethod{Connection: "ToyConnection", Icon: "custom.png"}
	if err := os.WriteFile(cfg.Path("custom.png"), []byte("icon bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "Toy.app")
	if err := writeBundle(cfg, app); err != nil {
		t.Fatal(err)
	}
	icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "menuicon.png"))
	if err != nil || string(icon) != "icon bytes" {
		t.Fatalf("menuicon.png = %q, %v", icon, err)
	}
	data, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<key>InputMethodConnectionName</key>\n\t<string>ToyConnection</string>",
		"<key>tsInputMethodIconFileKey</key>\n\t<string>menuicon.png</string>",
		"<key>TISIntendedLanguage</key>\n\t<string>zh-Hans</string>",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("Info.plist lacks %q", want)
		}
	}
}

func TestRepertoire(t *testing.T) {
	for lang, want := range map[string]string{"zh-Hans": "Hans", "zh-Hant": "Hant", "ja": "Jpan", "en": "Latn", "": "Hans"} {
		if got := repertoire(lang); got != want {
			t.Errorf("repertoire(%q) = %q, want %q", lang, got, want)
		}
	}
}

func TestInputMethodTopLevel(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"name":"Toy","id":"com.example.inputmethod.toy","version":"1.0.0","inputMethod":{"connection":"ToyConnection","language":"ja","icon":"custom.png","windows":{"clsid":"{12345678-1234-1234-1234-123456789abc}","profile":"{87654321-4321-4321-4321-abcdefabcdef}"},"linux":{"engine":"toy"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	legacy := *cfg
	legacy.InputMethod = nil
	legacy.MacOS.InputMethod = cfg.InputMethod
	if got, want := string(infoPlist(cfg, "")), string(infoPlist(&legacy, "")); got != want {
		t.Fatalf("top-level and legacy Info.plist differ:\n%s", got)
	}
}

func TestInputMethodAliasConflict(t *testing.T) {
	_, err := ParseConfig([]byte(`{"name":"Toy","id":"com.example.inputmethod.toy","version":"1.0.0","inputMethod":{},"macos":{"inputMethod":{}}}`))
	if err == nil || !strings.Contains(err.Error(), "macos.inputMethod") {
		t.Fatalf("ParseConfig = %v, want a conflict error", err)
	}
}

func TestInputModesPlist(t *testing.T) {
	cfg := &Config{Name: "Toy", ID: "com.example.inputmethod.toy", Version: "1.0.0",
		InputMethod: &InputMethod{Modes: []InputMode{
			{ID: "zh", Label: "中文", Short: "中", Icon: "zh.tiff"},
			{ID: "en", Label: "English & Latin", Short: "英"},
		}},
	}
	got := string(infoPlist(cfg, "icon"))
	for _, want := range []string{
		"<key>ComponentInputModeDict</key>",
		"<key>tsInputModeListKey</key>",
		"<key>com.example.inputmethod.toy.zh</key>",
		"<key>com.example.inputmethod.toy.en</key>",
		"<key>TISInputSourceID</key>\n\t\t\t\t<string>com.example.inputmethod.toy.en</string>",
		"<key>TISIntendedLanguage</key>\n\t\t\t\t<string>zh-Hans</string>",
		"<key>TISIntendedLanguage</key>\n\t\t\t\t<string>en</string>",
		"<key>tsInputModeScriptKey</key>\n\t\t\t\t<string>smUnicodeScript</string>",
		"<key>tsInputModeMenuIconFileKey</key>\n\t\t\t\t<string>mode-zh.tiff</string>",
		"<key>tsInputModeMenuIconFileKey</key>\n\t\t\t\t<string>icon.icns</string>",
		"<key>tsVisibleInputModeOrderedArrayKey</key>\n\t\t<array>\n\t\t\t<string>com.example.inputmethod.toy.zh</string>\n\t\t\t<string>com.example.inputmethod.toy.en</string>",
		"<key>TangoInputModes</key>",
		"<key>id</key>\n\t\t\t<string>zh</string>",
		"<key>label</key>\n\t\t\t<string>English &amp; Latin</string>",
		"<key>short</key>\n\t\t\t<string>中</string>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Info.plist lacks %q", want)
		}
	}
	if runtime.GOOS == "darwin" {
		path := filepath.Join(t.TempDir(), "Info.plist")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v: %s", err, out)
		}
	}
}

func TestInputModesIcons(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Name: "Toy", ID: "com.example.inputmethod.toy", Version: "1.0.0", dir: dir,
		InputMethod: &InputMethod{Modes: []InputMode{{ID: "zh", Label: "中文", Short: "中", Icon: "assets/mode.png"}}},
	}
	app := filepath.Join(dir, "Toy.app")
	if err := writeBundle(cfg, app); err == nil || !strings.Contains(err.Error(), "assets/mode.png") {
		t.Fatalf("writeBundle = %v, want missing icon error", err)
	}
	if err := os.MkdirAll(cfg.Path("assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Path("assets/mode.png"), []byte("mode icon"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeBundle(cfg, app); err != nil {
		t.Fatal(err)
	}
	if icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "mode-zh.png")); err != nil || string(icon) != "mode icon" {
		t.Fatalf("mode-zh.png = %q, %v", icon, err)
	}
}

func TestInputModesValidation(t *testing.T) {
	cfg := &Config{Name: "Toy", ID: "com.example.inputmethod.toy", Version: "1.0.0",
		InputMethod: &InputMethod{Modes: []InputMode{
			{ID: "zh", Label: "中文", Short: "中"},
			{ID: "zh", Label: "duplicate", Short: "d"},
			{ID: "../invalid", Icon: "/tmp/icon.png"},
		}},
	}
	err := cfg.validate()
	for _, want := range []string{"duplicate", "../invalid", ".label", ".short", ".icon"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("validate() = %v, want error containing %q", err, want)
		}
	}
}
