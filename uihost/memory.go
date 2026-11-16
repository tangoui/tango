package uihost

// Low-memory warnings (docs/specs/mobile-canvas.md §3.4): on
// platform.AppMemoryWarning (iOS memory warning, Android onTrimMemory) the
// GPU and CPU renderers of every window drop what they can rebuild (glyph
// masks, image and shadow caches, the GPU atlas). Discovered font chains and
// shaped-text caches are emptied (text.TrimFontCaches; editors' LineCaches
// stay) and every window redraws. The subscription is made
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
	text.TrimFontCaches()
	memRunners.Range(func(k, _ any) bool {
		k.(*ui.Runner).TrimCaches()
		return true
	})
}

// trim releases the GPU renderer's rebuildable caches and the CPU buffer
// of an oversized frame. Main thread.
func (g *gpuRenderer) trim() {
	if g.closed {
		return
	}
	if g.cpu != nil {
		g.cpu.Trim()
		return
	}
	if t, ok := g.gpu.(trimmer); ok {
		t.Trim()
	}
	if !g.bigFrame {
		g.big, g.bigNoted = nil, false
	} else if g.big != nil {
		// The current oversized frame must remain presentable, but its
		// rebuildable masks must not keep old font faces alive.
		g.big.Trim()
	}
}

// Trim implements the runner-owned cache reclamation contract.
func (g *gpuRenderer) Trim() { g.trim() }
