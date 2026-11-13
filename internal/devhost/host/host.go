// Package host is the hot-reload host process: it owns the canvas windows
// of an app run by `tan dev` and shows frames rendered by worker processes
// (each rebuild of the app), forwarding input to the current worker and
// swapping workers without closing windows. See
// docs/specs/ui-hotreload.md and package devhost for the protocol.
//
// The host is the app binary itself started with TANGO_DEV_ROLE=host:
// package uihost calls Main from its init, on the main thread, and exits
// without running the app's main. It only runs framework code.
package host

import (
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/appctl"
	"github.com/tangoui/tango/sys/clipboard"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Tunables (variables for tests).
var (
	// ResizeWait is how long CanvasFrame waits for a frame of the new
	// size after a resize, so live resize shows fresh content.
	ResizeWait = 50 * time.Millisecond
	// RequestTimeout re-sends a FrameRequest a worker did not answer.
	RequestTimeout = time.Second
	// SnapshotTimeout bounds waiting for the old worker's state.
	SnapshotTimeout = 2 * time.Second
	// OrphanGrace keeps windows of a replaced worker that the new worker
	// did not reopen (yet) before closing them.
	OrphanGrace = 2 * time.Second
	// NoWindowCommit swaps to a new worker that opened no window at all
	// after this long.
	NoWindowCommit = 5 * time.Second
	// FreezeLimit bounds how long input is held back from the old worker
	// for a new one that does not show a frame.
	FreezeLimit = 10 * time.Second
	// ReclaimWindow is how long after a swap the host takes activation
	// back from the new worker (see Host.lifecycle).
	ReclaimWindow = 2 * time.Second
)

// Main runs the host until its app quits or tan dev goes away and returns
// the process exit code. It must run on the main thread.
func Main() int {
	addr := os.Getenv(devhost.EnvAddr)
	name := os.Getenv(devhost.EnvName)
	if name == "" {
		name = "Tango"
	}
	token := os.Getenv(devhost.EnvToken)
	if token == "" && strings.HasPrefix(addr, "tcp:") {
		// A TCP port is open to every local user: never without a token.
		log.Printf("tango devhost: %s is not set", devhost.EnvToken)
		return 1
	}
	quit := false
	app := tango.New(tango.Options{Name: name, ID: os.Getenv(devhost.EnvID), QuitOnLastClose: &quit})
	ln, err := devhost.Listen(addr)
	if err != nil {
		log.Printf("tango devhost: %v", err)
		return 1
	}
	defer ln.Close()
	h := New(app)
	h.Token = token
	app.OnLifecycle(h.lifecycle)
	app.OnReady(func() { go h.Serve(ln) })
	if err := app.Run(); err != nil {
		log.Printf("tango devhost: %v", err)
		return 1
	}
	return 0
}

// Host is the hot-reload host. Its state is owned by the app's main thread
// unless noted.
type Host struct {
	app *tango.App

	// Token, when set before Serve, is the session token every peer must
	// send in Hello (devhost.EnvToken); others are refused.
	Token string

	wins    map[int]*win
	nextWin int
	active  *worker // worker on screen
	pending *worker // newest worker, until its first frame
	gen     int
	banner  devhost.Status
	ban     bannerCache

	snapWait map[int]snapWaiter // SnapshotReq id → requester
	nextSnap int
	seed     []devhost.Entry // state from a previous host (tan dev)
	queued   []queuedInput   // input held back while the active worker is frozen
	menus    map[int]bool    // menus a worker set (0: app menu, else window id); surface.go

	ctlMu sync.Mutex
	ctl   *devhost.Conn

	front        bool      // the host app is active (macOS)
	frontAt      time.Time // when it last stopped being active
	reclaimUntil time.Time // take activation back until then
}

// lifecycle tracks whether the host is the active app. A new worker
// process gets activated by the system shortly after it starts and then
// hides itself (uihost), which hands activation to some other app. If the
// host was in front when the worker connected, it takes activation back
// when it loses it during the swap; otherwise the user was elsewhere (the
// editor) and the host stays out of the way.
func (h *Host) lifecycle(ev tango.Lifecycle) {
	trace("lifecycle %v", ev)
	switch ev {
	case tango.Foreground:
		h.front = true
	case tango.Background:
		if h.front {
			h.frontAt = time.Now()
		}
		h.front = false
		if time.Now().Before(h.reclaimUntil) {
			trace("taking activation back from the new worker")
			_ = appctl.Show(h.app)
		}
	}
}

// wasFront reports whether the host was active just now.
func (h *Host) wasFront() bool { return h.front || time.Since(h.frontAt) < 300*time.Millisecond }

// reclaimFocus arms lifecycle to take activation back for a while after
// worker p went on screen, if the host was in front when p connected.
func (h *Host) reclaimFocus(p *worker) {
	trace("reclaim: host was front %v, is front %v", p.hostFront, h.front)
	if !p.hostFront {
		return
	}
	h.reclaimUntil = time.Now().Add(ReclaimWindow)
	if !h.front {
		_ = appctl.Show(h.app)
	}
}

var traceOn = os.Getenv("TANGO_DEV_TRACE") != ""

func trace(format string, a ...any) {
	if traceOn {
		log.Printf("tango devhost: trace: "+format, a...)
	}
}

type snapWaiter struct {
	w     *worker // nil: tan dev asked
	req   int
	reply func([]devhost.Entry)
	timer *time.Timer
}

type queuedInput struct {
	win int
	m   any // devhost.Input or devhost.Focus
}

// New returns a host for app. Call Serve once the app runs.
func New(app *tango.App) *Host {
	return &Host{app: app, wins: map[int]*win{}, snapWait: map[int]snapWaiter{}, menus: map[int]bool{}}
}

// Serve accepts connections until ln closes.
func (h *Host) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go h.serveConn(devhost.NewConn(c))
	}
}

func (h *Host) post(fn func()) { h.app.Post(fn) }

func (h *Host) serveConn(c *devhost.Conn) {
	m, err := c.Recv()
	if err != nil {
		c.Close()
		return
	}
	hello, ok := m.(devhost.Hello)
	if !ok {
		c.Close()
		return
	}
	if !devhost.TokenOK(hello.Token, h.Token) {
		_ = c.Send(devhost.Welcome{Err: "bad session token"})
		c.Close()
		return
	}
	if hello.Proto != devhost.ProtoVersion {
		_ = c.Send(devhost.Welcome{Err: fmt.Sprintf("protocol %d, host speaks %d", hello.Proto, devhost.ProtoVersion)})
		c.Close()
		return
	}
	switch hello.Role {
	case devhost.RoleCtl:
		h.serveCtl(c)
	case devhost.RoleWorker:
		h.serveWorker(c, hello)
	default:
		_ = c.Send(devhost.Welcome{Err: "unknown role " + hello.Role})
		c.Close()
	}
}

// ---- tan dev control connection ----

func (h *Host) serveCtl(c *devhost.Conn) {
	if err := c.Send(devhost.Welcome{}); err != nil {
		c.Close()
		return
	}
	h.ctlMu.Lock()
	if h.ctl != nil {
		h.ctl.Close()
	}
	h.ctl = c
	h.ctlMu.Unlock()
	for {
		m, err := c.Recv()
		if err != nil {
			break
		}
		switch m := m.(type) {
		case devhost.Status:
			h.post(func() { h.setBanner(m) })
		case devhost.SnapshotGet: // tan dev saves the state before restarting the host
			h.post(func() { h.snapshotGet(nil, m.Req) })
		case devhost.SnapshotData: // ... and hands it to the new host
			h.post(func() { h.seed = m.Entries })
		}
	}
	h.ctlMu.Lock()
	gone := h.ctl == c
	if gone {
		h.ctl = nil
	}
	h.ctlMu.Unlock()
	if gone {
		// tan dev went away: nothing will manage the workers any more.
		h.app.Quit()
	}
}

func (h *Host) notify(m any) {
	h.ctlMu.Lock()
	c := h.ctl
	h.ctlMu.Unlock()
	if c != nil {
		_ = c.Send(m)
	}
}

// ---- workers ----

// worker is one connected worker process.
type worker struct {
	h    *Host
	conn *devhost.Conn
	pid  int
	gen  int
	at   time.Time // connected

	mu    sync.Mutex
	segs  map[int]map[int]*devhost.Segment // win → seg gen → mapping
	inbox map[int]devhost.FrameReady       // win → newest frame not yet taken
	have  map[int]bool                     // win has a frame in inbox
	// arrive is signalled when a frame lands in inbox (CanvasFrame's
	// resize wait).
	arrive chan struct{}

	// Main thread.
	gone    bool
	retired bool                    // replaced by a newer worker
	frozen  bool                    // its state was snapshotted; input is held back
	opened  int                     // OpenWindow calls
	menus   map[int]devhost.SetMenu // its SetMenu calls by window; surface.go
	// hostFront: the host was the active app when this worker connected.
	hostFront bool
}

func (h *Host) serveWorker(c *devhost.Conn, hello devhost.Hello) {
	w := &worker{h: h, conn: c, pid: hello.PID, at: time.Now(),
		segs: map[int]map[int]*devhost.Segment{}, inbox: map[int]devhost.FrameReady{}, have: map[int]bool{},
		arrive: make(chan struct{}, 1), menus: map[int]devhost.SetMenu{}}
	ready := make(chan struct{})
	h.post(func() {
		h.gen++
		w.gen = h.gen
		h.addWorker(w)
		close(ready)
	})
	<-ready
	if err := c.Send(devhost.Welcome{Gen: w.gen}); err != nil {
		c.Close()
	}
	for {
		m, err := c.Recv()
		if err != nil {
			break
		}
		switch m := m.(type) {
		case devhost.ShmAttach:
			w.attach(m)
		case devhost.FrameReady:
			w.frameReady(m)
		case devhost.ReadClipboard:
			go func() {
				s, _ := clipboard.ReadText(h.app)
				_ = c.Send(devhost.ClipboardText{Req: m.Req, Text: s})
			}()
		case devhost.WriteClipboard:
			go func() { _ = clipboard.WriteText(h.app, m.Text) }()
		default:
			h.post(func() { h.handle(w, m) })
		}
	}
	c.Close()
	h.post(func() { h.workerGone(w) })
}

// attach maps a new segment. Reading goroutine.
func (w *worker) attach(m devhost.ShmAttach) {
	seg, err := devhost.OpenSegment(m.Name, m.Size)
	if err != nil {
		log.Printf("tango devhost: %v", err)
		return
	}
	seg.Unlink()
	w.mu.Lock()
	if w.segs[m.Win] == nil {
		w.segs[m.Win] = map[int]*devhost.Segment{}
	}
	w.segs[m.Win][m.Seg] = seg
	w.mu.Unlock()
}

// frameReady stores a frame in the inbox, releasing one it replaces, and
// tells the main thread. Reading goroutine.
func (w *worker) frameReady(f devhost.FrameReady) {
	w.mu.Lock()
	old, had := w.inbox[f.Win], w.have[f.Win]
	w.inbox[f.Win], w.have[f.Win] = f, true
	w.mu.Unlock()
	if had {
		_ = w.conn.Send(devhost.FrameAck{Win: old.Win, Seg: old.Seg, Slot: old.Slot})
	}
	select {
	case w.arrive <- struct{}{}:
	default:
	}
	w.h.post(func() { w.h.frameArrived(w, f.Win) })
}

// take removes and returns the inbox frame of win. Any goroutine.
func (w *worker) take(win int) (devhost.FrameReady, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, ok := w.inbox[win], w.have[win]
	delete(w.inbox, win)
	delete(w.have, win)
	return f, ok
}

// peekSize returns the size of the inbox frame of win.
func (w *worker) peekSize(win int) (int, int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, ok := w.inbox[win], w.have[win]
	return f.W, f.H, ok
}

func (w *worker) segment(win, gen int) *devhost.Segment {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.segs[win][gen]
}

// dropSegs unmaps the segments of win older than keep (all when keep < 0).
// Main thread, when nothing on screen refers to them.
func (w *worker) dropSegs(win, keep int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for g, s := range w.segs[win] {
		if keep < 0 || g < keep {
			s.Close()
			delete(w.segs[win], g)
		}
	}
	if len(w.segs[win]) == 0 {
		delete(w.segs, win)
	}
}

func (w *worker) send(m any) {
	if !w.gone {
		_ = w.conn.Send(m)
	}
}

// addWorker makes w the pending worker. Main thread.
func (h *Host) addWorker(w *worker) {
	if p := h.pending; p != nil {
		// An even newer build arrived before the previous one showed
		// anything: drop the previous one.
		h.abandonPending(p)
		p.conn.Close()
	}
	h.pending = w
	// The first worker of a fresh host: the app was just launched, so its
	// window belongs in front.
	w.hostFront = h.wasFront() || (h.active == nil && len(h.wins) == 0)
	time.AfterFunc(NoWindowCommit, func() {
		h.post(func() {
			if h.pending == w && !w.gone && w.opened == 0 {
				h.commit(w)
			}
		})
	})
}

// handle runs a worker message on the main thread.
func (h *Host) handle(w *worker, m any) {
	if w.gone {
		return
	}
	switch m := m.(type) {
	case devhost.OpenWindow:
		h.openWindow(w, m)
	case devhost.CloseWindow:
		if x := h.wins[m.Win]; x != nil {
			switch {
			case x.owner == w:
				x.surf.Close()
			case x.next == w:
				x.next = nil
				if x.owner == nil {
					x.surf.Close()
				}
			}
		}
	case devhost.Invalidate:
		if x := h.wins[m.Win]; x != nil {
			switch {
			case x.owner == w:
				x.wantFrame = true
				x.cv.Invalidate()
			case x.next == w:
				h.requestFrom(x, w)
			}
		}
	case devhost.SetCursor:
		if x := h.wins[m.Win]; x != nil {
			switch {
			case x.owner == w:
				x.cv.SetCursor(m.Cursor)
			case x.next == w:
				x.nextCursor = &m.Cursor
			}
		}
	case devhost.SetTextInput:
		if x := h.wins[m.Win]; x != nil {
			switch {
			case x.owner == w:
				x.cv.SetTextInput(m.Enabled, m.Caret)
			case x.next == w:
				x.nextText = &m
			}
		}
	case devhost.SurfaceOp:
		h.surfaceOp(w, m)
	case devhost.SetMenu:
		h.setMenu(w, m)
	case devhost.SnapshotGet:
		h.snapshotGet(w, m.Req)
	case devhost.SnapshotReply:
		h.snapshotReply(w, m)
	case devhost.A11yTree:
		h.a11yTree(w, m) // a11y.go
	}
}

// workerGone handles a closed worker connection. Main thread.
func (h *Host) workerGone(w *worker) {
	if w.gone {
		return
	}
	w.gone = true
	wasActive := h.active == w
	if h.pending == w {
		h.abandonPending(w)
	}
	if wasActive {
		h.active = nil
	}
	for id, x := range h.wins {
		if x.held == nil || x.held.w != w {
			w.dropSegs(id, -1)
		}
		if x.outstanding == w {
			x.outstanding = nil
		}
	}
	h.a11yWorkerGone(w) // a11y.go
	h.notify(devhost.WorkerLeft{PID: w.pid, Active: wasActive})
}

// abandonPending forgets the pending worker w (it died or was superseded):
// windows it created go away, windows it claimed stay with their owner and
// input held back for it goes to the active worker.
func (h *Host) abandonPending(w *worker) {
	if h.pending != w {
		return
	}
	h.pending = nil
	for _, x := range h.wins {
		if x.next == w {
			x.next, x.nextCursor, x.nextText, x.nextTitle, x.nextOps = nil, nil, nil, "", nil
			if x.owner == nil {
				x.surf.Close()
			}
		}
	}
	if a := h.active; a != nil && a.frozen {
		a.frozen = false
		h.replay(a)
	}
}

// replay delivers held-back input to the windows' owners.
func (h *Host) replay(to *worker) {
	q := h.queued
	h.queued = nil
	for _, in := range q {
		if x := h.wins[in.win]; x != nil && x.owner == to {
			to.send(in.m)
			x.wantFrame = true
			x.cv.Invalidate()
		}
	}
}

// commit puts the pending worker p on screen and retires the active one.
func (h *Host) commit(p *worker) {
	if h.pending != p || p.gone {
		return
	}
	old := h.active
	h.pending = nil
	h.active = p
	for _, x := range h.wins {
		if x.next != p {
			continue
		}
		x.owner, x.next = p, nil
		x.outstanding = nil
		x.orphanedAt = time.Time{}
		if x.nextCursor != nil {
			x.cv.SetCursor(*x.nextCursor)
		}
		if x.nextText != nil {
			x.cv.SetTextInput(x.nextText.Enabled, x.nextText.Caret)
		} else {
			x.cv.SetTextInput(false, geom.Rect{})
		}
		if x.nextTitle != "" {
			x.surf.SetTitle(x.nextTitle)
		}
		for _, op := range x.nextOps {
			h.applyOp(x, op)
		}
		x.nextCursor, x.nextText, x.nextTitle, x.nextOps = nil, nil, "", nil
		h.presentInbox(x)
		x.wantFrame = true
		x.cv.Invalidate()
	}
	if old != nil {
		old.retired = true
		for id, x := range h.wins {
			if x.owner == old {
				x.owner = nil
				x.orphanedAt = time.Now()
				h.scheduleOrphanCheck(id)
			}
		}
		old.conn.Close() // the old worker exits when its connection ends
	}
	h.commitMenus(p)
	h.replay(p)
	h.reclaimFocus(p)
	oldPID := 0
	if old != nil {
		oldPID = old.pid
	}
	h.notify(devhost.Swapped{Old: oldPID, New: p.pid, Took: time.Since(p.at)})
}

func (h *Host) scheduleOrphanCheck(id int) {
	time.AfterFunc(OrphanGrace, func() {
		h.post(func() {
			x := h.wins[id]
			if x != nil && x.owner == nil && x.next == nil && !x.orphanedAt.IsZero() && time.Since(x.orphanedAt) >= OrphanGrace-10*time.Millisecond {
				x.surf.Close()
			}
		})
	})
}

// ---- snapshots ----

// snapshotGet asks the active worker for its Persist state on behalf of
// worker w, or of tan dev when w is nil (before restarting the host).
func (h *Host) snapshotGet(w *worker, req int) {
	reply := func(entries []devhost.Entry) {
		if w == nil {
			h.notify(devhost.SnapshotData{Req: req, Entries: entries})
		} else {
			w.send(devhost.SnapshotData{Req: req, Entries: entries})
		}
	}
	a := h.active
	if a == nil || a == w || a.gone {
		// No running app to ask: a seed handed over from the previous
		// host, if any, serves the first worker.
		seed := h.seed
		h.seed = nil
		reply(seed)
		return
	}
	h.nextSnap++
	id := h.nextSnap
	sw := snapWaiter{w: w, req: req, reply: reply}
	sw.timer = time.AfterFunc(SnapshotTimeout, func() {
		h.post(func() {
			if s, ok := h.snapWait[id]; ok {
				delete(h.snapWait, id)
				log.Printf("tango devhost: the running app did not send its state in time")
				s.reply(nil)
			}
		})
	})
	h.snapWait[id] = sw
	a.send(devhost.SnapshotReq{Req: id})
}

func (h *Host) snapshotReply(from *worker, m devhost.SnapshotReply) {
	s, ok := h.snapWait[m.Req]
	if !ok {
		return
	}
	delete(h.snapWait, m.Req)
	s.timer.Stop()
	if s.w == nil { // tan dev
		s.reply(m.Entries)
		return
	}
	if s.w.gone || h.pending != s.w {
		return
	}
	// From now on the old worker's state could diverge from what the new
	// one restores: hold input back until the swap.
	if from == h.active {
		from.frozen = true
		p := s.w
		time.AfterFunc(FreezeLimit, func() {
			h.post(func() {
				// The new worker never made it to the screen: stop holding
				// input back from the old one.
				if from.frozen && h.active == from && h.pending == p {
					from.frozen = false
					h.replay(from)
				}
			})
		})
	}
	s.reply(m.Entries)
}

// ---- windows ----

// win is one host window.
type win struct {
	h    *Host
	id   int
	seq  int
	surf *tango.Surface
	cv   platform.Canvas

	title       string
	owner       *worker // draws the window
	next        *worker // pending worker that reopened it
	nextCursor  *input.Cursor
	nextText    *devhost.SetTextInput
	nextTitle   string
	nextOps     []devhost.SurfaceOp // Surface calls of next, applied when it takes over
	ax          winA11y             // accessibility (a11y.go)
	held        *heldFrame          // on screen
	wantFrame   bool
	outstanding *worker // FrameRequest sent, no frame yet
	requestedAt time.Time
	orphanedAt  time.Time
	closed      bool

	statsAt  time.Time
	statsN   int
	statsDur time.Duration
}

// stats logs the presented frame rate every 2s (TANGO_DEV_TRACE).
func (x *win) stats(f devhost.FrameReady, d time.Duration) {
	if x.statsAt.IsZero() {
		x.statsAt = time.Now()
	}
	x.statsN++
	x.statsDur += d
	if el := time.Since(x.statsAt); el >= 2*time.Second {
		trace("window %d: %.1f frames/s presented, %dx%d px, present %v/frame",
			x.id, float64(x.statsN)/el.Seconds(), f.W, f.H, (x.statsDur / time.Duration(x.statsN)).Round(10*time.Microsecond))
		x.statsAt, x.statsN, x.statsDur = time.Now(), 0, 0
	}
}

type heldFrame struct {
	w *worker
	f devhost.FrameReady
}

func (h *Host) openWindow(w *worker, m devhost.OpenWindow) {
	w.opened++
	reply := func(x *win, err error) {
		if err != nil {
			w.send(devhost.Opened{Req: m.Req, Err: err.Error()})
			return
		}
		cw, ch := x.cv.Size()
		w.send(devhost.Opened{Req: m.Req, Win: x.id, W: cw, H: ch, Scale: x.cv.Scale()})
	}
	if x := h.match(w, m); x != nil {
		if w == h.pending {
			x.next = w
			if m.Opts.Title != x.title {
				x.nextTitle = m.Opts.Title
			}
		} else {
			x.owner = w
			x.orphanedAt = time.Time{}
			if m.Opts.Title != x.title {
				x.surf.SetTitle(m.Opts.Title)
			}
		}
		x.title, x.seq = m.Opts.Title, m.Seq
		reply(x, nil)
		h.a11yOpened(x, w) // a11y.go
		h.requestFrom(x, w)
		return
	}
	h.nextWin++
	x := &win{h: h, id: h.nextWin, seq: m.Seq, title: m.Opts.Title}
	if w == h.pending {
		x.next = w
	} else {
		x.owner = w
	}
	so, cv, err := corehook.NewCanvasSurface(h.app, surfaceOptions(m.Opts), x)
	if err != nil {
		reply(nil, err)
		return
	}
	x.surf, x.cv = so.(*tango.Surface), cv
	x.initA11y() // a11y.go
	h.wins[x.id] = x
	x.surf.OnClosed(func() { h.windowClosed(x) })
	h.watchSurface(x)
	reply(x, nil)
	h.requestFrom(x, w)
}

// match finds the window a worker's OpenWindow takes over: one of the
// active worker's (or orphaned) windows not yet taken by w, with the same
// title, else the same sequence number.
func (h *Host) match(w *worker, m devhost.OpenWindow) *win {
	free := func(x *win) bool {
		if x.closed || x.next == w || x.owner == w {
			return false
		}
		if w == h.pending {
			return x.next == nil && (x.owner == nil || x.owner == h.active || x.owner.gone)
		}
		return x.owner == nil || x.owner.gone
	}
	var bySeq *win
	for _, x := range h.wins {
		if !free(x) {
			continue
		}
		if x.title == m.Opts.Title {
			if bySeq == nil || x.seq == m.Seq || x.id < bySeq.id {
				return x
			}
		}
		if x.seq == m.Seq && bySeq == nil {
			bySeq = x
		}
	}
	return bySeq
}

func (h *Host) windowClosed(x *win) {
	if x.closed {
		return
	}
	x.closed = true
	x.closeA11y() // a11y.go
	delete(h.wins, x.id)
	delete(h.menus, x.id)
	for _, w := range []*worker{x.owner, x.next} {
		if w != nil {
			w.send(devhost.WindowClosed{Win: x.id})
		}
	}
	if x.held != nil {
		x.held.w.send(devhost.FrameAck{Win: x.id, Seg: x.held.f.Seg, Slot: x.held.f.Slot})
		x.held.w.dropSegs(x.id, -1)
		x.held = nil
	}
	for _, w := range []*worker{x.owner, x.next, h.active, h.pending} {
		if w != nil {
			if f, ok := w.take(x.id); ok {
				w.send(devhost.FrameAck{Win: x.id, Seg: f.Seg, Slot: f.Slot})
			}
			w.dropSegs(x.id, -1)
		}
	}
}

// requestFrom sends a FrameRequest for x to w unless one is outstanding.
func (h *Host) requestFrom(x *win, w *worker) {
	if w == nil || w.gone || x.closed {
		return
	}
	if x.outstanding == w && time.Since(x.requestedAt) < RequestTimeout {
		return
	}
	cw, ch := x.cv.Size()
	x.outstanding, x.requestedAt = w, time.Now()
	w.send(devhost.FrameRequest{Win: x.id, W: cw, H: ch, Scale: x.cv.Scale()})
}

// frameArrived runs on the main thread after a worker's FrameReady.
func (h *Host) frameArrived(w *worker, id int) {
	x := h.wins[id]
	if x == nil {
		if f, ok := w.take(id); ok {
			w.send(devhost.FrameAck{Win: id, Seg: f.Seg, Slot: f.Slot})
		}
		return
	}
	if x.outstanding == w {
		x.outstanding = nil
	}
	switch {
	case x.owner == w:
		h.presentInbox(x)
		if x.wantFrame {
			x.cv.Invalidate()
		}
	case x.next == w && h.pending == w:
		h.commit(w)
	}
}

// presentInbox shows the owner's newest frame, if any.
func (h *Host) presentInbox(x *win) {
	w := x.owner
	if w == nil {
		return
	}
	f, ok := w.take(x.id)
	if !ok {
		return
	}
	h.present(x, w, f)
}

func (h *Host) present(x *win, w *worker, f devhost.FrameReady) {
	seg := w.segment(x.id, f.Seg)
	if seg == nil {
		w.send(devhost.FrameAck{Win: x.id, Seg: f.Seg, Slot: f.Slot})
		return
	}
	img, err := devhost.FrameImage(seg, f)
	if err != nil {
		log.Printf("tango devhost: %v", err)
		w.send(devhost.FrameAck{Win: x.id, Seg: f.Seg, Slot: f.Slot})
		return
	}
	if h.banner.Kind != "" {
		h.ban.draw(img, f.Scale, h.banner)
	}
	h.flushA11y(x) // the frame's tree before the frame (a11y.go)
	t0 := time.Now()
	if err := x.cv.Present(img); err != nil && !x.closed {
		log.Printf("tango devhost: present: %v", err)
	}
	if traceOn {
		x.stats(f, time.Since(t0))
	}
	prev := x.held
	x.held = &heldFrame{w, f}
	if prev != nil {
		prev.w.send(devhost.FrameAck{Win: x.id, Seg: prev.f.Seg, Slot: prev.f.Slot})
		if prev.w != w {
			prev.w.dropSegs(x.id, -1)
		}
	}
	w.dropSegs(x.id, f.Seg)
}

// represent redraws the held frame (banner change).
func (h *Host) represent(x *win) {
	if x.held == nil {
		return
	}
	hf := *x.held
	seg := hf.w.segment(x.id, hf.f.Seg)
	if seg == nil {
		return
	}
	img, err := devhost.FrameImage(seg, hf.f)
	if err != nil {
		return
	}
	if h.banner.Kind != "" {
		h.ban.draw(img, hf.f.Scale, h.banner)
	}
	_ = x.cv.Present(img)
}

func (h *Host) setBanner(st devhost.Status) {
	if st == h.banner {
		return
	}
	h.banner = st
	for _, x := range h.wins {
		if st.Kind != "" {
			h.represent(x)
		}
		// Ask for a clean frame (banner gone) or one with the banner
		// drawn by present.
		if x.owner != nil && !x.owner.gone {
			x.wantFrame = true
			x.cv.Invalidate()
		}
	}
}

// ---- platform.CanvasSink ----

func (x *win) CanvasFrame() {
	h := x.h
	if x.closed || x.cv == nil {
		return
	}
	h.pollA11y(x) // a11y.go
	h.presentInbox(x)
	h.flushA11y(x)
	w := x.owner
	if w == nil || w.gone {
		return
	}
	cw, ch := x.cv.Size()
	sc := x.cv.Scale()
	dw, dh := devSize(cw, sc), devSize(ch, sc)
	stale := x.held == nil || x.held.f.W != dw || x.held.f.H != dh
	if x.wantFrame || stale {
		x.wantFrame = false
		h.requestFrom(x, w)
	}
	if !stale || ResizeWait <= 0 {
		return
	}
	// Live resize: wait briefly for a frame of the new size so the window
	// does not show stretched content.
	deadline := time.NewTimer(ResizeWait)
	defer deadline.Stop()
	for {
		if fw, fh, ok := w.peekSize(x.id); ok {
			h.presentInbox(x)
			if fw == dw && fh == dh {
				x.outstanding = nil
				return
			}
			h.requestFrom(x, w)
		}
		select {
		case <-w.arrive:
		case <-deadline.C:
			return
		}
	}
}

func devSize(v, scale float32) int {
	if scale <= 0 {
		scale = 1
	}
	return int(math.Ceil(float64(v * scale)))
}

func (x *win) CanvasInput(ev input.Event) { x.forward(devhost.Input{Win: x.id, Ev: ev}) }

func (x *win) CanvasFocus(focused bool) { x.forward(devhost.Focus{Win: x.id, Focused: focused}) }

func (x *win) CanvasResized(w, h, scale float32) {
	x.wantFrame = true
	if x.cv != nil {
		x.cv.Invalidate()
	}
}

func (x *win) forward(m any) {
	if x.closed {
		return
	}
	w := x.owner
	if w == nil || w.gone {
		if x.next != nil {
			x.h.queued = append(x.h.queued, queuedInput{x.id, m})
		}
		return
	}
	if w.frozen {
		x.h.queued = append(x.h.queued, queuedInput{x.id, m})
		return
	}
	w.send(m)
	x.wantFrame = true
	if x.cv != nil {
		x.cv.Invalidate()
	}
}

// surfaceOptions converts the wire options.
func surfaceOptions(o devhost.WindowOpts) tango.SurfaceOptions {
	return tango.SurfaceOptions{
		Title: o.Title, Width: o.Width, Height: o.Height,
		MinWidth: o.MinWidth, MinHeight: o.MinHeight, MaxWidth: o.MaxWidth, MaxHeight: o.MaxHeight,
		Resizable: o.Resizable, Hidden: o.Hidden, X: o.X, Y: o.Y, Centered: o.Centered,
		State: tango.WindowState(o.State), Kind: tango.SurfaceKind(o.Kind),
		Appearance: tango.Appearance{
			TitleBar: tango.TitleBar(o.TitleBar), Transparent: o.Transparent, Background: o.Background,
			Material: tango.Material(o.Material), AlwaysOnTop: o.AlwaysOnTop, Opacity: o.Opacity,
			NoShadow: o.NoShadow, NoClose: o.NoClose, NoMinimize: o.NoMinimize, NoMaximize: o.NoMaximize,
		},
	}
}
