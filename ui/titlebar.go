package ui

import (
	"runtime"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shellMac selects the macOS conventions of the shell components (traffic
// lights at the start, ⌘ key names). Tests switch it.
var shellMac = runtime.GOOS == "darwin" || runtime.GOOS == "ios"

// WindowControlsStyle is how WindowControls draws the window buttons.
//
// Append-only: never reorder or insert; the zero value stays
// WindowControlsAuto.
type WindowControlsStyle uint8

const (
	// WindowControlsAuto follows the platform: on macOS traffic lights at
	// the start (only room for them when the system draws its own),
	// elsewhere caption buttons at the end; nothing when the window keeps
	// its system title bar.
	WindowControlsAuto WindowControlsStyle = iota
	// WindowControlsCaption draws Windows / Linux caption buttons
	// (minimize, maximize / restore, close) at the end.
	WindowControlsCaption
	// WindowControlsTraffic draws macOS traffic lights (close, minimize,
	// full screen) at the start.
	WindowControlsTraffic
	// WindowControlsNone draws no buttons.
	WindowControlsNone
)

// WindowAction is a window button the user activated. Open set.
type WindowAction string

const (
	WindowActionMinimize   WindowAction = "minimize"
	WindowActionMaximize   WindowAction = "maximize"
	WindowActionRestore    WindowAction = "restore" // back from maximized or full screen
	WindowActionFullscreen WindowAction = "fullscreen"
	WindowActionClose      WindowAction = "close"
)

// windowStyle resolves Auto for the window described by info (has: a
// frame was found). reserve reports that the system draws traffic lights
// the bar must keep clear of.
func windowStyle(s WindowControlsStyle, info WindowFrameInfo, has bool) (st WindowControlsStyle, reserve bool) {
	if s != WindowControlsAuto {
		return s, false
	}
	switch {
	case has && info.SystemTitleBar:
		return WindowControlsNone, false
	case has && info.SystemButtons:
		return WindowControlsNone, true
	case shellMac:
		return WindowControlsTraffic, false
	}
	return WindowControlsCaption, false
}

// applyWindowAction performs act on f.
func applyWindowAction(f WindowFrame, act WindowAction) {
	if f == nil {
		return
	}
	switch act {
	case WindowActionMinimize:
		f.SetWindowState(WindowMinimized)
	case WindowActionMaximize:
		f.SetWindowState(WindowMaximized)
	case WindowActionFullscreen:
		f.SetWindowState(WindowFullscreen)
	case WindowActionRestore:
		f.SetWindowState(WindowNormal)
	case WindowActionClose:
		f.CloseWindow()
	}
}

// ---------------------------------------------------------------------------
// WindowControls

// WindowControlsW is the window button group WindowControls returns.
type WindowControlsW struct {
	_ [0]func()

	c      *Context
	th     *theme.Theme
	tk     *theme.TitleBarToken
	box    *Box
	style  WindowControlsStyle
	frame  WindowFrame
	manual bool
	height float32
	act    WindowAction
	z      *lazy
}

// WindowControls adds the minimize / maximize / close buttons of a
// frameless window. By default they act on the window the page draws into
// (Context.Window, when it implements WindowFrame) and follow the
// platform (WindowControlsAuto). TitleBar includes them; use
// WindowControls alone in a custom title area.
func WindowControls(c *Context) *WindowControlsW {
	w := &WindowControlsW{c: c}
	w.th = c.ThemeFor(theme.CompTitleBar)
	w.tk = &w.th.Components.TitleBar
	w.height = w.tk.Height
	w.box = c.box("window-controls", layout.Row, []layout.Option{layout.Middle()})
	w.box.n.ln.NoStretch = true
	w.box.n.setRole(a11y.RoleGroup).setName(c.Locale().TitleBar.WindowControls)
	w.z = c.lazy(w.box.n, w.build)
	return w
}

// Style picks the button style (WindowControlsAuto by default).
func (w *WindowControlsW) Style(s WindowControlsStyle) *WindowControlsW { w.style = s; return w }

// Frame sets the window the buttons act on, in place of Context.Window.
func (w *WindowControlsW) Frame(f WindowFrame) *WindowControlsW { w.frame = f; return w }

// Manual only reports the activated button (Action) and leaves the window
// alone, for apps that confirm before closing or route the actions
// elsewhere.
func (w *WindowControlsW) Manual() *WindowControlsW { w.manual = true; return w }

// Height sets the height of caption buttons (the TitleBar token's Height
// by default).
func (w *WindowControlsW) Height(px float32) *WindowControlsW { w.height = px; return w }

// Action reports the button activated this frame ("" for none). Unless
// Manual, the window has already been asked to perform it.
func (w *WindowControlsW) Action() WindowAction { w.z.run(); return w.act }

// ID returns the group's ID.
func (w *WindowControlsW) ID() ID { return w.box.n.id }

func (w *WindowControlsW) frameOf() WindowFrame {
	if w.frame != nil {
		return w.frame
	}
	return w.c.windowFrame()
}

func (w *WindowControlsW) build() {
	f := w.frameOf()
	var info WindowFrameInfo
	if f != nil {
		info = f.FrameInfo()
	}
	st, _ := windowStyle(w.style, info, f != nil)
	switch st {
	case WindowControlsCaption:
		w.act = w.caption(info)
	case WindowControlsTraffic:
		w.act = w.traffic(info)
	}
	if w.act != "" && !w.manual {
		applyWindowAction(f, w.act)
		w.c.invalid = true
	}
}

// caption builds Windows / Linux caption buttons.
func (w *WindowControlsW) caption(info WindowFrameInfo) WindowAction {
	c, th, tk := w.c, w.th, w.tk
	loc := c.Locale()
	var act WindowAction
	btn := func(ic *icon.Icon, name string, a WindowAction, disabled, close bool) {
		n := c.add("window-control", name)
		n.ln.NoStretch = true
		n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: tk.ControlWidth, H: w.height} }
		n.setRole(a11y.RoleButton)
		n.disabled = n.disabled || disabled
		ia := c.interact(n, true, input.CursorDefault)
		if disabled {
			n.setState(a11y.Disabled)
		}
		if ia.Clicked && !n.disabled {
			act = a
		}
		focused := c.WindowFocused()
		n.paint = func(l *paint.List, r geom.Rect) {
			col := tk.ControlColor
			if !focused {
				col = tk.ControlInactive
			}
			switch {
			case n.disabled:
				col = th.TextDisabled
			case close && ia.Pressed:
				l.Fill(r, geom.Radii{}, tk.CloseActiveBg)
				col = tk.CloseHoverColor
			case close && ia.Hovered:
				l.Fill(r, geom.Radii{}, tk.CloseHoverBg)
				col = tk.CloseHoverColor
			case ia.Pressed:
				l.Fill(r, geom.Radii{}, tk.ControlActiveBg)
			case ia.Hovered:
				l.Fill(r, geom.Radii{}, tk.ControlHoverBg)
			}
			s := tk.ControlIconSize
			c.paintIcon(l, ic, geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, col, 0)
			if ia.FocusVisible {
				l.Stroke(geom.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}, geom.Radii{}, 2, th.FocusRing)
			}
		}
	}
	btn(icon.Minimize, loc.TitleBar.Minimize, WindowActionMinimize, info.NoMinimize, false)
	if info.State == WindowMaximized || info.State == WindowFullscreen {
		btn(icon.Restore, loc.TitleBar.Restore, WindowActionRestore, info.NoMaximize, false)
	} else {
		btn(icon.Maximize, loc.TitleBar.Maximize, WindowActionMaximize, info.NoMaximize, false)
	}
	btn(icon.Close, loc.Close, WindowActionClose, info.NoClose, true)
	return act
}

// traffic builds macOS-style traffic lights; their symbols show while the
// pointer is over the group.
func (w *WindowControlsW) traffic(info WindowFrameInfo) WindowAction {
	c, tk := w.c, w.tk
	loc := c.Locale()
	w.box.Style(layout.Gap(tk.TrafficGap), layout.PadVH(0, tk.TrafficInset))
	over := c.pointerOver(w.box.n.id)
	focused := c.WindowFocused()
	var act WindowAction
	light := func(col theme.Color, ic *icon.Icon, name string, a WindowAction, disabled bool) {
		n := c.add("window-control", name)
		n.ln.NoStretch = true
		n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: tk.TrafficSize, H: tk.TrafficSize} }
		n.setRole(a11y.RoleButton)
		n.disabled = n.disabled || disabled
		ia := c.interact(n, true, input.CursorDefault)
		if disabled {
			n.setState(a11y.Disabled)
		}
		if ia.Clicked && !n.disabled {
			act = a
		}
		n.paint = func(l *paint.List, r geom.Rect) {
			fill := col
			if (!focused && !over) || n.disabled {
				fill = tk.TrafficInactive
			}
			cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, r.W/2
			l.Path(paint.NewPath().Circle(cx, cy, rad), fill)
			if tk.TrafficBorder.A > 0 {
				l.StrokePath(paint.NewPath().Circle(cx, cy, rad-0.25), paint.StrokeStyle{Width: 0.5}, tk.TrafficBorder)
			}
			if over && !n.disabled {
				if ia.Pressed {
					l.Path(paint.NewPath().Circle(cx, cy, rad), theme.Fade(tk.TrafficGlyph, .25))
				}
				s := r.W * 0.7
				icon.Draw(l, ic, geom.Rect{X: cx - s/2, Y: cy - s/2, W: s, H: s}, tk.TrafficGlyph, 0)
			}
			if ia.FocusVisible {
				l.StrokePath(paint.NewPath().Circle(cx, cy, rad+2), paint.StrokeStyle{Width: 2}, w.th.FocusRing)
			}
		}
	}
	// Close, minimize, full screen: in reading order (mirrored in RTL like
	// macOS does).
	light(tk.TrafficClose, icon.Close, loc.Close, WindowActionClose, info.NoClose)
	light(tk.TrafficMinimize, icon.Minimize, loc.TitleBar.Minimize, WindowActionMinimize, info.NoMinimize)
	if info.State == WindowFullscreen {
		light(tk.TrafficZoom, icon.FullscreenExit, loc.ExitFullscreen, WindowActionRestore, info.NoMaximize)
	} else {
		light(tk.TrafficZoom, icon.Fullscreen, loc.Fullscreen, WindowActionFullscreen, info.NoMaximize)
	}
	return act
}

// ---------------------------------------------------------------------------
// TitleBar

// TitleBarW is the self-drawn title bar TitleBar returns.
type TitleBarW struct {
	_ [0]func()

	c      *Context
	th     *theme.Theme
	tk     *theme.TitleBarToken
	box    *Box
	title  string
	ic     *icon.Icon
	menu   func()
	center func()
	tools  func()
	style  WindowControlsStyle
	frame  WindowFrame
	manual bool
	ia     Interaction
	act    WindowAction
	z      *lazy
}

type titleBarState struct {
	last    time.Time // last press on the drag area (double-click detection)
	lastPos geom.Point
}

// TitleBar adds the title bar of a frameless window (SurfaceOptions
// Appearance.TitleBar = TitleBarNone, or TitleBarHidden on macOS):
//
//	ui.TitleBar(c).Icon(icon.Code).Title("Tango — main.go").
//		Menu(func() { ui.Menubar(c, menus) }).
//		Center(func() { ui.Search(c, &q) }).
//		Tools(func() { ui.IconButton(c, icon.Setting, "设置") })
//
// Pressing the bar's empty parts moves the window and a double click
// maximizes or restores it; the window buttons follow the platform (see
// WindowControlsStyle). Everything acts on Context.Window when it
// implements WindowFrame (uihost windows do); without a frame the bar
// still draws and reports Action.
func TitleBar(c *Context) *TitleBarW {
	t := &TitleBarW{c: c}
	t.th = c.ThemeFor(theme.CompTitleBar)
	t.tk = &t.th.Components.TitleBar
	t.box = c.box("titlebar", layout.Row, []layout.Option{layout.Middle()})
	t.box.n.paint = t.paint
	t.z = c.lazy(t.box.n, t.build)
	return t
}

// Title sets the window title shown in the bar.
func (t *TitleBarW) Title(s string) *TitleBarW { t.title = s; return t }

// Icon shows the app icon at the bar's start.
func (t *TitleBarW) Icon(ic *icon.Icon) *TitleBarW { t.ic = ic; return t }

// Menu builds content after the icon (a Menubar, a project switcher).
func (t *TitleBarW) Menu(fn func()) *TitleBarW { t.menu = fn; return t }

// Center builds content in the middle of the bar (a command search box).
// Without it the title is centered on macOS and follows the menu
// elsewhere.
func (t *TitleBarW) Center(fn func()) *TitleBarW { t.center = fn; return t }

// Tools builds content before the window buttons (layout toggles,
// notifications, the account menu).
func (t *TitleBarW) Tools(fn func()) *TitleBarW { t.tools = fn; return t }

// Controls picks the window button style (WindowControlsAuto by default).
func (t *TitleBarW) Controls(s WindowControlsStyle) *TitleBarW { t.style = s; return t }

// Frame sets the window the bar drives, in place of Context.Window.
func (t *TitleBarW) Frame(f WindowFrame) *TitleBarW { t.frame = f; return t }

// Manual only reports window actions (Action) and leaves the window alone;
// dragging still moves it.
func (t *TitleBarW) Manual() *TitleBarW { t.manual = true; return t }

// Layout adds layout options to the bar.
func (t *TitleBarW) Layout(o ...layout.Option) *TitleBarW {
	flexbox.Apply(&t.box.n.ln.Style, o...)
	return t
}

// Action reports the window action the user asked for this frame (a
// window button, or a double click on the bar), "" for none.
func (t *TitleBarW) Action() WindowAction { t.z.run(); return t.act }

// ID returns the bar's ID.
func (t *TitleBarW) ID() ID { return t.box.n.id }

func (t *TitleBarW) frameOf() WindowFrame {
	if t.frame != nil {
		return t.frame
	}
	return t.c.windowFrame()
}

func (t *TitleBarW) build() {
	c, tk := t.c, t.tk
	n := t.box.n
	f := t.frameOf()
	var info WindowFrameInfo
	if f != nil {
		info = f.FrameInfo()
	}
	style, reserve := windowStyle(t.style, info, f != nil)
	t.box.Style(layout.H(tk.Height), layout.Gap(tk.Gap))
	n.setRole(a11y.RoleGroup).setName(t.title)

	// The bar itself is the drag area: presses that land on no control
	// inside it move the window; a double click maximizes or restores.
	t.ia = c.interact(n, false, input.CursorDefault)
	if t.ia.PressedNow {
		st := State[titleBarState](c, hashID(n.id, "titlebar"), nil)
		p := c.pressPos
		dbl := !st.last.IsZero() && c.now.Sub(st.last) < 450*time.Millisecond &&
			abs32(p.X-st.lastPos.X) < 5 && abs32(p.Y-st.lastPos.Y) < 5
		if dbl {
			st.last = time.Time{}
			if !info.NoMaximize {
				if info.State == WindowMaximized {
					t.act = WindowActionRestore
				} else if info.State == WindowNormal {
					t.act = WindowActionMaximize
				}
			}
		} else {
			st.last, st.lastPos = c.now, p
			if f != nil {
				f.StartWindowMove()
				// The window manager owns the drag now and may swallow the
				// release: do not leave the bar pressed.
				c.active = 0
			}
		}
	}

	lead := tk.PaddingInline
	if style == WindowControlsTraffic {
		lead = 0
	}
	trail := tk.PaddingInline
	if style == WindowControlsCaption {
		trail = 0
	}
	t.box.Style(layout.Padding(geom.Insets{Left: lead, Right: trail}))

	switch {
	case reserve:
		Div(c, layout.W(max(0, tk.SystemInset-tk.Gap)))
	case style == WindowControlsTraffic:
		t.controls(style)
	}
	if t.ic != nil {
		Icon(c, t.ic).Size(tk.IconSize).Color(t.titleColor())
	}
	centered := t.center == nil && (style == WindowControlsTraffic || reserve)
	if t.title != "" && !centered {
		t.titleText()
	}
	if t.menu != nil {
		// A Menubar in the bar loses its own strip: no fill, outline, rule
		// or shadow, the bar's height.
		ConfigProvider(c).Components(theme.Components{Menubar: theme.MenubarToken{
			Bg: theme.Transparent, BorderColor: theme.Transparent, DividerColor: theme.Transparent,
			Shadow: theme.NoShadow, Height: tk.Height, PaddingInline: theme.ZeroPx,
		}}).Body(func() { Row(c, t.menu, layout.Middle(), layout.Gap(4)) })
	}
	Div(c, layout.Grow(1), layout.MinW(0))
	switch {
	case t.center != nil:
		Row(c, t.center, layout.Middle(), layout.Shrink(1), layout.MinW(0))
	case centered && t.title != "":
		t.titleText()
	}
	Div(c, layout.Grow(1), layout.MinW(0))
	if t.tools != nil {
		Row(c, t.tools, layout.Middle(), layout.Gap(4))
	}
	if style == WindowControlsCaption {
		t.controls(style)
	}
	if t.act != "" && !t.manual {
		applyWindowAction(f, t.act)
		c.invalid = true
	}
}

// controls adds the window buttons; their action becomes the bar's.
func (t *TitleBarW) controls(style WindowControlsStyle) {
	wc := WindowControls(t.c).Style(style).Height(t.tk.Height).Manual()
	if t.frame != nil {
		wc.Frame(t.frame)
	}
	if a := wc.Action(); a != "" {
		t.act = a
	}
}

func (t *TitleBarW) titleColor() theme.Color {
	if t.c.WindowFocused() {
		return t.tk.TitleColor
	}
	return t.tk.TitleInactiveColor
}

func (t *TitleBarW) titleText() {
	tk := t.tk
	tx := Text(t.c, t.title).Size(tk.TitleFontSize).Color(t.titleColor()).NoWrap().Ellipsis()
	if tk.TitleFontWeight > 0 {
		tx.Weight(text.Weight(tk.TitleFontWeight))
	}
	tx.Layout(layout.Shrink(1), layout.MinW(0))
	tx.n.aHidden = true // the bar is named by the title
}

func (t *TitleBarW) paint(l *paint.List, r geom.Rect) {
	tk := t.tk
	l.Fill(r, geom.Radii{}, tk.Bg)
	if tk.BorderColor.A > 0 {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.BorderColor)
	}
}
