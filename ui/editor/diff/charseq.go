package diff

import (
	"sort"
	"strings"
	"unicode"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// charSeq is the text of a range of lines as runes, with '\n' between the
// lines, used for character-level refinement. When whitespace changes are
// ignored, each line is trimmed first.
type charSeq struct {
	elems []int32
	// cols[k] is the byte column (in its source line) of element k; for a
	// '\n' element and for the final sentinel cols[len(elems)] it is the
	// column just past the line's included text.
	cols []int32
	// first[i] is the element offset where line startLine+i begins.
	first     []int
	startLine int
	// startCol[i] is the byte column where line startLine+i enters the
	// sequence (non-zero only for the first line); it is the column of the
	// line's left edge before trimmed white space.
	startCol []int
}

func newCharSeq(lines []string, r Range, considerWS bool) *charSeq {
	s := &charSeq{startLine: r.Start.Line}
	// Bytes bound runes from above, so this capacity avoids regrowth.
	size := r.End.Line - r.Start.Line + 1
	for ln := r.Start.Line; ln <= r.End.Line; ln++ {
		size += len(lines[ln])
	}
	s.elems = make([]int32, 0, size)
	s.cols = make([]int32, 0, size+1)
	for ln := r.Start.Line; ln <= r.End.Line; ln++ {
		line := lines[ln]
		lineStart := 0
		if ln == r.Start.Line && r.Start.Col > 0 {
			lineStart = min(r.Start.Col, len(line))
			line = line[lineStart:]
		}
		s.first = append(s.first, len(s.elems))
		s.startCol = append(s.startCol, lineStart)
		trimmedWS := 0
		if !considerWS {
			trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
			trimmedWS = len(line) - len(trimmed)
			line = strings.TrimRightFunc(trimmed, unicode.IsSpace)
		}
		limit := len(line)
		if ln == r.End.Line {
			limit = max(0, min(r.End.Col-lineStart-trimmedWS, len(line)))
		}
		base := int32(lineStart + trimmedWS)
		end := limit
		for i, c := range line {
			if i >= limit {
				end = i
				break
			}
			s.elems = append(s.elems, c)
			s.cols = append(s.cols, base+int32(i))
		}
		if ln < r.End.Line {
			s.elems = append(s.elems, '\n')
			s.cols = append(s.cols, base+int32(end))
		} else {
			s.cols = append(s.cols, base+int32(end))
		}
	}
	return s
}

func (s *charSeq) elements() []int32 { return s.elems }

func (s *charSeq) stronglyEqual(i, j int) bool { return s.elems[i] == s.elems[j] }

func (s *charSeq) text(r span) string { return string(s.elems[r.start:r.end]) }

// trimmedLen returns the number of elements in r once leading and trailing
// white space is trimmed, or some value above limit if it exceeds limit.
func (s *charSeq) trimmedLen(r span, limit int) int {
	start, end := r.start, r.end
	for start < end && unicode.IsSpace(s.elems[start]) {
		start++
	}
	for end > start && unicode.IsSpace(s.elems[end-1]) {
		end--
	}
	return min(end-start, limit+1)
}

// lineIndex returns the index of the line containing element offset off.
func (s *charSeq) lineIndex(off int) int {
	return sort.Search(len(s.first), func(i int) bool { return s.first[i] > off }) - 1
}

// pos translates element offset off to a text position. At the very start
// of a line, preferLeft keeps trimmed leading white space outside the
// position.
func (s *charSeq) pos(off int, preferLeft bool) buffer.Pos {
	i := s.lineIndex(off)
	if preferLeft && off == s.first[i] {
		return buffer.Pos{Line: s.startLine + i, Col: s.startCol[i]}
	}
	return buffer.Pos{Line: s.startLine + i, Col: int(s.cols[off])}
}

func (s *charSeq) translate(r span) Range {
	start := s.pos(r.start, false)
	end := s.pos(r.end, true)
	if posLess(end, start) {
		return Range{end, end}
	}
	return Range{start, end}
}

func posLess(a, b buffer.Pos) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col
}

func (s *charSeq) countLines(r span) int { return s.lineIndex(r.end) - s.lineIndex(r.start) }

// fullLines widens r to the start of its first line and past the line break
// of its last line.
func (s *charSeq) fullLines(r span) span {
	start := 0
	if i := s.lineIndex(r.start); i >= 0 {
		start = s.first[i]
	}
	end := len(s.elems)
	if j := sort.SearchInts(s.first, r.end); j < len(s.first) {
		end = s.first[j]
	}
	return span{start, end}
}

func isWordChar(c int32) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func isUpper(c int32) bool { return c >= 'A' && c <= 'Z' }

// word returns the ASCII word containing offset off.
func (s *charSeq) word(off int) (span, bool) {
	if off < 0 || off >= len(s.elems) || !isWordChar(s.elems[off]) {
		return span{}, false
	}
	start, end := off, off
	for start > 0 && isWordChar(s.elems[start-1]) {
		start--
	}
	for end < len(s.elems) && isWordChar(s.elems[end]) {
		end++
	}
	return span{start, end}, true
}

// subWord returns the camelCase part of a word containing offset off
// ("fooBar" has the sub-words "foo" and "Bar").
func (s *charSeq) subWord(off int) (span, bool) {
	if off < 0 || off >= len(s.elems) || !isWordChar(s.elems[off]) {
		return span{}, false
	}
	start, end := off, off
	for start > 0 && isWordChar(s.elems[start-1]) && !isUpper(s.elems[start]) {
		start--
	}
	for end < len(s.elems) && isWordChar(s.elems[end]) && !isUpper(s.elems[end]) {
		end++
	}
	return span{start, end}, true
}

// Character classes for boundary scoring.
const (
	catWordLower = iota
	catWordUpper
	catWordNumber
	catEnd
	catOther
	catSeparator
	catSpace
	catCR
	catLF
)

var catScore = [...]int{
	catWordLower: 0, catWordUpper: 0, catWordNumber: 0, catEnd: 10, catOther: 2,
	catSeparator: 30, catSpace: 3, catCR: 10, catLF: 10,
}

func category(c int32) int {
	switch {
	case c == '\n':
		return catLF
	case c == '\r':
		return catCR
	case c == ' ' || c == '\t':
		return catSpace
	case c >= 'a' && c <= 'z':
		return catWordLower
	case c >= 'A' && c <= 'Z':
		return catWordUpper
	case c >= '0' && c <= '9':
		return catWordNumber
	case c == -1:
		return catEnd
	case c == ',' || c == ';':
		return catSeparator
	}
	return catOther
}

// boundaryScore rates the gap before element gap as a place for a change
// boundary: line starts best, then separators, spaces and word edges.
func (s *charSeq) boundaryScore(gap int) int {
	prev, next := int32(-1), int32(-1)
	if gap > 0 {
		prev = s.elems[gap-1]
	}
	if gap < len(s.elems) {
		next = s.elems[gap]
	}
	pc, nc := category(prev), category(next)
	if pc == catCR && nc == catLF {
		return 0 // never between \r and \n
	}
	if pc == catLF {
		return 150 // prefer the line break before the change
	}
	score := 0
	if pc != nc {
		score += 10
		if pc == catWordLower && nc == catWordUpper {
			score++
		}
	}
	return score + catScore[pc] + catScore[nc]
}
