package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
)

// wrapPieces are the fragments of the generated long lines: words and
// blanks, CJK with its punctuation, tabs, emoji (with ZWJ and variation
// selectors), combining marks, long runs without break points, break-before
// and break-after characters, and color literals (the injected gaps).
var wrapPieces = []string{
	"word ", "identifier", " ", "  ", "\t", "\tx\t", "你好世界", "中文，标点。", "カタカナー",
	"😀", "👩‍💻", "❤️", "é̂", "á̂", strings.Repeat("x", 97), "foo(bar)", "a/b/c?d=1&e",
	"[1, 2, 3];", "#fff ", "#12345678 ", "rgb(1, 2, 3) ", "hsl(10, 20%, 30%) ", "#", "rgb(", "f", "0",
}

// genWrapLine returns about n bytes of wrapPieces, starting with indent.
func genWrapLine(r *rand.Rand, n int, indent string) string {
	var sb strings.Builder
	sb.WriteString(indent)
	for sb.Len() < n {
		sb.WriteString(wrapPieces[r.IntN(len(wrapPieces))])
	}
	return sb.String()
}

// fixBoundary moves byte offset i of s back to a rune start.
func fixBoundary(s string, i int) int {
	for i > 0 && i < len(s) && s[i]&0xC0 == 0x80 {
		i--
	}
	return i
}

func eqBreaks(a, b lineBreaks) bool {
	return a.indent == b.indent && slices.Equal(a.starts, b.starts) && slices.Equal(a.runes, b.runes)
}

// randomEdit returns a random insertion, deletion or replacement in s
// (mostly small, sometimes large; sometimes in the leading whitespace or
// making or breaking a color literal).
func randomEdit(r *rand.Rand, s string) (at, oldLen int, ins string) {
	at = fixBoundary(s, r.IntN(len(s)+1))
	if r.IntN(10) == 0 {
		at = fixBoundary(s, r.IntN(min(len(s), 8)+1)) // near the start
	}
	switch r.IntN(3) {
	case 0: // insert
	case 1: // delete
		oldLen = 1 + r.IntN(8)
	default: // replace
		oldLen = 1 + r.IntN(4)
	}
	if r.IntN(20) == 0 {
		oldLen = r.IntN(4096)
	}
	end := fixBoundary(s, min(len(s), at+oldLen))
	if end < at {
		end = at
	}
	oldLen = end - at
	if oldLen == 0 || r.IntN(3) != 1 {
		switch k := r.IntN(12); {
		case k < 6:
			ins = wrapPieces[r.IntN(len(wrapPieces))]
		case k < 8:
			ins = string(rune('a' + r.IntN(26)))
		case k < 9:
			ins = " "
		case k < 10:
			ins = "#abc"
		case k < 11:
			ins = genWrapLine(r, 64<<r.IntN(8), "")
		default:
			ins = "́" // a combining mark joining the previous cluster
		}
	}
	return at, oldLen, ins
}

// TestWrapIncrementalMatchesFull: rewrapEdit gives computeBreaks' result
// after 500 random edits of a 1 MB line (CJK, tabs, emoji, combining
// marks, long runs without break points) in every wrappingIndent mode, and
// of lines below the long-line limit holding color literals with their
// injected gaps (a long line has none: injectCfg.injections), so edits make,
// break and move gaps.
func TestWrapIncrementalMatchesFull(t *testing.T) {
	gaps := injectCfg{w: 17, colW: 8.4}
	big, edits := 1<<20, 500
	if testing.Short() {
		big, edits = 64<<10, 100
	}
	for ci, c := range []struct {
		name   string
		cfg    wrapConfig
		indent string
		size   int
	}{
		{"same", wrapConfig{col: 80, tab: 4, indent: WrapIndentSame}, "\t  ", big},
		{"indent", wrapConfig{col: 61, tab: 4, indent: WrapIndentIndent, inj: gaps}, "  ", 256 << 10},
		{"deep-tab8", wrapConfig{col: 120, tab: 8, indent: WrapIndentDeep}, "\t", 256 << 10},
		{"none-narrow", wrapConfig{col: 7, tab: 4, indent: WrapIndentNone}, "", 128 << 10},
		{"gaps-same", wrapConfig{col: 30, tab: 4, indent: WrapIndentSame, inj: gaps}, " \t", 1800},
		{"gaps-indent", wrapConfig{col: 23, tab: 2, indent: WrapIndentIndent, inj: gaps}, "  ", 1800},
		{"gaps-deep", wrapConfig{col: 40, tab: 4, indent: WrapIndentDeep, inj: injectCfg{w: 10, colW: 4}}, "\t", 1800},
		{"gaps-none-narrow", wrapConfig{col: 7, tab: 4, indent: WrapIndentNone, inj: injectCfg{w: 10, colW: 4}}, "", 1800},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := rand.New(rand.NewPCG(5, uint64(ci)))
			s := genWrapLine(r, c.size, c.indent)
			lb := computeBreaks(s, c.cfg)
			total, withGaps := 0, 0
			for i := range edits {
				at, oldLen, ins := randomEdit(r, s)
				if c.size < 4096 && len(ins) > 256 {
					ins = ins[:fixBoundary(ins, 256)]
				}
				ns := s[:at] + ins + s[at+oldLen:]
				if c.size < 4096 && len(ns) > 3*c.size/2 {
					ns = s[:at] + s[at+oldLen:] // keep it short: a deletion
					ins = ""
				}
				got, scanned := rewrapEdit(lineBreaks{}, lb, s, ns, at, oldLen, len(ins), c.cfg)
				want := computeBreaks(ns, c.cfg)
				if !eqBreaks(got, want) {
					k := 0
					for k < len(got.starts) && k < len(want.starts) && got.starts[k] == want.starts[k] && got.runes[k] == want.runes[k] {
						k++
					}
					t.Fatalf("edit %d (at %d, -%d +%q): %d breaks (indent %d), want %d (indent %d); first difference at break %d",
						i, at, oldLen, ins, len(got.starts), got.indent, len(want.starts), want.indent, k)
				}
				if len(c.cfg.inj.injections(ns, nil)) > 0 {
					withGaps++
				}
				total += scanned
				s, lb = ns, got
			}
			if c.cfg.inj.on() && c.size < 4096 && withGaps < edits/2 {
				t.Fatalf("only %d of %d edits left gaps in the line", withGaps, edits)
			}
			t.Logf("%d edits scanned %d runes (line %d bytes, %d with gaps)", edits, total, len(s), withGaps)
		})
	}
}

// TestWrapIncrementalBounded: an edit in the middle of a 1 MB line scans a
// few rows around it, a 64 KB paste the pasted text and a few rows.
func TestWrapIncrementalBounded(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 7))
	for _, cfg := range []wrapConfig{
		{col: 80, tab: 4, indent: WrapIndentSame},
		{col: 80, tab: 4, indent: WrapIndentIndent, inj: injectCfg{w: 17, colW: 8.4}},
	} {
		s := genWrapLine(r, 1<<20, "  ")
		lb := computeBreaks(s, cfg)
		mid := fixBoundary(s, len(s)/2)
		for _, c := range []struct {
			ins  string
			max  int
			name string
		}{
			{"x", 4096, "one character"},
			{genWrapLine(r, 64<<10, ""), 64*1024 + 4096, "64 KB"},
		} {
			ns := s[:mid] + c.ins + s[mid:]
			got, scanned := rewrapEdit(lineBreaks{}, lb, s, ns, mid, 0, len(c.ins), cfg)
			if !eqBreaks(got, computeBreaks(ns, cfg)) {
				t.Fatalf("%s: breaks differ from computeBreaks", c.name)
			}
			if scanned > c.max {
				t.Errorf("%s (gaps %v): scanned %d runes, want <= %d", c.name, cfg.inj.on(), scanned, c.max)
			}
		}
		// Deleting a range scans a few rows too.
		ns := s[:mid] + s[fixBoundary(s, mid+30_000):]
		got, scanned := rewrapEdit(lineBreaks{}, lb, s, ns, mid, fixBoundary(s, mid+30_000)-mid, 0, cfg)
		if !eqBreaks(got, computeBreaks(ns, cfg)) {
			t.Fatal("delete: breaks differ from computeBreaks")
		}
		if scanned > 4096 {
			t.Errorf("delete: scanned %d runes, want <= 4096", scanned)
		}
	}
}

// TestWrapIncrementalLeadingWhitespace: an edit of the leading whitespace
// changes every continuation row's indentation, so the whole line is
// wrapped again.
func TestWrapIncrementalLeadingWhitespace(t *testing.T) {
	cfg := wrapConfig{col: 30, tab: 4, indent: WrapIndentSame}
	s := "  " + strings.Repeat("alpha beta gamma ", 200)
	lb := computeBreaks(s, cfg)
	ns := "\t" + s
	got, scanned := rewrapEdit(lineBreaks{}, lb, s, ns, 0, 0, 1, cfg)
	if !eqBreaks(got, computeBreaks(ns, cfg)) {
		t.Fatal("breaks differ from computeBreaks")
	}
	if got.indent == lb.indent || scanned < len(ns)-8 {
		t.Fatalf("indent %d (was %d), scanned %d of %d", got.indent, lb.indent, scanned, len(ns))
	}
}

// wrapTestEditor is wrapEditor (500×400) with fixed model options (no
// indentation detection: the edited text and a fresh copy of it must wrap
// with the same tab size) and color decorators on.
func wrapTestEditor(t *testing.T, text string) (*State, *editDriver) {
	t.Helper()
	goText(t)
	st := newTestState(text, "a.css", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	st.Options.ColorDecorators = true
	ed := newEditDriver(st, 500, 400, pinMac)
	ed.focus(t)
	return st, ed
}

// TestWrapIncrementalEditor types, deletes and pastes in the middle of a
// long wrapped line through a View: the State's rows (view line count and
// the view position of every edited offset) equal a fresh State's on the
// same text, and the line is rewrapped incrementally (its breaks stay
// cached across the edits).
func TestWrapIncrementalEditor(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 9))
	line := genWrapLine(r, 200<<10, "  ")
	text := "first line\n" + line + "\nlast line #fff\n"
	st, ed := wrapTestEditor(t, text)
	ed.Frame()
	mid := st.Snapshot().LineStart(1) + fixBoundary(line, len(line)/2)
	st.MoveCaret(mid, false)
	st.RevealCaret()
	ed.Frame()
	check := func(what string) {
		t.Helper()
		snap := st.Snapshot()
		fresh, fed := wrapTestEditor(t, snap.String())
		fresh.MoveCaret(st.Primary().Head, false)
		fresh.RevealCaret()
		fed.Frame()
		fed.Frame()
		vl, fvl := st.lines(), fresh.lines()
		if vl.cfg != fvl.cfg {
			t.Fatalf("%s: configurations differ: %+v vs %+v", what, vl.cfg, fvl.cfg)
		}
		got, want := st.breaks(1), fresh.breaks(1)
		if !eqBreaks(*got, *want) {
			t.Fatalf("%s: line 1 has %d rows, a fresh State %d", what, got.rows(), want.rows())
		}
		if !eqBreaks(*got, computeBreaks(snap.Line(1), vl.cfg)) {
			t.Fatalf("%s: rows differ from computeBreaks", what)
		}
		if vl.Count() != fvl.Count() {
			t.Fatalf("%s: %d view lines, a fresh State %d", what, vl.Count(), fvl.Count())
		}
		for _, off := range []int{st.Primary().Head, snap.LineStart(1) + len(snap.Line(1)) - 1, snap.LineStart(2)} {
			l := snap.LineOf(off)
			v0, x0 := vl.ModelToView(l, off-snap.LineStart(l))
			v1, x1 := fvl.ModelToView(l, off-snap.LineStart(l))
			if v0 != v1 || x0 != x1 {
				t.Fatalf("%s: offset %d at view line %d x %v, a fresh State %d x %v", what, off, v0, x0, v1, x1)
			}
		}
	}
	check("before")
	for i := range 40 {
		ed.Type(string(rune('a' + i%26)))
		if i%13 == 0 {
			ed.Type(" #abc ")
		}
		// Rewrapped on the edit itself: the line's rows are known before
		// any frame measures them.
		if st.vl.breaks[1] == nil {
			t.Fatalf("key %d: line 1's rows were dropped by the edit", i)
		}
	}
	ed.Frame()
	check("typed")
	st.Insert("\t你好世界😀 " + genWrapLine(r, 64<<10, ""))
	ed.Frame()
	check("pasted")
	h := st.Primary().Head
	st.SetSelections([]Selection{sel(h-20_000, h)})
	st.Insert("")
	ed.Frame()
	check("deleted")
	// An edit in the leading whitespace: every continuation row moves.
	st.MoveCaret(st.Snapshot().LineStart(1), false)
	st.Insert("\t")
	ed.Frame()
	check("indented")
}
