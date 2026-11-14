package linux

// Platform-independent part of web views embedded in a canvas surface
// (platform.WebViewEmbedder, webembed_linux.go): which native calls a new
// placement needs. Kept free of build tags so it is tested on every OS.

import (
	"image"

	"github.com/tangoui/tango/internal/platform"
)

// embedStep is what one placement update has to do to the clip container
// (a GtkLayout overlay child) and the web view inside it.
type embedStep struct {
	Show, Hide bool // the container's visibility changes
	Clip       bool // the container rectangle changed: re-run get-child-position
	Frame      bool // the web view's offset or size inside the container changed
}

// none reports whether the update needs no native call.
func (st embedStep) none() bool { return !st.Show && !st.Hide && !st.Clip && !st.Frame }

// embedTracker remembers what has been applied to the native widgets.
// While hidden, the last visible geometry is kept, so showing the view
// again at the same place only shows it.
type embedTracker struct {
	shown   bool            // the container is shown
	applied bool            // clip and frame hold a geometry applied before
	clip    image.Rectangle // container rectangle, relative to the parent's content
	frame   image.Rectangle // web view rectangle, relative to the container
}

// update records g (from WebPlacement.Geometry) and returns the steps that
// bring the widgets to it.
func (t *embedTracker) update(g platform.EmbedGeometry) embedStep {
	var st embedStep
	if !g.Visible {
		if t.shown {
			t.shown = false
			st.Hide = true
		}
		return st
	}
	if !t.applied || g.Clip != t.clip {
		st.Clip = true
	}
	if !t.applied || g.Frame != t.frame {
		st.Frame = true
	}
	t.applied, t.clip, t.frame = true, g.Clip, g.Frame
	if !t.shown {
		t.shown = true
		st.Show = true
	}
	return st
}
