package uihost

// Low-memory warnings (docs/specs/mobile-canvas.md §3.4): on
// platform.AppMemoryWarning (iOS memory warning, Android onTrimMemory) the
// GPU renderers of every window drop what they can rebuild (gpu.Renderer
// Trim: the glyph atlas and image textures), the shaped-text caches are
// emptied (text.TrimCaches; editors' LineCaches stay) and every window
// draws a frame, which re-uploads what it shows. The subscription is made
// once per app, by the first Open or Attach.

import (
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/text"
)

// trimmer is implemented by GPU renderers that can release caches
// (gpu.Renderer and the presenters' renderers embedding it).
type trimmer interface{ Trim() }

var memWatch sync.Map // *tango.App -> struct{}: subscribed

// memRunners are the open runners, for the frame that follows a trim.
var memRunners sync.Map // *ui.Runner -> struct{}

// watchMemory subscribes app to memory warnings (once) and registers r;
// host.close unregisters it. Main thread.
func watchMemory(app *tango.App, r *ui.Runner) {
	if app != nil {
		if _, loaded := memWatch.LoadOrStore(app, struct{}{}); !loaded {
			app.OnEventKind(platform.AppMemoryWarning, func(tango.AppEvent) { trimMemory() })
		}
	}
	memRunners.Store(r, struct{}{})
}

// trimMemory runs on the main thread after AppMemoryWarning.
func trimMemory() {
	renderers.Range(func(_, v any) bool {
		v.(*gpuRenderer).trim()
		return true
	})
	text.TrimCaches()
	memRunners.Range(func(k, _ any) bool {
		k.(*ui.Runner).Invalidate()
		return true
	})
}

// trim releases the GPU renderer's rebuildable caches and the CPU buffer
// of an oversized frame. Main thread.
func (g *gpuRenderer) trim() {
	if g.closed || g.cpu != nil {
		return
	}
	if t, ok := g.gpu.(trimmer); ok {
		t.Trim()
	}
	if !g.bigFrame {
		g.big, g.bigNoted = nil, false
	}
}
