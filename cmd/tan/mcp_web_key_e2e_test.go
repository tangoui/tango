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
	key("CmdOrCtrl+B")
	var disabled struct{ Value any }
	call("tango_web_eval", "expr", "({events: keyEvents.slice(), disabledClicks})", &disabled)
	t.Logf("after disabled menu chord: %+v", disabled.Value)
	wait("disabledClicks === 0")
	eval("editor.value = 'replace me'")
	key("CmdOrCtrl+A")
	wait("editor.selectionStart === 0 && editor.selectionEnd === editor.value.length")
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
	eval("document.querySelector('#edit').textContent = 'Edit menu Select All: passed'")
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
	appStop(t, cs, app.Session)
	if processAlive(pid) {
		t.Fatalf("app process %d is still running", pid)
	}
}
