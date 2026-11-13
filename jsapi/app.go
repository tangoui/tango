package jsapi

import (
	"context"
	"encoding/json"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/appctl"
	"github.com/tangoui/tango/sys/env"
	"github.com/tangoui/tango/sys/screen"
	"github.com/tangoui/tango/sys/theme"
)

// App, Theme and Screens modules, and app event forwarding.

type envJS struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	OSName         string `json:"osName"`
	OSVersion      string `json:"osVersion"`
	Debug          bool   `json:"debug"`
	WebView        string `json:"webView"`
	WebViewVersion string `json:"webViewVersion"`
}

type themeJS struct {
	Dark   bool   `json:"dark"`
	Accent string `json:"accent"` // #rrggbbaa, "" when unknown
}

func toTheme(t theme.Theme) themeJS { return themeJS{t.Dark, theme.AccentCSS(t.Accent)} }

type displayJS struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Bounds   rectJS  `json:"bounds"`
	WorkArea rectJS  `json:"workArea"`
	Scale    float64 `json:"scale"`
	Primary  bool    `json:"primary"`
}

func toDisplay(d screen.Display) displayJS {
	return displayJS{d.ID, d.Name, toRect(d.Bounds), toRect(d.WorkArea), d.Scale, d.Primary}
}

func toDisplays(ds []screen.Display) []displayJS {
	out := make([]displayJS, 0, len(ds))
	for _, d := range ds {
		out = append(out, toDisplay(d))
	}
	return out
}

// pendingEvent is a launch-time app event kept until a page takes it.
type pendingEvent struct {
	kind string // "openURL" | "openFiles" | "reopen"
	data any
}

const maxPending = 32

func noArgs(fn func() (any, error)) Handler {
	return func(context.Context, *tango.Surface, json.RawMessage) (any, error) { return fn() }
}

func (api *API) registerApp() {
	a := api.app

	// ---- Info (read only) ----
	api.Handle(Info, "env.info", noArgs(func() (any, error) {
		e := env.Info(a)
		return envJS{e.OS, e.Arch, e.OSName, e.OSVersion, e.Debug, e.WebView, e.WebViewVersion}, nil
	}))
	api.Handle(Info, "theme.get", noArgs(func() (any, error) {
		t, err := theme.System(a)
		if err != nil {
			return nil, err
		}
		return toTheme(t), nil
	}))
	api.Handle(Info, "screens.all", noArgs(func() (any, error) {
		ds, err := screen.All(a)
		if err != nil {
			return nil, err
		}
		return toDisplays(ds), nil
	}))
	api.Handle(Info, "screens.primary", noArgs(func() (any, error) {
		d, err := screen.Primary(a)
		if err != nil {
			return nil, err
		}
		return toDisplay(d), nil
	}))
	api.Handle(Info, "screens.current", windowHandler(func(s *tango.Surface, _ json.RawMessage) (any, error) {
		b := s.Bounds()
		d, err := screen.At(a, b.X+b.Width/2, b.Y+b.Height/2)
		if err != nil {
			return nil, err
		}
		return toDisplay(d), nil
	}))
	api.Handle(Info, "app.takePending", func(_ context.Context, _ *tango.Surface, args json.RawMessage) (any, error) {
		var kind string
		if err := decode(args, &kind); err != nil {
			return nil, err
		}
		return api.takePending(kind), nil
	})

	// ---- AppControl ----
	api.Handle(AppControl, "app.quit", noArgs(func() (any, error) {
		later(a.Quit)
		return nil, nil
	}))
	api.Handle(AppControl, "app.hide", noArgs(func() (any, error) { return nil, appctl.Hide(a) }))
	api.Handle(AppControl, "app.show", noArgs(func() (any, error) { return nil, appctl.Show(a) }))
	api.Handle(AppControl, "app.setBadge", func(_ context.Context, _ *tango.Surface, args json.RawMessage) (any, error) {
		var text string
		if err := decode(args, &text); err != nil {
			return nil, err
		}
		return nil, appctl.SetBadge(a, text)
	})
	api.Handle(AppControl, "app.setProgress", func(_ context.Context, _ *tango.Surface, args json.RawMessage) (any, error) {
		var p *float64
		if err := decode(args, &p); err != nil {
			return nil, err
		}
		v := -1.0
		if p != nil {
			v = *p
		}
		return nil, appctl.SetProgress(a, v)
	})
	api.Handle(AppControl, "app.requestAttention", func(_ context.Context, _ *tango.Surface, args json.RawMessage) (any, error) {
		var critical bool
		if err := decode(args, &critical); err != nil {
			return nil, err
		}
		return nil, appctl.RequestAttention(a, critical)
	})
}

// onAppEvent forwards app events to the pages (main thread).
func (api *API) onAppEvent(ev tango.AppEvent) {
	if !api.Allowed(Info) {
		return
	}
	switch ev.Kind {
	case tango.EventThemeChanged:
		api.Broadcast("theme.change", toTheme(ev.Theme))
	case tango.EventDisplaysChanged:
		ds, err := screen.All(api.app)
		if err != nil {
			return
		}
		api.Broadcast("screens.change", toDisplays(ds))
	case tango.EventOpenURL:
		api.launchEvent("openURL", map[string]string{"url": ev.URL})
	case tango.EventOpenFiles:
		paths := ev.Paths
		if paths == nil {
			paths = []string{}
		}
		api.launchEvent("openFiles", map[string]any{"paths": paths})
	case tango.EventReopen:
		api.launchEvent("reopen", nil)
	}
}

// launchEvent broadcasts an event that may arrive before any page exists
// (a deep link that launched the app); with no page it is kept for
// app.takePending, which the JS subscription calls.
func (api *API) launchEvent(kind string, data any) {
	if len(api.pages()) > 0 {
		api.Broadcast("app."+kind, data)
		return
	}
	api.mu.Lock()
	if len(api.pending) < maxPending {
		api.pending = append(api.pending, pendingEvent{kind, data})
	}
	api.mu.Unlock()
}

// takePending removes and returns the kept events of kind.
func (api *API) takePending(kind string) []any {
	api.mu.Lock()
	defer api.mu.Unlock()
	out := []any{}
	keep := api.pending[:0]
	for _, p := range api.pending {
		if p.kind == kind {
			out = append(out, p.data)
		} else {
			keep = append(keep, p)
		}
	}
	api.pending = keep
	return out
}
