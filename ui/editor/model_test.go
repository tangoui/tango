package editor

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
)

func TestNewStatePicksLanguage(t *testing.T) {
	th := theme.Base("vs-dark")
	for _, c := range []struct {
		file, lang string
		hl         bool
	}{
		{"main.go", "go", true},
		{"src/App.TS", "typescript", true},
		{"notes.txt", "plaintext", false},
		{"data.unknownext", "plaintext", false},
		{"", "plaintext", false},
	} {
		st := NewState("x := 1\n", c.file, th)
		if st.Model.Language() != c.lang {
			t.Errorf("%q: Language = %q, want %q", c.file, st.Model.Language(), c.lang)
		}
		if (st.Highlighter() != nil) != c.hl {
			t.Errorf("%q: highlighter present = %v, want %v", c.file, st.Highlighter() != nil, c.hl)
		}
		if st.Theme != th {
			t.Errorf("%q: theme not kept", c.file)
		}
		if st.Model.Buffer().Snapshot().String() != "x := 1\n" {
			t.Errorf("%q: text %q", c.file, st.Model.Buffer().Snapshot().String())
		}
		if st.Primary() != (Selection{}) {
			t.Errorf("%q: initial selection %+v", c.file, st.Primary())
		}
	}
}

// walk moves the caret repeatedly and returns the offsets visited.
func walk(st *State, dir Direction, unit Unit, n int) []int {
	var out []int
	for range n {
		st.Move(dir, unit, false)
		out = append(out, st.Primary().Head)
	}
	return out
}

func TestMoveByGraphemeAndWord(t *testing.T) {
	// Each part is one grapheme cluster.
	parts := []string{
		"a",
		"👨‍👩‍👧", // ZWJ family
		"b",
		"é",       // combining acute
		"🇯🇵", "🇺🇸", // two regional-indicator pairs
		"日", "本",
		"☺️",  // variation selector
		"👍🏽",  // skin-tone modifier
		"각", // Hangul L V T jamo
		"क्ष", // Devanagari: KA + virama (Mn) + SSA... virama is Extend
		"x",
	}
	// U+0937 after a virama starts a new cluster in this subset (no GB9c);
	// split that part accordingly.
	parts = slices.Replace(parts, len(parts)-2, len(parts)-1, "क्", "ष")
	line := strings.Join(parts, "")
	var ends []int
	off := 0
	for _, p := range parts {
		off += len(p)
		ends = append(ends, off)
	}
	st := NewState(line+"\nnext", "a.txt", nil)
	if got := walk(st, Forward, Char, len(parts)+1); !slices.Equal(got, append(slices.Clone(ends), len(line)+1)) {
		t.Fatalf("forward by char:\n got  %v\n want %v (+ next line start)", got, ends)
	}
	// Back from the start of the next line: the line end, then each start.
	var want []int
	want = append(want, len(line))
	for i := len(ends) - 2; i >= 0; i-- {
		want = append(want, ends[i])
	}
	want = append(want, 0, 0)
	if got := walk(st, Backward, Char, len(want)); !slices.Equal(got, want) {
		t.Fatalf("backward by char:\n got  %v\n want %v", got, want)
	}

	// Words by the language's WordPattern (TypeScript): '.' separates,
	// '_' and CJK belong to words. At a line edge the move continues on the
	// adjacent line (Monaco).
	st = NewState("foo_bar baz.qux 日本語 x\nnext", "a.ts", nil)
	if got, want := walk(st, Forward, Word, 6), []int{7, 11, 15, 25, 27, 28 + 4}; !slices.Equal(got, want) {
		t.Fatalf("TS word right: got %v want %v", got, want)
	}
	if got, want := walk(st, Backward, Word, 7), []int{28, 26, 16, 12, 8, 0, 0}; !slices.Equal(got, want) {
		t.Fatalf("TS word left: got %v want %v", got, want)
	}
	// Without a WordPattern (Go, plain text): Unicode letters, digits,
	// marks and '_'.
	st = NewState("héllo wörld_2+é", "notes.txt", nil)
	if got, want := walk(st, Forward, Word, 3), []int{6, 15, 19}; !slices.Equal(got, want) {
		t.Fatalf("plain word right: got %v want %v", got, want)
	}

	// Extend by grapheme, then collapse to the selection's edge.
	st = NewState("ab👍🏽cd", "a.txt", nil)
	st.MoveCaret(2, false)
	st.Move(Forward, Char, true)
	if st.Primary() != (Selection{Anchor: 2, Head: 2 + len("👍🏽")}) {
		t.Fatalf("extend: %+v", st.Primary())
	}
	st.Move(Backward, Char, false)
	if st.Primary() != (Selection{Anchor: 2, Head: 2}) {
		t.Fatalf("collapse left: %+v", st.Primary())
	}
	// MoveCaret snaps into the document and onto a character boundary.
	st.MoveCaret(3, false)
	if st.Primary().Head != 2 {
		t.Fatalf("MoveCaret inside a character: %d", st.Primary().Head)
	}
	st.MoveCaret(-5, false)
	if st.Primary().Head != 0 {
		t.Fatalf("MoveCaret(-5): %d", st.Primary().Head)
	}

	// Vertical moves keep the goal column (CJK counts two columns).
	st = NewState("abcdef\nab\n日本語x", "a.txt", nil)
	st.MoveCaret(4, false)
	st.Move(Forward, Line, false)
	if st.Primary().Head != 7+2 {
		t.Fatalf("down to short line: %d", st.Primary().Head)
	}
	st.Move(Forward, Line, false)
	if want := 10 + len("日本"); st.Primary().Head != want {
		t.Fatalf("down keeps goal column: %d want %d", st.Primary().Head, want)
	}
	st.Move(Forward, Line, false)
	if st.Primary().Head != st.Model.Buffer().Snapshot().Len() {
		t.Fatalf("down on last line: %d", st.Primary().Head)
	}
	st.Move(Backward, Doc, false)
	st.Move(Backward, Line, false)
	if st.Primary().Head != 0 {
		t.Fatalf("up on first line: %d", st.Primary().Head)
	}

	// Line edges: Home toggles between the first non-blank and column 0.
	st = NewState("    foo", "a.txt", nil)
	st.MoveCaret(6, false)
	st.Move(Backward, LineEdge, false)
	if st.Primary().Head != 4 {
		t.Fatalf("home: %d", st.Primary().Head)
	}
	st.Move(Backward, LineEdge, false)
	if st.Primary().Head != 0 {
		t.Fatalf("home again: %d", st.Primary().Head)
	}
	st.Move(Forward, LineEdge, true)
	if st.Primary() != (Selection{Anchor: 0, Head: 7}) {
		t.Fatalf("shift+end: %+v", st.Primary())
	}

	// Page moves the caret and the viewport by a screenful.
	st = NewState(strings.Repeat("line\n", 100), "a.txt", nil)
	st.Viewport = Viewport{Height: 100, Width: 300, LineHeight: 10}
	st.Move(Forward, Page, false)
	if l := st.Model.Buffer().Snapshot().LineOf(st.Primary().Head); l != 10 {
		t.Fatalf("page down: line %d", l)
	}
	if st.Viewport.TopLine != 10 {
		t.Fatalf("page down scrolled to %d", st.Viewport.TopLine)
	}
	st.Move(Forward, Doc, false)
	if st.Primary().Head != st.Model.Buffer().Snapshot().Len() {
		t.Fatalf("doc end: %d", st.Primary().Head)
	}
}

func TestInsertDeleteUndo(t *testing.T) {
	st := NewState("hi", "a.txt", nil)
	clock := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	st.Model.Buffer().Now = func() time.Time { return clock }
	st.MoveCaret(2, false)
	for _, s := range []string{" ", "t", "h", "e", "r", "e"} {
		st.Insert(s)
		clock = clock.Add(50 * time.Millisecond)
	}
	if got := st.Model.Buffer().Snapshot().String(); got != "hi there" || st.Primary().Head != 8 {
		t.Fatalf("typing: %q caret %d", got, st.Primary().Head)
	}
	st.Insert("👨‍👩‍👧")
	st.Delete(Backward, Char) // the whole family goes
	if got := st.Model.Buffer().Snapshot().String(); got != "hi there" {
		t.Fatalf("backspace over ZWJ sequence: %q", got)
	}
	st.Delete(Backward, Word) // a step of its own (Monaco)
	if got := st.Model.Buffer().Snapshot().String(); got != "hi " || st.Primary().Head != 3 {
		t.Fatalf("delete word left: %q caret %d", got, st.Primary().Head)
	}
	// Replace a selection; CRLF is normalised and the caret follows it.
	st.MoveCaret(0, false)
	st.MoveCaret(2, true)
	st.Insert("a\r\nb")
	if got := st.Model.Buffer().Snapshot().String(); got != "a\nb " || st.Primary() != (Selection{Anchor: 3, Head: 3}) {
		t.Fatalf("replace selection: %q %+v", got, st.Primary())
	}
	st.Delete(Forward, Char)
	if got := st.Model.Buffer().Snapshot().String(); got != "a\nb" {
		t.Fatalf("delete forward: %q", got)
	}
	st.Delete(Forward, Char) // at the end: no-op
	if got := st.Model.Buffer().Snapshot().String(); got != "a\nb" {
		t.Fatalf("delete forward at end: %q", got)
	}

	// Undo walks back: delete-forward, replace, delete-word, emoji
	// insert/delete, typing.
	var texts []string
	for st.Undo() {
		texts = append(texts, st.Model.Buffer().Snapshot().String())
	}
	if texts[len(texts)-1] != "hi" {
		t.Fatalf("undo history %q does not end at the original text", texts)
	}
	if !slices.Contains(texts, "hi there") || !slices.Contains(texts, "hi ") {
		t.Fatalf("undo history %q", texts)
	}
	if st.Primary().Head != 2 {
		t.Fatalf("caret after full undo: %d", st.Primary().Head)
	}
	for st.Redo() {
	}
	if got := st.Model.Buffer().Snapshot().String(); got != "a\nb" {
		t.Fatalf("after redo: %q", got)
	}
}

// fullTokens tokenizes s from the start (the reference result).
func fullTokens(lx *monarch.Lexer, s buffer.Snapshot) [][]monarch.Token {
	out := make([][]monarch.Token, s.LineCount())
	state := lx.Initial()
	for i := range out {
		// A reference must never silently use a truncated, degraded result.
		// Retry from the SAME input state; a degraded end state is not trusted.
		for attempt := 0; ; attempt++ {
			toks, end, info := lx.TokenizeLineInfo(s.Line(i), state, nil)
			if !info.Degraded {
				out[i], state = toks, end
				break
			}
			if attempt == 3 {
				panic("fullTokens: reference repeatedly degraded")
			}
		}
	}
	return out
}

func checkHighlight(t *testing.T, st *State, ctx string) {
	t.Helper()
	h, s := st.Highlighter(), st.Model.Buffer().Snapshot()
	lx, err := DefaultRegistry().Lexer(st.Model.Language())
	if err != nil {
		t.Fatal(err)
	}
	ready := false
	for attempt := 0; attempt <= h.MaxRetries+1; attempt++ {
		if h.Highlight(s, s.LineCount(), time.Hour) {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatalf("%s: not ready after synchronous retries", ctx)
	}
	want := fullTokens(lx, s)
	for i := range want {
		got, final := h.Tokens(s, i, nil)
		if !final || !slices.Equal(got, want[i]) {
			t.Fatalf("%s: line %d final=%v\n got  %+v\n want %+v", ctx, i, final, got, want[i])
		}
	}
}

// Edits, undo and redo reach the Highlighter as edits (controller addition
// A): the lines above a multi-edit undo step stay final without any
// re-tokenizing, and the result equals a full re-tokenization.
func TestUndoRedoHighlightIncremental(t *testing.T) {
	var b strings.Builder
	for i := range 3000 {
		b.WriteString("const x" + strings.Repeat("y", i%7) + " = `a` + 1; // c\n")
	}
	st := NewState(b.String(), "big.ts", nil)
	// Drive retries synchronously, without depending on wall-clock delays or
	// background scheduling. Set tuning fields before the first highlight.
	st.Highlighter().RetryDelay = 0
	checkHighlight(t, st, "initial")
	s := st.Model.Buffer().Snapshot()
	st.Model.Buffer().PushUndoStop()
	off1 := s.LineStart(2900)
	if err := st.Edit([]buffer.Edit{{Start: off1, End: off1, Text: "/* open\n"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	s = st.Model.Buffer().Snapshot()
	off2 := s.LineStart(2990)
	if err := st.Edit([]buffer.Edit{{Start: off2, End: off2 + 5, Text: "*/ let"}, {Start: s.Len(), End: s.Len(), Text: "`"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	st.Model.Buffer().PushUndoStop()
	checkHighlight(t, st, "after edits")

	for _, op := range []struct {
		name string
		do   func() bool
	}{{"undo", st.Undo}, {"redo", st.Redo}, {"undo again", st.Undo}} {
		if !op.do() {
			t.Fatalf("%s failed", op.name)
		}
		s := st.Model.Buffer().Snapshot()
		for _, l := range []int{0, 1500, 2899} {
			if _, final := st.Highlighter().Tokens(s, l, nil); !final {
				t.Fatalf("%s: line %d not final: the highlighter resynchronised", op.name, l)
			}
		}
		checkHighlight(t, st, op.name)
	}
}

func TestScrollClamp(t *testing.T) {
	st := NewState(strings.Repeat("x\n", 99), "a.txt", nil) // 100 lines
	st.Viewport = Viewport{Height: 95, Width: 200, LineHeight: 10, ContentWidth: 500}
	st.ScrollBy(-50, 0)
	if v := st.Viewport; v.TopLine != 0 || v.OffsetY != 0 {
		t.Fatalf("scroll above top: %+v", v)
	}
	st.ScrollBy(25, 0)
	if v := st.Viewport; v.TopLine != 2 || v.OffsetY != 5 {
		t.Fatalf("scroll 25px: %+v", v)
	}
	if first, last := st.VisibleLines(); first != 2 || last != 11 {
		t.Fatalf("visible lines %d..%d, want 2..11", first, last)
	}
	st.ScrollBy(1e6, 0) // past the end: the last line at the top
	if v := st.Viewport; v.TopLine != 99 || v.OffsetY != 0 {
		t.Fatalf("scroll past end: %+v", v)
	}
	if first, last := st.VisibleLines(); first != 99 || last != 99 {
		t.Fatalf("visible at end %d..%d", first, last)
	}
	st.ScrollBy(0, 1000)
	if st.Viewport.ScrollX != 300 {
		t.Fatalf("ScrollX = %v, want 300", st.Viewport.ScrollX)
	}
	st.ScrollBy(0, -1000)
	if st.Viewport.ScrollX != 0 {
		t.Fatalf("ScrollX = %v, want 0", st.Viewport.ScrollX)
	}
	st.Viewport.ContentWidth = 0 // unknown width: only the left edge clamps
	st.ScrollBy(0, 1000)
	if st.Viewport.ScrollX != 1000 {
		t.Fatalf("ScrollX unknown width = %v", st.Viewport.ScrollX)
	}
	// The document shrinking clamps the viewport on the next query.
	st.MoveCaret(0, false)
	st.MoveCaret(st.Model.Buffer().Snapshot().Len(), true)
	st.Insert("only")
	if first, last := st.VisibleLines(); first != 0 || last != 0 || st.Viewport.TopLine != 0 {
		t.Fatalf("after shrink: %d..%d %+v", first, last, st.Viewport)
	}
}

func TestRevealCaret(t *testing.T) {
	st := NewState(strings.Repeat("x\n", 99), "a.txt", nil)
	st.Viewport = Viewport{Height: 95, Width: 200, LineHeight: 10}
	st.MoveCaret(st.Model.Buffer().Snapshot().LineStart(50), false)
	st.RevealCaret()
	if v := st.Viewport; v.TopLine != 41 || v.OffsetY != 5 {
		t.Fatalf("reveal below: %+v (want line 50 bottom at viewport bottom)", v)
	}
	before := st.Viewport
	st.MoveCaret(st.Model.Buffer().Snapshot().LineStart(45), false)
	st.RevealCaret()
	if st.Viewport != before {
		t.Fatalf("reveal of a visible caret scrolled: %+v", st.Viewport)
	}
	st.MoveCaret(st.Model.Buffer().Snapshot().LineStart(41), false) // partly hidden at the top
	st.RevealCaret()
	if v := st.Viewport; v.TopLine != 41 || v.OffsetY != 0 {
		t.Fatalf("reveal partly hidden top line: %+v", v)
	}
	st.MoveCaret(st.Model.Buffer().Snapshot().LineStart(3), false)
	st.RevealCaret()
	if v := st.Viewport; v.TopLine != 3 || v.OffsetY != 0 {
		t.Fatalf("reveal above: %+v", v)
	}
	// Horizontal reveal uses the caret x the View measured.
	st.RevealX(250, 252)
	if st.Viewport.ScrollX != 52 {
		t.Fatalf("RevealX right: ScrollX = %v", st.Viewport.ScrollX)
	}
	st.RevealX(10, 12)
	if st.Viewport.ScrollX != 10 {
		t.Fatalf("RevealX left: ScrollX = %v", st.Viewport.ScrollX)
	}
}

func TestEmptyDocumentState(t *testing.T) {
	for _, text := range []string{"", "\n"} {
		for _, file := range []string{"", "a.ts"} {
			st := NewState(text, file, nil)
			// No viewport size yet.
			if first, last := st.VisibleLines(); first != 0 || last != 0 {
				t.Fatalf("%q %q: visible %d..%d", text, file, first, last)
			}
			st.ScrollBy(100, 100)
			st.RevealCaret()
			for _, u := range []Unit{Char, Word, Line, LineEdge, Page, Doc} {
				for _, d := range []Direction{Backward, Forward} {
					st.Move(d, u, false)
					st.Delete(d, u)
				}
			}
			if got := st.Model.Buffer().Snapshot().String(); got != "" && got != text {
				t.Fatalf("%q %q: text %q after deletes", text, file, got)
			}
			st.Viewport = Viewport{Height: 100, Width: 100, LineHeight: 10}
			if first, last := st.VisibleLines(); first != 0 || last > 1 {
				t.Fatalf("%q %q: visible %d..%d", text, file, first, last)
			}
			st.Insert("a")
			if !strings.Contains(st.Model.Buffer().Snapshot().String(), "a") {
				t.Fatalf("%q %q: insert failed", text, file)
			}
			for st.Undo() {
			}
			if got := st.Model.Buffer().Snapshot().String(); got != text {
				t.Fatalf("%q %q: undo to %q", text, file, got)
			}
			if h := st.Highlighter(); h != nil {
				s := st.Model.Buffer().Snapshot()
				if !h.Highlight(s, s.LineCount(), time.Hour) {
					t.Fatalf("%q %q: not ready", text, file)
				}
			}
		}
	}
}

// Edge cases of the grapheme subset: odd regional-indicator runs, ZWJ not
// after a pictograph, marks at the start of a line, controls.
func TestGraphemeEdges(t *testing.T) {
	for _, c := range []struct {
		parts []string
	}{
		{[]string{"🇯🇵", "🇺🇸", "🇫", "x"}},
		{[]string{"a‍", "b"}},      // ZWJ extends 'a'; 'b' is not a pictograph
		{[]string{"́", "a"}},       // a lone mark is its own cluster
		{[]string{"a", "\t", "́"}}, // controls never join
		{[]string{"❤️‍🔥", "!"}},    // ZWJ sequence starting in the BMP
		{[]string{"ᄀ가", "각", "각ᆨ"}},
	} {
		s := strings.Join(c.parts, "")
		var got, want []int
		for i := 0; i < len(s); i = nextGrapheme(s, i) {
			got = append(got, i)
		}
		off := 0
		for _, p := range c.parts {
			want = append(want, off)
			off += len(p)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%+q: forward boundaries %v want %v", s, got, want)
		}
		var back []int
		for i := len(s); i > 0; {
			i = prevGrapheme(s, i)
			back = append(back, i)
		}
		slices.Reverse(back)
		if !slices.Equal(back, want) {
			t.Errorf("%+q: backward boundaries %v want %v", s, back, want)
		}
	}
}

// Typing a non-ASCII character continues the typing undo step (the kind
// check decodes the first rune, not the first byte).
func TestTypingNonASCIIMerges(t *testing.T) {
	st := NewState("", "a.txt", nil)
	for _, s := range []string{"日", "本", "é", "😀"} {
		st.Insert(s)
	}
	if !st.Undo() || st.Model.Buffer().Snapshot().String() != "" {
		t.Fatalf("one undo left %q, want the whole typing undone", st.Model.Buffer().Snapshot().String())
	}
}

// Two views of one Model (Monaco semantics): edits from one view move the
// other's selection, undo is shared and restores the selection in the view
// that runs it.
func TestTwoViewsShareModel(t *testing.T) {
	m := NewModel("file:///a.go", "zero\none\ntwo\nthree\n", nil)
	a, b := NewView(m, nil), NewView(m, nil)
	snap := m.Snapshot()
	if a.Highlighter() != b.Highlighter() || a.Snapshot().Version() != b.Snapshot().Version() {
		t.Fatal("views do not share the model")
	}

	// A inserts a line before B's caret: B's line moves down by one.
	b.MoveCaret(snap.LineStart(2)+1, false) // "t|wo"
	a.MoveCaret(snap.LineStart(1), false)
	a.Insert("new\n")
	snap = m.Snapshot()
	if p := snap.PosOf(b.Primary().Head); p.Line != 3 || p.Col != 1 || b.Primary().Anchor != b.Primary().Head {
		t.Fatalf("B after A's insert: %+v at %+v, want line 3 col 1", b.Primary(), p)
	}
	if got := snap.String(); got != "zero\nnew\none\ntwo\nthree\n" {
		t.Fatalf("text %q", got)
	}

	// An insert exactly at B's caret pushes it right.
	b.MoveCaret(snap.LineStart(3), false)
	a.MoveCaret(snap.LineStart(3), false)
	a.Insert("x")
	if want := m.Snapshot().LineStart(3) + 1; b.Primary() != (Selection{Anchor: want, Head: want}) {
		t.Fatalf("B after an insert at its caret: %+v, want %d", b.Primary(), want)
	}

	// A deletes a range containing B's selection: B collapses to its start.
	snap = m.Snapshot()
	b.MoveCaret(snap.LineStart(2)+1, false)
	b.MoveCaret(snap.LineStart(3)+2, true) // "o|ne\nxt|wo"
	del := Selection{Anchor: snap.LineStart(1), Head: snap.LineStart(4)}
	a.MoveCaret(del.Anchor, false)
	a.MoveCaret(del.Head, true)
	a.Delete(Backward, Char)
	if got := m.Snapshot().String(); got != "zero\nthree\n" {
		t.Fatalf("after delete %q", got)
	}
	if want := del.Anchor; b.Primary() != (Selection{Anchor: want, Head: want}) {
		t.Fatalf("B after A's delete: %+v, want collapsed at %d", b.Primary(), want)
	}

	// B undoes A's delete: the text comes back, B gets the selection the
	// undo step recorded (A's selection before the delete), and A's caret
	// is mapped like any other view's.
	if !b.Undo() {
		t.Fatal("B's undo failed")
	}
	if got := m.Snapshot().String(); got != "zero\nnew\none\nxtwo\nthree\n" {
		t.Fatalf("after B's undo %q", got)
	}
	if b.Primary() != del {
		t.Fatalf("B after undo: %+v, want %+v", b.Primary(), del)
	}
	if a.Primary() != (Selection{Anchor: del.Head, Head: del.Head}) {
		t.Fatalf("A after B's undo: %+v, want caret at %d (pushed by the re-insert)", a.Primary(), del.Head)
	}

	// Typing in one view after the other's edit starts a new undo step.
	a.MoveCaret(0, false)
	a.Insert("1")
	b.MoveCaret(m.Snapshot().Len(), false)
	b.Insert("2")
	a.Insert("3")
	if got := m.Snapshot().String(); got != "13zero\nnew\none\nxtwo\nthree\n2" {
		t.Fatalf("typing in both views: %q", got)
	}
	if !a.Undo() || m.Snapshot().String() != "1zero\nnew\none\nxtwo\nthree\n2" {
		t.Fatalf("undo merged across views: %q", m.Snapshot().String())
	}

	// A closed view no longer follows the model.
	b.Close()
	before := b.Primary()
	a.MoveCaret(0, false)
	a.Insert("abc\n")
	if b.Primary() != before {
		t.Fatalf("closed view moved: %+v -> %+v", before, b.Primary())
	}
}

// Another view's edits keep this view's first visible line on the same
// text.
func TestOtherViewEditKeepsTopLine(t *testing.T) {
	m := NewModel("a.txt", strings.Repeat("x\n", 99), nil)
	a, b := NewView(m, nil), NewView(m, nil)
	b.Viewport = Viewport{TopLine: 50, Height: 95, Width: 200, LineHeight: 10}
	a.Insert("1\n2\n3\n")
	if b.Viewport.TopLine != 53 {
		t.Fatalf("TopLine %d, want 53", b.Viewport.TopLine)
	}
}

func TestViewStateRoundTrip(t *testing.T) {
	m := NewModel("a.txt", strings.Repeat("line\n", 99), nil)
	st := NewView(m, nil)
	st.Viewport = Viewport{Height: 95, Width: 200, LineHeight: 10, ContentWidth: 500}
	st.ScrollBy(123, 40)
	st.MoveCaret(m.Snapshot().LineStart(20)+1, false)
	st.MoveCaret(m.Snapshot().LineStart(22)+3, true)
	v := st.SaveViewState() // folded regions: TestFoldViewState
	if v.TopLine != 12 || v.OffsetY != 3 || v.ScrollX != 40 || len(v.Selections) != 1 || v.Selections[0] != st.Primary() {
		t.Fatalf("saved %+v (sel %+v)", v, st.Primary())
	}

	// The same view after showing something else, and a new view.
	st.MoveCaret(0, false)
	st.ScrollBy(-1e6, -1e6)
	other := NewView(m, nil)
	other.Viewport = Viewport{Height: 95, Width: 200, LineHeight: 10, ContentWidth: 500}
	for _, s := range []*State{st, other} {
		s.RestoreViewState(v)
		if got := s.SaveViewState(); got.TopLine != v.TopLine || got.OffsetY != v.OffsetY || got.ScrollX != v.ScrollX ||
			!slices.Equal(got.Selections, v.Selections) || !slices.Equal(got.Folded, v.Folded) {
			t.Fatalf("restored %+v, want %+v", got, v)
		}
	}
}

func TestRestoreViewStateClamps(t *testing.T) {
	m := NewModel("a.txt", strings.Repeat("ab\n", 9), nil) // 10 lines, 27 bytes
	st := NewView(m, nil)
	st.Viewport = Viewport{Height: 50, Width: 200, LineHeight: 10, ContentWidth: 300}
	st.RestoreViewState(ViewState{
		TopLine:    1000,
		OffsetY:    7,
		ScrollX:    1e6,
		Selections: []Selection{{Anchor: -5, Head: 1e9}, {Anchor: 1, Head: 1}},
		Folded:     []int{-1, 4, 4, 2, 10, 1e6},
	})
	if v := st.Viewport; v.TopLine != 9 || v.OffsetY != 0 || v.ScrollX != 100 {
		t.Fatalf("viewport %+v", v)
	}
	if st.Primary() != (Selection{Anchor: 0, Head: 27}) {
		t.Fatalf("selection %+v", st.Primary())
	}
	if f := st.Folded(); len(f) != 0 { // no folding range starts on those lines
		t.Fatalf("folded %v", f)
	}

	st.RestoreViewState(ViewState{TopLine: -3, OffsetY: 25, ScrollX: -4})
	if v := st.Viewport; v.TopLine != 0 || v.OffsetY != 0 || v.ScrollX != 0 || st.Primary() != (Selection{}) {
		t.Fatalf("negative values: %+v sel %+v", v, st.Primary())
	}
	st.RestoreViewState(ViewState{TopLine: 2, OffsetY: 25})
	if v := st.Viewport; v.TopLine != 2 || v.OffsetY != 0 {
		t.Fatalf("OffsetY past the line height: %+v", v)
	}
}

func TestSetEOLUndo(t *testing.T) {
	m := NewModel("a.txt", "a\nb", nil)
	st := NewView(m, nil)
	var changes []ModelChange
	cancel := m.Subscribe(func(ch ModelChange) { changes = append(changes, ch) })
	defer cancel()

	m.SetEOL("x")
	m.SetEOL("\n")
	if len(changes) != 0 || m.EOL() != "\n" {
		t.Fatalf("invalid or unchanged EOL changed something: %d changes, EOL %q", len(changes), m.EOL())
	}
	m.SetEOL("\r\n")
	if m.EOL() != "\r\n" || m.Snapshot().TextWithEOL() != "a\r\nb" || m.Snapshot().String() != "a\nb" {
		t.Fatalf("EOL %q, text %q", m.EOL(), m.Snapshot().TextWithEOL())
	}
	if len(changes) != 1 || !changes[0].EOL || len(changes[0].Edits) != 0 || changes[0].Source != nil {
		t.Fatalf("changes %+v", changes)
	}

	// A text edit after it is a step of its own.
	st.MoveCaret(3, false)
	st.Insert("c")
	if !st.Undo() || m.Snapshot().String() != "a\nb" || m.EOL() != "\r\n" {
		t.Fatalf("undo of the typing: %q EOL %q", m.Snapshot().String(), m.EOL())
	}
	if !st.Undo() || m.EOL() != "\n" || m.Snapshot().TextWithEOL() != "a\nb" {
		t.Fatalf("undo of SetEOL: EOL %q", m.EOL())
	}
	if last := changes[len(changes)-1]; !last.Undo || !last.EOL || last.Source != st {
		t.Fatalf("undo change %+v", last)
	}
	if !st.Redo() || m.EOL() != "\r\n" {
		t.Fatalf("redo of SetEOL: EOL %q", m.EOL())
	}
	if st.Undo(); st.Undo() {
		t.Fatal("history has more than the EOL step")
	}
}

// TestCloseDropsHighlightWindow: closing a view frees its token window in
// the shared Highlighter; the other view's window stays.
func TestCloseDropsHighlightWindow(t *testing.T) {
	m := NewModel("a.go", "package main\n\nfunc main() {}\n", DefaultRegistry())
	a, b := NewView(m, nil), NewView(m, nil)
	hl := m.Highlighter()
	if hl == nil {
		t.Fatal("no highlighter for Go")
	}
	snap := m.Snapshot()
	hl.HighlightFor(a.hlKey, snap, 3, time.Second)
	hl.HighlightFor(b.hlKey, snap, 3, time.Second)
	if _, hi := hl.WindowRangeFor(a.hlKey); hi == 0 {
		t.Fatal("view a has no window")
	}
	a.Close()
	if lo, hi := hl.WindowRangeFor(a.hlKey); lo != 0 || hi != 0 {
		t.Errorf("closed view keeps its window [%d, %d)", lo, hi)
	}
	if _, hi := hl.WindowRangeFor(b.hlKey); hi == 0 {
		t.Error("the other view lost its window")
	}
	a.Close() // twice is safe
}
