package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
