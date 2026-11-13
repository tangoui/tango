package fake

// platform.CanvasDamagePresenter on fake canvases. Off by default: until
// EnableCanvasDamage, PresentDamage presents the whole frame (as a canvas
// that keeps no previous frame must), so it counts as a Present. When on,
// it copies only the dirty rectangles into the last frame and records
// them; FailCanvasDamage makes the next calls fail.

import (
	"errors"
	"image"
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.CanvasDamagePresenter = (*Canvas)(nil)

// errDamage is the error of a PresentDamage call made to fail.
var errDamage = errors.New("fake: PresentDamage failed")

type damageCfg struct {
	mu   sync.Mutex
	on   bool
	fail int // the next fail calls return errDamage
}

type damageRec struct {
	mu       sync.Mutex
	calls    int                 // PresentDamage calls
	partial  int                 // partial presents done
	rects    [][]image.Rectangle // dirty lists of the partial presents
	failures int
}

var (
	damageCfgs sync.Map // *Backend -> *damageCfg
	damageRecs sync.Map // *Canvas -> *damageRec
)

func (b *Backend) damageCfg() *damageCfg {
	c, _ := damageCfgs.LoadOrStore(b, &damageCfg{})
	return c.(*damageCfg)
}

func (c *Canvas) damageRec() *damageRec {
	r, _ := damageRecs.LoadOrStore(c, &damageRec{})
	return r.(*damageRec)
}

// EnableCanvasDamage turns partial presentation on for this backend's
// canvases. Any goroutine.
func (b *Backend) EnableCanvasDamage() {
	d := b.damageCfg()
	d.mu.Lock()
	d.on = true
	d.mu.Unlock()
}

// FailCanvasDamage makes the next n PresentDamage calls on this backend's
// canvases fail without changing the frame. Any goroutine.
func (b *Backend) FailCanvasDamage(n int) {
	d := b.damageCfg()
	d.mu.Lock()
	d.fail = n
	d.mu.Unlock()
}

// PresentDamage implements platform.CanvasDamagePresenter. Main thread.
func (c *Canvas) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	c.b.mustMain("PresentDamage")
	cfg := c.b.damageCfg()
	cfg.mu.Lock()
	on, fail := cfg.on, cfg.fail > 0
	if fail {
		cfg.fail--
	}
	cfg.mu.Unlock()
	rec := c.damageRec()
	rec.mu.Lock()
	rec.calls++
	if fail {
		rec.failures++
	}
	rec.mu.Unlock()
	if fail {
		return errDamage
	}
	c.mu.Lock()
	last := c.last
	c.mu.Unlock()
	if !on || img == nil || last == nil || last.Rect != img.Rect {
		return c.Present(img) // no previous frame to update: whole frame
	}
	// A new image: LastFrame results handed out before stay unchanged.
	next := &image.RGBA{Pix: slices.Clone(last.Pix), Stride: last.Stride, Rect: last.Rect}
	for _, r := range dirty {
		r = r.Intersect(img.Rect)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			i := img.PixOffset(r.Min.X, y)
			copy(next.Pix[next.PixOffset(r.Min.X, y):], img.Pix[i:i+r.Dx()*4])
		}
	}
	c.mu.Lock()
	c.last = next
	c.mu.Unlock()
	rec.mu.Lock()
	rec.partial++
	rec.rects = append(rec.rects, slices.Clone(dirty))
	rec.mu.Unlock()
	return nil
}

// DamageCalls counts PresentDamage calls (failed ones included);
// DamagePresents counts the partial presents done; DamageFailures counts
// the calls made to fail.
func (c *Canvas) DamageCalls() int {
	r := c.damageRec()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (c *Canvas) DamagePresents() int {
	r := c.damageRec()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.partial
}

func (c *Canvas) DamageFailures() int {
	r := c.damageRec()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failures
}

// DamageRects returns the dirty list of the last partial present.
func (c *Canvas) DamageRects() []image.Rectangle {
	r := c.damageRec()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.rects) == 0 {
		return nil
	}
	return slices.Clone(r.rects[len(r.rects)-1])
}
