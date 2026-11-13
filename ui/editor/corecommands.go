package editor

import (
	"strings"
)

// The editor's built-in commands (milestone 1's key set), Monaco's IDs. The
// default keybindings are keymap.DefaultRules.

func init() {
	for _, c := range coreCommands() {
		RegisterCommand(c)
	}
}

// writable is the precondition of Monaco's editing commands.
const writable = "!" + CtxEditorReadonly

func coreCommands() []Command {
	var cs []Command
	move := func(id, label string, d Direction, u Unit) {
		for _, sel := range []bool{false, true} {
			c := Command{ID: id, Label: label, Run: func(st *State, _ any) bool {
				st.Move(d, u, sel)
				st.revealCaret()
				return true
			}}
			if sel {
				c.ID += "Select"
				c.Label += " Select"
			}
			cs = append(cs, c)
		}
	}
	vertical := func(id, label string, d Direction, page bool) {
		for _, sel := range []bool{false, true} {
			c := Command{ID: id, Label: label, Run: func(st *State, _ any) bool {
				st.verticalCmd(d, page, sel)
				return true
			}}
			if sel {
				c.ID += "Select"
				c.Label += " Select"
			}
			cs = append(cs, c)
		}
	}
	move("cursorLeft", "Cursor Left", Backward, Char)
	move("cursorRight", "Cursor Right", Forward, Char)
	vertical("cursorUp", "Cursor Up", Backward, false)
	vertical("cursorDown", "Cursor Down", Forward, false)
	vertical("cursorPageUp", "Cursor Page Up", Backward, true)
	vertical("cursorPageDown", "Cursor Page Down", Forward, true)
	move("cursorHome", "Cursor Home", Backward, LineEdge)
	move("cursorEnd", "Cursor End", Forward, LineEdge)
	move("cursorTop", "Cursor Top", Backward, Doc)
	move("cursorBottom", "Cursor Bottom", Forward, Doc)

	del := func(id, label string, d Direction, u Unit) {
		cs = append(cs, Command{ID: id, Label: label, Precondition: writable, Mutates: true, Run: func(st *State, _ any) bool {
			v := st.Snapshot().Version()
			// Backspace between an auto-closed pair deletes both
			// (Monaco's deleteLeft, autoclose.go).
			if d == Backward && u == Char && st.deletePairs() {
				st.revealCaret()
				return true
			}
			st.Delete(d, u)
			st.revealCaret()
			return st.Snapshot().Version() != v
		}})
	}
	del("deleteLeft", "Delete Left", Backward, Char)
	del("deleteRight", "Delete Right", Forward, Char)
	del("deleteAllLeft", "Delete All Left", Backward, LineEdge)
	del("deleteAllRight", "Delete All Right", Forward, LineEdge)

	cs = append(cs,
		Command{ID: "editor.action.selectAll", Label: "Select All", Run: func(st *State, _ any) bool {
			st.MoveCaret(0, false)
			st.MoveCaret(st.Snapshot().Len(), true)
			return true
		}},
		Command{ID: "type", Label: "Type", Precondition: writable, Mutates: true, Run: cmdType},
		// Tab: spaces to the next tab stop, or indent the selected lines
		// (indent.go).
		Command{ID: "tab", Label: "Tab", Precondition: writable, Mutates: true, Run: func(st *State, _ any) bool {
			ok := st.tab()
			st.revealCaret()
			return ok
		}},
		Command{ID: "editor.action.toggleTabFocusMode", Label: "Toggle Tab Key Moves Focus", Run: func(st *State, _ any) bool {
			st.tabFocus = !st.tabFocus
			return true
		}},
		// Word wrap on and off for this editor (VS Code's
		// editor.action.toggleWordWrap; wrap.go).
		Command{ID: "editor.action.toggleWordWrap", Label: "View: Toggle Word Wrap", Run: func(st *State, _ any) bool {
			st.ToggleWordWrap()
			return true
		}},
		Command{ID: "undo", Label: "Undo", Mutates: true, Run: func(st *State, _ any) bool {
			ok := st.Undo()
			st.revealCaret()
			return ok
		}},
		Command{ID: "redo", Label: "Redo", Mutates: true, Run: func(st *State, _ any) bool {
			ok := st.Redo()
			st.revealCaret()
			return ok
		}},
	)
	return cs
}

// TypeArgs are the arguments of the type command (VS Code's {text}),
// typed through the typing assistance (State.Type); a
// map[string]any or map[string]string with a "text" entry, or a plain
// string, works too.
type TypeArgs struct{ Text string }

// cmdType types text as from the keyboard (State.Type): a line break is
// Enter with the language's onEnter and indentation rules, brackets and
// quotes auto-close (autoclose.go, onenter.go). The interceptors always
// run, as for Monaco's type from the keyboard; to insert text as is, use
// State.Insert (or the paste command).
func cmdType(st *State, args any) bool {
	var text string
	switch a := args.(type) {
	case string:
		text = a
	case TypeArgs:
		text = a.Text
	case *TypeArgs:
		if a != nil {
			text = a.Text
		}
	case map[string]any:
		text, _ = a["text"].(string)
	case map[string]string:
		text = a["text"]
	}
	if text == "" {
		return false
	}
	st.Type(text)
	st.revealCaret()
	return true
}

// revealCaret scrolls the caret into view after a command: through the
// view that last showed the State (horizontally too), else vertically.
func (s *State) revealCaret() {
	if s.view != nil {
		s.view.reveal()
		return
	}
	s.RevealCaret()
}

// revealOffset scrolls caret position off into view after a command:
// through the view that last showed the State (horizontally too), else
// vertically.
func (s *State) revealOffset(off int) {
	if s.view != nil {
		s.view.revealOff(off)
		return
	}
	snap := s.Snapshot()
	s.revealAt(off, clampOff(snap, off) == clampOff(snap, s.Primary().Head) && s.caretUp())
}

// verticalCmd moves the caret a line or a page up or down: by layout x
// through the view that last showed the State, else by column.
func (s *State) verticalCmd(d Direction, page, extend bool) {
	if w := s.view; w != nil {
		n := 1
		if page {
			n = s.pageLines()
		}
		w.vertical(d, n, page, extend)
		w.reveal()
		return
	}
	u := Line
	if page {
		u = Page
	}
	s.Move(d, u, extend)
	s.RevealCaret()
}

// tabText is what Tab inserts at offset a (a selection start): spaces to
// the next tab stop (Model.InsertSpaces), else a tab character.
func (s *State) tabText(a int) string {
	if !s.insertSpaces() {
		return "\t"
	}
	ts := s.indentSize()
	col := s.columnWith(s.Snapshot(), a, ts)
	return strings.Repeat(" ", ts-col%ts)
}

// insertSpaces is the Model's InsertSpaces.
func (s *State) insertSpaces() bool { return s.Model.InsertSpaces() }
