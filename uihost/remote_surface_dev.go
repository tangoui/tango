//go:build tango_dev

package uihost

// The worker's stand-in tango.Surface for a remote window: its native part
// (remoteSurface) forwards the Surface calls to the host window and the
// host window's events come back through it, so SetTitle, SetSize, Show,
// OnResized, OnClose and the rest act on the window the user sees. Menus
// set with sys/menu go to the host the same way.

import (
	"log"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
)

func init() {
	corehook.AppMenuProxy = devAppMenu
}

// surfaceInfoTimeout bounds waiting for the host in a Surface getter.
var surfaceInfoTimeout = 2 * time.Second

// remoteSurface is the platform.Surface of a stand-in surface. Its methods
// run on the main thread.
type remoteSurface struct {
	w        *remoteWin
	ev       corehook.ProxyEvents
	closed   bool
	hostGone bool // the host window is already closed
}

var (
	_ platform.Surface       = (*remoteSurface)(nil)
	_ platform.Geometry      = (*remoteSurface)(nil)
	_ platform.Stateful      = (*remoteSurface)(nil)
	_ platform.Styled        = (*remoteSurface)(nil)
	_ platform.SurfaceStatus = (*remoteSurface)(nil)
)

// op sends a call that needs no answer. Calls and requests share the
// connection, so a getter after a setter sees its effect.
func (n *remoteSurface) op(m devhost.SurfaceOp) {
	if n.closed {
		return
	}
	m.Win = n.w.id
	_ = n.w.cl.Send(m)
}

// info asks the host for the window's state.
func (n *remoteSurface) info() devhost.SurfaceInfo {
	if n.closed {
		return devhost.SurfaceInfo{}
	}
	m, err := n.w.cl.Request(func(id int) any {
		return devhost.SurfaceOp{Req: id, Win: n.w.id, Op: devhost.OpGet}
	}, surfaceInfoTimeout)
	if err != nil {
		log.Printf("tango: hot reload: window state: %v", err)
		return devhost.SurfaceInfo{}
	}
	return m.(devhost.SurfaceInfo)
}

func (n *remoteSurface) Show()  { n.op(devhost.SurfaceOp{Op: devhost.OpShow}) }
func (n *remoteSurface) Hide()  { n.op(devhost.SurfaceOp{Op: devhost.OpHide}) }
func (n *remoteSurface) Focus() { n.op(devhost.SurfaceOp{Op: devhost.OpFocus}) }

func (n *remoteSurface) SetTitle(t string) {
	n.op(devhost.SurfaceOp{Op: devhost.OpSetTitle, Title: t})
}

func (n *remoteSurface) SetSize(w, h int) {
	n.op(devhost.SurfaceOp{Op: devhost.OpSetSize, Rect: devhost.Rect{W: w, H: h}})
}

func (n *remoteSurface) Size() (int, int) {
	i := n.info()
	return i.W, i.H
}

// Close closes the host window (unless it is gone already) and then, like
// a backend, reports SurfaceClosed.
func (n *remoteSurface) Close() {
	if n.closed {
		return
	}
	n.closed = true
	if !n.hostGone {
		_ = n.w.cl.Send(devhost.CloseWindow{Win: n.w.id})
	}
	devPost(func() { n.ev.Event(platform.SurfaceEvent{Kind: platform.SurfaceClosed}) })
}

func (n *remoteSurface) Navigate(string) {}
func (n *remoteSurface) Eval(string)     {}
func (n *remoteSurface) OpenDevTools()   {}
func (n *remoteSurface) Native() uintptr { return 0 }

func (n *remoteSurface) Bounds() platform.Rect {
	b := n.info().Bounds
	return platform.Rect{X: b.X, Y: b.Y, Width: b.W, Height: b.H}
}

func (n *remoteSurface) SetBounds(r platform.Rect) {
	n.op(devhost.SurfaceOp{Op: devhost.OpSetBounds, Rect: devhost.Rect{X: r.X, Y: r.Y, W: r.Width, H: r.Height}})
}

func (n *remoteSurface) Center() { n.op(devhost.SurfaceOp{Op: devhost.OpCenter}) }

func (n *remoteSurface) SetSizeLimits(minW, minH, maxW, maxH int) {
	n.op(devhost.SurfaceOp{Op: devhost.OpSetSizeLimits, Limits: [4]int{minW, minH, maxW, maxH}})
}

func (n *remoteSurface) State() platform.WindowState { return platform.WindowState(n.info().State) }

func (n *remoteSurface) SetState(st platform.WindowState) {
	n.op(devhost.SurfaceOp{Op: devhost.OpSetState, State: int(st)})
}

func (n *remoteSurface) SetAppearance(a platform.Appearance) {
	n.op(devhost.SurfaceOp{Op: devhost.OpSetAppearance, Appearance: devhost.Appearance{
		TitleBar: int(a.TitleBar), Transparent: a.Transparent, Background: a.Background,
		Material: int(a.Material), AlwaysOnTop: a.AlwaysOnTop, Opacity: a.Opacity,
		NoShadow: a.NoShadow, NoClose: a.NoClose, NoMinimize: a.NoMinimize, NoMaximize: a.NoMaximize,
	}})
}

func (n *remoteSurface) IsVisible() bool { return n.info().Visible }
func (n *remoteSurface) IsFocused() bool { return n.info().Focused }

// surfaceEvent passes a host window event to the stand-in. Main thread.
func (w *remoteWin) surfaceEvent(m devhost.SurfaceEvent) {
	if w.closed || w.ns == nil || w.ns.closed {
		return
	}
	var ev platform.SurfaceEvent
	switch m.Kind {
	case devhost.EvResized:
		ev = platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: m.W, Height: m.H}
	case devhost.EvMoved:
		ev = platform.SurfaceEvent{Kind: platform.SurfaceMoved, X: m.X, Y: m.Y}
	case devhost.EvStateChanged:
		ev = platform.SurfaceEvent{Kind: platform.SurfaceStateChanged, State: platform.WindowState(m.State)}
	case devhost.EvFocused:
		ev = platform.SurfaceEvent{Kind: platform.SurfaceFocused}
	case devhost.EvBlurred:
		ev = platform.SurfaceEvent{Kind: platform.SurfaceBlurred}
	default:
		return
	}
	w.ns.ev.Event(ev)
}

// closeRequested: the user tried to close the host window; it closes if
// the stand-in's OnClose handlers agree. Main thread.
func (w *remoteWin) closeRequested() {
	if w.closed {
		return
	}
	if w.ns == nil || w.proxy == nil {
		_ = w.cl.Send(devhost.CloseWindow{Win: w.id})
		return
	}
	if w.ns.ev.ShouldClose() {
		w.proxy.Close()
	}
}

// ---- menus ----

// devAppMenu is corehook.AppMenuProxy: in a worker, the application menu
// and the window menus of stand-in surfaces show on the host.
func devAppMenu(app any, parent platform.SurfaceID) (platform.AppMenu, bool) {
	if devClient() == nil {
		return nil, false
	}
	a, _ := app.(*tango.App)
	if parent == 0 {
		return remoteMenu{app: a}, true
	}
	dev.mu.Lock()
	defer dev.mu.Unlock()
	for id, w := range dev.wins {
		if w.proxy != nil && w.proxy.ID() == parent && w.proxy.App() == a {
			return remoteMenu{app: a, win: id}, true
		}
	}
	return nil, false // one of the worker's own windows
}

// remoteMenu sets a menu on the host. Its SetAppMenu runs on the main
// thread.
type remoteMenu struct {
	app *tango.App
	win int // 0: the application menu
}

func (r remoteMenu) SetAppMenu(_ platform.SurfaceID, items []platform.MenuItem, onClick func(id uint32)) {
	cl := devClient()
	if cl == nil {
		return
	}
	dev.mu.Lock()
	if items == nil {
		delete(dev.menus, r.win)
	} else {
		dev.menus[r.win] = onClick
	}
	dev.mu.Unlock()
	_ = cl.Send(devhost.SetMenu{Win: r.win, Items: wireMenu(items), Clear: items == nil})
	if r.win == 0 && r.app != nil {
		// Also the worker's own WebView windows (Windows, Linux); on macOS
		// the worker's menu bar never shows.
		if am, ok := r.app.Backend().(platform.AppMenu); ok {
			am.SetAppMenu(0, items, onClick)
		}
	}
}

func wireMenu(items []platform.MenuItem) []devhost.MenuItem {
	if items == nil {
		return nil
	}
	out := make([]devhost.MenuItem, len(items))
	for i, it := range items {
		out[i] = devhost.MenuItem{
			ID: it.ID, Label: it.Label, Accelerator: it.Accelerator, Role: it.Role,
			Separator: it.Separator, Disabled: it.Disabled, Checked: it.Checked,
			Checkable: it.Checkable, RadioGroup: it.RadioGroup, Icon: it.Icon,
			Submenu: wireMenu(it.Submenu),
		}
	}
	return out
}
