//go:build darwin && !ios && arm64

package quartz

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

type node struct {
	owner   *renderer
	layer   objc.ID
	kind    itemKind
	key     string
	frame   image.Rectangle
	backing image.Point
	scale   float32
	color   color.NRGBA
	runs    []nativeglyph.Run
	texts   []textPart
	used    uint64
	dirty   bool
	paints  uint64
}
type imageResource struct {
	id   uintptr
	refs []any
	used uint64
}
type renderer struct {
	layer                  objc.ID
	scene                  *scene
	nodes, spare           []*node
	images                 map[string]*imageResource
	epoch                  uint64
	background             color.NRGBA
	haveBackground         bool
	closed                 bool
	drawErr                error
	renderCost, lastWork   time.Duration
	fallback, metricsReady bool
}

// NewPresenter attaches retained content layers to an existing hosted CALayer.
// All methods and native callbacks belong to the UI thread.
func NewPresenter(target uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
	if target == 0 {
		return nil, nil, fmt.Errorf("quartz: nil target layer")
	}
	if err := loadNative(); err != nil {
		return nil, nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := nativePoolPush()
	defer nativePoolPop(pool)
	layer := objc.ID(target)
	if byte(layer.Send(sel("isKindOfClass:"), objc.ID(objc.GetClass("CALayer")))) == 0 {
		return nil, nil, fmt.Errorf("quartz: target is not a CALayer")
	}
	layer.Send(sel("retain"))
	r := &renderer{layer: layer, scene: newScene(nativeglyph.CoreGraphics), images: map[string]*imageResource{}}
	done := nativeTransaction()
	defer done()
	layer.Send(sel("setGeometryFlipped:"), true)
	layer.Send(sel("setMasksToBounds:"), true)
	layer.Send(sel("setContents:"), uintptr(0))
	return r, r.present, nil
}

type reuseBucket struct {
	nodes []*node
	next  int
}

func (b *reuseBucket) take(epoch uint64) *node {
	for b.next < len(b.nodes) {
		n := b.nodes[b.next]
		b.next++
		if n.used != epoch {
			return n
		}
	}
	return nil
}

func (r *renderer) Render(l *paint.List, size geom.Size, scale float32) error {
	if r.closed {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := nativePoolPush()
	defer nativePoolPop(pool)
	start := time.Now()
	defer func() { r.renderCost = time.Since(start) }()
	r.metricsReady = false
	r.epoch++
	r.drawErr = nil
	if err := r.scene.plan(l, size, scale); err != nil {
		return err
	}
	defer r.scene.releaseScratch()
	r.fallback = r.scene.full
	done := nativeTransaction()
	defer done()
	if err := r.apply(size); err != nil {
		return err
	}
	// Draw changed backing stores inside the same explicit transaction as
	// their geometry/content changes. Drawing after commit creates a second
	// implicit submission, adding work to every animated text frame.
	r.drawText()
	if r.drawErr != nil {
		return r.drawErr
	}
	r.metricsReady = true
	return nil
}
func (r *renderer) apply(size geom.Size) (err error) {
	var created []*node
	defer func() {
		if err != nil {
			for _, n := range created {
				n.close()
			}
			for _, resource := range r.images {
				cgImageRelease(resource.id)
			}
			clear(r.images)
			for _, n := range r.nodes {
				if n.kind == itemPatch {
					n.key = ""
				}
			}
		}
	}()
	s := r.scene
	setNativeRect(r.layer, sel("setBounds:"), nativeRect{Size: nativeSize{float64(max(0, size.W)), float64(max(0, size.H))}})
	setNativeFloat(r.layer, sel("setContentsScale:"), float64(s.scale))
	background := s.background
	if s.full {
		background = color.NRGBA{}
	}
	if !r.haveBackground || background != r.background {
		nativeColor(r.layer, "setBackgroundColor:", background)
		r.background = background
		r.haveBackground = true
	}
	old := r.nodes
	next := r.spare[:0]
	needed := make(map[string]int, len(s.items))
	for _, it := range s.items {
		needed[it.key]++
	}
	buckets := make(map[string]*reuseBucket, len(old))
	for _, n := range old {
		b := buckets[n.key]
		if b == nil {
			b = &reuseBucket{}
			buckets[n.key] = b
		}
		b.nodes = append(b.nodes, n)
	}
	for i, it := range s.items {
		needed[it.key]--
		var n *node
		if i < len(old) && old[i].used != r.epoch && old[i].key == it.key {
			n = old[i]
		} else if b := buckets[it.key]; b != nil {
			n = b.take(r.epoch)
		}
		if n == nil && i < len(old) && old[i].used != r.epoch && old[i].kind == it.kind && needed[old[i].key] == 0 {
			n = old[i]
		}
		if n == nil {
			for _, candidate := range old {
				if candidate.used != r.epoch && candidate.kind == it.kind && needed[candidate.key] == 0 {
					n = candidate
					break
				}
			}
		}
		if n == nil {
			n = r.newNode(it.kind)
			if n.layer == 0 {
				return fmt.Errorf("quartz: cannot allocate content layer")
			}
			created = append(created, n)
		}
		n.used = r.epoch
		if err := r.update(n, it); err != nil {
			return err
		}
		next = append(next, n)
	}
	changed := len(next) != len(old)
	if !changed {
		for i, n := range next {
			if old[i] != n {
				changed = true
				break
			}
		}
	}
	if changed {
		array := objc.ID(objc.GetClass("NSMutableArray")).Send(sel("arrayWithCapacity:"), uintptr(len(next)))
		for _, n := range next {
			array.Send(sel("addObject:"), n.layer)
		}
		r.layer.Send(sel("setSublayers:"), array)
	}
	for _, n := range old {
		if n.used != r.epoch {
			n.close()
		}
	}
	for key, resource := range r.images {
		if resource.used != r.epoch {
			cgImageRelease(resource.id)
			delete(r.images, key)
		}
	}
	r.nodes = next
	clear(old)
	r.spare = old[:0]
	return nil
}
func (r *renderer) newNode(kind itemKind) *node {
	n := &node{owner: r, kind: kind, layer: objc.ID(objc.GetClass("CALayer")).Send(sel("new"))}
	if n.layer == 0 {
		return n
	}
	if kind == itemText {
		n.layer.Send(sel("setDrawsAsynchronously:"), false)
		n.layer.Send(sel("setDelegate:"), nativeDelegate)
		nativeDrawers[n.layer] = n
	}
	n.layer.Send(sel("setMasksToBounds:"), true)
	if kind == itemPatch {
		// Integer device-aligned image frames must not be filtered at a fractional
		// logical scale. Their bitmaps already contain the reference coverage.
		n.layer.Send(sel("setMinificationFilter:"), nativeNearest)
		n.layer.Send(sel("setMagnificationFilter:"), nativeNearest)
	}
	return n
}
func (r *renderer) update(n *node, it item) error {
	changed := n.key != it.key
	backing := it.frame.Size()
	if it.kind == itemText {
		backing = keepTextBacking(n.backing, backing)
	}
	if n.frame != it.frame || n.scale != r.scene.scale || n.backing != backing {
		s := float64(r.scene.scale)
		setNativeRect(n.layer, sel("setFrame:"), nativeRect{nativePoint{float64(it.frame.Min.X) / s, float64(it.frame.Min.Y) / s}, nativeSize{float64(backing.X) / s, float64(backing.Y) / s}})
		if n.scale != r.scene.scale {
			setNativeFloat(n.layer, sel("setContentsScale:"), s)
		}
		n.frame, n.backing, n.scale = it.frame, backing, r.scene.scale
	}

	switch it.kind {
	case itemSolid:
		if changed {
			nativeColor(n.layer, "setBackgroundColor:", it.color)
			n.color = it.color
		}
	case itemText:
		if changed {
			n.runs = cloneRuns(it.runs)
			n.texts = make([]textPart, len(it.texts))
			for i, part := range it.texts {
				n.texts[i] = part
				n.texts[i].runs = cloneRuns(part.runs)
			}
			n.color = it.color
			n.dirty = true
			n.layer.Send(sel("setNeedsDisplay"))
		}
	case itemPatch:
		resource := r.images[it.key]
		if resource == nil {
			id, err := nativeImage(r.scene.pixels(it))
			if err != nil {
				return err
			}
			resource = &imageResource{id: id, refs: append([]any(nil), it.refs...)}
			r.images[it.key] = resource
		}
		resource.used = r.epoch
		if changed {
			n.layer.Send(sel("setContents:"), resource.id)
		}
	}
	n.key = it.key
	return nil
}
func cloneRuns(src []nativeglyph.Run) []nativeglyph.Run {
	dst := make([]nativeglyph.Run, len(src))
	for i, run := range src {
		dst[i] = run
		dst[i].IDs = append([]uint16(nil), run.IDs...)
		dst[i].Positions = append([]geom.Point(nil), run.Positions...)
	}
	return dst
}
func (n *node) draw(ctx uintptr) {
	if ctx == 0 || n.owner == nil || n.owner.closed {
		return
	}
	n.paints++
	cgSave(ctx)
	defer cgRestore(ctx)
	// Native glyph positions are y-up. A flipped content context needs one
	// local reversal; parent layer geometry does not change glyph coordinates.
	if byte(n.layer.Send(sel("contentsAreFlipped"))) != 0 {
		cgTranslate(ctx, 0, float64(n.backing.Y)/float64(n.scale))
		cgScale(ctx, 1, -1)
	}
	cgScale(ctx, 1/float64(n.scale), 1/float64(n.scale))
	// The backing store may outlive small changes in ink dimensions. Keep its
	// top-left anchor, then clip the complete run to this frame's exact visible
	// ink rectangle. Padding must never reveal glyphs cropped by the scene clip.
	padY := n.backing.Y - n.frame.Dy()
	cgClip(ctx, nativeRect{nativePoint{0, float64(padY)}, nativeSize{float64(n.frame.Dx()), float64(n.frame.Dy())}})
	cgTranslate(ctx, 0, float64(padY))
	cgFillSpace(ctx, nativeSRGB)
	for i, fn := range cgFontFlags {
		fn(ctx, i < 5)
	}
	if len(n.texts) == 0 {
		n.drawRuns(ctx, n.runs, n.color)
	}
	for _, part := range n.texts {
		cgSave(ctx)
		cgTranslate(ctx, float64(part.frame.Min.X), float64(n.frame.Dy()-part.frame.Max.Y))
		cgClip(ctx, nativeRect{Size: nativeSize{float64(part.frame.Dx()), float64(part.frame.Dy())}})
		n.drawRuns(ctx, part.runs, part.color)
		cgRestore(ctx)
	}
	runtime.KeepAlive(n.runs)
	runtime.KeepAlive(n.texts)
}
func (n *node) drawRuns(ctx uintptr, runs []nativeglyph.Run, c color.NRGBA) {
	values := [4]float64{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255}
	cgFillColor(ctx, &values[0])
	provider := n.owner.scene.provider
	for _, run := range runs {
		if provider == nil || !provider.Draw(ctx, run) {
			n.owner.drawErr = fmt.Errorf("quartz: native glyph drawing failed")
			break
		}
	}
}

func (r *renderer) drawText() {
	for _, n := range r.nodes {
		if n.kind == itemText && n.dirty {
			n.layer.Send(sel("displayIfNeeded"))
			n.dirty = false
		}
	}
}

func (r *renderer) present(_ image.Point, _ float32) error {
	if r.closed {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := nativePoolPush()
	defer nativePoolPop(pool)
	start := time.Now()

	if r.metricsReady {
		r.lastWork = r.renderCost + time.Since(start)
	} else {
		r.lastWork = 0
	}
	return r.drawErr
}

// FrameMetrics consumes completed work from the most recently rendered frame.
func (r *renderer) FrameMetrics() (time.Duration, bool) {
	if !r.metricsReady {
		return 0, false
	}
	r.metricsReady = false
	return r.lastWork, r.fallback
}
func (n *node) close() {
	if n.layer == 0 {
		return
	}
	delete(nativeDrawers, n.layer)
	n.layer.Send(sel("setDelegate:"), objc.ID(0))
	n.layer.Send(sel("removeFromSuperlayer"))
	n.layer.Send(sel("release"))
	n.layer = 0
	n.owner = nil
	n.runs = nil
	n.texts = nil
}
func (r *renderer) Trim() {
	if r.closed {
		return
	}
	r.scene.trim()
	for _, resource := range r.images {
		cgImageRelease(resource.id)
	}
	clear(r.images)
	// Layer contents own the currently displayed images. Forgetting cache
	// ownership preserves those pixels until the next complete frame replaces them.
	for _, n := range r.nodes {
		n.key = ""
	}
}
func (r *renderer) InvalidateImage(img image.Image) {
	if !r.closed {
		r.scene.invalidateImage(img)
	}
}
func (r *renderer) Close() error {
	if r.closed {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := nativePoolPush()
	defer nativePoolPop(pool)
	r.closed = true
	r.metricsReady = false
	for _, n := range r.nodes {
		n.close()
	}
	r.nodes = nil
	clear(r.spare)
	r.spare = nil
	for _, resource := range r.images {
		cgImageRelease(resource.id)
	}
	clear(r.images)
	r.scene.releaseScratch()
	r.scene.cpu = nil
	r.layer.Send(sel("setBackgroundColor:"), uintptr(0))
	r.layer.Send(sel("release"))
	r.layer = 0
	return nil
}
