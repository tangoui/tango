// Package buffer is the immutable text model behind the code editor.
//
// A Snapshot is a persistent rope: every edit returns a new Snapshot that
// shares all untouched text with the old one, so snapshots are cheap to keep
// (undo, background tokenizing, diffing) and safe to read from any goroutine.
// Offsets are UTF-8 byte offsets into the LF-normalised text; UTF16Col and
// ByteCol convert to the UTF-16 columns used by LSP and Monaco.
//
// Indexing methods (Line, LineStart, LineOf, Slice) panic on out-of-range
// arguments, like Go slice indexing. Coordinate conversions (PosOf, OffsetOf,
// UTF16Col, ByteCol) clamp instead, since their inputs often come from
// outside (a language server, a stale cursor).
//
// Contracts:
//   - Snapshot.Version is monotonic only within one Buffer; it does not
//     identify a document across Buffers.
//   - An Anchor follows the text through edits, undo and redo of its own
//     Buffer by Monaco's marker rule (see Buffer.Resolve). Resolve replays
//     every edit since the anchor was made, so its cost is linear in that
//     number; long-lived anchors (breakpoints, diagnostics, bookmarks) should
//     be re-anchored at their resolved offset as the history grows.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package buffer

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Errors returned by Snapshot.Apply. They are wrapped with details; test with
// errors.Is.
var (
	ErrEditRange = errors.New("buffer: edit out of range")
	ErrEditOrder = errors.New("buffer: edits not sorted or overlapping")
	ErrSplitRune = errors.New("buffer: edit boundary splits a UTF-8 character")
)

// Snapshot is an immutable version of a document. It is a small value type;
// copying it is free and it may be shared across goroutines. The zero value
// is an empty document.
type Snapshot struct {
	root    *node
	eol     string
	version uint64
}

// Edit replaces bytes [Start, End) of a snapshot with Text.
type Edit struct {
	Start, End int
	Text       string
}

// NewSnapshot loads text as version 1 of a document. Like Monaco, "\r\n",
// a lone "\r" and "\n" are all line breaks, and the internal text stores
// each of them as "\n". The document's EOL is "\r\n" when CRLF breaks
// outnumber the other two kinds combined, otherwise "\n".
func NewSnapshot(text string) Snapshot {
	eol := "\n"
	if cr := strings.Count(text, "\r"); cr > 0 {
		crlf := strings.Count(text, "\r\n")
		lf := strings.Count(text, "\n")
		if loneCR, loneLF := cr-crlf, lf-crlf; crlf > loneCR+loneLF {
			eol = "\r\n"
		}
		text = normalizeEOL(text)
	}
	return Snapshot{root: build(text), eol: eol, version: 1}
}

// normalizeEOL rewrites "\r\n" and lone "\r" as "\n".
func normalizeEOL(text string) string {
	if strings.IndexByte(text, '\r') < 0 {
		return text
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.IndexByte(text, '\r') < 0 {
		return text
	}
	return strings.ReplaceAll(text, "\r", "\n")
}

// Len returns the length in bytes of the LF-normalised text.
func (s Snapshot) Len() int {
	if s.root == nil {
		return 0
	}
	return s.root.bytes
}

// LineCount returns the number of lines; it is the number of '\n' plus one,
// so empty text has one line and text ending in '\n' ends with an empty line.
func (s Snapshot) LineCount() int {
	if s.root == nil {
		return 1
	}
	return s.root.lines + 1
}

// LineStart returns the byte offset of the first byte of line i (0-based).
// It panics if i is out of range.
func (s Snapshot) LineStart(i int) int {
	if i < 0 || i >= s.LineCount() {
		panic(fmt.Sprintf("buffer: line %d out of range [0,%d)", i, s.LineCount()))
	}
	if i == 0 {
		return 0
	}
	return afterNewline(s.root, i)
}

// lineEnd returns the offset of the '\n' ending line i, or Len() for the last
// line.
func (s Snapshot) lineEnd(i int) int {
	if i+1 >= s.LineCount() {
		return s.Len()
	}
	return afterNewline(s.root, i+1) - 1
}

// LineOf returns the line containing byte offset off; off == Len() is on the
// last line. It panics if off is outside [0, Len()].
func (s Snapshot) LineOf(off int) int {
	s.checkOffset(off)
	if s.root == nil {
		return 0
	}
	return newlinesBefore(s.root, off)
}

// Line returns line i without its line break. It panics if i is out of range.
func (s Snapshot) Line(i int) string {
	start := s.LineStart(i)
	return s.slice(start, s.lineEnd(i))
}

// Slice returns bytes [start, end) of the LF-normalised text. It panics if
// the range is invalid.
func (s Snapshot) Slice(start, end int) string {
	s.checkOffset(start)
	s.checkOffset(end)
	if start > end {
		panic(fmt.Sprintf("buffer: invalid slice [%d:%d]", start, end))
	}
	return s.slice(start, end)
}

func (s Snapshot) slice(start, end int) string {
	if s.root == nil {
		return ""
	}
	return slice(s.root, start, end)
}

// String returns the whole LF-normalised text.
func (s Snapshot) String() string {
	return s.slice(0, s.Len())
}

// TextWithEOL returns the whole text with every line break written as EOL(),
// i.e. the form to save back to disk.
func (s Snapshot) TextWithEOL() string {
	if s.EOL() == "\n" || s.root == nil {
		return s.String()
	}
	var b strings.Builder
	b.Grow(s.root.bytes + s.root.lines*(len(s.eol)-1))
	appendRange(&b, s.root, 0, s.root.bytes, s.eol)
	return b.String()
}

// EOL returns the document's line ending, "\n" or "\r\n".
func (s Snapshot) EOL() string {
	if s.eol == "" {
		return "\n"
	}
	return s.eol
}

// Version returns the snapshot's version: 1 for NewSnapshot, and one more
// than its parent for each successful Apply. Versions count edits along one
// history; two snapshots derived from the same parent share a version.
//
// Version is monotonic only within one Buffer (whose undo/redo also produce
// new, higher versions). It is not a document identity: snapshots of
// different Buffers, or of two Applys on the same parent, can have equal
// versions with different text, so do not compare versions across Buffers.
func (s Snapshot) Version() uint64 { return s.version }

// Apply returns a new snapshot with edits applied. Offsets refer to s; edits
// must be sorted by Start and must not overlap (an edit may start where the
// previous one ends, so several inserts at one offset apply in order). Every
// boundary must fall on a UTF-8 character boundary. On error s is returned
// unchanged.
//
// Inserted text is normalised like NewSnapshot: "\r\n" and lone "\r" become
// "\n", so the inserted length can be shorter than len(Edit.Text). Offsets
// of all edits in one call still refer to s, so this never shifts the other
// edits; callers placing a cursor after an edit should measure the normalised
// text (or use the new snapshot) rather than len(Edit.Text).
func (s Snapshot) Apply(edits []Edit) (Snapshot, error) {
	n := s.Len()
	prevEnd := 0
	for i, e := range edits {
		if e.Start < 0 || e.End < e.Start || e.End > n {
			return s, fmt.Errorf("%w: edit %d [%d,%d) in text of length %d", ErrEditRange, i, e.Start, e.End, n)
		}
		if e.Start < prevEnd {
			return s, fmt.Errorf("%w: edit %d starts at %d before previous end %d", ErrEditOrder, i, e.Start, prevEnd)
		}
		prevEnd = e.End
		if s.splitsRune(e.Start) || (e.End != e.Start && s.splitsRune(e.End)) {
			return s, fmt.Errorf("%w: edit %d [%d,%d)", ErrSplitRune, i, e.Start, e.End)
		}
	}
	root := s.root
	// Apply back to front so earlier offsets stay valid.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		if e.Start == e.End && e.Text == "" {
			continue
		}
		root = replace(root, e.Start, e.End, normalizeEOL(e.Text))
	}
	return Snapshot{root: root, eol: s.eol, version: s.version + 1}, nil
}

// splitsRune reports whether off lies strictly inside a valid multi-byte
// UTF-8 sequence. Offsets inside invalid byte sequences are allowed so that
// non-UTF-8 files stay editable.
func (s Snapshot) splitsRune(off int) bool {
	if off <= 0 || off >= s.Len() {
		return false
	}
	lo := max(0, off-(utf8.UTFMax-1))
	window := s.slice(lo, min(s.Len(), lo+2*utf8.UTFMax))
	at := off - lo
	if utf8.RuneStart(window[at]) {
		return false
	}
	// Find the nearest preceding start byte and decode from there.
	for i := at - 1; i >= 0; i-- {
		if utf8.RuneStart(window[i]) {
			_, size := utf8.DecodeRuneInString(window[i:])
			return size > 1 && i+size > at
		}
	}
	return false
}

func (s Snapshot) checkOffset(off int) {
	if off < 0 || off > s.Len() {
		panic(fmt.Sprintf("buffer: offset %d out of range [0,%d]", off, s.Len()))
	}
}
