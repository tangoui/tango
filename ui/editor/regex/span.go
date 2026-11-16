package regex

import (
	"regexp/syntax"
	"time"
	"unicode"
	"unicode/utf8"
)

// Span plans are deliberately small. Unsupported patterns continue through the
// compiled regexp, including captures, assertions and ambiguous repetition.
const maxSpanSource = 512
const maxSpanAtoms = 64

type asciiSet [2]uint64

func (s *asciiSet) add(c rune) {
	if c >= 0 && c < utf8.RuneSelf {
		s[c/64] |= uint64(1) << (c % 64)
	}
}

func (s asciiSet) has(c byte) bool            { return s[c/64]&(uint64(1)<<(c%64)) != 0 }
func (s asciiSet) intersects(t asciiSet) bool { return s[0]&t[0] != 0 || s[1]&t[1] != 0 }

type spanAtom struct {
	set      asciiSet
	min, max int // -1 max means unlimited
}

type byteSpan struct {
	atoms []spanAtom
	end   bool
}

func runeSet(n *syntax.Regexp, r rune) asciiSet {
	var set asciiSet
	set.add(r)
	if n.Flags&syntax.FoldCase != 0 {
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			set.add(next)
		}
	}
	return set
}

func unitSet(n *syntax.Regexp) (asciiSet, bool) {
	for n.Op == syntax.OpCapture {
		n = n.Sub[0]
	}
	var set asciiSet
	switch n.Op {
	case syntax.OpLiteral:
		if len(n.Rune) != 1 {
			return set, false
		}
		return runeSet(n, n.Rune[0]), true
	case syntax.OpCharClass:
		for c := rune(0); c < utf8.RuneSelf; c++ {
			for i := 0; i < len(n.Rune); i += 2 {
				if c >= n.Rune[i] && c <= n.Rune[i+1] {
					set.add(c)
					break
				}
			}
		}
		return set, true
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		set = asciiSet{^uint64(0), ^uint64(0)}
		if n.Op == syntax.OpAnyCharNotNL {
			set[0] &^= uint64(1) << '\n'
		}
		return set, true
	}
	return set, false
}

// firstASCII is a necessary condition only: zero-width assertions are treated
// as nullable, so they can never cause a valid match to be rejected here.
func firstASCII(n *syntax.Regexp) (asciiSet, bool) {
	var set asciiSet
	switch n.Op {
	case syntax.OpNoMatch:
		return set, false
	case syntax.OpLiteral:
		if len(n.Rune) == 0 {
			return set, true
		}
		return runeSet(n, n.Rune[0]), false
	case syntax.OpCharClass, syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		set, _ = unitSet(n)
		return set, false
	case syntax.OpCapture:
		return firstASCII(n.Sub[0])
	case syntax.OpStar, syntax.OpQuest:
		set, _ = firstASCII(n.Sub[0])
		return set, true
	case syntax.OpPlus, syntax.OpRepeat:
		var nullable bool
		set, nullable = firstASCII(n.Sub[0])
		return set, nullable || n.Op == syntax.OpRepeat && n.Min == 0
	case syntax.OpConcat:
		for _, sub := range n.Sub {
			s, nullable := firstASCII(sub)
			set[0], set[1] = set[0]|s[0], set[1]|s[1]
			if !nullable {
				return set, false
			}
		}
		return set, true
	case syntax.OpAlternate:
		nullable := false
		for _, sub := range n.Sub {
			s, empty := firstASCII(sub)
			set[0], set[1] = set[0]|s[0], set[1]|s[1]
			nullable = nullable || empty
		}
		return set, nullable
	default:
		return set, true
	}
}

func compileSpan(source string) (*byteSpan, asciiSet, bool) {
	if len(source) > maxSpanSource {
		return nil, asciiSet{}, false
	}
	n, err := syntax.Parse(source, syntax.Perl)
	if err != nil {
		return nil, asciiSet{}, false
	}
	first, nullable := firstASCII(n)
	p := &byteSpan{}
	if !p.add(n) {
		return nil, first, !nullable
	}
	for i, a := range p.atoms {
		if a.min == a.max {
			continue
		}
		// Greedy repetition may be used directly only when no following
		// possible starting byte can require it to give characters back.
		for _, next := range p.atoms[i+1:] {
			if a.set.intersects(next.set) {
				return nil, first, !nullable
			}
			if next.min > 0 {
				break
			}
		}
	}
	return p, first, !nullable
}

func (p *byteSpan) add(n *syntax.Regexp) bool {
	if len(p.atoms) > maxSpanAtoms || n.Flags&syntax.NonGreedy != 0 {
		return false
	}
	switch n.Op {
	case syntax.OpCapture:
		return p.add(n.Sub[0])
	case syntax.OpConcat:
		for _, sub := range n.Sub {
			if !p.add(sub) {
				return false
			}
		}
		return len(p.atoms) <= maxSpanAtoms
	case syntax.OpEmptyMatch:
		return true
	case syntax.OpBeginText:
		return len(p.atoms) == 0 && !p.end
	case syntax.OpEndText:
		p.end = true
		return true
	case syntax.OpLiteral:
		if p.end || len(p.atoms)+len(n.Rune) > maxSpanAtoms {
			return false
		}
		for _, r := range n.Rune {
			p.atoms = append(p.atoms, spanAtom{set: runeSet(n, r), min: 1, max: 1})
		}
		return true
	case syntax.OpCharClass, syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		if p.end || len(p.atoms) == maxSpanAtoms {
			return false
		}
		set, _ := unitSet(n)
		p.atoms = append(p.atoms, spanAtom{set: set, min: 1, max: 1})
		return true
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		if p.end || len(p.atoms) == maxSpanAtoms {
			return false
		}
		set, ok := unitSet(n.Sub[0])
		if !ok {
			return false
		}
		a := spanAtom{set: set, min: n.Min, max: n.Max}
		switch n.Op {
		case syntax.OpStar:
			a.min, a.max = 0, -1
		case syntax.OpPlus:
			a.min, a.max = 1, -1
		case syntax.OpQuest:
			a.min, a.max = 0, 1
		}
		if a.max != 0 {
			p.atoms = append(p.atoms, a)
		}
		return true
	}
	return false
}

// supported is false as soon as Unicode or an invalid byte is encountered;
// the original engine then decides its meaning, including Unicode case folds.
func (p *byteSpan) match(s string) (end int, matched, supported bool) {
	pos := 0
	for _, a := range p.atoms {
		n := 0
		for pos < len(s) && (a.max < 0 || n < a.max) {
			c := s[pos]
			if c >= utf8.RuneSelf {
				return 0, false, false
			}
			if !a.set.has(c) {
				break
			}
			pos++
			n++
		}
		if n < a.min {
			return 0, false, true
		}
	}
	if p.end && pos != len(s) {
		return 0, false, true
	}
	return pos, true, true
}

func (r *stdRegexp) rejectASCII(s string) bool {
	return r.hasFirst && len(s) != 0 && s[0] < utf8.RuneSelf && !r.first.has(s[0])
}

// TryMatchStartNoCaptures returns only the overall byte span. dst is caller
// owned scratch storage and may be reused; ordinary Regexp methods keep their
// existing independent capture results. Unknown implementations use that API.
func TryMatchStartNoCaptures(re Regexp, s string, rs []rune, dst []int) ([]int, error) {
	switch r := re.(type) {
	case *stdRegexp:
		if r.rejectASCII(s) {
			return nil, nil
		}
		if r.span != nil {
			if end, matched, supported := r.span.match(s); supported {
				if !matched {
					return nil, nil
				}
				return append(dst[:0], 0, end), nil
			}
		}
		return r.re.FindStringIndex(s), nil
	case *backtrackRegexp:
		if r.rejectASCII(s) {
			return nil, nil
		}
		return r.tryMatchStartWithCaptures(s, rs, r.re.FindRunesMatch, time.Now, false, dst)
	default:
		m, err := re.TryMatchStart(s, rs)
		if m != nil {
			m = m[:2]
		}
		return m, err
	}
}
