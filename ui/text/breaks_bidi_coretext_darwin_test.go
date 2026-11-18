//go:build darwin && !ios && tango_coretext

package text

import (
	"slices"
	"testing"
)

func TestCTLineEndsChinesePunct(t *testing.T) {
	r := []rune("你好，世界。再见")
	ends := lineEnds(r, 0, len(r))
	for i, c := range r {
		if (c == '，' || c == '。') && slices.Contains(ends, i) {
			t.Errorf("ends %v allow a break before %q at %d", ends, c, i)
		}
	}
	if len(ends) == 0 || ends[len(ends)-1] != len(r) {
		t.Errorf("ends %v do not end with %d", ends, len(r))
	}
	if !slices.IsSorted(ends) {
		t.Errorf("ends %v not ascending", ends)
	}
}

func TestCTLineEndsLatinWords(t *testing.T) {
	r := []rune("hello world foo")
	if got, want := lineEnds(r, 0, len(r)), []int{6, 12, 15}; !slices.Equal(got, want) {
		t.Errorf("lineEnds = %v, want %v", got, want)
	}
	// A sub-range reports absolute rune indices.
	r = []rune("xx\nhello world")
	if got, want := lineEnds(r, 3, len(r)), []int{9, 14}; !slices.Equal(got, want) {
		t.Errorf("lineEnds(3) = %v, want %v", got, want)
	}
}

func TestCTLineEndsAstral(t *testing.T) {
	r := []rune("a😀b")
	ends := lineEnds(r, 0, len(r))
	if len(ends) == 0 || slices.Max(ends) != 3 {
		t.Errorf("lineEnds = %v, want max 3", ends)
	}
}

func TestCTLineEndsEmpty(t *testing.T) {
	r := []rune("ab")
	if got := lineEnds(r, 1, 1); !slices.Equal(got, []int{1}) {
		t.Errorf("lineEnds(empty) = %v, want [1]", got)
	}
}

func TestCTUTF16Index(t *testing.T) {
	r := []rune("xa😀b")
	got := utf16Index(r, 1, len(r))
	want := []int32{1, 2, 2, 3, 4} // a, high, low, b, end
	if !slices.Equal(got, want) {
		t.Errorf("utf16Index = %v, want %v", got, want)
	}
}

func ctLevels(s string, dir Direction) ([]rune, []level, level, bool) {
	r := []rune(s)
	lv := make([]level, len(r))
	base, mixed := paraLevels(r, 0, len(r), dir, lv)
	return r, lv, base, mixed
}

func TestCTParaLevelsPureLTR(t *testing.T) {
	_, lv, base, mixed := ctLevels("abc", DirAuto)
	if base != 0 || mixed || !slices.Equal(lv, []level{0, 0, 0}) {
		t.Errorf("base %d mixed %v lv %v", base, mixed, lv)
	}
}

func TestCTParaLevelsHebrewInLatin(t *testing.T) {
	r, lv, base, mixed := ctLevels("abc שלום def", DirAuto)
	if base != 0 || !mixed {
		t.Fatalf("base %d mixed %v", base, mixed)
	}
	for i, c := range r {
		switch {
		case c >= 0x5d0 && c <= 0x5ea:
			if !lv[i].rtl() {
				t.Errorf("Hebrew %q at %d: level %d", c, i, lv[i])
			}
		case c >= 'a' && c <= 'z':
			if lv[i].rtl() {
				t.Errorf("Latin %q at %d: level %d", c, i, lv[i])
			}
		}
	}
}

func TestCTParaLevelsRTLBase(t *testing.T) {
	_, lv, base, mixed := ctLevels("abc", DirRTL)
	if !base.rtl() || !mixed {
		t.Fatalf("base %d mixed %v", base, mixed)
	}
	for i, v := range lv {
		if v.rtl() || v <= base {
			t.Errorf("rune %d level %d", i, v)
		}
	}
}

func TestCTParaLevelsDigitsInRTL(t *testing.T) {
	r, lv, base, _ := ctLevels("שלום 123", DirAuto)
	if !base.rtl() {
		t.Fatalf("base %d", base)
	}
	for i, c := range r {
		if c >= '0' && c <= '9' && (lv[i].rtl() || lv[i] == 0) {
			t.Errorf("digit %q at %d: level %d", c, i, lv[i])
		}
	}
	var digits []rune
	for _, j := range visualOrder(lv) {
		if c := r[j]; c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	if string(digits) != "123" {
		t.Errorf("visual digits %q, want 123", string(digits))
	}
}

// Exact UAX #9 levels where a nested left-to-right run is not followed by
// another right-to-left run in logical order.
func TestCTParaLevelsNumberAfterHebrew(t *testing.T) {
	_, lv, base, mixed := ctLevels("abc שלום 123 def", DirAuto)
	want := []level{0, 0, 0, 0, 1, 1, 1, 1, 1, 2, 2, 2, 0, 0, 0, 0}
	if base != 0 || !mixed || !slices.Equal(lv, want) {
		t.Errorf("base %d mixed %v lv %v, want %v", base, mixed, lv, want)
	}
}

// DirAuto takes the first strong character (rules P2/P3): digits are weak.
func TestCTParaLevelsAutoFirstStrong(t *testing.T) {
	if _, _, base, mixed := ctLevels("123 שלום abc", DirAuto); base != 1 || !mixed {
		t.Errorf("digits first: base %d mixed %v", base, mixed)
	}
	if _, lv, base, mixed := ctLevels("שלום", DirAuto); base != 1 || !mixed || !slices.Equal(lv, []level{1, 1, 1, 1}) {
		t.Errorf("pure Hebrew: base %d mixed %v lv %v", base, mixed, lv)
	}
	if _, lv, base, _ := ctLevels("שלום", DirLTR); base != 0 || !slices.Equal(lv, []level{1, 1, 1, 1}) {
		t.Errorf("Hebrew in LTR: base %d lv %v", base, lv)
	}
	if _, lv, base, mixed := ctLevels("123", DirAuto); base != 0 || mixed || !slices.Equal(lv, []level{0, 0, 0}) {
		t.Errorf("no strong: base %d mixed %v lv %v", base, mixed, lv)
	}
}

// Text after a bidi paragraph separator other than '\n' resolves on its own
// and the separator takes the paragraph level.
func TestCTParaLevelsSeparator(t *testing.T) {
	r, lv, base, _ := ctLevels("abc שלום", DirAuto)
	if base != 0 || lv[3] != 0 {
		t.Errorf("base %d sep level %d", base, lv[3])
	}
	for i := 4; i < len(r); i++ {
		if !lv[i].rtl() {
			t.Errorf("rune %d level %d", i, lv[i])
		}
	}
}

// Strong characters that are not letters set the DirAuto level too (rules
// P2/P3): Arabic and Hebrew punctuation are R/AL, Roman numerals and
// circled letters are L.
func TestCTParaLevelsAutoStrongPunct(t *testing.T) {
	for _, c := range []struct {
		s    string
		base level
	}{
		{"؟ abc", 1},
		{"؟123", 1},
		{"־ abc", 1},
		{"۔ abc", 1},
		{"אְ", 1},    // a letter with a mark
		{"ְ abc", 0}, // a mark alone is weak (NSM)
		{"٠ abc", 0}, // Arabic-Indic digit is weak (AN)
		{"، abc", 0}, // Arabic comma is weak (CS)
		{"Ⅻ שלום", 0},
		{"Ⓐ שלום", 0},
		{"⁧שלום⁩ abc", 0}, // isolates are skipped
		{"‏123", 1},       // RLM
	} {
		r, lv, base, _ := ctLevels(c.s, DirAuto)
		if base != c.base {
			t.Errorf("%q: base %d, want %d (lv %v)", c.s, base, c.base, lv)
		}
		for i, v := range lv {
			if v < base {
				t.Errorf("%q: rune %d (%q) level %d below base %d", c.s, i, r[i], v, base)
			}
		}
	}
}

// A nested left-to-right stretch split into several glyph runs (font
// fallback for the full-width digit) stays nested as a whole.
func TestCTParaLevelsSplitNestedRun(t *testing.T) {
	s := "abc שלום 12３ def"
	r := []rune(s)
	n := 0
	for _, ru := range ctDirRuns(r, 0, len(r), ctWritingDirectionLeftToRight) {
		if ru.start >= 9 && ru.end <= 12 {
			n++
		}
	}
	if n < 2 {
		t.Fatalf("digits not split into several runs (%d); the test needs a split", n)
	}
	_, lv, base, mixed := ctLevels(s, DirAuto)
	want := []level{0, 0, 0, 0, 1, 1, 1, 1, 1, 2, 2, 2, 0, 0, 0, 0}
	if base != 0 || !mixed || !slices.Equal(lv, want) {
		t.Errorf("base %d mixed %v lv %v, want %v", base, mixed, lv, want)
	}
	wantVis := []int{0, 1, 2, 3, 9, 10, 11, 8, 7, 6, 5, 4, 12, 13, 14, 15}
	if got := visualOrder(lv); !slices.Equal(got, wantVis) {
		t.Errorf("visualOrder %v, want %v", got, wantVis)
	}
}
