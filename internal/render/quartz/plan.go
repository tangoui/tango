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

type item struct {
	kind  itemKind
	key   string
	frame image.Rectangle
	color color.NRGBA
	runs  []nativeglyph.Run
	op    paint.Op
	clip  image.Rectangle
	refs  []any
	image *image.RGBA
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
}

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
			k := keyData{}
			k.tag(byte(itemSolid))
			k.rgba(fill.Color)
			k.i(frame.Dx())
			k.i(frame.Dy())
			s.items = append(s.items, item{kind: itemSolid, key: string(k), frame: frame, color: fill.Color})
			continue
		}
		k, refs, e := s.patchKey(op, frame, clip)
		if e != nil {
			return e
		}
		s.items = append(s.items, item{kind: itemPatch, key: k, frame: frame, op: op, clip: clip, refs: refs})
	}
	// Text backing stores and immutable image snapshots must stay bounded even
	// when a display list stacks many large, partially transparent contents.
	// A complete software frame preserves ordering and alpha within one bitmap.
	var bitmapBytes uint64
	for _, it := range s.items {
		if it.kind == itemSolid {
			continue
		}
		bitmapBytes += uint64(it.frame.Dx()) * uint64(it.frame.Dy()) * 4
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
	// Reconstruct the integer device clip as logical geometry; the reference
	// renderer rounds its rectangular clips back to these exact device pixels.
	c := it.clip
	f := s.scale
	cr := geom.Rect{X: float32(c.Min.X) / f, Y: float32(c.Min.Y) / f, W: float32(c.Dx()) / f, H: float32(c.Dy()) / f}
	return s.cpu.RenderRegion([]paint.Op{paint.PushClip{Rect: cr}, it.op, paint.Pop{}}, it.frame, s.scale)
}
func (s *scene) releaseScratch() {
	clear(s.items)
	s.items = s.items[:0]
	clear(s.ops)
	s.ops = s.ops[:0]
}
func (s *scene) trim()                           { s.cpu = paint.NewCPU(); s.imageEpoch++; s.releaseScratch() }
func (s *scene) invalidateImage(img image.Image) { s.cpu.InvalidateImage(img); s.imageEpoch++ }
