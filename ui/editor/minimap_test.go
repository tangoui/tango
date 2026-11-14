package editor

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestMinimapInvalidUTF8Prefix(t *testing.T) {
	if got := miniatureLine("a\xff", 120, 4); got != "a\xff" {
		t.Fatalf("invalid UTF-8 prefix %q", got)
	}
	if got := miniatureLine("a\xffz", 2, 4); got != "a\xff" {
		t.Fatalf("bounded invalid UTF-8 prefix %q", got)
	}
}

func TestMinimapColors(t *testing.T) {
	goText(t)
	th := theme.Base("vs-dark")
	st := NewState("package main\nvar x = \"hello\"\n", "a.go", th)
	defer st.Close()
	st.Options.Minimap.ShowSlider = MinimapSliderMouseOver
	// Blocks draw the token colors exactly; the characters blend them
	// with the background (TestMinimapCharacters checks their colors).
	st.Options.Minimap.RenderCharacters = false
	d, vw := viewDriver(st, 640, 300, nil)
	var colors = map[color.NRGBA]bool{}
	for range 4 {
		d.Frame()
	}
	img := d.Frame()
	for _, ln := range (*vw).vs.mini.cache {
		for _, b := range ln.blocks {
			colors[b.color] = true
		}
	}
	toks, _ := st.Highlighter().TokensFor(st.hlKey, st.Snapshot(), 1, nil)
	for _, tok := range toks {
		fg := th.Style(tok.Type).Foreground
		if fg.A > 0 && !colors[fg] {
			t.Errorf("token color %v absent", fg)
		}
	}
	if len(colors) < 2 {
		t.Fatal("minimap not token colored")
	}
	r := (*vw).vs.minimapRect
	found := 0
	for c := range colors {
		found += countColor(img, r, c)
	}
	if found == 0 {
		t.Fatal("no colored minimap pixels")
	}
	o := DefaultOptions().Minimap
	if !o.Enabled || o.Side != MinimapRight || o.Size != MinimapProportional || o.MaxColumn != 120 || !o.RenderCharacters || o.ShowSlider != MinimapSliderMouseOver {
		t.Fatalf("defaults %+v", o)
	}
}

func TestMinimapDragScroll(t *testing.T) {
	goText(t)
	st := NewState(manyLines(1000), "a.txt", theme.Base("vs"))
	defer st.Close()
	st.Options.Minimap.ShowSlider = MinimapSliderAlways
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	r := (*vw).vs.minimapRect
	want := (*vw).vs.miniRows[int((r.H-10)/2)].vrow
	d.Click(geom.Pt(r.X+4, r.Bottom()-10))
	d.Frame()
	if st.Viewport.TopLine != want {
		t.Fatalf("click top %d want exact painted row %d", st.Viewport.TopLine, want)
	}
	slider := (*vw).vs.minimapSlider
	p := geom.Pt(slider.X+3, slider.Y+slider.H/2)
	d.Inject(input.Pointer{Kind: input.Press, Pos: p})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(p.X, r.Y+10)})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(p.X, r.Y+10)})
	d.Frame()
	if st.Viewport.TopLine != 0 {
		t.Fatalf("drag top %d", st.Viewport.TopLine)
	}
}

func TestMinimapTracksOwnLineFinality(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("var x = 1\n", 200), "a.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	hl.TokenWindow = 10
	hl.HighlightFor(st.hlKey, st.Snapshot(), 100, time.Second)
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	// This is a token-finality unit test, not a frame-budget test. A fixed
	// number of rasterized race frames does not guarantee spare token time.
	// Drive the cache builder separately; the exhausted-budget policy has
	// its own test below and the production frame still uses exactly 3ms.
	(*vw).prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	ln := (*vw).vs.mini.cache[100]
	if ln == nil {
		t.Fatal("missing minimap line")
	}
	if ln.final {
		t.Fatal("dirty line incorrectly cached as final just because its start state is final")
	}
	hl.HighlightFor(st.hlKey, st.Snapshot(), 201, time.Second)
	(*vw).prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	if !(*vw).vs.mini.cache[100].final {
		t.Fatal("line finalization failed to invalidate miniature tokens")
	}
}

func TestMinimapDefersTokensWhenBudgetExhausted(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("var x = 1\n", 200), "a.go", theme.Base("vs"))
	defer st.Close()
	hl := st.Highlighter()
	hl.TokenWindow = 10
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	(*vw).vs.mini.cache = nil
	lo, hi := hl.WindowRangeFor(st.hlKey)
	(*vw).prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(-time.Second))
	if (*vw).vs.mini.cache[100] != nil {
		t.Fatal("uncached offscreen line tokenized after token deadline")
	}
	lo2, hi2 := hl.WindowRangeFor(st.hlKey)
	if lo2 != lo || hi2 != hi {
		t.Fatal("exhausted minimap work moved visible token window")
	}
}

func TestMinimapIncremental(t *testing.T) {
	goText(t)
	st := NewState(manyLines(80), "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	for range 8 {
		d.Frame()
	}
	before := (*vw).vs.mini.rebuilt
	off := st.Snapshot().LineStart(3)
	if err := st.Edit([]buffer.Edit{{Start: off, End: off + 1, Text: "X"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if got := (*vw).vs.mini.rebuilt - before; got != 1 {
		t.Fatalf("rebuilt %d lines, want 1", got)
	}
	before = (*vw).vs.mini.rebuilt
	d.Frame()
	if (*vw).vs.mini.rebuilt != before {
		t.Fatal("unchanged frame rebuilt cache")
	}
	st.Theme = theme.Base("vs-dark")
	d.Frame()
	if (*vw).vs.mini.rebuilt == before {
		t.Fatal("theme failed to invalidate")
	}
}
