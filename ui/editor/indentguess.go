package editor

import (
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Indentation detection: a port of Monaco 0.56's guessIndentation
// (editor/common/model/indentationGuesser.js), which a new model runs when
// its creation option detectIndentation is on (TextModel.resolveOptions)
// and editor.action.detectIndentation runs on demand.

// guessMaxLines is how many lines from the top guessIndentation looks at.
const guessMaxLines = 10000

// guessIndentation scans the lines in windows of the text. A window
// starts at guessWindowMin bytes and doubles, up to guessWindow, while the
// lines fit in it; a line running past the window is skipped with one line
// lookup and the next window starts small again. So a run of short lines
// is read in few slices, and each line longer than the window costs at
// most one window, from guessWindowMin bytes (after another long line) to
// guessWindow (after short lines): 10000 lines read at most about
// 10000 * guessWindowMin bytes plus the short lines themselves.
const (
	guessWindowMin = 1 << 10
	guessWindow    = 64 << 10
)

// guessTabSizes are the tab sizes guessIndentation picks from, even ones
// first (Monaco's ALLOWED_TAB_SIZE_GUESSES).
var guessTabSizes = [...]int{2, 4, 6, 8, 3, 5, 7}

// guessLine is a line seen by GuessIndentation: its byte range in the
// snapshot and its indentation (the spaces and tabs before its first other
// character).
type guessLine struct {
	start, end int
	ind        string
}

// GuessIndentation guesses the tab size and whether the document indents
// with spaces from the indentation of its first 10000 lines, as Monaco
// 0.56's guessIndentation: lines indented with tabs against lines indented
// with two or more spaces decide insertSpaces (a tie keeps
// defaultInsertSpaces); with spaces, the most frequent difference in
// spaces between consecutive non-blank lines among 2, 4, 6, 8, 3, 5, 7
// (in that order of preference) is the tab size, with 2 winning over 4
// when it occurs at least 2/3 as often. Indentation that looks like
// alignment (a continuation under a line ending in a comma) is ignored
// unless its width is defaultTabSize and defaultInsertSpaces is set.
// With tabs, or nothing to go by, the defaults are returned.
//
// Only the indentation of each line is read (plus, for the alignment
// check, a few characters of the previous line) through a bounded window,
// so a long line costs about as much as a short one (at most one window
// of a few KB; see guessWindow), not its length.
func GuessIndentation(snap buffer.Snapshot, defaultTabSize int, defaultInsertSpaces bool) (tabSize int, insertSpaces bool) {
	lines := min(snap.LineCount(), guessMaxLines)
	tabLines := 0        // lines with at least one tab in their indentation
	spaceLines := 0      // lines indented with two or more spaces only
	var prev guessLine   // the last line with content; zero before the first
	var diffCount [9]int // spacesDiffCount: tab size scores, index 0..8

	r := guessReader{snap: snap}
	for i := 0; i < lines; i++ {
		cur, ok := r.line(i)
		if !ok { // empty or only whitespace
			continue
		}
		tabs := 0
		for j := 0; j < len(cur.ind); j++ {
			if cur.ind[j] == '\t' {
				tabs++
			}
		}
		if tabs > 0 {
			tabLines++
		} else if len(cur.ind) > 1 {
			spaceLines++
		}

		diff, alignment := guessSpacesDiff(snap, prev, cur)
		if alignment && !(defaultInsertSpaces && defaultTabSize == diff) {
			continue
		}
		if diff <= 8 {
			diffCount[diff]++
		}
		prev = cur
	}

	insertSpaces = defaultInsertSpaces
	if tabLines != spaceLines {
		insertSpaces = tabLines < spaceLines
	}
	tabSize = defaultTabSize
	if insertSpaces {
		score := 0
		for _, n := range guessTabSizes {
			if diffCount[n] > score {
				score = diffCount[n]
				tabSize = n
			}
		}
		// 2 wins over 4 when it has at least 2/3 of 4's occurrences
		// (YAML with some deeper nesting).
		if tabSize == 4 && diffCount[4] > 0 && diffCount[2] > 0 && 3*diffCount[2] >= 2*diffCount[4] {
			tabSize = 2
		}
	}
	return tabSize, insertSpaces
}

// guessReader reads the lines of a snapshot in order, through a window
// of the text.
type guessReader struct {
	snap buffer.Snapshot
	next int    // start of the next line
	win  string // text from offset base
	base int
	size int // bytes of the next window; 0 means guessWindowMin
}

// line returns line i, the line after the previous call's (0 first), with
// its indentation, and whether it has a character other than a space or
// tab.
func (r *guessReader) line(i int) (guessLine, bool) {
	snap := r.snap
	l := guessLine{start: r.next, end: -1}
	if l.start >= r.base+len(r.win) {
		if r.size == 0 {
			r.size = guessWindowMin
		}
		r.base, r.win = l.start, snap.Slice(l.start, min(snap.Len(), l.start+r.size))
		r.size = min(2*r.size, guessWindow)
	}
	rest := r.win[l.start-r.base:]
	if j := strings.IndexByte(rest, '\n'); j >= 0 {
		l.end = l.start + j
		rest = rest[:j]
	} else if r.base+len(r.win) == snap.Len() {
		l.end = snap.Len()
	}
	if l.end < 0 { // the line goes on past the window
		r.size = guessWindowMin // the next line may be long too
		l.end = snap.Len()
		if i+1 < snap.LineCount() {
			l.end = snap.LineStart(i+1) - 1
		}
	}
	r.next = l.end + 1

	j := 0
	for j < len(rest) && (rest[j] == ' ' || rest[j] == '\t') {
		j++
	}
	if j < len(rest) {
		l.ind = rest[:j]
		return l, true
	}
	// Whitespace up to the end of the window: read on (indentation longer
	// than the window; Monaco scans it all too).
	for pos := l.start + j; pos < l.end; {
		chunk := snap.Slice(pos, min(l.end, pos+guessWindow))
		for k := 0; k < len(chunk); k++ {
			if c := chunk[k]; c != ' ' && c != '\t' {
				l.ind = snap.Slice(l.start, pos+k)
				return l, true
			}
		}
		pos += len(chunk)
	}
	return l, false
}

// guessSpacesDiff is Monaco's spacesDiff between the indentation of the
// previous content line a and the current line b: the difference in
// spaces (or in spaces per tab, when the tab counts differ and divide the
// space difference) after their common prefix, 0 when either side mixes
// tabs and spaces there, and whether b's extra spaces look like alignment
// under a line ending in a comma. Character indexes are UTF-16 code units
// as in Monaco; the indentations are ASCII, so they equal byte indexes up
// to the first content character of a line.
func guessSpacesDiff(snap buffer.Snapshot, a, b guessLine) (diff int, alignment bool) {
	i := 0
	for i < len(a.ind) && i < len(b.ind) && a.ind[i] == b.ind[i] {
		i++
	}
	aSpaces, aTabs := countSpacesTabs(a.ind[i:])
	bSpaces, bTabs := countSpacesTabs(b.ind[i:])
	if aSpaces > 0 && aTabs > 0 || bSpaces > 0 && bTabs > 0 {
		return 0, false
	}
	tabsDiff := absInt(aTabs - bTabs)
	spacesDiff := absInt(aSpaces - bSpaces)
	if tabsDiff == 0 {
		// Monaco indexes both lines by bSpaces here, b's spaces after the
		// common prefix. b.charCodeAt(bSpaces) is in b's indentation or
		// is its first content character (never a space); bSpaces <
		// b.length always holds since b has content.
		if spacesDiff > 0 && bSpaces >= 1 && a.end > a.start &&
			(bSpaces == len(b.ind) || b.ind[bSpaces] != ' ') &&
			utf16SpaceAt(snap, a, bSpaces-1) &&
			snap.Slice(a.end-1, a.end) == "," { // a.charCodeAt(a.length-1) === ','
			alignment = true
		}
		return spacesDiff, alignment
	}
	if spacesDiff%tabsDiff == 0 {
		return spacesDiff / tabsDiff, false
	}
	return 0, false
}

func countSpacesTabs(ind string) (spaces, tabs int) {
	for j := 0; j < len(ind); j++ {
		if ind[j] == ' ' {
			spaces++
		} else {
			tabs++
		}
	}
	return spaces, tabs
}

// utf16SpaceAt reports whether line l's UTF-16 code unit k exists and is a
// space (Monaco's k < a.length && a.charCodeAt(k) === 32). It reads only
// as far as unit k.
func utf16SpaceAt(snap buffer.Snapshot, l guessLine, k int) bool {
	if k < len(l.ind) {
		return l.ind[k] == ' '
	}
	// Past the indentation: decode from the first content character. A
	// code unit takes at most 3 bytes, so 3 bytes per unit still needed
	// cover unit k whole.
	from := l.start + len(l.ind)
	need := k - len(l.ind) + 1
	s := snap.Slice(from, min(l.end, from+3*need))
	unit := len(l.ind)
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if unit == k {
			return r == ' '
		}
		unit++
		if r >= 0x10000 { // a surrogate pair: unit k may be its low half
			if unit == k {
				return false
			}
			unit++
		}
		s = s[size:]
	}
	return false
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
