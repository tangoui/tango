package editor

import (
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/when"
)

// Built-in context keys, Monaco's names, for keybinding when clauses and
// command preconditions:
//
//	editorFocus                  the view has keyboard focus (and its window is active)
//	editorTextFocus              same (the editor has no inner widgets yet)
//	textInputFocus               same
//	editorReadonly               edits are refused this frame
//	editorHasSelection           some selection is not empty
//	editorHasMultipleSelections  more than one selection (multiple cursors)
//	editorLangId                 the Model's language id ("go", "plaintext")
//	editorTabMovesFocus          "Tab moves focus" mode (editor.action.toggleTabFocusMode)
//	isMac, isWindows, isLinux    the keymap's platform
//
// The application adds keys of its own with ViewW.Context; its function
// is asked first, so it may also override a built-in key.
const (
	CtxEditorFocus                 = "editorFocus"
	CtxEditorTextFocus             = "editorTextFocus"
	CtxTextInputFocus              = "textInputFocus"
	CtxEditorReadonly              = "editorReadonly"
	CtxEditorHasSelection          = "editorHasSelection"
	CtxEditorHasMultipleSelections = "editorHasMultipleSelections"
	CtxEditorLangID                = "editorLangId"
	CtxEditorTabMovesFocus         = "editorTabMovesFocus"
	CtxIsMac                       = "isMac"
	CtxIsWindows                   = "isWindows"
	CtxIsLinux                     = "isLinux"
)

// contextKeys is the when.Context of a State shown by a view (w may be
// nil: no focus, the current platform).
type contextKeys struct {
	st *State
	w  *ViewW
	// keyFocus: a key is being dispatched to the view, which therefore
	// has focus (the view applies keys after the frame's focus changes).
	keyFocus bool
	// roSet: editorReadonly is ro instead of the view's ReadOnly of this
	// frame (the context menu, built before this frame's modifiers, uses
	// the last frame's; menu.go).
	ro, roSet bool
}

var _ when.Context = contextKeys{}

// contextKeys returns the State's context keys, as of the view that last
// showed it.
func (s *State) contextKeys() contextKeys { return contextKeys{st: s, w: s.view} }

// keyContext is the context of a key the view w dispatches.
func (w *ViewW) keyContext() contextKeys { return contextKeys{st: w.st, w: w, keyFocus: true} }

// Value implements when.Context.
func (c contextKeys) Value(key string) (any, bool) {
	if c.w != nil && c.w.ctxFn != nil {
		if v, ok := c.w.ctxFn(key); ok {
			return v, true
		}
	}
	st := c.st
	switch key {
	case CtxEditorFocus, CtxEditorTextFocus, CtxTextInputFocus:
		return c.keyFocus || (c.w != nil && c.w.active()), true
	case CtxEditorReadonly:
		if c.roSet {
			return c.ro, true
		}
		return st.readOnly(), true
	case CtxEditorHasSelection:
		return !st.allEmpty(), true
	case CtxEditorHasMultipleSelections:
		return len(st.Sels) > 1, true
	case CtxEditorLangID:
		return st.Model.Language(), true
	case CtxEditorTabMovesFocus:
		return st.tabFocus, true
	case CtxIsMac, CtxIsWindows, CtxIsLinux:
		p := keymap.Current()
		if c.w != nil {
			p = c.w.keymap().Platform()
		}
		switch key {
		case CtxIsMac:
			return p == keymap.Mac, true
		case CtxIsWindows:
			return p == keymap.Windows, true
		}
		return p == keymap.Linux, true
	}
	return nil, false
}
