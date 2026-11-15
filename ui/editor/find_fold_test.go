package editor

import (
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"
)

// TestFindLongSelectionNoRegexp: Cmd+D / Cmd+Shift+L from a selection
// longer than findRegexpMax match case-insensitively without compiling a
// regexp. Compiling "(?i)" + QuoteMeta(text) for a select-all of a large
// file stalls the frame (1M chars: ~85 ms, 227 MB) and panics in
// regexp.MustCompile past ~10–30M chars ("expression too large").
func TestFindLongSelectionNoRegexp(t *testing.T) {
	unit := strings.Repeat("Kelvin ſigma straße ", findRegexpMax/20+1) // > findRegexpMax bytes
	// The second copy differs only in case, including the non-ASCII
	// simple folds K/k/K (Kelvin sign) and ſ/s/S that (?i) also matches.
	other := strings.NewReplacer("Kelvin", "KELVIN", "ſigma", "SIGMA", "straße", "STRAßE").Replace(unit)
	doc := unit + "|" + other + "|" + unit
	st := NewState(doc, "a.txt", nil)
	st.SetSelections([]Selection{sel(0, len(unit))})

	f := st.findSessionNow()
	if f == nil {
		t.Fatal("no find session")
	}
	if f.re != nil {
		t.Errorf("compiled a %d-byte regexp for a long selection", len(f.text))
	}
	if !f.fold {
		t.Fatal("long selection does not match case-insensitively")
	}

	st.AddSelectionToNextFindMatch()
	b := len(unit) + 1
	wantSels(t, st, "next match differs in case", sel(0, len(unit)), sel(b, b+len(other)))
	st.AddSelectionToNextFindMatch()
	c := b + len(other) + 1
	wantSels(t, st, "third match", sel(0, len(unit)), sel(b, b+len(other)), sel(c, c+len(unit)))

	st.SetSelections([]Selection{sel(0, len(unit))})
	st.SelectHighlights()
	if len(st.Sels) != 3 {
		t.Errorf("select highlights: %d selections, want 3", len(st.Sels))
	}
}

// TestFindInvalidUTF8Selection: Cmd+D from a short selection with invalid
// UTF-8, which regexp refuses to compile (MustCompile used to panic),
// matches case-insensitively by foldIndex.
func TestFindInvalidUTF8Selection(t *testing.T) {
	st := NewState("x\xffA y \xffa", "a.txt", nil)
	st.SetSelections([]Selection{sel(1, 3)})
	st.AddSelectionToNextFindMatch()
	wantSels(t, st, "invalid UTF-8 selection", sel(1, 3), sel(6, 8))
}

// TestFoldIndex: foldIndex agrees with regexp (?i) matching of a literal
// (simple case folding, matches whose byte length differs from the
// needle's, invalid UTF-8, the lo bound).
func TestFoldIndex(t *testing.T) {
	for _, c := range []struct {
		text, needle string
		lo, a, b     int
	}{
		{"xxABCxx", "abc", 0, 2, 5},
		{"abc ABC", "abc", 1, 4, 7},
		{"K", "k", 0, 0, 3}, // Kelvin sign: 3 bytes for a 1-byte needle
		{"ok", "K", 0, 1, 2},
		{"STRASSE", "straße", 0, -1, -1}, // ß has no simple fold to "ss"
		{"ab", "abc", 0, -1, -1},
		{"a\xffb", "\xffB", 0, 1, 3},
		{"a\xfeb\xffb", "\xffB", 0, 3, 5}, // an invalid byte matches only itself
		{"abc", "", 0, -1, -1},
	} {
		a, b := foldIndex(c.text, c.lo, c.needle)
		if a != c.a || b != c.b {
			t.Errorf("foldIndex(%q, %d, %q) = %d, %d; want %d, %d", c.text, c.lo, c.needle, a, b, c.a, c.b)
		}
	}
	// Against regexp on random text over an alphabet of case variants.
	alpha := []string{"a", "A", "k", "K", "K", "s", "S", "ſ", "ß", "ẞ", "σ", "Σ", "ς", "é", "É", " ", "\xff"}
	rng := rand.New(rand.NewPCG(1, 2))
	gen := func(n int) string {
		var sb strings.Builder
		for range n {
			sb.WriteString(alpha[rng.IntN(len(alpha))])
		}
		return sb.String()
	}
	for range 20000 {
		text, needle := gen(rng.IntN(12)), gen(1+rng.IntN(3))
		lo := 0
		if len(text) > 0 {
			lo = rng.IntN(len(text) + 1)
		}
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(needle))
		if err != nil {
			continue // invalid UTF-8: the table above covers it
		}
		wa, wb := -1, -1
		if m := re.FindStringIndex(text[lo:]); m != nil && m[1] > m[0] {
			wa, wb = lo+m[0], lo+m[1]
		}
		if a, b := foldIndex(text, lo, needle); a != wa || b != wb {
			t.Fatalf("foldIndex(%q, %d, %q) = %d, %d; regexp %d, %d", text, lo, needle, a, b, wa, wb)
		}
	}
}
