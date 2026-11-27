package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPE2EWebKey(t *testing.T) {
	e := newE2ETan(t)
	cs, _ := e.connectStdio(t)
	app, pid := e.startApp(t, cs, "./internal/automation/testdata/webkey", "Web Key Test")
	before := frontmost(t)
	args := func() map[string]any {
		return map[string]any{"session": app.Session, "window": "Web Key Test"}
	}
	call := func(tool, param string, value any, out any) {
		t.Helper()
		a := args()
		a[param] = value
		callOK(t, cs, tool, a, out)
	}
	wait := func(expr string) {
		t.Helper()
		call("tango_web_wait", "expr", expr, nil)
	}
	key := func(chord string) {
		t.Helper()
		t.Logf("tango_web_key %s", chord)
		call("tango_web_key", "keys", chord, nil)
	}
	eval := func(expr string) {
		t.Helper()
		call("tango_web_eval", "expr", expr, nil)
	}
	wait("Array.isArray(window.keyEvents)")
	call("tango_web_click", "selector", "#editor", nil)
	key("Shift+K")
	wait("editor.value === 'K' && keyEvents.includes('keydown K KeyK true Shift') && keyEvents.includes('keyup K KeyK true Shift')")
	key("CmdOrCtrl+Shift+K")
	wait("keyEvents.some(e => /^keydown K KeyK true (Meta|Ctrl) Shift$/.test(e)) && keyEvents.some(e => /^keyup K KeyK true (Meta|Ctrl) Shift$/.test(e))")
	key("CmdOrCtrl+S")
	wait("document.querySelector('#save').textContent.endsWith(': 1')")
	// Menu callbacks reach the page through ordered Evals: once Save's has
	// run, a wrongly fired Disabled callback would have run too.
	var disabled struct{ Value int }
	call("tango_web_eval", "expr", "disabledClicks", &disabled)
	if disabled.Value != 0 {
		t.Fatalf("disabled menu item ran %d times", disabled.Value)
	}
	if runtime.GOOS == "linux" {
		key("Meta+J")
		wait("keyEvents.includes('keydown j KeyJ true Meta') && keyEvents.includes('keyup j KeyJ true Meta')")
	}
	eval("editor.value = 'replace me'")
	// On macOS only the Edit menu selects with Command-A. Chromium also
	// handles Ctrl+A itself, so on Windows this cannot tell the menu route
	// apart; TestWindowsWebKeyMenu covers the attached editing command.
	key("CmdOrCtrl+A")
	wait("editor.selectionStart === 0 && editor.selectionEnd === editor.value.length")
	eval("document.querySelector('#edit').textContent = 'Select All: ' + editor.selectionStart + '-' + editor.selectionEnd")
	// Clipboard verification is opt-in for an isolated desktop (e.g. Xvfb).
	testClipboard := os.Getenv("TANGO_WEB_KEY_TEST_CLIPBOARD") == "1"
	if testClipboard {
		key("CmdOrCtrl+C")
	}
	key("Shift+Z")
	wait("editor.value === 'Z'")
	if testClipboard {
		key("CmdOrCtrl+V")
		wait("editor.value === 'Zreplace me'")
		t.Log("Edit menu Copy and Paste preserved the selected text")
	}
	// A lone modifier reports the side-neutral keyCode, with its side in
	// location, as a real keyboard does.
	eval("(() => { window.modifierCodes = []; addEventListener('keydown', e => modifierCodes.push([e.key, e.code, e.keyCode, e.location].join(' '))); return true })()")
	for _, k := range []string{"ShiftLeft", "ShiftRight", "CtrlLeft", "CtrlRight", "AltLeft", "AltRight"} {
		key(k)
	}
	wait("modifierCodes.join('|') === 'Shift ShiftLeft 16 1|Shift ShiftRight 16 2|Control ControlLeft 17 1|Control ControlRight 17 2|Alt AltLeft 18 1|Alt AltRight 18 2'")
	var result struct {
		Value struct {
			Events            []string
			Input, Save, Edit string
		}
	}
	call("tango_web_eval", "expr", "({events: keyEvents, input: editor.value, save: document.querySelector('#save').textContent, edit: document.querySelector('#edit').textContent})", &result)
	t.Logf("page events:\n%s\ninput=%q\n%s\n%s", strings.Join(result.Value.Events, "\n"), result.Value.Input, result.Value.Save, result.Value.Edit)
	var output appOutputOutput
	callOK(t, cs, "tango_app_output", map[string]any{"session": app.Session}, &output)
	if !strings.Contains(output.Output, "native Save OnClick") {
		t.Fatalf("missing native menu callback in app output: %+v", output)
	}
	t.Log("application output confirmed native Save OnClick")
	if after := frontmost(t); after != "" && before != "" {
		t.Logf("frontmost before keys: %q, after keys: %q", before, after)
		if after == "app" || strings.EqualFold(after, "Web Key Test") {
			t.Errorf("WebView key dispatch brought the test app to the foreground")
		}
	}
	if dir := os.Getenv("TANGO_WEB_KEY_ARTIFACTS"); dir != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "tango_ui_screenshot", Arguments: args()})
		if err != nil || r.IsError {
			t.Fatalf("WebView screenshot: %v, %+v", err, r)
		}
		var screenshot []byte
		for _, c := range r.Content {
			if img, ok := c.(*mcp.ImageContent); ok {
				screenshot = img.Data
				break
			}
		}
		if len(screenshot) == 0 {
			t.Fatal("WebView screenshot returned no image")
		}
		name := runtime.GOOS
		if name == "darwin" {
			name = "macos"
		}
		path := filepath.Join(dir, name+"-web-key.png")
		if err := os.WriteFile(path, screenshot, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("saved %s", path)
	}
	if runtime.GOOS == "linux" {
		key("CmdOrCtrl+Shift+Y")
		a := args()
		a["keys"] = "Shift+K"
		if msg := callErr(t, cs, "tango_web_key", a); !strings.Contains(msg, "WebView cannot dispatch key") {
			t.Errorf("hidden WebView key error = %q", msg)
		}
	}
	appStop(t, cs, app.Session)
	if processAlive(pid) {
		t.Fatalf("app process %d is still running", pid)
	}
}
