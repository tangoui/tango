package editor

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Unicode highlight (Monaco's UnicodeHighlighter with its viewport
// highlighter, UnicodeTextModelHighlighter.computeUnicodeHighlights):
//
// Each drawn row's characters are checked as Monaco checks a line: a
// character in Options.UnicodeHighlight.AllowedCharacters is never marked;
// with NonBasicASCII every character outside basic ASCII (tab, line breaks,
// U+0020–U+007E) is; otherwise a character is a candidate when it is
// invisible (InvisibleCharacters: Monaco's InvisibleCharacters table but
// space and tab) or ambiguous (AmbiguousCharacters: Monaco's
// AmbiguousCharacters for the locales of AllowedLocales; unicode_locales.go,
// unicode_gen.go). A candidate
// inside a word (Monaco's DEFAULT_WORD_REGEXP: a run of characters that
// are neither the usual word separators nor white space) that has no basic
// ASCII character but some other character that is neither ambiguous nor
// invisible is not marked: "привет" is plainly Cyrillic, the "а" of
// "pаssword" is suspicious. A candidate in a comment token is not marked
// unless IncludeComments is on (Monaco's hideInCommentTokens), nor one in a
// string token when IncludeStrings is off.
//
// A marked character gets a box with a one pixel editorUnicodeHighlight
// .border and the editorUnicodeHighlight.background (Monaco's
// .unicode-highlight); a character without width gets a box two pixels
// wide (its borders). Monaco's hover and banner are not implemented; with more than 1000 marks in the
// drawn rows none are shown (paint.go). Long lines (more than
// longLineRunes runes) are not checked.

// unicodeOpts are the resolved unicode highlight options.
type unicodeOpts struct {
	nonBasic, ambiguous, invisible bool
	inComments, inStrings          bool
	allowed                        string
	amb                            *ambiguousSet // the locales' ambiguous characters
}

func (o *Options) unicodeOpts() unicodeOpts {
	u := o.UnicodeHighlight
	return unicodeOpts{
		nonBasic:   u.NonBasicASCII.on(),
		ambiguous:  u.AmbiguousCharacters,
		invisible:  u.InvisibleCharacters,
		inComments: u.IncludeComments.on(),
		inStrings:  u.IncludeStrings,
		allowed:    u.AllowedCharacters,
		amb:        u.AllowedLocales.ambiguous(),
	}
}

// any reports whether some character can be marked.
func (u unicodeOpts) any() bool { return u.nonBasic || u.ambiguous || u.invisible }

// confusable returns the basic character r can be confused with under
// the _default table (no locale of the data), false when r is not
// ambiguous there.
func confusable(r rune) (rune, bool) { return ambiguousFor("").confusable(r) }

// confusable returns the basic character r can be confused with, false
// when r is not ambiguous (nil: the _default table).
func (a *ambiguousSet) confusable(r rune) (rune, bool) {
	if a == nil {
		a = ambiguousFor("")
	}
	t := a.pairs
	n := len(t) / 2
	i := sort.Search(n, func(i int) bool { return t[2*i] >= uint32(r) })
	if i < n && t[2*i] == uint32(r) {
		return rune(t[2*i+1]), true
	}
	return 0, false
}

// isInvisible reports whether r is one of Monaco's invisible characters.
func isInvisible(r rune) bool {
	i := sort.Search(len(invisibleTable), func(i int) bool { return invisibleTable[i] >= uint32(r) })
	return i < len(invisibleTable) && invisibleTable[i] == uint32(r)
}

// isBasicASCII reports whether r is tab, a line break or U+0020–U+007E
// (Monaco's isBasicASCII).
func isBasicASCII(r rune) bool { return r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0x7e }

// wordRune reports whether r belongs to a word of Monaco's
// DEFAULT_WORD_REGEXP (not a usual word separator, not white space).
func wordRune(r rune) bool {
	if r < utf8.RuneSelf && strings.IndexByte(wordSeparators, byte(r)) >= 0 {
		return false
	}
	return !isJSSpace(r)
}

// mayNeedUnicode reports whether s has a character that may be marked
// (Monaco's mightContainNonBasicASCII, plus the ASCII controls).
func mayNeedUnicode(s string) bool {
	for i := 0; i < len(s); i++ {
		if b := s[i]; b >= utf8.RuneSelf || b < 0x20 && b != '\t' || b == 0x7f {
			return true
		}
	}
	return false
}

// unicodeReason is why a character is marked (Monaco's
// SimpleHighlightReason).
type unicodeReason uint8

const (
	unicodeNone unicodeReason = iota
	unicodeNonBasic
	unicodeInvisible
	unicodeAmbiguous
)

// unicodeMarks appends to dst the byte ranges of the characters of line s
// that are marked (see above); tokType returns the standard token type
// ("comment", "string", "regex" or "") at a byte of s, nil for none.
func unicodeMarks(s string, o unicodeOpts, tokType func(at int) string, dst []brange) []brange {
	if !o.any() || !mayNeedUnicode(s) {
		return dst
	}
	for i, r := range s {
		if isBasicASCII(r) {
			continue
		}
		if strings.ContainsRune(o.allowed, r) {
			continue
		}
		if !o.nonBasic && !(o.invisible && r != ' ' && r != '\t' && isInvisible(r)) {
			if _, amb := o.amb.confusable(r); !o.ambiguous || !amb {
				continue
			}
		}
		if unicodeReasonAt(s, i, r, o) == unicodeNone {
			continue
		}
		if tokType != nil {
			switch tokType(i) {
			case "comment":
				if !o.inComments {
					continue
				}
			case "string":
				if !o.inStrings {
					continue
				}
			}
		}
		dst = append(dst, brange{i, i + utf8.RuneLen(r)})
	}
	return dst
}

// unicodeReasonAt is Monaco's CodePointHighlighter.shouldHighlightNonBasicASCII
// for the character r at byte i of s, with its word as the context.
func unicodeReasonAt(s string, i int, r rune, o unicodeOpts) unicodeReason {
	if o.nonBasic {
		return unicodeNonBasic
	}
	if wordRune(r) {
		a, b := i, i+utf8.RuneLen(r)
		for a > 0 {
			q, n := utf8.DecodeLastRuneInString(s[:a])
			if !wordRune(q) {
				break
			}
			a -= n
		}
		for b < len(s) {
			q, n := utf8.DecodeRuneInString(s[b:])
			if !wordRune(q) {
				break
			}
			b += n
		}
		hasBasic, hasPlain := false, false
		for _, q := range s[a:b] {
			basic := isBasicASCII(q)
			hasBasic = hasBasic || basic
			if !basic {
				if _, amb := o.amb.confusable(q); !amb && !isInvisible(q) {
					hasPlain = true
				}
			}
		}
		if !hasBasic && hasPlain {
			return unicodeNone // plainly another script
		}
	}
	if o.invisible && r != ' ' && r != '\t' && r != '\n' && isInvisible(r) {
		return unicodeInvisible
	}
	if _, amb := o.amb.confusable(r); o.ambiguous && amb {
		return unicodeAmbiguous
	}
	return unicodeNone
}
