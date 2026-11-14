package diff

import "sort"

// UnchangedRegion is a run of unchanged lines that a diff view may fold:
// Original and Modified have the same length and the same text (up to
// ignored whitespace).
type UnchangedRegion struct{ Original, Modified LineRange }

// UnchangedRegions returns the foldable unchanged regions between changes,
// keeping context unchanged lines visible next to every change. A region
// is only reported when it would hide at least minHidden lines; regions at
// the start or end of the texts need context lines on one side only.
// Monaco's diff editor uses context = 3 and minHidden = 3. originalLines and
// modifiedLines are the line counts of the texts.
func UnchangedRegions(changes []Change, originalLines, modifiedLines, context, minHidden int) []UnchangedRegion {
	context, minHidden = max(context, 0), max(minHidden, 1)
	var out []UnchangedRegion
	add := func(o, m, n int) {
		atStart := o == 0 && m == 0
		atEnd := o+n == originalLines && m+n == modifiedLines
		switch {
		case (atStart || atEnd) && n >= context+minHidden:
			if atStart && !atEnd {
				n -= context
			}
			if atEnd && !atStart {
				o += context
				m += context
				n -= context
			}
		case n >= 2*context+minHidden:
			o += context
			m += context
			n -= 2 * context
		default:
			return
		}
		out = append(out, UnchangedRegion{LineRange{o, o + n}, LineRange{m, m + n}})
	}
	o, m := 0, 0
	for _, ch := range changes {
		if n := ch.Modified.Start - m; n > 0 {
			add(o, m, n)
		}
		o, m = ch.Original.End, ch.Modified.End
	}
	if n := modifiedLines - m; n > 0 {
		add(o, m, n)
	}
	return out
}

// UnchangedRegions is UnchangedRegions for the changes and line counts of r.
func (r Result) UnchangedRegions(context, minHidden int) []UnchangedRegion {
	return UnchangedRegions(r.Changes, r.OriginalLines, r.ModifiedLines, context, minHidden)
}

// Next returns the index in r.Changes of the first change that starts
// after line on side s, wrapping around to the first change at the end of
// the text (and when line is the last line, so that repeated "next" never
// gets stuck on a change at the very end). ok is false when there are no
// changes.
func (r Result) Next(s Side, line int) (index int, ok bool) {
	if len(r.Changes) == 0 {
		return -1, false
	}
	count := r.OriginalLines
	if s == Modified {
		count = r.ModifiedLines
	}
	if line >= count-1 {
		return 0, true
	}
	i := sort.Search(len(r.Changes), func(i int) bool { return r.Changes[i].lines(s).Start > line })
	if i == len(r.Changes) {
		i = 0
	}
	return i, true
}

// Prev returns the index in r.Changes of the last change that starts
// before line on side s, wrapping around to the last change. ok is false
// when there are no changes.
func (r Result) Prev(s Side, line int) (index int, ok bool) {
	if len(r.Changes) == 0 {
		return -1, false
	}
	i := sort.Search(len(r.Changes), func(i int) bool { return r.Changes[i].lines(s).Start >= line }) - 1
	if i < 0 {
		i = len(r.Changes) - 1
	}
	return i, true
}
