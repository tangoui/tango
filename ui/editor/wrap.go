package editor

import (
	"math"
	"strings"
	"unicode/utf8"
)

// Word wrap (Monaco's wordWrap, wordWrapColumn, wrappingIndent and the
// "simple" wrappingStrategy, monospaceLineBreaksComputer).
//
// A model line longer than the wrapping column is shown as several view
// lines (rows). The break points follow Monaco's rules, on the editor's
// column grid (State.advance: tabs to the next stop, wide characters two
// columns, other grapheme clusters one):
//
//   - a break may come before a character that is not a space when the
//     previous character is in wordWrapBreakAfterCharacters and this one
//     is not (after blanks, "/", ".", ...), when this one is in
//     wordWrapBreakBeforeCharacters and the previous one is not (before
//     "(", "[", ...), and before and after CJK ideographs, Hiragana and
//     Katakana (Monaco's BREAK_IDEOGRAPHIC: U+3040–U+30FF, U+3400–U+4DBF,
//     U+4E00–U+9FFF), unless that would put a break-after character at
//     the start of a row or leave a break-before one at the end;
//   - the last break point that keeps the row within the column is taken,
//     unless the rest of the text after it would not fit a continuation row
//     anyway; then the row breaks at the column (before the character that
//     overflows; a single character wider than the column gets a row of
//     its own);
//   - continuation rows are indented by wrappingIndent (the line's own
//     indentation for "same", one or two more tab stops for "indent" and
//     "deepIndent"), unless that leaves no room for a wide character.
//
// Deviations from Monaco: break points are grapheme cluster boundaries
// (Monaco works on UTF-16 code units and keeps only surrogate pairs
// together, so it may split a combining sequence); a cluster's class is
// that of its first rune, the class before a break that of the previous
// cluster's last rune, and runes above U+FFFF are never break points of
// their own (as Monaco's surrogate pairs). Tab stops on continuation rows
// count from the row's first character, as the row is drawn (Monaco counts
// them from the model line start, and renders the row from its indent).

// WrapMode is when lines wrap (Monaco's wordWrap option).
type WrapMode string

const (
	// WrapOff never wraps.
	WrapOff WrapMode = "off"
	// WrapOn wraps at the viewport width.
	WrapOn WrapMode = "on"
	// WrapColumn wraps at Options.WordWrapColumn.
	WrapColumn WrapMode = "wordWrapColumn"
	// WrapBounded wraps at the smaller of the viewport width and
	// Options.WordWrapColumn.
	WrapBounded WrapMode = "bounded"
)

// WrappingIndentMode is the indentation of continuation rows (Monaco's
// wrappingIndent option).
type WrappingIndentMode string

const (
	// WrapIndentNone starts continuation rows at column 0.
	WrapIndentNone WrappingIndentMode = "none"
	// WrapIndentSame indents them like the model line.
	WrapIndentSame WrappingIndentMode = "same"
	// WrapIndentIndent indents them one tab stop more than the line.
	WrapIndentIndent WrappingIndentMode = "indent"
	// WrapIndentDeep indents them two tab stops more than the line.
	WrapIndentDeep WrappingIndentMode = "deepIndent"
)

// DefaultWordWrapColumn is Monaco's wordWrapColumn default.
const DefaultWordWrapColumn = 80

// Monaco 0.56's wordWrapBreakBeforeCharacters and
// wordWrapBreakAfterCharacters defaults (editorOptions.ts).
const (
	wrapBreakBefore = "([{‘“〈《「『【〔（［｛｢£¥＄￡￥+＋"
	wrapBreakAfter  = " \t})]?|/&.,;¢°′″‰℃、。｡､￠，．：；？！％・･ゝゞヽヾーァィゥェォッャュョヮヵヶぁぃぅぇぉっゃゅょゎゕゖㇰㇱㇲㇳㇴㇵㇶㇷㇸㇹㇺㇻㇼㇽㇾㇿ々〻ｧｨｩｪｫｬｭｮｯｰ”〉》」』】〕）］｝｣"
)

// Character classes of the break rules (Monaco's CharacterClass).
const (
	wcNone = iota
	wcBreakBefore
	wcBreakAfter
	wcIdeographic
)

var wrapClasses = func() map[rune]uint8 {
	m := map[rune]uint8{}
	for _, r := range wrapBreakBefore {
		m[r] = wcBreakBefore
	}
	for _, r := range wrapBreakAfter {
		m[r] = wcBreakAfter
	}
	return m
}()

// wrapClass returns the class of r. As in Monaco's classifier, the
// ideographic ranges take precedence over the character lists above
// U+00FF (so "ー" and the small kana are ideographic).
func wrapClass(r rune) uint8 {
	if r >= 0x100 {
		if (r >= 0x3040 && r <= 0x30FF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF) {
			return wcIdeographic
		}
		if r > 0xFFFF {
			return wcNone
		}
	}
	return wrapClasses[r]
}

// canBreak is Monaco's canBreak (word break "normal"): whether a row may
// end between a character of class prev and r (class cur).
func canBreak(prev uint8, r rune, cur uint8) bool {
	return r != ' ' &&
		((prev == wcBreakAfter && cur != wcBreakAfter) ||
			(prev != wcBreakBefore && cur == wcBreakBefore) ||
			(prev == wcIdeographic && cur != wcBreakAfter) ||
			(cur == wcIdeographic && prev != wcBreakBefore))
}

// wrapConfig is how lines wrap: col is the wrapping column (<= 0: no
// wrapping), tab the tab size, indent the wrappingIndent.
type wrapConfig struct {
	col    int
	tab    int
	indent WrappingIndentMode
}

func (c wrapConfig) on() bool { return c.col > 0 }

// lineBreaks are the rows of a wrapped model line: starts are the byte
// offsets where rows 1.. start (empty: one row) and runes their rune
// indexes, indent the columns of the continuation rows' indentation.
type lineBreaks struct {
	starts []int
	runes  []int
	indent int
}

// rows returns the number of rows.
func (b lineBreaks) rows() int { return len(b.starts) + 1 }

// rowRange returns the byte range [b0, b1) of row k of a line of length n.
func (b lineBreaks) rowRange(k, n int) (int, int) {
	b0, b1 := 0, n
	if k > 0 {
		b0 = b.starts[k-1]
	}
	if k < len(b.starts) {
		b1 = b.starts[k]
	}
	return b0, b1
}

// rowRunes returns the rune range [r0, r1) of row k of a line of n runes.
func (b lineBreaks) rowRunes(k, n int) (int, int) {
	r0, r1 := 0, n
	if k > 0 {
		r0 = b.runes[k-1]
	}
	if k < len(b.runes) {
		r1 = b.runes[k]
	}
	return r0, r1
}

// rowOf returns the row of byte column col. A column at a row boundary is
// at the start of the later row, unless up (the caret drawn at the end of
// the earlier row, see State.caretUp).
func (b lineBreaks) rowOf(col int, up bool) int {
	k := 0
	for k < len(b.starts) && (col > b.starts[k] || (col == b.starts[k] && !up)) {
		k++
	}
	return k
}

// rowIndent returns the indentation columns of row k.
func (b lineBreaks) rowIndent(k int) int {
	if k == 0 {
		return 0
	}
	return b.indent
}

// wrappedIndent is Monaco's computeWrappedTextIndentLength.
func wrappedIndent(s string, tab, col int, mode WrappingIndentMode) int {
	if mode == WrapIndentNone {
		return 0
	}
	ws := len(s) - len(strings.TrimLeft(s, " \t"))
	if ws == len(s) {
		return 0 // blank line
	}
	n := 0
	for i := range ws {
		if s[i] == '\t' {
			n = (n/tab + 1) * tab
		} else {
			n++
		}
	}
	extra := 0
	switch mode {
	case WrapIndentIndent:
		extra = 1
	case WrapIndentDeep:
		extra = 2
	}
	for range extra {
		n = (n/tab + 1) * tab
	}
	if n+2 > col { // no room for a wide character after it
		return 0
	}
	return n
}

// wrapIndentMode returns m, or the default ("same") for unset or unknown
// values.
func wrapIndentMode(m WrappingIndentMode) WrappingIndentMode {
	switch m {
	case WrapIndentNone, WrapIndentIndent, WrapIndentDeep:
		return m
	}
	return WrapIndentSame
}

// computeBreaks returns the rows of line s wrapped at cfg (see the top of
// the file).
func computeBreaks(s string, cfg wrapConfig) lineBreaks {
	var lb lineBreaks
	if !cfg.on() || len(s) <= 1 {
		return lb
	}
	tab := max(cfg.tab, 1)
	lb.indent = wrappedIndent(s, tab, cfg.col, cfg.indent)
	cont := cfg.col - lb.indent // columns of a continuation row
	limit := cfg.col
	rowStart := 0
	vc := 0               // columns from the row start
	bo, boVC := -1, 0     // last break point in the row
	boRI := 0             // its rune index
	ri := 0               // rune index of p
	prev := uint8(wcNone) // class of the previous character
	for p := 0; p < len(s); {
		q := nextGrapheme(s, p)
		g := s[p:q]
		r, _ := utf8.DecodeRuneInString(g)
		cls := wrapClass(r)
		next := advanceCol(vc, g, tab)
		if p > rowStart && canBreak(prev, r, cls) {
			bo, boVC, boRI = p, vc, ri
		}
		if next > limit {
			at, atRI := bo, boRI
			if at < 0 || next-boVC > cont {
				at, atRI = p, ri // break at the column
				if p == rowStart {
					// A character wider than the row: a row of its own.
					at, atRI = q, ri+utf8.RuneCountInString(g)
				}
			}
			if at >= len(s) {
				break
			}
			lb.starts = append(lb.starts, at)
			lb.runes = append(lb.runes, atRI)
			// Continue from the new row's start.
			rowStart, limit, vc, bo = at, cont, 0, -1
			lr, _ := utf8.DecodeLastRuneInString(s[:at])
			prev = wrapClass(lr)
			p, ri = at, atRI
			continue
		}
		vc = next
		lr, _ := utf8.DecodeLastRuneInString(g)
		prev = wrapClass(lr)
		ri += utf8.RuneCountInString(g)
		p = q
	}
	return lb
}

// wrapColumn returns the wrapping column for mode in a text area textW
// pixels wide with columns colW wide (Monaco's EditorLayoutInfoComputer:
// the viewport column leaves 2 px for the caret; the editor has no
// vertical scrollbar to subtract). It is <= 0 when lines do not wrap.
func wrapColumn(mode WrapMode, column int, textW, colW float32) int {
	if column <= 0 {
		column = DefaultWordWrapColumn
	}
	viewport := 1
	if colW > 0 {
		viewport = max(1, int(math.Floor(float64((textW-2)/colW))))
	}
	switch mode {
	case WrapOn:
		return viewport
	case WrapColumn:
		return column
	case WrapBounded:
		return min(viewport, column)
	}
	return 0
}

// WordWrap returns the word wrap mode in effect: the one
// ToggleWordWrap set, else Options.WordWrap.
func (s *State) WordWrap() WrapMode {
	if s.wrapToggle != "" {
		return s.wrapToggle
	}
	return s.Options.wordWrap()
}

// ToggleWordWrap turns word wrap off when lines wrap, else on (to
// Options.WordWrap when that is not "off", else "on"), for this State
// only, overriding Options.WordWrap until toggled back (VS Code's
// editor.action.toggleWordWrap, Alt+Z). The View applies it from the next
// paint, keeping the top model line in place.
func (s *State) ToggleWordWrap() {
	opt := s.Options.wordWrap()
	next := WrapOff
	if s.WordWrap() == WrapOff {
		next = opt
		if opt == WrapOff {
			next = WrapOn
		}
	}
	s.wrapToggle = next
	if next == opt {
		s.wrapToggle = ""
	}
}
