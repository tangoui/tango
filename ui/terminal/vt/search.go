package vt

import (
	"slices"
	"unicode"
)

// Match is a search hit. Line is the absolute line position (0 the oldest
// scrollback line, ScrollbackLen the first active row); [From, To) are the
// cell columns it spans, widened to whole wide characters.
type Match struct {
	Line     int
	From, To int
}

// Search returns every occurrence of query in the scrollback and the
// active area, oldest line first and left to right within a line.
// Matching is case-insensitive unless caseSensitive. An empty query
// returns nil. Soft-wrapped lines are searched individually; a query does
// not match across a wrap. The caller must hold the screen's lock.
func (s *Screen) Search(query string, caseSensitive bool) []Match {
	if query == "" {
		return nil
	}
	q := []rune(query)
	if !caseSensitive {
		q = foldRunes(q)
	}
	var hits []Match
	for abs, total := 0, s.sbLen+s.rows; abs < total; abs++ {
		l := s.at(abs)
		text, colOf := lineRunes(l)
		if len(text) < len(q) {
			continue
		}
		hay := text
		if !caseSensitive {
			hay = foldRunes(text)
		}
		for i := 0; i+len(q) <= len(hay); {
			if slices.Equal(hay[i:i+len(q)], q) {
				last := colOf[i+len(q)-1]
				hits = append(hits, Match{Line: abs, From: colOf[i], To: last + cellWidth(l, last)})
				i += len(q)
			} else {
				i++
			}
		}
	}
	return hits
}

// lineRunes returns the line's base characters (a blank cell is a space,
// the right half of a wide character is skipped) and the cell column of
// each. Trailing blanks are dropped so empty padding does not match.
func lineRunes(l *Line) (text []rune, colOf []int) {
	for x, c := range l.Cells {
		if c.Width == 0 {
			continue
		}
		r := c.Rune
		if r == 0 {
			r = ' '
		}
		text = append(text, r)
		colOf = append(colOf, x)
	}
	for len(text) > 0 && text[len(text)-1] == ' ' {
		text = text[:len(text)-1]
		colOf = colOf[:len(colOf)-1]
	}
	return text, colOf
}

func cellWidth(l *Line, x int) int {
	if x < len(l.Cells) && l.Cells[x].Width > 0 {
		return int(l.Cells[x].Width)
	}
	return 1
}

func foldRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
}
