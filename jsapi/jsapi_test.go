package jsapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ipc"
)

// env is a running fake app with the API installed and the router fed
// from App.OnMessage, as an app would wire it.
type tenv struct {
	t   *testing.T
	a   *tango.App
	fb  *fake.Backend
	r   *ipc.Router
	api *API
}

func setup(t *testing.T, opts Options, minimal ...bool) *tenv {
	t.Helper()
	var a *tango.App
	var fb *fake.Backend
	if len(minimal) > 0 && minimal[0] {
		a, fb = faketest.NewMinimalApp(tango.Options{Name: "jsapi-test"})
	} else {
		a, fb = faketest.NewApp(tango.Options{Name: "jsapi-test"})
	}
	r := ipc.NewRouter()
	a.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
	api := Install(a, r, opts)
	faketest.Start(t, a, fb)
	return &tenv{t, a, fb, r, api}
}

// page plays the JS runtime of one surface.
type page struct {
	e      *tenv
	s      *tango.Surface
	fs     *fake.Surface
	nextID int
	seen   int
}

func (e *tenv) open() *page {
	e.t.Helper()
	s, err := e.a.NewSurface(tango.SurfaceOptions{Title: "t", Width: 400, Height: 300})
	if err != nil {
		e.t.Fatal(err)
	}
	e.fb.Load(s.ID())
	p := &page{e: e, s: s, fs: e.fb.Surface(s.ID()), nextID: -1}
	p.post(`{"tango":"hello"}`)
	return p
}

func (p *page) post(msg string) { p.e.fb.Message(p.s.ID(), msg) }

// received returns every ipc message delivered to the page so far.
func (p *page) received() []map[string]json.RawMessage {
	var out []map[string]json.RawMessage
	for _, ev := range p.fs.Evals() {
		lit, ok := strings.CutPrefix(ev, "window.__tango._recv(")
		if !ok {
			continue
		}
		var msg string
		if json.Unmarshal([]byte(strings.TrimSuffix(lit, ")")), &msg) != nil {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(msg), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

func (p *page) wait(match func(m map[string]json.RawMessage) bool) map[string]json.RawMessage {
	p.e.t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for time.Now().Before(deadline) {
		for _, m := range p.received() {
			if match(m) {
				return m
			}
		}
		time.Sleep(time.Millisecond)
	}
	p.e.t.Fatalf("timed out waiting for a message; got %d", len(p.received()))
	return nil
}

type reply struct {
	result json.RawMessage
	code   string
	msg    string
}

// start sends a call and returns a function waiting for its reply.
func (p *page) start(method string, args ...any) func() reply {
	if args == nil {
		args = []any{}
	}
	p.nextID += 2
	id := p.nextID
	b, _ := json.Marshal(map[string]any{"tango": "call", "id": id, "method": Prefix + method, "args": args})
	p.post(string(b))
	return func() reply {
		p.e.t.Helper()
		idJS, _ := json.Marshal(id)
		m := p.wait(func(m map[string]json.RawMessage) bool {
			return string(m["tango"]) == `"ret"` && bytes.Equal(m["id"], idJS)
		})
		var r reply
		r.result = m["result"]
		if raw, ok := m["error"]; ok {
			var e ipc.Error
			json.Unmarshal(raw, &e)
			r.code, r.msg = e.Code, e.Message
		}
		return r
	}
}

func (p *page) call(method string, args ...any) reply { return p.start(method, args...)() }

func (p *page) ok(method string, args ...any) json.RawMessage {
	p.e.t.Helper()
	r := p.call(method, args...)
	if r.code != "" {
		p.e.t.Fatalf("%s: %s: %s", method, r.code, r.msg)
	}
	return r.result
}

func (p *page) fails(code, method string, args ...any) {
	p.e.t.Helper()
	if r := p.call(method, args...); r.code != code {
		p.e.t.Fatalf("%s: code %q (%s), want %q", method, r.code, r.msg, code)
	}
}

// event waits for the n-th (1-based) event called name.
func (p *page) event(name string, n int) json.RawMessage {
	p.e.t.Helper()
	nameJS, _ := json.Marshal(Prefix + name)
	var data json.RawMessage
	p.wait(func(m map[string]json.RawMessage) bool {
		if string(m["tango"]) != `"event"` || !bytes.Equal(m["name"], nameJS) {
			return false
		}
		n--
		data = m["data"]
		return n == 0
	})
	return data
}

func (p *page) count(name string) int {
	nameJS, _ := json.Marshal(Prefix + name)
	c := 0
	for _, m := range p.received() {
		if string(m["tango"]) == `"event"` && bytes.Equal(m["name"], nameJS) {
			c++
		}
	}
	return c
}

func jsonEq(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("bad json %s", got)
	}
	json.Unmarshal([]byte(want), &w)
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Fatalf("got %s, want %s", gb, wb)
	}
}

func TestDefaultsAllowList(t *testing.T) {
	e := setup(t, Options{})
	p := e.open()
	p.ok("window.setTitle", "你好")
	if got := p.fs.Title(); got != "你好" {
		t.Fatalf("title %q", got)
	}
	var info envJS
	json.Unmarshal(p.ok("env.info"), &info)
	if info.OS == "" || info.WebView != "FakeView" {
		t.Fatalf("env %+v", info)
	}
	p.ok("theme.get")
	p.ok("screens.all")
	for _, m := range []string{
		"clipboard.readText", "clipboard.writeText", "dialog.message", "dialog.open", "notify.send",
		"shell.openURL", "shell.openPath", "app.quit", "app.setBadge", "hotkey.register",
	} {
		p.fails(CodeForbidden, m)
	}
	p.fails(ipc.CodeNotFound, "window.nope")
	if e.fb.ClipboardText() != "" || len(e.fb.ShellCalls()) != 0 || e.fb.Quits() != 0 {
		t.Fatal("a forbidden call reached the backend")
	}
}

func TestAllowAndDeny(t *testing.T) {
	e := setup(t, Options{Allow: ClipboardRead | ClipboardWrite | OpenURL, Deny: Window})
	p := e.open()
	p.fails(CodeForbidden, "window.setTitle", "x")
	p.ok("clipboard.writeText", "剪贴板")
	jsonEq(t, p.ok("clipboard.readText"), `"剪贴板"`)

	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	abs, _ := filepath.Abs("f.txt")
	p.ok("clipboard.writeFiles", []string{abs})
	p.fails(ipc.CodeBadArgs, "clipboard.writeFiles", []string{"rel.txt"})
	p.ok("clipboard.writeImage", b64)
	jsonEq(t, p.ok("clipboard.readImage"), `"`+b64+`"`)
	p.fails(ipc.CodeBadArgs, "clipboard.writeImage", base64.StdEncoding.EncodeToString([]byte("not png")))
	p.fails(ipc.CodeBadArgs, "clipboard.writeImage", "%%%")
	e.fb.SetClipboardFiles([]string{"/a", "/b"})
	jsonEq(t, p.ok("clipboard.readFiles"), `["/a","/b"]`)

	p.ok("shell.openURL", "https://example.com")
	p.fails(CodeForbidden, "shell.openURL", "file:///etc/passwd")
	p.fails(CodeForbidden, "shell.revealPath", "/tmp")
	if c := e.fb.ShellCalls(); len(c) != 1 {
		t.Fatalf("shell calls %v", c)
	}
	p.fails(ipc.CodeBadArgs, "clipboard.writeText", 42)
	p.fails(ipc.CodeBadArgs, "clipboard.writeText", "a", "b")
}

func TestWindowIdentity(t *testing.T) {
	e := setup(t, Options{})
	p1, p2 := e.open(), e.open()
	p1.ok("window.setTitle", "one")
	p2.ok("window.setTitle", "two")
	if p1.fs.Title() != "one" || p2.fs.Title() != "two" {
		t.Fatalf("titles %q %q", p1.fs.Title(), p2.fs.Title())
	}
	jsonEq(t, p1.ok("window.id"), `1`)
	jsonEq(t, p2.ok("window.id"), `2`)

	p2.ok("window.setSize", 800, 600)
	jsonEq(t, p2.ok("window.size"), `{"width":800,"height":600}`)
	if w, _ := p1.s.Size(); w != 400 {
		t.Fatalf("other window resized to %d", w)
	}
	p2.ok("window.setPosition", 10, 20)
	jsonEq(t, p2.ok("window.position"), `{"x":10,"y":20}`)
	p2.ok("window.setBounds", map[string]int{"x": 1, "y": 2, "width": 300, "height": 200})
	jsonEq(t, p2.ok("window.bounds"), `{"x":1,"y":2,"width":300,"height":200}`)
	p2.fails(ipc.CodeBadArgs, "window.setSize", 0, 10)

	p2.ok("window.maximize")
	jsonEq(t, p2.ok("window.state"), `"maximized"`)
	p2.ok("window.toggleMaximize")
	jsonEq(t, p2.ok("window.state"), `"normal"`)
	p2.ok("window.setState", "fullscreen")
	if p2.s.State() != tango.StateFullscreen || p1.s.State() != tango.StateNormal {
		t.Fatal("state went to the wrong window")
	}
	p2.fails(ipc.CodeBadArgs, "window.setState", "huge")

	p1.ok("window.setAlwaysOnTop", true)
	jsonEq(t, p1.ok("window.alwaysOnTop"), `true`)
	if ap, _ := p1.fs.CurrentAppearance(); !ap.AlwaysOnTop {
		t.Fatal("not on top")
	}
	p1.ok("window.setZoom", 1.5)
	jsonEq(t, p1.ok("window.zoom"), `1.5`)
	p1.ok("window.hide")
	if p1.fs.Visible() {
		t.Fatal("still visible")
	}
	p1.ok("window.show")
	p1.ok("window.center")
	p1.ok("window.openDevTools")
	if p1.fs.DevToolsOpened() != 1 {
		t.Fatal("devtools not opened")
	}
	jsonEq(t, p1.ok("window.title"), `"one"`)
	jsonEq(t, p1.ok("window.isVisible"), `true`)
	jsonEq(t, p1.ok("window.isFocused"), `false`)
	e.fb.FocusChange(p1.s.ID(), true)
	p1.event("window.focus", 1)
	jsonEq(t, p1.ok("window.isFocused"), `true`)
	e.fb.FocusChange(p1.s.ID(), false)
	p1.event("window.blur", 1)

	p2.ok("window.close")
	deadline := time.Now().Add(faketest.Timeout)
	for !p2.fs.Closed() {
		if time.Now().After(deadline) {
			t.Fatal("not closed")
		}
		time.Sleep(time.Millisecond)
	}
	if p1.fs.Closed() {
		t.Fatal("wrong window closed")
	}
}

func TestNotASurface(t *testing.T) {
	e := setup(t, Options{})
	c := &chanConn{ch: make(chan string, 8)}
	e.r.HandleMessage(c, `{"tango":"call","id":1,"method":"tango:window.setTitle","args":["x"]}`)
	if msg := <-c.ch; !strings.Contains(msg, `"not_found"`) {
		t.Fatal(msg)
	}
	e.r.HandleMessage(c, `{"tango":"call","id":3,"method":"tango:env.info","args":[]}`)
	if msg := <-c.ch; !strings.Contains(msg, `"result"`) {
		t.Fatal(msg)
	}
}

type chanConn struct{ ch chan string }

func (c *chanConn) Send(m string) { c.ch <- m }

func TestUnsupported(t *testing.T) {
	e := setup(t, Options{Allow: All}, true)
	p := e.open()
	p.fails(CodeUnsupported, "theme.get")
	p.fails(CodeUnsupported, "clipboard.readImage")
	p.fails(CodeUnsupported, "notify.send", map[string]string{"title": "x"})
	p.fails(CodeUnsupported, "menu.popup", []any{}, 0, 0)
}

func TestWindowEvents(t *testing.T) {
	e := setup(t, Options{})
	p1, p2 := e.open(), e.open()
	e.fb.Resize(p1.s.ID(), 500, 400)
	jsonEq(t, p1.event("window.resize", 1), `{"width":500,"height":400}`)
	e.fb.Move(p1.s.ID(), 7, 8)
	jsonEq(t, p1.event("window.move", 1), `{"x":7,"y":8}`)
	e.fb.ChangeState(p1.s.ID(), platform.StateMinimized)
	jsonEq(t, p1.event("window.state", 1), `{"state":"minimized"}`)
	e.fb.DropFiles(p1.s.ID(), []string{"/x.txt"}, 3, 4)
	jsonEq(t, p1.event("window.filesDropped", 1), `{"paths":["/x.txt"],"x":3,"y":4}`)
	p2.ok("window.id") // sync point: p2's queue is flushed
	if n := p2.count("window.resize") + p2.count("window.move"); n != 0 {
		t.Fatalf("other window got %d events", n)
	}

	// Without Window, nothing is forwarded.
	e2 := setup(t, Options{Deny: Window})
	q := e2.open()
	e2.fb.Resize(q.s.ID(), 1, 1)
	q.ok("env.info")
	if q.count("window.resize") != 0 {
		t.Fatal("forwarded without Window")
	}
}

func TestAppEvents(t *testing.T) {
	e := setup(t, Options{})
	// A deep link before any page exists is kept for takePending.
	e.fb.DeepLink("demo://launch")
	e.fb.OpenFiles("/f1")
	p1 := e.open()
	jsonEq(t, p1.ok("app.takePending", "openURL"), `[{"url":"demo://launch"}]`)
	jsonEq(t, p1.ok("app.takePending", "openURL"), `[]`)
	jsonEq(t, p1.ok("app.takePending", "openFiles"), `[{"paths":["/f1"]}]`)

	p2 := e.open()
	e.fb.SetTheme(platform.Theme{Dark: true, Accent: 0x112233ff})
	for _, p := range []*page{p1, p2} {
		jsonEq(t, p.event("theme.change", 1), `{"dark":true,"accent":"#112233ff"}`)
	}
	e.fb.SetDisplays([]platform.Display{{ID: "d1", Primary: true, Scale: 2, Bounds: platform.Rect{Width: 100, Height: 50}}})
	var ds []displayJS
	json.Unmarshal(p2.event("screens.change", 1), &ds)
	if len(ds) != 1 || ds[0].ID != "d1" || ds[0].Scale != 2 {
		t.Fatalf("displays %+v", ds)
	}
	e.fb.DeepLink("demo://live")
	jsonEq(t, p1.event("app.openURL", 1), `{"url":"demo://live"}`)
	jsonEq(t, p2.event("app.openURL", 1), `{"url":"demo://live"}`)
	e.fb.Reopen()
	p1.event("app.reopen", 1)
	e.api.secondInstance([]string{"--x", "demo://2"}, "/home")
	jsonEq(t, p2.event("app.secondInstance", 1), `{"args":["--x","demo://2"],"cwd":"/home"}`)

	// Closed pages are forgotten.
	p1.s.Close()
	deadline := time.Now().Add(faketest.Timeout)
	for len(e.api.pages()) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("closed page still attached")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDialogs(t *testing.T) {
	e := setup(t, Options{Allow: Dialogs})
	p := e.open()
	var mu sync.Mutex
	var parent platform.SurfaceID
	var got platform.MessageOptions
	e.fb.OnMessageDialog = func(par platform.SurfaceID, o platform.MessageOptions) (int, error) {
		mu.Lock()
		parent, got = par, o
		mu.Unlock()
		return 1, nil
	}
	jsonEq(t, p.ok("dialog.message", map[string]any{"kind": "warning", "message": "m", "buttons": []string{"A", "B"}}), `1`)
	mu.Lock()
	if parent != p.s.ID() || got.Kind != platform.MessageWarning || got.Message != "m" {
		t.Fatalf("dialog %v %+v", parent, got)
	}
	mu.Unlock()
	p.fails(ipc.CodeBadArgs, "dialog.message", map[string]any{"kind": "loud"})
	p.fails(ipc.CodeCanceled, "dialog.open", map[string]any{})
	e.fb.OnOpenDialog = func(par platform.SurfaceID, o platform.FileOptions) ([]string, error) {
		if !o.Multiple || len(o.Filters) != 1 || o.Filters[0].Patterns[0] != "*.png" {
			return nil, platform.ErrCanceled
		}
		return []string{"/a.png", "/b.png"}, nil
	}
	jsonEq(t, p.ok("dialog.open", map[string]any{"multiple": true, "filters": []any{map[string]any{"name": "图", "patterns": []string{"*.png"}}}}), `["/a.png","/b.png"]`)
	p.fails(ipc.CodeCanceled, "dialog.save", map[string]any{"filename": "x.txt"})
}

func TestNotify(t *testing.T) {
	e := setup(t, Options{Allow: Notify})
	p1, p2 := e.open(), e.open()
	jsonEq(t, p1.ok("notify.requestPermission"), `true`)
	var id string
	json.Unmarshal(p1.ok("notify.send", map[string]any{"title": "T", "body": "B"}), &id)
	if n := e.fb.Notifications(); len(n) != 1 || n[0].Title != "T" {
		t.Fatalf("notes %+v", n)
	}
	e.fb.ClickNotification(id)
	jsonEq(t, p1.event("notify.click", 1), `{"id":"`+id+`"}`)
	p2.ok("window.id")
	if p2.count("notify.click") != 0 {
		t.Fatal("click went to the other page")
	}
}

func TestMenuPopup(t *testing.T) {
	e := setup(t, Options{})
	p := e.open()
	items := []any{
		map[string]any{"id": "open", "label": "打开"},
		map[string]any{"type": "separator"},
		map[string]any{"id": "wrap", "label": "Wrap", "type": "checkbox"},
		map[string]any{"label": "More", "submenu": []any{map[string]any{"id": "deep", "label": "Deep"}}},
		map[string]any{"role": "copy"},
	}
	choose := func(path ...int) reply {
		wait := p.start("menu.popup", items, 5, 6)
		var pop *fake.Popup
		deadline := time.Now().Add(faketest.Timeout)
		n := len(e.fb.Popups())
		for {
			if ps := e.fb.Popups(); len(ps) > 0 && !ps[len(ps)-1].Closed() && len(ps) >= n {
				pop = ps[len(ps)-1]
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("no popup")
			}
			time.Sleep(time.Millisecond)
		}
		if pop.Surface != p.s.ID() || pop.X != 5 || pop.Y != 6 {
			t.Fatalf("popup %+v", pop)
		}
		if path == nil {
			pop.Dismiss()
		} else {
			it := pop.Items[path[0]]
			for _, i := range path[1:] {
				it = it.Submenu[i]
			}
			pop.Choose(it.ID)
		}
		return wait()
	}
	jsonEq(t, choose(0).result, `{"id":"open","checked":false}`)
	jsonEq(t, choose(2).result, `{"id":"wrap","checked":true}`)
	jsonEq(t, choose(3, 0).result, `{"id":"deep","checked":false}`)
	jsonEq(t, choose().result, `{"id":null,"checked":false}`)
	p.fails(CodeForbidden, "menu.popup", []any{map[string]any{"role": "quit"}}, 0, 0)
	p.fails(ipc.CodeBadArgs, "menu.popup", []any{map[string]any{"type": "weird"}}, 0, 0)
}

func TestHotkeys(t *testing.T) {
	e := setup(t, Options{Allow: Hotkeys})
	p1, p2 := e.open(), e.open()
	var id int
	p1.fails(ipc.CodeBadArgs, "hotkey.register", "Ctrl+Shift+NoSuchKey")
	json.Unmarshal(p1.ok("hotkey.register", "CmdOrCtrl+Shift+J"), &id)
	if e.fb.PressHotkey("CmdOrCtrl+Shift+J") != 1 {
		t.Fatal("not registered")
	}
	jsonEq(t, p1.event("hotkey.fire", 1), `{"id":1,"accelerator":"CmdOrCtrl+Shift+J"}`)
	p2.fails(ipc.CodeNotFound, "hotkey.unregister", id) // not p2's
	p1.ok("hotkey.unregister", id)
	if len(e.fb.Hotkeys()) != 0 {
		t.Fatal("still registered")
	}
	// A reload (new hello) drops the page's hotkeys.
	p1.ok("hotkey.register", "CmdOrCtrl+Shift+L")
	p1.post(`{"tango":"hello"}`)
	if len(e.fb.Hotkeys()) != 0 {
		t.Fatal("reload kept hotkeys")
	}
	// So does closing the window.
	p2.ok("hotkey.register", "CmdOrCtrl+Shift+M")
	p2.s.Close()
	if len(e.fb.Hotkeys()) != 0 {
		t.Fatal("close kept hotkeys")
	}
}

func TestAppControl(t *testing.T) {
	e := setup(t, Options{Allow: AppControl})
	p := e.open()
	p.ok("app.setBadge", "3")
	p.ok("app.setProgress", 0.5)
	p.ok("app.setProgress", nil)
	p.ok("app.hide")
	st := e.fb.AppControl()
	if st.Badge != "3" || st.Progress != -1 || !st.Hidden {
		t.Fatalf("appctl %+v", st)
	}
	p.ok("app.quit")
	select {
	case <-e.fb.Done():
	case <-time.After(faketest.Timeout):
		t.Fatal("did not quit")
	}
}

func TestHandleExtension(t *testing.T) {
	const custom Cap = 1 << 40
	a, fb := faketest.NewApp(tango.Options{})
	r := ipc.NewRouter()
	a.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
	on := Install(a, r, Options{})
	on.Handle(custom, "share.text", func(_ context.Context, s *tango.Surface, args json.RawMessage) (any, error) {
		return s != nil, nil
	})
	faketest.Start(t, a, fb)
	e := &tenv{t, a, fb, r, on}
	p := e.open()
	p.fails(CodeForbidden, "share.text")

	a2, fb2 := faketest.NewApp(tango.Options{})
	r2 := ipc.NewRouter()
	a2.OnMessage(func(s *tango.Surface, m string) { r2.HandleMessage(s, m) })
	on2 := Install(a2, r2, Options{Allow: custom})
	on2.Handle(custom, "share.text", func(_ context.Context, s *tango.Surface, _ json.RawMessage) (any, error) {
		return s != nil, nil
	})
	faketest.Start(t, a2, fb2)
	p2 := (&tenv{t, a2, fb2, r2, on2}).open()
	jsonEq(t, p2.ok("share.text"), `true`)
}
