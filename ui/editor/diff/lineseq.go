package diff

import (
	"strings"
	"unicode"
)

// sequence is what the shifting heuristics need from a sequence: its
// elements, how good a place each gap is for a change boundary, and exact
// equality of two elements (elements may compare equal by hash while their
// text differs, e.g. in whitespace).
type sequence interface {
	elements() []int32
	boundaryScore(gap int) int
	stronglyEqual(i, j int) bool
}

// lineSeq is a run of lines, compared by the hash of their trimmed text.
type lineSeq struct {
	hashes []int32
	lines  []string
}

func (s *lineSeq) elements() []int32 { return s.hashes }

// boundaryScore prefers gaps next to less indented lines, so that an added
// block of code starts and ends at its own indentation level.
func (s *lineSeq) boundaryScore(gap int) int {
	before, after := 0, 0
	if gap > 0 {
		before = indentation(s.lines[gap-1])
	}
	if gap < len(s.lines) {
		after = indentation(s.lines[gap])
	}
	return 1000 - (before + after)
}

func (s *lineSeq) stronglyEqual(i, j int) bool { return s.lines[i] == s.lines[j] }

func indentation(line string) int {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

// hasher numbers distinct strings densely, so lines compare as int32.
type hasher map[string]int32

func (h hasher) hash(s string) int32 {
	v, ok := h[s]
	if !ok {
		v = int32(len(h))
		h[s] = v
	}
	return v
}

// trimSpace trims like JavaScript's String.prototype.trim (Unicode white
// space), with a fast path for the common already-trimmed ASCII line.
func trimSpace(s string) string {
	if s == "" {
		return s
	}
	if first, last := s[0], s[len(s)-1]; first < 0x80 && last < 0x80 && !asciiSpace(first) && !asciiSpace(last) {
		return s
	}
	return strings.TrimFunc(s, unicode.IsSpace)
}

func asciiSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// trimEqual reports whether a and b are equal after trimming.
func trimEqual(a, b string) bool { return a == b || trimSpace(a) == trimSpace(b) }

// nonSpaceLen returns the number of non-white-space runes in s, stopping
// once it exceeds limit.
func nonSpaceLen(s string, limit int) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
			if n > limit {
				break
			}
		}
	}
	return n
}
