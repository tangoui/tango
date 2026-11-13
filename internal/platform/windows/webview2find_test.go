package windows

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRuntime lays out a WebView2 runtime directory for goarch.
func fakeRuntime(t *testing.T, root, version, goarch string, withExe bool) string {
	t.Helper()
	dir := filepath.Join(root, version)
	dll := filepath.Join(dir, webview2ClientDir, webview2ArchDir(goarch), webview2ClientDLL)
	if err := os.MkdirAll(filepath.Dir(dll), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dll, nil, 0o644)
	if withExe {
		os.WriteFile(filepath.Join(dir, webview2BrowserExe), nil, 0o644)
	}
	return dir
}

func TestFindWebView2(t *testing.T) {
	root := t.TempDir()
	oldV := fakeRuntime(t, root, "120.0.2210.91", "amd64", true)
	newV := fakeRuntime(t, root, "131.0.2903.70", "amd64", true)
	edgeOnly := fakeRuntime(t, root, "999.0.0.0", "amd64", false)
	stable := webview2Channels[0]
	reg := map[string]string{}
	read := func(machine bool, path, name string) string {
		if name != webview2ClientDir {
			t.Errorf("value name %q", name)
		}
		k := "user:"
		if machine {
			k = "machine:"
		}
		return reg[k+path]
	}
	key := `SOFTWARE\Microsoft\EdgeUpdate\ClientState\`

	if _, _, err := findWebView2("", "amd64", read); err != errWebView2NotInstalled {
		t.Errorf("none = %v", err)
	}
	reg["machine:"+key+stable] = oldV
	reg["user:"+key+stable] = newV
	if dir, fixed, err := findWebView2("", "amd64", read); err != nil || dir != newV || fixed {
		t.Errorf("newest = %q %v %v", dir, fixed, err)
	}
	// Wrong architecture: the arm64 DLL is missing.
	if _, _, err := findWebView2("", "arm64", read); err == nil || !strings.Contains(err.Error(), "missing or incomplete") {
		t.Errorf("arch = %v", err)
	}
	// A directory with the DLL but no msedgewebview2.exe is Edge, not the runtime.
	reg = map[string]string{"machine:" + key + stable: edgeOnly}
	if _, _, err := findWebView2("", "amd64", read); err == nil {
		t.Error("edge-only directory accepted")
	}
	// Beta channel used when stable is absent.
	reg = map[string]string{"user:" + key + webview2Channels[1]: oldV}
	if dir, _, err := findWebView2("", "amd64", read); err != nil || dir != oldV {
		t.Errorf("beta = %q %v", dir, err)
	}
	// Fixed version runtime.
	if dir, fixed, err := findWebView2(newV, "amd64", read); err != nil || !fixed || dir != newV {
		t.Errorf("fixed = %q %v %v", dir, fixed, err)
	}
	if _, _, err := findWebView2(edgeOnly, "amd64", read); err == nil {
		t.Error("bad fixed folder accepted")
	}
}

func TestVersions(t *testing.T) {
	if compareVersions(parseVersion("131.0.2903.70"), parseVersion("131.0.2903.9")) <= 0 {
		t.Error("numeric compare")
	}
	if compareVersions(parseVersion("1.2"), parseVersion("1.2.0")) != 0 {
		t.Error("trailing zero")
	}
	if parseVersion("1.x") != nil {
		t.Error("bad version")
	}
	if webview2ArchDir("386") != "x86" || webview2ArchDir("amd64") != "x64" || webview2ArchDir("arm64") != "arm64" {
		t.Error("arch dirs")
	}
}

func TestUserDataDir(t *testing.T) {
	env := map[string]string{"LOCALAPPDATA": filepath.FromSlash("/home/u/AppData/Local")}
	get := func(k string) string { return env[k] }
	if got := userDataDir(get, "/exe", "com.example.app", "My App"); got != filepath.Join(env["LOCALAPPDATA"], "com.example.app", "WebView2") {
		t.Errorf("by ID = %q", got)
	}
	if got := userDataDir(get, "/exe", "", `My: App?`); got != filepath.Join(env["LOCALAPPDATA"], "My_ App_", "WebView2") {
		t.Errorf("by name = %q", got)
	}
	if got := userDataDir(get, "/exe", "", ""); !strings.HasSuffix(got, filepath.Join("Tango", "WebView2")) {
		t.Errorf("fallback = %q", got)
	}
	env["WEBVIEW2_USER_DATA_FOLDER"] = "data"
	if got := userDataDir(get, filepath.FromSlash("/exe"), "x", ""); got != filepath.Join(filepath.FromSlash("/exe"), "data") {
		t.Errorf("relative override = %q", got)
	}
	abs := filepath.Join(t.TempDir(), "wv")
	env["WEBVIEW2_USER_DATA_FOLDER"] = abs
	if got := userDataDir(get, "/exe", "x", ""); got != abs {
		t.Errorf("absolute override = %q", got)
	}
	if sanitizeFileName(" . ") != "Tango" || sanitizeFileName("a/b\\c") != "a_b_c" {
		t.Error("sanitize")
	}
}
