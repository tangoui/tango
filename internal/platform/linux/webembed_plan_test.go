package linux

import (
	"image"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestEmbedTracker(t *testing.T) {
	var tr embedTracker
	p := platform.WebPlacement{
		Frame: platform.RectF{X: 10, Y: -20, W: 200, H: 400},
		Clip:  platform.RectF{X: 0, Y: 0, W: 400, H: 300},
	}
	g := p.Geometry(1, false)
	if want := image.Rect(10, 0, 210, 300); g.Clip != want {
		t.Fatalf("clip %v, want %v", g.Clip, want)
	}
	if want := image.Rect(0, -20, 200, 380); g.Frame != want {
		t.Fatalf("frame %v, want %v", g.Frame, want)
	}

	// Created hidden: an empty placement does nothing.
	if st := tr.update(platform.WebPlacement{}.Geometry(1, false)); !st.none() {
		t.Fatalf("empty placement on a new view: %+v", st)
	}
	if st := tr.update(g); st != (embedStep{Show: true, Clip: true, Frame: true}) {
		t.Fatalf("first placement: %+v", st)
	}
	if st := tr.update(g); !st.none() {
		t.Fatalf("same placement: %+v", st)
	}

	// Scrolling by 5: the clip stays, the frame moves inside it.
	p.Frame.Y = -25
	g2 := p.Geometry(1, false)
	if st := tr.update(g2); st != (embedStep{Frame: true}) {
		t.Fatalf("scrolled: %+v", st)
	}

	// The app hides it, then shows it again at the same place.
	if st := tr.update(p.Geometry(1, true)); st != (embedStep{Hide: true}) {
		t.Fatalf("hidden: %+v", st)
	}
	if st := tr.update(p.Geometry(1, true)); !st.none() {
		t.Fatalf("hidden again: %+v", st)
	}
	if st := tr.update(g2); st != (embedStep{Show: true}) {
		t.Fatalf("shown at the same place: %+v", st)
	}

	// Clipped away entirely, then back with a smaller clip (the frame
	// keeps its offset inside the container).
	p.Clip = platform.RectF{X: 500, Y: 500, W: 10, H: 10}
	if st := tr.update(p.Geometry(1, false)); st != (embedStep{Hide: true}) {
		t.Fatalf("clipped away: %+v", st)
	}
	p.Clip = platform.RectF{X: 0, Y: 0, W: 100, H: 100}
	if st := tr.update(p.Geometry(1, false)); st != (embedStep{Show: true, Clip: true}) {
		t.Fatalf("back: %+v", st)
	}
}
