//go:build darwin

package metal

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
)

// recordTimeline installs a timeline callback on d and returns the slice
// it appends to (read it after Wait/Close; the callback runs under d.mu).
func recordTimeline(d *Device) *[]Timing {
	var got []Timing
	d.SetTimeline(func(t Timing) { got = append(got, t) })
	return &got
}

// TestMetalTimelineFrames: every retired frame reports its frame number,
// commit time and, when Metal provides it, GPU start/end; frames carry no
// drawable wait.
func TestMetalTimelineFrames(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	got := recordTimeline(p.dev)
	l := gputest.Scenes[0].Make(t)
	for range 3 {
		if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 3 {
		t.Fatalf("%d timings, want 3", len(*got))
	}
	var last Timing
	for i, tm := range *got {
		if tm.Kind != "frame" || tm.Frame != uint64(i+1) || tm.Err != nil {
			t.Fatalf("timing %d = %+v, want frame %d without error", i, tm, i+1)
		}
		if tm.Commit.IsZero() || (i > 0 && tm.Commit.Before(last.Commit)) {
			t.Fatalf("timing %d commit %v not monotonic after %v", i, tm.Commit, last.Commit)
		}
		if tm.GPUKnown && tm.GPUEnd < tm.GPUStart {
			t.Fatalf("timing %d GPU end %v before start %v", i, tm.GPUEnd, tm.GPUStart)
		}
		if !tm.GPUKnown && (tm.GPUStart != 0 || tm.GPUEnd != 0) {
			t.Fatalf("timing %d has unknown GPU time but values %v..%v", i, tm.GPUStart, tm.GPUEnd)
		}
		if tm.DrawableWait != 0 || tm.Drawable {
			t.Fatalf("timing %d: a frame reports a drawable (%v, %v)", i, tm.Drawable, tm.DrawableWait)
		}
		last = tm
	}
}

// TestMetalTimelinePresent: a present reports its drawable acquisition
// separately from GPU time, and a present without a drawable reports a
// miss without committing anything.
func TestMetalTimelinePresent(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	got := recordTimeline(p.dev)
	size := devicePixels(gputest.SceneSize, 1)
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
	var present *Timing
	for i := range *got {
		if (*got)[i].Kind == "present" {
			present = &(*got)[i]
		}
	}
	if present == nil {
		t.Fatalf("no present timing in %+v", *got)
	}
	if !present.Drawable || present.DrawableWait < 0 || present.Frame != 1 || present.Commit.IsZero() {
		t.Fatalf("present timing = %+v", *present)
	}

	n := len(*got)
	msgSetID(layer, sel("setDevice:"), 0) // no drawable to give
	start := time.Now()
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	if len(*got) != n+1 {
		t.Fatalf("a missed drawable reported %d timings, want 1", len(*got)-n)
	}
	miss := (*got)[n]
	if miss.Kind != "present" || miss.Drawable || !miss.Commit.IsZero() || miss.GPUKnown {
		t.Fatalf("miss timing = %+v, want a present without drawable or commit", miss)
	}
	if miss.DrawableWait > took {
		t.Fatalf("drawable wait %v longer than the present call %v", miss.DrawableWait, took)
	}
}

// TestMetalTimelineCloseDrains: Close retires (and reports) the frames in
// flight before releasing the device.
func TestMetalTimelineCloseDrains(t *testing.T) {
	d, err := NewDevice()
	if err != nil {
		t.Skipf("no Metal device: %v", err)
	}
	got := recordTimeline(d)
	f := copyTargetSeed(image.Pt(64, 48))
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if len(*got) != 1 || (*got)[0].Kind != "frame" {
		t.Fatalf("timings after Close = %+v, want the in-flight frame", *got)
	}
}

// TestNewPresenterTimelineSource (review M5): the renderer NewPresenter
// returns is the metal.TimelineSource uihost's TANGO_FRAME_TRACE asserts
// it to, and its timeline delivers the present's drawable wait and the
// frame's GPU time.
func TestNewPresenterTimelineSource(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	r, present, err := NewPresenter(uintptr(layer))
	if err != nil {
		t.Skipf("no Metal device: %v", err)
	}
	t.Cleanup(func() { r.(interface{ Close() error }).Close() })
	src, ok := r.(TimelineSource)
	if !ok {
		t.Fatalf("NewPresenter's renderer %T is not a TimelineSource", r)
	}
	var kinds []string
	src.SetTimeline(func(tm Timing) { kinds = append(kinds, tm.Kind) })
	if err := r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := present(devicePixels(gputest.SceneSize, 1), 1); err != nil {
		t.Fatal(err)
	}
	r.(interface{ Close() error }).Close() // drains: both are retired and reported
	if !slices.Contains(kinds, "frame") || !slices.Contains(kinds, "present") {
		t.Fatalf("timeline kinds %v, want a frame and a present", kinds)
	}
}
