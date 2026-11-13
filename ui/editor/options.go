package editor

// DefaultFontSize is the default Options.FontSize in logical pixels.
//
// Monaco 0.56's fontSize default is platform dependent (12 on macOS, 14
// elsewhere); the editor uses 14 everywhere so the layout is identical on
// every platform (milestone 1).
const DefaultFontSize = 14

// DefaultTabSize is a new Model's tab size (Monaco's tabSize default).
const DefaultTabSize = 4

// LineNumbersMode is how the gutter shows line numbers (Monaco's
// lineNumbers option).
type LineNumbersMode string

const (
	// LineNumbersOn shows absolute line numbers.
	LineNumbersOn LineNumbersMode = "on"
	// LineNumbersOff hides the line numbers.
	LineNumbersOff LineNumbersMode = "off"
)

// Options are one editor's settings (Monaco's editor options). Each field
// defaults to Monaco 0.56's default (DefaultOptions); the comment names the
// Monaco option.
//
// The tab size and insertSpaces are not here: like Monaco's model options
// (ITextModel.updateOptions) they belong to the document, so every view of
// it indents alike; see Model.TabSize and Model.InsertSpaces.
type Options struct {
	// FontSize is the font size in logical pixels (fontSize; see
	// DefaultFontSize). Values <= 0 mean DefaultFontSize.
	FontSize float32
	// ReadOnly refuses edits: every command marked Mutates (typing,
	// deleting, Enter, Tab, paste, undo, redo, the line commands ...), the
	// IME, the deletion of cut and dragging text; the caret, selection,
	// navigation and copy keep working, and the context key
	// editorReadonly is true (readOnly: false).
	ReadOnly bool
	// LineNumbers shows or hides the line numbers (lineNumbers: "on").
	// Milestone 2 Task 2 supports LineNumbersOn and LineNumbersOff; any
	// other value shows them.
	LineNumbers LineNumbersMode
	// Comments are the settings of the comment commands (comments).
	Comments CommentsOptions

	// AutoClosingBrackets is when typing an opening bracket also inserts
	// its closing bracket (autoClosingBrackets: "languageDefined").
	AutoClosingBrackets AutoClosingStrategy
	// AutoClosingQuotes is when typing a quote also inserts the closing
	// quote (autoClosingQuotes: "languageDefined").
	AutoClosingQuotes AutoClosingStrategy
	// AutoClosingComments is when typing the start of a block comment
	// pair (TypeScript's "/**") also inserts its end
	// (autoClosingComments: "languageDefined").
	AutoClosingComments AutoClosingStrategy
	// AutoClosingOvertype is when typing a closing bracket or quote types
	// over the same character after the caret (autoClosingOvertype:
	// "auto", only over characters the editor auto-inserted).
	AutoClosingOvertype AutoClosingEditStrategy
	// AutoClosingDelete is when Backspace between an opening and a closing
	// character deletes both (autoClosingDelete: "auto", only a pair the
	// editor auto-closed).
	AutoClosingDelete AutoClosingEditStrategy
	// AutoSurround is when typing a bracket or quote over a selection
	// wraps it instead of replacing it (autoSurround: "languageDefined").
	AutoSurround AutoSurroundStrategy
	// AutoIndent is how Enter, Tab on a blank line, typing, moving lines
	// and inserting lines adjust the indentation (autoIndent: "full").
	AutoIndent AutoIndentStrategy

	// EmptySelectionClipboard makes Copy and Cut with a caret (no
	// selection) copy or cut its whole line, and Paste of such a copy
	// insert the line above the caret's line (emptySelectionClipboard:
	// true).
	EmptySelectionClipboard bool
	// MultiCursorPaste is how pasted text is shared among several cursors
	// (multiCursorPaste: "spread"); unset or unknown values are "spread".
	MultiCursorPaste MultiCursorPasteMode
	// DragAndDrop lets the mouse drag the selected text to another place,
	// moving it, or copying it with Alt (Option) on macOS and Ctrl
	// elsewhere (dragAndDrop: true).
	DragAndDrop bool
	// ContextMenu shows the context menu on a right click in the text
	// (contextmenu: true; menu.go). Off, a right click only focuses the
	// view and moves the caret there when it is outside the selections.
	ContextMenu bool

	// WordWrap is when long lines wrap onto several view lines (wordWrap:
	// "off"; wrap.go); unset or unknown values are "off".
	// editor.action.toggleWordWrap (Alt+Z) overrides it per State.
	WordWrap WrapMode
	// WordWrapColumn is the wrapping column of WrapColumn and WrapBounded
	// (wordWrapColumn: 80); values <= 0 mean 80.
	WordWrapColumn int
	// WrappingIndent is the indentation of continuation rows
	// (wrappingIndent: "same"); unset or unknown values are "same".
	WrappingIndent WrappingIndentMode
}

// AutoClosingStrategy is when a pair is auto-closed (Monaco's
// EditorAutoClosingStrategy).
type AutoClosingStrategy string

const (
	// AutoCloseAlways closes the pair wherever the caret is.
	AutoCloseAlways AutoClosingStrategy = "always"
	// AutoCloseLanguageDefined closes the pair when the character after
	// the caret is one of the language's autoCloseBefore characters (or a
	// closing bracket), and not in the tokens its notIn lists.
	AutoCloseLanguageDefined AutoClosingStrategy = "languageDefined"
	// AutoCloseBeforeWhitespace closes the pair when a blank or the line
	// end follows the caret.
	AutoCloseBeforeWhitespace AutoClosingStrategy = "beforeWhitespace"
	// AutoCloseNever never closes the pair.
	AutoCloseNever AutoClosingStrategy = "never"
)

// AutoClosingEditStrategy is when typing over or deleting an auto-closed
// character acts on the pair (Monaco's EditorAutoClosingEditStrategy).
type AutoClosingEditStrategy string

const (
	// AutoClosingEditAlways acts on any matching pair.
	AutoClosingEditAlways AutoClosingEditStrategy = "always"
	// AutoClosingEditAuto acts only on characters the editor auto-closed,
	// while the caret stays between the pair.
	AutoClosingEditAuto AutoClosingEditStrategy = "auto"
	// AutoClosingEditNever never acts on the pair.
	AutoClosingEditNever AutoClosingEditStrategy = "never"
)

// AutoSurroundStrategy is which characters typed over a selection wrap it
// (Monaco's EditorAutoSurroundStrategy).
type AutoSurroundStrategy string

const (
	// AutoSurroundLanguageDefined wraps with the language's surrounding
	// pairs, brackets and quotes.
	AutoSurroundLanguageDefined AutoSurroundStrategy = "languageDefined"
	// AutoSurroundQuotes wraps with quotes only.
	AutoSurroundQuotes AutoSurroundStrategy = "quotes"
	// AutoSurroundBrackets wraps with brackets only.
	AutoSurroundBrackets AutoSurroundStrategy = "brackets"
	// AutoSurroundNever never wraps.
	AutoSurroundNever AutoSurroundStrategy = "never"
)

// AutoIndentStrategy is how much the editor adjusts the indentation
// (Monaco's autoIndent option, EditorAutoIndentStrategy).
type AutoIndentStrategy string

const (
	// AutoIndentNone: Enter inserts a bare line break.
	AutoIndentNone AutoIndentStrategy = "none"
	// AutoIndentKeep: Enter keeps the current line's indentation.
	AutoIndentKeep AutoIndentStrategy = "keep"
	// AutoIndentBrackets: also indents after an opening bracket and
	// splits a bracket pair over three lines.
	AutoIndentBrackets AutoIndentStrategy = "brackets"
	// AutoIndentAdvanced: also applies the language's onEnter rules.
	AutoIndentAdvanced AutoIndentStrategy = "advanced"
	// AutoIndentFull: also applies the language's indentation rules on
	// Enter, on typing, on Tab in a blank line and when moving lines.
	AutoIndentFull AutoIndentStrategy = "full"
)

// autoIndentLevels orders the strategies as Monaco's enum (0 none .. 4
// full).
var autoIndentLevels = map[AutoIndentStrategy]int{
	AutoIndentNone: 0, AutoIndentKeep: 1, AutoIndentBrackets: 2, AutoIndentAdvanced: 3, AutoIndentFull: 4,
}

// Monaco's autoIndent levels.
const (
	indentNone = iota
	indentKeep
	indentBrackets
	indentAdvanced
	indentFull
)

// autoIndent returns the AutoIndent level; unset or unknown values are
// the default ("full").
func (o *Options) autoIndent() int {
	if v, ok := autoIndentLevels[o.AutoIndent]; ok {
		return v
	}
	return indentFull
}

// closing returns an auto-closing strategy, the default for unset or
// unknown values.
func closing(v AutoClosingStrategy) AutoClosingStrategy {
	switch v {
	case AutoCloseAlways, AutoCloseBeforeWhitespace, AutoCloseNever:
		return v
	}
	return AutoCloseLanguageDefined
}

// closingEdit returns an auto-closing edit strategy, the default for unset
// or unknown values.
func closingEdit(v AutoClosingEditStrategy) AutoClosingEditStrategy {
	switch v {
	case AutoClosingEditAlways, AutoClosingEditNever:
		return v
	}
	return AutoClosingEditAuto
}

// surround returns the AutoSurround strategy, the default for unset or
// unknown values.
func (o *Options) surround() AutoSurroundStrategy {
	switch o.AutoSurround {
	case AutoSurroundQuotes, AutoSurroundBrackets, AutoSurroundNever:
		return o.AutoSurround
	}
	return AutoSurroundLanguageDefined
}

// CommentsOptions are the settings of the comment commands (Monaco's
// comments option).
type CommentsOptions struct {
	// InsertSpace puts a space after a line comment token and inside the
	// tokens of a block comment (comments.insertSpace: true).
	InsertSpace bool
	// IgnoreEmptyLines leaves blank lines alone when toggling, adding or
	// removing line comments over several lines (comments.ignoreEmptyLines:
	// true).
	IgnoreEmptyLines bool
}

// DefaultOptions returns the default Options.
func DefaultOptions() Options {
	return Options{
		FontSize:    DefaultFontSize,
		LineNumbers: LineNumbersOn,
		Comments:    CommentsOptions{InsertSpace: true, IgnoreEmptyLines: true},

		AutoClosingBrackets: AutoCloseLanguageDefined,
		AutoClosingQuotes:   AutoCloseLanguageDefined,
		AutoClosingComments: AutoCloseLanguageDefined,
		AutoClosingOvertype: AutoClosingEditAuto,
		AutoClosingDelete:   AutoClosingEditAuto,
		AutoSurround:        AutoSurroundLanguageDefined,
		AutoIndent:          AutoIndentFull,

		EmptySelectionClipboard: true,
		MultiCursorPaste:        MultiCursorPasteSpread,
		DragAndDrop:             true,
		ContextMenu:             true,

		WordWrap:       WrapOff,
		WordWrapColumn: DefaultWordWrapColumn,
		WrappingIndent: WrapIndentSame,
	}
}

// fontSize returns FontSize, or the default when unset.
func (o *Options) fontSize() float32 {
	if o.FontSize > 0 {
		return o.FontSize
	}
	return DefaultFontSize
}

// lineNumbers reports whether the gutter shows line numbers.
func (o *Options) lineNumbers() bool { return o.LineNumbers != LineNumbersOff }

// wordWrap returns WordWrap, "off" for unset or unknown values.
func (o *Options) wordWrap() WrapMode {
	switch o.WordWrap {
	case WrapOn, WrapColumn, WrapBounded:
		return o.WordWrap
	}
	return WrapOff
}
