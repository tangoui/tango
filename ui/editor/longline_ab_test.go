package editor

import (
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
)

// Keystroke work in a long wrapped line that does not depend on the line's
// length (editor M2 failed-gate follow-up, Task A): the copies of the line
// out of the rope, the highlighter's work and the minimap's per-row work.

// keystrokeAlloc is what one keystroke typed in the middle of a single
// wrapped line of about n bytes (bracketLongLine, View defaults) costs:
// bytes allocated, bytes the minimap walked grapheme by grapheme, and
// whether the highlighter still had the line final right after the edit
// (before the key's frame).
type keystrokeAlloc struct {
	alloc     uint64
	miniScan  int64
	finalKeys int // keys after which the line was still final
}

func measureKeystrokeAlloc(t *testing.T, n int) keystrokeAlloc {
	t.Helper()
	goText(t)
	old := bracketFrameBudget
	bracketFrameBudget = time.Minute
	countWork = true
	defer func() { bracketFrameBudget, countWork = old, false }()
	line := bracketLongLine(n)
	st := newTestState(line, "bundle.ts", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	ed := newEditDriver(st, 900, 600, pinMac)
	ed.focus(t)
	mid := strings.Index(line[len(line)/2:], "; ") + len(line)/2 + 2
	st.MoveCaret(mid, false)
	st.RevealCaret()
	ed.Frame()
	ed.Frame()
	var out keystrokeAlloc
	key := func(count bool) {
		if count {
			// The key's edit, applied before its frame as the view's
			// TextInput handling does (State.Type).
			n := st.Snapshot().Len()
			st.Type("x")
			if st.Snapshot().Len() != n+1 {
				t.Fatal("the key did not insert")
			}
			if hl := st.Highlighter(); hl != nil && hl.FinalBefore(st.Snapshot(), 1) {
				out.finalKeys++
			}
		} else {
			ed.Inject(input.TextInput{Text: "x"})
		}
		ed.Frame()
	}
	for range 3 {
		key(false)
	}
	if !st.wrapping() || st.Snapshot().LineCount() != 1 {
		t.Fatal("not one wrapped line")
	}
	const keys = 8
	resetWork()
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for range keys {
		key(true)
	}
	runtime.ReadMemStats(&m1)
	out.alloc = (m1.TotalAlloc - m0.TotalAlloc) / keys
	out.miniScan = workMiniScan.Load() / keys
	return out
}

// TestLongLineKeystrokeAllocs: a keystroke in the middle of a 1 MB wrapped
// line copies the line out of the rope once (the rewrap's read, shared by
// every other reader through lineMemo), not once more for the folding
// ranges, the indentation guides and the highlighter.
func TestLongLineKeystrokeAllocs(t *testing.T) {
	small := measureKeystrokeAlloc(t, 64<<10)
	large := measureKeystrokeAlloc(t, 1<<20)
	t.Logf("per key, 64 KB line: %+v", small)
	t.Logf("per key, 1 MB line:  %+v", large)
	perCopy := float64((1 << 20) - (64 << 10))
	if copies := (float64(large.alloc) - float64(small.alloc)) / perCopy; copies > 1.5 {
		t.Errorf("a key on the 1 MB line allocates %d bytes, %d on the 64 KB one: about %.1f copies of the line, want 1",
			large.alloc, small.alloc, copies+1)
	}
}

// TestLongLineEditPastTokenizationCut: an edit of a long line after the
// part the lexer reads (monarch.MaxLineLength) leaves its tokens and end
// state as they are, so the line stays final and nothing (the view, the
// bracket tree) tokenizes it again on the key.
func TestLongLineEditPastTokenizationCut(t *testing.T) {
	goText(t)
	line := bracketLongLine(1 << 20)
	st := newTestState(line, "bundle.ts", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	retryDegradedAtOnce(st)
	ed := newEditDriver(st, 900, 600, pinMac)
	ed.focus(t)
	mid := strings.Index(line[len(line)/2:], "; ") + len(line)/2 + 2
	st.MoveCaret(mid, false)
	st.RevealCaret()
	// Fixed warm-up frames can leave the initial long line Degraded on
	// a loaded machine. Establish the precondition explicitly, rather
	// than expecting an unfinished line to stay final after an edit.
	settleTokens(t, st)
	for key := range 8 {
		n := st.Snapshot().Len()
		st.Type("x")
		if st.Snapshot().Len() != n+1 {
			t.Fatal("the key did not insert")
		}
		// Do not finish the work after this edit: retaining final tokens
		// immediately is exactly the property this test checks.
		if !st.Highlighter().FinalBefore(st.Snapshot(), 1) {
			t.Fatalf("line not final immediately after key %d", key+1)
		}
		ed.Frame()
	}
}

// TestLongLineMinimapRowsReused: the minimap rows of a long wrapped line
// whose source text a key did not change are not walked again: the key
// walks the rows it changed (the caret's, and those the rewrap moved), not
// every row in the minimap.
func TestLongLineMinimapRowsReused(t *testing.T) {
	r := measureKeystrokeAlloc(t, 1<<20)
	t.Logf("minimap grapheme bytes per key: %d", r.miniScan)
	if r.miniScan > 8<<10 {
		t.Errorf("the minimap walked %d bytes per key, want at most %d (a few rows)", r.miniScan, 8<<10)
	}
}

// TestNextGraphemeASCIIFast: nextGrapheme's ASCII shortcut gives the
// clusters the segmentation rules give, for every pair of ASCII characters
// followed by any of a set of characters (ASCII, combining marks, ZWJ,
// emoji, regional indicators, Hangul, CJK, invalid bytes), and on random
// mixed strings.
func TestNextGraphemeASCIIFast(t *testing.T) {
	tails := []string{"", "a", "\r", "\n", "́", "‍", "\U0001F600", "\U0001F1E6", "ᄀ", "가", "中", "\xff", "ः", "؀"}
	for c := range 128 {
		for d := range 128 {
			for _, tail := range tails {
				s := string([]byte{byte(c), byte(d)}) + tail
				for i := range len(s) {
					if got, want := nextGrapheme(s, i), nextGraphemeRules(s, i); got != want {
						t.Fatalf("nextGrapheme(%q, %d) = %d, rules %d", s, i, got, want)
					}
				}
			}
		}
	}
	parts := []string{"a", " ", "\t", "\r", "\n", "\r\n", "́", "‍", "\U0001F600", "\U0001F469", "\U0001F1E6", "ᄀ", "ᅡ", "가", "中", "\xff", "é", "؀", "x"}
	rng := rand.New(rand.NewPCG(3, 5))
	for range 20000 {
		var sb strings.Builder
		for range 1 + rng.IntN(12) {
			sb.WriteString(parts[rng.IntN(len(parts))])
		}
		s := sb.String()
		for i := range len(s) + 1 {
			if got, want := nextGrapheme(s, i), nextGraphemeRules(s, i); got != want {
				t.Fatalf("nextGrapheme(%q, %d) = %d, rules %d", s, i, got, want)
			}
		}
	}
}

// TestMiniatureDepExact: a source that keeps a cached row's length, short
// flag and first dep bytes (miniatureDep) has the cached row's miniature,
// whatever the rest of it holds.
func TestMiniatureDepExact(t *testing.T) {
	parts := []string{"a", "b", " ", "\t", "\r", "\n", "́", "‍", "\U0001F600", "\U0001F1E6", "ᄀ", "가", "中", "\xff", "\xe4\xb8", "é", "x"}
	rng := rand.New(rand.NewPCG(9, 13))
	for n := range 100000 {
		var sb strings.Builder
		for range rng.IntN(40) {
			sb.WriteString(parts[rng.IntN(len(parts))])
		}
		short := rng.IntN(2) == 0
		cols, tab := 1+rng.IntN(20), 1+rng.IntN(4)
		raw := trimMiniSource(sb.String(), short)
		text, dep := miniatureDep(raw, short, cols, tab)
		if text != miniatureOf(sb.String(), short, cols, tab) {
			t.Fatal("miniatureDep differs from miniatureOf")
		}
		// Other bytes after dep, same length.
		b := []byte(raw)
		for k := dep; k < len(b); k++ {
			const alphabet = "a\xcc\x81\xe4\xb8\xad\xf0\x9f\x98\x80 \t"
			b[k] = alphabet[rng.IntN(len(alphabet))]
		}
		other := string(b)
		if trimMiniSource(other, short) != other {
			continue // no longer the same trimmed length
		}
		if got, _ := miniatureDep(other, short, cols, tab); got != text {
			t.Fatalf("case %d: %q (dep %d) -> %q, but %q -> %q", n, raw, dep, text, other, got)
		}
	}
}
