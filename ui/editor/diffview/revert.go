package diffview

import (
	"fmt"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/diff"
)

// revertEdit returns the edit of mod that reverts change i of r: the
// modified lines of the change are replaced by its original lines (Monaco's
// "revert block"). orig and mod are the texts r describes; mod may have
// other changes reverted already when they come after change i. The view
// applies the edit as one undo step and recomputes the diff.
func revertEdit(r diff.Result, i int, orig, mod buffer.Snapshot) (buffer.Edit, error) {
	if i < 0 || i >= len(r.Changes) {
		return buffer.Edit{}, fmt.Errorf("diffview: change %d out of range [0,%d)", i, len(r.Changes))
	}
	c := r.Changes[i]
	if c.Original.End > orig.LineCount() || c.Modified.End > mod.LineCount() {
		return buffer.Edit{}, fmt.Errorf("diffview: change %v does not fit texts of %d and %d lines", c, orig.LineCount(), mod.LineCount())
	}
	// Work on the texts with a line break appended, so that every line,
	// the last one included, ends in one: the lines [a, b) are then the
	// bytes [lineOff(a), lineOff(b)). Map the edit back at the end.
	text := linesText(orig, c.Original)
	start, end := lineOff(mod, c.Modified.Start), lineOff(mod, c.Modified.End)
	n := mod.Len()
	if start > n { // insertion after the last line
		start, text = n, "\n"+text
	}
	if end <= n {
		return buffer.Edit{Start: start, End: end, Text: text}, nil
	}
	// The edit swallows the appended line break; the text's own final
	// break takes its place, or, when nothing is inserted, the break
	// before the deleted lines goes.
	switch {
	case text != "":
		text = text[:len(text)-1]
	case start > 0:
		start--
	}
	return buffer.Edit{Start: start, End: n, Text: text}, nil
}

// lineOff returns the offset of line l in s with a line break appended:
// LineStart, or Len()+1 for l == LineCount().
func lineOff(s buffer.Snapshot, l int) int {
	if l < s.LineCount() {
		return s.LineStart(l)
	}
	return s.Len() + 1
}

// linesText returns lines lr of s, each followed by a line break.
func linesText(s buffer.Snapshot, lr diff.LineRange) string {
	if lr.Empty() {
		return ""
	}
	if lr.End < s.LineCount() {
		return s.Slice(s.LineStart(lr.Start), s.LineStart(lr.End))
	}
	return s.Slice(s.LineStart(lr.Start), s.Len()) + "\n"
}
