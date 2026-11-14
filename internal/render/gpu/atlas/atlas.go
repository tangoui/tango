// Package atlas is the GPU renderer's persistent mask atlas: an R8
// texture holding coverage masks across frames — glyphs (KindGlyph) and
// filled paths (KindPath).
//
// The atlas is made of square pages of pageSize texels stacked vertically
// in one texture (Pixels is pageSize wide and pageSize*pages tall; M1 uses a
// single texture). Masks are shelf-packed into pages. When no page has room
// a new page is opened, up to maxPages; after that the least recently used
// page that was not used in the current frame (see BeginFrame) is emptied
// and reused. Eviction is per page, so it is cheap and never fragments.
// A page holds masks of one Kind only (it takes the kind of its first
// entry), so pages of short-lived path masks are evicted without the
// glyphs drawn every frame. Path masks hold at most maxPages-1 pages (none
// with a single page): paths new in every frame (each a new key) can then
// never pin every page for a frame, so a glyph always has a page it may
// use or evict.
//
// The package does not depend on internal/render/gpu: the renderer turns TakeDirty
// rectangles of Pixels into texture uploads. Atlas is not safe for
// concurrent use.
package atlas

import (
	"errors"
	"image"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

var (
	// ErrTooLarge is returned by Insert for a mask wider or taller than a
	// page.
	ErrTooLarge = errors.New("atlas: mask larger than a page")
	// ErrFull is returned by Insert when no page has room and every page
	// was used in the current frame, so none can be evicted.
	ErrFull = errors.New("atlas: all pages in use this frame")
)

// Kinds of masks (Key.Kind).
const (
	KindGlyph = 0 // a glyph: Src, ID, Size, Sub, Skew
	KindPath  = 1 // a filled path's coverage: Path, M, Size
)

// Key identifies a rasterized mask.
//
// For a glyph (Kind KindGlyph) Size is the device pixel size and Sub the
// sub-pixel x bucket, as passed to paint.CPU.GlyphMask; Path and M are
// zero. For a path (KindPath) Path is the paint.Path by pointer (its
// content must not change; see paint.FillPath), M the transform from its
// logical coordinates to device pixels and Size the device scale; the
// glyph fields are zero.
type Key struct {
	Kind uint8
	Src  paint.GlyphSource
	ID   uint32
	Size float32
	Sub  uint8
	Skew float32
	Path *paint.Path
	M    geom.Affine
}

// Entry locates a mask: Rect is in Pixels coordinates (texels of the whole
// atlas texture) and lies inside page Page, i.e. within rows
// [Page*pageSize, (Page+1)*pageSize). Off is the offset given to Insert:
// for a glyph the mask's top-left relative to the device pen position, for
// a path its device position. An empty mask's entry has an empty Rect
// (and Page -1): there is nothing to draw.
type Entry struct {
	Page int
	Rect image.Rectangle
	Off  image.Point
}

// gap is the spacing between entries in texels. The quad shader clamps
// samples half a texel inside each entry, so this is only a safety margin.
const gap = 1

type shelf struct{ y, h, x int } // page-local

type page struct {
	shelves []shelf
	bottom  int // first free row (page-local)
	used    uint32
	keys    []Key
	kind    uint8 // Kind of keys (meaningless while keys is empty)
}

// maxEmpty bounds the remembered empty masks; beyond it they are
// forgotten (and rasterized again when next drawn).
const maxEmpty = 4096

// glyphKey and pathKey are the map forms of a Key: float fields as their
// bits, so the maps hash plain memory (a Key hashes its float32 and
// geom.Affine fields one by one), and a glyph lookup — the hot path, one
// per glyph per frame — hashes only the glyph fields.
type glyphKey struct {
	src            paint.GlyphSource
	id, size, skew uint32
	sub, kind      uint8
}

type pathKey struct {
	path *paint.Path
	m    [6]uint32
	size uint32
}

func (k Key) glyphKey() glyphKey {
	return glyphKey{src: k.Src, id: k.ID, size: math.Float32bits(k.Size), skew: math.Float32bits(k.Skew), sub: k.Sub, kind: k.Kind}
}

func (k Key) pathKey() pathKey {
	m := k.M
	return pathKey{path: k.Path, size: math.Float32bits(k.Size), m: [6]uint32{math.Float32bits(m.A), math.Float32bits(m.B),
		math.Float32bits(m.C), math.Float32bits(m.D), math.Float32bits(m.E), math.Float32bits(m.F)}}
}

// get, set and del access the entry of k in the map of its kind.
func (a *Atlas) get(k Key) (Entry, bool) {
	if k.Kind == KindPath {
		e, ok := a.paths[k.pathKey()]
		return e, ok
	}
	e, ok := a.glyphs[k.glyphKey()]
	return e, ok
}

func (a *Atlas) set(k Key, e Entry) {
	if k.Kind == KindPath {
		a.paths[k.pathKey()] = e
	} else {
		a.glyphs[k.glyphKey()] = e
	}
}

func (a *Atlas) del(k Key) {
	if k.Kind == KindPath {
		delete(a.paths, k.pathKey())
	} else {
		delete(a.glyphs, k.glyphKey())
	}
}

// Atlas is a persistent glyph atlas. The zero value is not usable; call
// New.
type Atlas struct {
	size, maxPages int
	pages          []page
	glyphs         map[glyphKey]Entry // entries by Key (empty masks: Page -1)
	paths          map[pathKey]Entry
	empties        int // empty-mask entries in glyphs and paths
	pix            *image.Alpha
	gen            uint32
	dirty, spare   []image.Rectangle
	evictions      int
}

// New returns an empty atlas of one page of pageSize×pageSize texels that
// grows to at most maxPages pages.
func New(pageSize, maxPages int) *Atlas {
	a := &Atlas{size: max(1, pageSize), maxPages: max(1, maxPages), glyphs: map[glyphKey]Entry{}, paths: map[pathKey]Entry{}}
	a.Reset()
	return a
}

// BeginFrame starts a new frame for the LRU: pages used (looked up or
// inserted into) from now on are protected from eviction until the next
// BeginFrame.
func (a *Atlas) BeginFrame() { a.gen++ }

// Reset drops every entry and shrinks the atlas to one page; call it when
// the device scale changes. Pending dirty rectangles are dropped too: the
// caller re-uploads what it inserts afterwards.
func (a *Atlas) Reset() {
	clear(a.glyphs)
	clear(a.paths)
	a.empties = 0
	a.pages = append(a.pages[:0], page{used: a.gen})
	a.pix = image.NewAlpha(image.Rect(0, 0, a.size, a.size))
	a.dirty = a.dirty[:0]
}

// Evictions returns the number of pages emptied by LRU eviction since New
// (Reset does not count, nor reset it).
func (a *Atlas) Evictions() int { return a.evictions }

// PageSize returns the side of a page in texels: no mask wider or taller
// than this fits.
func (a *Atlas) PageSize() int { return a.size }

// Pixels returns the CPU copy of the whole atlas texture. Opening a page
// replaces the image (it grows by a page), so do not keep it across
// Insert calls.
func (a *Atlas) Pixels() *image.Alpha { return a.pix }

// emptyEntry is the entry of an empty mask.
var emptyEntry = Entry{Page: -1}

// Lookup returns the entry of key and marks its page used this frame. An
// empty mask inserted earlier is found too, with an empty Rect.
func (a *Atlas) Lookup(key Key) (Entry, bool) {
	e, ok := a.get(key)
	if ok && e.Page >= 0 {
		a.pages[e.Page].used = a.gen
	}
	return e, ok
}

// Insert copies mask into the atlas and returns its entry, with off
// recorded in Entry.Off; the page is marked used this frame and the
// written rectangle is reported by the next TakeDirty. Inserting a key
// already present returns its entry unchanged. An empty (or nil) mask
// takes no space: it is remembered (up to maxEmpty of them) so Lookup
// finds it, and its entry has an empty Rect.
// It fails with ErrTooLarge for a mask bigger than a page and with ErrFull
// when there is no room without evicting a page used this frame.
func (a *Atlas) Insert(key Key, mask *image.Alpha, off image.Point) (Entry, error) {
	if e, ok := a.Lookup(key); ok {
		return e, nil
	}
	var w, h int
	if mask != nil {
		w, h = mask.Rect.Dx(), mask.Rect.Dy()
	}
	if w <= 0 || h <= 0 {
		if a.empties >= maxEmpty {
			a.forgetEmpties()
		}
		a.set(key, emptyEntry)
		a.empties++
		return emptyEntry, nil // nothing to store or draw
	}
	if w > a.size || h > a.size {
		return Entry{}, ErrTooLarge
	}
	// May the mask take a page holding no path masks yet (an empty page, a
	// new one, or one evicted)? Not a path mask once paths hold
	// maxPages-1 pages.
	fresh := key.Kind != KindPath || a.pathPages() < a.maxPages-1
	pi, at, ok := -1, image.Point{}, false
	for i := range a.pages {
		p := &a.pages[i]
		if len(p.keys) > 0 && p.kind != key.Kind {
			continue // a page holds one kind of mask
		}
		if len(p.keys) == 0 && !fresh {
			continue
		}
		if at, ok = a.alloc(p, w, h); ok {
			pi = i
			break
		}
	}
	if !ok && fresh && len(a.pages) < a.maxPages {
		pi = a.grow()
		at, ok = a.alloc(&a.pages[pi], w, h)
	}
	if !ok {
		pi = a.victim(fresh)
		if pi < 0 {
			return Entry{}, ErrFull
		}
		a.evict(pi)
		at, ok = a.alloc(&a.pages[pi], w, h)
	}
	if !ok { // an empty page fits any mask that is not too large
		return Entry{}, ErrFull
	}
	r := image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}.Add(image.Pt(0, pi*a.size))
	for y := 0; y < h; y++ {
		copy(a.pix.Pix[a.pix.PixOffset(r.Min.X, r.Min.Y+y):][:w], mask.Pix[mask.PixOffset(mask.Rect.Min.X, mask.Rect.Min.Y+y):][:w])
	}
	e := Entry{Page: pi, Rect: r, Off: off}
	p := &a.pages[pi]
	p.used = a.gen
	p.kind = key.Kind
	p.keys = append(p.keys, key)
	a.set(key, e)
	a.dirty = append(a.dirty, r)
	return e, nil
}

// forgetEmpties drops every empty-mask entry.
func (a *Atlas) forgetEmpties() {
	for k, e := range a.glyphs {
		if e.Page < 0 {
			delete(a.glyphs, k)
		}
	}
	for k, e := range a.paths {
		if e.Page < 0 {
			delete(a.paths, k)
		}
	}
	a.empties = 0
}

// TakeDirty returns the Pixels rectangles written since the previous call.
// The slice is valid until the next TakeDirty.
func (a *Atlas) TakeDirty() []image.Rectangle {
	d := a.dirty
	a.dirty, a.spare = a.spare[:0], d
	return d
}

// alloc finds room for w×h in p: best fit among the existing shelves (the
// shortest shelf that is tall enough and has room left in its row; the
// first such shelf on a tie), else a new shelf opened at the bottom. Shelf
// heights are rounded up to a multiple of 4 so glyphs of one font size
// share shelves.
func (a *Atlas) alloc(p *page, w, h int) (image.Point, bool) {
	best := -1
	for i, s := range p.shelves {
		if h <= s.h && s.x+w <= a.size && (best < 0 || s.h < p.shelves[best].h) {
			best = i
		}
	}
	if best >= 0 {
		s := &p.shelves[best]
		at := image.Pt(s.x, s.y)
		s.x += w + gap
		return at, true
	}
	sh := (h + 3) &^ 3
	if p.bottom+sh > a.size {
		sh = h
	}
	if p.bottom+sh > a.size || w > a.size {
		return image.Point{}, false
	}
	p.shelves = append(p.shelves, shelf{y: p.bottom, h: sh, x: w + gap})
	at := image.Pt(0, p.bottom)
	p.bottom += sh + gap
	return at, true
}

// grow opens a new page at the bottom of the texture and returns its index.
func (a *Atlas) grow() int {
	n := len(a.pages)
	pix := image.NewAlpha(image.Rect(0, 0, a.size, a.size*(n+1)))
	copy(pix.Pix, a.pix.Pix)
	a.pix = pix
	a.pages = append(a.pages, page{used: a.gen})
	return n
}

// pathPages returns the number of pages holding path masks.
func (a *Atlas) pathPages() int {
	n := 0
	for _, p := range a.pages {
		if len(p.keys) > 0 && p.kind == KindPath {
			n++
		}
	}
	return n
}

// victim returns the least recently used page not used this frame, or -1;
// with any false, only a page holding path masks.
func (a *Atlas) victim(any bool) int {
	v := -1
	for i, p := range a.pages {
		if !any && (len(p.keys) == 0 || p.kind != KindPath) {
			continue
		}
		if p.used != a.gen && (v < 0 || a.gen-p.used > a.gen-a.pages[v].used) {
			v = i
		}
	}
	return v
}

// evict empties page i. Its texels are left as they are: nothing samples
// them until new entries overwrite (and re-upload) their rectangles.
func (a *Atlas) evict(i int) {
	a.evictions++
	p := &a.pages[i]
	for _, k := range p.keys {
		a.del(k)
	}
	clear(p.keys)
	p.keys = p.keys[:0]
	p.shelves = p.shelves[:0]
	p.bottom = 0
}
