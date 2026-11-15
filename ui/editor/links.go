package editor

import (
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Links (Monaco's LinkComputer with the LinkDetector contribution):
//
// With Options.Links, the URLs of each drawn row's model line are found by
// Monaco's LinkComputer rules: a link starts with "http://", "https://" or
// "file://" (any case) and runs until a character that forces its end (a
// blank, <, >, quotes, |, the CJK punctuation of Monaco's list; a closing
// bracket without its opening one inside the link; a quote, backtick or
// asterisk matching the character before the link); it never ends in
// ".", ",", ";" or ":", and a link wrapped in (), [] or {} leaves the
// closing bracket out. Links are underlined in their text color (Monaco's
// .detected-link). While the pointer is over a link with the trigger
// modifier held (Cmd on macOS, Ctrl elsewhere: Monaco's multiCursorModifier
// "alt"), the link takes editorLink.activeForeground and the pointer
// becomes a hand; a press and release on it with the modifier hands its URL
// to the function set with ViewW.OnOpenLink (Monaco opens it through its
// opener service). The click also places the caret, as any click does.
//
// Long lines (more than longLineRunes runes) are not searched. Models over
// Monaco's worker sync limit have no links (largefile.go).

// linkSpan is a link of a line: its bytes and its URL.
type linkSpan struct {
	a, b int
	url  string
}

// Link character classes (Monaco's CharacterClass).
const (
	linkNone = iota
	linkForceEnd
	linkCannotEnd
)

// linkForceEndChars are Monaco's FORCE_TERMINATION_CHARACTERS.
const linkForceEndChars = " \t<>'\"、。｡､，．：；‘〈「『〔（［｛｢｣｝］）〕』」〉’｀～…|"

func linkClass(r rune) int {
	switch {
	case strings.ContainsRune(".,;:", r):
		return linkCannotEnd
	case strings.ContainsRune(linkForceEndChars, r):
		return linkForceEnd
	}
	return linkNone
}

// Link scanner states (Monaco's linkComputer State).
const (
	lsInvalid = iota
	lsStart
	lsH
	lsHT
	lsHTT
	lsHTTP
	lsF
	lsFI
	lsFIL
	lsBeforeColon
	lsAfterColon
	lsAlmostThere
	lsEnd
	lsAccept
)

// linkNext is Monaco's link state machine.
func linkNext(st int, r rune) int {
	lower := r | 0x20
	switch st {
	case lsStart:
		switch lower {
		case 'h':
			return lsH
		case 'f':
			return lsF
		}
	case lsH:
		if lower == 't' {
			return lsHT
		}
	case lsHT:
		if lower == 't' {
			return lsHTT
		}
	case lsHTT:
		if lower == 'p' {
			return lsHTTP
		}
	case lsHTTP:
		if lower == 's' {
			return lsBeforeColon
		}
		if r == ':' {
			return lsAfterColon
		}
	case lsF:
		if lower == 'i' {
			return lsFI
		}
	case lsFI:
		if lower == 'l' {
			return lsFIL
		}
	case lsFIL:
		if lower == 'e' {
			return lsBeforeColon
		}
	case lsBeforeColon:
		if r == ':' {
			return lsAfterColon
		}
	case lsAfterColon:
		if r == '/' {
			return lsAlmostThere
		}
	case lsAlmostThere:
		if r == '/' {
			return lsEnd
		}
	}
	return lsInvalid
}

// computeLinks appends the links of line to dst (Monaco's
// LinkComputer.computeLinks on one line).
func computeLinks(line string, dst []linkSpan) []linkSpan {
	if !strings.Contains(line, "://") {
		return dst
	}
	st := lsStart
	begin := 0
	var beginCh rune
	parens, squares, inSquares, curlies := false, false, false, false
	for j := 0; j < len(line); {
		r, n := utf8.DecodeRuneInString(line[j:])
		reset := false
		switch st {
		case lsAccept:
			var cls int
			switch r {
			case '(':
				parens, cls = true, linkNone
			case ')':
				cls = forceUnless(parens)
			case '[':
				inSquares, squares, cls = true, true, linkNone
			case ']':
				inSquares = false
				cls = forceUnless(squares)
			case '{':
				curlies, cls = true, linkNone
			case '}':
				cls = forceUnless(curlies)
			case '\'', '"', '`':
				switch {
				case beginCh == r:
					cls = linkForceEnd
				case beginCh == '\'' || beginCh == '"' || beginCh == '`':
					cls = linkNone
				default:
					cls = linkForceEnd
				}
			case '*':
				cls = linkNone
				if beginCh == '*' {
					cls = linkForceEnd
				}
			case ' ':
				cls = forceUnless(inSquares)
			default:
				cls = linkClass(r)
			}
			if cls == linkForceEnd {
				dst = append(dst, makeLink(line, begin, j))
				reset = true
			}
		case lsEnd:
			cls := linkClass(r)
			if r == '[' {
				squares, cls = true, linkNone
			}
			if cls == linkForceEnd {
				reset = true
			} else {
				st = lsAccept
			}
		default:
			if st = linkNext(st, r); st == lsInvalid {
				reset = true
			}
		}
		if reset {
			st = lsStart
			parens, squares, curlies = false, false, false
			begin, beginCh = j+n, r
		}
		j += n
	}
	if st == lsAccept {
		dst = append(dst, makeLink(line, begin, len(line)))
	}
	return dst
}

func forceUnless(open bool) int {
	if open {
		return linkNone
	}
	return linkForceEnd
}

// makeLink is Monaco's LinkComputer._createLink for the bytes [begin, end)
// of line.
func makeLink(line string, begin, end int) linkSpan {
	// li is the start of the last character kept.
	_, n := utf8.DecodeLastRuneInString(line[:end])
	li := end - n
	for {
		r, _ := utf8.DecodeRuneInString(line[li:])
		if linkClass(r) != linkCannotEnd {
			break
		}
		_, n := utf8.DecodeLastRuneInString(line[:li])
		li -= n
		if li <= begin {
			break
		}
	}
	if begin > 0 {
		before, lastCh := line[begin-1], line[li]
		if before == '(' && lastCh == ')' || before == '[' && lastCh == ']' || before == '{' && lastCh == '}' {
			_, n := utf8.DecodeLastRuneInString(line[:li])
			li -= n
		}
	}
	_, n = utf8.DecodeRuneInString(line[li:])
	return linkSpan{a: begin, b: li + n, url: line[begin : li+n]}
}

// lineLinks returns the links of model line i (none when links are off for
// the document or the line is long).
func (w *ViewW) lineLinks(i int, text string, dst []linkSpan) []linkSpan {
	if !w.opts.Links || w.st.tooLargeForWorker() || isLong(text) {
		return dst
	}
	return computeLinks(text, dst)
}

// linkHit is a drawn link, for the pointer: its boxes (window
// coordinates), line and bytes.
type linkHit struct {
	r    geom.Rect
	line int
	a, b int
	url  string
}

// linkKey names a link: its model line and start byte (line -1: none).
type linkKey struct{ line, a int }

// triggerMods reports whether m holds the link trigger modifier (Cmd on
// macOS, Ctrl elsewhere).
func (w *ViewW) triggerMods(m input.Modifiers) bool {
	if w.keymap().Platform() == keymap.Mac {
		return m&input.ModMeta != 0
	}
	return m&input.ModCtrl != 0
}

// linkAt returns the link drawn at window point p in the last paint.
func (w *ViewW) linkAt(p geom.Point) (linkHit, bool) {
	for _, h := range w.vs.linkHits {
		if h.r.Contains(p) {
			return h, true
		}
	}
	return linkHit{}, false
}

// linkHover updates the active link (the one under the pointer while the
// trigger modifier is held) and reports whether there is one.
func (w *ViewW) linkHover() bool {
	vs, c := w.vs, w.c
	vs.activeLink = linkKey{line: -1}
	p, in := c.Pointer()
	if !in || !w.triggerMods(c.PointerMods()) || !w.opts.Links {
		return false
	}
	if _, over := c.RectOf(w.box.ID()); !over {
		return false
	}
	if h, ok := w.linkAt(p); ok && vs.text.Contains(p) {
		vs.activeLink = linkKey{h.line, h.a}
		return true
	}
	return false
}

// OnOpenLink sets the function a Cmd+click (Ctrl+click elsewhere) on a
// link calls with its URL (Monaco's LinkDetector opening the link). The
// View keeps the last function set; nil turns opening off.
func (w *ViewW) OnOpenLink(fn func(url string)) *ViewW {
	w.vs.openLink = fn
	return w
}

// linkPress notes a press with the trigger modifier on a link; linkRelease
// opens it when the release is on the same link.
func (w *ViewW) linkPress(p pending) {
	w.vs.linkDown = nil
	if !p.trigger {
		return
	}
	if h, ok := w.linkAt(p.pressPos); ok {
		w.vs.linkDown = &h
	}
}

func (w *ViewW) linkRelease(p pending) {
	vs := w.vs
	d := vs.linkDown
	vs.linkDown = nil
	if d == nil || !p.release {
		return
	}
	if h, ok := w.linkAt(p.relPos); ok && h.line == d.line && h.a == d.a && vs.openLink != nil {
		vs.openLink(h.url)
	}
}
