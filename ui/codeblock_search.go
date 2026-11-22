package ui

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// CodeBlockMatch is a half-open UTF-8 byte range in the code after CRLF and
// CR line endings have been normalized to LF.
type CodeBlockMatch struct {
	Start, End int
}

type codeBlockSearch struct {
	query     string
	ver       uint64
	sensitive bool
	matches   []CodeBlockMatch
	active    int
	offset    int
	width     float32
	pending   bool
}

// Search highlights non-overlapping literal matches throughout the code.
// An empty query clears the highlights. Use SearchMatches to read the ranges.
func (w *CodeBlockW) Search(query string, caseSensitive bool) *CodeBlockW {
	w.query, w.sensitive = query, caseSensitive
	return w
}

// ActiveMatch marks and reveals the zero-based search result at index when
// the index or result set changes. Pass -1 to clear the active highlight.
func (w *CodeBlockW) ActiveMatch(index int) *CodeBlockW {
	w.match = index
	return w
}

// Reveal scrolls to a UTF-8 byte offset in the normalized code. Call this
// modifier in response to a navigation action. Offsets outside the document
// are clamped; offsets inside a rune move to its start.
func (w *CodeBlockW) Reveal(offset int) *CodeBlockW {
	w.offset, w.reveal = offset, true
	return w
}

// SearchMatches returns a copy of the matches in document order.
func (w *CodeBlockW) SearchMatches() []CodeBlockMatch {
	w.z.run()
	return slices.Clone(w.st.search.matches)
}

func (w *CodeBlockW) configureSearch(v *txView) {
	st := w.st
	s := &st.search
	if s.ver != st.ver || s.query != w.query || s.sensitive != w.sensitive {
		*s = codeBlockSearch{ver: st.ver, query: w.query, sensitive: w.sensitive, active: -1}
		if w.query != "" {
			q := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(w.query)
			if w.sensitive {
				for from := 0; from < len(st.code); {
					i := strings.Index(st.code[from:], q)
					if i < 0 {
						break
					}
					start := from + i
					from = start + len(q)
					s.matches = append(s.matches, CodeBlockMatch{start, from})
				}
			} else if re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(q)); err == nil {
				for _, m := range re.FindAllStringIndex(st.code, -1) {
					s.matches = append(s.matches, CodeBlockMatch{m[0], m[1]})
				}
			}
		}
	}
	if w.match != s.active {
		s.active = w.match
		if w.match >= 0 && w.match < len(s.matches) {
			s.offset, s.pending = s.matches[w.match].Start, true
		}
	}
	if w.reveal {
		s.offset, s.pending = w.offset, true
	}
	if len(s.matches) > 0 {
		v.match, v.matchC, v.matchB = w.c.Theme.Warning.Bg, w.c.Theme.Warning.BgHover, w.c.Theme.Warning.Border
		v.marks = func(line int, dst []txvMark) []txvMark {
			start, end := st.starts[line], st.starts[line]+len(st.line(line))
			i := sort.Search(len(s.matches), func(i int) bool { return s.matches[i].End > start })
			for ; i < len(s.matches) && s.matches[i].Start < end; i++ {
				m := s.matches[i]
				dst = append(dst, txvMark{max(m.Start, start) - start, min(m.End, end) - start, i == s.active})
			}
			return dst
		}
	}
	v.fullLine = len(s.matches) > 0 || w.reveal || s.pending || s.width > 0
	if s.pending || s.active >= 0 && s.active < len(s.matches) {
		v.reveal = func() {
			if !s.pending && s.width == v.st.view.W {
				return
			}
			revealCodeOffset(v, st, s.offset)
			s.width, s.pending = v.st.view.W, v.st.view.W <= 0
		}
	}
}

func revealCodeOffset(v *txView, st *codeBlockState, offset int) {
	offset = max(0, min(offset, len(st.code)))
	for offset > 0 && offset < len(st.code) && !utf8.RuneStart(st.code[offset]) {
		offset--
	}
	i := sort.Search(len(st.starts), func(i int) bool { return st.starts[i] > offset }) - 1
	col := offset - st.starts[i]
	row := v.lineRow(i)
	if v.wrap && v.cells(i) > v.st.wrapCols {
		a := st.wrapBoundaries(i, v.st.wrapCols)
		seg := sort.Search(len(a), func(k int) bool { return a[k].byteOffset > col }) - 1
		row += min(seg, v.lineRow(i+1)-row-1)
	}
	v.revealRow(row)
	v.revealCol(i, col)
}
