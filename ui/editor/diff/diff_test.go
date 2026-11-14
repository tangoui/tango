package diff

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

var bg = context.Background()

// checkResult verifies the invariants every result must satisfy, whatever
// alignment was chosen: changes are ordered and separated by unchanged
// lines that really are equal, and (when whitespace counts) replaying the
// character mappings on the original text yields the modified text.
func checkResult(t *testing.T, orig, mod []string, opts Options, res Result) {
	t.Helper()
	if len(orig) == 0 {
		orig = []string{""}
	}
	if len(mod) == 0 {
		mod = []string{""}
	}
	if res.OriginalLines != len(orig) || res.ModifiedLines != len(mod) {
		t.Fatalf("line counts %d/%d, want %d/%d", res.OriginalLines, res.ModifiedLines, len(orig), len(mod))
	}
	if res.Identical != slices.Equal(orig, mod) {
		t.Fatalf("Identical = %v", res.Identical)
	}
	eq := func(a, b string) bool { return a == b }
	if opts.IgnoreTrimWhitespace {
		eq = trimEqual
	}
	o, m := 0, 0
	for i, c := range res.Changes {
		if c.Original.Start < o || c.Modified.Start < m || c.Original.End < c.Original.Start || c.Modified.End < c.Modified.Start {
			t.Fatalf("change %d %v out of order", i, c)
		}
		if c.Original.Start-o != c.Modified.Start-m {
			t.Fatalf("change %d %v: unequal unchanged gap before it", i, c)
		}
		if i > 0 && c.Original.Start == o {
			t.Fatalf("change %d %v touches the previous change", i, c)
		}
		for k := 0; k < c.Original.Start-o; k++ {
			if !eq(orig[o+k], mod[m+k]) {
				t.Fatalf("unchanged lines %d/%d differ: %q vs %q", o+k, m+k, orig[o+k], mod[m+k])
			}
		}
		if len(c.Inner) == 0 {
			t.Fatalf("change %d has no inner changes", i)
		}
		o, m = c.Original.End, c.Modified.End
	}
	if len(orig)-o != len(mod)-m || o > len(orig) {
		t.Fatalf("unequal unchanged tail: %d/%d", len(orig)-o, len(mod)-m)
	}
	for k := 0; k < len(orig)-o; k++ {
		if !eq(orig[o+k], mod[m+k]) {
			t.Fatalf("unchanged tail lines %d/%d differ: %q vs %q", o+k, m+k, orig[o+k], mod[m+k])
		}
	}
	if !opts.IgnoreTrimWhitespace {
		if got, want := applyInner(orig, mod, res.Changes), strings.Join(mod, "\n"); got != want {
			t.Fatalf("replaying inner changes gives\n%q\nwant\n%q", got, want)
		}
	}
	for _, mv := range res.Moves {
		if mv.Original.Empty() || mv.Modified.Empty() || mv.Original.End > len(orig) || mv.Modified.End > len(mod) {
			t.Fatalf("bad move %v -> %v", mv.Original, mv.Modified)
		}
	}
}

// applyInner replaces every inner original range by its modified text.
func applyInner(orig, mod []string, changes []Change) string {
	origText, modText := strings.Join(orig, "\n"), strings.Join(mod, "\n")
	offset := func(lines []string, p buffer.Pos) int {
		n := 0
		for i := 0; i < p.Line; i++ {
			n += len(lines[i]) + 1
		}
		return n + p.Col
	}
	var b strings.Builder
	last := 0
	for _, c := range changes {
		for _, in := range c.Inner {
			s, e := offset(orig, in.Original.Start), offset(orig, in.Original.End)
			b.WriteString(origText[last:s])
			b.WriteString(modText[offset(mod, in.Modified.Start):offset(mod, in.Modified.End)])
			last = e
		}
	}
	b.WriteString(origText[last:])
	return b.String()
}

func lines(s string) []string { return strings.Split(s, "\n") }

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		orig     string
		mod      string
		opts     Options
		want     string // changes in formatChanges form, joined by " "
		noChange bool
	}{
		{name: "identical", orig: "a\nb", mod: "a\nb", noChange: true},
		{name: "both empty", orig: "", mod: "", noChange: true},
		{name: "empty original", orig: "", mod: "a\nb", want: "[0,1,0,2,[[0,0,0,0,0,0,1,1]]]"},
		{name: "empty modified", orig: "a\nb", mod: "", want: "[0,2,0,1,[[0,0,1,1,0,0,0,0]]]"},
		{name: "all changed", orig: "one\ntwo", mod: "alpha\nbeta\ngamma", want: "[0,2,0,3,[[0,0,1,3,0,0,2,5]]]"},
		// Adding a final newline adds an empty last line: an insertion
		// after line 1 whose text is the line break.
		{name: "add final newline", orig: "a\nb", mod: "a\nb\n", want: "[2,2,2,3,[[1,1,1,1,1,1,2,0]]]"},
		{name: "remove final newline", orig: "a\nb\n", mod: "a\nb", want: "[2,3,2,2,[[1,1,2,0,1,1,1,1]]]"},
		{name: "whitespace only", orig: "a\n  b", mod: "a\n\tb", want: "[1,2,1,2,[[1,0,1,2,1,0,1,1]]]"},
		{name: "whitespace ignored", orig: "a\n  b ", mod: "a\n\tb", opts: Options{IgnoreTrimWhitespace: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Lines(bg, lines(tc.orig), lines(tc.mod), tc.opts)
			checkResult(t, lines(tc.orig), lines(tc.mod), tc.opts, res)
			if got := strings.Join(formatChanges(res.Changes), " "); got != tc.want {
				t.Errorf("changes = %s, want %s", got, tc.want)
			}
			if res.Identical != tc.noChange || res.TimedOut {
				t.Errorf("Identical = %v, TimedOut = %v", res.Identical, res.TimedOut)
			}
		})
	}
}

func TestEmptySlices(t *testing.T) {
	res := Lines(bg, nil, []string{}, Options{})
	if !res.Identical || res.OriginalLines != 1 || res.ModifiedLines != 1 {
		t.Fatalf("got %+v", res)
	}
	res = Lines(bg, nil, []string{"x"}, Options{})
	if got := strings.Join(formatChanges(res.Changes), " "); got != "[0,1,0,1,[[0,0,0,0,0,0,0,1]]]" {
		t.Fatalf("changes = %s", got)
	}
}

func TestSplitLines(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a\n", []string{"a", ""}},
		{"a\r\nb\rc\nd", []string{"a", "b", "c", "d"}},
		{"\r\n\r\n", []string{"", "", ""}},
		{"a\r", []string{"a", ""}},
	} {
		if got := SplitLines(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("SplitLines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCRLF(t *testing.T) {
	crlf, lf := "one\r\ntwo\r\nthree\r\n", "one\ntwo\nthree\n"
	if res := Lines(bg, SplitLines(crlf), SplitLines(lf), Options{}); !res.Identical || len(res.Changes) != 0 {
		t.Errorf("SplitLines: CRLF vs LF reported %v", res.Changes)
	}
	if res := Snapshots(bg, buffer.NewSnapshot(crlf), buffer.NewSnapshot(lf), Options{}); !res.Identical {
		t.Errorf("Snapshots: CRLF vs LF reported %v", res.Changes)
	}
	// Splitting at "\n" only leaves the "\r" in the lines; it then shows
	// up as a trailing character change, as it should.
	res := Lines(bg, strings.Split(crlf, "\n"), strings.Split("one\r\ntwo\nthree\r\n", "\n"), Options{})
	if got := strings.Join(formatChanges(res.Changes), " "); got != "[1,2,1,2,[[1,3,1,4,1,3,1,3]]]" {
		t.Errorf("changes = %s", got)
	}
	if res := Lines(bg, strings.Split(crlf, "\n"), strings.Split("one\r\ntwo\nthree\r\n", "\n"), Options{IgnoreTrimWhitespace: true}); len(res.Changes) != 0 || res.Identical {
		t.Errorf("trailing \\r should be ignored as whitespace: %v", res.Changes)
	}
}

func TestSnapshots(t *testing.T) {
	a := buffer.NewSnapshot("package p\n\nfunc f() {\n\treturn\n}\n")
	b, err := a.Apply([]buffer.Edit{{Start: a.LineStart(3) + 1, End: a.LineStart(3) + 1 + len("return"), Text: "panic(1)"}})
	if err != nil {
		t.Fatal(err)
	}
	res := Snapshots(bg, a, b, Options{})
	if got := strings.Join(formatChanges(res.Changes), " "); got != "[3,4,3,4,[[3,1,3,7,3,1,3,9]]]" {
		t.Fatalf("changes = %s", got)
	}
	if res.OriginalLines != a.LineCount() || res.ModifiedLines != b.LineCount() {
		t.Fatalf("line counts %d/%d", res.OriginalLines, res.ModifiedLines)
	}
	// Columns are byte columns usable with Snapshot.OffsetOf.
	in := res.Changes[0].Inner[0]
	if got := b.Slice(b.OffsetOf(in.Modified.Start), b.OffsetOf(in.Modified.End)); got != "panic(1)" {
		t.Fatalf("modified range selects %q", got)
	}
}

func TestUnicodeColumns(t *testing.T) {
	res := Lines(bg, []string{"名前 = 値"}, []string{"名前 = 数値"}, Options{})
	in := res.Changes[0].Inner
	if len(in) != 1 {
		t.Fatalf("inner = %v", in)
	}
	// "名前 = " is 9 bytes; the inserted 数 is 3 bytes.
	if got := in[0].Modified; got.Start.Col != 9 || got.End.Col != 12 || in[0].Original.Start.Col != 9 || !in[0].Original.Empty() {
		t.Fatalf("inner = %v", in)
	}
}

func TestLongLine(t *testing.T) {
	long := strings.Repeat("abc def ", 50_000) // 400 KB
	mod := long[:200_000] + "XYZ" + long[200_000:]
	res := Lines(bg, []string{"head", long, "tail"}, []string{"head", mod, "tail"}, Options{})
	checkResult(t, []string{"head", long, "tail"}, []string{"head", mod, "tail"}, Options{}, res)
	if len(res.Changes) != 1 || len(res.Changes[0].Inner) != 1 {
		t.Fatalf("changes = %v", formatChanges(res.Changes))
	}
	in := res.Changes[0].Inner[0]
	if in.Modified.Start.Line != 1 || in.Modified.End.Col-in.Modified.Start.Col > 16 {
		t.Fatalf("inner change too wide: %v", in)
	}
}

// randomLines returns n lines drawn from a small vocabulary, so that the
// alignment has many ambiguous choices.
func randomLines(r *rand.Rand, n int) []string {
	vocab := []string{"", "{", "}", "\treturn x", "  if (a) {", "foo(bar);", "let x = 1;", "x += 2;  ", "// note", "end"}
	out := make([]string, n)
	for i := range out {
		out[i] = vocab[r.IntN(len(vocab))]
	}
	return out
}

func mutate(r *rand.Rand, a []string) []string {
	b := slices.Clone(a)
	for range 1 + r.IntN(5) {
		p := r.IntN(len(b) + 1)
		switch r.IntN(4) {
		case 0:
			b = slices.Insert(b, p, randomLines(r, 1+r.IntN(3))...)
		case 1:
			if p < len(b) {
				b = slices.Delete(b, p, min(len(b), p+1+r.IntN(3)))
			}
		case 2:
			if p < len(b) {
				q := r.IntN(len(b[p]) + 1)
				b[p] = b[p][:q] + "z " + b[p][q:]
			}
		default:
			if p < len(b) {
				b[p] = "\t" + b[p] + " "
			}
		}
	}
	return b
}

func TestRandomInvariants(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := range 2000 {
		a := randomLines(r, r.IntN(40))
		b := mutate(r, a)
		opts := Options{IgnoreTrimWhitespace: i%3 == 0, ComputeMoves: i%2 == 0, ExtendToSubwords: i%5 == 0}
		res := Lines(bg, a, b, opts)
		t.Run(fmt.Sprint(i), func(t *testing.T) { checkResult(t, a, b, opts, res) })
		if t.Failed() {
			t.Fatalf("original %q\nmodified %q", a, b)
		}
	}
}

// differentFiles returns two large files with almost nothing in common, the
// worst case for Myers.
func differentFiles(n int) ([]string, []string) {
	r := rand.New(rand.NewPCG(3, 4))
	a, b := make([]string, n), make([]string, n)
	for i := range n {
		a[i] = fmt.Sprintf("a%d %d", i, r.Int())
		b[i] = fmt.Sprintf("b%d %d", i, r.Int())
		if i%50 == 0 {
			b[i] = a[i]
		}
	}
	return a, b
}

func TestTimeout(t *testing.T) {
	a, b := differentFiles(20_000)
	start := time.Now()
	res := Lines(bg, a, b, Options{Timeout: 20 * time.Millisecond, ComputeMoves: true})
	if !res.TimedOut {
		t.Fatal("TimedOut not set")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("timed-out diff took %v", d)
	}
	checkResult(t, a, b, Options{}, res)
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(bg)
	cancel()
	a := []string{"x", "a", "b", "c", "y"}
	b := []string{"x", "c", "b", "a", "y"}
	res := Lines(ctx, a, b, Options{})
	if !res.TimedOut {
		t.Fatal("TimedOut not set")
	}
	checkResult(t, a, b, Options{}, res)
	// The coarse fallback still keeps the common prefix and suffix out.
	if got := strings.Join(formatChanges(res.Changes), " "); got != "[1,4,1,4,[[1,0,4,0,1,0,4,0]]]" {
		t.Fatalf("changes = %s", got)
	}
}

// bigFile returns n lines of plausible source code.
func bigFile(n int) []string {
	out := make([]string, n)
	for i := range out {
		switch i % 10 {
		case 0:
			out[i] = fmt.Sprintf("func f%d(x int) int {", i)
		case 9:
			out[i] = "}"
		case 5:
			out[i] = ""
		default:
			out[i] = fmt.Sprintf("\tx = x*%d + compute(x, %q)", i%7, fmt.Sprint("v", i))
		}
	}
	return out
}

func TestLargeFileSmallEdit(t *testing.T) {
	a := bigFile(100_000)
	b := slices.Clone(a)
	b[50_001] = "\tx = x*1 + compute(x, \"changed\")"
	b = slices.Insert(b, 90_000, "\t// new line")
	res := Lines(bg, a, b, Options{Timeout: DefaultTimeout})
	checkResult(t, a, b, Options{}, res)
	if got := strings.Join(formatChanges(res.Changes), " "); got != `[50001,50002,50001,50002,[[50001,7,50001,8,50001,7,50001,8],[50001,23,50001,29,50001,23,50001,30]]] [90000,90000,90000,90001,[[90000,0,90000,0,90000,0,90001,0]]]` {
		t.Fatalf("changes = %s", got)
	}
}

func TestMoves(t *testing.T) {
	block := []string{"func helper(a, b int) int {", "\tif a > b {", "\t\treturn a - b", "\t}", "\treturn b - a", "}"}
	filler := bigFile(30)
	a := slices.Concat(block, filler)
	b := slices.Concat(filler, block)
	res := Lines(bg, a, b, Options{ComputeMoves: true})
	checkResult(t, a, b, Options{}, res)
	if len(res.Moves) != 1 {
		t.Fatalf("moves = %v", formatMoves(res.Moves))
	}
	mv := res.Moves[0]
	if mv.Original != (LineRange{0, 6}) || mv.Modified != (LineRange{30, 36}) || len(mv.Changes) != 0 {
		t.Fatalf("move = %v", formatMoves(res.Moves))
	}
	if res := Lines(bg, a, b, Options{}); res.Moves != nil {
		t.Fatal("moves computed without ComputeMoves")
	}
}

func TestUnchangedRegions(t *testing.T) {
	ch := func(o0, o1, m0, m1 int) Change {
		return Change{Original: LineRange{o0, o1}, Modified: LineRange{m0, m1}}
	}
	tests := []struct {
		name                string
		changes             []Change
		origLines, modLines int
		context, minHidden  int
		want                string
	}{
		{"no changes", nil, 20, 20, 3, 3, "[0,20)/[0,20)"},
		{"too short", nil, 5, 5, 3, 3, ""},
		{"middle change", []Change{ch(10, 11, 10, 12)}, 30, 31, 3, 3, "[0,7)/[0,7) [14,30)/[15,31)"},
		{"start and end change", []Change{ch(0, 1, 0, 1), ch(29, 30, 29, 30)}, 30, 30, 3, 3, "[4,26)/[4,26)"},
		{"gap below threshold", []Change{ch(0, 1, 0, 1), ch(9, 10, 9, 10)}, 10, 10, 3, 3, ""},
		{"gap at threshold", []Change{ch(0, 1, 0, 1), ch(10, 11, 10, 11)}, 11, 11, 3, 3, "[4,7)/[4,7)"},
		{"insertion shifts sides", []Change{ch(5, 5, 5, 8)}, 20, 23, 2, 1, "[0,3)/[0,3) [7,20)/[10,23)"},
		{"zero context", []Change{ch(5, 6, 5, 6)}, 10, 10, 0, 1, "[0,5)/[0,5) [6,10)/[6,10)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var parts []string
			for _, r := range UnchangedRegions(tc.changes, tc.origLines, tc.modLines, tc.context, tc.minHidden) {
				if r.Original.Len() != r.Modified.Len() {
					t.Fatalf("unequal region %v", r)
				}
				parts = append(parts, r.Original.String()+"/"+r.Modified.String())
			}
			if got := strings.Join(parts, " "); got != tc.want {
				t.Errorf("regions = %q, want %q", got, tc.want)
			}
		})
	}
	// The method form uses the result's own line counts.
	a := bigFile(100)
	b := slices.Clone(a)
	b[50] = "changed"
	res := Lines(bg, a, b, Options{})
	got := res.UnchangedRegions(3, 3)
	if len(got) != 2 || got[0].Original != (LineRange{0, 47}) || got[1].Original != (LineRange{54, 100}) {
		t.Fatalf("regions = %v", got)
	}
}

func TestNavigation(t *testing.T) {
	res := Result{
		OriginalLines: 40, ModifiedLines: 42,
		Changes: []Change{
			{Original: LineRange{5, 6}, Modified: LineRange{5, 6}},
			{Original: LineRange{10, 10}, Modified: LineRange{10, 12}},
			{Original: LineRange{20, 23}, Modified: LineRange{22, 22}},
			{Original: LineRange{40, 40}, Modified: LineRange{39, 42}},
		},
	}
	tests := []struct {
		side       Side
		line       int
		next, prev int
	}{
		{Modified, 0, 0, 3},
		{Modified, 5, 1, 3},
		{Modified, 6, 1, 0},
		{Modified, 10, 2, 0},
		{Modified, 22, 3, 1},
		{Modified, 38, 3, 2},
		{Modified, 39, 0, 2},
		{Modified, 41, 0, 3}, // last line: next wraps to the first change
		{Original, 10, 2, 0},
		{Original, 30, 3, 2},
		{Original, 39, 0, 2}, // change 3 starts past the last line
	}
	for _, tc := range tests {
		if got, ok := res.Next(tc.side, tc.line); !ok || got != tc.next {
			t.Errorf("Next(%v, %d) = %d, %v; want %d", tc.side, tc.line, got, ok, tc.next)
		}
		if got, ok := res.Prev(tc.side, tc.line); !ok || got != tc.prev {
			t.Errorf("Prev(%v, %d) = %d, %v; want %d", tc.side, tc.line, got, ok, tc.prev)
		}
	}
	var empty Result
	if _, ok := empty.Next(Modified, 0); ok {
		t.Error("Next on no changes")
	}
	if _, ok := empty.Prev(Original, 0); ok {
		t.Error("Prev on no changes")
	}
}

func TestLineRange(t *testing.T) {
	r := LineRange{3, 6}
	if r.Len() != 3 || r.Empty() || !r.Contains(3) || r.Contains(6) || r.String() != "[3,6)" {
		t.Fatal(r)
	}
	if !(LineRange{4, 4}).Empty() {
		t.Fatal("empty range")
	}
}

func TestCancelledWhitespaceOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(bg)
	cancel()
	a, b := []string{"a", " b", "c"}, []string{"a", "b ", "c"}
	res := Lines(ctx, a, b, Options{})
	checkResult(t, a, b, Options{}, res)
	if got := strings.Join(formatChanges(res.Changes), " "); got != "[1,2,1,2,[[1,0,2,0,1,0,2,0]]]" {
		t.Fatalf("changes = %s", got)
	}
}

func TestLineSet(t *testing.T) {
	var s lineSet
	for _, r := range []LineRange{{10, 12}, {2, 4}, {4, 6}, {20, 25}, {11, 15}} {
		s.add(r)
	}
	if fmt.Sprint(s.ranges) != "[[2,6) [10,15) [20,25)]" {
		t.Fatalf("ranges = %v", s.ranges)
	}
	for line, want := range map[int]bool{1: false, 2: true, 5: true, 6: false, 14: true, 15: false, 24: true, 25: false} {
		if s.contains(line) != want {
			t.Errorf("contains(%d) = %v", line, !want)
		}
	}
	if got := s.subtractFrom(LineRange{0, 30}); fmt.Sprint(got.ranges) != "[[0,2) [6,10) [15,20) [25,30)]" {
		t.Errorf("subtractFrom = %v", got.ranges)
	}
	if got := s.subtractFrom(LineRange{3, 12}); fmt.Sprint(got.ranges) != "[[6,10)]" {
		t.Errorf("subtractFrom = %v", got.ranges)
	}
	if got := s.subtractFrom(LineRange{11, 14}); len(got.ranges) != 0 {
		t.Errorf("subtractFrom = %v", got.ranges)
	}
	other := lineSet{ranges: []LineRange{{0, 3}, {5, 11}, {24, 30}}}
	if got := s.intersection(other); fmt.Sprint(got.ranges) != "[[2,3) [5,6) [10,11) [24,25)]" {
		t.Errorf("intersection = %v", got.ranges)
	}
	if got := s.withDelta(-2); fmt.Sprint(got.ranges) != "[[0,4) [8,13) [18,23)]" {
		t.Errorf("withDelta = %v", got.ranges)
	}
}
