package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	wire "github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/automation/client"
	"github.com/tangoui/tango/ui/a11y"
)

// newFakeApp serves the automation wire protocol on a loopback port, as a
// tango_dev app would: it answers OpHello (token checked) with
// wire.Version, OpWindows with windows, and other ops with handlers[op]
// (an empty answer when there is no handler). A handler returning nil
// leaves the request unanswered. got returns the requests received after
// the hello, in order.
func newFakeApp(t *testing.T, windows []wire.WindowInfo, handlers map[string]func(*wire.Request) *wire.Response) (addr, token string, got func() []wire.Request) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	token = "fake-token"
	var (
		mu    sync.Mutex
		reqs  []wire.Request
		conns []net.Conn
	)
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	})
	serve := func(c net.Conn) {
		defer c.Close()
		var hello wire.Request
		if err := wire.ReadFrame(c, &hello); err != nil {
			return
		}
		if hello.Op != wire.OpHello || hello.Token != token {
			wire.WriteFrame(c, &wire.Response{ID: hello.ID, Code: wire.CodeAuth, Error: "bad token"})
			return
		}
		if wire.WriteFrame(c, &wire.Response{ID: hello.ID, Version: wire.Version}) != nil {
			return
		}
		for {
			var req wire.Request
			if err := wire.ReadFrame(c, &req); err != nil {
				return
			}
			mu.Lock()
			reqs = append(reqs, req)
			mu.Unlock()
			var resp *wire.Response
			if h := handlers[req.Op]; h != nil {
				resp = h(&req)
			} else if req.Op == wire.OpWindows {
				resp = &wire.Response{Windows: windows}
			} else {
				resp = &wire.Response{}
			}
			if resp == nil {
				continue
			}
			resp.ID = req.ID
			if wire.WriteFrame(c, resp) != nil {
				return
			}
		}
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
			go serve(c)
		}
	}()
	return ln.Addr().String(), token, func() []wire.Request {
		mu.Lock()
		defer mu.Unlock()
		return append([]wire.Request(nil), reqs...)
	}
}

var canvasWindow = []wire.WindowInfo{{ID: 7, Title: "Main", Kind: wire.KindCanvas, Visible: true, Focused: true, Width: 100, Height: 80}}

// fakeSession connects to a fake app and adds the session to s; it is an
// attached session unless the caller changes the fields before using it.
func fakeSession(t *testing.T, s *mcpServer, windows []wire.WindowInfo, handlers map[string]func(*wire.Request) *wire.Response) (*appSession, func() []wire.Request) {
	t.Helper()
	addr, token, got := newFakeApp(t, windows, handlers)
	conn, err := client.Dial(addr, token)
	if err != nil {
		t.Fatal(err)
	}
	a := &appSession{conn: conn, out: newTailBuffer(appOutputMax), exited: make(chan struct{}), exitErr: func() error { return nil }}
	s.apps.add(a)
	return a, got
}

func ops(reqs []wire.Request) []string {
	var out []string
	for _, r := range reqs {
		out = append(out, r.Op)
	}
	return out
}

func TestUIStartRejectsTangoEnv(t *testing.T) {
	s, cs := projectServer(t)
	msg := callErr(t, cs, "tango_app_start", map[string]any{"target": ".", "env": []string{"TANGO_LAUNCH_BACKGROUND=0"}})
	if !strings.Contains(msg, "TANGO_") {
		t.Fatalf("error %q", msg)
	}
	if n := len(s.apps.list()); n != 0 {
		t.Fatalf("%d sessions after a refused start", n)
	}
	msg = callErr(t, cs, "tango_app_start", map[string]any{"target": ".", "tags": []string{"-x"}})
	if !strings.Contains(msg, "tags") {
		t.Fatalf("tags error %q", msg)
	}
}

func TestUITreeTruncates(t *testing.T) {
	s, cs := projectServer(t)
	var dump strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&dump, "%sbutton \"b%d\"\n", strings.Repeat("  ", i%3), i)
	}
	a, _ := fakeSession(t, s, canvasWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpTree: func(*wire.Request) *wire.Response { return &wire.Response{Dump: dump.String()} },
	})
	var out struct {
		Tree      string `json:"tree"`
		Truncated int    `json:"truncated"`
	}
	callOK(t, cs, "tango_ui_tree", map[string]any{"session": a.ID, "maxNodes": 50}, &out)
	lines := strings.Split(strings.TrimSuffix(out.Tree, "\n"), "\n")
	if len(lines) != 51 || !strings.Contains(lines[50], "950 more nodes") || out.Truncated != 950 {
		t.Fatalf("%d lines, last %q, truncated %d", len(lines), lines[len(lines)-1], out.Truncated)
	}
	callOK(t, cs, "tango_ui_tree", map[string]any{"session": a.ID}, &out)
	if n := strings.Count(out.Tree, "\n"); n != 401 || out.Truncated != 600 {
		t.Fatalf("default: %d lines, truncated %d", n, out.Truncated)
	}
}

func TestUIFindNotFoundIncludesTree(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, canvasWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpFind: func(r *wire.Request) *wire.Response {
			return &wire.Response{Code: wire.CodeTimeout, Error: "automation: no match after 50ms"}
		},
		wire.OpTree: func(*wire.Request) *wire.Response {
			return &wire.Response{Dump: "window \"Main\"\n  button \"Save\"\n  textbox \"Name\"\n"}
		},
	})
	msg := callErr(t, cs, "tango_ui_find", map[string]any{"session": a.ID, "query": map[string]any{"role": "checkbox", "name": "Agree"}, "timeoutMs": 50})
	for _, want := range []string{"checkbox", "Agree", "button \"Save\"", "textbox \"Name\""} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
	for _, r := range got() {
		if r.Op == wire.OpFind && (r.Window != 7 || r.Timeout != 50 || r.Query == nil || r.Query.Role != "checkbox") {
			t.Errorf("find request %+v", r)
		}
	}
}

func TestUIFindAppExitsMidWait(t *testing.T) {
	s, cs := projectServer(t)
	a, _ := fakeSession(t, s, canvasWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpFind: func(*wire.Request) *wire.Response { return nil },
	})
	exited := make(chan struct{})
	a.started = true
	a.exited = exited
	a.exitErr = func() error {
		select {
		case <-exited:
			return errors.New("exit status 2")
		default:
			return nil
		}
	}
	a.out.Write([]byte("panic: boom\n"))
	time.AfterFunc(200*time.Millisecond, func() { close(exited) })
	start := time.Now()
	msg := callErr(t, cs, "tango_ui_find", map[string]any{"session": a.ID, "query": map[string]any{"role": "button"}, "timeoutMs": 10000})
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("find returned after %v", d)
	}
	for _, want := range []string{"exited", "exit status 2", "panic: boom"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q: %s", want, msg)
		}
	}
	// Later calls fail the same way, at once.
	msg = callErr(t, cs, "tango_ui_windows", map[string]any{"session": a.ID})
	if !strings.Contains(msg, "exit status 2") {
		t.Errorf("after exit: %s", msg)
	}
}

func TestUIWebViewWindowRejected(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, []wire.WindowInfo{{ID: 3, Title: "Page", Kind: wire.KindWebView}}, nil)
	for _, c := range []struct {
		tool string
		args map[string]any
	}{
		{"tango_ui_tree", nil},
		{"tango_ui_find", map[string]any{"query": map[string]any{"role": "button"}}},
		{"tango_ui_act", map[string]any{"query": map[string]any{"role": "button"}, "action": "press"}},
		{"tango_ui_click", map[string]any{"x": 1, "y": 2}},
		{"tango_ui_type", map[string]any{"text": "a"}},
		{"tango_ui_key", map[string]any{"keys": "Enter"}},
		{"tango_ui_wait", map[string]any{"query": map[string]any{"role": "button"}}},
	} {
		args := map[string]any{"session": a.ID}
		for k, v := range c.args {
			args[k] = v
		}
		if msg := callErr(t, cs, c.tool, args); !strings.Contains(msg, "tango_web_") || !strings.Contains(msg, `"Page"`) {
			t.Errorf("%s: %s", c.tool, msg)
		}
	}
	for _, op := range ops(got()) {
		if op != wire.OpWindows {
			t.Errorf("sent %s to a WebView window", op)
		}
	}
}

func TestUICloseAllQuitsStarted(t *testing.T) {
	s, _ := projectServer(t)
	started, gotStarted := fakeSession(t, s, canvasWindow, nil)
	started.started = true
	attached, gotAttached := fakeSession(t, s, canvasWindow, nil)
	s.apps.closeAll()
	if !strings.Contains(strings.Join(ops(gotStarted()), " "), wire.OpQuit) {
		t.Errorf("started session: requests %v, want quit", ops(gotStarted()))
	}
	if o := ops(gotAttached()); len(o) != 0 {
		t.Errorf("attached session: requests %v, want none", o)
	}
	if _, err := attached.conn.Do(&wire.Request{Op: wire.OpPing}, time.Second); err == nil {
		t.Error("attached session still connected")
	}
	if n := len(s.apps.list()); n != 0 {
		t.Errorf("%d sessions left", n)
	}
	if _, err := s.apps.get(started.ID); err == nil {
		t.Error("get after closeAll succeeded")
	}
}

func TestUISessionWindow(t *testing.T) {
	s, _ := projectServer(t)
	wins := []wire.WindowInfo{
		{ID: 1, Title: "One", Kind: wire.KindCanvas},
		{ID: 2, Title: "Two", Kind: wire.KindCanvas, Focused: true},
		{ID: 30, Title: "3", Kind: wire.KindWebView},
	}
	a, _ := fakeSession(t, s, wins, nil)
	for ref, want := range map[string]uint64{"": 2, "1": 1, "Two": 2, "30": 30, "3": 30} {
		w, err := a.window(ref)
		if err != nil || w.ID != want {
			t.Errorf("window(%q) = %d, %v; want %d", ref, w.ID, err, want)
		}
	}
	if _, err := a.window("Nope"); err == nil || !strings.Contains(err.Error(), "One") {
		t.Errorf("unknown window: %v", err)
	}
	if got, err := s.apps.get(a.ID); err != nil || got != a {
		t.Errorf("get(%q) = %v, %v", a.ID, got, err)
	}
	if _, err := s.apps.get("app-99"); err == nil {
		t.Error("get of an unknown session succeeded")
	}
}

func TestUIInputRequests(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, canvasWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpAction: func(r *wire.Request) *wire.Response {
			return &wire.Response{Node: &a11y.Node{ID: 5, Role: a11y.RoleButton, Name: "Save", States: a11y.Focusable, Actions: a11y.Press}}
		},
	})
	callOK(t, cs, "tango_ui_click", map[string]any{"session": a.ID, "x": 10, "y": 20, "clicks": 2}, nil)
	callOK(t, cs, "tango_ui_type", map[string]any{"session": a.ID, "window": "Main", "text": "hi"}, nil)
	callOK(t, cs, "tango_ui_key", map[string]any{"session": a.ID, "keys": "Cmd+S"}, nil)
	var act struct {
		Node struct {
			Role, Name, States, Actions string
		} `json:"node"`
	}
	callOK(t, cs, "tango_ui_act", map[string]any{"session": a.ID, "query": map[string]any{"role": "button", "name": "Save"}, "action": "press"}, &act)
	if act.Node.Role != "button" || act.Node.Name != "Save" || act.Node.Actions != "press" || act.Node.States != "focusable" {
		t.Errorf("act node %+v", act.Node)
	}
	var inputs []*wire.Input
	for _, r := range got() {
		if r.Op == wire.OpInput {
			if r.Window != 7 {
				t.Errorf("input to window %d", r.Window)
			}
			inputs = append(inputs, r.Input)
		}
		if r.Op == wire.OpAction && (r.Action != "press" || r.Timeout != 10000) {
			t.Errorf("action request %+v", r)
		}
	}
	if len(inputs) != 3 || inputs[0].Kind != "click" || inputs[0].X != 10 || inputs[0].Y != 20 || inputs[0].Clicks != 2 ||
		inputs[1].Kind != "text" || inputs[1].Text != "hi" || inputs[2].Kind != "key" || inputs[2].Keys != "Cmd+S" {
		t.Errorf("inputs %+v", inputs)
	}
}

func TestUIScreenshotCrops(t *testing.T) {
	s, cs := projectServer(t)
	// A 200x160 image of a 100x80 window (scale 2), red at logical (10,10)-(30,20).
	img := image.NewRGBA(image.Rect(0, 0, 200, 160))
	for y := 20; y < 40; y++ {
		for x := 20; x < 60; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	a, _ := fakeSession(t, s, canvasWindow, map[string]func(*wire.Request) *wire.Response{
		wire.OpScreenshot: func(*wire.Request) *wire.Response { return &wire.Response{PNG: buf.Bytes()} },
		wire.OpFind: func(*wire.Request) *wire.Response {
			return &wire.Response{Node: &a11y.Node{ID: 2, Role: a11y.RoleButton, Bounds: a11y.Rect{X: 10, Y: 10, W: 20, H: 10}}, Count: 1}
		},
	})
	decode := func(res *mcp.CallToolResult) image.Image {
		t.Helper()
		if res.IsError || len(res.Content) != 1 {
			t.Fatalf("screenshot result %+v", res)
		}
		ic, ok := res.Content[0].(*mcp.ImageContent)
		if !ok || ic.MIMEType != "image/png" {
			t.Fatalf("content %T", res.Content[0])
		}
		im, err := png.Decode(bytes.NewReader(ic.Data))
		if err != nil {
			t.Fatal(err)
		}
		return im
	}
	if b := decode(call(t, cs, "tango_ui_screenshot", map[string]any{"session": a.ID})).Bounds(); b.Dx() != 200 || b.Dy() != 160 {
		t.Errorf("whole window %v", b)
	}
	im := decode(call(t, cs, "tango_ui_screenshot", map[string]any{"session": a.ID, "query": map[string]any{"role": "button"}}))
	b := im.Bounds()
	if b.Dx() != 40 || b.Dy() != 20 {
		t.Fatalf("cropped %v", b)
	}
	if r, g, _, _ := im.At(b.Min.X+1, b.Min.Y+1).RGBA(); r>>8 != 255 || g != 0 {
		t.Errorf("cropped pixel not red")
	}
}

func TestUIOutputAndStop(t *testing.T) {
	s, cs := projectServer(t)
	a, got := fakeSession(t, s, canvasWindow, nil)
	a.started = true
	a.out.Write([]byte(strings.Repeat("x", 20<<10) + "end\n"))
	var out struct {
		Output    string `json:"output"`
		Truncated bool   `json:"truncated"`
	}
	callOK(t, cs, "tango_app_output", map[string]any{"session": a.ID}, &out)
	if len(out.Output) != 16<<10 || !out.Truncated || !strings.HasSuffix(out.Output, "end\n") {
		t.Errorf("output %d bytes, truncated %v", len(out.Output), out.Truncated)
	}
	var w struct {
		Windows []wire.WindowInfo `json:"windows"`
	}
	callOK(t, cs, "tango_ui_windows", map[string]any{"session": a.ID}, &w)
	if len(w.Windows) != 1 || w.Windows[0].Title != "Main" {
		t.Errorf("windows %+v", w.Windows)
	}
	callOK(t, cs, "tango_app_stop", map[string]any{"session": a.ID}, nil)
	if !strings.Contains(strings.Join(ops(got()), " "), wire.OpQuit) {
		t.Errorf("stop sent %v", ops(got()))
	}
	if msg := callErr(t, cs, "tango_ui_windows", map[string]any{"session": a.ID}); !strings.Contains(msg, a.ID) {
		t.Errorf("after stop: %s", msg)
	}
}

func TestUIAttach(t *testing.T) {
	s, cs := projectServer(t)
	addr, token, _ := newFakeApp(t, canvasWindow, nil)
	var out struct {
		Session string            `json:"session"`
		Windows []wire.WindowInfo `json:"windows"`
	}
	callOK(t, cs, "tango_app_attach", map[string]any{"addr": addr, "token": token}, &out)
	if !strings.HasPrefix(out.Session, "app-") || len(out.Windows) != 1 {
		t.Fatalf("attach %+v", out)
	}
	a, err := s.apps.get(out.Session)
	if err != nil || a.started || a.proc != nil {
		t.Fatalf("session %+v, %v", a, err)
	}
	if msg := callErr(t, cs, "tango_app_attach", map[string]any{"addr": "10.1.2.3:80", "token": "x"}); !strings.Contains(msg, "loopback") {
		t.Errorf("non-loopback attach: %s", msg)
	}
}
