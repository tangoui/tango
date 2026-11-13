package when

import (
	"slices"
)

// Implies reports whether p provably implies q, Monaco's implies (used by
// keybinding removal rules, KeybindingResolver.whenIsEntirelyIncluded): a
// nil q (no when, always true) is implied by everything; a nil p implies
// only a nil q. Expressions are compared after flattening parentheses and
// nested && / || and ignoring term order; each term (a key, a comparison,
// a negation) compares by its String. Monaco's further normalizations
// (constant folding, distributing && over ||) are not applied.
//
//	Implies("a && b", "a")        true
//	Implies("b && a", "(a && b)") true
//	Implies("a", "a && b")        false
//	Implies("a", "a || b")        true
//	Implies("a || b", "a")        false
func Implies(p, q Expr) bool {
	if q == nil {
		return true
	}
	if p == nil {
		return false
	}
	return implies(normalize(p), normalize(q))
}

// normalize strips parentheses and flattens nested && and || into one
// level, terms sorted by String and deduplicated.
func normalize(e Expr) Expr {
	switch x := e.(type) {
	case parenExpr:
		return normalize(x.e)
	case andExpr:
		var out andExpr
		for _, t := range x {
			n := normalize(t)
			if a, ok := n.(andExpr); ok {
				out = append(out, a...)
			} else {
				out = append(out, n)
			}
		}
		return andExpr(sortTerms(out))
	case orExpr:
		var out orExpr
		for _, t := range x {
			n := normalize(t)
			if o, ok := n.(orExpr); ok {
				out = append(out, o...)
			} else {
				out = append(out, n)
			}
		}
		return orExpr(sortTerms(out))
	case notExpr:
		return notExpr{normalize(x.e)}
	}
	return e
}

func sortTerms(ts []Expr) []Expr {
	slices.SortFunc(ts, func(a, b Expr) int {
		sa, sb := a.String(), b.String()
		switch {
		case sa < sb:
			return -1
		case sa > sb:
			return 1
		}
		return 0
	})
	return slices.CompactFunc(ts, func(a, b Expr) bool { return a.String() == b.String() })
}

func isConst(e Expr, v bool) bool {
	c, ok := e.(constExpr)
	return ok && bool(c) == v
}

// implies is Monaco's implies over normalized expressions.
func implies(p, q Expr) bool {
	if isConst(p, false) || isConst(q, true) {
		return true
	}
	if po, ok := p.(orExpr); ok {
		if qo, ok := q.(orExpr); ok {
			return allIncluded(po, qo)
		}
		return false
	}
	if qo, ok := q.(orExpr); ok {
		for _, t := range qo {
			if implies(p, t) {
				return true
			}
		}
		return false
	}
	if pa, ok := p.(andExpr); ok {
		if qa, ok := q.(andExpr); ok {
			return allIncluded(qa, pa)
		}
		for _, t := range pa {
			if implies(t, q) {
				return true
			}
		}
		return false
	}
	return p.String() == q.String()
}

// allIncluded reports whether every term of a is a term of b.
func allIncluded(a, b []Expr) bool {
	for _, t := range a {
		s := t.String()
		if !slices.ContainsFunc(b, func(u Expr) bool { return u.String() == s }) {
			return false
		}
	}
	return true
}
