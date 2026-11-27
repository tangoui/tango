package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIMEConfigWindows(t *testing.T) {
	cfg := &Config{Name: "Pinyin", ID: "com.example.inputmethod.pinyin", Version: "1.0.0", InputMethod: &InputMethod{}}
	first, err := cfg.windowsIME()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := cfg.windowsIME()
	if first != second || first.CLSID == first.Profile || !imeGUIDPattern.MatchString(first.CLSID) ||
		first.CLSID[15] != '5' || first.LangID != "0804" {
		t.Fatalf("defaults = %+v, duplicate = %+v", first, second)
	}
	cfg.InputMethod.Windows = &IMEWindows{CLSID: "invalid"}
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "clsid") {
		t.Fatalf("validate: %v", err)
	}
}

func TestNSISIMEScript(t *testing.T) {
	cfg := &Config{Name: "Pinyin", ID: "com.example.inputmethod.pinyin", Version: "1.0.0", InputMethod: &InputMethod{}}
	a := artifact{target: target{"windows", "amd64"}, exe: "Pinyin.exe",
		ime: &imePackage{DLL: nsisHelper{Name: "TangoIMEBridge-1.0.0-x64.dll", Source: "/test/TangoIMEBridge-1.0.0-x64.dll"}},
	}
	script := nsisScript(cfg, a, "setup.exe")
	for _, want := range []string{
		"RequestExecutionLevel admin", "SetRegView 64",
		`WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Run"`,
		`"$SYSDIR\regsvr32.exe" /s "$INSTDIR\TangoIMEBridge-1.0.0-x64.dll"`,
		`"$SYSDIR\regsvr32.exe" /s /u`,
		`Delete /REBOOTOK "$INSTDIR\`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestIMEBridgeDownload(t *testing.T) {
	t.Setenv("TANGO_IME_BRIDGE_DIR", "")
	t.Setenv("GOWORK", "off")
	proxy := t.TempDir()
	module := filepath.Join(proxy, "github.com", "tangoui", "tango", "@v")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"v1.2.3.mod":  "module github.com/tangoui/tango\n\ngo 1.27\n",
		"v1.2.3.info": `{"Version":"v1.2.3","Time":"2026-01-01T00:00:00Z"}`,
	} {
		if err := os.WriteFile(filepath.Join(module, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOPROXY", (&url.URL{Scheme: "file", Path: filepath.ToSlash(proxy)}).String())
	t.Setenv("GOSUMDB", "off")
	dir := t.TempDir()
	cfg := &Config{dir: dir}
	if err := os.WriteFile(cfg.Path("go.mod"), []byte("module example.com/ime\n\ngo 1.27\n\nrequire github.com/tangoui/tango v1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	data := []byte("test release DLL")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1.2.3/TangoIMEBridge-x64.dll" {
			t.Errorf("download URL = %s", r.URL.Path)
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	originalURL, originalSums := imeBridgeBaseURL, imeBridgeSums
	defer func() { imeBridgeBaseURL, imeBridgeSums = originalURL, originalSums }()
	imeBridgeBaseURL = server.URL
	imeBridgeSums = "v1.2.3 x64 " + strings.Repeat("0", 64)
	_, err := imeBridgeFile(context.Background(), cfg, "x64", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("unverified download: %v", err)
	}
	imeBridgeSums = fmt.Sprintf("v1.2.3 x64 %x", sha256.Sum256(data))
	path, err := imeBridgeFile(context.Background(), cfg, "x64", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
		t.Fatalf("verified file = %q, %v", got, err)
	}
	local := filepath.Join(dir, "TangoIMEBridge-x64.dll")
	if err := os.WriteFile(local, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TANGO_IME_BRIDGE_DIR", dir)
	if got, err := imeBridgeFile(context.Background(), cfg, "x64", dir); got != local || err != nil || requests.Load() != 2 {
		t.Fatalf("local path = %q, %v; requests=%d", got, err, requests.Load())
	}
}

func TestNSISIMECompile(t *testing.T) {
	builder, err := exec.LookPath("makensis")
	if err != nil {
		t.Skip("makensis not installed")
	}
	dir := t.TempDir()
	cfg := &Config{Name: "Pinyin", ID: "com.example.inputmethod.pinyin", Version: "1.0.0",
		InputMethod: &InputMethod{Modes: []InputMode{{ID: "zh", Label: "中文", Short: "中"}}}, dir: dir}
	bridge := filepath.Join(dir, "TangoIMEBridge-x64.dll")
	a := artifact{target: target{"windows", "amd64"}, exe: filepath.Join(dir, "Pinyin.exe"), dir: dir}
	for _, path := range []string{bridge, a.exe} {
		if err := os.WriteFile(path, []byte("MZ"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TANGO_IME_BRIDGE_DIR", dir)
	a.ime, err = prepareIMEPackage(context.Background(), cfg, a)
	if err != nil {
		t.Fatal(err)
	}
	setup := filepath.Join(dir, "Pinyin-setup.exe")
	script, err := writeNSISScript(cfg, a, setup)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(builder, "-V2", "-WX", script).CombinedOutput(); err != nil {
		t.Fatalf("makensis: %v\n%s", err, out)
	}
}

func TestNSISWithoutIMEGolden(t *testing.T) {
	cfg := &Config{Name: "Pinyin", ID: "com.example.pinyin", Version: "1.0.0"}
	a := artifact{target: target{"windows", "amd64"}, exe: "Pinyin.exe"}
	const want = "a435140b8e6d3e93564796e0c20bec3692269987effe713f65e1470e5a53ad9d"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(nsisScript(cfg, a, "Pinyin-setup.exe")))); got != want {
		t.Fatalf("ordinary installer SHA-256 = %s, want %s", got, want)
	}
}
