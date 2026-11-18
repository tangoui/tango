// Package quartz plans retained native layers for macOS windows. Other systems
// can exercise its display-list semantics without loading native graphics APIs.
package quartz

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

type itemKind uint8

const (
	itemSolid itemKind = iota
	itemText
	itemPatch
)

type textPart struct {
	frame image.Rectangle // local top-left device pixels inside the item
	color color.NRGBA
	runs  []nativeglyph.Run
}

type item struct {
	kind  itemKind
	key   string
	frame image.Rectangle
	color color.NRGBA
	runs  []nativeglyph.Run
	texts []textPart
	op    paint.Op
	clip  image.Rectangle
	refs  []any
	image *image.RGBA
	// bulkOps: a window raster of these text ops, each with its own clip
	// (see bulkText). The item is a patch keyed by frame serial.
	bulkOps []bulkOp
	parts   []bulkOp // coalesced text: each part's source op and clip
	hash    uint64   // text identity for bulk change detection
}

type bulkOp struct {
	op   paint.Text
	src  paint.Op // op as found in the display list (already boxed)
	clip image.Rectangle
}

type scene struct {
	provider           nativeglyph.Provider
	cpu                *paint.CPU
	items              []item
	ops                []paint.Op
	bounds             image.Rectangle
	background         color.NRGBA
	scale              float32
	full               bool
	serial, imageEpoch uint64
	identitySeq        uint64

	// Bulk text (bulkText): previous frame's text identities and the mode.
	bulk        bool
	bulkMerged  bool // this frame's text was merged into one bulk raster
	bulkOff     bool // tests: never merge (the per-layer reference)
	calm        int
	prevText    map[uint64]int
	nextText    map[uint64]int
	prevLazy    bool // prevText holds lazy (op) hashes, not native-key hashes
	replan      bool // re-planning a lazy bulk frame that could not merge
	bulkScratch []paint.Op
	keepScratch []item
	opsScratch  []bulkOp
	inkScratch  []image.Rectangle
	inkTiles    map[image.Point][]int32
	keys        map[string]string // interned solid keys (few distinct)
	kbuf        keyData
}

const bulkCalmFrames = 30

func newScene(provider nativeglyph.Provider) *scene {
	return &scene{provider: provider, cpu: paint.NewCPU()}
}

func (s *scene) plan(l *paint.List, size geom.Size, scale float32) error {
	if l == nil {
		return fmt.Errorf("quartz: nil display list")
	}
	if !finite(scale) || scale <= 0 {
		scale = 1
	}
	if !finite(size.W) || !finite(size.H) || size.W > 1<<24 || size.H > 1<<24 {
		return fmt.Errorf("quartz: invalid frame size")
	}
	s.serial++
	s.identitySeq = 0
	s.scale = scale
	s.background = l.Background
	s.full = false
	clear(s.items)
	s.items = s.items[:0]
	clear(s.ops)
	s.ops = s.ops[:0]
	w, h := int(math.Ceil(float64(max(0, size.W)*scale))), int(math.Ceil(float64(max(0, size.H)*scale)))
	s.bounds = image.Rect(0, 0, w, h)
	if s.bounds.Empty() {
		return nil
	}
	for _, op := range l.Ops {
		if op == nil {
			continue
		}
		v := reflect.ValueOf(op)
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				continue
			}
			var ok bool
			op, ok = v.Elem().Interface().(paint.Op)
			if !ok {
				return fmt.Errorf("quartz: invalid drawing pointer %T", op)
			}
		}
		s.ops = append(s.ops, op)
		switch o := op.(type) {
		case paint.PushTransform, paint.PushOpacity, paint.BackdropBlur:
			s.full = true
		case paint.PushClip:
			if !o.Radii.IsZero() {
				s.full = true
			}
		case paint.FillRect, paint.StrokeRect, paint.ShadowRect, paint.FillPath, paint.LinearGradient, paint.Gradient, paint.Checker, paint.Text, paint.Image, paint.Pop, paint.Damage, paint.ScrollRegion:
		default:
			return fmt.Errorf("quartz: unknown drawing command %T", op)
		}

	}
	if s.full {
		return s.fullFrame(l, size)
	}

	s.cpu.BeginFrame(scale)
	lazy := s.bulk && !s.replan && !s.bulkOff
	clip := s.bounds
	var clips []image.Rectangle
	for _, op := range s.ops {
		switch o := op.(type) {
		case paint.PushClip:
			clips = append(clips, clip)
			clip = clip.Intersect(clipBounds(o.Rect, scale))
			continue
		case paint.Pop:
			if len(clips) > 0 {
				clip = clips[len(clips)-1]
				clips = clips[:len(clips)-1]
			}
			continue
		case paint.Damage, paint.ScrollRegion:
			continue
		}
		if clip.Empty() {
			continue
		}
		if text, ok := op.(paint.Text); ok {
			if lazy {
				// Bulk frames draw text in Tango's rasterizer: native glyph
				// runs, bounds and layer keys would be built and discarded.
				if text.Color.A != 0 {
					s.items = append(s.items, item{kind: itemText, op: op, clip: clip, hash: s.textHash(text)})
				}
				continue
			}
			if e := s.textItem(text, clip); e != nil {
				return e
			}
			continue
		}
		frame, visible := s.opBounds(op)
		if !visible {
			continue
		}
		frame = frame.Intersect(clip).Intersect(s.bounds)
		if frame.Empty() {
			continue
		}
		if fill, ok := op.(paint.FillRect); ok && fill.Radii.IsZero() && integralRect(fill.Rect, scale) {
			k := s.kbuf[:0]
			k.tag(byte(itemSolid))
			k.rgba(fill.Color)
			k.i(frame.Dx())
			k.i(frame.Dy())
			s.kbuf = k
			s.items = append(s.items, item{kind: itemSolid, key: s.intern(k), frame: frame, color: fill.Color})
			continue
		}
		k, refs, e := s.patchKey(op, frame, clip)
		if e != nil {
			return e
		}
		s.items = append(s.items, item{kind: itemPatch, key: k, frame: frame, op: op, clip: clip, refs: refs})
	}
	if !lazy {
		s.coalesceText()
	}
	if s.replan {
		s.bulkMerged = false
	} else {
		s.bulkText(lazy)
	}
	if lazy && !s.bulkMerged {
		// Bulk mode ended or text cannot be lifted: plan native text layers.
		// Change tracking already counted this frame.
		s.replan = true
		defer func() { s.replan = false }()
		return s.plan(l, size, scale)
	}
	// Text backing stores and immutable image snapshots must stay bounded even
	// when a display list stacks many large, partially transparent contents.
	// A complete software frame preserves ordering and alpha within one bitmap.
	var bitmapBytes uint64
	for _, it := range s.items {
		if it.kind == itemSolid {
			continue
		}
		bytes := uint64(it.frame.Dx()) * uint64(it.frame.Dy()) * 4
		if it.kind == itemText {
			bytes *= 4 // keepTextBacking caps retained text capacity at this ratio
		}
		bitmapBytes += bytes
		if bitmapBytes > 64<<20 {
			return s.fullFrame(l, size)
		}
	}

	return nil
}
func (s *scene) fullFrame(l *paint.List, size geom.Size) error {
	copyList := *l
	copyList.Ops = s.ops
	if err := s.cpu.Render(&copyList, size, s.scale); err != nil {
		return err
	}
	s.full = true
	clear(s.items)
	s.items = s.items[:0]
	k := keyData{}
	k.tag(250)
	k.u64(s.serial)
	s.items = append(s.items, item{kind: itemPatch, key: string(k), frame: s.bounds, image: s.cpu.Image(), clip: s.bounds})
	return nil
}

func finite(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
func rectBounds(r geom.Rect, s float32) image.Rectangle {
	return image.Rect(int(math.Floor(float64(r.X)*float64(s))), int(math.Floor(float64(r.Y)*float64(s))), int(math.Ceil(float64(r.Right())*float64(s))), int(math.Ceil(float64(r.Bottom())*float64(s))))
}
func clipBounds(r geom.Rect, s float32) image.Rectangle {
	return image.Rect(int(math.Round(float64(r.X*s))), int(math.Round(float64(r.Y*s))), int(math.Round(float64(r.Right()*s))), int(math.Round(float64(r.Bottom()*s))))
}
func integralRect(r geom.Rect, s float32) bool {
	for _, v := range []float32{r.X, r.Y, r.Right(), r.Bottom()} {
		x := float64(v) * float64(s)
		if x != math.Round(x) {
			return false
		}
	}
	return true
}
func (s *scene) opBounds(op paint.Op) (image.Rectangle, bool) {
	var b image.Rectangle
	switch o := op.(type) {
	case paint.FillRect:
		if o.Rect.Empty() || o.Color.A == 0 {
			return b, false
		}
		b = rectBounds(o.Rect, s.scale)
	case paint.StrokeRect:
		if o.Color.A == 0 || o.Width <= 0 {
			return b, false
		}
		b = rectBounds(o.Rect, s.scale)
	case paint.ShadowRect:
		if o.Shadow.Color.A == 0 {
			return b, false
		}
		g, blur := paint.ShadowPlacement(o, s.scale)
		if blur {
			b = g.Rect.Inset(-g.Margin)
		} else {
			b = rectBounds(g.Shape, s.scale)
		}
	case paint.FillPath:
		if o.Path == nil || o.Color.A == 0 {
			return b, false
		}
		b = rectBounds(o.Path.Bounds(), s.scale).Inset(-1)
	case paint.LinearGradient:
		b = rectBounds(o.Rect, s.scale)
	case paint.Gradient:
		if o.Path != nil {
			b = rectBounds(o.Path.Bounds(), s.scale).Inset(-1)
		} else {
			b = rectBounds(o.Rect, s.scale)
		}
	case paint.Checker:
		b = rectBounds(o.Rect, s.scale)
	case paint.Image:
		if o.Img == nil {
			return b, false
		}
		b = rectBounds(o.Rect, s.scale)
	default:
		return b, false
	}
	return b, !b.Empty()
}
func (s *scene) pixels(it item) *image.RGBA {
	if it.image != nil {
		return it.image
	}
	if it.bulkOps != nil {
		return s.cpu.RenderRegion(s.bulkList(it), it.frame, s.scale)
	}
	// Reconstruct the integer device clip as logical geometry; the reference
	// renderer rounds its rectangular clips back to these exact device pixels.
	c := it.clip
	f := s.scale
	cr := geom.Rect{X: float32(c.Min.X) / f, Y: float32(c.Min.Y) / f, W: float32(c.Dx()) / f, H: float32(c.Dy()) / f}
	return s.cpu.RenderRegion([]paint.Op{paint.PushClip{Rect: cr}, it.op, paint.Pop{}}, it.frame, s.scale)
}

var popOp paint.Op = paint.Pop{}

// bulkList is the bulk raster's drawing: each text op inside its own clip.
func (s *scene) bulkList(it item) []paint.Op {
	f := s.scale
	ops := s.bulkScratch[:0]
	for _, b := range it.bulkOps {
		src := b.src
		if src == nil {
			src = b.op
		}
		c := b.clip
		if c.Intersect(it.frame) == it.frame {
			ops = append(ops, src) // the raster's own bounds already clip
			continue
		}
		cr := geom.Rect{X: float32(c.Min.X) / f, Y: float32(c.Min.Y) / f, W: float32(c.Dx()) / f, H: float32(c.Dy()) / f}
		ops = append(ops, paint.PushClip{Rect: cr}, src, popOp)
	}
	s.bulkScratch = ops
	return ops
}

func (s *scene) releaseScratch() {
	clear(s.bulkScratch)
	s.bulkScratch = s.bulkScratch[:0]
	clear(s.opsScratch) // drop the frame's glyph slices
	s.opsScratch = s.opsScratch[:0]
	clear(s.items)
	s.items = s.items[:0]
	clear(s.ops)
	s.ops = s.ops[:0]
}
func (s *scene) trim() {
	s.cpu = paint.NewCPU()
	s.imageEpoch++
	s.releaseScratch()
}
func (s *scene) invalidateImage(img image.Image) { s.cpu.InvalidateImage(img); s.imageEpoch++ }

// Small ink changes (especially changing digits) must not repeatedly resize a
// CALayer's backing store. Retain capacity in local coordinates, so scrolling
// only moves the layer. Large replacements shrink instead of keeping a peak.
func keepTextBacking(previous, need image.Point) image.Point {
	kept := image.Pt(max(previous.X, need.X), max(previous.Y, need.Y))
	if uint64(kept.X)*uint64(kept.Y) > 4*uint64(need.X)*uint64(need.Y) {
		return need
	}
	return kept
}

// Thresholds for bulk text. A frame whose changed text items reach
// bulkMinChanged (and at least half of all text items) is a dashboard
// updating in place: every changed label would otherwise re-render its own
// CALayer backing store, one Objective-C round trip and one bitmap upload per
// label. Bulk mode draws all text into one window-sized layer instead.
const (
	bulkMinChanged = 32
	bulkMinText    = 32
)

// bulkText merges every text item into one retained layer, drawn last, when
// many labels change every frame. It only does so when no later non-text item
// overlaps a text frame, so lifting text above the remaining items cannot
// change any pixel. It leaves bulk mode after bulkCalmFrames frames in which
// few labels changed, returning to per-label layers (which move without
// redrawing).
func (s *scene) bulkText(lazy bool) {
	if s.nextText == nil {
		s.nextText = map[uint64]int{}
	}
	// prevText stays nil until the first frame has been recorded. Lazy and
	// native hashes describe the same text differently: a frame that switches
	// between them has nothing to compare with either.
	clear(s.nextText)
	texts, changed := 0, 0
	compared := s.prevText != nil && s.prevLazy == lazy
	for _, it := range s.items {
		if it.kind != itemText {
			continue
		}
		texts++
		s.nextText[it.hash]++
		if s.prevText[it.hash] > 0 {
			s.prevText[it.hash]--
		} else {
			changed++
		}
	}
	s.prevText, s.nextText = s.nextText, s.prevText
	s.prevLazy = lazy
	busy := compared && texts >= bulkMinText && changed >= bulkMinChanged && 2*changed >= texts
	switch {
	case busy:
		s.bulk, s.calm = true, 0
	case s.bulk:
		s.calm++
		if s.calm >= bulkCalmFrames {
			s.bulk, s.calm = false, 0
		}
	}
	s.bulkMerged = false
	if s.bulkOff || !s.bulk || texts < 2 || !s.textLiftSafe() {
		return
	}
	// One window-sized raster in Tango's own CPU renderer. Its glyph masks
	// are cached across frames, so a changed label costs a mask copy, not a
	// CoreGraphics text draw into a freshly allocated CALayer backing store.
	batch := item{kind: itemPatch, frame: s.bounds, clip: s.bounds, bulkOps: s.opsScratch[:0]}
	out := s.items[:0]
	keep := s.keepScratch[:0]
	for _, it := range s.items {
		if it.kind != itemText {
			keep = append(keep, it)
			continue
		}
		if len(it.texts) > 0 {
			batch.bulkOps = append(batch.bulkOps, it.parts...)
			continue
		}
		batch.bulkOps = append(batch.bulkOps, bulkOp{op: it.op.(paint.Text), src: it.op, clip: it.clip})
	}
	k := keyData{}
	k.tag(252)
	k.u64(s.serial) // the window raster changes every bulk frame
	batch.key = string(k)
	s.bulkMerged = true
	out = append(out, keep...)
	out = append(out, batch)
	clear(s.items[len(out):])
	s.items = out
	clear(keep)
	s.keepScratch = keep[:0]
	s.opsScratch = batch.bulkOps
}

// textLiftSafe reports whether every text item can be drawn after every item
// that follows it: no non-text item overlaps the ink of any text item that
// precedes it in paint order. Earlier backgrounds stay below text either way.
// Ink is the exact coverage of the CPU glyph masks the bulk raster draws
// (clipped), not the native bounds, which carry a smoothing allowance that
// reaches into neighbouring cells.
func (s *scene) textLiftSafe() bool {
	// Text ink painted so far, indexed by liftTile-pixel tiles: a data grid
	// checks each cell background against its neighbours, not every label.
	before := s.inkScratch[:0]
	if s.inkTiles == nil {
		s.inkTiles = map[image.Point][]int32{}
	}
	for k, v := range s.inkTiles {
		s.inkTiles[k] = v[:0]
	}
	defer func() { s.inkScratch = before[:0] }()
	var union image.Rectangle
	for _, it := range s.items {
		if it.kind == itemText {
			ink := s.textInk(it)
			if ink.Empty() {
				continue
			}
			i := int32(len(before))
			before = append(before, ink)
			union = union.Union(ink)
			for ty := ink.Min.Y / liftTile; ty <= (ink.Max.Y-1)/liftTile; ty++ {
				for tx := ink.Min.X / liftTile; tx <= (ink.Max.X-1)/liftTile; tx++ {
					t := image.Pt(tx, ty)
					s.inkTiles[t] = append(s.inkTiles[t], i)
				}
			}
			continue
		}
		f := it.frame.Intersect(union)
		if f.Empty() {
			continue
		}
		for ty := f.Min.Y / liftTile; ty <= (f.Max.Y-1)/liftTile; ty++ {
			for tx := f.Min.X / liftTile; tx <= (f.Max.X-1)/liftTile; tx++ {
				for _, i := range s.inkTiles[image.Pt(tx, ty)] {
					if it.frame.Overlaps(before[i]) {
						return false
					}
				}
			}
		}
	}
	return true
}

// intern returns k as a string shared across frames, so the keys of the
// few distinct solid fills are not reallocated per item per frame.
func (s *scene) intern(k keyData) string {
	if v, ok := s.keys[string(k)]; ok {
		return v
	}
	if s.keys == nil || len(s.keys) >= 4096 {
		s.keys = map[string]string{}
	}
	v := string(k)
	s.keys[v] = v
	return v
}

// liftTile is textLiftSafe's index tile size in device pixels.
const liftTile = 64

// textHash identifies a lazily planned text op's appearance independent of
// integer movement, like the native text key. It only drives the bulk
// heuristic; pixels never depend on it.
func (s *scene) textHash(op paint.Text) uint64 {
	h := uint64(14695981039346656037)
	mix := func(v uint64) { h ^= v; h *= 1099511628211 }
	mix(uint64(op.Color.R) | uint64(op.Color.G)<<8 | uint64(op.Color.B)<<16 | uint64(op.Color.A)<<24)
	fx := op.Origin.X*s.scale - float32(math.Floor(float64(op.Origin.X*s.scale)))
	fy := op.Origin.Y*s.scale - float32(math.Floor(float64(op.Origin.Y*s.scale)))
	mix(uint64(math.Float32bits(fx)) | uint64(math.Float32bits(fy))<<32)
	for _, g := range op.Glyphs {
		mix(uint64(g.ID) | uint64(math.Float32bits(g.Size))<<32)
		mix(uint64(math.Float32bits(g.X)) | uint64(math.Float32bits(g.Y))<<32)
		mix(uint64(math.Float32bits(g.Skew)))
	}
	return h
}

// keyHash is the bulk change-detection identity of a native text key.
func keyHash(k string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(k); i++ {
		h ^= uint64(k[i])
		h *= 1099511628211
	}
	return h
}

// textInk is the device rectangle the CPU rasterizer covers for a text item.
func (s *scene) textInk(it item) image.Rectangle {
	ops := []bulkOp{{}}
	if len(it.parts) > 0 {
		ops = it.parts
	} else {
		ops[0] = bulkOp{op: it.op.(paint.Text), clip: it.clip}
	}
	var ink image.Rectangle
	for _, b := range ops {
		var r image.Rectangle
		for _, g := range b.op.Glyphs {
			if g.Src == nil {
				continue
			}
			x, y, sub := paint.GlyphPlacement(b.op.Origin, g, s.scale)
			if mask, off := s.cpu.GlyphMask(g, g.Size*s.scale, sub); mask != nil {
				r = r.Union(mask.Rect.Add(image.Pt(x, y).Add(off)))
			}
		}
		ink = ink.Union(r.Intersect(b.clip))
	}
	return ink
}

// Coalesce only adjacent same-row text. Each part keeps its own exact clip and
// color. Bounding whitespace is capped, and non-text content is an ordering
// barrier, so grouping cannot paint across a background, image or effect.
func (s *scene) coalesceText() {
	in := s.items
	out := in[:0]
	for i := 0; i < len(in); {
		first := in[i]
		if first.kind != itemText {
			out = append(out, first)
			i++
			continue
		}
		frame := first.frame
		area := uint64(frame.Dx()) * uint64(frame.Dy())
		j := i + 1
		for j < len(in) && j-i < 8 && in[j].kind == itemText {
			next := in[j].frame
			overlap := min(frame.Max.Y, next.Max.Y) - max(frame.Min.Y, next.Min.Y)
			if overlap <= 0 || 2*overlap < min(frame.Dy(), next.Dy()) {
				break
			}
			union := frame.Union(next)
			sum := area + uint64(next.Dx())*uint64(next.Dy())
			if uint64(union.Dx())*uint64(union.Dy()) > 2*sum {
				break
			}
			frame, area = union, sum
			j++
		}
		if j == i+1 {
			out = append(out, first)
			i = j
			continue
		}
		batch := item{kind: itemText, frame: frame}
		k := keyData{}
		k.tag(253)
		k.i(frame.Dx())
		k.i(frame.Dy())
		k.i(j - i)
		for _, part := range in[i:j] {
			local := part.frame.Sub(frame.Min)
			k.i(local.Min.X)
			k.i(local.Min.Y)
			k.i(len(part.key))
			k = append(k, part.key...)
			batch.texts = append(batch.texts, textPart{frame: local, color: part.color, runs: part.runs})
			batch.parts = append(batch.parts, bulkOp{op: part.op.(paint.Text), src: part.op, clip: part.clip})
		}
		batch.key = string(k)
		batch.hash = keyHash(batch.key)
		out = append(out, batch)
		i = j
	}
	clear(in[len(out):])
	s.items = out
}
