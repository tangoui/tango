package automation

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// Host is the app side the server needs (package tango implements it).
type Host interface {
	// Main runs fn on the main thread and waits. It reports false when
	// the app has quit or fn panicked (the panic went to the app's panic
	// handling).
	Main(fn func()) bool
	// Guard runs fn on the calling goroutine, handing a panic to the
	// app's panic handling; it reports whether fn returned normally.
	Guard(fn func()) bool
	// Windows lists the open windows; Kind may be left empty (the server
	// fills it in). Main thread.
	Windows() []WindowInfo
	// Eval evaluates a JavaScript expression in a WebView window and
	// returns its JSON value. Any goroutine.
	Eval(win uint64, expr string, timeout time.Duration) ([]byte, error)
	// Screenshot captures a WebView window as PNG. Any goroutine.
	Screenshot(win uint64) ([]byte, error)
	// Quit asks the app to quit. Any goroutine.
	Quit()
}

// Canvas is a self-drawn window (package uihost registers one per
// window). Its methods run on the main thread.
type Canvas interface {
	// Tree returns the window's current semantics tree (nil until the
	// first frame that builds one) and a generation that changes with
	// every new tree. The first call turns the tree on.
	Tree() (tree *a11y.Tree, gen uint64, err error)
	// Perform asks node id to perform act (ui.Runner.PerformAction).
	Perform(id a11y.NodeID, act a11y.Action, arg string) error
	// Input delivers events as the platform would.
	Input(evs []input.Event) error
	// Snapshot draws the window's last frame.
	Snapshot() (*image.RGBA, error)
}

// ErrHotReload is what Canvas implementations return under tan dev.
var ErrHotReload = &Error{Code: CodeHotReload,
	Msg: "热加载模式（tan dev）不支持自动化自绘窗口，请用 go run 或 tan build 的开发构建（hot reload is not supported）"}

var registry struct {
	mu sync.Mutex
	m  map[regKey]Canvas
}

type regKey struct {
	app any
	win uint64
}

// Register makes c the self-drawn window win of app (a *tango.App); the
// returned function removes it (idempotent).
func Register(app any, win uint64, c Canvas) (unregister func()) {
	k := regKey{app, win}
	registry.mu.Lock()
	if registry.m == nil {
		registry.m = map[regKey]Canvas{}
	}
	registry.m[k] = c
	registry.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			registry.mu.Lock()
			if registry.m[k] == c {
				delete(registry.m, k)
			}
			registry.mu.Unlock()
		})
	}
}

func lookup(app any, win uint64) Canvas {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.m[regKey{app, win}]
}

// Server is a running control channel.
type Server struct {
	app       any
	host      Host
	ln        net.Listener
	addr      string // TokenFile.Addr
	token     string
	unixPath  string
	tokenFile string

	// Timings, copied from the package variables at Listen.
	helloTimeout, pollEvery, treeTimeout, settleFor time.Duration
	maxConns                                        int

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
	wg     sync.WaitGroup
}

// Listen starts the control channel of app on spec (an EnvAddr value).
// When tokenFile is not "" the address and token are written there,
// otherwise printed to stderr.
func Listen(app any, h Host, spec, tokenFile string) (*Server, error) {
	network, addr, err := ParseAddr(spec)
	if err != nil {
		return nil, err
	}
	token, err := NewToken()
	if err != nil {
		return nil, err
	}
	if network == "unix" {
		// A stale socket of an earlier run blocks Listen; only remove
		// sockets, never other files.
		if fi, err := os.Lstat(addr); err == nil && fi.Mode()&os.ModeSocket != 0 {
			os.Remove(addr)
		}
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, fmt.Errorf("automation: %v", err)
	}
	s := &Server{app: app, host: h, ln: ln, token: token, tokenFile: tokenFile, conns: map[net.Conn]struct{}{},
		helloTimeout: helloTimeout, pollEvery: pollEvery, treeTimeout: treeTimeout, settleFor: settleFor,
		maxConns: maxConns}
	if network == "unix" {
		s.unixPath = addr
		os.Chmod(addr, 0o600)
		s.addr = "unix:" + addr
	} else {
		s.addr = "tcp:" + ln.Addr().String()
	}
	if tokenFile != "" {
		if err := WriteTokenFile(tokenFile, TokenFile{Addr: s.addr, Token: token, PID: os.Getpid(), Version: Version}); err != nil {
			ln.Close()
			return nil, fmt.Errorf("automation: token file: %v", err)
		}
		log.Printf("tango: automation listening on %s (token in %s)", s.addr, tokenFile)
	} else {
		log.Printf("tango: automation listening on %s token %s", s.addr, token)
	}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Addr returns the address clients dial (TokenFile.Addr).
func (s *Server) Addr() string { return s.addr }

// Token returns the token clients must present.
func (s *Server) Token() string { return s.token }

// Close stops listening, drops the connections and removes the socket and
// token files. It waits for the connection handlers to return.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	err := s.ln.Close()
	s.wg.Wait()
	if s.unixPath != "" {
		os.Remove(s.unixPath)
	}
	if s.tokenFile != "" {
		os.Remove(s.tokenFile)
	}
	return err
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return
		}
		if len(s.conns) >= s.maxConns {
			// Too many at once: refuse without reading anything, so
			// unauthenticated peers cannot pile up goroutines and buffers.
			s.mu.Unlock()
			c.Close()
			continue
		}
		s.conns[c] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.conns, c)
				s.mu.Unlock()
				c.Close()
			}()
			s.host.Guard(func() { s.conn(c) })
		}()
	}
}

// helloTimeout bounds the wait for the whole first request (header and
// body: the deadline is absolute).
var helloTimeout = 5 * time.Second

// maxConns is MaxConns, a variable for tests (read by Listen only).
var maxConns = MaxConns

func (s *Server) conn(c net.Conn) {
	c.SetReadDeadline(time.Now().Add(s.helloTimeout))
	var req Request
	// Unauthenticated: only a small frame is accepted.
	if err := ReadFrameLimit(c, &req, MaxHelloFrame); err != nil {
		return
	}
	if req.Op != OpHello || !TokenOK(s.token, req.Token) {
		WriteFrame(c, &Response{ID: req.ID, Code: CodeAuth, Error: "automation: missing or wrong token"})
		return
	}
	c.SetReadDeadline(time.Time{})
	if WriteFrame(c, &Response{ID: req.ID, Version: Version}) != nil {
		return
	}
	for {
		var req Request
		if err := ReadFrame(c, &req); err != nil {
			return
		}
		var resp *Response
		if !s.host.Guard(func() { resp = s.dispatch(&req) }) {
			resp = fail(CodeInternal, "automation: internal panic while handling %q (see the app's log)", req.Op)
		}
		resp.ID = req.ID
		if WriteFrame(c, resp) != nil {
			return
		}
		if req.Op == OpQuit && resp.Error == "" {
			s.host.Quit()
			return
		}
	}
}

func fail(code, format string, args ...any) *Response {
	return &Response{Code: code, Error: fmt.Sprintf(format, args...)}
}

func failErr(err error) *Response {
	var e *Error
	if errors.As(err, &e) {
		return &Response{Code: e.Code, Error: e.Msg}
	}
	return &Response{Code: CodeInternal, Error: err.Error()}
}

// main runs fn on the main thread.
func (s *Server) main(fn func()) error {
	if !s.host.Main(fn) {
		return &Error{Code: CodeQuit, Msg: "automation: the app has quit or the call panicked (see the app's log)"}
	}
	return nil
}

func (s *Server) dispatch(req *Request) *Response {
	switch req.Op {
	case OpPing:
		return &Response{}
	case OpHello:
		return &Response{Version: Version}
	case OpQuit:
		return &Response{}
	case OpWindows:
		var ws []WindowInfo
		if err := s.main(func() { ws = s.host.Windows() }); err != nil {
			return failErr(err)
		}
		for i := range ws {
			if lookup(s.app, ws[i].ID) != nil {
				ws[i].Kind = KindCanvas
			} else {
				ws[i].Kind = KindWebView
			}
		}
		return &Response{Windows: ws}
	case OpEval:
		if lookup(s.app, req.Window) != nil {
			return fail(CodeUnsupported, "automation: window %d is self-drawn: Eval needs a WebView window", req.Window)
		}
		if !s.windowExists(req.Window) {
			return fail(CodeNoWindow, "automation: no window %d", req.Window)
		}
		to := req.TimeoutDuration()
		if to <= 0 {
			to = 10 * time.Second
		}
		v, err := s.host.Eval(req.Window, req.Script, to)
		if err != nil {
			return failErr(err)
		}
		return &Response{Value: v}
	case OpScreenshot:
		if c := lookup(s.app, req.Window); c != nil {
			var img *image.RGBA
			var err error
			if e := s.main(func() { img, err = c.Snapshot() }); e != nil {
				return failErr(e)
			}
			if err != nil {
				return failErr(err)
			}
			var b bytes.Buffer
			if err := png.Encode(&b, img); err != nil {
				return failErr(err)
			}
			return &Response{PNG: b.Bytes()}
		}
		if !s.windowExists(req.Window) {
			return fail(CodeNoWindow, "automation: no window %d", req.Window)
		}
		p, err := s.host.Screenshot(req.Window)
		if err != nil {
			return failErr(err)
		}
		return &Response{PNG: p}
	case OpTree, OpFind, OpCount, OpWait, OpAction, OpInput:
		c := lookup(s.app, req.Window)
		if c == nil {
			if s.windowExists(req.Window) {
				return fail(CodeUnsupported, "automation: window %d is a WebView: use Eval and the selector helpers", req.Window)
			}
			return fail(CodeNoWindow, "automation: no self-drawn window %d (closed?)", req.Window)
		}
		return s.canvasOp(c, req)
	}
	return fail(CodeBadRequest, "automation: unknown operation %q", req.Op)
}

func (s *Server) windowExists(id uint64) bool {
	found := false
	s.main(func() {
		for _, w := range s.host.Windows() {
			if w.ID == id {
				found = true
			}
		}
	})
	return found
}

// pollEvery is the interval of tree polling while waiting.
var pollEvery = 15 * time.Millisecond

// snapshot reads c's tree on the main thread and calls fn there with it
// (nil: no tree yet).
func (s *Server) snapshot(c Canvas, fn func(t *a11y.Tree, gen uint64)) error {
	var err error
	if e := s.main(func() {
		var t *a11y.Tree
		var gen uint64
		t, gen, err = c.Tree()
		if err == nil {
			fn(t, gen)
		}
	}); e != nil {
		return e
	}
	return err
}

// waitTree polls until c has a tree for which done reports true, or the
// timeout passes (one check at least).
func (s *Server) waitTree(c Canvas, timeout time.Duration, done func(t *a11y.Tree) bool) (timedOut bool, err error) {
	deadline := time.Now().Add(timeout)
	for {
		ok := false
		if err := s.snapshot(c, func(t *a11y.Tree, _ uint64) { ok = t != nil && done(t) }); err != nil {
			return false, err
		}
		if ok {
			return false, nil
		}
		if !time.Now().Before(deadline) {
			return true, nil
		}
		time.Sleep(s.pollEvery)
	}
}

// treeTimeout is how long a request without a timeout waits for the
// first tree after turning it on (one or two frames normally).
var treeTimeout = 5 * time.Second

func (s *Server) canvasOp(c Canvas, req *Request) *Response {
	switch req.Op {
	case OpTree:
		var resp *Response
		to := max(req.TimeoutDuration(), s.treeTimeout)
		timedOut, err := s.waitTree(c, to, func(t *a11y.Tree) bool {
			resp = &Response{Dump: t.Dump()}
			for _, n := range t.Nodes() {
				resp.Nodes = append(resp.Nodes, *n)
			}
			return true
		})
		if err != nil {
			return failErr(err)
		}
		if timedOut {
			return fail(CodeTimeout, "automation: window %d produced no semantics tree within %v", req.Window, to)
		}
		return resp
	case OpInput:
		batches, err := Events(req.Input)
		if err != nil {
			return fail(CodeBadRequest, "%v", err)
		}
		var gen0 uint64
		for i, b := range batches {
			if i > 0 {
				time.Sleep(20 * time.Millisecond) // let a frame run between drag steps
			}
			var ierr error
			if e := s.main(func() {
				if i == 0 {
					_, gen0, _ = c.Tree()
				}
				ierr = c.Input(b)
			}); e != nil {
				return failErr(e)
			}
			if ierr != nil {
				return failErr(ierr)
			}
		}
		s.settle(c, gen0)
		return &Response{}
	}

	if req.Query == nil {
		return fail(CodeBadRequest, "automation: %s needs a query", req.Op)
	}
	m, err := req.Query.Compile()
	if err != nil {
		return fail(CodeBadRequest, "%v", err)
	}
	q := *req.Query
	switch req.Op {
	case OpCount:
		resp := &Response{}
		_, err := s.waitTree(c, s.treeTimeout, func(t *a11y.Tree) bool {
			for _, n := range m.All(t) {
				resp.Nodes = append(resp.Nodes, *n)
			}
			resp.Count = len(resp.Nodes)
			return true
		})
		if err != nil {
			return failErr(err)
		}
		return resp
	case OpFind, OpWait, OpAction:
		gone := req.Op == OpWait && req.Gone
		node, count, dump, timedOut, err := s.resolve(c, m, gone, req.TimeoutDuration())
		if err != nil {
			return failErr(err)
		}
		if timedOut {
			return notFound(q, gone, count, req.TimeoutDuration(), dump)
		}
		if req.Op != OpAction {
			return &Response{Node: node, Count: count}
		}
		act, ok := ParseAction(req.Action)
		if !ok {
			return fail(CodeBadRequest, "automation: unknown action %q", req.Action)
		}
		var gen0 uint64
		var perr error
		if e := s.main(func() {
			_, gen0, _ = c.Tree()
			perr = c.Perform(node.ID, act, req.Arg)
		}); e != nil {
			return failErr(e)
		}
		if perr != nil {
			return fail(CodeAction, "automation: %s: %s: %v (supported: %s)", q, req.Action, perr, node.Actions)
		}
		s.settle(c, gen0)
		return &Response{Node: node, Count: count}
	}
	return fail(CodeBadRequest, "automation: unknown operation %q", req.Op)
}

func notFound(q Query, gone bool, count int, to time.Duration, dump string) *Response {
	var msg string
	switch {
	case gone:
		msg = fmt.Sprintf("automation: %s still present after %v", q, to)
	case count > 0:
		msg = fmt.Sprintf("automation: %s: only %d match(es) after %v", q, count, to)
	default:
		msg = fmt.Sprintf("automation: no %s after %v", q, to)
	}
	return &Response{Code: CodeTimeout, Error: msg, Dump: dump}
}

// resolve waits until m picks a node (or, with gone, picks none). On
// timeout it returns the last count and a dump of the last tree. Before
// the first tree exists (it is being turned on) it waits for it even with
// a zero timeout, then checks once.
func (s *Server) resolve(c Canvas, m *Matcher, gone bool, timeout time.Duration) (node *a11y.Node, count int, dump string, timedOut bool, err error) {
	var last *a11y.Tree // trees are immutable: safe to read off the main thread
	ok := false
	check := func(t *a11y.Tree) bool {
		last = t
		n, k := m.Pick(t)
		count = k
		node = nil
		if n != nil {
			cp := *n
			node = &cp
		}
		ok = (n == nil) == gone
		return ok
	}
	timedOut, err = s.waitTree(c, timeout, check)
	if err == nil && timedOut && last == nil {
		_, err = s.waitTree(c, s.treeTimeout, func(t *a11y.Tree) bool { check(t); return true })
		timedOut = !ok
	}
	if timedOut && last != nil {
		dump = last.Dump()
	}
	return
}

// settleFor bounds how long an action waits for the tree to change.
var settleFor = 400 * time.Millisecond

// settle waits until c's tree generation differs from gen0 (the frame
// that handled an action or input has run) or settleFor passes.
func (s *Server) settle(c Canvas, gen0 uint64) {
	deadline := time.Now().Add(s.settleFor)
	for time.Now().Before(deadline) {
		changed := false
		if s.snapshot(c, func(_ *a11y.Tree, gen uint64) { changed = gen != gen0 }) != nil || changed {
			return
		}
		time.Sleep(s.pollEvery)
	}
}
