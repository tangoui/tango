//go:build tango_dev

package uihost

// Hot reload under `tan dev` (docs/specs/ui-hotreload.md). tan dev starts
// the app binary twice with TANGO_DEV_ROLE set:
//
//   - "host": init below runs the dev host (internal/devhost/host) instead
//     of the app's main. It owns the windows and lives across rebuilds.
//   - "worker": the app runs normally, but Open asks the host for the
//     window and returns a remote ui.Window: frames go to the host through
//     shared memory, input comes back over a socket.
//
// Only built with -tags tango_dev (tan dev adds it); none of this is linked
// into production builds, the default (remote_release.go).

import (
	"errors"
	"fmt"
	"image"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devhost"
	devhostmain "github.com/tangoui/tango/internal/devhost/host"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/appctl"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// devState is the worker's connection and windows.
type devState struct {
	mu      sync.Mutex
	cl      *devhost.Client
	app     *tango.App
	wins    map[int]*remoteWin
	seq     int
	proxies map[*tango.Surface]bool
	hidden  bool
	early   map[int][]any           // host messages for windows not registered yet
	menus   map[int]func(id uint32) // SetMenu click routers by window (0: app menu)
	gen     int                     // connection generation: messages of older connections are dropped

	persist    devhost.Registry
	snap       []devhost.Entry
	snapLoaded bool
}

var dev devState

// devExit ends the worker when the host goes away (a variable for tests).
var devExit = func() { os.Exit(3) }

func init() {
	switch devhost.Role() {
	case devhost.RoleHost:
		os.Exit(devhostmain.Main())
	case devhost.RoleWorker:
		devTrace("init")
		if err := connectDev(os.Getenv(devhost.EnvAddr), os.Getenv(devhost.EnvDir)); err != nil {
			log.Printf("tango: hot reload: %v; running without the dev host", err)
		}
	}
}

var (
	devTraceOn = os.Getenv("TANGO_DEV_TRACE") != ""
	devStart   = time.Now()
)

// devTrace logs a worker milestone with TANGO_DEV_TRACE=1.
func devTrace(what string) {
	if devTraceOn {
		log.Printf("tango: hot reload trace: %s at %v", what, time.Since(devStart).Round(time.Millisecond))
	}
}

// connectDev connects to the host as a worker.
func connectDev(addr, dir string) error {
	// Window IDs restart with every host, so a late message of a previous
	// connection (tests reconnect in one process) must not reach the
	// windows, or the early queue, of this one.
	dev.mu.Lock()
	dev.gen++
	gen := dev.gen
	dev.mu.Unlock()
	cl, err := devhost.Connect(addr, dir, func(m any) {
		dev.mu.Lock()
		cur := dev.gen == gen
		dev.mu.Unlock()
		if cur {
			devHandle(m)
		}
	})
	if err != nil {
		return err
	}
	attachDev(cl)
	devTrace("connected")
	return nil
}

func attachDev(cl *devhost.Client) {
	dev.mu.Lock()
	dev.cl = cl
	dev.wins = map[int]*remoteWin{}
	dev.proxies = map[*tango.Surface]bool{}
	dev.menus = map[int]func(uint32){}
	dev.mu.Unlock()
	go func() {
		<-cl.Done()
		dev.mu.Lock()
		same, exit := dev.cl == cl, devExit
		dev.mu.Unlock()
		if same {
			exit()
		}
	}()
}

func devClient() *devhost.Client {
	dev.mu.Lock()
	defer dev.mu.Unlock()
	return dev.cl
}

func devWin(id int) *remoteWin {
	dev.mu.Lock()
	defer dev.mu.Unlock()
	return dev.wins[id]
}

// devPost runs fn on the app's main thread.
func devPost(fn func()) {
	dev.mu.Lock()
	app := dev.app
	dev.mu.Unlock()
	if app != nil {
		app.Post(fn)
	}
}

// msgWin returns the window a host message is for.
func msgWin(m any) (int, bool) {
	switch m := m.(type) {
	case devhost.Input:
		return m.Win, true
	case devhost.Focus:
		return m.Win, true
	case devhost.FrameRequest:
		return m.Win, true
	case devhost.WindowClosed:
		return m.Win, true
	case devhost.SurfaceEvent:
		return m.Win, true
	case devhost.CloseRequest:
		return m.Win, true
	case devhost.A11yState:
		return m.Win, true
	case devhost.A11yAction:
		return m.Win, true
	}
	return 0, false
}

// devHandle handles host messages on the client's reading goroutine.
// Messages for a window Open has not registered yet (the host may send
// them right after Opened) wait in dev.early.
func devHandle(m any) {
	if id, ok := msgWin(m); ok {
		dev.mu.Lock()
		if dev.wins[id] == nil {
			if dev.early == nil {
				dev.early = map[int][]any{}
			}
			if len(dev.early[id]) < 256 {
				dev.early[id] = append(dev.early[id], m)
			}
			dev.mu.Unlock()
			return
		}
		dev.mu.Unlock()
	}
	switch m := m.(type) {
	case devhost.Input:
		if w := devWin(m.Win); w != nil {
			devPost(func() { w.input(m.Ev) })
		}
	case devhost.Focus:
		if w := devWin(m.Win); w != nil {
			devPost(func() { w.input(input.WindowFocus{Focused: m.Focused}) })
		}
	case devhost.FrameRequest:
		if w := devWin(m.Win); w != nil {
			w.request(m)
		}
	case devhost.FrameAck:
		if w := devWin(m.Win); w != nil {
			w.fw.Ack(m.Seg, m.Slot)
		}
	case devhost.WindowClosed:
		if w := devWin(m.Win); w != nil {
			devPost(w.closedByHost)
		}
	case devhost.SurfaceEvent:
		if w := devWin(m.Win); w != nil {
			devPost(func() { w.surfaceEvent(m) })
		}
	case devhost.CloseRequest:
		if w := devWin(m.Win); w != nil {
			devPost(w.closeRequested)
		}
	case devhost.A11yState: // remote_a11y_dev.go
		if w := devWin(m.Win); w != nil {
			devPost(func() { w.a11yState(m) })
		}
	case devhost.A11yAction:
		if w := devWin(m.Win); w != nil {
			devPost(func() { w.a11yAction(m) })
		}
	case devhost.MenuClick:
		devPost(func() {
			dev.mu.Lock()
			fn := dev.menus[m.Win]
			dev.mu.Unlock()
			if fn != nil {
				fn(m.ID)
			}
		})
	case devhost.SnapshotReq:
		encode := func() {
			entries, err := dev.persist.Encode()
			if err != nil {
				log.Printf("tango: hot reload: state not saved: %v", err)
			}
			if cl := devClient(); cl != nil {
				_ = cl.Send(devhost.SnapshotReply{Req: m.Req, Entries: entries})
			}
		}
		dev.mu.Lock()
		app := dev.app
		dev.mu.Unlock()
		if app != nil {
			app.Post(encode) // the state is the UI thread's
		} else {
			encode()
		}
	}
}

// devOpen is Open in a worker: the window lives in the host. ok is false
// outside tan dev.
func devOpen(app *tango.App, opts Options, page func(*ui.Context), ropts ui.RunnerOptions) (r *ui.Runner, s *tango.Surface, ok bool, err error) {
	cl := devClient()
	if cl == nil {
		return nil, nil, false, nil
	}
	devTrace("open")
	dev.mu.Lock()
	if dev.app == nil {
		dev.app = app
	}
	seq := dev.seq
	dev.seq++
	dev.mu.Unlock()

	m, err := cl.Request(func(id int) any {
		return devhost.OpenWindow{Req: id, Seq: seq, Opts: windowOpts(opts.Surface)}
	}, 10*time.Second)
	if err != nil {
		return nil, nil, true, fmt.Errorf("uihost: dev host: %w", err)
	}
	o := m.(devhost.Opened)
	if o.Err != "" {
		return nil, nil, true, fmt.Errorf("uihost: dev host: %s", o.Err)
	}
	r = ui.NewRunner(page, ropts)
	w := &remoteWin{cl: cl, id: o.Win, r: r, size: geom.Size{W: o.W, H: o.H}, scale: o.Scale}
	w.fw = devhost.NewFrameWriter(o.Win, cl.Dir, cl.Send)
	r.Attach(w)

	// The app gets an ordinary surface standing in for the remote window,
	// so OnClosed, QuitOnLastClose and the like keep working; its calls and
	// events go to and come from the host window (remote_surface_dev.go).
	s = proxySurface(app, opts.Surface, w)
	w.proxy = s
	w.sc = newShortcuts(app, s) // shortcuts.go
	dev.mu.Lock()
	dev.wins[o.Win] = w
	if s != nil {
		dev.proxies[s] = true
	}
	early := dev.early[o.Win]
	delete(dev.early, o.Win)
	dev.mu.Unlock()
	for _, m := range early {
		devHandle(m)
	}
	if s != nil {
		s.OnClosed(w.closedLocally)
	}
	hideWorker(app)
	devTrace("opened")
	w.Invalidate()
	return r, s, true, nil
}

// proxySurface creates the stand-in surface for w, whose native part
// forwards to the host window; nil if that fails.
func proxySurface(app *tango.App, o tango.SurfaceOptions, w *remoteWin) *tango.Surface {
	if corehook.NewProxySurface == nil {
		return nil
	}
	so, err := corehook.NewProxySurface(app, o, func(_ platform.SurfaceID, ev corehook.ProxyEvents) (platform.Surface, error) {
		w.ns = &remoteSurface{w: w, ev: ev}
		return w.ns, nil
	})
	if err != nil {
		log.Printf("tango: hot reload: stand-in surface: %v", err)
		return nil
	}
	return so.(*tango.Surface)
}

// hideWorker keeps the worker out of the Dock and gives activation back to
// the app that had it (the editor, or the host's window) when the worker
// has no windows of its own. macOS only; elsewhere these fail quietly.
func hideWorker(app *tango.App) {
	dev.mu.Lock()
	if dev.hidden {
		dev.mu.Unlock()
		return
	}
	dev.hidden = true
	own := 0
	for _, s := range app.Surfaces() {
		if !dev.proxies[s] {
			own++
		}
	}
	dev.mu.Unlock()
	_ = appctl.SetDockVisible(app, false)
	if own == 0 {
		_ = appctl.Hide(app)
	}
}

func windowOpts(o tango.SurfaceOptions) devhost.WindowOpts {
	return devhost.WindowOpts{
		Title: o.Title, Width: o.Width, Height: o.Height,
		MinWidth: o.MinWidth, MinHeight: o.MinHeight, MaxWidth: o.MaxWidth, MaxHeight: o.MaxHeight,
		Resizable: o.Resizable, Hidden: o.Hidden, X: o.X, Y: o.Y, Centered: o.Centered,
		State: int(o.State), Kind: int(o.Kind),
		TitleBar: int(o.Appearance.TitleBar), Transparent: o.Appearance.Transparent,
		Background: o.Appearance.Background, Material: int(o.Appearance.Material),
		AlwaysOnTop: o.Appearance.AlwaysOnTop, Opacity: o.Appearance.Opacity,
		NoShadow: o.Appearance.NoShadow, NoClose: o.Appearance.NoClose,
		NoMinimize: o.Appearance.NoMinimize, NoMaximize: o.Appearance.NoMaximize,
	}
}

// remoteWin is a ui.Window shown by the host.
type remoteWin struct {
	cl    *devhost.Client
	id    int
	r     *ui.Runner
	fw    *devhost.FrameWriter
	proxy *tango.Surface
	ns    *remoteSurface // proxy's native part
	sc    *shortcuts     // window shortcuts (shortcuts.go)

	// Main thread.
	size    geom.Size
	scale   float32
	closed  bool
	lastErr string
	cursor  input.Cursor
	curSet  bool
	textOn  bool
	caret   geom.Rect
	textSet bool
	ax      remoteA11y // accessibility (remote_a11y_dev.go)

	traced bool

	invalidSent atomic.Bool // Invalidate sent, no FrameRequest since

	reqMu   sync.Mutex
	req     devhost.FrameRequest
	reqPost bool // a frame is posted to the main thread

	wakeMu sync.Mutex
	wakeAt time.Time
	wake   *time.Timer
}

func (w *remoteWin) Size() geom.Size { return w.size }
func (w *remoteWin) Scale() float32  { return w.scale }

// Invalidate asks the host for a FrameRequest. Any goroutine.
func (w *remoteWin) Invalidate() {
	if !w.invalidSent.Swap(true) {
		_ = w.cl.Send(devhost.Invalidate{Win: w.id})
	}
}

// InvalidateAt invalidates at t. Any goroutine.
func (w *remoteWin) InvalidateAt(t time.Time) {
	w.wakeMu.Lock()
	defer w.wakeMu.Unlock()
	if !w.wakeAt.IsZero() && !t.Before(w.wakeAt) {
		return
	}
	if w.wake != nil {
		w.wake.Stop()
	}
	w.wakeAt = t
	w.wake = time.AfterFunc(time.Until(t), func() {
		w.wakeMu.Lock()
		w.wakeAt = time.Time{}
		w.wakeMu.Unlock()
		w.Invalidate()
	})
}

func (w *remoteWin) SetCursor(c input.Cursor) {
	if w.curSet && c == w.cursor {
		return
	}
	w.cursor, w.curSet = c, true
	_ = w.cl.Send(devhost.SetCursor{Win: w.id, Cursor: c})
}

func (w *remoteWin) SetTextInput(on bool, caret geom.Rect) {
	w.sc.setText(on)
	if w.textSet && on == w.textOn && caret == w.caret {
		return
	}
	w.textOn, w.caret, w.textSet = on, caret, true
	_ = w.cl.Send(devhost.SetTextInput{Win: w.id, Enabled: on, Caret: caret})
}

func (w *remoteWin) ReadClipboard() string {
	m, err := w.cl.Request(func(id int) any { return devhost.ReadClipboard{Req: id} }, 2*time.Second)
	if err != nil {
		return ""
	}
	return m.(devhost.ClipboardText).Text
}

func (w *remoteWin) WriteClipboard(s string) { _ = w.cl.Send(devhost.WriteClipboard{Text: s}) }

func (w *remoteWin) Present(img *image.RGBA) error {
	if img == nil {
		return errors.New("uihost: remote window needs a CPU frame")
	}
	return w.fw.Write(img, w.scale)
}

// request records a FrameRequest and posts one frame. Reading goroutine.
func (w *remoteWin) request(m devhost.FrameRequest) {
	w.reqMu.Lock()
	w.req = m
	post := !w.reqPost
	w.reqPost = true
	w.reqMu.Unlock()
	if post {
		devPost(w.frame)
	}
}

// frame renders at the latest requested geometry. Main thread.
func (w *remoteWin) frame() {
	w.reqMu.Lock()
	req := w.req
	w.reqPost = false
	w.reqMu.Unlock()
	w.invalidSent.Store(false)
	if w.closed {
		return
	}
	if !w.traced {
		w.traced = true
		devTrace("first frame")
	}
	w.size, w.scale = geom.Size{W: req.W, H: req.H}, req.Scale
	if err := w.r.Frame(); err != nil {
		if msg := err.Error(); msg != w.lastErr {
			w.lastErr = msg
			log.Printf("tango: uihost: remote window %d: frame: %v", w.id, err)
		}
	} else {
		w.lastErr = ""
	}
	if w.r.Animating() {
		w.Invalidate()
	}
	if t := w.r.NextWake(); !t.IsZero() {
		w.InvalidateAt(t)
	}
}

func (w *remoteWin) input(ev input.Event) {
	if w.closed {
		return
	}
	if devTraceOn {
		if p, ok := ev.(input.Pointer); !ok || p.Kind != input.Move {
			devTrace(fmt.Sprintf("input %T%+v", ev, ev))
		}
	}
	if w.sc.handle(ev) { // a window shortcut consumed its key (shortcuts.go)
		w.Invalidate()
		return
	}
	w.r.HandleEvent(ev)
}

// closedByHost: the host window is gone. Main thread.
func (w *remoteWin) closedByHost() {
	if w.closed {
		return
	}
	if w.ns == nil || w.proxy == nil {
		w.shutdown()
		if w.proxy != nil {
			w.proxy.Close()
		}
		return
	}
	w.ns.hostGone = true
	w.proxy.Close() // → SurfaceClosed → closedLocally
}

// closedLocally: the proxy surface closed (its native Close told the
// host). Main thread.
func (w *remoteWin) closedLocally() {
	if w.closed {
		return
	}
	w.shutdown()
	if w.ns == nil {
		_ = w.cl.Send(devhost.CloseWindow{Win: w.id})
	}
}

func (w *remoteWin) shutdown() {
	w.closed = true
	w.r.Close()
	w.fw.Close()
	dev.mu.Lock()
	delete(dev.wins, w.id)
	delete(dev.menus, w.id)
	if w.proxy != nil {
		delete(dev.proxies, w.proxy)
	}
	dev.mu.Unlock()
}

// Persist keeps the value ptr points to across hot reloads under tan dev:
// before a rebuilt app replaces the running one, the value is gob-encoded,
// and Persist in the new process decodes it back into ptr. Call it early
// (before the first frame), once per value, on the main goroutine; ptr
// must point to a gob-encodable value (exported fields). Values are
// matched by type name and call order. When the type changed so that the
// old state no longer decodes, it is discarded with a message on stderr.
//
// Outside tan dev, and in production builds (without -tags tango_dev),
// Persist does nothing.
func Persist(ptr any) {
	cl := devClient()
	if cl == nil {
		return
	}
	key, typ, err := dev.persist.Add(ptr)
	if err != nil {
		log.Printf("tango: hot reload: %v", err)
		return
	}
	dev.mu.Lock()
	loaded := dev.snapLoaded
	dev.mu.Unlock()
	if !loaded {
		var entries []devhost.Entry
		m, err := cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, 5*time.Second)
		if err != nil {
			log.Printf("tango: hot reload: no saved state: %v", err)
		} else {
			entries = m.(devhost.SnapshotData).Entries
		}
		dev.mu.Lock()
		dev.snap, dev.snapLoaded = entries, true
		dev.mu.Unlock()
		devTrace("snapshot")
	}
	dev.mu.Lock()
	snap := dev.snap
	dev.mu.Unlock()
	found, err := devhost.Decode(snap, key, ptr)
	switch {
	case err != nil:
		log.Printf("tango: hot reload: discarded the saved %s (its type changed): %v", typ, err)
	case found:
		log.Printf("tango: hot reload: restored %s", typ)
	}
}

func devRegistry() (r devhost.Registry) { return }
