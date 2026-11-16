package main

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// The three programs must share one copy of the component scenes' data.
func TestComponentsDataSameInEveryProgram(t *testing.T) {
	want, err := os.ReadFile("components_data.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []string{"../mygo/components_data.go", "../nsgo/components_data.go"} {
		got, err := os.ReadFile(other)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from tango/components_data.go", other)
		}
	}
}

func TestComponentsData(t *testing.T) {
	if len(inputSeed) != 20 {
		t.Errorf("input seed has %d characters, want 20", len(inputSeed))
	}
	if s := textareaSeed(); len(s) != 2000 {
		t.Errorf("text area seed has %d characters, want 2000", len(s))
	}
	// An input grows by one per frame typed into it and resets after 40.
	s := inputSeed
	for f := 1; f <= inputMax-len(inputSeed); f++ {
		if s = typeInto(s, f); len(s) != len(inputSeed)+f {
			t.Fatalf("after %d letters the input has %d characters", f, len(s))
		}
	}
	if s = typeInto(s, 0); s != inputSeed {
		t.Errorf("an input past %d characters is %q, want the seed", inputMax, s)
	}
	seed := textareaSeed()
	ta := seed
	for f := 1; len(ta) > 0 && f < textareaMax; f++ {
		next := appendText(ta, seed, f)
		if next == seed {
			if len(ta) != textareaMax-1 {
				t.Errorf("text area reset at %d characters, want %d", len(ta)+1, textareaMax)
			}
			break
		}
		ta = next
	}
	if v := nextPercent(100); v != 0 {
		t.Errorf("nextPercent(100) = %v, want 0", v)
	}
	if lo, hi := treeRow(0), treeRow(treeNodes-2*treeMargin-1); lo != treeMargin || hi != treeNodes-treeMargin-1 {
		t.Errorf("tree rows span %d..%d", lo, hi)
	}
	for f := 0; f < 62; f++ {
		if d := pickDay(f); d.Month() != time.March {
			t.Fatalf("pickDay(%d) = %v, not in March", f, d)
		}
	}
	// Hue 0 is red, 120 green, 240 blue, at saturation 0.75 and value 0.85.
	for _, c := range []struct {
		h       int
		r, g, b uint8
	}{{0, 217, 54, 54}, {120, 54, 217, 54}, {240, 54, 54, 217}, {60, 217, 217, 54}} {
		if r, g, b := hueRGB(c.h); r != c.r || g != c.g || b != c.b {
			t.Errorf("hueRGB(%d) = %d,%d,%d, want %d,%d,%d", c.h, r, g, b, c.r, c.g, c.b)
		}
	}
	for h := 0; h < 360; h++ {
		a1, a2, a3 := hueRGB(h)
		b1, b2, b3 := hueRGB(h + 1)
		if h < 359 && a1 == b1 && a2 == b2 && a3 == b3 {
			t.Errorf("hue %d and %d give the same color", h, h+1)
		}
	}
}
