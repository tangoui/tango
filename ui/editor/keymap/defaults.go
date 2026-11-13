package keymap

// Default keybindings: Monaco 0.56's default keybindings for the commands
// the editor implements (package editor registers them), checked against
// MinCode's curated table (vue-app/src/domain/keybindings/monaco-defaults.ts)
// where it lists the command. A command the editor does not implement yet
// is not listed; each later milestone task appends the defaults of the
// commands it adds to defaultBindings.
//
// When clauses are Monaco's keybinding when (kbExpr), using the editor's
// context keys. Monaco's writable precondition ("!editorReadonly") is the
// command's Precondition / Mutates in package editor, not part of the when,
// except where MinCode's table spells it in the when (deleteAllLeft,
// deleteAllRight). Deviations from Monaco:
//
//   - enter → type {"text": "\n"}: Monaco types Enter through its hidden
//     textarea; the editor binds it to VS Code's type command, which runs
//     Monaco's Enter (onEnter and indentation rules).
//   - ctrl+m → editor.action.toggleTabFocusMode also on macOS, besides
//     Monaco's ctrl+shift+m (milestone 1 used Ctrl+M on every platform).
//   - undo and redo use when editorTextFocus (MinCode's table) where
//     Monaco registers them without one.
//   - cursorWordLeft/... use textInputFocus without Monaco's
//     accessibility-mode clause on Windows.

const (
	textInput   = "textInputFocus"
	textFocus   = "editorTextFocus"
	editorFocus = "editorFocus"
	writable    = "editorTextFocus && !editorReadonly"
)

// platforms is a set of platforms (bit 1<<Platform); 0 means all.
type platforms uint8

const (
	onMac     = platforms(1 << Mac)
	onWindows = platforms(1 << Windows)
	onLinux   = platforms(1 << Linux)
	onPC      = onWindows | onLinux
)

// defaultBinding is a row of the default table.
type defaultBinding struct {
	command string
	key     string
	on      platforms
	when    string
	args    any
}

var defaultBindings = []defaultBinding{
	// Cursor movement (Monaco coreCommands).
	{"cursorLeft", "left", 0, textInput, nil},
	{"cursorLeft", "ctrl+b", onMac, textInput, nil},
	{"cursorLeftSelect", "shift+left", 0, textInput, nil},
	{"cursorRight", "right", 0, textInput, nil},
	{"cursorRight", "ctrl+f", onMac, textInput, nil},
	{"cursorRightSelect", "shift+right", 0, textInput, nil},
	{"cursorUp", "up", 0, textInput, nil},
	{"cursorUp", "ctrl+p", onMac, textInput, nil},
	{"cursorUpSelect", "shift+up", 0, textInput, nil},
	{"cursorDown", "down", 0, textInput, nil},
	{"cursorDown", "ctrl+n", onMac, textInput, nil},
	{"cursorDownSelect", "shift+down", 0, textInput, nil},
	{"cursorPageUp", "pageup", 0, textInput, nil},
	{"cursorPageUpSelect", "shift+pageup", 0, textInput, nil},
	{"cursorPageDown", "pagedown", 0, textInput, nil},
	{"cursorPageDownSelect", "shift+pagedown", 0, textInput, nil},
	{"cursorHome", "home", 0, textInput, nil},
	{"cursorHome", "cmd+left", onMac, textInput, nil},
	{"cursorHomeSelect", "shift+home", 0, textInput, nil},
	{"cursorHomeSelect", "shift+cmd+left", onMac, textInput, nil},
	{"cursorEnd", "end", 0, textInput, nil},
	{"cursorEnd", "cmd+right", onMac, textInput, nil},
	{"cursorEndSelect", "shift+end", 0, textInput, nil},
	{"cursorEndSelect", "shift+cmd+right", onMac, textInput, nil},
	{"cursorTop", "ctrl+home", onPC, textInput, nil},
	{"cursorTop", "cmd+up", onMac, textInput, nil},
	{"cursorTopSelect", "ctrl+shift+home", onPC, textFocus, nil},
	{"cursorTopSelect", "shift+cmd+up", onMac, textFocus, nil},
	{"cursorBottom", "ctrl+end", onPC, textInput, nil},
	{"cursorBottom", "cmd+down", onMac, textInput, nil},
	{"cursorBottomSelect", "ctrl+shift+end", onPC, textFocus, nil},
	{"cursorBottomSelect", "shift+cmd+down", onMac, textFocus, nil},

	// Word movement (Monaco wordOperations).
	{"cursorWordLeft", "ctrl+left", onPC, textInput, nil},
	{"cursorWordLeft", "alt+left", onMac, textInput, nil},
	{"cursorWordLeftSelect", "ctrl+shift+left", onPC, textInput, nil},
	{"cursorWordLeftSelect", "alt+shift+left", onMac, textInput, nil},
	{"cursorWordEndRight", "ctrl+right", onPC, textInput, nil},
	{"cursorWordEndRight", "alt+right", onMac, textInput, nil},
	{"cursorWordEndRightSelect", "ctrl+shift+right", onPC, textInput, nil},
	{"cursorWordEndRightSelect", "alt+shift+right", onMac, textInput, nil},
	{"cursorWordPartLeft", "ctrl+alt+left", onMac, textInput, nil},
	{"cursorWordPartLeftSelect", "ctrl+alt+shift+left", onMac, textInput, nil},
	{"cursorWordPartRight", "ctrl+alt+right", onMac, textInput, nil},
	{"cursorWordPartRightSelect", "ctrl+alt+shift+right", onMac, textInput, nil},

	// Selection.
	{"editor.action.selectAll", "mod+a", 0, "", nil},

	// Deleting.
	{"deleteLeft", "backspace", 0, textInput, nil},
	{"deleteLeft", "shift+backspace", 0, textInput, nil},
	{"deleteLeft", "ctrl+h", onMac, textInput, nil},
	{"deleteLeft", "ctrl+backspace", onMac, textInput, nil},
	{"deleteRight", "delete", 0, textInput, nil},
	{"deleteRight", "ctrl+d", onMac, textInput, nil},
	{"deleteRight", "ctrl+delete", onMac, textInput, nil},
	{"deleteWordLeft", "ctrl+backspace", onPC, textInput, nil},
	{"deleteWordLeft", "alt+backspace", onMac, textInput, nil},
	{"deleteWordRight", "ctrl+delete", onPC, textInput, nil},
	{"deleteWordRight", "alt+delete", onMac, textInput, nil},
	{"deleteWordPartLeft", "ctrl+alt+backspace", onMac, textInput, nil},
	{"deleteWordPartRight", "ctrl+alt+delete", onMac, textInput, nil},
	{"deleteAllLeft", "cmd+backspace", onMac, writable, nil},
	{"deleteAllRight", "ctrl+k", onMac, writable, nil},
	{"deleteAllRight", "cmd+delete", onMac, writable, nil},

	// Typing.
	{"type", "enter", 0, writable, map[string]any{"text": "\n"}},
	{"tab", "tab", 0, "editorTextFocus && !editorTabMovesFocus", nil},
	{"editor.action.toggleTabFocusMode", "ctrl+shift+m", onMac, "", nil},
	{"editor.action.toggleTabFocusMode", "ctrl+m", 0, "", nil},

	// View (VS Code's workbench binding; Monaco's standalone editor has
	// no toggleWordWrap).
	{"editor.action.toggleWordWrap", "alt+z", 0, "", nil},

	// History.
	{"undo", "mod+z", 0, textFocus, nil},
	{"redo", "ctrl+y", onPC, textFocus, nil},
	{"redo", "ctrl+shift+z", onPC, textFocus, nil},
	{"redo", "shift+cmd+z", onMac, textFocus, nil},

	// Multiple cursors (Monaco multicursor contribution, coreCommands,
	// cursorUndo contribution). Column selection is Monaco's
	// Cmd+Shift+Alt / Ctrl+Shift+Alt + arrows, with no Linux binding;
	// Shift+Alt+arrows stay word selection on macOS and add cursors on
	// Linux.
	{"editor.action.insertCursorAbove", "alt+cmd+up", onMac, textFocus, nil},
	{"editor.action.insertCursorBelow", "alt+cmd+down", onMac, textFocus, nil},
	{"editor.action.insertCursorAbove", "ctrl+alt+up", onWindows, textFocus, nil},
	{"editor.action.insertCursorBelow", "ctrl+alt+down", onWindows, textFocus, nil},
	{"editor.action.insertCursorAbove", "shift+alt+up", onLinux, textFocus, nil},
	{"editor.action.insertCursorBelow", "shift+alt+down", onLinux, textFocus, nil},
	{"editor.action.insertCursorAbove", "ctrl+shift+up", onLinux, textFocus, nil},
	{"editor.action.insertCursorBelow", "ctrl+shift+down", onLinux, textFocus, nil},
	{"editor.action.insertCursorAtEndOfEachLineSelected", "shift+alt+i", 0, textFocus, nil},
	{"editor.action.addSelectionToNextFindMatch", "mod+d", 0, editorFocus, nil},
	{"editor.action.selectHighlights", "mod+shift+l", 0, editorFocus, nil},
	{"removeSecondaryCursors", "escape", 0, textInput, nil},
	{"removeSecondaryCursors", "shift+escape", 0, textInput, nil},
	{"cursorUndo", "mod+u", 0, textInput, nil},
	{"cursorColumnSelectLeft", "mod+shift+alt+left", onMac | onWindows, textInput, nil},
	{"cursorColumnSelectRight", "mod+shift+alt+right", onMac | onWindows, textInput, nil},
	{"cursorColumnSelectUp", "mod+shift+alt+up", onMac | onWindows, textInput, nil},
	{"cursorColumnSelectDown", "mod+shift+alt+down", onMac | onWindows, textInput, nil},
	{"cursorColumnSelectPageUp", "mod+shift+alt+pageup", onMac | onWindows, textInput, nil},
	{"cursorColumnSelectPageDown", "mod+shift+alt+pagedown", onMac | onWindows, textInput, nil},

	// Line operations (Monaco linesOperations), indentation (coreCommands
	// outdent) and comments (comment contribution). The when is MinCode's
	// (writable) where its table lists the command, else Monaco's kbExpr.
	{"editor.action.copyLinesUpAction", "shift+alt+up", onMac | onWindows, writable, nil},
	{"editor.action.copyLinesUpAction", "ctrl+shift+alt+up", onLinux, writable, nil},
	{"editor.action.copyLinesDownAction", "shift+alt+down", onMac | onWindows, writable, nil},
	{"editor.action.copyLinesDownAction", "ctrl+shift+alt+down", onLinux, writable, nil},
	{"editor.action.moveLinesUpAction", "alt+up", 0, writable, nil},
	{"editor.action.moveLinesDownAction", "alt+down", 0, writable, nil},
	{"editor.action.deleteLines", "mod+shift+k", 0, writable, nil},
	{"editor.action.joinLines", "ctrl+j", onMac, writable, nil},
	{"editor.action.insertLineAfter", "mod+enter", 0, writable, nil},
	{"editor.action.insertLineBefore", "mod+shift+enter", 0, writable, nil},
	{"editor.action.trimTrailingWhitespace", "mod+k mod+x", 0, writable, nil},
	{"outdent", "shift+tab", 0, "editorTextFocus && !editorTabMovesFocus", nil},
	{"editor.action.indentLines", "mod+]", 0, textFocus, nil},
	{"editor.action.outdentLines", "mod+[", 0, textFocus, nil},
	{"editor.action.commentLine", "mod+/", 0, writable, nil},
	{"editor.action.addCommentLine", "mod+k mod+c", 0, writable, nil},
	{"editor.action.removeCommentLine", "mod+k mod+u", 0, writable, nil},
	{"editor.action.blockComment", "shift+alt+a", onMac | onWindows, writable, nil},
	{"editor.action.blockComment", "ctrl+shift+a", onLinux, writable, nil},

	// Clipboard.
	{"editor.action.clipboardCopyAction", "mod+c", 0, textInput, nil},
	{"editor.action.clipboardCopyAction", "ctrl+insert", onWindows, textInput, nil},
	{"editor.action.clipboardCutAction", "mod+x", 0, textInput, nil},
	{"editor.action.clipboardCutAction", "shift+delete", onWindows, textInput, nil},
	{"editor.action.clipboardPasteAction", "mod+v", 0, textInput, nil},
	{"editor.action.clipboardPasteAction", "shift+insert", onWindows, textInput, nil},
}

// DefaultRules returns the default rules for platform p (see the table
// above), lowest priority first.
//
// The table lists a command's primary key first, then its secondary keys
// (Monaco's kbOpts primary / secondary). Monaco registers the secondary
// keys with a lower weight, so its primary key is the command's
// highest-priority binding: the one menus show (lookupPrimaryKeybinding)
// and Keymap.Keys returns first. DefaultRules therefore emits each run of
// rows of one command in reverse order. The rows of a command have
// different keys, so this changes no key's resolution.
func DefaultRules(p Platform) []Rule {
	var out []Rule
	run := 0 // start in out of the current command's rows
	for _, d := range defaultBindings {
		if d.on != 0 && d.on&(1<<p) == 0 {
			continue
		}
		if len(out) == 0 || out[len(out)-1].Command != d.command {
			run = len(out)
		}
		out = append(out, Rule{Key: d.key, Command: d.command, When: d.when, Args: d.args})
		copy(out[run+1:], out[run:len(out)-1])
		out[run] = Rule{Key: d.key, Command: d.command, When: d.when, Args: d.args}
	}
	return out
}
