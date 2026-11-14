package editor

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// click presses and releases the primary button at p with mods.
func (h *hintHost) click(p geom.Point, mods input.Modifiers) {
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p, Mods: mods})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p, Mods: mods})
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p, Mods: mods})
}

// shortcutMod is the platform's Cmd (macOS) or Ctrl.
func shortcutMod() input.Modifiers {
	if keymap.Current() == keymap.Mac {
		return input.ModMeta
	}
	return input.ModCtrl
}

// TestOccurrencesDebounced: the occurrences of the word at the caret are
// computed occDebounce after the caret came to rest (each move restarts
// the wait, so a held arrow key computes nothing), through one damaged
// wake-up, and then drawn after OccurrencesHighlightDelay (0 by default:
// in the same frame); until then the old occurrences stay (no blank
// frame), and they stay for good while the caret moves between them.
// Typing removes them at once and its caret moves do not bring them back;
// a cursor move that is not an edit does. No frame is requested once they
// are shown.
func TestOccurrencesDebounced(t *testing.T) {
	goText(t)
	if DefaultOccurrencesHighlightDelay != 0 || DefaultOptions().OccurrencesHighlightDelay != 0 {
		t.Fatal("OccurrencesHighlightDelay default is not Monaco's 0")
	}
	st := NewState("foo bar foo\nbaz foo(foo1)\nbar\n", "main.go", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 480, H: 200}, 1)
	h.focusAt(2)
	h.now = h.now.Add(time.Second) // the focus run: "bar" at the caret
	h.frame()
	occ := func() int { return h.vw.vs.decoCount[decoOcc] }
	if occ() != 2 {
		t.Fatalf("after focusing: %d occurrences, want bar's 2", occ())
	}
	// wakeIn checks that the next wake-up is d from now (the debounce, not
	// the caret blink).
	wakeIn := func(what string, d time.Duration) {
		t.Helper()
		if next := h.r.NextWake(); !next.Equal(h.now.Add(d)) {
			t.Fatalf("%s: NextWake in %v, want %v", what, next.Sub(h.now), d)
		}
	}
	// noWake checks that the occurrences request no frame (the next
	// wake-up is the caret blink's).
	noWake := func(what string) {
		t.Helper()
		o := h.vw.vs.occ
		if o.run || o.word != "" && !o.shown {
			t.Fatalf("%s: occurrences still waiting: %+v", what, o)
		}
		blink := h.vw.vs.activity
		for !blink.After(h.now) {
			blink = blink.Add(blinkPeriod / 2)
		}
		if next := h.r.NextWake(); next.Before(blink) {
			t.Fatalf("%s: a frame is requested in %v, before the blink in %v", what, next.Sub(h.now), blink.Sub(h.now))
		}
	}

	st.MoveCaret(1, false) // in "foo"
	h.frame()
	wakeIn("move", occDebounce)
	h.now = h.now.Add(occDebounce - time.Millisecond)
	h.frame()
	if occ() != 2 {
		t.Fatalf("before the debounce: %d occurrences, want bar's 2 kept", occ())
	}
	h.now = h.now.Add(time.Millisecond)
	h.frame()
	if occ() != 3 { // "foo" ×3; not "foo1"
		t.Fatalf("occurrences after the debounce: %d, want 3", occ())
	}
	h.frame()
	noWake("shown")

	// Holding an arrow key: a move every 30 ms restarts the debounce; the
	// old occurrences stay meanwhile.
	for range 5 {
		h.key(input.KeyRight, 0)
		h.frame()
		if occ() != 3 {
			t.Fatalf("held key: occurrences %d, want the old 3 kept", occ())
		}
		h.now = h.now.Add(30 * time.Millisecond)
	}
	// The caret is now at 6, in "bar": when the debounce runs, "foo" is
	// replaced by "bar" in the same frame (no blank frame).
	h.frame()
	if occ() != 3 {
		t.Fatalf("before the debounce: %d", occ())
	}
	h.now = h.now.Add(occDebounce)
	h.frame()
	if occ() != 2 {
		t.Fatalf("bar occurrences: %d, want 2", occ())
	}
	noWake("bar shown")

	// Onto another occurrence of "bar": they stay.
	st.MoveCaret(st.Snapshot().LineStart(2)+1, false)
	h.frame()
	h.now = h.now.Add(occDebounce)
	h.frame()
	if occ() != 2 {
		t.Fatalf("moving between occurrences: %d", occ())
	}

	// Typing removes them at once; its caret moves do not bring them back.
	h.typeText("x")
	h.frame()
	if occ() != 0 {
		t.Fatalf("occurrences right after typing: %d", occ())
	}
	h.now = h.now.Add(time.Second)
	h.frame()
	if occ() != 0 {
		t.Fatalf("occurrences after typing: %d", occ())
	}
	noWake("typed")
	h.key(input.KeyLeft, 0) // "bxar"
	h.frame()
	h.now = h.now.Add(occDebounce)
	h.frame()
	if occ() != 1 {
		t.Fatalf("occurrences after a cursor move: %d, want 1", occ())
	}

	// A delay: the old occurrences go when the highlighter runs, the new
	// ones come after the delay.
	st.Options.OccurrencesHighlightDelay = 200
	st.MoveCaret(1, false)
	h.frame()
	h.now = h.now.Add(occDebounce)
	h.frame()
	if occ() != 0 {
		t.Fatalf("with a delay: %d occurrences during it", occ())
	}
	wakeIn("delay", 200*time.Millisecond)
	h.now = h.now.Add(200 * time.Millisecond)
	h.frame()
	if occ() != 3 {
		t.Fatalf("after the delay: %d, want 3", occ())
	}

	// Off, and a selection over two lines: none.
	st.SetSelections([]Selection{{Anchor: 1, Head: 14}})
	h.frame()
	h.now = h.now.Add(time.Second)
	h.frame()
	if occ() != 0 {
		t.Fatalf("multi-line selection: %d occurrences", occ())
	}
	st.Options.OccurrencesHighlight = OccurrencesOff
	st.MoveCaret(1, false)
	h.frame()
	h.now = h.now.Add(time.Second)
	h.frame()
	if occ() != 0 {
		t.Fatal("occurrences with the option off")
	}
}

// TestSelectionHighlight: the other matches of a one-line selection are
// marked at once, ignoring case; not the selections themselves, not over
// 200 characters, and with several cursors only when they hold the same
// text.
func TestSelectionHighlight(t *testing.T) {
	goText(t)
	long := strings.Repeat("ab", 101) // 202 characters
	st := NewState("Value value VALUE\nvalue  \n"+long+" "+long+"\nvalUE other\n", "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 480, H: 200}, 1)
	h.focusAt(0)
	n := func() int { return h.vw.vs.decoCount[decoSelHL] }
	st.SetSelections([]Selection{{Anchor: 6, Head: 11}})
	h.frame()
	if n() != 4 {
		t.Fatalf("selection matches %d, want 4", n())
	}
	st.SetSelections([]Selection{{Anchor: 24, Head: 26}}) // blanks only
	h.frame()
	if n() != 0 {
		t.Fatalf("blank selection matches %d", n())
	}
	st.SetSelections([]Selection{{Anchor: 6, Head: 20}}) // two lines
	h.frame()
	if n() != 0 {
		t.Fatalf("multi-line selection matches %d", n())
	}
	snap := st.Snapshot()
	l2 := snap.LineStart(2)
	st.SetSelections([]Selection{{Anchor: l2, Head: l2 + 200}}) // 200: highlighted
	h.frame()
	if n() == 0 {
		t.Fatal("no matches of a 200 character selection")
	}
	st.SetSelections([]Selection{{Anchor: l2, Head: l2 + 201}}) // over the cap
	h.frame()
	if n() != 0 {
		t.Fatalf("201 character selection: %d matches", n())
	}
	// Several cursors on the same text (any case): their matches but
	// themselves.
	l3 := snap.LineStart(3)
	st.SetSelections([]Selection{{Anchor: 0, Head: 5}, {Anchor: l3, Head: l3 + 5}})
	h.frame()
	if n() != 3 {
		t.Fatalf("two cursors on value: %d matches, want 3", n())
	}
	// Several cursors on different texts: none.
	st.SetSelections([]Selection{{Anchor: 0, Head: 5}, {Anchor: l3 + 6, Head: l3 + 11}})
	h.frame()
	if n() != 0 {
		t.Fatalf("two cursors on different texts: %d matches", n())
	}
}

// TestUnicodeAmbiguousHighlight: a Cyrillic "а" in an English word is
// marked; a plainly Cyrillic word and comments are not.
func TestUnicodeAmbiguousHighlight(t *testing.T) {
	o := DefaultOptions()
	uo := o.unicodeOpts()
	if r, ok := confusable('а'); !ok || r != 'a' {
		t.Fatalf("confusable(а) = %q %v", r, ok)
	}
	line := "let pаssword = 1 // привет"
	got := unicodeMarks(line, uo, nil, nil)
	i := strings.Index(line, "а")
	if len(got) != 1 || got[0] != (brange{i, i + 2}) {
		t.Fatalf("marks %v, want [%d %d]", got, i, i+2)
	}
	inComment := func(int) string { return "comment" }
	if got := unicodeMarks("x // pаss", uo, inComment, nil); len(got) != 0 {
		t.Fatalf("marked in a comment: %v", got)
	}
	o.UnicodeHighlight.IncludeComments = UnicodeHighlightTrue
	if got := unicodeMarks("x // pаss", o.unicodeOpts(), inComment, nil); len(got) != 1 {
		t.Fatalf("IncludeComments: %v", got)
	}
	o = DefaultOptions()
	o.UnicodeHighlight.AmbiguousCharacters = false
	if got := unicodeMarks(line, o.unicodeOpts(), nil, nil); len(got) != 0 {
		t.Fatalf("AmbiguousCharacters off: %v", got)
	}
	o = DefaultOptions()
	o.UnicodeHighlight.AllowedCharacters = "а"
	if got := unicodeMarks(line, o.unicodeOpts(), nil, nil); len(got) != 0 {
		t.Fatalf("allowed: %v", got)
	}
	o = DefaultOptions()
	o.UnicodeHighlight.NonBasicASCII = UnicodeHighlightTrue
	if got := unicodeMarks("é привет", o.unicodeOpts(), nil, nil); len(got) != 7 {
		t.Fatalf("NonBasicASCII: %d marks, want 7", len(got))
	}
	// A line with no candidates must not ask for token types (the View
	// fetches tokens lazily through this callback).
	o = DefaultOptions()
	calls := 0
	unicodeMarks(strings.Repeat("中文测试", 100), o.unicodeOpts(), func(int) string {
		calls++
		return ""
	}, nil)
	if calls != 0 {
		t.Fatalf("%d token lookups for a CJK line with no candidates", calls)
	}
	// In the View: a box on the drawn row.
	goText(t)
	st := NewState("let pаssword = 1\n", "a.txt", theme.Base("vs-dark"))
	h := newHintHost(t, st, geom.Size{W: 480, H: 200}, 1)
	h.frame()
	if n := h.vw.vs.decoCount[decoUnicode]; n != 1 {
		t.Fatalf("view marks %d", n)
	}
}

// TestInvisibleCharacters: zero width and odd blanks are marked, the
// space and the tab are not.
func TestInvisibleCharacters(t *testing.T) {
	def := DefaultOptions()
	uo := def.unicodeOpts()
	for _, c := range []struct {
		s    string
		want int
	}{
		{"a\u200bb", 1},   // zero width space
		{"a\u3000b", 1},   // ideographic space
		{"x\u00adx", 1},   // soft hyphen
		{"a b\tc", 0},     // space, tab
		{"\u2028", 1},     // line separator
		{"a\ufeffb", 1},   // BOM
		{"a\u200db", 1},   // zero width joiner
		{"\x0b\x7f", 2},   // vertical tab, DEL
		{"中文，标点", 0},      // CJK punctuation
		{"emoji 😀 ok", 0}, // not invisible
	} {
		if got := unicodeMarks(c.s, uo, nil, nil); len(got) != c.want {
			t.Errorf("%q: %d marks %v, want %d", c.s, len(got), got, c.want)
		}
	}
	o := DefaultOptions()
	o.UnicodeHighlight.InvisibleCharacters = false
	if got := unicodeMarks("a\u200bb", o.unicodeOpts(), nil, nil); len(got) != 0 {
		t.Errorf("InvisibleCharacters off: %v", got)
	}
}

// TestLinkDetect: Monaco's LinkComputer cases, the trailing punctuation
// and brackets rules included; a Cmd/Ctrl+click opens the link.
func TestLinkDetect(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`x = "http://foo.bar";`, "http://foo.bar"},
		{`x = (http://foo.bar);`, "http://foo.bar"},
		{`x = 'http://foo.bar';`, "http://foo.bar"},
		{`x =  http://foo.bar ;`, "http://foo.bar"},
		{`x = <http://foo.bar>;`, "http://foo.bar"},
		{`x = {http://foo.bar};`, "http://foo.bar"},
		{`x = [http://foo.bar];`, "http://foo.bar"},
		{`(see http://foo.bar)`, "http://foo.bar"},
		{`[see http://foo.bar]`, "http://foo.bar"},
		{`<url>http://mylink.com</url>`, "http://mylink.com"},
		{`// More: https://go.microsoft.com/fwlink/?LinkID=513275&clcid=0x409`, "https://go.microsoft.com/fwlink/?LinkID=513275&clcid=0x409"},
		{`see http://foo.bar/path. More`, "http://foo.bar/path"},
		{`http://foo.bar.,;:`, "http://foo.bar"},
		{`https://foo.bar/#/path,`, "https://foo.bar/#/path"},
		{`http://[::1]:5000/connect/token`, "http://[::1]:5000/connect/token"},
		{`https://en.wikipedia.org/wiki/Tree_(data_structure)`, "https://en.wikipedia.org/wiki/Tree_(data_structure)"},
		{`(https://en.wikipedia.org/wiki/Tree_(data_structure))`, "https://en.wikipedia.org/wiki/Tree_(data_structure)"},
		{`*https://foo.bar*`, "https://foo.bar"},
		{`"https://foo.bar/'x'"`, "https://foo.bar/'x'"},
		{`HTTPS://FOO.BAR/x`, "HTTPS://FOO.BAR/x"},
		{`file:///c:/x/y.txt`, "file:///c:/x/y.txt"},
		{`见 https://foo.bar/路径。下一句`, "https://foo.bar/路径"},
		{`ftp://foo.bar`, ""},
		{`http:/foo.bar`, ""},
	} {
		ls := computeLinks(c.line, nil)
		got := ""
		if len(ls) > 0 {
			got = ls[0].url
			if c.line[ls[0].a:ls[0].b] != got {
				t.Errorf("%q: range %v does not hold the url", c.line, ls[0])
			}
		}
		if got != c.want || len(ls) > 1 {
			t.Errorf("%q: links %v, want %q", c.line, ls, c.want)
		}
	}

	goText(t)
	st := NewState("go to https://example.com/x now\n", "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 480, H: 200}, 1)
	h.frame()
	if n := h.vw.vs.decoCount[decoLink]; n != 1 || len(h.vw.vs.linkHits) != 1 {
		t.Fatalf("links %d, hits %d", n, len(h.vw.vs.linkHits))
	}
	var opened []string
	h.vw.OnOpenLink(func(u string) { opened = append(opened, u) })
	hit := h.vw.vs.linkHits[0].r
	p := geom.Pt(hit.X+hit.W/2, hit.Y+hit.H/2)
	h.click(p, 0)
	h.frame()
	if len(opened) != 0 {
		t.Fatal("a plain click opened the link")
	}
	// Hovering with the modifier makes it active (its color), a click
	// opens it.
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p, Mods: shortcutMod()})
	h.frame()
	h.frame()
	if h.vw.vs.activeLink.line != 0 {
		t.Fatalf("active link %v", h.vw.vs.activeLink)
	}
	h.click(p, shortcutMod())
	h.frame()
	if len(opened) != 1 || opened[0] != "https://example.com/x" {
		t.Fatalf("opened %v", opened)
	}
	st.Options.Links = false
	h.frame()
	if h.vw.vs.decoCount[decoLink] != 0 {
		t.Fatal("links with the option off")
	}
}

// TestColorDecoratorEdit: CSS color literals get swatches; picking a color
// writes it back in the literal's presentation, and the picker's changes
// undo as one step.
func TestColorDecoratorEdit(t *testing.T) {
	for _, c := range []struct {
		line string
		want []color.NRGBA
	}{
		{"color: #ff0000;", []color.NRGBA{{255, 0, 0, 255}}},
		{"a: #f00 #f008 #ff000080", []color.NRGBA{{255, 0, 0, 255}, {255, 0, 0, 0x88}, {255, 0, 0, 0x80}}},
		{`"#abc"`, []color.NRGBA{{0xaa, 0xbb, 0xcc, 255}}},
		{"#fff at line start", []color.NRGBA{{255, 255, 255, 255}}},
		{"x#fff #fffff #ffg", nil},
		{"rgb(255, 0, 0) rgb(0 128 0)", []color.NRGBA{{255, 0, 0, 255}, {0, 128, 0, 255}}},
		{"rgba(0, 0, 255, 0.5) rgba(0 0 255 / .5)", []color.NRGBA{{0, 0, 255, 128}, {0, 0, 255, 128}}},
		{"hsl(120, 100%, 25%)", []color.NRGBA{{0, 128, 0, 255}}},
		{"hsla(240, 100%, 50%, 0.25)", []color.NRGBA{{0, 0, 255, 64}}},
		{"rgb(256, 0, 0) xrgb(1,2,3)", nil},
	} {
		var got []color.NRGBA
		for _, sp := range computeColors(c.line, nil) {
			got = append(got, sp.c)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%q: colors %v, want %v", c.line, got, c.want)
		}
	}
	for _, c := range []struct {
		lit  string
		col  color.NRGBA
		want string
	}{
		{"#ff0000", color.NRGBA{0, 128, 0, 255}, "#008000"},
		{"#ff0000", color.NRGBA{0, 128, 0, 128}, "#00800080"},
		{"rgb(255, 0, 0)", color.NRGBA{0, 128, 0, 255}, "rgb(0, 128, 0)"},
		{"rgb(255, 0, 0)", color.NRGBA{0, 128, 0, 128}, "rgba(0, 128, 0, 0.5)"},
		{"hsl(0, 100%, 50%)", color.NRGBA{0, 0, 255, 255}, "hsl(240, 100%, 50%)"},
		{"hsl(0, 100%, 50%)", color.NRGBA{0, 0, 255, 64}, "hsla(240, 100%, 50%, 0.25)"},
	} {
		sp := computeColors(c.lit, nil)
		if len(sp) != 1 {
			t.Fatalf("%q: %v", c.lit, sp)
		}
		if got := colorFormats[guessPresentation(sp[0].c, c.lit)](c.col); got != c.want {
			t.Errorf("%q → %v: %q, want %q", c.lit, c.col, got, c.want)
		}
	}

	goText(t)
	const doc = "a {\n  color: #ff0000;\n}\n"
	st := NewState(doc, "a.css", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 480, H: 300}, 1)
	h.focusAt(0)
	vs := h.vw.vs
	if len(vs.swatches) != 1 {
		t.Fatalf("swatches %v", vs.swatches)
	}
	sw := vs.swatches[0].r
	head := st.Primary().Head
	h.click(geom.Pt(sw.X+sw.W/2, sw.Y+sw.H/2), 0)
	h.frame()
	h.frame()
	if vs.picker == nil {
		t.Fatal("no picker after clicking the swatch")
	}
	if st.Primary().Head != head {
		t.Fatal("the swatch click moved the caret")
	}
	vs.picker.col = color.NRGBA{0, 128, 0, 255}
	h.frame()
	if got := st.Snapshot().Line(1); got != "  color: #008000;" {
		t.Fatalf("after picking: %q", got)
	}
	vs.picker.col = color.NRGBA{0, 0, 255, 255}
	h.frame()
	if got := st.Snapshot().Line(1); got != "  color: #0000ff;" {
		t.Fatalf("after picking again: %q", got)
	}
	h.key(input.KeyEscape, 0)
	h.frame()
	h.frame()
	if vs.picker != nil {
		t.Fatal("Escape did not close the picker")
	}
	if !st.Undo() || st.Snapshot().String() != doc {
		t.Fatalf("undo: %q, want the original text", st.Snapshot().String())
	}
	st.Options.ColorDecorators = false
	h.frame()
	if len(vs.swatches) != 0 {
		t.Fatal("swatches with the option off")
	}
}

// TestColorSwatchPointer: a literal starting its line gets a swatch in the
// gap injected before it (the literal moves right past it) that opens the
// picker; only the swatch takes the press (a click on the quote before
// "#fff" places the caret); the picker follows the swatch when the view
// scrolls and closes when it leaves the view.
func TestColorSwatchPointer(t *testing.T) {
	goText(t)
	var sb strings.Builder
	sb.WriteString("#ff0000\nx = \"#fff\"\n")
	for i := range 60 {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	st := NewState(sb.String(), "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 400, H: 200}, 1)
	h.focusAt(5)
	vs := h.vw.vs
	if len(vs.swatches) != 2 {
		t.Fatalf("swatches %v, want 2", vs.swatches)
	}
	first := vs.swatches[0]
	if first.a != 0 || first.r.X < vs.text.X || first.r.W < 4 || first.r.H < 4 {
		t.Fatalf("line start swatch %v (text area x %v)", first.r, vs.text.X)
	}
	if lv := h.vw.rowAt(st.Snapshot(), 0, 0); true {
		if x, _ := lv.caretXSide(0, true); vs.text.X+x < first.r.Right() {
			t.Fatalf("the literal at %v is under its swatch %v", vs.text.X+x, first.r)
		}
	}
	// The quote before "#fff": the caret goes there, no picker.
	snap := st.Snapshot()
	quote := snap.LineStart(1) + 4
	qx, _ := h.vw.contentX(snap, quote)
	qp := geom.Pt(vs.text.X+qx+0.1, vs.swatches[1].r.Y+vs.swatches[1].r.H/2)
	if vs.swatches[1].r.Contains(qp) {
		t.Fatal("the swatch covers the quote's left edge: test point off")
	}
	h.click(qp, 0)
	h.frame()
	if vs.picker != nil || (st.Primary().Head < quote || st.Primary().Head > quote+1) {
		t.Fatalf("click on the quote: picker %v, caret %d want %d", vs.picker != nil, st.Primary().Head, quote)
	}
	// The line start swatch opens the picker.
	h.click(geom.Pt(first.r.X+first.r.W/2, first.r.Y+first.r.H/2), 0)
	h.frame()
	h.frame()
	if vs.picker == nil {
		t.Fatal("no picker from the line start swatch")
	}
	// Scrolling a little: the picker follows the swatch.
	y0 := vs.picker.anchor.Y
	st.ScrollBy(3, 0)
	h.frame()
	h.frame()
	if vs.picker == nil || vs.picker.anchor.Y != y0-3 {
		t.Fatalf("picker after scrolling 3 px: %+v (swatch was at y %v)", vs.picker, y0)
	}
	// Scrolled out of view: closed.
	st.ScrollBy(200, 0)
	h.frame()
	h.frame()
	if vs.picker != nil {
		t.Fatal("picker open with its swatch scrolled away")
	}
}

// TestDecorationLimits: at most maxFindMatches occurrences or selection
// matches are drawn, and no unicode mark at all when there are more than
// maxUnicodeMarks.
func TestDecorationLimits(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat(strings.Repeat("ab ", 100)+"\n", 12)+strings.Repeat("\u200b", 1001)+"\n", "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 400, H: 400}, 1)
	h.focusAt(0)
	st.MoveCaret(1, false)
	h.frame()
	h.now = h.now.Add(time.Second)
	h.frame()
	if n := h.vw.vs.decoCount[decoOcc]; n != maxFindMatches {
		t.Fatalf("occurrences %d, want %d", n, maxFindMatches)
	}
	st.SetSelections([]Selection{{Anchor: 0, Head: 2}})
	h.frame()
	if n := h.vw.vs.decoCount[decoSelHL]; n != maxFindMatches-1 {
		t.Fatalf("selection matches %d, want %d (the selected match consumes quota)", n, maxFindMatches-1)
	}
	if n := h.vw.vs.decoCount[decoUnicode]; n != 0 {
		t.Fatalf("%d unicode marks over the limit", n)
	}
	st2 := NewState(strings.Repeat("\u200b", maxUnicodeMarks)+"\n", "a.txt", theme.Base("vs"))
	h2 := newHintHost(t, st2, geom.Size{W: 400, H: 200}, 1)
	h2.frame()
	if n := h2.vw.vs.decoCount[decoUnicode]; n != maxUnicodeMarks {
		t.Fatalf("%d unicode marks, want %d", n, maxUnicodeMarks)
	}
}

// TestWheelZoom: with MouseWheelZoom, Cmd/Ctrl+wheel changes the zoom
// level (and so the font size) instead of scrolling; the fontZoom
// commands step and reset it.
func TestWheelZoom(t *testing.T) {
	goText(t)
	t.Cleanup(func() { SetZoomLevel(0) })
	st := NewState(hintDoc(200), "main.go", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 480, H: 300}, 1)
	h.frame()
	lh0 := st.Viewport.LineHeight
	p := geom.Pt(240, 150)
	// gap ends the wheel gesture.
	gap := func() { h.now = h.now.Add(2 * zoomGestureGap) }
	wheel := func(dy float32, mods input.Modifiers) {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy, Mods: mods})
		h.frame()
	}
	wheel(-zoomWheelPx, shortcutMod())
	if ZoomLevel() != 0 {
		t.Fatal("zoomed with MouseWheelZoom off")
	}
	st.Options.MouseWheelZoom = true
	gap()
	wheel(-zoomWheelPx, shortcutMod())
	h.frame()
	if ZoomLevel() != 1 {
		t.Fatalf("zoom level %v, want 1", ZoomLevel())
	}
	if lh := st.Viewport.LineHeight; lh <= lh0 {
		t.Fatalf("line height %v, want more than %v", lh, lh0)
	}
	gap()
	wheel(zoomWheelPx, shortcutMod())
	if ZoomLevel() != 0 || st.Viewport.TopLine != 0 || st.Viewport.OffsetY != 0 {
		t.Fatalf("zoom out: level %v, top %d %v (no scrolling)", ZoomLevel(), st.Viewport.TopLine, st.Viewport.OffsetY)
	}
	// A trackpad gesture: small steps every 16 ms accumulate, fractional
	// levels; 0.4 s of fast swiping (25 frames of 20 px) is 2.5 levels, far
	// from the maximum.
	gap()
	for range 25 {
		wheel(-20, shortcutMod())
		h.now = h.now.Add(16 * time.Millisecond)
	}
	if z := ZoomLevel(); z < 2.49 || z > 2.51 {
		t.Fatalf("trackpad gesture: level %v, want 2.5", z)
	}
	// The modifier is locked at the gesture's start: released during it
	// (inertia), the gesture still zooms...
	wheel(zoomWheelPx, 0)
	if z := ZoomLevel(); z < 1.49 || z > 1.51 {
		t.Fatalf("inertia without the modifier: level %v, want 1.5", z)
	}
	// ...and a gesture started without it scrolls even when it is pressed.
	SetZoomLevel(0)
	gap()
	wheel(40, 0)
	top, off := st.Viewport.TopLine, st.Viewport.OffsetY
	h.now = h.now.Add(16 * time.Millisecond)
	wheel(40, shortcutMod())
	if ZoomLevel() != 0 || st.Viewport.TopLine == top && st.Viewport.OffsetY == off {
		t.Fatalf("scroll gesture with the modifier pressed: level %v, top %d", ZoomLevel(), st.Viewport.TopLine)
	}
	for range 30 {
		st.Execute("editor.action.fontZoomIn", nil)
	}
	if ZoomLevel() != 20 {
		t.Fatalf("zoom in clamps at 20: %v", ZoomLevel())
	}
	st.Execute("editor.action.fontZoomReset", nil)
	st.Execute("editor.action.fontZoomOut", nil)
	h.frame()
	if ZoomLevel() != -1 || st.Viewport.LineHeight >= lh0 {
		t.Fatalf("zoom out: level %v, line height %v", ZoomLevel(), st.Viewport.LineHeight)
	}
	st.Execute("editor.action.fontZoomReset", nil)
	h.frame()
	if ZoomLevel() != 0 || st.Viewport.LineHeight != lh0 {
		t.Fatalf("reset: level %v, line height %v", ZoomLevel(), st.Viewport.LineHeight)
	}
}

// TestLargeFileDegrades: a large file has no folding, word wrap,
// occurrences or selection matches; it keeps syntax highlighting, bracket
// colors (they go over 5 000 000 bytes only) and the unicode marks; links
// and color swatches go only past Monaco's worker sync limit.
func TestLargeFileDegrades(t *testing.T) {
	goText(t)
	var sb strings.Builder
	sb.WriteString("package main\n\nfunc f() { x := g(h(1)); pаss := 0 // http://a.b \"#fff\"\n\tif x {\n\t\treturn\n\t}\n}\n")
	for range LargeFileLines {
		sb.WriteString("var x = 1\n")
	}
	st := NewState(sb.String(), "main.go", theme.Base("vs-dark"))
	if !st.LargeFile() {
		t.Fatal("not a large file")
	}
	h := newHintHost(t, st, geom.Size{W: 640, H: 300}, 1)
	h.focusAt(2)
	snap := st.Snapshot()
	st.MoveCaret(snap.LineStart(2)+12, false) // on "x"
	h.frame()
	h.now = h.now.Add(time.Second)
	h.frame()
	vs := h.vw.vs
	if st.foldingOn() || st.Execute("editor.fold", nil) && len(st.fold.folds) > 0 {
		t.Error("folding on")
	}
	if vs.decoCount[decoOcc] != 0 {
		t.Error("occurrences highlighted")
	}
	st.SetSelections([]Selection{{Anchor: snap.LineStart(4) + 2, Head: snap.LineStart(4) + 8}})
	h.frame()
	if vs.decoCount[decoSelHL] != 0 {
		t.Error("selection matches highlighted")
	}
	// Word wrap: off whatever the options (Monaco shows large files
	// unwrapped).
	st.Options.WordWrap = WrapOn
	h.frame()
	if st.wrapping() || st.WordWrap() != WrapOff {
		t.Error("a large file wraps")
	}
	st.Options.WordWrap = WrapOff
	h.frame()
	// Still on: bracket colors (under 5 000 000 bytes), syntax
	// highlighting, unicode marks, links, swatches.
	if snap.Len() >= maxBracketTreeLen {
		t.Fatalf("test document of %d bytes is too large for a bracket tree", snap.Len())
	}
	if len(vs.brk[2]) == 0 {
		t.Errorf("no bracket colors on line 3 (%v)", vs.brk)
	}
	toks, _ := st.Highlighter().TokensFor(st.hlKey, snap, 2, nil)
	if len(toks) < 3 {
		t.Errorf("no highlighting: %v", toks)
	}
	if vs.decoCount[decoUnicode] != 1 || vs.decoCount[decoLink] != 1 || vs.decoCount[decoSwatch] != 1 {
		t.Errorf("decorations %v, want a unicode mark, a link and a swatch", vs.decoCount)
	}
	// Past the worker sync limit (50 MB): no links, no swatches.
	st.Model.syncLarge = true
	h.frame()
	if vs.decoCount[decoLink] != 0 || vs.decoCount[decoSwatch] != 0 || vs.decoCount[decoUnicode] != 1 {
		t.Errorf("past the sync limit: %v", vs.decoCount)
	}

	// Over 5 000 000 bytes: no bracket colors (Monaco's canBuildAST).
	big := NewState("f(g(1))\n"+strings.Repeat("var x = (1)\n", maxBracketTreeLen/12+1), "main.go", theme.Base("vs-dark"))
	hb := newHintHost(t, big, geom.Size{W: 640, H: 300}, 1)
	hb.frame()
	hb.frame()
	if len(hb.vw.vs.brk) != 0 {
		t.Errorf("bracket colors over 5 000 000 bytes: %v", hb.vw.vs.brk)
	}
}

// TestDocumentHighlightQuota: the first 999 matches of the document, not
// of the viewport, consume the quota. The selected match consumes quota
// too (1000 document matches minus the last, then minus the selected first,
// leaves 998). Scrolling neither changes that set nor reruns the search.
func TestDocumentHighlightQuota(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("ab\n", 1000), "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 400, H: 200}, 1)
	h.focusAt(0)
	st.MoveCaret(1, false)
	h.frame()
	h.now = h.now.Add(occDebounce)
	h.frame()
	vs := h.vw.vs
	if len(vs.occMatches.ranges) != 999 || vs.occMatches.ranges[998].a != 998*3 {
		t.Fatalf("document occurrences: %d, want first 999", len(vs.occMatches.ranges))
	}
	st.SetSelections([]Selection{{Anchor: 0, Head: 2}})
	h.frame()
	if len(vs.selMatches.ranges) != 999 {
		t.Fatalf("selection search quota: %d", len(vs.selMatches.ranges))
	}
	// All ranges are absolute offsets. Selection exclusion must not mutate
	// the cached search results.
	vis, _ := visibleSels(st.Snapshot(), st.sels(), 0, st.Snapshot().Len())
	if n := len(dropSelected(slices.Clone(vs.selMatches.ranges), 0, vis)); n != 998 {
		t.Fatalf("after excluding selected first match: %d, want 998", n)
	}
	oc, sc := vs.occMatches.scans, vs.selMatches.scans
	for _, top := range []int{995, 999, 0, 999} {
		st.Viewport.TopLine, st.Viewport.OffsetY = top, 0
		h.frame()
		if top == 999 {
			// The two overscan rows above the viewport still hold valid
			// matches; the viewport's own line (match 1000) has none.
			ds := h.vw.lineDecos(st.Snapshot(), 999, "ab", nil, vis, &decorFrame{
				occ: vs.occMatches.ranges, sel: vs.selMatches.ranges,
			})
			for _, d := range ds {
				if d.kind == decoOcc || d.kind == decoSelHL {
					t.Fatalf("match 1000 highlighted after scrolling: %+v", ds)
				}
			}
		} else if top == 995 {
			// The two overscan rows above 995 and matches 995..998 remain in
			// quota. The last row (999) must never enter it.
			if vs.decoCount[decoOcc] != 6 || vs.decoCount[decoSelHL] != 6 {
				t.Fatalf("near quota edge: %v, want six eligible overscan/visible matches", vs.decoCount)
			}
		}
		if vs.occMatches.scans != oc || vs.selMatches.scans != sc {
			t.Fatal("scrolling rescanned document")
		}
	}
	// A version change invalidates both cached searches. Occurrences still
	// respect the edit-clears-until-cursor-move rule.
	st.MoveCaret(0, false)
	st.Insert("z")
	h.frame()
	if vs.decoCount[decoOcc] != 0 {
		t.Fatal("edit kept occurrences")
	}
	st.MoveCaret(5, false)
	h.frame()
	h.now = h.now.Add(occDebounce)
	h.frame()
	if vs.occMatches.scans <= oc {
		t.Fatal("edit did not invalidate occurrence search")
	}
}

// A supplementary-plane character uses two UTF-16 code units: 100 emoji
// fit selectionHighlightMax, 101 do not.
func TestSelectionHighlightUTF16Boundary(t *testing.T) {
	goText(t)
	str := strings.Repeat("😀", 101)
	st := NewState(str+" "+str, "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 400, H: 200}, 1)
	h.frame()
	st.SetSelections([]Selection{{Anchor: 0, Head: 400}})
	h.frame()
	if len(h.vw.vs.selMatches.ranges) != 2 || h.vw.vs.decoCount[decoSelHL] != 1 {
		t.Fatalf("200 UTF-16 units not matched: %v", h.vw.vs.decoCount)
	}
	st.SetSelections([]Selection{{Anchor: 0, Head: 404}})
	h.frame()
	if h.vw.vs.decoCount[decoSelHL] != 0 {
		t.Fatal("202 UTF-16 units highlighted")
	}
}
