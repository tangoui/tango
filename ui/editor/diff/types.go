package diff

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// DefaultTimeout is the time budget Monaco's diff editor uses. Options with a
// zero Timeout run without a limit; pass DefaultTimeout (or a context with a
// deadline) when the diff runs for an interactive view.
const DefaultTimeout = 5 * time.Second

// Options control a diff computation. The zero value compares whitespace,
// skips move detection and has no time limit.
type Options struct {
	// IgnoreTrimWhitespace treats lines that differ only in leading or
	// trailing whitespace as equal, and leaves such whitespace out of
	// character ranges.
	IgnoreTrimWhitespace bool
	// ComputeMoves detects blocks of lines that were moved (see Move).
	ComputeMoves bool
	// ExtendToSubwords widens character ranges to whole camelCase sub-words
	// when most of the sub-word changed.
	ExtendToSubwords bool
	// Timeout bounds the computation; 0 means no limit. When the budget runs
	// out the remaining work falls back to coarser results and
	// Result.TimedOut is set.
	Timeout time.Duration
}

// LineRange is the half-open range [Start, End) of 0-based line indices. An
// empty range marks the position between line Start-1 and line Start.
type LineRange struct{ Start, End int }

// Len returns the number of lines in r.
func (r LineRange) Len() int { return r.End - r.Start }

// Empty reports whether r contains no line.
func (r LineRange) Empty() bool { return r.End <= r.Start }

// Contains reports whether line lies in r.
func (r LineRange) Contains(line int) bool { return r.Start <= line && line < r.End }

func (r LineRange) String() string { return fmt.Sprintf("[%d,%d)", r.Start, r.End) }

func (r LineRange) join(o LineRange) LineRange {
	return LineRange{min(r.Start, o.Start), max(r.End, o.End)}
}

func (r LineRange) intersectsOrTouches(o LineRange) bool {
	return r.Start <= o.End && o.Start <= r.End
}

func (r LineRange) span() span { return span{r.Start, r.End} }

// Range is the text between two positions; End is exclusive. Columns are
// UTF-8 byte columns into the line (see buffer.Pos).
type Range struct{ Start, End buffer.Pos }

// Empty reports whether r selects no text.
func (r Range) Empty() bool { return r.Start == r.End }

func (r Range) String() string {
	return fmt.Sprintf("[%d:%d,%d:%d)", r.Start.Line, r.Start.Col, r.End.Line, r.End.Col)
}

// RangeMapping says that the text at Original was replaced by the text at
// Modified.
type RangeMapping struct{ Original, Modified Range }

func (m RangeMapping) String() string { return m.Original.String() + "->" + m.Modified.String() }

// Change is one changed block: lines Original of the original text became
// lines Modified of the modified text. One of the ranges is empty for a pure
// insertion or deletion. Inner lists the character-level replacements inside
// the block, in order; it is never empty for changes produced by this
// package.
type Change struct {
	Original, Modified LineRange
	Inner              []RangeMapping
}

func (c Change) String() string { return c.Original.String() + "->" + c.Modified.String() }

// Move is a block of lines that moved: Original and Modified have similar
// (not necessarily identical) text. Changes lists the differences between
// the two copies, in the coordinates of the whole texts. The lines of a
// move are also covered by Result.Changes, which describe them as a deletion
// and an insertion.
type Move struct {
	Original, Modified LineRange
	Changes            []Change
}

// Result is the outcome of a diff.
type Result struct {
	// Changes are the changed blocks, sorted and disjoint on both sides,
	// with at least one unchanged line between neighbours.
	Changes []Change
	// Moves are the detected moves, sorted by Original.Start; nil unless
	// Options.ComputeMoves is set.
	Moves []Move
	// Identical reports that the two texts are equal line for line. Changes
	// can be empty while Identical is false, when IgnoreTrimWhitespace hid
	// all differences.
	Identical bool
	// TimedOut reports that the time budget ran out or the context was
	// cancelled. The changes are still correct, but blocks may be larger
	// and character ranges coarser than with more time.
	TimedOut bool
	// OriginalLines and ModifiedLines are the line counts of the two texts
	// (at least 1).
	OriginalLines, ModifiedLines int
}

// Side selects one of the two texts of a diff.
type Side uint8

// The two sides of a diff.
const (
	Original Side = iota
	Modified
)

func (s Side) String() string {
	if s == Original {
		return "Original"
	}
	return "Modified"
}

// lines returns the side's range of c.
func (c Change) lines(s Side) LineRange {
	if s == Original {
		return c.Original
	}
	return c.Modified
}
