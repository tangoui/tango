package windows

// Platform-independent parts of WebViews embedded in a canvas surface
// (webembed_windows.go): what a placement change means for the clip
// container window and for the WebView2 controller, and where keyboard
// focus goes when the view is hidden or the window is activated again.
// Kept free of build tags so they are tested on every OS.

import (
	"image"

	"github.com/tangoui/tango/internal/platform"
)

// embedScale is the device-pixel scale of a window at dpi.
func embedScale(dpi int) float64 { return float64(normDPI(dpi)) / baseDPI }

// embedOps are the native calls that bring one target (the container
// window or the controller) from the geometry it has to the one wanted.
type embedOps struct {
	Show bool            // make it visible (after Move)
	Hide bool            // make it invisible
	Move bool            // set its rectangle to Rect
	Rect image.Rectangle // container: Clip in the parent; controller: Frame in the container
}

// None reports whether nothing has to be done.
func (o embedOps) None() bool { return !o.Show && !o.Hide && !o.Move }

// embedTarget is the geometry last applied to one target. known is false
// before anything was applied (a new window or controller): the first
// update then always sets the rectangle and the visibility.
type embedTarget struct {
	known   bool
	visible bool
	rect    image.Rectangle
}

// diff returns the calls that move t to visibility visible and rectangle
// r and records them as applied. A hidden target keeps its rectangle (it
// is not moved while hidden; showing it moves it first).
func (t *embedTarget) diff(visible bool, r image.Rectangle) embedOps {
	var o embedOps
	switch {
	case visible:
		if !t.known || !t.visible || t.rect != r {
			o.Move, o.Rect = true, r
			t.rect = r
		}
		if !t.known || !t.visible {
			o.Show = true
		}
	case !t.known || t.visible:
		o.Hide = true
	}
	t.known, t.visible = true, visible
	return o
}

// embedPlan is the placement state of one embedded WebView: the last
// placement asked for, the application's hidden flag and what was applied
// to the container window and to the controller.
type embedPlan struct {
	placement platform.WebPlacement
	placed    bool // SetPlacement was called at least once
	hidden    bool // Hide (application level)

	container embedTarget
	ctl       embedTarget
}

// geometry is the wanted geometry at scale: nothing shows before the
// first placement, while hidden or with an empty clip.
func (p *embedPlan) geometry(scale float64) platform.EmbedGeometry {
	if !p.placed {
		return platform.EmbedGeometry{}
	}
	return p.placement.Geometry(scale, p.hidden)
}

// sync computes the container and controller calls for the current state
// at scale. hasCtl is false while the controller does not exist yet: its
// state stays unknown, so its first sync applies everything.
func (p *embedPlan) sync(scale float64, hasCtl bool) (container, ctl embedOps) {
	g := p.geometry(scale)
	container = p.container.diff(g.Visible, g.Clip)
	if hasCtl {
		ctl = p.ctl.diff(g.Visible, g.Frame)
	}
	return container, ctl
}

// place records a new placement.
func (p *embedPlan) place(pl platform.WebPlacement) {
	p.placement, p.placed = pl, true
}

// controllerReset forgets what was applied to the controller (it was just
// created).
func (p *embedPlan) controllerReset() { p.ctl = embedTarget{} }

// visible reports whether the container was last made visible.
func (p *embedPlan) visible() bool { return p.container.known && p.container.visible }

// size is Surface.Size: the frame of the last placement, rounded.
func (p *embedPlan) size() (int, int) { return p.placement.Size() }

// embedBlurOnHide reports whether hiding a view must first move keyboard
// focus out of it: a hidden window keeps the focus otherwise, and keys
// would go to an invisible page.
func embedBlurOnHide(ops embedOps, focused bool) bool { return ops.Hide && focused }
