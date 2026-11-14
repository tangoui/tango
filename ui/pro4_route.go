package ui

import (
	"github.com/tangoui/tango/ui/layout"
)

// ---------------------------------------------------------------------------
// RouteContext and GridContent (Ant Design Pro RouteContext / GridContent)
//
//	ui.ProLayout(c, cfg, func() {
//		rc := ui.RouteContext(c) // layout, collapsed, breadcrumb, content width…
//		ui.GridContent(c).Body(page) // 1200px centred when cfg.ContentWidth is "Fixed"
//	})
//
// RouteContext reads what the enclosing ProLayout knows about the shell
// (Ant's React context of the same name); outside a ProLayout InLayout is
// false. GridContent is the content container that follows the shell's
// content width: Fluid fills the area, Fixed centres it at 1200px.

// RouteInfo is the shell state seen by the content of a ProLayout.
type RouteInfo struct {
	InLayout     bool     // built inside a ProLayout content area
	Layout       string   // "side", "top" or "mix"
	Collapsed    bool     // sider collapsed
	IsMobile     bool     // window narrower than MD
	HasSiderMenu bool     // side and mix layouts
	Breadcrumb   []string // the shell's breadcrumb
	PageTitle    string
	Selected     string // selected menu key
	ContentWidth string // "Fluid" or "Fixed"
	FixedHeader  bool
}

// RouteContext returns the state of the innermost ProLayout being built
// around the caller.
func RouteContext(c *Context) RouteInfo {
	if pc := proLayoutContext(c); pc.depth > 0 {
		return pc.info
	}
	return RouteInfo{}
}

// GridContentFixedWidth is the content width of a Fixed GridContent.
const GridContentFixedWidth = 1200

// GridContentW builds a GridContent.
type GridContentW struct {
	_ [0]func()

	c     *Context
	width string
	max   float32
	opts  []layout.Option
}

// GridContent starts a content container; chain options, then Body.
//
// ConfigProvider.Defaults("GridContent", ...) keys: "contentWidth"
// ("Fluid" | "Fixed"). Without either, the enclosing ProLayout's content
// width is used.
func GridContent(c *Context) *GridContentW {
	g := &GridContentW{c: c, max: GridContentFixedWidth}
	g.width = Prop(c, "GridContent", "contentWidth", "")
	if g.width == "" {
		g.width = RouteContext(c).ContentWidth
	}
	return g
}

// ContentWidth sets "Fluid" (fill) or "Fixed" (centred, MaxWidth).
func (g *GridContentW) ContentWidth(w string) *GridContentW { g.width = w; return g }

// MaxWidth sets the width of a Fixed container (default 1200).
func (g *GridContentW) MaxWidth(px float32) *GridContentW { g.max = px; return g }

// Layout applies layout options to the inner column.
func (g *GridContentW) Layout(o ...layout.Option) *GridContentW {
	g.opts = append(g.opts, o...)
	return g
}

// Fixed reports whether the container centres its content.
func (g *GridContentW) Fixed() bool { return g.width == "Fixed" }

// Body builds fn inside the container and returns the inner column.
func (g *GridContentW) Body(fn func()) *Box {
	c := g.c
	if !g.Fixed() {
		return Column(c, fn, append([]layout.Option{layout.MinW(0)}, g.opts...)...)
	}
	var inner *Box
	Row(c, func() {
		inner = Column(c, fn, append([]layout.Option{layout.Grow(1), layout.MinW(0), layout.MaxW(g.max)}, g.opts...)...)
	}, layout.Center(), layout.MinW(0))
	return inner
}
