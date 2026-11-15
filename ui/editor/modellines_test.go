package editor

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// rowGlyphs returns the glyph IDs and the colors of the Text ops drawn on
// the row at y (in the text area right of x0), in order.
func rowGlyphs(ops []paint.Op, y, x0 float32) (ids []uint32, cols []uint32) {
	for _, op := range ops {
		if t, ok := op.(paint.Text); ok && t.Origin.Y == y && t.Origin.X >= x0 {
			for _, g := range t.Glyphs {
				ids = append(ids, g.ID)
				cols = append(cols, uint32(t.Color.R)|uint32(t.Color.G)<<8|uint32(t.Color.B)<<16)
			}
		}
	}
	return ids, cols
}

// TestModelLines: a zone drawn with ModelLines shows another Model's lines
// with the same glyphs and colors as a view of that Model; its version is
// 0 until the lines' tokens are final, then stable, and changes with the
// text and the decorations.
func TestModelLines(t *testing.T) {
	goText(t)
	th := theme.Base("vs")
	src := NewModel("orig.go", "package main\n\nfunc main() {\n\treturn 42 // answer\n}\n", DefaultRegistry())
	_, v0 := ModelLines(src, 2, 5, nil)
	if v0 != 0 {
		t.Fatalf("version %d before highlighting, want 0", v0)
	}
	// The source drawn by a view of its own.
	ref := NewView(src, th)
	ref.Options.LineNumbers = LineNumbersOff
	ref.Options.BracketPairColorization = false // not drawn by ModelLines (as Monaco's inline diff)
	rh := newHintHost(t, ref, geom.Size{W: 480, H: 200}, 1)
	rh.frame()
	rh.frame()
	refOps := rh.r.List().Ops
	rg := rh.vw.vs
	lh := ref.Viewport.LineHeight

	// A zone in another view draws lines 2..4.
	host := NewState("a\nb\nc\n", "host.txt", th)
	host.Options.LineNumbers = LineNumbersOff
	paintFn, v1 := ModelLines(src, 2, 5, nil)
	if v1 == 0 {
		t.Fatal("version 0 after the view highlighted the lines")
	}
	if _, again := ModelLines(src, 2, 5, nil); again != v1 {
		t.Fatal("version not stable")
	}
	host.SetViewZones("diff", []ViewZone{{ID: 1, AfterLine: 0, Rows: 3, Version: v1, Paint: paintFn}})
	hh := newHintHost(t, host, geom.Size{W: 480, H: 200}, 1)
	hh.frame()
	hh.frame()
	hg := hh.vw.vs
	hostOps := hh.r.List().Ops
	if hg.text.X != rg.text.X {
		t.Fatalf("text areas differ: %v, %v", hg.text, rg.text)
	}
	for k := range 3 {
		want, wantCols := rowGlyphs(refOps, rg.text.Y+float32(2+k)*lh, rg.text.X)
		got, gotCols := rowGlyphs(hostOps, hg.text.Y+float32(1+k)*lh, hg.text.X)
		if len(want) == 0 && k != 2 {
			t.Fatalf("reference line %d drew nothing", 2+k)
		}
		if !slices.Equal(got, want) || !slices.Equal(gotCols, wantCols) {
			t.Fatalf("line %d: zone glyphs %v colors %x, view's %v %x", 2+k, got, gotCols, want, wantCols)
		}
	}
	// Decorations: the background and the inline color.
	red := &DecorationOptions{Background: decoRed, Inline: &InlineStyle{Color: decoPurple}}
	snap := src.Snapshot()
	ds := []Decoration{{Start: snap.LineStart(3) + 1, End: snap.LineStart(3) + 7, Options: red}}
	paintFn, v2 := ModelLines(src, 2, 5, ds)
	if v2 == v1 || v2 == 0 {
		t.Fatalf("version with decorations %d (without %d)", v2, v1)
	}
	host.SetViewZones("diff", []ViewZone{{ID: 1, AfterLine: 0, Rows: 3, Version: v2, Paint: paintFn}})
	hh.frame()
	ops := hh.r.List().Ops
	if len(fillsOf(ops, decoRed)) != 1 || len(textsOf(ops, decoPurple)) != 1 {
		t.Fatalf("decorations: %d boxes, %d inline runs", len(fillsOf(ops, decoRed)), len(textsOf(ops, decoPurple)))
	}
	// Another text: another version.
	ref.MoveCaret(snap.LineStart(3), false)
	ref.Insert("x")
	if _, v3 := ModelLines(src, 2, 5, ds); v3 == v2 && v3 != 0 {
		t.Fatal("version unchanged by an edit")
	}
}
