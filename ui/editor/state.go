package editor

import (
	"slices"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/highlight"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
)

// State is one editor's view of a Model: the selection, the viewport and
// the Options. The application owns it and passes it to the View every
// frame. Several States may show the same Model; an edit made through one
// moves the others' selections (see NewView). It is not safe for
// concurrent use; drive it from the UI goroutine.
type State struct {
	// Model is the document shown. It is set by NewView; do not replace
	// it (create another State with NewView, and carry the per-editor state
	// over with SaveViewState / RestoreViewState).
	Model *Model
	Theme *theme.Theme
	// Options are the editor's persistent settings: the application sets
	// them (the tab size and insertSpaces are the Model's: Model.TabSize,
	// Model.InsertSpaces). The View's modifiers (FontSize, TabSize, ReadOnly, LineNumbers)
	// override them for one frame only and do not write them.
	Options Options

	// Sels are the selections, the primary first (Monaco's
	// getSelections()); Head is the caret. There is always at least one.
	// Set them through SetSelections, MoveCaret or Move to keep them valid
	// and merged and the undo grouping right; see cursors.go.
	Sels []Selection
	// Viewport is the scroll position and the sizes the View measured.
	Viewport Viewport

	// vl maps model lines to view lines (wrapping; viewlines.go), ups are
	// the carets' row affinities and wrapToggle the word wrap
	// editor.action.toggleWordWrap set ("" for none: Options.WordWrap).
	vl         viewLines
	ups        caretAffinity
	wrapToggle WrapMode

	fold foldState // folded regions and folding ranges (folding.go)

	goals    []int // goal columns of vertical moves, one per selection; nil when unset
	lastKind editKind
	cancel   func() // Model subscription

	hlKey    highlight.ViewKey // this view's token window in the Highlighter
	frameTab int               // the View's TabSize override this frame, 0 for none

	// view is the View that last showed the State: commands that need
	// layout, the clipboard or the frame's options use it (see Execute).
	view *ViewW
	// tabFocus is "Tab moves focus" mode (editor.action.toggleTabFocusMode).
	tabFocus bool

	// Cursor history (cursorUndo / cursorRedo), the last column
	// selection and the addSelectionToNextFindMatch session; see
	// cursors.go.
	curUndo, curRedo []cursorStep
	cursorRestoring  bool
	colSel           *colSelect
	find             *findSession
	lastAdded        int // index in Sels of the last added cursor (Monaco's lastAddedCursorIndex)

	// autoClosed are the closing characters auto-closing inserted that
	// overtype and Backspace still treat as auto-inserted (autoclose.go).
	autoClosed []*autoClosedAction
	// gate is the typing assistance's "cheap to tokenize" state for the
	// current snapshot (autoclose.go).
	gate *tokGate
}

// editKind groups consecutive edits into undo steps: a change of kind (from
// typing to deleting) starts a new step, and "other" edits (paste,
// multi-character inserts, selection replacement) are a step of their own.
type editKind uint8

const (
	kindNone editKind = iota
	kindType
	kindDelete
	kindOther
)

// NewView returns a new editor State showing m, with DefaultOptions and the
// caret at the start. While it is open (until Close), an edit, undo or redo
// another view makes on m maps this view's selections through the edits as
// Monaco does: a caret where text is inserted moves after it, a caret
// inside deleted text moves to the start of the deletion. The first visible
// line follows the text the same way.
//
// The State subscribes to m: call Close when the editor goes away, or the
// Model keeps it (and keeps mapping its selection) for as long as the
// Model lives. Close does not release a ModelPool reference; Release the
// Model separately.
func NewView(m *Model, th *theme.Theme) *State {
	s := &State{Model: m, Theme: th, Options: DefaultOptions(), Sels: []Selection{{}}, hlKey: highlight.NewViewKey()}
	s.cancel = m.Subscribe(s.modelChanged)
	return s
}

// NewState returns a State showing a private Model (NewModel with filename
// as the URI and grammars from DefaultRegistry) holding text.
func NewState(text, filename string, th *theme.Theme) *State {
	return NewStateWithRegistry(text, filename, th, DefaultRegistry())
}

// NewStateWithRegistry is NewState with grammars from reg.
func NewStateWithRegistry(text, filename string, th *theme.Theme, reg *monarch.Registry) *State {
	return NewView(NewModel(filename, text, reg), th)
}

// Close releases the State's subscription to its Model, so the Model no
// longer holds or updates it. It is required when an editor goes away while
// its Model stays open (another view, a ModelPool). It does not release a
// ModelPool reference (call ModelPool.Release for that), and it is safe
// after the pool disposed the Model and when called twice. The State must
// not be used afterwards.
func (s *State) Close() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.Model.hl.DropView(s.hlKey)
	}
	s.view = nil
}

// modelChanged follows a change another view (or the Model) made.
func (s *State) modelChanged(ch ModelChange) {
	if ch.Source == s || len(ch.Edits) == 0 {
		return
	}
	// Mapping keeps the order: the last added cursor is remapped.
	s.Sels, _, s.lastAdded = mergeSelsLast(clampSels(ch.New, mapSels(ch.Edits, s.sels()), false), s.lastAdded)
	s.clampLast()
	s.autoClosedEdited(ch.Edits)
	s.goals = nil
	s.lastKind = kindNone
	s.clearCursorHistory()
	// The first visible line keeps showing the same text, except at the
	// very top: a view scrolled to line 0 stays at the top and its content
	// moves down when lines are inserted above (Monaco keeps the pixel
	// scrollTop, which is 0 there).
	mapTop := func(top int) int {
		if top > 0 && top < ch.Old.LineCount() {
			return ch.New.LineOf(clampOff(ch.New, mapOffset(ch.Edits, ch.Old.LineStart(top))))
		}
		return top
	}
	switch vl := &s.vl; {
	case vl.mapped() && vl.ver == ch.Old.Version() && vl.buf == s.Model.Buffer():
		// Wrapping or folding: the top model line follows the text,
		// keeping its row; the folds follow their anchors.
		line, row := vl.find(s.Viewport.TopLine)
		vl.syncEdits(s.Model.Buffer(), ch.Old, ch.Edits, ch.New)
		s.syncFolds()
		s.anchorTop(mapTop(line), row)
	case vl.mapped() || len(s.fold.folds) > 0:
		line, row := s.topAnchor()
		s.anchorTop(mapTop(line), row)
	default:
		s.Viewport.TopLine = mapTop(s.Viewport.TopLine)
	}
	s.clampViewport()
	s.revealFolds()
}

// Highlighter returns the Model's highlighter, nil for plain text.
func (s *State) Highlighter() *highlight.Highlighter { return s.Model.Highlighter() }

// tabSize returns the tab width in effect: the View's TabSize override this
// frame, else the Model's TabSize.
func (s *State) tabSize() int {
	if s.frameTab > 0 {
		return s.frameTab
	}
	return s.Model.TabSize()
}

// indentSize returns the tab size the indentation commands use (Tab,
// outdent, re-indent, comment alignment, converting the indentation): the
// Model's TabSize. Monaco's commands read the model's options; the View's
// per-frame TabSize modifier only changes how tabs are displayed.
func (s *State) indentSize() int { return s.Model.TabSize() }

// Snapshot returns the current text.
func (s *State) Snapshot() buffer.Snapshot { return s.Model.buf.Snapshot() }

// Insert replaces every selection with text (typing, paste, IME commit)
// and puts each caret after the inserted text, as one edit set. Line
// breaks are normalised as by buffer.Snapshot.Apply. Typing a single
// character at carets (no selection) continues the current undo step;
// anything else is a step of its own.
func (s *State) Insert(text string) {
	if text == "" && s.allEmpty() {
		return
	}
	kind := kindOther
	if s.allEmpty() && nextGrapheme(text, 0) == len(text) && !isControl(text) {
		kind = kindType
	}
	s.replaceSels(kind, func(int, int, int) string { return text })
}

// Delete deletes every selection, and at each caret the text between it
// and where Move(dir, unit) would put it, as one edit set; ranges of
// several cursors that overlap merge. Consecutive Char deletes form one
// undo step; every other delete is a step of its own. Line deletes the
// caret's whole line including one adjacent line break, whatever dir;
// LineEdge deletes to column 0 or to the line end (not to the first
// non-blank).
func (s *State) Delete(dir Direction, unit Unit) {
	s.normalize()
	snap := s.Snapshot()
	sels := s.Sels
	ces := make([]cursorEdit, len(sels))
	allEmpty, any := true, false
	for k, sel := range sels {
		a, b := ordered(Selection{Anchor: clampOff(snap, sel.Anchor), Head: clampOff(snap, sel.Head)})
		if a != b {
			allEmpty = false
		} else {
			switch unit {
			case Line:
				a, b = lineRange(snap, snap.LineOf(a))
			case LineEdge:
				l := snap.LineOf(a)
				if dir == Backward {
					a = snap.LineStart(l)
				} else {
					b = lineEnd(snap, l)
				}
			default:
				t := s.target(dir, unit, a)
				a, b = min(a, t), max(a, t)
			}
		}
		any = any || a != b
		ces[k] = cursorEdit{a: a, b: b}
	}
	if !any {
		return
	}
	kind := kindOther // word, line, ... deletes are steps of their own (Monaco)
	if unit == Char && allEmpty {
		kind = kindDelete
	}
	s.applyCursorEdits(ces, kind)
}

// isControl reports whether text starts with a control character (a line
// break, tab, ...).
func isControl(text string) bool {
	r, _ := utf8.DecodeRuneInString(text)
	return classOf(r) == gbControl
}

// lineRange is the range that deletes line i with one adjacent line break.
func lineRange(snap buffer.Snapshot, i int) (int, int) {
	switch {
	case i+1 < snap.LineCount():
		return snap.LineStart(i), snap.LineStart(i + 1)
	case i > 0:
		return lineEnd(snap, i-1), snap.Len()
	}
	return 0, snap.Len()
}

// Edit applies arbitrary edits (as buffer.Buffer.Edit: sorted,
// non-overlapping, in the current snapshot's offsets) and sets the selection
// to after, an offset pair in the new text (clamped), the only selection.
// It neither ends nor starts undo steps: callers group with
// s.Model.Buffer().PushUndoStop (except that an edit following another
// view's edit starts a new step). On error nothing changes.
func (s *State) Edit(edits []buffer.Edit, after Selection) error {
	return s.edit(edits, kindNone, []Selection{after}, true)
}

// edit applies edits to the model and sets the selections to after (in
// the new text; clamped onto character boundaries when exact, else only
// into the document). During a Model notification it is queued (see
// Model.Subscribe) and reports nil.
func (s *State) edit(edits []buffer.Edit, kind editKind, after []Selection, exact bool) error {
	if m := s.Model; m.notifying {
		edits, after = slices.Clone(edits), slices.Clone(after)
		m.enqueue(func(since [][]buffer.Edit) {
			s.edit(mapEdits(edits, since), kind, mapSelsSince(after, since), true)
		})
		return nil
	}
	buf := s.Model.buf
	if kind != kindNone && (kind != s.lastKind || kind == kindOther) {
		buf.PushUndoStop()
	}
	err := s.Model.edit(s, edits, s.sels(), after, func(ns buffer.Snapshot) {
		s.Sels, _ = mergeSels(clampSels(ns, after, exact))
		s.clampLast()
		s.autoClosedEdited(edits)
		s.goals = nil
		s.clearCursorHistory()
		s.clampViewport()
		s.revealFolds()
	})
	if err != nil {
		return err
	}
	switch kind {
	case kindNone:
		s.lastKind = kindNone
	case kindOther:
		buf.PushUndoStop()
		s.lastKind = kindNone
	default:
		s.lastKind = kind
	}
	return nil
}

// Undo reverts the Model's last undo step (whichever view made it) and
// restores the selections from before it in this view; it reports false
// when there is nothing to undo. During a Model notification it is queued
// (see Model.Subscribe) and reports true.
func (s *State) Undo() bool { return s.history(false) }

// Redo re-applies the last undone step and restores the selections from
// after it in this view; it reports false when there is nothing to redo.
// During a Model notification it is queued and reports true.
func (s *State) Redo() bool { return s.history(true) }

func (s *State) history(redo bool) bool {
	if m := s.Model; m.notifying {
		m.enqueue(func([][]buffer.Edit) { s.history(redo) })
		return true
	}
	return s.Model.history(s, redo, func(ns buffer.Snapshot, sels []Selection) {
		if len(sels) == 0 {
			sels = s.sels()
		}
		s.Sels, _ = mergeSels(clampSels(ns, sels, false))
		s.clampLast()
		if len(s.autoClosed) > 0 {
			_, edits := s.Model.buf.LastChange()
			s.autoClosedEdited(edits)
		}
		s.goals = nil
		s.lastKind = kindNone
		s.clearCursorHistory()
		s.clampViewport()
		s.revealFolds()
	})
}
