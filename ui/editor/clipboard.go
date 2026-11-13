package editor

import (
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Clipboard commands (Monaco's clipboard contribution and its
// emptySelectionClipboard / multiCursorPaste options):
//
//   - Copy writes the selected text, the selections in document order
//     joined with line breaks. With EmptySelectionClipboard (the default),
//     a caret copies its whole line with its line break, once per line;
//     without it, an empty primary selection copies nothing at all.
//   - Cut copies as Copy and, when the view is writable, deletes what it
//     copied: the selected text, and for a caret its whole line (the line
//     break after it, or before it on the last line). In a read-only view
//     it only copies, as Monaco's.
//   - Paste inserts the clipboard text as is (no auto-closing, no onEnter:
//     those are typing assistance; see cmdType) as an undo step of its
//     own. Text copied from a caret (a whole line) is inserted above the
//     caret's line, the caret staying where it was in the text. With
//     several cursors the text is spread one piece per cursor, in document
//     order, when it came from as many selections, or (MultiCursorPaste
//     "spread", the default) when it has as many lines as there are
//     cursors.
//
// What the copy was (from a caret, the pieces of several selections) is
// remembered in this process only, for the very text written to the
// clipboard, as Monaco's InMemoryClipboardMetadataManager: text another
// application wrote, or that changed since, pastes as plain text.

func init() {
	for _, c := range clipboardCommands() {
		RegisterCommand(c)
	}
}

func clipboardCommands() []Command {
	return []Command{
		{ID: "editor.action.clipboardCopyAction", Label: "Copy", Run: func(st *State, _ any) bool {
			return st.copySelection()
		}},
		// Cut copies in a read-only view too, so it is not Mutates; it
		// deletes only when the view is writable.
		{ID: "editor.action.clipboardCutAction", Label: "Cut", Run: func(st *State, _ any) bool {
			if !st.copySelection() {
				return false
			}
			if !st.readOnly() {
				st.cut()
				st.revealCaret()
			}
			return true
		}},
		{ID: "editor.action.clipboardPasteAction", Label: "Paste", Precondition: writable, Mutates: true, Run: func(st *State, _ any) bool {
			w := st.view
			if w == nil {
				return false
			}
			win := w.c.Window()
			if win == nil {
				return false
			}
			s := win.ReadClipboard()
			if s == "" {
				return false
			}
			st.paste(s)
			st.revealCaret()
			return true
		}},
	}
}

// MultiCursorPasteMode is how pasted text is shared among several cursors
// (Monaco's multiCursorPaste option).
type MultiCursorPasteMode string

const (
	// MultiCursorPasteSpread gives each cursor one line of the text when
	// the text has as many lines as there are cursors.
	MultiCursorPasteSpread MultiCursorPasteMode = "spread"
	// MultiCursorPasteFull pastes the whole text at every cursor (unless
	// it was copied from as many selections).
	MultiCursorPasteFull MultiCursorPasteMode = "full"
)

// clipMeta is what the editor remembers about text it wrote to the
// clipboard (Monaco's ClipboardStoredMetadata).
type clipMeta struct {
	// fromEmpty: copied from a single caret, a whole line.
	fromEmpty bool
	// multi: the pieces of several selections, in document order; nil for
	// one.
	multi []string
}

// clipMemory is the metadata of the last text the editor copied, for this
// process only (Monaco's InMemoryClipboardMetadataManager).
var clipMemory struct {
	sync.Mutex
	text string
	meta clipMeta
	ok   bool
}

// rememberClip records meta for text, the text just written to the
// clipboard.
func rememberClip(text string, meta clipMeta) {
	clipMemory.Lock()
	clipMemory.text, clipMemory.meta, clipMemory.ok = text, meta, true
	clipMemory.Unlock()
}

// recallClip returns the metadata of text when it is the text last copied;
// any other text forgets it (as Monaco's).
func recallClip(text string) (clipMeta, bool) {
	clipMemory.Lock()
	defer clipMemory.Unlock()
	if clipMemory.ok && clipMemory.text == text {
		return clipMemory.meta, true
	}
	clipMemory.text, clipMemory.meta, clipMemory.ok = "", clipMeta{}, false
	return clipMeta{}, false
}

// frameOpts returns the options in effect: the last View's for this frame,
// else the State's.
func (s *State) frameOpts() *Options {
	if s.view != nil {
		return &s.view.opts
	}
	return &s.Options
}

// copyData returns the text to copy from sels (in the stored text, with
// "\n" line breaks) as Monaco's getPlainTextToCopy: the ranges in document
// order; with emptyClip, a caret contributes its line and a line break
// (once per line); without it carets contribute nothing. pieces holds one
// entry per range; ok is false when there is nothing to copy.
func copyData(snap buffer.Snapshot, sels []Selection, emptyClip bool) (pieces []string, fromEmpty, ok bool) {
	rs := make([][2]int, len(sels))
	hasEmpty, hasRange := false, false
	for i, x := range sels {
		a, b := ordered(Selection{Anchor: clampOff(snap, x.Anchor), Head: clampOff(snap, x.Head)})
		rs[i] = [2]int{a, b}
		if a == b {
			hasEmpty = true
		} else {
			hasRange = true
		}
	}
	if !hasRange && !emptyClip {
		return nil, false, false
	}
	slices.SortStableFunc(rs, func(x, y [2]int) int {
		if x[0] != y[0] {
			return x[0] - y[0]
		}
		return x[1] - y[1]
	})
	if hasEmpty && emptyClip {
		prev := -1
		for _, r := range rs {
			l := snap.LineOf(r[0])
			if r[0] == r[1] {
				if l != prev {
					pieces = append(pieces, snap.Line(l)+"\n")
				}
			} else {
				pieces = append(pieces, snap.Slice(r[0], r[1]))
			}
			prev = l
		}
	} else {
		for _, r := range rs {
			if r[0] != r[1] {
				pieces = append(pieces, snap.Slice(r[0], r[1]))
			}
		}
	}
	fromEmpty = emptyClip && len(sels) == 1 && rs[0][0] == rs[0][1]
	return pieces, fromEmpty, true
}

// copySelection writes the selected text to the window clipboard (with
// the document's line ending; see copyData) and remembers how it was
// copied (clipMeta). It reports false when there is nothing to copy or no
// window.
func (s *State) copySelection() bool {
	if s.view == nil {
		return false
	}
	win := s.view.c.Window()
	if win == nil {
		return false
	}
	snap := s.Snapshot()
	emptyClip := s.frameOpts().EmptySelectionClipboard
	// Without emptySelectionClipboard, Monaco copies nothing when the
	// primary selection is empty, whatever the other selections hold.
	if p := s.Primary(); !emptyClip && p.Anchor == p.Head {
		return false
	}
	pieces, fromEmpty, ok := copyData(snap, s.sels(), emptyClip)
	if !ok {
		return false
	}
	if eol := snap.EOL(); eol != "\n" {
		for i, p := range pieces {
			pieces[i] = strings.ReplaceAll(p, "\n", eol)
		}
	}
	text := strings.Join(pieces, snap.EOL())
	meta := clipMeta{fromEmpty: fromEmpty}
	if len(pieces) > 1 {
		meta.multi = pieces
	}
	win.WriteClipboard(text)
	rememberClip(text, meta)
	return true
}

// cut deletes what copySelection copied (Monaco's DeleteOperations.cut):
// the selected text and, with EmptySelectionClipboard, the line of each
// caret with its line break (on the last line, the break before it; the
// text of a single line). The carets end at the start of what they
// deleted. One undo step.
func (s *State) cut() bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	emptyClip := s.frameOpts().EmptySelectionClipboard
	order := make([]int, len(sels))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int {
		ax, _ := ordered(sels[x])
		ay, _ := ordered(sels[y])
		return ax - ay
	})
	cmds := make([]opCmd, len(sels))
	last := -1 // line where the previous whole-line cut ended
	n := snap.LineCount()
	for _, k := range order {
		a, b := ordered(sels[k])
		if a != b {
			cmds[k].edits = []buffer.Edit{{Start: a, End: b}}
			continue
		}
		if !emptyClip {
			continue
		}
		l := snap.LineOf(a)
		var da, db, endLine int
		switch {
		case l+1 < n:
			da, db, endLine = snap.LineStart(l), snap.LineStart(l+1), l+1
		case l > 0 && last != l:
			da, db, endLine = lineEnd(snap, l-1), lineEnd(snap, l), l
		default:
			da, db, endLine = snap.LineStart(l), lineEnd(snap, l), l
		}
		last = endLine
		if da != db {
			cmds[k].edits = []buffer.Edit{{Start: da, End: db}}
		}
	}
	return s.runOps(cmds)
}

// distributePaste returns the piece of text each of n cursors gets
// (Monaco's _distributePasteToCursors), nil to paste the whole text at
// every cursor: the pieces of a copy from n selections, else, unless the
// text is a whole line copied from a caret, in "spread" mode the lines of
// text when there are n of them (a trailing line break dropped).
func distributePaste(n int, text string, onNewLine bool, multi []string, mode MultiCursorPasteMode) []string {
	if n <= 1 {
		return nil
	}
	if len(multi) == n {
		return multi
	}
	if onNewLine || mode == MultiCursorPasteFull {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\r")
	t := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if lines := strings.Split(t, "\n"); len(lines) == n {
		return lines
	}
	return nil
}

// paste inserts clipboard text as an undo step of its own, without the
// typing assistance (Monaco's PasteOperation): spread over the cursors in
// document order (distributePaste), else a whole line copied from a caret
// goes above each caret's line (the caret keeping its place in the text),
// else the text replaces every selection.
func (s *State) paste(text string) {
	o := s.frameOpts()
	var meta clipMeta
	if m, ok := recallClip(text); ok {
		meta = m
	}
	onNewLine := o.EmptySelectionClipboard && meta.fromEmpty
	s.normalize()
	if pieces := distributePaste(len(s.Sels), text, onNewLine, meta.multi, o.MultiCursorPaste); pieces != nil {
		// Pieces go to the cursors in document order. The primary stays
		// the same cursor (Monaco makes the cursor first in the document
		// the primary after a spread paste).
		rank := make([]int, len(s.Sels))
		order := make([]int, len(s.Sels))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(x, y int) int {
			ax, _ := ordered(s.Sels[x])
			ay, _ := ordered(s.Sels[y])
			return ax - ay
		})
		for r, k := range order {
			rank[k] = r
		}
		s.replaceSels(kindOther, func(k, _, _ int) string { return pieces[rank[k]] })
		return
	}
	// A whole line goes above the caret's line only when it is one line
	// ending in its line break, and not over a selection (Monaco checks
	// the cursors in order and stops at the first selection).
	if onNewLine && strings.IndexByte(text, '\n') != len(text)-1 {
		onNewLine = false
	}
	if !onNewLine {
		s.insertStep(text)
		return
	}
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		if sel.Anchor != sel.Head {
			onNewLine = false
		}
		if !onNewLine {
			a, b := ordered(sel)
			cmds[k] = opCmd{edits: []buffer.Edit{{Start: a, End: b, Text: text}}, cursor: func(r *opResult, _ Selection) Selection {
				end := r.inv[k][0][1]
				return Selection{Anchor: end, Head: end}
			}}
			continue
		}
		ls := snap.LineStart(snap.LineOf(sel.Head))
		// The caret moves with the inserted line even at the line start
		// (Monaco's forceMoveMarkers).
		cmds[k] = opCmd{edits: []buffer.Edit{{Start: ls, End: ls, Text: text}}, cursor: func(r *opResult, sel Selection) Selection {
			return Selection{Anchor: r.tr.Map(sel.Anchor, buffer.Right), Head: r.tr.Map(sel.Head, buffer.Right)}
		}}
	}
	s.runOps(cmds)
}
