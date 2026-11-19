//go:build !darwin || ios || tango_gotext

package text

import (
	"testing"

	"github.com/go-text/typesetting/font"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/tangoui/tango/ui/paint"
)

// A system family registers every style it has, but only the faces a
// layout picks parse their tables (megabytes each for CJK and variable
// fonts).
func TestLazyEngineFacesBuildWhenPicked(t *testing.T) {
	built := map[Weight]int{}
	lazy := func(w Weight, ok bool) *Face {
		return newLazyEngineFace("Lazy", w, false, func() (*font.Face, glyphEngine, func(uint32) *paint.Path) {
			built[w]++
			if !ok {
				return nil, nil, nil
			}
			gt, _, err := goTextFace(goregular.TTF)
			if err != nil {
				t.Fatal(err)
			}
			return gt, nil, nil
		})
	}
	fm := &family{faces: []*Face{lazy(Light, true), lazy(Regular, true), lazy(Bold, true)}, loaded: true}
	if len(built) != 0 {
		t.Fatalf("faces built before use: %v", built)
	}
	f, _ := fm.pick(Regular, false)
	if f == nil || f.Weight != Regular || !f.HasRune('a') {
		t.Fatalf("picked %+v", f)
	}
	if asc, desc := f.Metrics(16); asc <= 0 || desc <= 0 {
		t.Fatalf("metrics %v %v", asc, desc)
	}
	f.HasRune('b')
	if built[Regular] != 1 || built[Light] != 0 || built[Bold] != 0 {
		t.Fatalf("built %v, want only Regular once", built)
	}
}

// A face whose tables cannot be read is dropped, and matching goes on
// with the other weights.
func TestLazyEngineFaceThatFailsIsDropped(t *testing.T) {
	ok := newLazyEngineFace("Lazy", Bold, false, func() (*font.Face, glyphEngine, func(uint32) *paint.Path) {
		gt, _, err := goTextFace(goregular.TTF)
		if err != nil {
			t.Fatal(err)
		}
		return gt, nil, nil
	})
	bad := newLazyEngineFace("Lazy", Regular, false, func() (*font.Face, glyphEngine, func(uint32) *paint.Path) {
		return nil, nil, nil
	})
	fm := &family{faces: []*Face{bad, ok}, loaded: true}
	if f, _ := fm.pick(Regular, false); f != ok {
		t.Fatalf("picked %+v, want the bold face", f)
	}
	if len(fm.faces) != 1 || fm.failed {
		t.Fatalf("faces %d failed %v", len(fm.faces), fm.failed)
	}
	only := &family{faces: []*Face{newLazyEngineFace("Lazy", Regular, false, func() (*font.Face, glyphEngine, func(uint32) *paint.Path) {
		return nil, nil, nil
	})}, loaded: true}
	if f, _ := only.pick(Regular, false); f != nil || !only.failed {
		t.Fatalf("picked %+v from a family that cannot be built", f)
	}
}
