package uihost_test

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

// damagePage draws a box whose colour flips with *red and declares the
// box as the frame's damage, so every frame after the first is partial.
func damagePage(red *atomic.Bool) func(*ui.Context) {
	return func(c *ui.Context) {
		col := theme.Hex("#0000ff")
		if red.Load() {
			col = theme.Hex("#ff0000")
		}
		ui.Div(c, layout.Size(40, 40)).Bg(col)
		c.Damage(geom.Rect{W: 40, H: 40})
	}
}

func openDamage(t *testing.T, fb *fake.Backend, a *tango.App, red *atomic.Bool) *fake.Canvas {
	t.Helper()
	fb.SetCanvasScale(2)
	_, _, cv := open(t, a, fb, 200, 100, damagePage(red))
	for cv.FlushFrame() { // settle
	}
	return cv
}

func pixel(img *image.RGBA, x, y int) color.RGBA {
	return img.RGBAAt(x, y)
}

func TestDamagePresentedPartially(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasDamage()
	var red atomic.Bool
	cv := openDamage(t, fb, a, &red)
	presents, partial := cv.Presents(), cv.DamagePresents()
	if presents < 1 {
		t.Fatal("first frame not presented whole")
	}
	red.Store(true)
	cv.Frame()
	if cv.Presents() != presents || cv.DamagePresents() != partial+1 {
		t.Fatalf("presents %d->%d, partial %d->%d", presents, cv.Presents(), partial, cv.DamagePresents())
	}
	// Logical (0,0)-(40,40) at scale 2, as device pixels.
	if got := cv.DamageRects(); len(got) != 1 || got[0] != image.Rect(0, 0, 80, 80) {
		t.Fatalf("dirty = %v", got)
	}
	if p := pixel(cv.LastFrame(), 40, 40); p.R != 255 || p.B != 0 {
		t.Fatalf("damaged region not updated: %v", p)
	}
}

func TestDamageWholeFrameAfterResize(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasDamage()
	var red atomic.Bool
	cv := openDamage(t, fb, a, &red)
	cv.Frame() // partial
	presents, partial := cv.Presents(), cv.DamagePresents()
	// Same size and scale: the runner itself would present partially, the
	// window must not.
	cv.Resize(200, 100, 2)
	cv.Frame()
	if cv.Presents() != presents+1 || cv.DamagePresents() != partial {
		t.Fatalf("after resize: presents %d->%d, partial %d->%d", presents, cv.Presents(), partial, cv.DamagePresents())
	}
	cv.Frame()
	if cv.DamagePresents() != partial+1 {
		t.Fatal("partial presentation did not resume after the whole frame")
	}
	// A real size change: whole frame of the new size.
	cv.Resize(300, 100, 2)
	presents = cv.Presents()
	cv.Frame()
	if cv.Presents() != presents+1 {
		t.Fatal("frame after a size change was not whole")
	}
	if w, h := cv.LastFrameSize(); w != 600 || h != 200 {
		t.Fatalf("frame %dx%d", w, h)
	}
}

func TestDamageFailureFallsBack(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasDamage()
	var red atomic.Bool
	cv := openDamage(t, fb, a, &red)
	presents, partial := cv.Presents(), cv.DamagePresents()
	fb.FailCanvasDamage(1)
	red.Store(true)
	cv.Frame()
	if cv.DamageFailures() != 1 || cv.Presents() != presents+1 || cv.DamagePresents() != partial {
		t.Fatalf("failed partial: failures %d presents %d->%d partial %d->%d",
			cv.DamageFailures(), presents, cv.Presents(), partial, cv.DamagePresents())
	}
	if p := pixel(cv.LastFrame(), 40, 40); p.R != 255 {
		t.Fatalf("fallback frame not shown: %v", p)
	}
	red.Store(false)
	cv.Frame()
	if cv.DamagePresents() != partial+1 {
		t.Fatal("one failure turned partial presentation off")
	}
}

func TestDamageRepeatedFailuresTurnOff(t *testing.T) {
	restore := uihost.SetDamageLogf(func(string, ...any) {})
	defer restore()
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasDamage()
	var red atomic.Bool
	cv := openDamage(t, fb, a, &red)
	fb.FailCanvasDamage(3)
	for range 3 {
		red.Store(!red.Load())
		cv.Frame()
	}
	calls, presents := cv.DamageCalls(), cv.Presents()
	if cv.DamageFailures() != 3 {
		t.Fatalf("failures %d", cv.DamageFailures())
	}
	red.Store(!red.Load())
	cv.Frame()
	if cv.DamageCalls() != calls || cv.Presents() != presents+1 {
		t.Fatalf("after 3 failures: calls %d->%d presents %d->%d", calls, cv.DamageCalls(), presents, cv.Presents())
	}
}

// A canvas without a partial path (the fake by default) gets whole frames.
func TestDamageWithoutPartialPath(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var red atomic.Bool
	cv := openDamage(t, fb, a, &red)
	presents := cv.Presents()
	red.Store(true)
	cv.Frame()
	if cv.Presents() != presents+1 || cv.DamagePresents() != 0 {
		t.Fatalf("presents %d->%d partial %d", presents, cv.Presents(), cv.DamagePresents())
	}
	if p := pixel(cv.LastFrame(), 40, 40); p.R != 255 {
		t.Fatalf("frame not shown: %v", p)
	}
}
