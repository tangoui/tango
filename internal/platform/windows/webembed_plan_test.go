package windows

import (
	"image"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestEmbedScale(t *testing.T) {
	for _, c := range []struct {
		dpi  int
		want float64
	}{{96, 1}, {144, 1.5}, {192, 2}, {0, 1}, {-5, 1}} {
		if got := embedScale(c.dpi); got != c.want {
			t.Errorf("embedScale(%d) = %v, want %v", c.dpi, got, c.want)
		}
	}
}

func TestEmbedTargetDiff(t *testing.T) {
	var tg embedTarget
	r1 := image.Rect(0, 0, 10, 10)
	r2 := image.Rect(5, 5, 20, 20)

	// Unknown and hidden: hide once, so a fresh window is in a known state.
	if o := tg.diff(false, image.Rectangle{}); o != (embedOps{Hide: true}) {
		t.Fatalf("first hidden: %+v", o)
	}
	if o := tg.diff(false, image.Rectangle{}); !o.None() {
		t.Fatalf("still hidden: %+v", o)
	}
	if o := tg.diff(true, r1); o != (embedOps{Show: true, Move: true, Rect: r1}) {
		t.Fatalf("show: %+v", o)
	}
	if o := tg.diff(true, r1); !o.None() {
		t.Fatalf("unchanged: %+v", o)
	}
	if o := tg.diff(true, r2); o != (embedOps{Move: true, Rect: r2}) {
		t.Fatalf("move: %+v", o)
	}
	if o := tg.diff(false, image.Rectangle{}); o != (embedOps{Hide: true}) {
		t.Fatalf("hide: %+v", o)
	}
	// Showing again at the same rectangle moves too: nothing moved it
	// while hidden, but the call is cheap and keeps the order simple.
	if o := tg.diff(true, r2); o != (embedOps{Show: true, Move: true, Rect: r2}) {
		t.Fatalf("show again: %+v", o)
	}

	// Unknown and visible: move and show.
	var fresh embedTarget
	if o := fresh.diff(true, r1); o != (embedOps{Show: true, Move: true, Rect: r1}) {
		t.Fatalf("first visible: %+v", o)
	}
}

func TestEmbedPlanSync(t *testing.T) {
	var p embedPlan

	// Before any placement: hidden, the controller untouched while absent.
	c, k := p.sync(1, false)
	if c != (embedOps{Hide: true}) || !k.None() {
		t.Fatalf("unplaced: %+v %+v", c, k)
	}

	// A frame scrolled half out at the top, at 150%.
	p.place(platform.WebPlacement{
		Frame: platform.RectF{X: 10, Y: -20, W: 100, H: 80},
		Clip:  platform.RectF{X: 0, Y: 0, W: 400, H: 300},
	})
	c, k = p.sync(1.5, false)
	wantClip := image.Rect(15, 0, 165, 90)
	if c != (embedOps{Show: true, Move: true, Rect: wantClip}) || !k.None() {
		t.Fatalf("placed, no controller: %+v %+v", c, k)
	}
	if !p.visible() {
		t.Fatal("visible() false after show")
	}

	// The controller arrives: it gets the frame relative to the container.
	c, k = p.sync(1.5, true)
	wantFrame := image.Rect(0, -30, 150, 90)
	if !c.None() || k != (embedOps{Show: true, Move: true, Rect: wantFrame}) {
		t.Fatalf("controller ready: %+v %+v", c, k)
	}
	if c, k = p.sync(1.5, true); !c.None() || !k.None() {
		t.Fatalf("unchanged: %+v %+v", c, k)
	}

	// DPI change: both move.
	c, k = p.sync(1, true)
	if c != (embedOps{Move: true, Rect: image.Rect(10, 0, 110, 60)}) ||
		k != (embedOps{Move: true, Rect: image.Rect(0, -20, 100, 60)}) {
		t.Fatalf("dpi change: %+v %+v", c, k)
	}

	// Application hide wins over the placement; show restores it.
	p.hidden = true
	c, k = p.sync(1, true)
	if c != (embedOps{Hide: true}) || k != (embedOps{Hide: true}) {
		t.Fatalf("hidden: %+v %+v", c, k)
	}
	p.place(platform.WebPlacement{Frame: platform.RectF{W: 50, H: 50}, Clip: platform.RectF{W: 50, H: 50}})
	if c, k = p.sync(1, true); !c.None() || !k.None() {
		t.Fatalf("placement while hidden: %+v %+v", c, k)
	}
	p.hidden = false
	c, k = p.sync(1, true)
	r := image.Rect(0, 0, 50, 50)
	if c != (embedOps{Show: true, Move: true, Rect: r}) || k != (embedOps{Show: true, Move: true, Rect: r}) {
		t.Fatalf("shown: %+v %+v", c, k)
	}
	if w, h := p.size(); w != 50 || h != 50 {
		t.Fatalf("size %d×%d", w, h)
	}

	// An empty clip hides.
	p.place(platform.WebPlacement{Frame: platform.RectF{W: 50, H: 50}, Clip: platform.RectF{X: 60, W: 10, H: 10}})
	c, k = p.sync(1, true)
	if c != (embedOps{Hide: true}) || k != (embedOps{Hide: true}) || p.visible() {
		t.Fatalf("clipped away: %+v %+v", c, k)
	}

	// A new controller (reset) is brought up to date even when hidden.
	p.controllerReset()
	if c, k = p.sync(1, true); !c.None() || k != (embedOps{Hide: true}) {
		t.Fatalf("reset while hidden: %+v %+v", c, k)
	}
}

func TestEmbedBlurOnHide(t *testing.T) {
	if !embedBlurOnHide(embedOps{Hide: true}, true) {
		t.Error("focused view hidden: want blur")
	}
	if embedBlurOnHide(embedOps{Hide: true}, false) || embedBlurOnHide(embedOps{Move: true}, true) {
		t.Error("blur without hide or focus")
	}
}
