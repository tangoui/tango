package main

import (
	"strings"
	"testing"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/automation/client"
)

var webWindow = []wire.WindowInfo{{ID: 3, Title: "Page", Kind: wire.KindWebView, Visible: true, Focused: true, Width: 800, Height: 600}}

// evals returns the OpEval requests among reqs.
func evals(reqs []wire.Request) []wire.Request {
	var out []wire.Request
	for _, r := range reqs {
		if r.Op == wire.OpEval {
			out = append(out, r)
		}
	}
	return out
}

func TestWebClickSendsClickJS(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, webWindow, nil)
	var out map[string]any
	callOK(t, cs, "tango_web_click", map[string]any{"session": a.ID, "selector": "#add"}, &out)
	if len(out) != 0 {
		t.Errorf("output %v, want {}", out)
	}
	ev := evals(got())
	if len(ev) != 1 {
		t.Fatalf("requests %v, want one eval", ops(got()))
	}
	if want := client.ClickJS("#add", 10*time.Second); ev[0].Script != want {
		t.Errorf("script\n%s\nwant\n%s", ev[0].Script, want)
	}
	if ev[0].Window != 3 || ev[0].Timeout != 12000 {
		t.Errorf("eval window %d timeout %d, want 3 and 12000", ev[0].Window, ev[0].Timeout)
	}
}

func TestWebToolsSendScripts(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, webWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpEval: func(r *wire.Request) *wire.Response {
			switch {
			case r.Script == "1+1":
				return &wire.Response{Value: []byte(`{"n":2,"items":["a","b"]}`)}
			case strings.Contains(r.Script, "innerText"):
				return &wire.Response{Value: []byte(`"3 items"`)}
			case r.Script == "void 0":
				return &wire.Response{}
			}
			return &wire.Response{Value: []byte("true")}
		},
	})
	var ev struct {
		Value any `json:"value"`
	}
	callOK(t, cs, "tango_web_eval", map[string]any{"session": a.ID, "expr": "1+1"}, &ev)
	if m, ok := ev.Value.(map[string]any); !ok || m["n"] != 2.0 || len(m["items"].([]any)) != 2 {
		t.Errorf("eval value %#v", ev.Value)
	}
	callOK(t, cs, "tango_web_eval", map[string]any{"session": a.ID, "expr": "void 0"}, &ev)
	if ev.Value != nil {
		t.Errorf("eval undefined: %v", ev.Value)
	}
	var text struct {
		Text string `json:"text"`
	}
	callOK(t, cs, "tango_web_text", map[string]any{"session": a.ID, "window": "Page", "selector": "ul", "timeoutMs": 500}, &text)
	if text.Text != "3 items" {
		t.Errorf("text %q", text.Text)
	}
	callOK(t, cs, "tango_web_fill", map[string]any{"session": a.ID, "selector": "#name", "text": "Ann", "timeoutMs": 500}, nil)
	callOK(t, cs, "tango_web_wait", map[string]any{"session": a.ID, "selector": ".done", "timeoutMs": 500}, nil)
	callOK(t, cs, "tango_web_wait", map[string]any{"session": a.ID, "expr": "window.ready", "timeoutMs": 500}, nil)

	to := 500 * time.Millisecond
	want := []struct {
		script  string
		timeout int64
	}{
		{"1+1", 12000},
		{"void 0", 12000},
		{client.TextJS("ul", to), 2500},
		{client.FillJS("#name", "Ann", to), 2500},
		{client.WaitSelectorJS(".done", to), 2500},
		{client.WaitExprJS("window.ready", to), 2500},
	}
	got1 := evals(got())
	if len(got1) != len(want) {
		t.Fatalf("%d evals, want %d", len(got1), len(want))
	}
	for i, w := range want {
		if got1[i].Script != w.script || got1[i].Timeout != w.timeout || got1[i].Window != 3 {
			t.Errorf("eval %d: script %q timeout %d window %d, want %q %d 3", i, got1[i].Script, got1[i].Timeout, got1[i].Window, w.script, w.timeout)
		}
	}
}

func TestWebWaitNeedsOneOf(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, webWindow, nil)
	if msg := callErr(t, cs, "tango_web_wait", map[string]any{"session": a.ID, "selector": "#a", "expr": "true"}); !strings.Contains(msg, "selector") || !strings.Contains(msg, "expr") {
		t.Errorf("both: %s", msg)
	}
	if msg := callErr(t, cs, "tango_web_wait", map[string]any{"session": a.ID}); !strings.Contains(msg, "selector") || !strings.Contains(msg, "expr") {
		t.Errorf("neither: %s", msg)
	}
	if ev := evals(got()); len(ev) != 0 {
		t.Errorf("sent %d evals", len(ev))
	}
}

func TestWebOnCanvasRejected(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, canvasWindow, nil)
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"tango_web_eval", map[string]any{"expr": "1"}},
		{"tango_web_click", map[string]any{"selector": "#a"}},
		{"tango_web_fill", map[string]any{"selector": "#a", "text": "x"}},
		{"tango_web_text", map[string]any{"selector": "#a"}},
		{"tango_web_wait", map[string]any{"selector": "#a"}},
	} {
		args := map[string]any{"session": a.ID}
		for k, v := range c.args {
			args[k] = v
		}
		if msg := callErr(t, cs, c.tool, args); !strings.Contains(msg, "tango_ui_") || !strings.Contains(msg, `"Main"`) {
			t.Errorf("%s: %s", c.tool, msg)
		}
	}
	for _, op := range ops(got()) {
		if op != wire.OpWindows {
			t.Errorf("sent %s to a self-drawn window", op)
		}
	}
}

func TestWebScriptErrorSurfaced(t *testing.T) {
	s, cs := projectServer(t)
	a, _ := fakeSession(t, s, webWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpEval: func(*wire.Request) *wire.Response {
			return &wire.Response{Code: wire.CodeScript, Error: "ReferenceError: x is not defined"}
		},
	})
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"tango_web_eval", map[string]any{"expr": "x"}},
		{"tango_web_click", map[string]any{"selector": "#a"}},
		{"tango_web_text", map[string]any{"selector": "#a"}},
	} {
		args := map[string]any{"session": a.ID}
		for k, v := range c.args {
			args[k] = v
		}
		if msg := callErr(t, cs, c.tool, args); !strings.Contains(msg, "ReferenceError: x is not defined") {
			t.Errorf("%s: %s", c.tool, msg)
		}
	}
}
