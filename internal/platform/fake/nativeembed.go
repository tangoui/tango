package fake

import (
	"errors"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Embedded app native views (platform.NativeViewEmbedder).

var _ platform.NativeViewEmbedder = (*Backend)(nil)

var nativeEmbedErrs sync.Map // *Backend → error

// SetNativeEmbedErr makes NewEmbeddedNativeView fail with err (nil
// restores).
func (b *Backend) SetNativeEmbedErr(err error) {
	if err == nil {
		nativeEmbedErrs.Delete(b)
		return
	}
	nativeEmbedErrs.Store(b, err)
}

// NativeView is a fake embedded native view: it records its placements.
type NativeView struct {
	b      *Backend
	parent platform.SurfaceID
	view   uintptr
	opts   platform.NativeViewOptions
	closed func()

	mu         sync.Mutex
	placements []platform.WebPlacement
	removed    bool
}

var natives struct {
	mu sync.Mutex
	m  map[*Backend][]*NativeView
}

// NewEmbeddedNativeView implements platform.NativeViewEmbedder.
func (b *Backend) NewEmbeddedNativeView(parent platform.SurfaceID, view uintptr, opts platform.NativeViewOptions, closed func()) (platform.EmbeddedNativeView, error) {
	b.mustMain("NewEmbeddedNativeView")
	if v, ok := nativeEmbedErrs.Load(b); ok {
		return nil, v.(error)
	}
	p := b.Surface(parent)
	if p == nil || p.Closed() {
		return nil, errors.New("fake: no such parent")
	}
	n := &NativeView{b: b, parent: parent, view: view, opts: opts, closed: closed}
	natives.mu.Lock()
	if natives.m == nil {
		natives.m = map[*Backend][]*NativeView{}
	}
	natives.m[b] = append(natives.m[b], n)
	natives.mu.Unlock()
	return n, nil
}

// NativeViews returns the native views ever embedded in parent, oldest
// first, removed ones included.
func (b *Backend) NativeViews(parent platform.SurfaceID) []*NativeView {
	natives.mu.Lock()
	defer natives.mu.Unlock()
	var out []*NativeView
	for _, n := range natives.m[b] {
		if n.parent == parent {
			out = append(out, n)
		}
	}
	return out
}

// closeNatives removes the native views of a closing parent and reports
// it to their owners (Surface.Close).
func (b *Backend) closeNatives(parent platform.SurfaceID) {
	for _, n := range b.NativeViews(parent) {
		n.mu.Lock()
		was := n.removed
		n.removed = true
		n.mu.Unlock()
		if !was && n.closed != nil {
			n.closed()
		}
	}
}

func (n *NativeView) SetPlacement(p platform.WebPlacement) {
	n.b.mustMain("SetPlacement")
	n.mu.Lock()
	n.placements = append(n.placements, p)
	n.mu.Unlock()
}

func (n *NativeView) Close() {
	n.b.mustMain("Close")
	n.mu.Lock()
	n.removed = true
	n.mu.Unlock()
}

// View returns the application's view handle.
func (n *NativeView) View() uintptr { return n.view }

// Options returns the creation options.
func (n *NativeView) Options() platform.NativeViewOptions { return n.opts }

// Removed reports whether the view was closed or removed with its parent.
func (n *NativeView) Removed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.removed
}

// Placement returns the last placement (zero before the first).
func (n *NativeView) Placement() platform.WebPlacement {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.placements) == 0 {
		return platform.WebPlacement{}
	}
	return n.placements[len(n.placements)-1]
}
