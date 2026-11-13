package automation

import (
	"encoding/binary"
	"errors"
	"image"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// fakeHost simulates the app: a mutex stands in for the main thread.
type fakeHost struct {
	mu      sync.Mutex
	windows []WindowInfo
	quit    chan struct{}
	panics  int
}

func (h *fakeHost) Main(fn func()) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Guard(fn)
}

func (h *fakeHost) Guard(fn func()) (ok bool) {
	defer func() {
		if !ok {
			recover()
			h.panics++
		}
	}()
	fn()
	return true
}

func (h *fakeHost) Windows() []WindowInfo { return append([]WindowInfo(nil), h.windows...) }
func (h *fakeHost) Eval(win uint64, expr string, _ time.Duration) ([]byte, error) {
	if expr == "bad" {
		return nil, &Error{Code: CodeScript, Msg: "script error"}
	}
	return []byte(`"` + expr + `"`), nil
}
func (h *fakeHost) Screenshot(uint64) ([]byte, error) { return []byte("png"), nil }
func (h *fakeHost) Quit()                             { close(h.quit) }

// fakeCanvas: Perform and Input change the tree "on the next frame".
type fakeCanvas struct {
	h       *fakeHost
	tree    *a11y.Tree
	gen     uint64
	err     error
	panic   bool
	actions []string
	events  []input.Event
}

func (c *fakeCanvas) Tree() (*a11y.Tree, uint64, error) {
	if c.panic {
		panic("canvas bug")
	}
	return c.tree, c.gen, c.err
}

func (c *fakeCanvas) set(t *a11y.Tree) { c.tree = t; c.gen++ }

func (c *fakeCanvas) later(fn func()) {
	go func() {
		time.Sleep(30 * time.Millisecond)
		c.h.mu.Lock()
		fn()
		c.h.mu.Unlock()
	}()
}

func (c *fakeCanvas) Perform(id a11y.NodeID, act a11y.Action, arg string) error {
	n := c.tree.Node(id)
	if !n.Actions.Has(act) {
		return errors.New("unsupported")
	}
	c.actions = append(c.actions, n.Name+":"+act.String()+":"+arg)
	if n.Name == "记一笔" {
		c.later(func() { c.set(sampleTree()) }) // the dialog opens
	}
	return nil
}

func (c *fakeCanvas) Input(evs []input.Event) error {
	c.events = append(c.events, evs...)
	c.later(func() { c.gen++ })
	return nil
}

func (c *fakeCanvas) Snapshot() (*image.RGBA, error) {
	return image.NewRGBA(image.Rect(0, 0, 4, 3)), nil
}

func mainTree() *a11y.Tree {
	t := a11y.NewTree(1)
	t.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2}})
	t.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: "记一笔", Actions: a11y.Press})
	return t
}

type testConn struct {
	t  *testing.T
	c  net.Conn
	id uint64
}

func (tc *testConn) do(req *Request) *Response {
	tc.t.Helper()
	tc.id++
	req.ID = tc.id
	if err := WriteFrame(tc.c, req); err != nil {
		tc.t.Fatal(err)
	}
	var r Response
	if err := ReadFrame(tc.c, &r); err != nil {
		tc.t.Fatalf("%s: %v", req.Op, err)
	}
	if r.ID != req.ID {
		tc.t.Fatalf("id %d for %d", r.ID, req.ID)
	}
	return &r
}

func startServer(t *testing.T, addr string) (*Server, *fakeHost, *fakeCanvas, string) {
	t.Helper()
	pollEvery = 2 * time.Millisecond // read by Listen only
	h := &fakeHost{quit: make(chan struct{}), windows: []WindowInfo{{ID: 1, Title: "Canvas"}, {ID: 2, Title: "Web"}}}
	app := new(int)
	c := &fakeCanvas{h: h}
	t.Cleanup(Register(app, 1, c))
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	s, err := Listen(app, h, addr, tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, h, c, tokenFile
}

func connect(t *testing.T, s *Server, token string) (*testConn, *Response) {
	t.Helper()
	nc, err := Dial(s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })
	tc := &testConn{t: t, c: nc}
	return tc, tc.do(&Request{Op: OpHello, Token: token})
}

func TestServerRejectsBadToken(t *testing.T) {
	s, _, _, tokenFile := startServer(t, "127.0.0.1:0")
	tf, err := ReadTokenFile(tokenFile)
	if err != nil || tf.Addr != s.Addr() || tf.Token != s.Token() {
		t.Fatalf("token file %+v %v", tf, err)
	}
	for _, bad := range []string{"", "nope", s.Token()[:63], strings.ToUpper(s.Token())} {
		tc, r := connect(t, s, bad)
		if r.Code != CodeAuth {
			t.Fatalf("token %q: %+v", bad, r)
		}
		var x Response // the server hangs up
		if err := ReadFrame(tc.c, &x); err == nil {
			t.Fatal("connection kept open after a bad token")
		}
	}
	// Any other first request is refused too, even with the right token.
	nc, _ := Dial(s.Addr(), time.Second)
	defer nc.Close()
	tc := &testConn{t: t, c: nc}
	if r := tc.do(&Request{Op: OpWindows, Token: s.Token()}); r.Code != CodeAuth {
		t.Fatalf("windows before hello: %+v", r)
	}
	// The right token works.
	_, r := connect(t, s, s.Token())
	if r.Error != "" || r.Version != Version {
		t.Fatalf("hello %+v", r)
	}
}

func TestServerHelloTimeout(t *testing.T) {
	old := helloTimeout
	helloTimeout = 50 * time.Millisecond
	defer func() { helloTimeout = old }()
	s, _, _, _ := startServer(t, "127.0.0.1:0")
	nc, _ := Dial(s.Addr(), time.Second)
	defer nc.Close()
	nc.SetReadDeadline(time.Now().Add(2 * time.Second))
	var b [1]byte
	if _, err := nc.Read(b[:]); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("silent client not dropped: %v", err)
	}
}

func TestServerOps(t *testing.T) {
	s, h, c, _ := startServer(t, "127.0.0.1:0")
	tc, _ := connect(t, s, s.Token())

	ws := tc.do(&Request{Op: OpWindows}).Windows
	if len(ws) != 2 || ws[0].Kind != KindCanvas || ws[1].Kind != KindWebView {
		t.Fatalf("windows %+v", ws)
	}

	// No tree yet: the first tree arrives a few frames later.
	c.later(func() { c.set(mainTree()) })
	r := tc.do(&Request{Op: OpFind, Window: 1, Query: &Query{Role: "button", Name: "记一笔"}})
	if r.Error != "" || r.Node == nil || r.Node.ID != 2 || r.Count != 1 {
		t.Fatalf("find without timeout waits for the first tree: %+v", r)
	}
	// Not there, no waiting: timeout error with a dump.
	r = tc.do(&Request{Op: OpFind, Window: 1, Query: &Query{Role: "button", Name: "保存"}})
	if r.Code != CodeTimeout || !strings.Contains(r.Dump, `button "记一笔"`) {
		t.Fatalf("missing node: %+v", r)
	}
	// Press opens the dialog; the call returns after the tree changed.
	r = tc.do(&Request{Op: OpAction, Window: 1, Query: &Query{Role: "button", Name: "记一笔"}, Action: "press", Timeout: 1000})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	h.mu.Lock()
	settled := c.tree.Find(a11y.RoleDialog, "记一笔") != nil
	h.mu.Unlock()
	if !settled {
		t.Fatal("action returned before the frame that handled it")
	}
	r = tc.do(&Request{Op: OpWait, Window: 1, Query: &Query{Role: "dialog"}, Timeout: 1000})
	if r.Node == nil || r.Node.Name != "记一笔" {
		t.Fatalf("wait %+v", r)
	}
	// Unsupported action.
	r = tc.do(&Request{Op: OpAction, Window: 1, Query: &Query{Role: "button", Name: "保存", States: []string{"disabled"}}, Action: "press"})
	if r.Code != CodeAction {
		t.Fatalf("press on a node without it: %+v", r)
	}
	// Wait for disappearance.
	c.later(func() { c.set(mainTree()) })
	if r = tc.do(&Request{Op: OpWait, Window: 1, Query: &Query{Role: "dialog"}, Gone: true, Timeout: 1000}); r.Error != "" {
		t.Fatal(r.Error)
	}
	if r = tc.do(&Request{Op: OpWait, Window: 1, Query: &Query{Role: "button"}, Gone: true, Timeout: 30}); r.Code != CodeTimeout {
		t.Fatalf("gone timeout %+v", r)
	}
	if r = tc.do(&Request{Op: OpCount, Window: 1, Query: &Query{}}); r.Count != 2 {
		t.Fatalf("count %+v", r)
	}
	if r = tc.do(&Request{Op: OpTree, Window: 1}); len(r.Nodes) != 2 || !strings.HasPrefix(r.Dump, "window") {
		t.Fatalf("tree %+v", r)
	}
	// Input.
	if r = tc.do(&Request{Op: OpInput, Window: 1, Input: &Input{Kind: "drag", X2: 10, Steps: 2}}); r.Error != "" {
		t.Fatal(r.Error)
	}
	h.mu.Lock()
	n := len(c.events)
	h.mu.Unlock()
	if n != 5 {
		t.Fatalf("%d events for a 2-step drag", n)
	}
	if r = tc.do(&Request{Op: OpInput, Window: 1, Input: &Input{Kind: "key", Keys: "Hyper+Q"}}); r.Code != CodeBadRequest {
		t.Fatalf("bad key %+v", r)
	}
	// Screenshots and eval by window kind.
	if r = tc.do(&Request{Op: OpScreenshot, Window: 1}); len(r.PNG) < 8 || string(r.PNG[1:4]) != "PNG" {
		t.Fatalf("canvas screenshot %q", r.PNG)
	}
	if r = tc.do(&Request{Op: OpScreenshot, Window: 2}); string(r.PNG) != "png" {
		t.Fatalf("web screenshot %+v", r)
	}
	if r = tc.do(&Request{Op: OpEval, Window: 2, Script: "1+1"}); string(r.Value) != `"1+1"` {
		t.Fatalf("eval %+v", r)
	}
	if r = tc.do(&Request{Op: OpEval, Window: 2, Script: "bad"}); r.Code != CodeScript {
		t.Fatalf("eval error %+v", r)
	}
	if r = tc.do(&Request{Op: OpEval, Window: 1, Script: "1"}); r.Code != CodeUnsupported {
		t.Fatalf("eval on canvas %+v", r)
	}
	if r = tc.do(&Request{Op: OpFind, Window: 2, Query: &Query{}}); r.Code != CodeUnsupported {
		t.Fatalf("find on webview %+v", r)
	}
	if r = tc.do(&Request{Op: OpFind, Window: 9, Query: &Query{}}); r.Code != CodeNoWindow {
		t.Fatalf("find on missing window %+v", r)
	}
	if r = tc.do(&Request{Op: OpFind, Window: 1, Query: &Query{Role: "nope"}}); r.Code != CodeBadRequest {
		t.Fatalf("bad role %+v", r)
	}
	if r = tc.do(&Request{Op: "dance"}); r.Code != CodeBadRequest {
		t.Fatalf("unknown op %+v", r)
	}

	// Hot reload: the canvas reports it; the error reaches the client.
	h.mu.Lock()
	c.err = ErrHotReload
	h.mu.Unlock()
	if r = tc.do(&Request{Op: OpFind, Window: 1, Query: &Query{}}); r.Code != CodeHotReload || !strings.Contains(r.Error, "热加载") {
		t.Fatalf("hot reload %+v", r)
	}

	// A panic in the canvas is reported; the connection keeps working.
	h.mu.Lock()
	c.err, c.panic = nil, true
	h.mu.Unlock()
	if r = tc.do(&Request{Op: OpCount, Window: 1, Query: &Query{}}); r.Error == "" {
		t.Fatal("panic not reported")
	}
	h.mu.Lock()
	c.panic = false
	h.mu.Unlock()
	if r = tc.do(&Request{Op: OpPing}); r.Error != "" {
		t.Fatal(r.Error)
	}

	// Quit answers, then quits.
	if r = tc.do(&Request{Op: OpQuit}); r.Error != "" {
		t.Fatal(r.Error)
	}
	select {
	case <-h.quit:
	case <-time.After(2 * time.Second):
		t.Fatal("no quit")
	}
}

func TestServerUnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix sockets: covered by TCP on Windows")
	}
	dir, err := shortTempDir(t)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "a.sock")
	s, _, _, tokenFile := startServer(t, "unix:"+path)
	_, r := connect(t, s, s.Token())
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	s.Close()
	if _, err := ReadTokenFile(tokenFile); err == nil {
		t.Fatal("token file left after Close")
	}
	if _, err := net.Dial("unix", path); err == nil {
		t.Fatal("socket still accepting after Close")
	}
}

func TestListenRejectsRemoteAddress(t *testing.T) {
	if _, err := Listen(new(int), &fakeHost{}, "0.0.0.0:0", ""); err == nil {
		t.Fatal("listened on all interfaces")
	}
}

func TestServerHelloFrameLimit(t *testing.T) {
	s, _, _, _ := startServer(t, "127.0.0.1:0")
	nc, err := Dial(s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	// Announce a 64 MiB hello: the server must hang up after the header
	// instead of reading (and allocating) the body.
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], MaxFrame)
	if _, err := nc.Write(h[:]); err != nil {
		t.Fatal(err)
	}
	nc.SetReadDeadline(time.Now().Add(2 * time.Second))
	var b [1]byte
	if _, err := nc.Read(b[:]); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("oversized hello not refused: %v", err)
	}
	// A hello just over the limit is refused too, even with the right token.
	nc2, _ := Dial(s.Addr(), time.Second)
	defer nc2.Close()
	if err := WriteFrame(nc2, &Request{ID: 1, Op: OpHello, Token: s.Token(), Script: strings.Repeat("x", MaxHelloFrame)}); err != nil {
		t.Fatal(err)
	}
	nc2.SetReadDeadline(time.Now().Add(2 * time.Second))
	var r Response
	if err := ReadFrame(nc2, &r); err == nil {
		t.Fatalf("hello over %d bytes answered: %+v", MaxHelloFrame, r)
	}
	// After authentication large frames are fine.
	tc, r2 := connect(t, s, s.Token())
	if r2.Error != "" {
		t.Fatal(r2.Error)
	}
	if r := tc.do(&Request{Op: OpPing, Script: strings.Repeat("x", 64<<10)}); r.Error != "" {
		t.Fatal(r.Error)
	}
}

func TestServerConnLimit(t *testing.T) {
	s, _, _, _ := startServer(t, "127.0.0.1:0")
	var held []net.Conn
	for i := 0; i < MaxConns; i++ {
		nc, err := Dial(s.Addr(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { nc.Close() })
		held = append(held, nc)
	}
	// One more is closed at once (the accept loop handles connections in
	// order, so the first MaxConns are registered by now).
	extra, err := Dial(s.Addr(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	extra.SetReadDeadline(time.Now().Add(2 * time.Second))
	var b [1]byte
	if _, err := extra.Read(b[:]); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("connection over the limit kept: %v", err)
	}
	// The held ones still work.
	tc := &testConn{t: t, c: held[0]}
	if r := tc.do(&Request{Op: OpHello, Token: s.Token()}); r.Error != "" {
		t.Fatal(r.Error)
	}
	// Freeing a slot lets a new client in.
	held[1].Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		nc, err := Dial(s.Addr(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		WriteFrame(nc, &Request{ID: 1, Op: OpHello, Token: s.Token()})
		nc.SetReadDeadline(time.Now().Add(time.Second))
		var r Response
		err = ReadFrame(nc, &r)
		nc.Close()
		if err == nil && r.Error == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot not freed: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
