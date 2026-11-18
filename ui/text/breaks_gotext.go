//go:build !darwin || ios || tango_gotext

package text

import "github.com/go-text/typesetting/segmenter"

// breaker is reused by lineEnds; it is only used under mu (from layout).
var breaker segmenter.Segmenter

// lineEnds returns the UAX #14 line break opportunities of runes[a:b]
// (go-text's segmenter): rune indices a break may come before, ascending,
// ending with b.
func lineEnds(runes []rune, a, b int) []int {
	breaker.Init(runes[a:b])
	it := breaker.LineIterator()
	var ends []int
	for it.Next() {
		ln := it.Line()
		ends = append(ends, a+ln.Offset+len(ln.Text))
	}
	return ends
}
