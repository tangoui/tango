package host

// Window calls, window events and menus of workers (see
// devhost/proto_surface.go): a worker's stand-in tango.Surface acts on the
// host window, the host window's events go to the worker that draws it,
// and menus a worker sets with sys/menu show here.

import (
	"runtime"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
)

// surfaceOp runs a worker's Surface call on its window and answers it.
// Calls of a pending worker wait for it to take over the window, so the
// old version keeps its window as it was until the swap (except for a
// window nobody draws yet). Main thread.
func (h *Host) surfaceOp(w *worker, m devhost.SurfaceOp) {
	x := h.wins[m.Win]
	if x != nil && !x.closed && m.Op != devhost.OpGet {
		switch {
		case x.owner == w:
			h.applyOp(x, m)
		case x.next == w && (x.owner == nil || x.owner.gone):
			h.applyOp(x, m)
		case x.next == w:
			x.nextOps = append(x.nextOps, m)
		}
	}
	if m.Req != 0 {
		w.send(h.info(x, m.Req))
	}
}

func (h *Host) applyOp(x *win, m devhost.SurfaceOp) {
	s := x.surf
	switch m.Op {
	case devhost.OpShow:
		s.Show()
	case devhost.OpHide:
		s.Hide()
	case devhost.OpFocus:
		s.Focus()
	case devhost.OpSetTitle:
		s.SetTitle(m.Title)
	case devhost.OpSetSize:
		s.SetSize(m.Rect.W, m.Rect.H)
	case devhost.OpSetBounds:
		_ = s.SetBounds(tango.Rect{X: m.Rect.X, Y: m.Rect.Y, Width: m.Rect.W, Height: m.Rect.H})
	case devhost.OpCenter:
		_ = s.Center()
	case devhost.OpSetSizeLimits:
		_ = s.SetSizeLimits(m.Limits[0], m.Limits[1], m.Limits[2], m.Limits[3])
	case devhost.OpSetState:
		_ = s.SetState(tango.WindowState(m.State))
	case devhost.OpSetAppearance:
		a := m.Appearance
		_ = s.SetAppearance(tango.Appearance{
			TitleBar: tango.TitleBar(a.TitleBar), Transparent: a.Transparent, Background: a.Background,
			Material: tango.Material(a.Material), AlwaysOnTop: a.AlwaysOnTop, Opacity: a.Opacity,
			NoShadow: a.NoShadow, NoClose: a.NoClose, NoMinimize: a.NoMinimize, NoMaximize: a.NoMaximize,
		})
	}
}

// info is the current state of window x (nil: gone).
func (h *Host) info(x *win, req int) devhost.SurfaceInfo {
	if x == nil || x.closed {
		return devhost.SurfaceInfo{Req: req, Err: "window closed"}
	}
	s := x.surf
	b := s.Bounds()
	w, ht := s.Size()
	return devhost.SurfaceInfo{
		Req: req, Title: s.Title(),
		Bounds: devhost.Rect{X: b.X, Y: b.Y, W: b.Width, H: b.Height},
		W:      w, H: ht, State: int(s.State()),
		Visible: s.IsVisible(), Focused: s.IsFocused(),
	}
}

// watchSurface forwards the events of x's surface to the worker drawing
// it and lets that worker decide about closing it.
func (h *Host) watchSurface(x *win) {
	s := x.surf
	s.OnResized(func(w, ht int) { x.event(devhost.SurfaceEvent{Kind: devhost.EvResized, W: w, H: ht}) })
	s.OnMoved(func(px, py int) { x.event(devhost.SurfaceEvent{Kind: devhost.EvMoved, X: px, Y: py}) })
	s.OnStateChanged(func(st tango.WindowState) {
		x.event(devhost.SurfaceEvent{Kind: devhost.EvStateChanged, State: int(st)})
	})
	s.OnFocused(func() { x.event(devhost.SurfaceEvent{Kind: devhost.EvFocused}) })
	s.OnBlurred(func() { x.event(devhost.SurfaceEvent{Kind: devhost.EvBlurred}) })
	s.OnClose(x.closeAsked)
}

// event sends a window event to the worker drawing x, or about to (the
// first one, before its first frame). Main thread.
func (x *win) event(m devhost.SurfaceEvent) {
	if x.closed {
		return
	}
	m.Win = x.id
	if w := x.drawer(); w != nil {
		w.send(m)
	}
}

// drawer is the owner, or the pending worker of a window without one.
func (x *win) drawer() *worker {
	w := x.owner
	if w == nil || w.gone {
		w = x.next
	}
	if w == nil || w.gone {
		return nil
	}
	return w
}

// closeAsked is the host window's OnClose: the worker drawing it asks its
// own OnClose handlers and closes the window (CloseWindow) if they agree.
// Without a worker the window just closes. Main thread.
func (x *win) closeAsked() bool {
	w := x.drawer()
	if w == nil {
		return true
	}
	// Also while w is frozen for a swap: holding the request back could
	// leave the window unclosable if the swap does not happen.
	w.send(devhost.CloseRequest{Win: x.id})
	return false
}

// ---- menus ----

// setMenu records a worker's menu and shows it if the worker is on
// screen; a pending worker's menus replace the old ones when it takes
// over (commitMenus).
func (h *Host) setMenu(w *worker, m devhost.SetMenu) {
	w.menus[m.Win] = m
	if w == h.active && !w.retired {
		h.showMenu(m)
	}
}

// commitMenus shows the menus of p, which just went on screen, and clears
// those of the previous worker p did not set.
func (h *Host) commitMenus(p *worker) {
	for win := range h.menus {
		if _, ok := p.menus[win]; !ok {
			h.showMenu(devhost.SetMenu{Win: win, Clear: true})
		}
	}
	for _, m := range p.menus {
		h.showMenu(m)
	}
}

// defaultMenu is what the host shows when no worker set an application
// menu: the backend's default on macOS (App, Edit, Window), none elsewhere.
func defaultMenu() []platform.MenuItem {
	if runtime.GOOS == "darwin" {
		return []platform.MenuItem{{Role: "appMenu"}, {Role: "editMenu"}, {Role: "windowMenu"}}
	}
	return nil
}

func (h *Host) showMenu(m devhost.SetMenu) {
	am, ok := h.app.Backend().(platform.AppMenu)
	if !ok {
		return
	}
	var parent platform.SurfaceID
	if m.Win != 0 {
		x := h.wins[m.Win]
		if x == nil || x.closed {
			return
		}
		parent = x.surf.ID()
	}
	if m.Clear {
		if !h.menus[m.Win] && m.Win != 0 {
			return
		}
		delete(h.menus, m.Win)
		var items []platform.MenuItem
		if m.Win == 0 {
			items = defaultMenu()
		}
		am.SetAppMenu(parent, items, func(uint32) {})
		return
	}
	h.menus[m.Win] = true
	win := m.Win
	am.SetAppMenu(parent, platformMenu(m.Items), func(id uint32) {
		if a := h.active; a != nil && !a.gone {
			a.send(devhost.MenuClick{Win: win, ID: id})
		}
	})
}

func platformMenu(items []devhost.MenuItem) []platform.MenuItem {
	out := make([]platform.MenuItem, len(items))
	for i, it := range items {
		out[i] = platform.MenuItem{
			ID: it.ID, Label: it.Label, Accelerator: it.Accelerator, Role: it.Role,
			Separator: it.Separator, Disabled: it.Disabled, Checked: it.Checked,
			Checkable: it.Checkable, RadioGroup: it.RadioGroup, Icon: it.Icon,
		}
		if it.Submenu != nil {
			out[i].Submenu = platformMenu(it.Submenu)
		}
	}
	return out
}
