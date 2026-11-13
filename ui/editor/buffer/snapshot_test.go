package buffer

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func mustApply(t *testing.T, s Snapshot, edits ...Edit) Snapshot {
	t.Helper()
	ns, err := s.Apply(edits)
	if err != nil {
		t.Fatalf("Apply(%v): %v", edits, err)
	}
	checkInvariants(t, ns)
	return ns
}

func TestApplyBasic(t *testing.T) {
	s := NewSnapshot("hello world")
	if s.Version() != 1 {
		t.Fatalf("fresh Version() = %d, want 1", s.Version())
	}
	s2 := mustApply(t, s, Edit{Start: 6, End: 11, Text: "tango"})
	if got := s2.String(); got != "hello tango" {
		t.Fatalf("replace: got %q", got)
	}
	if s2.Version() != 2 {
		t.Fatalf("Version() after Apply = %d, want 2", s2.Version())
	}
	// The original snapshot is immutable.
	if got := s.String(); got != "hello world" {
		t.Fatalf("original changed: %q", got)
	}
	s3 := mustApply(t, s2, Edit{Start: 0, End: 0, Text: "> "})
	if got := s3.String(); got != "> hello tango" {
		t.Fatalf("insert: got %q", got)
	}
	s4 := mustApply(t, s3, Edit{Start: 0, End: s3.Len(), Text: ""})
	if s4.String() != "" || s4.Len() != 0 || s4.LineCount() != 1 {
		t.Fatalf("delete all: %q len=%d lines=%d", s4.String(), s4.Len(), s4.LineCount())
	}
	s5 := mustApply(t, s4, Edit{Start: 0, End: 0, Text: "x\ny"})
	if s5.String() != "x\ny" || s5.LineCount() != 2 || s5.Version() != 5 {
		t.Fatalf("insert into empty: %q lines=%d v=%d", s5.String(), s5.LineCount(), s5.Version())
	}
}

func TestApplyMulti(t *testing.T) {
	s := NewSnapshot("one two three four")
	s2 := mustApply(t, s,
		Edit{Start: 0, End: 3, Text: "1"},
		Edit{Start: 4, End: 7, Text: "TWO"},
		Edit{Start: 14, End: 18, Text: "4!"},
	)
	if got := s2.String(); got != "1 TWO three 4!" {
		t.Fatalf("multi: got %q", got)
	}
	if s2.Version() != s.Version()+1 {
		t.Fatalf("multi Apply bumps version once: %d", s2.Version())
	}
	// Two inserts at the same offset keep their order; adjacent edits are fine.
	s3 := mustApply(t, NewSnapshot("ab"),
		Edit{Start: 1, End: 1, Text: "X"},
		Edit{Start: 1, End: 1, Text: "Y"},
		Edit{Start: 1, End: 2, Text: "Z"},
	)
	if got := s3.String(); got != "aXYZ" {
		t.Fatalf("same-offset inserts: got %q", got)
	}

	bad := [][]Edit{
		{{Start: 2, End: 1}},                     // start > end
		{{Start: -1, End: 0}},                    // negative
		{{Start: 0, End: 99}},                    // past end
		{{Start: 3, End: 5}, {Start: 0, End: 1}}, // unsorted
		{{Start: 0, End: 5}, {Start: 4, End: 6}}, // overlapping
	}
	for _, edits := range bad {
		if _, err := s.Apply(edits); err == nil {
			t.Errorf("Apply(%v) succeeded, want error", edits)
		}
	}
}

func TestLines(t *testing.T) {
	empty := NewSnapshot("")
	if empty.LineCount() != 1 || empty.Line(0) != "" || empty.Len() != 0 {
		t.Fatalf("empty: lines=%d line0=%q", empty.LineCount(), empty.Line(0))
	}
	if empty.LineStart(0) != 0 || empty.LineOf(0) != 0 {
		t.Fatalf("empty LineStart/LineOf")
	}
	var zero Snapshot
	if zero.LineCount() != 1 || zero.String() != "" || zero.EOL() != "\n" {
		t.Fatalf("zero Snapshot not an empty document")
	}

	s := NewSnapshot("a\nb\n")
	if s.LineCount() != 3 {
		t.Fatalf("LineCount = %d, want 3", s.LineCount())
	}
	wantLines := []string{"a", "b", ""}
	wantStarts := []int{0, 2, 4}
	for i := range wantLines {
		if got := s.Line(i); got != wantLines[i] {
			t.Errorf("Line(%d) = %q, want %q", i, got, wantLines[i])
		}
		if got := s.LineStart(i); got != wantStarts[i] {
			t.Errorf("LineStart(%d) = %d, want %d", i, got, wantStarts[i])
		}
	}
	wantLineOf := []int{0, 0, 1, 1, 2}
	for off, want := range wantLineOf {
		if got := s.LineOf(off); got != want {
			t.Errorf("LineOf(%d) = %d, want %d", off, got, want)
		}
	}
	if p := s.PosOf(3); p != (Pos{Line: 1, Col: 1}) {
		t.Errorf("PosOf(3) = %+v", p)
	}
	if off := s.OffsetOf(Pos{Line: 1, Col: 1}); off != 3 {
		t.Errorf("OffsetOf = %d", off)
	}
	// OffsetOf clamps out-of-range columns/lines; PosOf clamps offsets.
	if off := s.OffsetOf(Pos{Line: 0, Col: 50}); off != 1 {
		t.Errorf("OffsetOf clamp col = %d, want 1", off)
	}
	if off := s.OffsetOf(Pos{Line: 9, Col: 0}); off != 4 {
		t.Errorf("OffsetOf clamp line = %d, want 4", off)
	}
	if p := s.PosOf(99); p != (Pos{Line: 2, Col: 0}) {
		t.Errorf("PosOf clamp = %+v", p)
	}
	if got := s.Slice(1, 4); got != "\nb\n" {
		t.Errorf("Slice = %q", got)
	}
}

func TestEOLRoundTrip(t *testing.T) {
	src := "a\r\nb\r\n"
	s := NewSnapshot(src)
	if s.EOL() != "\r\n" {
		t.Fatalf("EOL = %q, want CRLF", s.EOL())
	}
	if s.String() != "a\nb\n" || s.Len() != 4 {
		t.Fatalf("internal text = %q (len %d), want LF-normalised", s.String(), s.Len())
	}
	if s.LineCount() != 3 || s.Line(0) != "a" || s.Line(1) != "b" || s.Line(2) != "" {
		t.Fatalf("CRLF lines: count=%d %q %q %q", s.LineCount(), s.Line(0), s.Line(1), s.Line(2))
	}
	if got := s.TextWithEOL(); got != src {
		t.Fatalf("TextWithEOL = %q, want %q", got, src)
	}
	// EOL survives edits.
	s2 := mustApply(t, s, Edit{Start: 4, End: 4, Text: "c\n"})
	if s2.EOL() != "\r\n" || s2.TextWithEOL() != "a\r\nb\r\nc\r\n" {
		t.Fatalf("after edit: eol=%q text=%q", s2.EOL(), s2.TextWithEOL())
	}

	mixed := NewSnapshot("a\r\nb\nc\r\n")
	if mixed.EOL() != "\r\n" {
		t.Fatalf("mixed EOL = %q, want CRLF", mixed.EOL())
	}
	if mixed.LineCount() != 4 || mixed.Line(0) != "a" || mixed.Line(1) != "b" || mixed.Line(2) != "c" || mixed.Line(3) != "" {
		t.Fatalf("mixed lines wrong: %q", mixed.String())
	}
	if got := mixed.TextWithEOL(); got != "a\r\nb\r\nc\r\n" {
		t.Fatalf("mixed TextWithEOL = %q", got)
	}

	for _, tc := range []struct{ src, eol string }{
		{"a\nb\n", "\n"},
		{"no newline", "\n"},
		{"", "\n"},
		{"a\r\nb\n", "\n"}, // tie: not a CRLF majority
		{"a\r\nb\r\nc\n", "\r\n"},
	} {
		s := NewSnapshot(tc.src)
		if s.EOL() != tc.eol {
			t.Errorf("NewSnapshot(%q).EOL() = %q, want %q", tc.src, s.EOL(), tc.eol)
		}
	}
	// LF files round-trip byte-for-byte.
	lf := "x\ny\n\nz"
	if got := NewSnapshot(lf).TextWithEOL(); got != lf {
		t.Fatalf("LF round trip = %q", got)
	}
	// A large CRLF file spanning many leaves round-trips too.
	big := strings.Repeat("line 中文 😀 text\r\n", 5000)
	bs := NewSnapshot(big)
	if bs.EOL() != "\r\n" || bs.LineCount() != 5001 || bs.TextWithEOL() != big {
		t.Fatalf("big CRLF round trip failed: eol=%q lines=%d", bs.EOL(), bs.LineCount())
	}
}

func TestApplyNormalizesCRLF(t *testing.T) {
	s := NewSnapshot("x\r\ny\r\n")
	if s.EOL() != "\r\n" || s.LineCount() != 3 {
		t.Fatalf("setup: eol=%q lines=%d", s.EOL(), s.LineCount())
	}
	s2 := mustApply(t, s, Edit{Start: 2, End: 2, Text: "a\r\nb"})
	if s2.LineCount() != s.LineCount()+1 {
		t.Fatalf("LineCount = %d, want %d", s2.LineCount(), s.LineCount()+1)
	}
	if strings.ContainsRune(s2.String(), '\r') {
		t.Fatalf("String() contains CR: %q", s2.String())
	}
	if s2.String() != "x\na\nby\n" || s2.Len() != 7 {
		t.Fatalf("String() = %q (len %d)", s2.String(), s2.Len())
	}
	if got := s2.TextWithEOL(); got != "x\r\na\r\nby\r\n" {
		t.Fatalf("TextWithEOL = %q", got)
	}
	// Lone CRs in inserted text are line breaks too; later edits in the same
	// batch still use offsets of the original snapshot.
	s3 := mustApply(t, NewSnapshot("abc"),
		Edit{Start: 1, End: 1, Text: "1\r2\r\n3"},
		Edit{Start: 2, End: 3, Text: "Z\r"},
	)
	if got := s3.String(); got != "a1\n2\n3bZ\n" || s3.LineCount() != 4 {
		t.Fatalf("batch normalisation: %q lines=%d", got, s3.LineCount())
	}
}

func TestLoneCR(t *testing.T) {
	s := NewSnapshot("a\rb")
	if s.LineCount() != 2 || s.Line(0) != "a" || s.Line(1) != "b" {
		t.Fatalf("lone CR: lines=%d %q", s.LineCount(), s.String())
	}
	if s.EOL() != "\n" || s.String() != "a\nb" {
		t.Fatalf("lone CR: eol=%q text=%q", s.EOL(), s.String())
	}
	// Lone CRs vote with LF in EOL detection.
	for _, tc := range []struct {
		src, eol string
		lines    int
	}{
		{"a\r\nb\rc", "\n", 3},        // 1 CRLF vs 1 lone CR: tie
		{"a\r\nb\r\nc\rd", "\r\n", 4}, // 2 CRLF vs 1 lone CR
		{"a\r\rb", "\n", 3},           // CR CR is two breaks
		{"a\r", "\n", 2},
		{"\r\n\r", "\n", 3}, // CRLF then lone CR: tie
	} {
		s := NewSnapshot(tc.src)
		if s.EOL() != tc.eol || s.LineCount() != tc.lines || strings.ContainsRune(s.String(), '\r') {
			t.Errorf("NewSnapshot(%q): eol=%q lines=%d text=%q, want eol=%q lines=%d",
				tc.src, s.EOL(), s.LineCount(), s.String(), tc.eol, tc.lines)
		}
	}
}

func TestUTF16Columns(t *testing.T) {
	s := NewSnapshot("first\na中😀b\n")
	// a=1 byte, 中=3 bytes (1 unit), 😀=4 bytes (2 units, surrogate pair), b=1 byte.
	byteCols := []int{0, 1, 4, 8, 9}
	utf16Cols := []int{0, 1, 2, 4, 5}
	for i := range byteCols {
		if got := s.UTF16Col(1, byteCols[i]); got != utf16Cols[i] {
			t.Errorf("UTF16Col(1, %d) = %d, want %d", byteCols[i], got, utf16Cols[i])
		}
		if got := s.ByteCol(1, utf16Cols[i]); got != byteCols[i] {
			t.Errorf("ByteCol(1, %d) = %d, want %d", utf16Cols[i], got, byteCols[i])
		}
	}
	if got := s.UTF16Col(1, 8); got != 4 {
		t.Fatalf("byte col 8 -> utf16 %d, want 4", got)
	}
	if got := s.ByteCol(1, 4); got != 8 {
		t.Fatalf("utf16 col 4 -> byte %d, want 8", got)
	}
	// Mid-character inputs snap back to the character start.
	if got := s.ByteCol(1, 3); got != 4 {
		t.Errorf("ByteCol inside surrogate pair = %d, want 4", got)
	}
	if got := s.UTF16Col(1, 2); got != 1 {
		t.Errorf("UTF16Col inside 中 = %d, want 1", got)
	}
	// Columns past the end clamp to the line length.
	if got := s.UTF16Col(1, 100); got != 5 {
		t.Errorf("UTF16Col clamp = %d", got)
	}
	if got := s.ByteCol(1, 100); got != 9 {
		t.Errorf("ByteCol clamp = %d", got)
	}
	if got := s.UTF16Col(0, 5); got != 5 {
		t.Errorf("ASCII UTF16Col = %d", got)
	}
}

func TestEditRejectsSplitRune(t *testing.T) {
	s := NewSnapshot("a中😀b") // 中 at [1,4), 😀 at [4,8)
	for _, e := range []Edit{
		{Start: 2, End: 2, Text: "x"}, // inside 中
		{Start: 3, End: 4},            // start inside 中
		{Start: 4, End: 6},            // end inside 😀
		{Start: 5, End: 9},            // start inside 😀
		{Start: 0, End: 7},            // end inside 😀
	} {
		ns, err := s.Apply([]Edit{e})
		if !errors.Is(err, ErrSplitRune) {
			t.Errorf("Apply(%+v) err = %v, want ErrSplitRune", e, err)
		}
		if ns.String() != s.String() || ns.Version() != s.Version() {
			t.Errorf("failed Apply must return the unchanged snapshot")
		}
	}
	// One bad edit in a batch rejects the whole batch.
	if _, err := s.Apply([]Edit{{Start: 0, End: 1, Text: "A"}, {Start: 6, End: 6}}); !errors.Is(err, ErrSplitRune) {
		t.Errorf("batch err = %v, want ErrSplitRune", err)
	}
	for _, e := range []Edit{{Start: 1, End: 4}, {Start: 4, End: 8, Text: "🎉"}, {Start: 8, End: 9}, {Start: 0, End: 9}} {
		if _, err := s.Apply([]Edit{e}); err != nil {
			t.Errorf("Apply(%+v) on rune boundaries: %v", e, err)
		}
	}
	// Invalid UTF-8 in the document must not make offsets unusable.
	inv := NewSnapshot("a\x80\x80b")
	if _, err := inv.Apply([]Edit{{Start: 2, End: 3}}); err != nil {
		t.Errorf("edit inside invalid UTF-8 rejected: %v", err)
	}
}

// --- random differential test ---

var randPieces = []string{"a", "bc", "\n", "中", "😀", "hello ", "\n\n", "é", "xyz\nuvw", "世界\n", "\r\n", "\r", "p\rq"}

// modelEOL is the reference line-break normalisation: CRLF and lone CR become LF.
var modelEOL = strings.NewReplacer("\r\n", "\n", "\r", "\n")

func randText(r *rand.Rand) string {
	var b strings.Builder
	switch r.IntN(10) {
	case 0:
		return ""
	case 1: // large insert, forces multi-leaf chunks
		n := 100 + r.IntN(1500)
		for range n {
			b.WriteString(randPieces[r.IntN(len(randPieces))])
		}
	default:
		n := r.IntN(6)
		for range n {
			b.WriteString(randPieces[r.IntN(len(randPieces))])
		}
	}
	return b.String()
}

// runeFloor moves off back to the start of the rune containing it.
func runeFloor(s string, off int) int {
	for off > 0 && off < len(s) && !utf8.RuneStart(s[off]) {
		off--
	}
	return off
}

func TestRandomEditsMatchString(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var init strings.Builder
	for init.Len() < 40_000 {
		init.WriteString(randPieces[r.IntN(len(randPieces))])
	}
	s := NewSnapshot(init.String())
	model := modelEOL.Replace(init.String())
	checkInvariants(t, s)

	for step := range 10_000 {
		var edits []Edit
		n := 1
		if r.IntN(5) == 0 {
			n = 2 + r.IntN(4)
		}
		// Pick sorted rune-aligned offsets for n non-overlapping edits.
		offs := make([]int, 2*n)
		for i := range offs {
			offs[i] = runeFloor(model, r.IntN(len(model)+1))
		}
		sortInts(offs)
		for i := range n {
			start, end := offs[2*i], offs[2*i+1]
			if len(model) > 60_000 || r.IntN(3) != 0 {
				// keep many edits small so the document size stays bounded
				if end-start > 64 {
					end = runeFloor(model, start+r.IntN(64))
				}
			}
			edits = append(edits, Edit{Start: start, End: end, Text: randText(r)})
		}
		if len(model) > 80_000 {
			edits = []Edit{{Start: 0, End: runeFloor(model, len(model)/2), Text: ""}}
		}

		ns, err := s.Apply(edits)
		if err != nil {
			t.Fatalf("step %d: Apply(%v): %v", step, edits, err)
		}
		for i := len(edits) - 1; i >= 0; i-- {
			e := edits[i]
			model = model[:e.Start] + modelEOL.Replace(e.Text) + model[e.End:]
		}
		s = ns

		if s.Len() != len(model) {
			t.Fatalf("step %d: Len %d want %d", step, s.Len(), len(model))
		}
		wantLines := strings.Count(model, "\n") + 1
		if s.LineCount() != wantLines {
			t.Fatalf("step %d: LineCount %d want %d", step, s.LineCount(), wantLines)
		}
		// Spot-check line/offset queries every step and full content periodically.
		li := r.IntN(wantLines)
		ls := lineStartModel(model, li)
		if got := s.LineStart(li); got != ls {
			t.Fatalf("step %d: LineStart(%d)=%d want %d", step, li, got, ls)
		}
		le := strings.IndexByte(model[ls:], '\n')
		if le < 0 {
			le = len(model)
		} else {
			le += ls
		}
		if got := s.Line(li); got != model[ls:le] {
			t.Fatalf("step %d: Line(%d)=%q want %q", step, li, got, model[ls:le])
		}
		lineText := model[ls:le]
		bc := runeFloor(lineText, r.IntN(len(lineText)+1))
		uc := len(utf16.Encode([]rune(lineText[:bc])))
		if got := s.UTF16Col(li, bc); got != uc {
			t.Fatalf("step %d: UTF16Col(%d,%d)=%d want %d", step, li, bc, got, uc)
		}
		if got := s.ByteCol(li, uc); got != bc {
			t.Fatalf("step %d: ByteCol(%d,%d)=%d want %d", step, li, uc, got, bc)
		}
		off := r.IntN(len(model) + 1)
		if got, want := s.LineOf(off), strings.Count(model[:off], "\n"); got != want {
			t.Fatalf("step %d: LineOf(%d)=%d want %d", step, off, got, want)
		}
		// OffsetOf snaps a mid-character position back to the character start.
		p := s.PosOf(off)
		if got, want := s.OffsetOf(p), runeFloor(model, off); got != want {
			t.Fatalf("step %d: OffsetOf(PosOf(%d)) = %d, want %d", step, off, got, want)
		}
		a, b := r.IntN(len(model)+1), r.IntN(len(model)+1)
		if a > b {
			a, b = b, a
		}
		if got := s.Slice(a, b); got != model[a:b] {
			t.Fatalf("step %d: Slice(%d,%d) mismatch", step, a, b)
		}
		if step%250 == 0 || step == 9_999 {
			if s.String() != model {
				t.Fatalf("step %d: String() mismatch", step)
			}
			checkInvariants(t, s)
		}
	}
}

func lineStartModel(s string, line int) int {
	off := 0
	for range line {
		off += strings.IndexByte(s[off:], '\n') + 1
	}
	return off
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// checkInvariants verifies the rope's structural invariants: AVL balance,
// cached byte/newline counts, leaf size limits and rune-aligned leaves.
func checkInvariants(t *testing.T, s Snapshot) {
	t.Helper()
	if s.root == nil {
		return
	}
	leaves := 0
	valid := utf8.ValidString(s.String())
	var walk func(n *node) (height, bytes, lines int)
	walk = func(n *node) (int, int, int) {
		if n.left == nil && n.right == nil {
			leaves++
			if len(n.text) == 0 || len(n.text) > leafMax {
				t.Fatalf("leaf size %d out of range", len(n.text))
			}
			if valid && !utf8.RuneStart(n.text[0]) {
				t.Fatalf("leaf starts mid-rune")
			}
			if n.height != 0 || n.bytes != len(n.text) || n.lines != strings.Count(n.text, "\n") {
				t.Fatalf("bad leaf cache")
			}
			return 0, n.bytes, n.lines
		}
		if n.left == nil || n.right == nil {
			t.Fatalf("internal node with one child")
		}
		lh, lb, ll := walk(n.left)
		rh, rb, rl := walk(n.right)
		if lh-rh > 1 || rh-lh > 1 {
			t.Fatalf("unbalanced: %d vs %d", lh, rh)
		}
		h := max(lh, rh) + 1
		if int(n.height) != h || n.bytes != lb+rb || n.lines != ll+rl {
			t.Fatalf("bad internal cache")
		}
		return h, lb + rb, ll + rl
	}
	walk(s.root)
	if leaves > 1 {
		// No undersized leaves once the document spans several leaves.
		var minLeaf func(n *node) int
		minLeaf = func(n *node) int {
			if n.isLeaf() {
				return len(n.text)
			}
			return min(minLeaf(n.left), minLeaf(n.right))
		}
		if m := minLeaf(s.root); m < leafMin {
			t.Fatalf("fragmented: smallest of %d leaves is %d bytes", leaves, m)
		}
	}
}
