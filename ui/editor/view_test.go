package editor

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
)

var update = flag.Bool("update", false, "rewrite golden images in testdata/")

// shapeCounter records the View's LineCache calls through shapeHook.
type shapeCounter struct {
	mu    sync.Mutex
	calls int
	runes int
	lines map[int]bool
}

func countShapes(t *testing.T) *shapeCounter {
	t.Helper()
	sc := &shapeCounter{lines: map[int]bool{}}
	shapeHook = func(line int, s string) {
		sc.mu.Lock()
		defer sc.mu.Unlock()
		sc.calls++
		sc.runes += utf8.RuneCountInString(s)
		sc.lines[line] = true
	}
	t.Cleanup(func() { shapeHook = nil })
	return sc
}

func (sc *shapeCounter) reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.calls, sc.runes = 0, 0
	sc.lines = map[int]bool{}
}

// goText switches to the pure-Go text engine for the test.
func goText(t *testing.T) {
	t.Helper()
	prev := text.UseSystemEngine(false)
	t.Cleanup(func() { text.UseSystemEngine(prev) })
}

// viewDriver returns a driver showing st in a w×h window and a pointer to
// the ViewW of the last build.
func viewDriver(st *State, w, h float32, mods func(*ViewW)) (*ui.Driver, **ViewW) {
	var vw *ViewW
	d := ui.NewDriver(func(c *ui.Context) {
		vw = View(c, st)
		if mods != nil {
			mods(vw)
		}
	}, ui.DriverOptions{Width: w, Height: h})
	return d, &vw
}

func rgba(c color.NRGBA) color.RGBA { return color.RGBA{c.R, c.G, c.B, 255} }

// countColor counts pixels of r in img exactly equal to c (opaque).
func countColor(img *image.RGBA, r geom.Rect, c color.NRGBA) int {
	want := rgba(c)
	n := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if img.RGBAAt(x, y) == want {
				n++
			}
		}
	}
	return n
}

// countTinted counts pixels of r that are a blend of bg and c (antialiased
// text drawn in c) with at least half coverage.
func countTinted(img *image.RGBA, r geom.Rect, bg, c color.NRGBA) int {
	n := 0
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			p := img.RGBAAt(x, y)
			pc := [3]float64{float64(p.R), float64(p.G), float64(p.B)}
			b := [3]float64{float64(bg.R), float64(bg.G), float64(bg.B)}
			f := [3]float64{float64(c.R), float64(c.G), float64(c.B)}
			// Coverage from the channel where bg and c differ most.
			k, best := 0, 0.0
			for i := range 3 {
				if d := abs(f[i] - b[i]); d > best {
					k, best = i, d
				}
			}
			if best < 32 {
				continue
			}
			t := (pc[k] - b[k]) / (f[k] - b[k])
			if t < 0.5 || t > 1.05 {
				continue
			}
			ok := true
			for i := range 3 {
				if abs(b[i]+t*(f[i]-b[i])-pc[i]) > 12 {
					ok = false
				}
			}
			if ok {
				n++
			}
		}
	}
	return n
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func manyLines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "line %d of the document\n", i+1)
	}
	return b.String()
}

func TestViewRendersVisibleOnly(t *testing.T) {
	st := NewState(manyLines(100_000), "big.txt", theme.Base("vs"))
	sc := countShapes(t)
	d, _ := viewDriver(st, 800, 600, nil)
	d.Frame()
	check := func(when string) {
		t.Helper()
		sc.reset()
		d.Frame()
		first, last := st.VisibleLines()
		if last <= first {
			t.Fatalf("%s: visible lines %d..%d", when, first, last)
		}
		if lim := last - first + 1 + 4; sc.calls > lim {
			t.Errorf("%s: %d LineCache calls, want <= %d (visible %d..%d)", when, sc.calls, lim, first, last)
		}
		for l := range sc.lines {
			if l < first-2 || l > last+2 {
				t.Errorf("%s: shaped line %d outside %d..%d ± 2", when, l, first, last)
			}
		}
	}
	check("top")
	if st.Viewport.LineHeight <= 0 || st.Viewport.Height <= 0 || st.Viewport.Width <= 0 {
		t.Fatalf("viewport not measured: %+v", st.Viewport)
	}
	st.Viewport.TopLine = 50_000
	check("middle")
	st.Viewport.TopLine = 99_999
	check("end")
}

func TestViewSyntaxColors(t *testing.T) {
	goText(t)
	th := theme.Base("vs")
	src := "package main\n\nfunc main() {\n\treturn\n}\n"
	st := NewState(src, "main.go", th)
	d, vw := viewDriver(st, 600, 300, nil)
	d.Frame()
	img := d.Frame()
	kw := th.Style("keyword.go").Foreground
	bg := defaultColor(th, "editor.background")
	if kw.A == 0 || kw == th.Style("").Foreground {
		t.Fatalf("theme keyword color %v not distinct", kw)
	}
	g := (*vw).vs
	lh := st.Viewport.LineHeight
	// "package" spans 7 columns from the text origin on line 0.
	colW := g.colW
	r0 := geom.Rect{X: g.text.X, Y: g.text.Y, W: 7 * colW, H: lh}
	if n := countTinted(img, r0, bg, kw); n < 10 {
		t.Errorf("line 0 \"package\": %d keyword-colored pixels, want >= 10", n)
	}
	// "main" after it is an identifier, not a keyword.
	r1 := geom.Rect{X: g.text.X + 8*colW, Y: g.text.Y, W: 4 * colW, H: lh}
	if n := countTinted(img, r1, bg, kw); n != 0 {
		t.Errorf("identifier \"main\": %d keyword-colored pixels, want 0", n)
	}
	// "func" on line 2.
	r2 := geom.Rect{X: g.text.X, Y: g.text.Y + 2*lh, W: 4 * colW, H: lh}
	if n := countTinted(img, r2, bg, kw); n < 10 {
		t.Errorf("line 2 \"func\": %d keyword-colored pixels, want >= 10", n)
	}
}

func TestViewScrollWheel(t *testing.T) {
	long := strings.Repeat("x", 400)
	st := NewState(manyLines(500)+long+"\n", "a.txt", theme.Base("vs-dark"))
	d, vw := viewDriver(st, 600, 400, nil)
	d.Frame()
	d.Frame()
	lh := st.Viewport.LineHeight
	p := geom.Pt(300, 200)
	before := d.Frame().SubImage(image.Rect(0, 0, 600, 400)).(*image.RGBA)
	snap := append([]uint8(nil), before.Pix...)
	d.Scroll(p, 10*lh)
	img := d.Frame()
	if st.Viewport.TopLine != 10 || st.Viewport.OffsetY != 0 {
		t.Fatalf("after wheel 10 lines: TopLine %d OffsetY %v", st.Viewport.TopLine, st.Viewport.OffsetY)
	}
	if string(snap) == string(img.Pix) {
		t.Errorf("image unchanged after scrolling")
	}
	d.Scroll(p, -3.5*lh)
	d.Frame()
	if st.Viewport.TopLine != 6 || st.Viewport.OffsetY != lh/2 {
		t.Errorf("after wheel up 3.5 lines: TopLine %d OffsetY %v", st.Viewport.TopLine, st.Viewport.OffsetY)
	}
	d.Scroll(p, -1000*lh)
	d.Frame()
	if st.Viewport.TopLine != 0 || st.Viewport.OffsetY != 0 {
		t.Errorf("wheel past the top: TopLine %d OffsetY %v", st.Viewport.TopLine, st.Viewport.OffsetY)
	}
	// Horizontal: the 400-column line must be seen to be known, so scroll
	// to it first.
	d.Scroll(p, 495*lh)
	d.Frame()
	d.Frame()
	if st.Viewport.ContentWidth <= st.Viewport.Width {
		t.Fatalf("content width %v not above viewport width %v", st.Viewport.ContentWidth, st.Viewport.Width)
	}
	d.Inject(input.Scroll{Pos: p, DX: 120})
	d.Frame()
	if st.Viewport.ScrollX != 120 {
		t.Errorf("ScrollX = %v after DX 120", st.Viewport.ScrollX)
	}
	// Shift+wheel scrolls horizontally too.
	d.Inject(input.Scroll{Pos: p, DY: 30, Mods: input.ModShift})
	d.Frame()
	if st.Viewport.ScrollX != 150 {
		t.Errorf("ScrollX = %v after shift+wheel 30", st.Viewport.ScrollX)
	}
	d.Inject(input.Scroll{Pos: p, DX: 1e6})
	d.Frame()
	if max := st.Viewport.ContentWidth - st.Viewport.Width; st.Viewport.ScrollX != max {
		t.Errorf("ScrollX = %v, want clamped to %v", st.Viewport.ScrollX, max)
	}
	_ = vw
}

func TestEmptyAndPlainDocuments(t *testing.T) {
	goText(t)
	for _, tc := range []struct {
		name, text, file string
		lines            int
	}{
		{"empty", "", "empty.go", 1},
		{"newline", "\n", "nl.go", 2},
		{"plain", "just some words\n\tindented", "notes.unknownext", 2},
		{"empty-plain", "", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			th := theme.Base("vs")
			st := NewState(tc.text, tc.file, th)
			d, vw := viewDriver(st, 400, 200, nil)
			d.Frame()
			d.Click(geom.Pt(200, 100)) // focus
			img := d.Frame()
			w := *vw
			if !w.Focused() {
				t.Fatalf("view not focused after click")
			}
			if w.Changed() {
				t.Errorf("Changed() without an edit")
			}
			g := w.vs
			lh := st.Viewport.LineHeight
			// Caret: cursor-colored pixels near the text origin of the
			// caret's line.
			cur := defaultColor(th, "editorCursor.foreground")
			cl := st.Snapshot().LineOf(st.Primary().Head)
			r := geom.Rect{X: g.text.X - 2, Y: g.text.Y + float32(cl)*lh, W: g.text.W + 2, H: lh}
			if n := countColor(img, r, cur); n < int(lh/2) {
				t.Errorf("caret: %d cursor pixels on line %d, want >= %d", n, cl, int(lh/2))
			}
			// Line numbers: the last number is drawn, nothing after it.
			ln := defaultColor(th, "editorLineNumber.foreground")
			act := defaultColor(th, "editorLineNumber.activeForeground")
			for i := 0; i < tc.lines+2; i++ {
				rr := geom.Rect{X: g.gutter.X, Y: g.gutter.Y + float32(i)*lh, W: g.gutter.W, H: lh}
				n := countColor(img, rr, ln) + countColor(img, rr, act)
				if i < tc.lines && n == 0 {
					t.Errorf("line number %d not drawn", i+1)
				}
				if i >= tc.lines && n != 0 {
					t.Errorf("line number drawn on row %d past the end", i+1)
				}
			}
		})
	}
}

// minifiedJS returns one line of about n characters of minified JavaScript.
func minifiedJS(n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "var a%d=\"s%d\";function f%d(x){return x+%d}", i, i, i, i)
	}
	return b.String()[:n]
}

func TestVeryLongLineView(t *testing.T) {
	goText(t)
	line := minifiedJS(20_000)
	st := NewState(line+"\n", "app.min.js", theme.Base("vs-dark"))
	sc := countShapes(t)
	d, vw := viewDriver(st, 800, 400, nil)
	d.Frame()
	d.Frame()
	g := (*vw).vs
	lim := 3 * longChunk
	if sc.runes > 2*lim {
		t.Errorf("first frames shaped %d runes of a %d-rune line", sc.runes, len(line))
	}
	if want := float32(20_000) * g.colW; st.Viewport.ContentWidth < want*0.99 {
		t.Errorf("ContentWidth %v, want about %v", st.Viewport.ContentWidth, want)
	}
	p := geom.Pt(400, 100)
	for _, dx := range []float32{5000, 50000, 40000} {
		sc.reset()
		d.Inject(input.Scroll{Pos: p, DX: dx})
		img := d.Frame()
		if sc.runes > lim {
			t.Errorf("after DX %v: shaped %d runes, want <= %d", dx, sc.runes, lim)
		}
		// Text is drawn in the visible window: some pixels of line 0 in
		// the text area differ from the background.
		bg := defaultColor(st.Theme, "editor.background")
		r := geom.Rect{X: g.text.X, Y: g.text.Y, W: g.text.W, H: st.Viewport.LineHeight}
		total := int(r.W) * int(r.H)
		if n := countColor(img, r, bg); n > total*97/100 {
			t.Errorf("after DX %v (ScrollX %v): line 0 looks empty (%d of %d background pixels)", dx, st.Viewport.ScrollX, n, total)
		}
	}
	if st.Viewport.ScrollX != 95000 {
		t.Errorf("ScrollX = %v, want 95000", st.Viewport.ScrollX)
	}
	// Scrolling back over already shaped chunks hits the cache: no
	// reshaping of the same window.
	sc.reset()
	d.Frame()
	if sc.runes > lim {
		t.Errorf("idle frame shaped %d runes", sc.runes)
	}
}

func TestViewThemeDefaults(t *testing.T) {
	vs, dark := theme.Base("vs"), theme.Base("vs-dark")
	for _, c := range []struct {
		th   *theme.Theme
		key  string
		want color.NRGBA
	}{
		{vs, "editor.background", color.NRGBA{0xff, 0xff, 0xfe, 0xff}}, // the theme's own value
		{dark, "editor.background", color.NRGBA{0x1e, 0x1e, 0x1e, 0xff}},
		{vs, "editorLineNumber.foreground", color.NRGBA{0x23, 0x78, 0x93, 0xff}},
		{dark, "editorLineNumber.foreground", color.NRGBA{0x85, 0x85, 0x85, 0xff}},
		{vs, "editor.selectionBackground", color.NRGBA{0xad, 0xd6, 0xff, 0xff}},
		{dark, "editor.selectionBackground", color.NRGBA{0x26, 0x4f, 0x78, 0xff}},
		{vs, "editorCursor.foreground", color.NRGBA{0, 0, 0, 0xff}},
		{dark, "editorCursor.foreground", color.NRGBA{0xae, 0xaf, 0xad, 0xff}},
		{vs, "editor.lineHighlightBorder", color.NRGBA{0xee, 0xee, 0xee, 0xff}},
		{dark, "editor.lineHighlightBorder", color.NRGBA{0x28, 0x28, 0x28, 0xff}},
	} {
		if got := defaultColor(c.th, c.key); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.th.BaseName(), c.key, got, c.want)
		}
	}
	if _, ok := lookupColor(vs, "editor.lineHighlightBackground"); ok {
		t.Errorf("vs: lineHighlightBackground has no default in Monaco (a border is drawn instead)")
	}
}

// goldenSource is the document of the golden images: keywords, strings,
// comments, numbers, a tab, CJK and a selection over two lines.
const goldenSource = `// Package demo 演示高亮
package demo

import "fmt"

func Sum(xs []int) (n int) {
	for _, x := range xs {
		n += x * 2 // double
	}
	fmt.Println("sum:", n, 0x1F)
	return
}
`

func TestViewGolden(t *testing.T) {
	goText(t)
	got := strings.Join(text.Families(), "\n")
	fonts := filepath.Join("testdata", "golden_fonts.txt")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(fonts, []byte(got), 0o644)
	} else if want, err := os.ReadFile(fonts); err != nil || string(want) != got {
		t.Skip("golden images were recorded with different fonts")
	}
	for _, name := range []string{"vs", "vs-dark"} {
		t.Run(name, func(t *testing.T) {
			st := NewState(goldenSource, "demo.go", theme.Base(name))
			d, _ := viewDriver(st, 520, 300, nil)
			d.Frame()
			d.Click(geom.Pt(400, 280)) // focus
			snap := st.Snapshot()
			// Select from "range" on line 6 to "x * 2" on line 7.
			a := snap.LineStart(6) + strings.Index(snap.Line(6), "range")
			b := snap.LineStart(7) + strings.Index(snap.Line(7), "x *")
			st.MoveCaret(a, false)
			st.MoveCaret(b, true)
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+".png")
		})
		// Every display decoration of Task 12 on: relative line numbers,
		// whitespace marks, a control character, indentation guides with
		// the active one, colored brackets (an unexpected one), the matched
		// pair at the caret, the line highlight in the gutter and the text,
		// and a top padding.
		t.Run(name+"/decorations", func(t *testing.T) {
			st := NewState(goldenDecoSource, "demo.go", theme.Base(name))
			o := &st.Options
			o.LineNumbers = LineNumbersRelative
			o.RenderWhitespace = RenderWhitespaceAll
			o.RenderLineHighlight = RenderLineHighlightAll
			o.Padding = PaddingOptions{Top: 6}
			d, _ := viewDriver(st, 520, 300, nil)
			d.Frame()
			d.Click(geom.Pt(400, 280)) // focus
			snap := st.Snapshot()
			// The caret after the "{" of the for loop (line 4).
			st.MoveCaret(snap.LineStart(4)+strings.Index(snap.Line(4), "{")+1, false)
			checkGolden(t, d.Frame(), "view_"+strings.ReplaceAll(name, "-", "_")+"_decorations.png")
		})
	}
}

// goldenDecoSource is the document of the decorations golden images.
const goldenDecoSource = "// Package demo 演示装饰\n" +
	"package demo\n" +
	"\n" +
	"func Sum(xs []int) (n int) {\n" +
	"\tfor _, x := range xs {\n" +
	"\t\tif x > 0 {\n" +
	"\t\t\tn += x  // twice\n" +
	"\n" +
	"\t\t}\n" +
	"\t}\n" +
	"\treturn n \x01\n" +
	"}\n" +
	"\n" +
	"var bad = [1, 2)\n"

// checkGolden compares img with testdata/file (rewritten with -update).
func checkGolden(t *testing.T, img image.Image, file string) {
	t.Helper()
	path := filepath.Join("testdata", file)
	if *update {
		if err := savePNG(path, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := loadPNG(path)
	if err != nil {
		t.Fatalf("missing golden (go test ./ui/editor -run Golden -update): %v", err)
	}
	if diff := comparePNG(img, want, 24); diff > 0.005 {
		out := filepath.Join(t.TempDir(), filepath.Base(path))
		savePNG(out, img)
		t.Fatalf("%.2f%% pixels differ; got %s", diff*100, out)
	}
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func loadPNG(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// comparePNG returns the fraction of pixels whose channels differ by more
// than tol.
func comparePNG(a, b image.Image, tol int) float64 {
	if a.Bounds() != b.Bounds() {
		return 1
	}
	bad, total := 0, 0
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			total++
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			d := func(p, q uint32) bool { return absDiff(int(p>>8), int(q>>8)) > tol }
			if d(ar, br) || d(ag, bg) || d(ab, bb) || d(aa, ba) {
				bad++
			}
		}
	}
	return float64(bad) / float64(total)
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// TestCaretFarAwayKeepsTokenWindow: a focused view whose caret is far
// above the visible lines neither shapes the caret line nor moves the
// highlighter's token window to it while scrolling (the IME caret rect
// does not need the caret line's layout).
func TestCaretFarAwayKeepsTokenWindow(t *testing.T) {
	goText(t)
	var b strings.Builder
	for i := range 20_000 {
		fmt.Fprintf(&b, "var v%d = %d // line\n", i, i)
	}
	st := NewState(b.String(), "big.go", theme.Base("vs"))
	hl := st.Highlighter()
	if hl == nil {
		t.Fatal("no highlighter")
	}
	sc := countShapes(t)
	d, vw := viewDriver(st, 600, 300, nil)
	d.Frame()
	d.Click(geom.Pt(300, 150))
	d.Advance(time.Second)
	if !(*vw).Focused() {
		t.Fatal("not focused")
	}
	st.MoveCaret(0, false)
	d.Frame()
	st.Viewport.TopLine = 10_000
	d.Frame()
	for k := range 20 {
		sc.reset()
		d.Scroll(geom.Pt(300, 150), 3*st.Viewport.LineHeight)
		first, last := st.VisibleLines()
		if first < 10_000 {
			t.Fatalf("frame %d: scrolled back to %d", k, first)
		}
		lo, hi := hl.WindowRangeFor(st.hlKey)
		if lo > first || hi <= last {
			t.Fatalf("frame %d: token window %d..%d does not hold the visible lines %d..%d", k, lo, hi, first, last)
		}
		if sc.lines[0] {
			t.Fatalf("frame %d: the off-screen caret line was shaped", k)
		}
		if on, r := d.TextInputActive(); !on || r.H <= 0 {
			t.Fatalf("frame %d: no IME caret rect", k)
		}
	}
}
