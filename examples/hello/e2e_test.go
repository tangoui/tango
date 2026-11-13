package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/automation"
)

// TestE2EWebView drives the WebView page through the automation channel:
// evaluate JavaScript, fill the input, click the button and wait for the
// Go round trip to show up. Skipped unless TANGO_E2E is set:
//
//	TANGO_E2E=1 go test ./examples/hello -run E2E -v
func TestE2EWebView(t *testing.T) {
	app := automation.Launch(t, ".")
	win := app.Window("Tango Hello")
	if win.Kind() != automation.KindWebView {
		t.Fatalf("kind %q", win.Kind())
	}
	win.WaitJS(`document.readyState === "complete" && !!window.tango`)
	if got := win.EvalString(`document.title`); got != "Tango Hello" {
		t.Fatalf("title %q", got)
	}
	var sum int
	win.EvalInto(`new Promise(r => setTimeout(() => r(20 + 22), 10))`, &sum)
	if sum != 42 {
		t.Fatalf("awaited value %d", sum)
	}

	win.Fill("#name", "自动化")
	win.ClickSelector("#go")
	win.WaitJS(`document.getElementById("out").textContent.includes("你好，自动化！")`)
	if out := win.TextOf("#out"); !strings.Contains(out, "stream: tick 5") {
		t.Fatalf("output %q", out)
	}

	shot := win.Screenshot()
	if len(shot) < 1000 {
		t.Fatalf("screenshot %d bytes", len(shot))
	}
	if dir := os.Getenv("HELLO_SHOT"); dir != "" {
		os.WriteFile(filepath.Join(dir, "hello-e2e.png"), shot, 0o644)
	}
	app.Quit()
}
