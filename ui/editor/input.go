package editor

import (
	"math"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Keyboard and mouse input of the View.
//
// Input is collected while the View is built (View: the keys, committed
// text, IME composition and pointer state the Context delivers to the
// focused view) into the retained viewState, and applied once per frame by
// finish: when the application asks Changed (after the modifiers such as
// ReadOnly are set), or else at the start of paint. Collected input lives in
// the retained state, so it survives the extra build passes the Runner runs
// after input (only the pass that sees the events collects them).
//
// Keys: every key press goes through the view's keymap (ViewW.Keymap,
// default keymap.DefaultRules for the platform: Monaco 0.56's default
// bindings), which resolves it, with the context keys (contextkeys.go), to
// a registered command (command.go, corecommands.go) run by
// State.Execute. Keys no rule binds do nothing; characters are typed
// through committed text (TextInput), not keybindings. While an IME
// composition is in progress keys are left to the input method and the
// keymap is not consulted. A two-chord binding's first chord is pending
// until the next key, with no timeout; losing focus or the window's
// activation clears it. On macOS and Linux a key that produces text
// arrives as text only (no KeyEvent): while a chord is pending such text
// is resolved as the chord's next key (chordText) and never typed. A
// command whose Precondition fails, or a Mutates command in a read-only
// view, does not take its key (commandEnabled). Milestone 1's fixed keys are now these commands
// (macOS binding / Windows and Linux binding):
//
//	cursorLeft/Right/Up/Down(Select)        arrows (Shift)
//	cursorWordLeft / cursorWordEndRight     Alt+Left/Right / Ctrl+Left/Right
//	cursorWordPartLeft / Right(Select)      Ctrl+Alt(+Shift)+Left/Right (macOS only; wordpart.go)
//	deleteWordPartLeft / Right              Ctrl+Alt+Backspace/Delete (macOS only)
//	cursorHome / cursorEnd                  Home/End and Cmd+Left/Right / Home/End
//	cursorTop / cursorBottom                Cmd+Up/Down / Ctrl+Home/End
//	cursorPageUp / cursorPageDown           PageUp / PageDown
//	editor.action.selectAll                 Cmd+A / Ctrl+A
//	deleteLeft / deleteRight                Backspace / Delete
//	deleteWordLeft / deleteWordRight        Alt+Backspace/Delete / Ctrl+Backspace/Delete
//	deleteAllLeft / deleteAllRight          Cmd+Backspace, Cmd+Delete / (none)
//	type {"text": "\n"}                     Enter (onEnter and indentation rules, onenter.go)
//	tab                                     Tab (spaces to the next tab stop or a tab)
//	editor.action.toggleTabFocusMode        Ctrl+Shift+M (and Ctrl+M) / Ctrl+M
//	undo / redo                             Cmd+Z, Shift+Cmd+Z / Ctrl+Z, Ctrl+Y, Ctrl+Shift+Z
//	editor.action.clipboard{Copy,Cut,Paste}Action  Cmd+C/X/V / Ctrl+C/X/V
//
// Multiple cursors (cursors.go; every command above acts on every cursor):
//
//	editor.action.insertCursorAbove/Below   Alt+Cmd+Up/Down / Ctrl+Alt+Up/Down (Windows),
//	                                        Shift+Alt+Up/Down and Ctrl+Shift+Up/Down (Linux)
//	editor.action.insertCursorAtEndOfEachLineSelected  Shift+Alt+I
//	editor.action.addSelectionToNextFindMatch  Cmd+D / Ctrl+D
//	editor.action.selectHighlights          Shift+Cmd+L / Ctrl+Shift+L
//	removeSecondaryCursors                  Esc, Shift+Esc (with several cursors)
//	cursorUndo / cursorRedo                 Cmd+U / Ctrl+U; cursorRedo has no key
//	cursorColumnSelect{Left,Right,Up,Down,PageUp,PageDown}  Shift+Alt+Cmd+arrows/PageUp/PageDown /
//	                                        Ctrl+Shift+Alt+... (Windows), no Linux key (Monaco)
//
// Line operations, indentation and comments (lineops.go, indent.go,
// comment.go):
//
//	editor.action.copyLinesUp/DownAction    Shift+Alt+Up/Down / Ctrl+Shift+Alt+Up/Down (Linux)
//	editor.action.moveLinesUp/DownAction    Alt+Up/Down
//	editor.action.deleteLines               Shift+Cmd+K / Ctrl+Shift+K
//	editor.action.joinLines                 Ctrl+J (macOS only)
//	editor.action.insertLineAfter/Before    Cmd+Enter, Shift+Cmd+Enter / Ctrl+Enter, Ctrl+Shift+Enter
//	editor.action.trimTrailingWhitespace    Cmd+K Cmd+X / Ctrl+K Ctrl+X
//	tab / outdent                           Tab / Shift+Tab (indent the selected lines, outdent)
//	editor.action.indentLines/outdentLines  Cmd+] / Cmd+[ (Ctrl+] / Ctrl+[)
//	editor.action.commentLine               Cmd+/ / Ctrl+/
//	editor.action.addCommentLine            Cmd+K Cmd+C / Ctrl+K Ctrl+C
//	editor.action.removeCommentLine         Cmd+K Cmd+U / Ctrl+K Ctrl+U
//	editor.action.blockComment              Shift+Alt+A / Ctrl+Shift+A (Linux)
//
// duplicateSelection, transpose, sortLines{Ascending,Descending},
// reindentlines and indentationTo{Spaces,Tabs} have no default key (as in
// Monaco).
//
// Tab: while the view is focused, editable and not in "Tab moves focus"
// mode it captures Tab (Context.CaptureTab), so Tab and Shift+Tab arrive
// as keys; Ctrl+Tab, or Esc then Tab, still move focus out (no keyboard
// trap). The capture is declared when the view paints, after this frame's
// input (a Ctrl+M toggle) and modifiers (ReadOnly) are applied, so it is
// right for the very next key; the Runner reads declarations at the end of
// the frame.
//
// Mouse: a press puts the caret at the nearest grapheme boundary and
// removes the secondary cursors (Shift extends the primary selection),
// dragging extends it; a double click selects the word, a triple click the
// line with its line break, and dragging after them extends by words or
// lines. Monaco's multiCursorModifier "alt": Alt+click adds a cursor (or
// removes the selection clicked in, when there are others) and Alt+drag
// extends the added one; Shift+Alt+click and Shift+Alt+drag select a
// column box from the primary selection's anchor (or from the start of
// the current column selection) to the pointer's line and visual column
// (State.ColumnSelect). A press on the primary selection's text picks it
// up and dragging moves the text, or copies it with Alt (Option) on macOS
// and Ctrl elsewhere (Monaco's dragAndDrop; dnd.go).
//
// Clipboard (clipboard.go): with no selection, Copy and Cut take the whole
// line (emptySelectionClipboard) and Paste of such a copy inserts it above
// the caret's line; several cursors share pasted text one line each
// (multiCursorPaste "spread").
//
// Read-only (ViewW.ReadOnly, Options.ReadOnly): every Mutates command is
// refused (State.Execute, commandEnabled), as are typing, the IME, the
// deletion of Cut and dragging text; copy, selection and navigation work.

// blinkPeriod is a full caret blink cycle: visible for the first half
// (Monaco's 530 ms on, 530 ms off).
const blinkPeriod = 1060 * time.Millisecond

// Drag units.
const (
	unitChar = iota
	unitWord
	unitLine
)

// Drag modes.
const (
	dragPrimary = iota
	dragAdded
	dragColumn
	dragNone // an Alt+press removed or merged a cursor: drags do nothing
)

// pending is the input collected for finish.
type pending struct {
	// events are the keys and committed text in arrival order
	// (input.KeyEvent, input.TextInput, imeCommit), applied by applyEvents.
	events   []input.Event
	press    bool
	pressPos geom.Point
	clicks   int
	shift    bool
	alt      bool
	drag     bool
	dragPos  geom.Point
	// ptr is the pointer position after the frame's last pointer event;
	// cancel: the system cancelled the view's pointer capture.
	ptr    geom.Point
	cancel bool
	// release: the primary button was released over the view's press this
	// frame, at relPos (the release event's own position, not moved by a
	// later move in the same frame; Context.ReleasePos).
	release bool
	relPos  geom.Point
	// menu is the command chosen in the context menu this frame (menu.go).
	menu *menuRun
}

// inputState is the View's retained input state (embedded in viewState).
type inputState struct {
	pend pending

	dragUnit     int
	dragA, dragB int // range selected by the press that started the drag
	// dragMode is how a drag changes the selections: dragPrimary (the one
	// selection), dragAdded (the cursor an Alt+press added, the others,
	// dragBase, staying), dragColumn (a column box from colLine/colCol) or
	// dragNone.
	dragMode        int
	dragBase        []Selection
	colLine, colCol int
	// dnd is a drag of selected text in progress (dnd.go), nil for none.
	dnd *dndState

	// Goal x of vertical moves: valid while the caret stays where the last
	// vertical move put it in the same text.
	goalX    float32
	goalHead int
	goalVer  uint64
	goalOK   bool

	// IME composition shown at the caret, and a composition cancelled by
	// losing focus that the Context may still report after refocus.
	comp      string
	compCur   int
	imeActive bool // a non-empty composition was seen and not committed yet
	staleComp string
	stale     bool

	// Keymap of the view when the application sets none (ViewW.Keymap),
	// and whether the view was focused in an active window last frame (a
	// pending chord is cleared when that ends).
	defKeymap *keymap.Keymap
	hadFocus  bool

	// Caret blink: restarted by input and caret moves.
	activity   time.Time
	blinkKey   blinkKey
	blinkFocus bool

	// Content x of the caret when its line was last drawn (paintCaret):
	// the IME caret rect while the caret line is scrolled out of view.
	lastCaret lastCaretX
}

type lastCaretX struct {
	ok   bool
	ver  uint64
	head int
	x    float32
}

type blinkKey struct {
	ver     uint64
	sels    uint64 // selsKey
	comp    string
	compCur int
}

// imeCommit is committed text that ends an IME composition: an undo step
// of its own (see commitText).
type imeCommit struct{ input.TextInput }

// collect takes this frame's input for the view from the Context.
//
// Keys and committed text come from Context.EventsFor in arrival order, so
// Backspace then "a" in one frame is not "a" then Backspace. Composition
// events in that stream only mark the text committed after them as an IME
// commit; the composition shown is the Context's current one
// (CompositionFor).
func (w *ViewW) collect() {
	c, id, vs := w.c, w.box.ID(), w.vs
	p := &vs.pend
	if !w.ia.Focused {
		// Losing focus cancels the composition: nothing is written, the
		// text input client goes off (WantTextInput is not called).
		if vs.comp != "" {
			vs.staleComp, vs.stale = vs.comp, true
		}
		vs.comp, vs.compCur, vs.imeActive = "", 0, false
		return
	}
	// While an IME composition is in progress keys belong to the input
	// method: they are not resolved as shortcuts (a Cmd+D while composing
	// neither interrupts the composition nor runs a command).
	composing := vs.comp != ""
	for _, e := range c.EventsFor(id) {
		switch e := e.(type) {
		case input.KeyEvent:
			if !composing {
				p.events = append(p.events, e)
			}
		case input.TextInput:
			if vs.imeActive {
				p.events = append(p.events, imeCommit{e})
			} else {
				p.events = append(p.events, e)
			}
			vs.imeActive, vs.stale = false, false
			composing = false
		case input.Composition:
			live := e.Text != "" && !(vs.stale && e.Text == vs.staleComp)
			if live {
				vs.imeActive = true
			}
			composing = live
		}
	}
	comp, cc := c.CompositionFor(id)
	if vs.stale {
		if comp == vs.staleComp {
			comp, cc = "", 0
		} else {
			vs.stale = false
		}
	}
	vs.comp, vs.compCur = comp, cc
	if comp != "" {
		vs.imeActive = true
	}
	pos, _ := c.Pointer()
	if w.ia.PressedNow {
		// 1 single, 2 double, 3 triple; a fourth click starts over.
		p.press, p.pressPos, p.clicks = true, pos, (c.PressCount()-1)%3+1
		p.shift = c.PressMods().Has(input.ModShift)
		p.alt = c.PressMods().Has(input.ModAlt)
	}
	if c.Dragging(id) {
		p.drag, p.dragPos = true, pos
	}
	p.ptr, p.cancel = pos, c.PointerCancelled(id)
	// Only the first build pass of a frame sees the release: keep it.
	if rp, ok := c.ReleasePos(id); ok {
		p.release, p.relPos = true, rp
	}
}

// finish applies the collected input once per ViewW.
func (w *ViewW) finish() {
	if w.done {
		return
	}
	w.done = true
	vs, st := w.vs, w.st
	p := vs.pend
	vs.pend = pending{}
	// Losing focus or the window's activation cancels a pending chord or
	// double tap (chords have no timeout).
	active := w.active()
	if vs.hadFocus && !active {
		w.keymap().ClearPending()
	}
	vs.hadFocus = active
	ver := st.Snapshot().Version()
	sel := selsKey(st.Sels)
	if p.press {
		w.press(p)
	}
	// A drag of text sees the pointer before this frame's keys: a
	// Shift+Arrow arriving in the release frame is applied after the drop
	// (keys and pointer are not ordered against each other in a frame).
	if vs.dnd != nil {
		w.dndInput(p)
	} else if p.drag {
		w.drag(p.dragPos)
	}
	if p.release {
		if l := vs.placeholderPress; l >= 0 && w.placeholderAt(p.relPos) == l {
			st.toggleFoldAt(l, false)
		}
		vs.placeholderPress = -1
	}
	if r := p.menu; r != nil {
		st.execute(r.cmd, r.args, w.keyContext())
	}
	w.applyEvents(p.events)
	if st.Snapshot().Version() != ver {
		w.changed = true
	}
	if p.press || p.drag || len(p.events) > 0 || p.menu != nil || selsKey(st.Sels) != sel {
		vs.activity = w.c.Now()
	}
	// The context menu opens before the next frame's modifiers: it uses
	// these.
	m := &vs.menu
	m.ro, m.on, m.seen, m.km, m.ctxFn = w.opts.ReadOnly, w.opts.ContextMenu, true, w.keymap(), w.ctxFn
}

// applyEvents applies keys and committed text in order: the one consumer
// of keyboard input. Keys go through the keymap (key; releases matter only
// to double-tap bindings); compositions are shown, never applied.
func (w *ViewW) applyEvents(evs []input.Event) {
	swallow := false // the last key press was a keybinding: drop its text
	for _, e := range evs {
		switch e := e.(type) {
		case input.KeyEvent:
			if c := w.key(e); e.Down {
				swallow = c && typesText(e)
			}
		case input.TextInput:
			if swallow {
				swallow = false
				continue
			}
			if w.chordText(e.Text) {
				continue
			}
			w.commitText(e.Text, false)
		case imeCommit:
			w.keymap().ClearPending()
			w.commitText(e.Text, true)
		}
	}
}

// commitText inserts committed text (typing, IME commit, dictation): typed
// text goes through the typing assistance (State.Type: auto-closing,
// overtype, surround, ...); a commit that ends an IME composition is
// inserted as is, as an undo step of its own.
func (w *ViewW) commitText(s string, ime bool) {
	if w.opts.ReadOnly || s == "" {
		return
	}
	if len(s) == 1 && (s[0] < 0x20 || s[0] == 0x7f) {
		return // control characters arrive as keys (Enter, Tab, Backspace)
	}
	if ime {
		w.st.insertStep(s)
		return
	}
	w.st.Type(s)
}

// insertStep is Insert as an undo step of its own (IME commits, paste):
// text replaces every selection.
func (s *State) insertStep(text string) {
	if text == "" && s.allEmpty() {
		return
	}
	s.replaceSels(kindOther, func(int, int, int) string { return text })
}

// selsKey is a fingerprint of the selections (FNV-1a), to notice that
// they changed.
func selsKey(sels []Selection) uint64 {
	h := uint64(14695981039346656037)
	mix := func(v int) {
		h ^= uint64(v)
		h *= 1099511628211
	}
	mix(len(sels))
	for _, x := range sels {
		mix(x.Anchor)
		mix(x.Head)
	}
	return h
}

// typesText reports whether a key press may also arrive as committed text
// (a character key without Ctrl or Cmd: "c" ending the chord "mod+k c", or
// ⌥L typing "¬" on macOS), which a keybinding that consumed the key drops.
func typesText(k input.KeyEvent) bool {
	if k.Mods&(input.ModCtrl|input.ModMeta) != 0 {
		return false
	}
	return k.Key.Rune() != 0 || k.Code.Rune() != 0
}

// key applies one key event through the keymap and reports whether the
// keymap consumed it (a command ran, a chord started, or the key cancelled
// a pending chord): the committed text the same key produces, if any
// (typesText), is then dropped (applyEvents). Keys no rule binds do nothing; typing reaches the
// text as TextInput.
func (w *ViewW) key(k input.KeyEvent) bool {
	km := w.keymap()
	was := km.Pending()
	cmd, args, pending := km.ResolveEnabled(k, w.keyContext(), w.commandEnabled)
	if cmd != "" {
		w.st.Execute(cmd, args)
		return k.Down
	}
	return k.Down && (pending || was)
}

// chordText handles committed text that arrives without a KeyEvent (macOS
// and Linux send text-producing keys as text only; see package keymap):
// while a chord is pending, the text stands for the chord's next key — a
// single US-layout character is resolved as that key press
// (keymap.EventForText), anything else cancels the chord — and is dropped
// either way, reported true. Otherwise the text voids a double tap in
// progress (Shift held for a capital letter) and is typed (false).
func (w *ViewW) chordText(text string) bool {
	km := w.keymap()
	if !km.Pending() {
		km.ResetTap()
		return false
	}
	ev, ok := keymap.EventForText(text)
	if !ok {
		km.ClearPending()
		return true
	}
	if cmd, args, _ := km.ResolveEnabled(ev, w.keyContext(), w.commandEnabled); cmd != "" {
		w.st.Execute(cmd, args)
	}
	return true
}

// commandEnabled reports whether the keybinding of command may fire: a
// registered command's Precondition holds and, for Mutates commands, the
// view is writable (Monaco folds the precondition into the keybinding's
// when). Unknown commands count as enabled.
func (w *ViewW) commandEnabled(id string) bool {
	commands.RLock()
	c, ok := commands.m[id]
	commands.RUnlock()
	if !ok {
		return true
	}
	if c.pre != nil && !c.pre.Eval(w.keyContext()) {
		return false
	}
	return !c.Mutates || !w.st.readOnly()
}

// hasGeom reports whether a frame has been painted (geometry for hit
// testing and caret x).
func (w *ViewW) hasGeom() bool { return w.vs.colW > 0 && w.st.Viewport.LineHeight > 0 }

// reveal scrolls the primary caret into view after a keyboard action.
func (w *ViewW) reveal() { w.revealOff(w.st.Primary().Head) }

// revealOff scrolls the caret position off into view.
func (w *ViewW) revealOff(off int) {
	st := w.st
	snap := st.Snapshot()
	up := clampOff(snap, off) == clampOff(snap, st.Primary().Head) && st.caretUp()
	st.revealAt(off, up)
	if !w.hasGeom() || st.Viewport.Width <= 0 {
		return
	}
	if x, ok := w.contentX(snap, off); ok {
		st.RevealX(x, x+2)
	}
}

// vertical moves the caret n lines (view lines while wrapping) up or down
// to the layout x of the caret (kept across consecutive vertical moves);
// page also scrolls by the same number of lines. Above the first line it
// goes to the document start, below the last to its end. Past the end of
// a row that continues on the next one the caret stays at that row's end
// (State.caretUp), as Monaco's view cursor does. Without geometry, and
// with several cursors (each keeping its own goal column on the column
// grid), it falls back to State.Move.
func (w *ViewW) vertical(d Direction, n int, page, extend bool) {
	st, vs := w.st, w.vs
	if !w.hasGeom() || len(st.Sels) > 1 {
		unit := Line
		if page {
			unit = Page
		}
		st.Move(d, unit, extend)
		return
	}
	snap := st.Snapshot()
	head := clampOff(snap, st.Primary().Head)
	up := st.caretUp()
	x := vs.goalX
	if !vs.goalOK || vs.goalHead != head || vs.goalVer != snap.Version() {
		x, _ = w.contentXUp(snap, head, up)
	}
	line, row := st.rowPos(head, up)
	l, r, ok := st.stepRows(line, row, d, n)
	var off int
	toUp := false
	switch {
	case !ok && d == Backward:
		off = 0
	case !ok:
		off = snap.Len()
	default:
		lv := w.rowAt(snap, l, r)
		col := w.colAtX(&lv, x)
		off = snap.LineStart(l) + col
		toUp = !lv.last() && col == lv.b1
	}
	st.MoveCaret(off, extend)
	if toUp {
		st.setCaretUp(true)
	}
	if page {
		dy := float32(n) * st.Viewport.LineHeight
		if d == Backward {
			dy = -dy
		}
		st.ScrollBy(dy, 0)
	}
	vs.goalX, vs.goalHead, vs.goalVer, vs.goalOK = x, st.Primary().Head, snap.Version(), true
}

// press handles a primary button press at p.
func (w *ViewW) press(p pending) {
	st, vs := w.st, w.vs
	if !w.hasGeom() {
		return
	}
	if w.foldControlPress(p) {
		return
	}
	// A click on a folded line's placeholder unfolds it when released on
	// the same placeholder (Monaco's FoldingController.onEditorMouseDown /
	// onEditorMouseUp), so a drag starting there only selects.
	vs.placeholderPress = w.placeholderAt(p.pressPos)
	snap := st.Snapshot()
	off, up := w.hitAt(snap, p.pressPos)
	vs.dragMode, vs.dragBase = dragPrimary, nil
	if w.dndStart(p, snap, off) {
		return // picked up the selection (dnd.go)
	}
	switch {
	case p.alt && p.shift:
		d := st.columnData()
		line, col := w.visualAt(snap, p.pressPos)
		vs.dragMode, vs.colLine, vs.colCol = dragColumn, d.fromLine, d.fromCol
		st.ColumnSelect(d.fromLine, d.fromCol, line, col)
		return
	case p.shift && p.clicks == 1:
		st.MoveCaret(off, true)
		st.setCaretUp(up)
		a := st.Primary().Anchor
		vs.dragUnit, vs.dragA, vs.dragB = unitChar, a, a
		return
	case p.alt:
		vs.dragUnit = min(p.clicks-1, unitLine)
		a, b := w.unitRange(snap, off, vs.dragUnit)
		vs.dragA, vs.dragB = a, b
		if a == b {
			n := len(st.sels())
			st.AddCursor(a)
			if len(st.Sels) < n {
				vs.dragMode = dragNone // removed the selection clicked in
				return
			}
		} else {
			st.setSels(append(slices.Clone(st.sels()), Selection{Anchor: a, Head: b}), nil)
		}
		vs.dragMode, vs.dragBase = dragAdded, slices.Clone(st.Sels)
		// The added cursor is the last one unless it merged.
		if k := len(vs.dragBase) - 1; k > 0 && vs.dragBase[k] == (Selection{Anchor: a, Head: b}) {
			vs.dragBase = vs.dragBase[:k]
		} else {
			vs.dragMode, vs.dragBase = dragNone, nil // merged into another
		}
		return
	}
	vs.dragUnit = min(p.clicks-1, unitLine)
	a, b := w.unitRange(snap, off, vs.dragUnit)
	vs.dragA, vs.dragB = a, b
	w.setSel(a, b)
	st.setCaretUp(up && a == b)
}

// foldControlPress handles a press on a folding control in the gutter:
// it folds or unfolds the region starting on that line (with Shift
// recursively, Monaco's FoldingController.onEditorMouseUp), and reports
// whether it did. The caret does not move.
func (w *ViewW) foldControlPress(p pending) bool {
	vs, st := w.vs, w.st
	if vs.foldX < 0 || !st.foldingOn() {
		return false
	}
	pt := p.pressPos
	if !vs.gutter.Contains(pt) || pt.X < vs.foldX || pt.X >= vs.foldX+foldControlW {
		return false
	}
	r := w.pointRow(pt)
	if r < 0 || r >= st.viewCount() {
		return false
	}
	line, row := st.lines().find(r)
	if row != 0 {
		return false
	}
	return st.toggleFoldAt(line, p.shift)
}

// placeholderAt returns the folded line whose placeholder is at window
// point pt, -1 for none.
func (w *ViewW) placeholderAt(pt geom.Point) int {
	vs, st := w.vs, w.st
	if len(st.fold.folds) == 0 || !vs.text.Contains(pt) {
		return -1
	}
	r := w.pointRow(pt)
	if r < 0 || r >= st.viewCount() {
		return -1
	}
	line, row := st.lines().find(r)
	if st.fold.foldAt(line) < 0 {
		return -1
	}
	snap := st.Snapshot()
	lv := w.rowAt(snap, line, row)
	if !lv.last() {
		return -1
	}
	x0, x1 := w.placeholderX(&lv)
	if x := pt.X - vs.text.X + st.Viewport.ScrollX; x >= x0-1 && x < x1 {
		return line
	}
	return -1
}

// visualAt returns the line and the visual column (State.ColumnSelect) at
// window point p, on the View's column grid; lines are clamped to the
// document, columns may lie past the line end. On a row of a wrapped line
// the column counts from the model line's start (the columns before the
// row plus those into it after its indentation; column selection works on
// model lines).
func (w *ViewW) visualAt(snap buffer.Snapshot, p geom.Point) (line, col int) {
	vs, v := w.vs, w.st.Viewport
	x := p.X - vs.text.X + v.ScrollX
	col = max(0, int(math.Round(float64(x/vs.colW))))
	line, row := w.st.lines().find(w.pointRow(p))
	if w.st.lines().on() {
		text := snap.Line(line)
		b0, _, ind := w.st.rowText(line, row, text)
		col = w.st.lineColumn(text, b0) + max(0, col-ind)
	}
	return line, col
}

// pointRow returns the view line at window y of p (not clamped).
func (w *ViewW) pointRow(p geom.Point) int {
	vs, v := w.vs, w.st.Viewport
	return v.TopLine + int(math.Floor(float64((p.Y-vs.text.Y+v.OffsetY)/v.LineHeight)))
}

// drag extends the selection of the press to the pointer at p.
func (w *ViewW) drag(p geom.Point) {
	st, vs := w.st, w.vs
	if !w.hasGeom() {
		return
	}
	snap := st.Snapshot()
	if vs.dragMode == dragNone {
		return
	}
	if vs.dragMode == dragColumn {
		line, col := w.visualAt(snap, p)
		st.ColumnSelect(vs.colLine, vs.colCol, line, col)
		if !vs.text.Contains(p) {
			w.revealOff(st.columnHead())
		}
		return
	}
	off, up := w.hitAt(snap, p)
	a, b := w.unitRange(snap, off, vs.dragUnit)
	sel := Selection{Anchor: vs.dragA, Head: max(b, vs.dragB)}
	if a < vs.dragA {
		sel = Selection{Anchor: vs.dragB, Head: a}
	}
	if vs.dragMode == dragAdded {
		st.setSels(append(slices.Clone(vs.dragBase), sel), nil)
		if !vs.text.Contains(p) {
			w.revealOff(sel.Head)
		}
		return
	}
	w.setSel(sel.Anchor, sel.Head)
	st.setCaretUp(up && sel.Head == off)
	if !vs.text.Contains(p) {
		w.reveal() // dragging past an edge scrolls
	}
}

// setSel sets the only selection.
func (w *ViewW) setSel(anchor, head int) {
	st := w.st
	if len(st.Sels) == 1 && st.Sels[0] == (Selection{Anchor: anchor, Head: head}) {
		return
	}
	st.MoveCaret(anchor, false)
	st.MoveCaret(head, true)
}

// unitRange returns the range of the unit (char: empty, word, line with
// its line break) at off.
func (w *ViewW) unitRange(snap buffer.Snapshot, off, unit int) (int, int) {
	switch unit {
	case unitWord:
		return w.wordAt(snap, off)
	case unitLine:
		l := snap.LineOf(off)
		if l+1 < snap.LineCount() {
			return snap.LineStart(l), snap.LineStart(l + 1)
		}
		return snap.LineStart(l), snap.Len()
	}
	return off, off
}

// wordAt returns the word at off (by the language's word definition), else
// the run of blanks or the grapheme cluster at off.
func (w *ViewW) wordAt(snap buffer.Snapshot, off int) (int, int) {
	l := snap.LineOf(off)
	ls := snap.LineStart(l)
	text := snap.Line(l)
	col := off - ls
	ws := w.st.words(text)
	for _, m := range ws {
		if m[0] <= col && col < m[1] {
			return ls + m[0], ls + m[1]
		}
	}
	for _, m := range ws {
		if col == m[1] {
			return ls + m[0], ls + m[1]
		}
	}
	if col >= len(text) {
		return off, off
	}
	blank := func(b byte) bool { return b == ' ' || b == '\t' }
	if blank(text[col]) {
		a, b := col, col
		for a > 0 && blank(text[a-1]) {
			a--
		}
		for b < len(text) && blank(text[b]) {
			b++
		}
		return ls + a, ls + b
	}
	return off, ls + nextGrapheme(text, col)
}

// offsetAt returns the document offset at window point p (the last
// painted geometry): the nearest grapheme boundary of the view line under
// p; above the first line the document start, below the last its end.
func (w *ViewW) offsetAt(snap buffer.Snapshot, p geom.Point) int {
	off, _ := w.hitAt(snap, p)
	return off
}

// hitAt is offsetAt, also reporting whether p is past the end of a row
// that continues on the next one (the offset is that row's end: see
// State.caretUp).
func (w *ViewW) hitAt(snap buffer.Snapshot, p geom.Point) (off int, up bool) {
	vs, v := w.vs, w.st.Viewport
	r := w.pointRow(p)
	switch {
	case r < 0:
		return 0, false
	case r >= w.st.viewCount():
		return snap.Len(), false
	}
	line, row := w.st.lines().find(r)
	lv := w.rowAt(snap, line, row)
	col := w.colAtX(&lv, p.X-vs.text.X+v.ScrollX)
	return snap.LineStart(line) + col, !lv.last() && col == lv.b1
}

// rowAt lays out row of model line i as paint does (through the same
// LineCache): the whole line without wrapping.
func (w *ViewW) rowAt(snap buffer.Snapshot, i, row int) lineView {
	th := themeOf(w.st)
	hl := w.st.Highlighter()
	if w.st.lines().on() {
		row = max(0, min(row, w.st.breaks(i).rows()-1))
		if rows := w.layoutWrapped(nil, snap, i, row, row, hl, th, w.vs.colW); len(rows) == 1 {
			return rows[0]
		}
	}
	lv := w.layoutLine(snap, i, hl, th, w.vs.colW)
	lv.vi = w.st.lines().viewOf(i)
	if lv.long != nil {
		w.layoutChunks(&lv, th, w.vs.colW)
	}
	return lv
}

// contentX returns the content x of the caret before offset off, from the
// layout of its view line (the column grid for parts of long lines that
// are not shaped); at a row boundary the primary caret's row affinity
// (State.caretUp) applies. It needs a painted frame.
func (w *ViewW) contentX(snap buffer.Snapshot, off int) (float32, bool) {
	off = clampOff(snap, off)
	return w.contentXUp(snap, off, off == clampOff(snap, w.st.Primary().Head) && w.st.caretUp())
}

// contentXUp is contentX at the end of the earlier row (up) or the start
// of the later one at a row boundary.
func (w *ViewW) contentXUp(snap buffer.Snapshot, off int, up bool) (float32, bool) {
	if !w.hasGeom() {
		return 0, false
	}
	off = clampOff(snap, off)
	l, row := w.st.rowPos(off, up)
	lv := w.rowAt(snap, l, row)
	if x, ok := lv.caretX(lv.runeAt(off - snap.LineStart(l))); ok {
		return x, true
	}
	return float32(w.st.viewColumn(snap, off, up)) * w.vs.colW, true
}

// colAtX returns the byte column of lv nearest content x, on a grapheme
// cluster boundary.
func (w *ViewW) colAtX(lv *lineView, x float32) int {
	colW := w.vs.colW
	grid := func() int { // column grid (unshaped parts of long lines)
		return w.st.offsetForColumn(lv.text, int(math.Round(float64(x/colW))))
	}
	if len(lv.segs) == 0 {
		if lv.long != nil {
			return grid()
		}
		return 0
	}
	k := 0
	for k+1 < len(lv.segs) && x >= lv.segs[k+1].x0 {
		k++
	}
	sg := lv.segs[k]
	if lv.long != nil && ((k == 0 && x < sg.x0 && sg.r0 > 0) ||
		(k == len(lv.segs)-1 && x > sg.x0+sg.lay.Width && sg.r1 < lv.runes)) {
		return grid()
	}
	s := lv.text[sg.b0:sg.b1]
	lx := x - sg.x0
	ri := sg.lay.IndexAt(lx, sg.lay.LineHeight/2)
	col := byteOfRune(s, ri)
	xAt := func(c int) float32 {
		cx, _, _ := sg.lay.Caret(utf8.RuneCountInString(s[:c]))
		return cx
	}
	return sg.b0 + snapCluster(s, col, lx, xAt)
}

// byteOfRune returns the byte offset of rune index ri of s.
func byteOfRune(s string, ri int) int {
	if ri <= 0 {
		return 0
	}
	n := 0
	for i := range s {
		if n == ri {
			return i
		}
		n++
	}
	return len(s)
}

// snapCluster moves byte column col of s, when it falls inside a grapheme
// cluster, to the cluster edge whose caret is nearer x.
func snapCluster(s string, col int, x float32, xAt func(int) float32) int {
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		if col > i && col < j {
			if abs32f(xAt(i)-x) <= abs32f(xAt(j)-x) {
				return i
			}
			return j
		}
		if j > col {
			break
		}
		i = j
	}
	return col
}

func abs32f(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
