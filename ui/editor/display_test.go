package editor

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

func TestRelativeLineNumbers(t *testing.T) {
	o := DefaultOptions()
	type want struct {
		num        int
		show, left bool
	}
	check := func(mode LineNumbersMode, i, caret, n int, w want) {
		t.Helper()
		o.LineNumbers = mode
		num, show, left := o.lineNumber(i, caret, n)
		if (want{num, show, left}) != w {
			t.Errorf("%s line %d caret %d of %d: %d %v %v, want %+v", mode, i, caret, n, num, show, left, w)
		}
	}
	check(LineNumbersOn, 4, 9, 50, want{5, true, false})
	check("", 4, 9, 50, want{5, true, false}) // unset: "on"
	check(LineNumbersOff, 4, 9, 50, want{})
	// Relative: the distance; the caret's line its own number, left aligned.
	check(LineNumbersRelative, 4, 9, 50, want{5, true, false})
	check(LineNumbersRelative, 12, 9, 50, want{3, true, false})
	check(LineNumbersRelative, 9, 9, 50, want{10, true, true})
	// Interval: every tenth line, the last line and the caret's line.
	check(LineNumbersInterval, 9, 3, 50, want{10, true, false})
	check(LineNumbersInterval, 4, 3, 50, want{})
	check(LineNumbersInterval, 3, 3, 50, want{4, true, false})
	check(LineNumbersInterval, 46, 3, 47, want{47, true, false})

	// Drawn: in relative mode line 3's number is "2" with the caret on
	// line 5, and the numbers follow the caret.
	goText(t)
	th := theme.Base("vs")
	st := NewState(strings.Repeat("x\n", 30), "a.txt", th)
	st.Options.LineNumbers = LineNumbersRelative
	d, vw := viewDriver(st, 300, 300, nil)
	st.MoveCaret(st.Snapshot().LineStart(5), false)
	d.Frame()
	gutter := (*vw).vs.gutter
	lh := (*vw).lineHeight()
	row := func(i int) geom.Rect { return geom.Rect{X: gutter.X, Y: gutter.Y + float32(i)*lh, W: gutter.W, H: lh} }
	fg := defaultColor(th, "editorLineNumber.foreground")
	bg := defaultColor(th, "editorGutter.background")
	img := d.Frame()
	ink := func(i int) int { return countTinted(img, row(i), bg, fg) }
	if ink(3) == 0 || ink(4) == 0 {
		t.Fatal("relative numbers not drawn")
	}
	// "2" (line 3) and "1" (line 4) differ.
	if ink(3) == ink(4) {
		t.Errorf("lines 3 and 4 drawn alike (%d)", ink(3))
	}
	st.Options.LineNumbers = LineNumbersInterval
	img = d.Frame()
	ink = func(i int) int { return countTinted(img, row(i), bg, fg) }
	if ink(3) != 0 || ink(9) == 0 {
		t.Errorf("interval: line 4 ink %d (want none), line 10 ink %d", ink(3), ink(9))
	}
}

func TestRenderWhitespaceModes(t *testing.T) {
	const row = "  a  b c\t " // blanks at 0 1, 3 4, 6, 8 (tab), 9
	for _, c := range []struct {
		mode RenderWhitespaceMode
		sels [][2]int
		last bool
		want []int
	}{
		{RenderWhitespaceNone, nil, true, nil},
		{RenderWhitespaceAll, nil, true, []int{0, 1, 3, 4, 6, 8, 9}},
		// Boundary: leading, trailing, tabs, runs of two or more spaces.
		{RenderWhitespaceBoundary, nil, true, []int{0, 1, 3, 4, 8, 9}},
		{RenderWhitespaceTrailing, nil, true, []int{8, 9}},
		{RenderWhitespaceTrailing, nil, false, nil}, // a row continuing on the next
		{RenderWhitespaceSelection, nil, true, nil},
		{RenderWhitespaceSelection, [][2]int{{3, 7}}, true, []int{3, 4, 6}},
	} {
		got := whitespaceMarked(row, 0, c.mode, c.sels, c.last, nil)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s %v: %v, want %v", c.mode, c.sels, got, c.want)
		}
	}
	// A blank row: every blank is trailing.
	if got := whitespaceMarked(" \t ", 0, RenderWhitespaceTrailing, nil, true, nil); !slices.Equal(got, []int{0, 1, 2}) {
		t.Errorf("blank row trailing: %v", got)
	}
	// A wrapped row's offset (b0) applies to the selections and the result.
	if got := whitespaceMarked("a b", 10, RenderWhitespaceSelection, [][2]int{{11, 12}}, true, nil); !slices.Equal(got, []int{11}) {
		t.Errorf("row at byte 10: %v", got)
	}

	// Drawn: "all" marks spaces in editorWhitespace.foreground; "none" not.
	goText(t)
	th := theme.Base("vs")
	st := NewState("a    b", "a.txt", th)
	st.Options.RenderWhitespace = RenderWhitespaceAll
	d, vw := viewDriver(st, 300, 100, nil)
	d.Frame()
	img := d.Frame()
	text := (*vw).vs.text
	area := geom.Rect{X: text.X, Y: text.Y, W: 120, H: (*vw).lineHeight()}
	bg := defaultColor(th, "editor.background")
	ws := defaultColor(th, "editorWhitespace.foreground")
	marked := countDiffering(img, area, bg)
	st.Options.RenderWhitespace = RenderWhitespaceNone
	img = d.Frame()
	plain := countDiffering(img, area, bg)
	if marked <= plain {
		t.Errorf("whitespace marks not drawn (%d vs %d differing pixels; color %v)", marked, plain, ws)
	}
}

// countDiffering counts the pixels of r that are not bg.
func countDiffering(img interface {
	RGBAAt(x, y int) color.RGBA
}, r geom.Rect, bg color.NRGBA) int {
	want := rgba(bg)
	n := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if img.RGBAAt(x, y) != want {
				n++
			}
		}
	}
	return n
}

func TestIndentGuidesActive(t *testing.T) {
	text := "func f() {\n" + // 0: level 0
		"\tif x {\n" + //        1: 1
		"\t\ty()\n" + //         2: 2
		"\n" + //                3: blank between two level 2 lines: 2
		"\t\tz()\n" + //         4: 2
		"\t}\n" + //             5: 1
		"}" //                   6: 0
	st := NewState(text, "a.go", theme.Base("vs"))
	w := &ViewW{st: st, opts: DefaultOptions()}
	rows := make([]lineView, st.Snapshot().LineCount())
	for i := range rows {
		rows[i] = lineView{i: i, vi: i, rows: 1}
	}
	levels := func(g []rowGuides) []int {
		out := make([]int, len(g))
		for k := range g {
			out[k] = g[k].level
		}
		return out
	}
	actives := func(g []rowGuides) []int {
		out := make([]int, len(g))
		for k := range g {
			out[k] = g[k].active
		}
		return out
	}
	snap := st.Snapshot()
	g := w.guidesFor(snap, rows, 2)
	if got, want := levels(g), []int{0, 1, 2, 2, 2, 1, 0}; !slices.Equal(got, want) {
		t.Errorf("levels %v, want %v", got, want)
	}
	for _, c := range []struct {
		caret int
		want  []int
	}{
		{2, []int{0, 0, 2, 2, 2, 0, 0}}, // inside the if block
		{1, []int{0, 0, 2, 2, 2, 0, 0}}, // the block's first line: its child level
		{5, []int{0, 0, 2, 2, 2, 0, 0}}, // the block's last line
		{6, []int{0, 1, 1, 1, 1, 1, 0}}, // the function's closing brace
		{0, []int{0, 1, 1, 1, 1, 1, 0}},
	} {
		if got := actives(w.guidesFor(snap, rows, c.caret)); !slices.Equal(got, c.want) {
			t.Errorf("caret on line %d: active %v, want %v", c.caret, got, c.want)
		}
	}
	w.opts.Guides.HighlightActiveIndentation = false
	if got := actives(w.guidesFor(snap, rows, 2)); slices.ContainsFunc(got, func(v int) bool { return v != 0 }) {
		t.Errorf("active guide without highlightActiveIndentation: %v", got)
	}
	w.opts.Guides.Indentation = false
	if g := w.guidesFor(snap, rows, 2); g != nil {
		t.Errorf("guides with indentation off: %v", g)
	}

	// A blank line inside a block opening below: the upper block's level
	// plus one; offSide languages take the level below.
	gc := &guideCtx{snap: NewState("a\n\n\tb\n\nc", "a.txt", nil).Snapshot(), tab: 4, ind: map[int]int{}, above: map[int]int{}, below: map[int]int{}}
	if l1, l3 := gc.level(1), gc.level(3); l1 != 1 || l3 != 1 {
		t.Errorf("blank levels %d %d, want 1 1", l1, l3)
	}
	gc = &guideCtx{snap: gc.snap, tab: 4, offSide: true, ind: map[int]int{}, above: map[int]int{}, below: map[int]int{}}
	if l3 := gc.level(3); l3 != 0 {
		t.Errorf("offSide blank level %d, want 0", l3)
	}

	// Drawn: the active guide in its color, the others in theirs.
	goText(t)
	th := theme.Base("vs")
	d, vw := viewDriver(st, 400, 200, nil)
	d.Frame()
	st.MoveCaret(snap.LineStart(2)+2, false)
	img := d.Frame()
	gc2 := guideColorsOf(th)
	tx := (*vw).vs.text.X
	lh := (*vw).lineHeight()
	colW := (*vw).vs.colW
	at := func(line, level int) geom.Rect {
		return geom.Rect{X: tx + float32((level-1)*4)*colW - 1, Y: (*vw).vs.text.Y + float32(line)*lh, W: 3, H: lh}
	}
	near := func(r geom.Rect, c color.NRGBA) bool {
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			for x := int(r.X); x < int(r.X+r.W); x++ {
				p := img.RGBAAt(x, y)
				if absDiff(int(p.R), int(c.R)) < 24 && absDiff(int(p.G), int(c.G)) < 24 && absDiff(int(p.B), int(c.B)) < 24 {
					return true
				}
			}
		}
		return false
	}
	if !near(at(3, 2), gc2.active[0]) {
		t.Errorf("active guide of line 4 (level 2) not drawn in %v", gc2.active[0])
	}
	if !near(at(3, 1), gc2.bg[0]) {
		t.Errorf("inactive guide of line 4 (level 1) not drawn in %v", gc2.bg[0])
	}
}

func TestControlCharacters(t *testing.T) {
	if got := controlPictures("a\x01b\x7f\tc"); got != "a␁b␡\tc" {
		t.Errorf("controlPictures = %q", got)
	}
	goText(t)
	th := theme.Base("vs")
	st := NewState("ab\x01\x02cd", "a.txt", th)
	d, vw := viewDriver(st, 300, 100, nil)
	d.Frame()
	img := d.Frame()
	area := (*vw).vs.text
	area.H = (*vw).lineHeight()
	if n := countColor(img, area, controlBG); n == 0 {
		t.Error("control characters have no dark red background")
	}
	st.Options.RenderControlCharacters = false
	img = d.Frame()
	if n := countColor(img, area, controlBG); n != 0 {
		t.Errorf("renderControlCharacters off: %d dark red pixels", n)
	}
}

func TestPadding(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("line\n", 50), "a.txt", theme.Base("vs"))
	st.Options.Padding = PaddingOptions{Top: 20, Bottom: 5}
	d, vw := viewDriver(st, 300, 200, nil)
	d.Frame()
	v := &st.Viewport
	if v.TopLine != 0 || v.OffsetY != -20 {
		t.Fatalf("at the top: TopLine %d OffsetY %v, want 0 -20", v.TopLine, v.OffsetY)
	}
	// The first line is drawn below the padding: a click in the padding
	// is on line 0.
	if r := (*vw).pointRow(geom.Pt((*vw).vs.text.X+5, (*vw).vs.text.Y+5)); r != -1 {
		t.Errorf("row in the padding = %d, want -1 (above line 0)", r)
	}
	st.ScrollBy(10, 0)
	if v.TopLine != 0 || v.OffsetY != -10 {
		t.Errorf("scrolled 10: %d %v", v.TopLine, v.OffsetY)
	}
	st.ScrollBy(1e6, 0)
	if n := st.Snapshot().LineCount(); v.TopLine != n-1 || v.OffsetY != 5 {
		t.Errorf("scrolled to the end: %d %v, want %d 5 (the bottom padding)", v.TopLine, v.OffsetY, n-1)
	}
	st.ScrollBy(-1e6, 0)
	if v.TopLine != 0 || v.OffsetY != -20 {
		t.Errorf("back to the top: %d %v", v.TopLine, v.OffsetY)
	}
	// Removing the padding keeps the view at the top.
	st.Options.Padding = PaddingOptions{}
	d.Frame()
	if v.TopLine != 0 || v.OffsetY != 0 {
		t.Errorf("padding removed: %d %v", v.TopLine, v.OffsetY)
	}
}
