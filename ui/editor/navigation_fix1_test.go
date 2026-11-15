package editor

import (
	"image"
	"image/color"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestStickyEntryAndPushOff(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("func scope() {\n", 40), "a.go", theme.Base("vs"))
	defer st.Close()
	p := &testProvider{rs: []FoldingRange{{Start: 0, End: 30}, {Start: 3, End: 8}}}
	d, vw := viewDriver(st, 640, 300, func(w *ViewW) { w.FoldingProvider(p) })
	d.Frame()
	lh := st.Viewport.LineHeight
	st.Viewport.setScrollY(float64(2.5*lh), st.viewCount())
	d.Frame()
	if len((*vw).vs.stickyLines) != 2 {
		t.Fatalf("inner must enter accumulated header zone: %v", (*vw).vs.stickyLines)
	}
	st.Viewport.setScrollY(float64(7.5*lh), st.viewCount())
	a0 := d.Frame()
	a := image.NewRGBA(a0.Bounds())
	copy(a.Pix, a0.Pix)
	if len((*vw).vs.stickyY) != 2 || abs32((*vw).vs.stickyY[1]-lh*.5) > 0.01 {
		t.Fatalf("push-off positions %v", (*vw).vs.stickyY)
	}
	r := (*vw).vs.stickyRect
	if abs32(r.H-1.5*lh) > .01 {
		t.Fatalf("push-off height %v", r)
	}
	// The outgoing inner row is clipped below the outer row, and its actual
	// keyword pixels move, not just the bookkeeping rectangle.
	st.ScrollBy(lh*.25, 0)
	b := d.Frame()
	if comparePNG(a, b, 0) == 0 || abs32((*vw).vs.stickyY[1]-lh*.25) > .01 {
		t.Fatalf("header pixels did not move smoothly: diff=%v y=%v lh=%v", comparePNG(a, b, 0), (*vw).vs.stickyY, lh)
	}
}

func TestMinimapExactSourceNavigation(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("x\n", 999)+"x", "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	r := (*vw).vs.minimapRect
	d.Click(geom.Pt(r.X+4, r.Y+200.25))
	d.Frame()
	if st.Viewport.TopLine != 100 {
		t.Fatalf("clicked miniature line100, got top %d", st.Viewport.TopLine)
	}
}

func TestMinimapSliderExtentAndEndpoints(t *testing.T) {
	goText(t)
	for _, n := range []int{13, 1000} {
		for _, mode := range []MinimapSize{MinimapProportional, MinimapFit, MinimapFill} {
			st := NewState(strings.Repeat("x\n", n-1)+"x", "a.txt", theme.Base("vs"))
			st.Options.Minimap.Size = mode
			st.Options.Minimap.ShowSlider = MinimapSliderAlways
			d, vw := viewDriver(st, 640, 300, nil)
			d.Frame()
			w := *vw
			r := w.vs.minimapRect
			mh := float32(2)
			if mode == MinimapFit {
				mh = min(mh, r.H/float32(n))
			}
			if mode == MinimapFill {
				mh = r.H / float32(n)
			}
			want := min(float32(n), st.Viewport.Height/st.Viewport.LineHeight) * mh
			if abs32(w.vs.minimapSlider.H-want) > .02 {
				t.Errorf("n%d %s slider H %v want %v", n, mode, w.vs.minimapSlider.H, want)
			}
			st.ScrollBy(1e8, 0)
			d.Frame()
			endpoint := st.Viewport.TopLine
			st.Viewport.setScrollY(0, st.viewCount())
			d.Frame()
			slider := (*vw).vs.minimapSlider
			from := geom.Pt(slider.X+3, slider.Y+slider.H/2)
			d.Inject(input.Pointer{Kind: input.Move, Pos: from})
			d.Inject(input.Pointer{Kind: input.Press, Pos: from})
			d.Frame()
			to := geom.Pt(from.X, r.Bottom()+5)
			d.Inject(input.Pointer{Kind: input.Move, Pos: to})
			d.Frame()
			d.Inject(input.Pointer{Kind: input.Release, Pos: to})
			d.Frame()
			if st.Viewport.TopLine != endpoint || endpoint != n-1 {
				t.Errorf("n%d %s drag endpoint %d wheel %d", n, mode, st.Viewport.TopLine, endpoint)
			}
			st.Close()
		}
	}
}

func TestNavigationViewCoordinates(t *testing.T) {
	goText(t)
	for _, folded := range []bool{false, true} {
		for _, mode := range []MinimapSize{MinimapProportional, MinimapFit, MinimapFill} {
			doc := strings.Repeat("a", 400) + "\n" + strings.Repeat("tail\n", 199)
			st := NewState(doc, "a.txt", theme.Base("vs"))
			st.Options.WordWrap = WrapColumn
			st.Options.WordWrapColumn = 10
			st.Options.Minimap.Size = mode
			d, vw := viewDriver(st, 640, 300, nil)
			d.Frame()
			if folded {
				st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 1, End: 150}}})
				st.MoveCaret(st.Snapshot().LineStart(1), false)
				st.Execute("editor.fold", nil)
				st.Viewport.setScrollY(0, st.viewCount())
				d.Frame()
			}
			w := *vw
			rows := st.vl.Count()
			r := w.vs.overviewRect
			total := float32(rows-1)*st.Viewport.LineHeight + st.Viewport.Height + st.Viewport.PadTop
			want := min(rows-1, int((total*.5-st.Viewport.PadTop)/st.Viewport.LineHeight))
			d.Click(geom.Pt(r.X+4, r.Y+r.H*.5))
			d.Frame()
			if got := st.peekRowOf(st.Primary().Head, st.caretUp()); got != want {
				t.Errorf("fold%v %s ruler midpoint row %d want%d", folded, mode, got, want)
			}
			if folded && len(st.Folded()) != 1 {
				t.Fatal("ruler navigation unfolded a hidden line")
			}
			st.Viewport.setScrollY(0, st.viewCount())
			d.Frame()
			w = *vw
			target := w.vs.miniRows[min(40, len(w.vs.miniRows)-1)]
			d.Click(geom.Pt(w.vs.minimapRect.X+4, target.y+target.h*.25))
			d.Frame()
			if st.Viewport.TopLine != target.vrow {
				t.Errorf("fold%v %s miniature row got%d want%d", folded, mode, st.Viewport.TopLine, target.vrow)
			}
			st.Close()
		}
	}
}

func TestOverviewHiddenMarkUsesFoldHeader(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("line\n", 200), "a.txt", theme.Base("vs"))
	defer st.Close()
	st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 0, End: 150}}})
	st.Execute("editor.fold", nil)
	st.MoveCaret(st.Snapshot().LineStart(180), false)
	red := color.NRGBA{255, 0, 0, 255}
	st.SetOverviewMarks("hidden", []OverviewMark{{Line: 100, Color: red, Lane: 1}})
	d, vw := viewDriver(st, 640, 300, nil)
	img := d.Frame()
	r := (*vw).vs.overviewRect
	if countColor(img, geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 4}, red) == 0 {
		t.Fatal("hidden marker did not map to fold header")
	}
}

func TestMinimapLongRopeBounded(t *testing.T) {
	goText(t)
	doc := strings.Repeat("x\n", 100) + strings.Repeat("a", 19<<20) + "\nend"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	defer st.Close()
	st.Options.Folding = false
	st.Options.StickyScroll.Enabled = false
	if st.LargeFile() {
		t.Fatal("fixture must be below20MiB")
	}
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	w := *vw
	w.vs.mini.cache = nil
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for range 3 {
		w.prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	}
	runtime.ReadMemStats(&b)
	if delta := b.TotalAlloc - a.TotalAlloc; delta > 2<<20 {
		t.Fatalf("bounded preview allocated %d bytes", delta)
	}
	runtime.GC()
	runtime.ReadMemStats(&b)
	if b.HeapAlloc > a.HeapAlloc && b.HeapAlloc-a.HeapAlloc > 2<<20 {
		t.Fatalf("prefix retained %d bytes", b.HeapAlloc-a.HeapAlloc)
	}
}

func TestFix1DamageCoordinateMatrix(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 2} {
		for _, mode := range []MinimapSize{MinimapProportional, MinimapFit, MinimapFill} {
			for _, folded := range []bool{false, true} {
				st := NewState(strings.Repeat("a", 400)+"\n"+strings.Repeat("tail\n", 80), "a.txt", theme.Base("vs"))
				st.Options.WordWrap = WrapColumn
				st.Options.WordWrapColumn = 10
				st.Options.Minimap.Size = mode
				st.Options.Minimap.ShowSlider = MinimapSliderAlways
				st.Options.OccurrencesHighlight = OccurrencesOff
				if folded {
					st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 1, End: 60}}})
					st.MoveCaret(st.Snapshot().LineStart(1), false)
					st.Execute("editor.fold", nil)
				}
				st.Viewport.TopLine, st.Viewport.OffsetY = 0, 0
				h := newHintHost(t, st, geom.Size{W: 480, H: 240}, scale)
				h.frame()
				h.focusAt(3)
				st.SetOverviewMarks("matrix", []OverviewMark{{Line: 20, Color: color.NRGBA{255, 0, 0, 255}, Lane: 5}})
				h.requirePartialStep("wrap/fold mapped mark")
				st.SetSelections([]Selection{{Anchor: 3, Head: 100}})
				h.requireExecutedStep("selection spans wrapped rows")
				r := h.vw.vs.minimapRect
				sample := h.vw.vs.miniRows[len(h.vw.vs.miniRows)/2]
				p := geom.Pt(r.X+3, sample.y+sample.h*.25)
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
				h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
				h.requireExecutedStep("miniature source click")
				if st.Viewport.TopLine != sample.vrow {
					t.Fatalf("matrix source click row%d want%d", st.Viewport.TopLine, sample.vrow)
				}
				slider := h.vw.vs.minimapSlider
				p = geom.Pt(slider.X+3, slider.Y+slider.H/2)
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
				h.requireExecutedStep("slider grab")
				if !h.vw.c.Dragging(h.vw.box.ID()) {
					t.Fatal("slider press did not capture")
				}
				p.Y = r.Bottom() + 5
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				// This jump exceeds the visible body; full redraw is intentional.
				h.requireExecutedStep("slider reaches wheel endpoint")
				h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
				h.requireExecutedStep("slider release")
				if h.vw.c.Dragging(h.vw.box.ID()) {
					t.Fatal("slider release kept capture")
				}
				if st.Viewport.TopLine != st.vl.Count()-1 {
					t.Fatal("drag did not reach last view row")
				}
				r = h.vw.vs.overviewRect
				p = geom.Pt(r.X+3, r.Y+r.H/2)
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
				h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
				h.requireExecutedStep("ruler midpoint")
				st.Close()
			}
		}
		st := NewState(strings.Repeat("func scope() {\n", 40), "a.go", theme.Base("vs"))
		st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 0, End: 30}, {Start: 3, End: 8}}})
		h := newHintHost(t, st, geom.Size{W: 480, H: 240}, scale)
		h.focusAt(2)
		for _, pos := range []float32{2.5, 7.5, 7.75, 8.25, 9} {
			st.Viewport.setScrollY(float64(pos*st.Viewport.LineHeight), st.viewCount())
			// Arbitrary quarter-line displacement may not be an integer
			// device pixel; the renderer cannot safely copy it and may draw full.
			h.requireExecutedStep("sticky entry/collision/push-off")
		}
		st.Close()
	}
}

func TestBoundedMiniatureGraphemes(t *testing.T) {
	text := strings.Repeat("a", 119) + "👩‍💻é" + strings.Repeat("z", 1000)
	st := NewState(text, "a.txt", nil)
	defer st.Close()
	if got := boundedMiniature(st.Snapshot(), 0, 0, 120, 4); got != strings.Repeat("a", 119) {
		t.Fatalf("split wide grapheme %q", got)
	}
	if got := boundedMiniature(st.Snapshot(), 0, 0, 122, 4); got != strings.Repeat("a", 119)+"👩‍💻é" {
		t.Fatalf("lost cluster %q", got)
	}
	huge := NewState("a"+strings.Repeat("́", 10000), "a.txt", nil)
	defer huge.Close()
	if got := boundedMiniature(huge.Snapshot(), 0, 0, 120, 4); got != "" {
		t.Fatal("unbounded combining cluster was split")
	}
}

func TestMinimapPaddingAndFrozenMapping(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("a", 400)+"\n"+strings.Repeat("tail\n", 100), "a.txt", theme.Base("vs"))
	defer st.Close()
	st.Options.WordWrap = WrapColumn
	st.Options.WordWrapColumn = 10
	st.Options.Padding = PaddingOptions{Top: 7, Bottom: 40}
	st.SetOverviewMarks("remote", []OverviewMark{{Line: 90, Lane: 7, Color: color.NRGBA{255, 0, 0, 255}}})
	d, vw := viewDriver(st, 640, 300, nil)
	old := shapeHook
	defer func() { shapeHook = old }()
	count, top := -1, -1
	shapeHook = func(int, string) {
		if count < 0 {
			count, top = st.vl.Count(), st.Viewport.TopLine
		}
	}
	d.Frame()
	w := *vw
	if st.vl.Count() != count || st.Viewport.TopLine != top {
		t.Fatal("paint changed mapping after layout began")
	}
	want := (st.Viewport.Height - 7) / st.Viewport.LineHeight * 2
	if abs32(w.vs.minimapSlider.H-want) > .02 {
		t.Fatalf("padding slider H%v want%v", w.vs.minimapSlider.H, want)
	}
	st.ScrollBy(400, 0)
	d.Frame()
	w.vs.railGrab = 1
	w.navigateMinimap(geom.Pt(w.vs.minimapRect.X+3, w.vs.minimapRect.Y))
	if st.Viewport.TopLine != 0 || st.Viewport.OffsetY != -7 {
		t.Fatalf("top endpoint lost padding: %+v", st.Viewport)
	}
}

func TestOverviewWrappedSelectionPixels(t *testing.T) {
	goText(t)
	th, err := theme.Parse([]byte(`{"base":"vs","inherit":true,"colors":{"editorOverviewRuler.selectionHighlightForeground":"#FF00FF"}}`))
	if err != nil {
		t.Fatal(err)
	}
	st := NewState(strings.Repeat("a", 400)+"\n"+strings.Repeat("tail\n", 100), "a.txt", th)
	defer st.Close()
	st.Options.WordWrap = WrapColumn
	st.Options.WordWrapColumn = 10
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	st.SetSelections([]Selection{{Anchor: 10, Head: 100}})
	img := d.Frame()
	r := (*vw).vs.overviewRect
	total := float32(st.vl.Count()-1)*st.Viewport.LineHeight + st.Viewport.Height
	y0 := float32(1) * st.Viewport.LineHeight / total * r.H
	y1 := float32(10) * st.Viewport.LineHeight / total * r.H
	magenta := color.NRGBA{255, 0, 255, 255}
	if countColor(img, geom.Rect{X: r.X + 1, Y: y0 + 2, W: r.W - 1, H: max(1, y1-y0-3)}, magenta) == 0 {
		t.Fatal("selection within one wrapped model line was collapsed to a single mark")
	}
}

func TestNavigationRailPixelEdges(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 2} {
		st := NewState("x\n", "a.txt", theme.Base("vs"))
		h := newHintHost(t, st, geom.Size{W: 420, H: 300}, scale)
		h.frame()
		x := h.vw.vs.minimapRect.X * scale
		if abs32(x-float32(int(x+.5))) > .001 {
			t.Fatalf("fixed rail edge shares device pixels with scrolling body: %v", x)
		}
		st.Close()
	}
}

func TestMinimapBlankPreviewClick(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("x\n", 12)+"x", "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	r := (*vw).vs.minimapRect
	d.Click(geom.Pt(r.X+3, r.Y+200))
	d.Frame()
	if st.Viewport.TopLine != 0 {
		t.Fatal("blank preview area has no source row and must not navigate")
	}
}

func TestMiniatureBlockGraphemeColumns(t *testing.T) {
	for _, s := range []string{"a👩‍💻", "中"} {
		blocks := miniBlocks(s, nil, theme.Base("vs"), 4)
		want := 3
		if s == "中" {
			want = 2
		}
		if blocks[len(blocks)-1].b != want {
			t.Fatalf("%q block end %d want%d", s, blocks[len(blocks)-1].b, want)
		}
	}
}

func TestShortMinimapDragGrip(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("x\n", 12)+"x", "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	r := (*vw).vs.minimapSlider
	from := geom.Pt(r.X+3, r.Y+r.H/2)
	d.Inject(input.Pointer{Kind: input.Move, Pos: from})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from})
	d.Frame()
	// With all13 miniature rows visible, the center of the shrinking thumb
	// is 13+scrollRow pixels. Move its center6 pixels -> source row6.
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(from.X, from.Y+6)})
	d.Frame()
	if st.Viewport.TopLine != 6 {
		t.Fatalf("short preview grip jumped to %d want6", st.Viewport.TopLine)
	}
}

func TestStickySiblingCollision(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("scope\n", 30), "a.txt", theme.Base("vs"))
	defer st.Close()
	st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 0, End: 8}, {Start: 9, End: 20}}})
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	st.Viewport.setScrollY(float64(8.5*st.Viewport.LineHeight), st.viewCount())
	d.Frame()
	if len((*vw).vs.stickyLines) != 1 || (*vw).vs.stickyLines[0] != 0 {
		t.Fatalf("incoming sibling must not be treated as nested: %v", (*vw).vs.stickyLines)
	}
}
