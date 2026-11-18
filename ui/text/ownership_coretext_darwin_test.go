//go:build darwin && !ios && !tango_gotext

package text

import (
	"sync"
	"testing"

	"github.com/ebitengine/purego"
)

// A face that loses the faceForCTFont registration race releases its
// CTFont at once; the winner's stays alive.
func TestCTOwnershipFaceRaceLoserReleased(t *testing.T) {
	f := ctTestFont(t, "Menlo")
	saveUserFonts(t) // starts with an empty Face table
	var mu sync.Mutex
	released := map[uintptr]int{}
	old := ctFaceRelease
	ctFaceRelease = func(p uintptr) {
		mu.Lock()
		released[p]++
		mu.Unlock()
		cfRelease(p)
	}
	t.Cleanup(func() { ctFaceRelease, ctFaceRaceHook = old, nil })
	var winner *Face
	ctFaceRaceHook = func() {
		ctFaceRaceHook = nil // the inner call registers first
		winner = faceForCTFont(f)
	}
	got := faceForCTFont(f)
	if winner == nil || got != winner {
		t.Fatalf("faceForCTFont = %p, want the face registered first %p", got, winner)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(released) == 0 {
		t.Fatal("the losing face released nothing")
	}
	// CoreText may unique equal copies, so the loser's CTFont can be the
	// winner's pointer: each holds its own reference, released once.
	for p, n := range released {
		if n != 1 {
			t.Errorf("CTFont %#x released %d times", p, n)
		}
	}
	if _, _, ok := winner.RasterGlyph(ctGlyph(t, winner, 'm'), 16, 0, 0); !ok {
		t.Error("winning face cannot rasterize")
	}
}

// shapeItems releases every CTFont it takes from Face.ctFont: shaping the
// same text 1000 times leaves the sized fonts' retain counts bounded (a
// reference kept per shape would add 1000; CoreText's own caches may hold
// a few dozen at most).
func TestCTOwnershipShapeReleasesSizedFonts(t *testing.T) {
	needCT(t)
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		t.Skip(err)
	}
	var retainCount func(uintptr) int64
	purego.RegisterLibFunc(&retainCount, cf, "CFGetRetainCount")

	st := Style{Size: 17}.norm()
	fm := &fontmap{chain: reg.chain(false), w: st.Weight, eng: reg.engineFallback()}
	runes := []rune("Hello 中文 עברית 😀")
	adv := make([]float32, len(runes)+1)
	mu.Lock()
	shapeRange(runes, 0, len(runes), st, fm, adv) // builds the sized copies
	mu.Unlock()
	var fonts []uintptr
	seen := map[*Face]bool{}
	for _, r := range runes {
		if f, _ := fm.resolve(r); f != nil && !seen[f] {
			seen[f] = true
			if c := f.ctFont(st.Size); c != 0 {
				fonts = append(fonts, c)
			}
		}
	}
	defer func() {
		for _, c := range fonts {
			cfRelease(c)
		}
	}()
	if len(fonts) < 2 {
		t.Fatalf("only %d faces resolved", len(fonts))
	}
	before := make([]int64, len(fonts))
	for i, c := range fonts {
		before[i] = retainCount(c)
	}
	shapes := 1000
	if testing.Short() {
		shapes = 200
	}
	mu.Lock()
	for range shapes {
		clear(adv)
		shapeRange(runes, 0, len(runes), st, fm, adv)
	}
	mu.Unlock()
	for i, c := range fonts {
		t.Logf("font %d: retain count %d -> %d", i, before[i], retainCount(c))
		if n := retainCount(c); n-before[i] > 64 {
			t.Errorf("font %d: retain count %d after 1000 shapes, %d before", i, n, before[i])
		}
	}
}
