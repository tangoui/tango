package main

import (
	"strings"
	"testing"
)

// The bundle declares its localizations, so AppKit translates the menu
// items it names itself (Undo/Redo, Start Dictation, ...).
func TestInfoPlistLocalizations(t *testing.T) {
	got := string(infoPlist(&Config{Name: "A", ID: "com.example.a", Version: "1.0.0"}, ""))
	for _, want := range []string{
		"<key>CFBundleDevelopmentRegion</key>\n\t<string>en</string>",
		"<key>CFBundleLocalizations</key>\n\t<array>\n\t\t<string>en</string>\n\t\t<string>zh-Hans</string>\n\t\t<string>zh-Hant</string>\n\t\t<string>ja</string>\n\t</array>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Info.plist lacks\n%s\n---\n%s", want, got)
		}
	}
}

func TestLocalizationsOverrideAndIOS(t *testing.T) {
	cfg := &Config{Name: "A", ID: "com.example.a", Version: "1.0.0", Localizations: []string{"en", "fr"}}
	mac := string(infoPlist(cfg, ""))
	if !strings.Contains(mac, "<string>fr</string>") || strings.Contains(mac, "<string>zh-Hans</string>") {
		t.Errorf("macOS Info.plist ignores localizations override:\n%s", mac)
	}
	if got := (&Config{}).AppleLocalizations(); len(got) != 4 || got[1] != "zh-Hans" {
		t.Errorf("default localizations = %v", got)
	}
	p := iosInfoPlist(&Config{Name: "A", ID: "com.example.a", Version: "1.0.0"}, iosSDK{platform: "iPhoneOS"}, nil)
	found := false
	for _, e := range p {
		if e.key == "CFBundleLocalizations" {
			found = true
		}
	}
	if !found {
		t.Error("iOS Info.plist lacks CFBundleLocalizations")
	}
}
