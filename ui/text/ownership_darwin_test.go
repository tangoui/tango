//go:build darwin && (ios || tango_gotext)

package text

import (
	"bytes"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/go-text/typesetting/font"
	"github.com/tangoui/tango/ui/paint"
)

type ctReleaseLog struct {
	mu sync.Mutex
	n  map[uintptr]int
}

func (l *ctReleaseLog) release(f uintptr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n == nil {
		l.n = map[uintptr]int{}
	}
	l.n[f]++
}

func (l *ctReleaseLog) count(f uintptr) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n[f]
}

func (l *ctReleaseLog) total() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	var n int
	for _, count := range l.n {
		n += count
	}
	return n
}

// GC and cleanup scheduling are intentionally separate; GC alone does not
// promise that a cleanup has finished. Keep polling bounded and local to these
// ownership tests, not to the public trim operation.
func waitCTCleanup(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for CoreText ownership cleanup")
		}
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
}

func TestCTOwnershipUnbuiltLazyFace(t *testing.T) {
	log := &ctReleaseLog{}
	f := func() *Face {
		owner := newCTFontOwner(1, log.release)
		return newLazyEngineFace("unused", Regular, false, func() (*font.Face, glyphEngine, func(uint32) *paint.Path) {
			defer runtime.KeepAlive(owner)
			panic("unused lazy face must not build")
		})
	}()
	runtime.GC()
	if n := log.count(1); n != 0 {
		t.Fatalf("live lazy face released its font %d times", n)
	}
	runtime.KeepAlive(f)
	f = nil
	waitCTCleanup(t, func() bool { return log.count(1) == 1 })
}

func TestCTOwnershipAllRetainsReleasedOnce(t *testing.T) {
	log := &ctReleaseLog{}
	func() {
		owner := newCTFontOwner(1, log.release)
		owner.desc = 2
		owner.unit = 3
		owner.sized[ctSizeKey{size: 12}] = 4
		// Create/Copy may return the same address with another retain. Do not
		// deduplicate releases by pointer value.
		owner.sized[ctSizeKey{size: 14}] = 1
		owner.sized[ctSizeKey{size: 16}] = 0
		runtime.KeepAlive(owner)
	}()
	waitCTCleanup(t, func() bool { return log.count(1) == 2 && log.count(2) == 1 && log.count(3) == 1 && log.count(4) == 1 })
	if n := log.count(0); n != 0 {
		t.Fatalf("released a nil CF reference %d times", n)
	}
}

func TestCTOwnershipFailedBuildReleaseIsIdempotent(t *testing.T) {
	log := &ctReleaseLog{}
	func() {
		owner := newCTFontOwner(1, log.release)
		owner.release()
		owner.release()
		if n := log.count(1); n != 1 {
			t.Fatalf("failed build released its font %d times", n)
		}
		runtime.KeepAlive(owner)
	}()
	// A second sentinel verifies that a later cleanup pass has run without
	// depending on the implementation's cleanup ordering.
	func() { _ = newCTFontOwner(2, log.release) }()
	waitCTCleanup(t, func() bool { return log.count(2) == 1 })
	if n := log.count(1); n != 1 {
		t.Fatalf("cleanup released an already failed build again: %d", n)
	}
}

func TestCTOwnershipRasterAndMethodRetainOwner(t *testing.T) {
	log := &ctReleaseLog{}
	r := &ctRaster{ctFontOwner: newCTFontOwner(1, log.release)}
	path := r.unitPath
	r = nil
	runtime.GC()
	if n := log.count(1); n != 0 {
		t.Fatalf("bound outline method lost its owner: %d releases", n)
	}
	runtime.KeepAlive(path)
	path = nil
	waitCTCleanup(t, func() bool { return log.count(1) == 1 })
}

// Track the release function captured by owners created during e.load. These
// tests are deliberately not parallel: all wrapped native functions still call
// the real implementation, but installing the package-level hooks is serial.
func trackCTReleases(t *testing.T) *ctReleaseLog {
	t.Helper()
	log := &ctReleaseLog{}
	release := cfRelease
	cfRelease = func(p uintptr) {
		release(p)
		log.release(p)
	}
	t.Cleanup(func() { cfRelease = release })
	return log
}

func TestCTOwnershipEngineUnbuiltFonts(t *testing.T) {
	coreTextOn(t)
	log := trackCTReleases(t)
	var want int
	func() {
		faces := (&ctEngine{}).load(ctSystemUI)
		if len(faces) != 10 {
			t.Fatalf("system UI lazy faces = %d, want 5 weights * 2 slants", len(faces))
		}
		before := log.total()
		runtime.GC()
		if n := log.total(); n != before {
			t.Fatalf("reachable lazy faces released native fonts: %d -> %d", before, n)
		}
		want = before + len(faces)
		for _, f := range faces {
			if f.build == nil || f.face != nil {
				t.Fatal("test unexpectedly built a lazy face")
			}
		}
		runtime.KeepAlive(faces)
	}()
	waitCTCleanup(t, func() bool { return log.total() == want })
}

func TestCTOwnershipEngineBuildFailure(t *testing.T) {
	coreTextOn(t)
	log := trackCTReleases(t)
	faces := (&ctEngine{}).load(ctSystemUI)
	if len(faces) != 10 {
		t.Fatalf("system UI lazy faces = %d, want 10", len(faces))
	}
	copyTables := ctFontCopyAvailableTables
	ctFontCopyAvailableTables = func(uintptr, uint32) uintptr { return 0 }
	t.Cleanup(func() { ctFontCopyAvailableTables = copyTables })
	before := log.total()
	for _, f := range faces {
		f.ready()
		if !f.broken {
			t.Fatal("face built with missing shaping tables")
		}
		f.ready() // sync.Once must not repeat release on another access.
	}
	if n := log.total(); n != before+len(faces) {
		t.Fatalf("failed builds released %d fonts, want %d", n-before, len(faces))
	}
	runtime.GC()
	if n := log.total(); n != before+len(faces) {
		t.Fatalf("failed-build cleanup repeated a release: %d", n-before)
	}
	runtime.KeepAlive(faces)
}

func TestCTOwnershipRetainedLayoutSurvivesTrim(t *testing.T) {
	coreTextOn(t)
	log := &ctReleaseLog{}
	var base uintptr
	func() {
		layout := Shape("A", Style{Size: 19}, Options{NoWrap: true})
		if len(layout.Glyphs) != 1 {
			t.Fatalf("glyphs = %d, want 1", len(layout.Glyphs))
		}
		g := layout.Glyphs[0]
		f := g.Src.(*Face)
		r := f.ras.(*ctRaster)
		base = r.base
		release := cfRelease
		r.releaseRef = func(p uintptr) {
			release(p)
			log.release(p)
		}
		mask, offset, ok := f.RasterGlyph(g.ID, 19, 0, 0)
		outline := r.unitPath(g.ID)
		if !ok || mask == nil || outline == nil {
			t.Fatal("missing initial raster or outline")
		}
		for i := 0; i < 3; i++ {
			TrimFontCaches()
			runtime.GC()
			if n := log.count(base); n != 0 {
				t.Fatalf("trim released a retained Layout's base font %d times", n)
			}
			got, at, ok := f.RasterGlyph(g.ID, 19, 0, 0)
			path := r.unitPath(g.ID)
			if !ok || got == nil || at != offset || !bytes.Equal(got.Pix, mask.Pix) || path == nil || path.Bounds() != outline.Bounds() {
				t.Fatal("retained Layout's raster or outline changed after trim")
			}
			if fresh := Shape("A", Style{Size: 19}, Options{NoWrap: true}); fresh.Glyphs[0].Src == g.Src {
				t.Fatal("trim did not create a distinct font chain")
			}
		}
		runtime.KeepAlive(layout)
	}()
	waitCTCleanup(t, func() bool { return log.count(base) == 1 })
}

func TestCTOwnershipConcurrentNativeUse(t *testing.T) {
	coreTextOn(t)
	r, id := func() (*ctRaster, uint32) {
		layout := Shape("A", Style{Size: 19}, Options{NoWrap: true})
		g := layout.Glyphs[0]
		return g.Src.(*Face).ras.(*ctRaster), g.ID
	}()
	TrimFontCaches() // Only the local raster, not the chain, keeps its owner alive.
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				if m, _, ok := r.raster(id, float32(12+i), 1, 0.1); !ok || m == nil {
					t.Error("concurrent raster failed")
				}
				if r.unitPath(id) == nil || r.advance(id, 19) <= 0 {
					t.Error("concurrent outline or advance failed")
				}
			}
		}()
	}
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	wg.Wait()
	runtime.KeepAlive(r)
}

func TestCTOwnershipSizedEviction(t *testing.T) {
	coreTextOn(t)
	var log *ctReleaseLog
	want := map[uintptr]int{}
	func() {
		r := newCTRaster(uiFontWeighted(Regular, false, 17), nil)
		// Begin after constructor temporaries have been released: native
		// allocators may reuse their addresses for a later sized font.
		log = trackCTReleases(t)
		r.releaseRef = cfRelease
		r.mu.Lock()
		for size := 100; size < 166; size++ {
			f := r.font(float32(size), 0)
			if f == 0 {
				r.mu.Unlock()
				t.Fatal("sized font creation failed")
			}
			if size < 165 {
				want[f]++
			}
		}
		r.mu.Unlock()
		for f, n := range want {
			if got := log.count(f); got != n {
				t.Fatalf("evicted sized font released %d times, want %d", got, n)
			}
		}
		if len(r.sized) != 1 || log.total() != 65 {
			t.Fatalf("after eviction: cached=%d, releases=%d", len(r.sized), log.total())
		}
		for _, f := range r.sized {
			want[f]++
		}
		want[r.base]++
		runtime.KeepAlive(r)
	}()
	waitCTCleanup(t, func() bool { return log.total() == 67 })
	for f, n := range want {
		if got := log.count(f); got != n {
			t.Errorf("sized/base font released %d times after cleanup, want %d", got, n)
		}
	}
}

func TestCTOwnershipRealRasterCleanup(t *testing.T) {
	coreTextOn(t)
	log := &ctReleaseLog{}
	var retain, retainCount func(uintptr) uintptr
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		t.Fatal(err)
	}
	defer purego.Dlclose(cf)
	purego.RegisterLibFunc(&retain, cf, "CFRetain")
	purego.RegisterLibFunc(&retainCount, cf, "CFGetRetainCount")

	// Hold one observer retain on each object so reading the count after
	// cleanup is safe even if CoreText does not cache the object.
	var handles []uintptr
	var before []uintptr
	func() {
		f := uiFontWeighted(Regular, false, 17)
		if f == 0 {
			t.Fatal("no system UI font")
		}
		r := newCTRaster(f, map[uint32]float32{wghtTag: 400, opszTag: 17})
		release := cfRelease
		r.ctFontResources.releaseRef = func(p uintptr) {
			release(p)
			log.release(p)
		}
		r.mu.Lock()
		sized := r.font(19, 0.1)
		r.mu.Unlock()
		_ = r.unitPath(36)
		if r.desc == 0 || sized == 0 || r.unit == 0 {
			t.Fatal("did not populate every native raster resource")
		}
		for _, p := range []uintptr{r.base, r.desc, sized, r.unit} {
			retain(p)
			handles = append(handles, p)
			before = append(before, retainCount(p))
		}
		runtime.KeepAlive(r)
	}()
	defer func() {
		for _, p := range handles {
			cfRelease(p)
		}
	}()
	waitCTCleanup(t, func() bool {
		for _, p := range handles {
			if log.count(p) != 1 {
				return false
			}
		}
		return true
	})
	for i, p := range handles {
		// CoreText may release additional internal references as dependent
		// fonts die. The observed retain count must at least decrease by one.
		if after := retainCount(p); after >= before[i] {
			t.Errorf("resource %#x retain count %d -> %d", p, before[i], after)
		}
	}
}
