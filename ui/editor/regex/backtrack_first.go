package regex

import (
	"unicode"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"github.com/dlclark/regexp2/syntax"
)

// Use the same parser, options and first-character analysis as the matching
// engine. Only a small immutable ASCII bitmap survives compilation. Nullable,
// unsupported and large expressions keep their original matching path.
func compileBacktrackFirst(source string, opts regexp2.RegexOptions) (asciiSet, bool) {
	if len(source) > maxSpanSource {
		return asciiSet{}, false
	}
	tree, err := syntax.Parse(source, syntax.RegexOptions(opts))
	if err != nil {
		return asciiSet{}, false
	}
	code, err := syntax.Write(tree)
	if err != nil || code.RightToLeft || code.FcPrefix == nil {
		return asciiSet{}, false
	}
	prefix := code.FcPrefix
	var first asciiSet
	for c := rune(0); c < utf8.RuneSelf; c++ {
		candidate := c
		if prefix.CaseInsensitive {
			candidate = unicode.ToLower(candidate)
		}
		if prefix.PrefixSet.CharIn(candidate) {
			first.add(c)
		}
	}
	return first, true
}

func (r *backtrackRegexp) rejectASCII(s string) bool {
	return r.hasFirst && len(s) != 0 && s[0] < utf8.RuneSelf && !r.first.has(s[0])
}
