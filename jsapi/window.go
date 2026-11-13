package jsapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
)

// Window module: the calling surface controls itself.

type sizeJS struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type pointJS struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type rectJS struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func toRect(r tango.Rect) rectJS { return rectJS{r.X, r.Y, r.Width, r.Height} }

var stateNames = map[tango.WindowState]string{
	tango.StateNormal:     "normal",
	tango.StateMinimized:  "minimized",
	tango.StateMaximized:  "maximized",
	tango.StateFullscreen: "fullscreen",
}

func stateName(st tango.WindowState) string {
	if n, ok := stateNames[st]; ok {
		return n
	}
	return "normal"
}

func parseState(n string) (tango.WindowState, bool) {
	for st, name := range stateNames {
		if name == n {
			return st, true
		}
	}
	return 0, false
}

// replyDelay lets the reply of a call that ends the page (close, reload,
// quit) reach it first.
const replyDelay = 50 * time.Millisecond

func later(fn func()) { time.AfterFunc(replyDelay, fn) }

// windowHandler adapts a handler that needs the calling surface.
func windowHandler(fn func(s *tango.Surface, args json.RawMessage) (any, error)) Handler {
	return func(_ context.Context, s *tango.Surface, args json.RawMessage) (any, error) {
		if s == nil {
			return nil, errNoSurface
		}
		return fn(s, args)
	}
}

// do adapts a no-argument action.
func do(fn func(s *tango.Surface) error) Handler {
	return windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) { return nil, fn(s) })
}

func (api *API) registerWindow() {
	h := func(name string, fn Handler) { api.Handle(Window, "window."+name, fn) }

	h("id", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) { return uint64(s.ID()), nil }))
	h("setTitle", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var title string
		if err := decode(args, &title); err != nil {
			return nil, err
		}
		s.SetTitle(title)
		return nil, nil
	}))
	h("size", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) {
		w, ht := s.Size()
		return sizeJS{w, ht}, nil
	}))
	h("setSize", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var w, ht int
		if err := decode(args, &w, &ht); err != nil {
			return nil, err
		}
		if w <= 0 || ht <= 0 {
			return nil, ipc.NewError(ipc.CodeBadArgs, "width and height must be positive")
		}
		s.SetSize(w, ht)
		return nil, nil
	}))
	h("setSizeLimits", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var minW, minH, maxW, maxH int
		if err := decode(args, &minW, &minH, &maxW, &maxH); err != nil {
			return nil, err
		}
		return nil, s.SetSizeLimits(minW, minH, maxW, maxH)
	}))
	h("bounds", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) {
		return toRect(s.Bounds()), nil
	}))
	h("setBounds", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var r rectJS
		if err := decode(args, &r); err != nil {
			return nil, err
		}
		if r.Width <= 0 || r.Height <= 0 {
			return nil, ipc.NewError(ipc.CodeBadArgs, "width and height must be positive")
		}
		return nil, s.SetBounds(tango.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height})
	}))
	h("position", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) {
		b := s.Bounds()
		return pointJS{b.X, b.Y}, nil
	}))
	h("setPosition", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var x, y int
		if err := decode(args, &x, &y); err != nil {
			return nil, err
		}
		b := s.Bounds()
		if b.Width == 0 || b.Height == 0 {
			b.Width, b.Height = s.Size()
		}
		b.X, b.Y = x, y
		return nil, s.SetBounds(b)
	}))
	h("center", do((*tango.Surface).Center))
	h("state", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) {
		return stateName(s.State()), nil
	}))
	h("setState", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var name string
		if err := decode(args, &name); err != nil {
			return nil, err
		}
		st, ok := parseState(name)
		if !ok {
			return nil, ipc.NewError(ipc.CodeBadArgs, "unknown window state "+name)
		}
		return nil, s.SetState(st)
	}))
	h("minimize", do((*tango.Surface).Minimize))
	h("maximize", do((*tango.Surface).Maximize))
	h("fullscreen", do((*tango.Surface).Fullscreen))
	h("restore", do((*tango.Surface).Restore))
	h("toggleMaximize", do(func(s *tango.Surface) error {
		if s.State() == tango.StateMaximized {
			return s.Restore()
		}
		return s.Maximize()
	}))
	h("setAlwaysOnTop", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var on bool
		if err := decode(args, &on); err != nil {
			return nil, err
		}
		ap := s.Appearance()
		ap.AlwaysOnTop = on
		return nil, s.SetAppearance(ap)
	}))
	h("alwaysOnTop", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) {
		return s.Appearance().AlwaysOnTop, nil
	}))
	h("show", do(func(s *tango.Surface) error { s.Show(); return nil }))
	h("hide", do(func(s *tango.Surface) error { s.Hide(); return nil }))
	h("focus", do(func(s *tango.Surface) error { s.Focus(); return nil }))
	h("close", do(func(s *tango.Surface) error {
		later(s.Close)
		return nil
	}))
	h("zoom", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) { return s.Zoom(), nil }))
	h("setZoom", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var f float64
		if err := decode(args, &f); err != nil {
			return nil, err
		}
		if f <= 0 {
			return nil, ipc.NewError(ipc.CodeBadArgs, "zoom factor must be positive")
		}
		return nil, s.SetZoom(f)
	}))
	h("reload", do(func(s *tango.Surface) error {
		later(func() { _ = s.Reload() })
		return nil
	}))
	h("print", do((*tango.Surface).Print))
	h("openDevTools", do(func(s *tango.Surface) error { s.OpenDevTools(); return nil }))
	h("title", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) { return s.Title(), nil }))
	h("isVisible", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) { return s.IsVisible(), nil }))
	h("isFocused", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) { return s.IsFocused(), nil }))
}

// hookWindow forwards s's window events to its page.
func (api *API) hookWindow(s *tango.Surface) {
	if !api.Allowed(Window) {
		return
	}
	s.OnResized(func(w, h int) { api.Emit(s, "window.resize", sizeJS{w, h}) })
	s.OnFocused(func() { api.Emit(s, "window.focus", nil) })
	s.OnBlurred(func() { api.Emit(s, "window.blur", nil) })
	s.OnMoved(func(x, y int) { api.Emit(s, "window.move", pointJS{x, y}) })
	s.OnStateChanged(func(st tango.WindowState) {
		api.Emit(s, "window.state", map[string]string{"state": stateName(st)})
	})
	s.OnFilesDropped(func(paths []string, x, y int) {
		if paths == nil {
			paths = []string{}
		}
		api.Emit(s, "window.filesDropped", map[string]any{"paths": paths, "x": x, "y": y})
	})
}
