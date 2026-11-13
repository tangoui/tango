package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// iconProject writes tan.json with icon settings (a JSON fragment) into a
// temporary project and returns its config.
func iconProject(t *testing.T, fragment string) *Config {
	t.Helper()
	dir := t.TempDir()
	js := `{"name": "Hello", "id": "com.example.hello", "version": "1.0.0", "frontend": {"dir": "web", "dist": "."}` + fragment + `}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// findCheck returns the rows named name whose detail contains sub.
func findCheck(cs []check, name, sub string) []check {
	var out []check
	for _, c := range cs {
		if c.name == name && strings.Contains(c.detail, sub) {
			out = append(out, c)
		}
	}
	return out
}

func wantCheck(t *testing.T, cs []check, name string, st checkStatus, sub string) {
	t.Helper()
	got := findCheck(cs, name, sub)
	if len(got) != 1 || got[0].status != st {
		t.Errorf("want one %s row %q containing %q; got %+v", st, name, sub, cs)
	}
}

func TestIconChecksNoProject(t *testing.T) {
	if cs := iconChecks(t.TempDir()); cs != nil {
		t.Errorf("checks outside a project: %+v", cs)
	}
}

func TestIconChecksGood(t *testing.T) {
	cfg := iconProject(t, `, "icon": "icon.png", "icons": {"windows": "app.ico", "android": {"background": "#1677FF"}}`)
	saveTestPNG(t, cfg, "icon.png", iconTestImage(1024, 1024, testBlue, testOrange, 0.5))
	os.WriteFile(cfg.Path("app.ico"), []byte("ico"), 0o644)
	cs := iconChecks(cfg.dir)
	if len(cs) != 1 || cs[0].status != statusOK {
		t.Errorf("good project: %+v", cs)
	}
}

func TestIconChecksMaster(t *testing.T) {
	cs := iconChecks(iconProject(t, ``).dir)
	wantCheck(t, cs, "Icon", statusWarn, `no "icon"`)

	cs = iconChecks(iconProject(t, `, "icon": "missing.png"`).dir)
	wantCheck(t, cs, "Icon", statusError, "does not exist")

	cfg := iconProject(t, `, "icon": "icon.png"`)
	os.WriteFile(cfg.Path("icon.png"), []byte("GIF89a not a png"), 0o644)
	wantCheck(t, iconChecks(cfg.dir), "Icon", statusError, "not a valid PNG")

	saveTestPNG(t, cfg, "icon.png", iconTestImage(1024, 512, testBlue, nil, 0))
	wantCheck(t, iconChecks(cfg.dir), "Icon", statusError, "must be square")

	saveTestPNG(t, cfg, "icon.png", iconTestImage(512, 512, testBlue, nil, 0))
	wantCheck(t, iconChecks(cfg.dir), "Icon", statusWarn, "use 1024x1024")

	// A shaped (rounded, transparent-margin) icon given as the master.
	saveTestPNG(t, cfg, "icon.png", iconTestImage(1024, 1024, testClear, testBlue, 0.8))
	wantCheck(t, iconChecks(cfg.dir), "Icon", statusWarn, "transparent corners")
}

func TestIconChecksDesktop(t *testing.T) {
	cfg := iconProject(t, `, "icon": "icon.png", "icons": {"macos": "mac.png", "windows": "win.png", "linux": {"app": "linux.png", "symbolic": "s.svg"}}`)
	saveTestPNG(t, cfg, "icon.png", iconTestImage(1024, 1024, testBlue, nil, 0))
	saveTestPNG(t, cfg, "mac.png", iconTestImage(512, 512, testBlue, nil, 0))
	saveTestPNG(t, cfg, "win.png", iconTestImage(128, 128, testBlue, nil, 0))
	cs := iconChecks(cfg.dir)
	wantCheck(t, cs, "Icon (macOS)", statusWarn, "should be 1024x1024")
	wantCheck(t, cs, "Icon (macOS)", statusWarn, "opaque corners")
	wantCheck(t, cs, "Icon (Windows)", statusWarn, "256 px or more")
	wantCheck(t, cs, "Icon (Linux)", statusError, "linux.png does not exist")
	wantCheck(t, cs, "Icon (Linux)", statusError, "s.svg does not exist")

	saveTestPNG(t, cfg, "mac.png", iconTestImage(1024, 1024, testClear, testBlue, 0.8))
	saveTestPNG(t, cfg, "win.png", iconTestImage(256, 256, testClear, testBlue, 0.8))
	saveTestPNG(t, cfg, "linux.png", iconTestImage(512, 512, testClear, testBlue, 0.8))
	os.WriteFile(cfg.Path("s.svg"), []byte("<svg/>"), 0o644)
	if cs := iconChecks(cfg.dir); len(cs) != 1 || cs[0].status != statusOK {
		t.Errorf("fixed desktop icons: %+v", cs)
	}
}

func TestIconChecksMobile(t *testing.T) {
	cfg := iconProject(t, `, "icon": "icon.png", "icons": {
		"ios": {"dark": "dark.png", "tinted": "tinted.png"},
		"android": {"foreground": "fg.png", "background": "bg.png", "monochrome": "mono.png"},
		"notification": "notify.png"}`)
	saveTestPNG(t, cfg, "icon.png", iconTestImage(1024, 1024, testBlue, nil, 0))
	saveTestPNG(t, cfg, "dark.png", iconTestImage(512, 512, testClear, testBlue, 0.5))
	saveTestPNG(t, cfg, "tinted.png", iconTestImage(1024, 900, color.Gray{0x80}, nil, 0))
	saveTestPNG(t, cfg, "fg.png", iconTestImage(432, 432, testClear, testOrange, 0.9)) // past 66/108
	saveTestPNG(t, cfg, "bg.png", iconTestImage(432, 300, testBlue, nil, 0))
	saveTestPNG(t, cfg, "mono.png", iconTestImage(432, 432, testClear, testOrange, 0.5))
	saveTestPNG(t, cfg, "notify.png", iconTestImage(96, 96, testOrange, nil, 0))
	cs := iconChecks(cfg.dir)
	wantCheck(t, cs, "Icon (iOS)", statusWarn, `"icons.ios.dark" should be 1024x1024`)
	wantCheck(t, cs, "Icon (iOS)", statusError, `"icons.ios.tinted" must be square`)
	wantCheck(t, cs, "Icon (Android)", statusWarn, "outside the centre 66/108")
	wantCheck(t, cs, "Icon (Android)", statusError, `"icons.android.background" must be square`)
	wantCheck(t, cs, "Icon (Android)", statusWarn, "coloured pixels")
	wantCheck(t, cs, "Icon (notification)", statusError, "white square")

	saveTestPNG(t, cfg, "notify.png", iconTestImage(96, 96, testClear, testOrange, 0.6))
	wantCheck(t, iconChecks(cfg.dir), "Icon (notification)", statusWarn, "coloured pixels")

	saveTestPNG(t, cfg, "dark.png", iconTestImage(1024, 1024, testClear, testBlue, 0.5))
	saveTestPNG(t, cfg, "tinted.png", iconTestImage(1024, 1024, color.Gray{0x80}, nil, 0))
	saveTestPNG(t, cfg, "fg.png", iconTestImage(432, 432, testClear, testOrange, 0.6))
	saveTestPNG(t, cfg, "bg.png", iconTestImage(432, 432, testBlue, nil, 0))
	saveTestPNG(t, cfg, "mono.png", iconTestImage(432, 432, testClear, color.White, 0.6))
	saveTestPNG(t, cfg, "notify.png", iconTestImage(96, 96, testClear, color.White, 0.6))
	if cs := iconChecks(cfg.dir); len(cs) != 1 || cs[0].status != statusOK {
		t.Errorf("fixed mobile icons: %+v", cs)
	}
}

func TestIconChecksBadConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`{`), 0o644)
	if cs := iconChecks(dir); len(cs) != 1 || cs[0].status != statusError {
		t.Errorf("broken tan.json: %+v", cs)
	}
}
