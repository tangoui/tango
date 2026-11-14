package fake

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Embedded WebViews (platform.WebViewEmbedder, docs/specs/webview-embed.md).

var _ platform.WebViewEmbedder = (*Backend)(nil)

// embedErrs holds the SetEmbedErr errors.
var embedErrs sync.Map // *Backend → error

// SetEmbedErr makes NewEmbeddedWebView fail with err (nil restores).
func (b *Backend) SetEmbedErr(err error) {
	if err == nil {
		embedErrs.Delete(b)
		return
	}
	embedErrs.Store(b, err)
}

// Embedded is a fake embedded WebView. It reuses Surface for the page
// side (navigations, evals, WebView extensions) and records placements
// and focus calls.
type Embedded struct {
	*Surface
	parent platform.SurfaceID

	emu        sync.Mutex
	placements []platform.WebPlacement
	focusWeb   int
	blurWeb    int
	hidden     bool
}

var embeds struct {
	mu sync.Mutex
	m  map[*Backend][]*Embedded
}

// NewEmbeddedWebView implements platform.WebViewEmbedder.
func (b *Backend) NewEmbeddedWebView(id, parent platform.SurfaceID, opts platform.SurfaceOptions, web platform.WebOptions) (platform.EmbeddedWebView, error) {
	b.mustMain("NewEmbeddedWebView")
	if v, ok := embedErrs.Load(b); ok {
		return nil, v.(error)
	}
	p := b.Surface(parent)
	if p == nil || p.Closed() {
		return nil, errors.New("fake: no such parent")
	}
	opts.Visible = false
	ns, err := b.NewSurface(id, opts)
	if err != nil {
		return nil, err
	}
	e := &Embedded{Surface: ns.(*Surface), parent: parent}
	embeds.mu.Lock()
	if embeds.m == nil {
		embeds.m = map[*Backend][]*Embedded{}
	}
	embeds.m[b] = append(embeds.m[b], e)
	embeds.mu.Unlock()
	return e, nil
}

// Embedded returns the embedded WebView with id, or nil.
func (b *Backend) Embedded(id platform.SurfaceID) *Embedded {
	embeds.mu.Lock()
	defer embeds.mu.Unlock()
	for _, e := range embeds.m[b] {
		if e.id == id {
			return e
		}
	}
	return nil
}

// closeEmbedded closes the embedded views of parent (Surface.Close).
func (b *Backend) closeEmbedded(parent platform.SurfaceID) {
	embeds.mu.Lock()
	var kids []*Embedded
	for _, e := range embeds.m[b] {
		if e.parent == parent {
			kids = append(kids, e)
		}
	}
	embeds.mu.Unlock()
	for _, e := range kids {
		e.Close()
	}
}

func (e *Embedded) SetPlacement(p platform.WebPlacement) {
	e.b.mustMain("SetPlacement")
	e.emu.Lock()
	e.placements = append(e.placements, p)
	e.emu.Unlock()
}

func (e *Embedded) FocusWeb() {
	e.b.mustMain("FocusWeb")
	e.emu.Lock()
	e.focusWeb++
	e.emu.Unlock()
}

func (e *Embedded) BlurWeb() {
	e.b.mustMain("BlurWeb")
	e.emu.Lock()
	e.blurWeb++
	e.emu.Unlock()
}

func (e *Embedded) Show() { e.emu.Lock(); e.hidden = false; e.emu.Unlock() }
func (e *Embedded) Hide() { e.emu.Lock(); e.hidden = true; e.emu.Unlock() }

// Placement returns the last placement (zero before the first).
func (e *Embedded) Placement() platform.WebPlacement {
	e.emu.Lock()
	defer e.emu.Unlock()
	if len(e.placements) == 0 {
		return platform.WebPlacement{}
	}
	return e.placements[len(e.placements)-1]
}

// FocusCalls returns how often FocusWeb and BlurWeb were called.
func (e *Embedded) FocusCalls() (focus, blur int) {
	e.emu.Lock()
	defer e.emu.Unlock()
	return e.focusWeb, e.blurWeb
}

// Parent returns the parent surface's id.
func (e *Embedded) Parent() platform.SurfaceID { return e.parent }

// EmitFocus reports the page gaining or losing keyboard focus.
func (b *Backend) EmitFocus(id platform.SurfaceID, focused bool) {
	b.Do(func() {
		k := platform.SurfaceBlurred
		if focused {
			k = platform.SurfaceFocused
		}
		b.h.OnSurfaceEvent(id, platform.SurfaceEvent{Kind: k})
	})
}

// EmitMessage delivers a page message from id with the asset origin.
func (b *Backend) EmitMessage(id platform.SurfaceID, msg string) {
	b.Do(func() { b.h.OnMessage(id, b.AssetOrigin(), msg) })
}

// Screenshot returns a small decodable PNG (Surface.Screenshot returns
// FakePNG, which is not one).
func (e *Embedded) Screenshot(done func([]byte, error)) {
	e.b.mustMain("Screenshot")
	e.b.enqueue(func() { done(snapshotPNG(), nil) })
}

func snapshotPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range 16 {
		img.Set(i%4, i/4, color.NRGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// Size reports the frame size of the last placement.
func (e *Embedded) Size() (int, int) { return e.Placement().Size() }
