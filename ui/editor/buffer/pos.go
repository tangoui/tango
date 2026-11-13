package buffer

import "unicode/utf8"

// Pos is a zero-based line/column position. Col counts UTF-8 bytes from the
// start of the line; use UTF16Col/ByteCol to convert to and from the UTF-16
// columns used by LSP and Monaco.
type Pos struct{ Line, Col int }

// PosOf returns the position of byte offset off. Out-of-range offsets are
// clamped to [0, Len()].
func (s Snapshot) PosOf(off int) Pos {
	off = clamp(off, 0, s.Len())
	line := s.LineOf(off)
	return Pos{Line: line, Col: off - s.LineStart(line)}
}

// OffsetOf returns the byte offset of p. The line is clamped to
// [0, LineCount()-1] and the column to [0, line length], so stale positions
// (for example from a language server) still map into the document. A column
// that falls inside a multi-byte character snaps back to the start of that
// character, so the result is always a valid edit boundary.
func (s Snapshot) OffsetOf(p Pos) int {
	line := clamp(p.Line, 0, s.LineCount()-1)
	start := s.LineStart(line)
	off := start + clamp(p.Col, 0, s.lineEnd(line)-start)
	// A UTF-8 character is at most utf8.UTFMax bytes, so this loops at most
	// three times.
	for off > start && s.splitsRune(off) {
		off--
	}
	return off
}

// UTF16Col converts byte column byteCol on line to a UTF-16 code-unit column.
// byteCol is clamped to the line; a column inside a multi-byte character
// counts only the characters that end at or before it. Invalid UTF-8 bytes
// count as one unit each.
func (s Snapshot) UTF16Col(line, byteCol int) int {
	// Read a few bytes past byteCol so a character straddling it decodes
	// whole instead of looking like invalid UTF-8.
	// byteCol is clamped to the line length first so huge inputs cannot
	// overflow the addition.
	text := s.linePrefix(line, min(max(byteCol, 0), s.lineLen(line))+utf8.UTFMax)
	byteCol = clamp(byteCol, 0, len(text))
	col := 0
	for i := 0; i < byteCol; {
		c := text[i]
		if c < utf8.RuneSelf {
			i++
			col++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if i+size > byteCol {
			break
		}
		i += size
		col += utf16Len(r)
	}
	return col
}

// ByteCol converts UTF-16 column utf16Col on line to a byte column. Columns
// past the end clamp to the line length; a column that falls between the two
// halves of a surrogate pair snaps back to the start of that character.
func (s Snapshot) ByteCol(line, utf16Col int) int {
	// Each UTF-16 unit takes at most 3 UTF-8 bytes, so only that much of the
	// line (plus one whole character) needs to be read; long minified lines
	// stay cheap. A line of n bytes has at most n UTF-16 units, so clamping
	// to the line length first keeps the multiplication from overflowing.
	text := s.linePrefix(line, 3*min(max(utf16Col, 0), s.lineLen(line))+utf8.UTFMax)
	col := 0
	i := 0
	for i < len(text) {
		c := text[i]
		if c < utf8.RuneSelf {
			if col+1 > utf16Col {
				break
			}
			i++
			col++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		n := utf16Len(r)
		if col+n > utf16Col {
			break
		}
		i += size
		col += n
	}
	return i
}

// linePrefix returns at most n bytes from the start of line (clamped to the
// document's lines), without its line break.
func (s Snapshot) linePrefix(line, n int) string {
	line = clamp(line, 0, s.LineCount()-1)
	start := s.LineStart(line)
	end := s.lineEnd(line)
	return s.slice(start, start+clamp(n, 0, end-start))
}

// lineLen returns the byte length of line (clamped to the document's lines),
// without its line break.
func (s Snapshot) lineLen(line int) int {
	line = clamp(line, 0, s.LineCount()-1)
	return s.lineEnd(line) - s.LineStart(line)
}

func utf16Len(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
