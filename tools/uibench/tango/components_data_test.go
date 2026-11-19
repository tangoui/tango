package main

import (
	"bytes"
	"os"
	"regexp"
	"slices"
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
	ta, reset := seed, false
	for f := 1; f <= textareaMax; f++ {
		next := appendText(ta, seed, f)
		if next == seed {
			if len(ta) != textareaMax-1 {
				t.Errorf("text area reset at %d characters, want %d", len(ta)+1, textareaMax)
			}
			reset = true
			break
		}
		if len(next) != len(ta)+1 {
			t.Fatalf("frame %d: text area went from %d to %d characters", f, len(ta), len(next))
		}
		ta = next
	}
	if !reset {
		t.Errorf("text area never reset to its seed; it reached %d characters", len(ta))
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

// sceneRow matches a scene table entry in main.go: "name": {w, h, ...
var sceneRow = regexp.MustCompile(`(?m)^\s*"([a-z-]+)":\s*\{(\d+), (\d+),`)

// sceneTable reads the scene names and sizes of a program's main.go.
func sceneTable(t *testing.T, path string) map[string]string {
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range sceneRow.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = m[2] + "x" + m[3]
	}
	return out
}

// The three programs offer the same scenes at the same window sizes: the
// counter at 360x260, every other scene at 800x600.
func TestSceneTablesMatch(t *testing.T) {
	want := sceneTable(t, "main.go")
	if len(want) != len(scenes()) {
		t.Fatalf("read %d scenes from main.go, scenes() has %d", len(want), len(scenes()))
	}
	for name, size := range want {
		if exp := map[bool]string{true: "360x260", false: "800x600"}[name == "counter"]; size != exp {
			t.Errorf("tango %s is %s, want %s", name, size, exp)
		}
	}
	names := func(m map[string]string) []string {
		var s []string
		for k := range m {
			s = append(s, k)
		}
		slices.Sort(s)
		return s
	}
	for _, other := range []string{"../mygo/main.go", "../nsgo/main.go"} {
		got := sceneTable(t, other)
		if !slices.Equal(names(got), names(want)) {
			t.Errorf("%s scenes %v, tango %v", other, names(got), names(want))
			continue
		}
		for name, size := range want {
			if got[name] != size {
				t.Errorf("%s %s is %s, tango %s", other, name, got[name], size)
			}
		}
	}
}
