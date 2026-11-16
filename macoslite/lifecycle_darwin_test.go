//go:build darwin

package macoslite

import (
	"image"
	"sync"
	"testing"
)

// fakeBackend records native calls. dispatchMain queues work until flush,
// like performSelectorOnMainThread from a worker goroutine.
type fakeBackend struct {
	mu        sync.Mutex
	opens     int
	loops     int
	needs     int
	presents  []image.Rectangle
	closes    int
	queue     []func()
	runInline bool
}

func (f *fakeBackend) open(Options) error { f.mu.Lock(); f.opens++; f.mu.Unlock(); return nil }
func (f *fakeBackend) loop() error        { f.mu.Lock(); f.loops++; f.mu.Unlock(); return nil }
func (f *fakeBackend) setNeedsDisplay()   { f.mu.Lock(); f.needs++; f.mu.Unlock() }
func (f *fakeBackend) close()             { f.mu.Lock(); f.closes++; f.mu.Unlock() }
func (f *fakeBackend) present(img *image.RGBA) {
	f.mu.Lock()
	f.presents = append(f.presents, img.Bounds())
	f.mu.Unlock()
}
func (f *fakeBackend) dispatchMain(fn func()) {
	f.mu.Lock()
	if f.runInline {
		f.mu.Unlock()
		fn()
		return
	}
	f.queue = append(f.queue, fn)
	f.mu.Unlock()
}
func (f *fakeBackend) flush() {
	f.mu.Lock()
	q := f.queue
	f.queue = nil
	f.mu.Unlock()
	for _, fn := range q {
		fn()
	}
}

type countView struct {
	draws, closes int
	onDraw        func(*Frame, *Canvas)
}

func (v *countView) Draw(f *Frame, c *Canvas) {
	v.draws++
	if v.onDraw != nil {
		v.onDraw(f, c)
	}
}
func (v *countView) Mouse(MouseEvent) {}
func (v *countView) Key(KeyEvent)     {}
func (v *countView) Close()           { v.closes++ }

var testOpts = Options{Name: "Lite", Title: "Lite Counter", Width: 360, Height: 260}

func newFakeApp(t *testing.T, v View) (*App, *fakeBackend) {
	t.Helper()
	be := &fakeBackend{runInline: true}
	a, err := newWithBackend(testOpts, v, be)
	if err != nil {
		t.Fatal(err)
	}
	return a, be
}

func TestLifecycleRunStopClose(t *testing.T) {
	v := &countView{}
	a, be := newFakeApp(t, v)
	if be.opens != 0 {
		t.Fatal("New must not touch native APIs")
	}
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	if be.opens != 1 || be.loops != 1 {
		t.Fatalf("opens=%d loops=%d, want 1/1", be.opens, be.loops)
	}
	a.Stop()
	a.Stop()
	a.handleClose() // the window's own close notification after Stop
	if be.closes != 1 || v.closes != 1 {
		t.Fatalf("closes native=%d view=%d, want 1/1", be.closes, v.closes)
	}
	a.Invalidate()
	if be.needs != 0 {
		t.Fatal("Invalidate after Stop must not schedule a frame")
	}
}

func TestLifecycleUserCloseCallsViewOnce(t *testing.T) {
	v := &countView{}
	a, be := newFakeApp(t, v)
	_ = a.Run()
	a.handleClose()
	a.handleClose()
	a.Stop()
	if v.closes != 1 || be.closes != 0 {
		t.Fatalf("view closes=%d native closes=%d, want 1/0 (window already closed)", v.closes, be.closes)
	}
}

func TestDrawFrameNoReentry(t *testing.T) {
	var a *App
	v := &countView{}
	v.onDraw = func(*Frame, *Canvas) {
		a.Invalidate()
		a.drawFrame(100, 100, 1) // re-entrant draw must be ignored
	}
	a, be := newFakeApp(t, v)
	a.drawFrame(100, 100, 1)
	if v.draws != 1 {
		t.Fatalf("draws = %d, want 1", v.draws)
	}
	if be.needs != 1 {
		t.Fatalf("setNeedsDisplay = %d, want exactly 1 follow-up", be.needs)
	}
	if len(be.presents) != 1 {
		t.Fatalf("presents = %d, want 1", len(be.presents))
	}
}

func TestInvalidateFromGoroutine(t *testing.T) {
	a, be := newFakeApp(t, &countView{})
	be.runInline = false
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); a.Invalidate() }()
	}
	wg.Wait()
	be.flush()
	if be.needs != 1 {
		t.Fatalf("setNeedsDisplay = %d, want 1 (coalesced)", be.needs)
	}
	a.drawFrame(10, 10, 1) // clears pending
	a.Invalidate()
	be.flush()
	if be.needs != 2 {
		t.Fatalf("after a frame, Invalidate must schedule again: needs = %d", be.needs)
	}
}

func TestCanvasResizeAndZeroSize(t *testing.T) {
	v := &countView{}
	var got Frame
	v.onDraw = func(f *Frame, _ *Canvas) { got = *f }
	a, be := newFakeApp(t, v)
	a.drawFrame(100.5, 50, 2)
	if be.presents[0] != image.Rect(0, 0, 201, 100) {
		t.Fatalf("bitmap = %v, want 201x100", be.presents[0])
	}
	if got != (Frame{Width: 100.5, Height: 50, Scale: 2}) {
		t.Fatalf("frame = %+v", got)
	}
	first := a.img
	a.drawFrame(100.5, 50, 2)
	if a.img != first {
		t.Fatal("same size must reuse the bitmap")
	}
	a.drawFrame(60, 50, 1)
	if be.presents[2] != image.Rect(0, 0, 60, 50) {
		t.Fatalf("resized bitmap = %v", be.presents[2])
	}
	a.drawFrame(0, 50, 2)
	if v.draws != 3 || len(be.presents) != 3 {
		t.Fatalf("zero-size frame drew: draws=%d presents=%d", v.draws, len(be.presents))
	}
}

func TestCanvasFillBlendsAndClips(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	c := newCanvas(img, 2)
	c.Fill(Rect{X: 1, Y: 0, W: 5, H: 1}, Color{R: 255, A: 255}) // clipped at right edge
	if p := img.RGBAAt(3, 1); p.R != 255 || p.A != 255 {
		t.Fatalf("scaled fill pixel = %+v", p)
	}
	if p := img.RGBAAt(1, 1); p.A != 0 {
		t.Fatalf("outside pixel = %+v", p)
	}
	c.Fill(Rect{X: 1, Y: 0, W: 1, H: 1}, Color{B: 255, A: 128})
	p := img.RGBAAt(2, 0)
	if p.A != 255 || p.B != 128 || p.R < 126 || p.R > 128 {
		t.Fatalf("half-alpha blend = %+v, want premultiplied source-over", p)
	}
}

func TestRoundRectFill(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	newCanvas(img, 1).RoundRect(Rect{W: 10, H: 10}, 3, Color{R: 255, A: 255})
	if p := img.RGBAAt(5, 5); p.R != 255 || p.A != 255 {
		t.Fatalf("center = %+v", p)
	}
	if p := img.RGBAAt(0, 0); p.A != 0 {
		t.Fatalf("corner = %+v, want transparent", p)
	}
	if p := img.RGBAAt(0, 5); p.A != 255 {
		t.Fatalf("edge middle = %+v, want opaque", p)
	}
}
