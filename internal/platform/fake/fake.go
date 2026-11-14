// Package fake is a headless platform.Backend for testing Tango's core.
//
// Its event loop runs on the goroutine that calls Run, which is treated as
// the main thread. "Native events" (page messages, close requests, dialog
// answers, menu clicks, ...) are queued with the helper methods and
// processed by Run or Step on that goroutine, like a real toolkit's run
// loop would.
//
// Design adapted from mygo's internal/fake (github.com/egoist/mygo v0.9.0,
// internal/fake/fake.go), Copyright (c) 2026 MyGo contributors, MIT
// License: channel-driven Run/Step loop and goroutine-id main-thread
// tracking.
package fake

import (
	"bytes"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/platform"
)

// DefaultOrigin is the AssetOrigin of a Backend whose Origin is empty.
const DefaultOrigin = "tango://app"

// Backend is a fake platform backend. It implements platform.Backend and
// the optional capabilities platform.Clipboard, platform.Dialogs and
// platform.AppMenu. Use Minimal for a backend without capabilities.
//
// Exported configuration fields must be set before Run.
type Backend struct {
	// Origin is what AssetOrigin returns; DefaultOrigin when empty.
	Origin string
	// AutoLoad makes surfaces emit SurfaceLoaded (as a queued native event)
	// after creation with a URL and after every Navigate.
	AutoLoad bool
	// InitErr is returned by Init; NewSurfaceErr by NewSurface.
	InitErr       error
	NewSurfaceErr error

	// Dialog answers. They run on the main thread; their result is delivered
	// to done as a later native event (dialogs are asynchronous). Nil
	// answers return (0, nil), (nil, platform.ErrCanceled) and
	// ("", platform.ErrCanceled).
	OnMessageDialog func(parent platform.SurfaceID, opts platform.MessageOptions) (int, error)
	OnOpenDialog    func(parent platform.SurfaceID, opts platform.FileOptions) ([]string, error)
	OnSaveDialog    func(parent platform.SurfaceID, opts platform.FileOptions) (string, error)

	// ClipboardErr, when set, fails clipboard reads and writes.
	ClipboardErr error

	// NoNestedLoop makes the backend report, through platform.NestedLoop,
	// that Step cannot process native events from inside a callback (as
	// on Android).
	NoNestedLoop bool

	h      platform.Host
	mainG  atomic.Uint64
	wake   chan struct{}
	notify chan struct{}
	quit   chan struct{}
	done   chan struct{} // closed when Run returns
	qonce  sync.Once

	wakes  atomic.Int64
	drains atomic.Int64
	steps  atomic.Int64

	mu       sync.Mutex
	events   []func()
	surfaces []*Surface
	quits    int
	clip     string
	menus    map[platform.SurfaceID]menuState
	recent   []string // NoteRecentDocument; see appevents.go
}

type menuState struct {
	items   []platform.MenuItem
	onClick func(uint32)
}

// New returns a fake backend.
func New() *Backend {
	return &Backend{
		wake:   make(chan struct{}, 1),
		notify: make(chan struct{}, 1),
		quit:   make(chan struct{}),
		done:   make(chan struct{}),
		menus:  map[platform.SurfaceID]menuState{},
	}
}

type minimal struct{ platform.Backend }

// Minimal returns b seen through the bare platform.Backend interface: the
// core finds none of the optional capabilities on it. b's helpers keep
// working.
func (b *Backend) Minimal() platform.Backend { return minimal{b} }

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(string(s), 10, 64)
	return id
}

// IsMainThread reports whether the caller runs on the goroutine running Run.
func (b *Backend) IsMainThread() bool {
	m := b.mainG.Load()
	return m != 0 && goroutineID() == m
}

func (b *Backend) mustMain(what string) {
	if m := b.mainG.Load(); m != 0 && goroutineID() != m {
		panic("fake: " + what + " called off the main thread")
	}
}

// ---- platform.Backend ----

func (b *Backend) Init(h platform.Host, _ platform.AppOptions) error {
	if b.InitErr != nil {
		return b.InitErr
	}
	b.h = h
	b.mainG.Store(goroutineID())
	return nil
}

func (b *Backend) Run() error {
	b.mainG.Store(goroutineID())
	defer close(b.done)
	b.h.OnLifecycle(platform.LifecycleStarted)
	for !b.quitting() {
		select {
		case <-b.quit:
		case <-b.wake:
			b.drain()
		case <-b.notify:
			b.runEvents()
		}
	}
	b.h.OnLifecycle(platform.LifecycleTerminating)
	return nil
}

func (b *Backend) Quit() {
	b.mustMain("Quit")
	b.mu.Lock()
	b.quits++
	b.mu.Unlock()
	b.qonce.Do(func() { close(b.quit) })
}

func (b *Backend) Wake() {
	b.wakes.Add(1)
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// NestedLoopSupported implements platform.NestedLoop; see NoNestedLoop.
func (b *Backend) NestedLoopSupported() bool { return !b.NoNestedLoop }

func (b *Backend) Step(block bool) bool {
	b.mustMain("Step")
	b.steps.Add(1)
	if b.quitting() {
		return false
	}
	if block {
		select {
		case <-b.quit:
		case <-b.wake:
			b.drain()
		case <-b.notify:
			b.runEvents()
		}
	} else {
		select {
		case <-b.wake:
			b.drain()
		case <-b.notify:
			b.runEvents()
		default:
		}
	}
	return !b.quitting()
}

func (b *Backend) AssetOrigin() string {
	if b.Origin == "" {
		return DefaultOrigin
	}
	return b.Origin
}

func (b *Backend) NewSurface(id platform.SurfaceID, opts platform.SurfaceOptions) (platform.Surface, error) {
	b.mustMain("NewSurface")
	if b.NewSurfaceErr != nil {
		return nil, b.NewSurfaceErr
	}
	w, h := opts.Width, opts.Height
	if w == 0 {
		w = 1024
	}
	if h == 0 {
		h = 768
	}
	s := &Surface{b: b, id: id, opts: opts, title: opts.Title, w: w, h: h, visible: opts.Visible, url: opts.URL}
	if opts.URL != "" {
		s.navs = []string{opts.URL}
	}
	b.mu.Lock()
	b.surfaces = append(b.surfaces, s)
	b.mu.Unlock()
	if b.AutoLoad && opts.URL != "" {
		b.enqueue(func() { b.emitLoaded(s, opts.URL) })
	}
	return s, nil
}

func (b *Backend) quitting() bool {
	select {
	case <-b.quit:
		return true
	default:
		return false
	}
}

func (b *Backend) drain() {
	b.drains.Add(1)
	b.h.Drain()
}

func (b *Backend) enqueue(fn func()) {
	b.mu.Lock()
	b.events = append(b.events, fn)
	b.mu.Unlock()
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// runEvents runs queued native events one at a time; an event that pumps a
// nested loop (Step) runs the ones behind it, so re-notify while some wait.
func (b *Backend) runEvents() {
	for {
		b.mu.Lock()
		if len(b.events) == 0 {
			b.mu.Unlock()
			return
		}
		fn := b.events[0]
		b.events[0] = nil
		b.events = b.events[1:]
		more := len(b.events) > 0
		b.mu.Unlock()
		if more {
			select {
			case b.notify <- struct{}{}:
			default:
			}
		}
		fn()
	}
}

// ---- test helpers (any goroutine) ----

// Do runs fn on the main thread as a native event and waits for it. On the
// main thread it runs fn directly. It reports false if the loop stopped
// before fn ran.
func (b *Backend) Do(fn func()) bool {
	if b.IsMainThread() {
		fn()
		return true
	}
	// state: 0 queued, 1 started (by the loop), 2 abandoned (by Do).
	var state atomic.Int32
	ran := make(chan struct{})
	b.enqueue(func() {
		if !state.CompareAndSwap(0, 1) {
			return
		}
		fn()
		close(ran)
	})
	select {
	case <-ran:
		return true
	case <-b.quit:
	case <-b.done:
	}
	// fn may itself have made the loop stop (closing the last surface quits
	// the app on Windows and Linux): once it started, wait for it to finish
	// rather than return while it still writes the caller's variables.
	if state.CompareAndSwap(0, 2) {
		return false
	}
	<-ran
	return true
}

// Done is closed when Run returns.
func (b *Backend) Done() <-chan struct{} { return b.done }

// Quits counts Quit calls; Wakes, Drains and Steps count those calls.
func (b *Backend) Quits() int    { b.mu.Lock(); defer b.mu.Unlock(); return b.quits }
func (b *Backend) Wakes() int64  { return b.wakes.Load() }
func (b *Backend) Drains() int64 { return b.drains.Load() }
func (b *Backend) Steps() int64  { return b.steps.Load() }

// Surfaces returns every surface created, in creation order (closed ones
// included).
func (b *Backend) Surfaces() []*Surface {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.surfaces)
}

// Surface returns the surface with the given id, or nil.
func (b *Backend) Surface(id platform.SurfaceID) *Surface {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.surfaces {
		if s.id == id {
			return s
		}
	}
	return nil
}

// Message simulates the page of surface id calling
// window.__tangoNative.post(msg) from AssetOrigin.
func (b *Backend) Message(id platform.SurfaceID, msg string) bool {
	return b.MessageFrom(id, b.AssetOrigin(), msg)
}

// MessageFrom is Message with an explicit page origin.
func (b *Backend) MessageFrom(id platform.SurfaceID, origin, msg string) bool {
	return b.Do(func() { b.h.OnMessage(id, origin, msg) })
}

// Load simulates the surface's current page finishing loading.
func (b *Backend) Load(id platform.SurfaceID) bool {
	return b.Do(func() {
		if s := b.Surface(id); s != nil {
			b.emitLoaded(s, s.URL())
		}
	})
}

// LoadURL simulates the page navigating itself to url and finishing loading.
func (b *Backend) LoadURL(id platform.SurfaceID, url string) bool {
	return b.Do(func() {
		if s := b.Surface(id); s != nil {
			s.mu.Lock()
			s.url = url
			s.mu.Unlock()
			b.emitLoaded(s, url)
		}
	})
}

func (b *Backend) emitLoaded(s *Surface, url string) {
	if s.Closed() {
		return
	}
	b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoaded, URL: url})
}

// RequestClose simulates the user clicking the close button: it asks
// Host.ShouldClose and closes the surface if allowed. It reports whether
// the surface closed.
func (b *Backend) RequestClose(id platform.SurfaceID) bool {
	closed := false
	b.Do(func() {
		s := b.Surface(id)
		if s == nil || s.Closed() {
			return
		}
		if b.h.ShouldClose(id) {
			s.Close()
			closed = true
		}
	})
	return closed
}

// Resize simulates the user resizing the surface.
func (b *Backend) Resize(id platform.SurfaceID, w, h int) bool {
	return b.Do(func() {
		if s := b.Surface(id); s != nil {
			s.mu.Lock()
			s.w, s.h = w, h
			s.mu.Unlock()
			b.h.OnSurfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: w, Height: h})
		}
	})
}

// FocusChange simulates the surface's window gaining (focused) or losing
// keyboard focus.
func (b *Backend) FocusChange(id platform.SurfaceID, focused bool) bool {
	kind := platform.SurfaceBlurred
	if focused {
		kind = platform.SurfaceFocused
	}
	return b.Do(func() {
		if s := b.Surface(id); s != nil && !s.Closed() {
			b.h.OnSurfaceEvent(id, platform.SurfaceEvent{Kind: kind})
		}
	})
}

// Lifecycle simulates the platform emitting ev.
func (b *Backend) Lifecycle(ev platform.LifecycleEvent) bool {
	return b.Do(func() { b.h.OnLifecycle(ev) })
}

// ---- platform.Clipboard ----

func (b *Backend) ClipboardReadText() (string, error) {
	b.mustMain("ClipboardReadText")
	if b.ClipboardErr != nil {
		return "", b.ClipboardErr
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.clip, nil
}

func (b *Backend) ClipboardWriteText(text string) error {
	b.mustMain("ClipboardWriteText")
	if b.ClipboardErr != nil {
		return b.ClipboardErr
	}
	b.mu.Lock()
	b.clip = text
	b.mu.Unlock()
	return nil
}

// ClipboardText returns the fake clipboard's content.
func (b *Backend) ClipboardText() string { b.mu.Lock(); defer b.mu.Unlock(); return b.clip }

// ---- platform.Dialogs ----

func (b *Backend) MessageDialog(parent platform.SurfaceID, opts platform.MessageOptions, done func(int, error)) {
	b.mustMain("MessageDialog")
	n, err := 0, error(nil)
	if b.OnMessageDialog != nil {
		n, err = b.OnMessageDialog(parent, opts)
	}
	b.enqueue(func() { done(n, err) })
}

func (b *Backend) OpenFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func([]string, error)) {
	b.mustMain("OpenFileDialog")
	var paths []string
	err := platform.ErrCanceled
	if b.OnOpenDialog != nil {
		paths, err = b.OnOpenDialog(parent, opts)
	}
	b.enqueue(func() { done(paths, err) })
}

func (b *Backend) SaveFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(string, error)) {
	b.mustMain("SaveFileDialog")
	path := ""
	err := platform.ErrCanceled
	if b.OnSaveDialog != nil {
		path, err = b.OnSaveDialog(parent, opts)
	}
	b.enqueue(func() { done(path, err) })
}

// ---- platform.AppMenu ----

func (b *Backend) SetAppMenu(parent platform.SurfaceID, items []platform.MenuItem, onClick func(uint32)) {
	b.mustMain("SetAppMenu")
	b.mu.Lock()
	b.menus[parent] = menuState{items: items, onClick: onClick}
	b.mu.Unlock()
}

// Menu returns the items last set for parent (0 is the app menu).
func (b *Backend) Menu(parent platform.SurfaceID) []platform.MenuItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.menus[parent].items
}

// ClickMenu simulates the user choosing item id of parent's menu. It
// reports false if no menu is set for parent.
func (b *Backend) ClickMenu(parent platform.SurfaceID, id uint32) bool {
	b.mu.Lock()
	m, ok := b.menus[parent]
	b.mu.Unlock()
	if !ok || m.onClick == nil {
		return false
	}
	return b.Do(func() { m.onClick(id) })
}

// ---- Surface ----

// Surface is a fake platform.Surface. Its accessors are safe from any
// goroutine.
type Surface struct {
	b    *Backend
	id   platform.SurfaceID
	opts platform.SurfaceOptions

	mu       sync.Mutex
	title    string
	w, h     int
	visible  bool
	closed   bool
	url      string
	navs     []string
	evals    []string
	focuses  int
	devtools int
	laneCaps []string // SetLaneCaps
	lanePkts [][]byte // SendLanePacket
}

func (s *Surface) Show() { s.b.mustMain("Show"); s.mu.Lock(); s.visible = true; s.mu.Unlock() }
func (s *Surface) Hide() { s.b.mustMain("Hide"); s.mu.Lock(); s.visible = false; s.mu.Unlock() }

// Close destroys the surface and emits SurfaceClosed synchronously, the
// harder of the two orders a real backend may use.
func (s *Surface) Close() {
	s.b.mustMain("Close")
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed, s.visible = true, false
	s.mu.Unlock()
	s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

func (s *Surface) SetTitle(t string) {
	s.b.mustMain("SetTitle")
	s.mu.Lock()
	s.title = t
	s.mu.Unlock()
}
func (s *Surface) SetSize(w, h int) {
	s.b.mustMain("SetSize")
	s.mu.Lock()
	s.w, s.h = w, h
	s.mu.Unlock()
}
func (s *Surface) Size() (int, int) {
	s.b.mustMain("Size")
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w, s.h
}
func (s *Surface) Focus() { s.b.mustMain("Focus"); s.mu.Lock(); s.focuses++; s.mu.Unlock() }

func (s *Surface) Navigate(url string) {
	s.b.mustMain("Navigate")
	s.mu.Lock()
	s.url = url
	s.navs = append(s.navs, url)
	s.mu.Unlock()
	if s.b.AutoLoad {
		s.b.enqueue(func() { s.b.emitLoaded(s, url) })
	}
}

func (s *Surface) Eval(js string) {
	s.b.mustMain("Eval")
	s.mu.Lock()
	s.evals = append(s.evals, js)
	s.mu.Unlock()
}

func (s *Surface) OpenDevTools() {
	s.b.mustMain("OpenDevTools")
	s.mu.Lock()
	s.devtools++
	s.mu.Unlock()
}
func (s *Surface) Native() uintptr { return uintptr(0xD0E7_0000) + uintptr(s.id) }

// ID returns the core-allocated id.
func (s *Surface) ID() platform.SurfaceID { return s.id }

// Options returns the options the surface was created with.
func (s *Surface) Options() platform.SurfaceOptions { return s.opts }

func (s *Surface) Title() string { s.mu.Lock(); defer s.mu.Unlock(); return s.title }
func (s *Surface) Visible() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.visible }
func (s *Surface) Closed() bool  { s.mu.Lock(); defer s.mu.Unlock(); return s.closed }
func (s *Surface) URL() string   { s.mu.Lock(); defer s.mu.Unlock(); return s.url }
func (s *Surface) Focuses() int  { s.mu.Lock(); defer s.mu.Unlock(); return s.focuses }

// DevToolsOpened returns how often OpenDevTools was called.
func (s *Surface) DevToolsOpened() int { s.mu.Lock(); defer s.mu.Unlock(); return s.devtools }
func (s *Surface) Navigations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.navs)
}

// Evals returns every script passed to Eval, in order.
func (s *Surface) Evals() []string { s.mu.Lock(); defer s.mu.Unlock(); return slices.Clone(s.evals) }

// ---- IPC fast lanes (platform.BinaryLane, platform.LaneHost) ----

var _ platform.BinaryLane = (*Surface)(nil)

// SetLaneCaps sets what surface id's LaneCaps reports. Any goroutine.
func (b *Backend) SetLaneCaps(id platform.SurfaceID, caps []string) {
	if s := b.Surface(id); s != nil {
		s.mu.Lock()
		s.laneCaps = slices.Clone(caps)
		s.mu.Unlock()
	}
}

// LanePackets returns copies of the packets the core sent on surface id's
// platform lane (SendLanePacket), in order.
func (b *Backend) LanePackets(id platform.SurfaceID) [][]byte {
	s := b.Surface(id)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.lanePkts))
	for i, p := range s.lanePkts {
		out[i] = slices.Clone(p)
	}
	return out
}

// LaneUp simulates surface id's page at origin sending p on a platform
// upstream lane: it calls the host's OnLanePacket on the calling goroutine,
// as a backend's lane thread would. It does nothing if the host is not a
// platform.LaneHost.
func (b *Backend) LaneUp(id platform.SurfaceID, origin string, p []byte) {
	if lh, ok := b.h.(platform.LaneHost); ok {
		lh.OnLanePacket(id, origin, p)
	}
}

// LaneCaps implements platform.BinaryLane (any goroutine).
func (s *Surface) LaneCaps() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.laneCaps)
}

// SendLanePacket implements platform.BinaryLane (any goroutine): it records
// p, refusing it once the surface is closed.
func (s *Surface) SendLanePacket(p []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.lanePkts = append(s.lanePkts, p)
	return true
}
