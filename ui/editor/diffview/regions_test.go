package diffview

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

func lr(a, b int) diff.LineRange { return diff.LineRange{Start: a, End: b} }

func TestRegionTransitions(t *testing.T) {
	// A region of 30 unchanged lines at original 10, modified 12.
	base := region{orig: 10, mod: 12, n: 30}
	type step struct {
		name     string
		do       func(g *region)
		top, bot int
	}
	steps := []step{
		{"collapsed", func(g *region) {}, 0, 0},
		{"more above", func(g *region) { g.showMoreAbove(20) }, 20, 0},
		{"more below clamps", func(g *region) { g.showMoreBelow(20) }, 20, 10},
		{"more above clamps", func(g *region) { g.showMoreAbove(20) }, 20, 10},
		{"collapse", func(g *region) { g.collapse() }, 0, 0},
		{"show all", func(g *region) { g.showAll() }, 0, 30},
		{"collapse again", func(g *region) { g.collapse() }, 0, 0},
		{"line near top", func(g *region) { g.showLine(diff.Modified, 14, revealCloser) }, 3, 0},
		{"line near bottom", func(g *region) { g.showLine(diff.Original, 37, revealCloser) }, 3, 3},
		{"line from top", func(g *region) { g.showLine(diff.Original, 30, revealFromTop) }, 21, 3},
		{"visible line is a no-op", func(g *region) { g.showLine(diff.Original, 12, revealFromBottom) }, 21, 3},
		{"line from bottom", func(g *region) { g.showLine(diff.Modified, 34, revealFromBottom) }, 21, 8},
	}
	g := base
	for _, s := range steps {
		s.do(&g)
		if g.top != s.top || g.bottom != s.bot {
			t.Fatalf("%s: top %d bottom %d, want %d %d", s.name, g.top, g.bottom, s.top, s.bot)
		}
		ho, hm := g.hidden(diff.Original), g.hidden(diff.Modified)
		if ho.Len() != hm.Len() || ho.Start-10 != hm.Start-12 || ho.Len() != 30-s.top-s.bot {
			t.Fatalf("%s: hidden %v / %v", s.name, ho, hm)
		}
	}
	// Every line can be revealed from either side and preference.
	for _, pref := range []revealPreference{revealCloser, revealFromTop, revealFromBottom} {
		for l := 12; l < 42; l++ {
			g := base
			g.showLine(diff.Modified, l, pref)
			if g.hidden(diff.Modified).Contains(l) || g.top+g.bottom > g.n {
				t.Fatalf("pref %d line %d: %+v", pref, l, g)
			}
		}
	}
}

func TestRegionsOf(t *testing.T) {
	a := make([]string, 40)
	for i := range a {
		a[i] = "l" + string(rune('a'+i%26))
	}
	b := slices.Clone(a)
	b[20] = "changed"
	res := diffLines(a, b)
	got := regionsOf(res, 3, 3)
	want := []region{{orig: 0, mod: 0, n: 17}, {orig: 24, mod: 24, n: 16}}
	if !slices.Equal(got, want) {
		t.Fatalf("regions %+v, want %+v", got, want)
	}
	rs := regionSet{regions: got}
	rs.ensureVisible(diff.Original, 5, revealCloser)
	if rs.regions[0].hidden(diff.Original).Contains(5) {
		t.Fatalf("line 5 still hidden: %+v", rs.regions[0])
	}
	rs.showAll()
	if h := rs.hidden(diff.Modified); len(h) != 0 {
		t.Fatalf("show all left %v hidden", h)
	}
	rs.collapseAll()
	if h := rs.hidden(diff.Modified); !slices.Equal(h, []diff.LineRange{lr(0, 17), lr(24, 40)}) {
		t.Fatalf("collapse all: %v", h)
	}
}

func TestMigrate(t *testing.T) {
	cases := []struct {
		name   string
		new    []region
		hidden []hiddenMap
		first  bool
		want   []region
	}{
		{"first result collapses everything", []region{{orig: 0, mod: 0, n: 10}}, nil, true,
			[]region{{orig: 0, mod: 0, n: 10}}},
		{"previously visible stays visible", []region{{orig: 0, mod: 0, n: 10}}, nil, false,
			[]region{{orig: 0, mod: 0, n: 10, bottom: 10}}},
		{"same hidden range keeps its reveal counts",
			[]region{{orig: 5, mod: 7, n: 30}},
			[]hiddenMap{{orig: lr(10, 30), mod: lr(12, 32)}}, false,
			[]region{{orig: 5, mod: 7, n: 30, top: 5, bottom: 5}}},
		{"hidden range clipped to the new region",
			[]region{{orig: 5, mod: 7, n: 10}},
			[]hiddenMap{{orig: lr(0, 30), mod: lr(2, 32)}}, false,
			[]region{{orig: 5, mod: 7, n: 10}}},
		{"two hidden ranges split the region",
			[]region{{orig: 0, mod: 0, n: 30}},
			[]hiddenMap{{orig: lr(2, 8), mod: lr(2, 8)}, {orig: lr(20, 25), mod: lr(20, 25)}}, false,
			[]region{{orig: 0, mod: 0, n: 8, top: 2}, {orig: 8, mod: 8, n: 22, top: 12, bottom: 5}}},
		{"hidden range of another alignment is ignored",
			[]region{{orig: 0, mod: 3, n: 10}},
			[]hiddenMap{{orig: lr(2, 8), mod: lr(2, 8)}}, false,
			[]region{{orig: 0, mod: 3, n: 10, bottom: 10}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := migrate(c.new, c.hidden, c.first)
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestMapHidden(t *testing.T) {
	text := strings.Repeat("line\n", 30)
	old := buffer.NewSnapshot(text)
	hs := []hiddenMap{{orig: lr(5, 15), mod: lr(8, 18)}, {orig: lr(20, 25), mod: lr(23, 28)}}
	// On the modified side: replace line 10 by two lines, and insert a line
	// at the top.
	edits := []buffer.Edit{
		{Start: 0, End: 0, Text: "new\n"},
		{Start: old.LineStart(10), End: old.LineStart(10) + 4, Text: "a\nb"},
	}
	got := mapHidden(hs, diff.Modified, old, edits)
	want := []hiddenMap{
		{orig: lr(5, 7), mod: lr(9, 11)},
		{orig: lr(8, 15), mod: lr(13, 20)},
		{orig: lr(20, 25), mod: lr(25, 30)},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// TestRegionsFollowEdits runs the whole pipeline a diff view runs while
// the user types: map the hidden ranges and the last result through the
// edit, recompute the regions and migrate their state. Hidden lines must
// stay unchanged lines of the new texts, equal on both sides.
func TestRegionsFollowEdits(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 9))
	for iter := 0; iter < 1000; iter++ {
		a := randLines(r, 20+r.IntN(40))
		b := mutate(r, a)
		res := diffLines(a, b)
		rs := regionSet{regions: migrate(regionsOf(res, 3, 3), nil, true)}
		for i := range rs.regions {
			switch r.IntN(4) {
			case 0:
				rs.regions[i].showMoreAbove(r.IntN(5))
			case 1:
				rs.regions[i].showMoreBelow(r.IntN(5))
			case 2:
				rs.regions[i].showAll()
			}
		}
		side := diff.Side(r.IntN(2))
		texts := [2][]string{a, b}
		old := buffer.NewSnapshot(strings.Join(texts[side], "\n"))
		edits := randEdits(r, old)
		ns, _ := old.Apply(edits)
		texts[side] = split(ns.String())
		hidden := mapHidden(rs.hiddenMaps(), side, old, edits)
		mapped := applyEdits(res, side, old, edits)
		next := migrate(regionsOf(mapped, 3, 3), hidden, false)
		for _, g := range next {
			if g.top < 0 || g.bottom < 0 || g.top+g.bottom > g.n {
				t.Fatalf("iter %d: bad region %+v", iter, g)
			}
			ho, hm := g.hidden(diff.Original), g.hidden(diff.Modified)
			for k := 0; k < ho.Len(); k++ {
				if texts[0][ho.Start+k] != texts[1][hm.Start+k] {
					t.Fatalf("iter %d: hidden pair %d/%d differs", iter, ho.Start+k, hm.Start+k)
				}
			}
			// Previously hidden lines that survived the edit stay hidden.
			for _, h := range hidden {
				for l := h.mod.Start; l < h.mod.End; l++ {
					in := lr(g.mod, g.mod+g.n).Contains(l) && h.orig.Start-h.mod.Start == g.orig-g.mod
					if in && !hm.Contains(l) {
						t.Fatalf("iter %d: line %d was hidden, now shown in %+v (hidden %+v)", iter, l, g, hidden)
					}
				}
			}
		}
	}
}
