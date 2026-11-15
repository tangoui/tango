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
	// LineNumbersRelative shows each line's distance to the primary
	// caret's line, and that line's own number (left aligned).
	LineNumbersRelative LineNumbersMode = "relative"
	// LineNumbersInterval shows the numbers of every tenth line, of the
	// last line and of the primary caret's line.
	LineNumbersInterval LineNumbersMode = "interval"
)

// RenderWhitespaceMode is which blanks are drawn as visible marks: a
// middle dot for a space, an arrow for a tab (Monaco's renderWhitespace).
type RenderWhitespaceMode string

const (
	// RenderWhitespaceNone draws no marks.
	RenderWhitespaceNone RenderWhitespaceMode = "none"
	// RenderWhitespaceBoundary marks the leading and trailing blanks, tabs,
	// and runs of two or more spaces.
	RenderWhitespaceBoundary RenderWhitespaceMode = "boundary"
	// RenderWhitespaceSelection marks the blanks inside the selections.
	RenderWhitespaceSelection RenderWhitespaceMode = "selection"
	// RenderWhitespaceTrailing marks the trailing blanks (all of a blank
	// line's).
	RenderWhitespaceTrailing RenderWhitespaceMode = "trailing"
	// RenderWhitespaceAll marks every blank.
	RenderWhitespaceAll RenderWhitespaceMode = "all"
)

// MatchBracketsMode is when the brackets around the caret are highlighted
// (Monaco's matchBrackets).
type MatchBracketsMode string

const (
	// MatchBracketsAlways highlights the pair a caret touches, else the
	// pair enclosing it.
	MatchBracketsAlways MatchBracketsMode = "always"
	// MatchBracketsNear highlights only the pair a caret touches.
	MatchBracketsNear MatchBracketsMode = "near"
	// MatchBracketsNever highlights no brackets.
	MatchBracketsNever MatchBracketsMode = "never"
)

// RenderLineHighlightMode is where the caret's line is highlighted
// (Monaco's renderLineHighlight).
type RenderLineHighlightMode string

const (
	// RenderLineHighlightNone highlights nothing.
	RenderLineHighlightNone RenderLineHighlightMode = "none"
	// RenderLineHighlightGutter highlights the line's gutter (also with a
	// selection).
	RenderLineHighlightGutter RenderLineHighlightMode = "gutter"
	// RenderLineHighlightLine highlights the line's text area while every
	// selection is empty.
	RenderLineHighlightLine RenderLineHighlightMode = "line"
	// RenderLineHighlightAll is both.
	RenderLineHighlightAll RenderLineHighlightMode = "all"
)

// GuidesOptions are the guide lines drawn in the text (Monaco's guides).
type GuidesOptions struct {
	// Indentation draws a vertical line at each indentation level
	// (guides.indentation: true).
	Indentation bool
	// HighlightActiveIndentation draws the guide of the primary caret's
	// block with editorIndentGuide.activeBackground
	// (guides.highlightActiveIndentation: true).
	HighlightActiveIndentation bool
	// BracketPairs draws bracket pair guides (guides.bracketPairs:
	// false): a vertical line from each bracket pair's opening line to its
	// closing line, for every pair or only the primary caret's
	// (bracketguides.go). "" is BracketGuidesOff.
	BracketPairs BracketGuideMode
	// BracketPairsHorizontal draws, with the bracket pair guides, the
	// horizontal lines to the brackets (guides.bracketPairsHorizontal:
	// "active"). "" is BracketGuidesOff.
	BracketPairsHorizontal BracketGuideMode
	// HighlightActiveBracketPair draws the guides of the primary caret's
	// pair in editorBracketPairGuide.activeBackground
	// (guides.highlightActiveBracketPair: true).
	HighlightActiveBracketPair bool
}

// BracketGuideMode is which bracket pairs get guides (Monaco's
// guides.bracketPairs and guides.bracketPairsHorizontal: true, "active",
// false).
type BracketGuideMode string

const (
	// BracketGuidesOff draws none.
	BracketGuidesOff BracketGuideMode = "false"
	// BracketGuidesOn draws them for every pair.
	BracketGuidesOn BracketGuideMode = "true"
	// BracketGuidesActive draws them for the primary caret's pair only.
	BracketGuidesActive BracketGuideMode = "active"
)

// PaddingOptions are the empty space above the first line and below the
// last (Monaco's padding), in logical pixels.
type PaddingOptions struct {
	// Top is the space above the first line, scrolled away with the text
	// (padding.top: 0).
	Top float32
	// Bottom is the space below the last line (padding.bottom: 0). As in
	// Monaco with scrollBeyondLastLine, scrolling stops with the last line
	// at the top whatever it is, so it changes nothing visible.
	Bottom float32
}

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
	// LineNumbers is how the gutter shows line numbers (lineNumbers:
	// "on"); unset or unknown values are "on".
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
	// "off"; wrap.go); unset or unknown values are "off". Large files
	// (Model.LargeFile) never wrap.
	// editor.action.toggleWordWrap (Alt+Z) overrides it per State.
	WordWrap WrapMode
	// WordWrapColumn is the wrapping column of WrapColumn and WrapBounded
	// (wordWrapColumn: 80); values <= 0 mean 80.
	WordWrapColumn int
	// WrappingIndent is the indentation of continuation rows
	// (wrappingIndent: "same"); unset or unknown values are "same".
	WrappingIndent WrappingIndentMode

	// Folding enables code folding: the fold commands, the gutter's
	// folding controls and hidden regions (folding: true; folding.go).
	// Large files (Model.LargeFile) have no folding.
	Folding bool
	// FoldingStrategy is how the folding ranges are computed
	// (foldingStrategy: "auto"); unset or unknown values are "auto".
	FoldingStrategy FoldingStrategy
	// FoldingHighlight gives folded regions' start lines a background
	// (foldingHighlight: true).
	FoldingHighlight bool
	// ShowFoldingControls is when the gutter shows the folding controls
	// (showFoldingControls: "mouseover"); unset or unknown values are
	// "mouseover".
	ShowFoldingControls ShowFoldingControls

	// RenderWhitespace is which blanks are drawn as marks
	// (renderWhitespace: "selection"); unset or unknown values are
	// "selection".
	RenderWhitespace RenderWhitespaceMode
	// RenderControlCharacters draws control characters (U+0000–U+001F but
	// tab, and U+007F) as their Control Pictures symbols, white on dark red
	// (renderControlCharacters: true).
	RenderControlCharacters bool
	// Guides are the indentation guides (guides).
	Guides GuidesOptions
	// BracketPairColorization colors brackets by nesting level and
	// unmatched ones as errors (bracketPairColorization.enabled: true).
	// Documents over 5 000 000 bytes (no bracket tree) are not colorized.
	BracketPairColorization bool
	// MatchBrackets is when the bracket pair at the caret is highlighted
	// (matchBrackets: "always"); unset or unknown values are "always".
	MatchBrackets MatchBracketsMode
	// RenderLineHighlight is where the caret's line is highlighted
	// (renderLineHighlight: "line"); unset or unknown values are "line".
	RenderLineHighlight RenderLineHighlightMode
	// Padding is the space above the first line and below the last
	// (padding: {top: 0, bottom: 0}).
	Padding PaddingOptions

	// OccurrencesHighlight highlights, once the caret came to rest, the
	// whole-word, case-sensitive
	// occurrences of the word at the caret (occurrencesHighlight:
	// "singleFile"; occurrences.go). "multiFile" works as "singleFile":
	// only the text of the view's own document is matched, and only
	// textually (no language documentHighlight provider yet). Large files
	// (Model.LargeFile) have none.
	OccurrencesHighlight OccurrencesHighlightMode
	// OccurrencesHighlightDelay is how long, in milliseconds, the
	// occurrences wait to be drawn once the highlighter ran, 50 ms after
	// the caret came to rest (occurrencesHighlightDelay: 0). Values < 0
	// mean 0.
	OccurrencesHighlightDelay int
	// SelectionHighlight highlights the other matches of a selection that
	// spans one line (selectionHighlight: true; occurrences.go), at most
	// 200 characters long (selectionHighlightMaxLength). Large files have
	// none.
	SelectionHighlight bool
	// UnicodeHighlight marks suspicious characters with a box
	// (unicodeHighlight; unicode.go).
	UnicodeHighlight UnicodeHighlightOptions
	// Links underlines the URLs found in the text (Monaco's LinkComputer);
	// Cmd+click (Ctrl+click elsewhere) hands the URL to the host through
	// ViewW.OnOpenLink (links: true; links.go). Documents over 50 MB (the
	// limit of Monaco's editor worker) have none.
	Links bool
	// ColorDecorators draws a swatch before each CSS color literal (#rgb,
	// #rgba, #rrggbb, #rrggbbaa, rgb(), rgba(), hsl(), hsla()); a click on
	// it edits the color with a picker (colorDecorators: true; colors.go).
	// Documents over 50 MB have none.
	ColorDecorators bool
	// MouseWheelZoom makes Cmd+wheel (Ctrl+wheel elsewhere) change the
	// editor zoom level, as the editor.action.fontZoomIn/Out/Reset
	// commands do (mouseWheelZoom: false; see SetZoomLevel).
	MouseWheelZoom bool

	// StickyScroll pins enclosing folding range starts (stickyScroll).
	StickyScroll StickyScrollOptions
	// Minimap is the token-colored document preview (minimap).
	Minimap MinimapOptions
	// OverviewRulerLanes is the number of marker lanes (overviewRulerLanes: 3).
	// Zero hides the ruler; values are clamped to 0..3.
	OverviewRulerLanes int
	// OverviewRulerBorder draws the left border (overviewRulerBorder: true).
	OverviewRulerBorder bool

	// zoom is the editor zoom level in effect this frame (View copies
	// ZoomLevel into the View's options): fontSize scales by 1 + zoom/10.
	zoom float64
}

// OccurrencesHighlightMode is where the occurrences of the word at the
// caret are highlighted (Monaco's occurrencesHighlight).
type OccurrencesHighlightMode string

const (
	// OccurrencesOff highlights nothing.
	OccurrencesOff OccurrencesHighlightMode = "off"
	// OccurrencesSingleFile highlights the occurrences in the document.
	OccurrencesSingleFile OccurrencesHighlightMode = "singleFile"
	// OccurrencesMultiFile is Monaco's experimental cross-document mode;
	// the View treats it as OccurrencesSingleFile.
	OccurrencesMultiFile OccurrencesHighlightMode = "multiFile"
)

// UnicodeHighlightSetting is an option of Monaco's unicodeHighlight that
// takes true, false or "inUntrustedWorkspace".
type UnicodeHighlightSetting string

const (
	UnicodeHighlightTrue  UnicodeHighlightSetting = "true"
	UnicodeHighlightFalse UnicodeHighlightSetting = "false"
	// UnicodeHighlightInUntrustedWorkspace is on in an untrusted
	// workspace only. The editor has no workspace trust: it counts as
	// trusted, so this is false.
	UnicodeHighlightInUntrustedWorkspace UnicodeHighlightSetting = "inUntrustedWorkspace"
)

// on reports whether the setting is on (trusted workspace).
func (v UnicodeHighlightSetting) on() bool { return v == UnicodeHighlightTrue }

// UnicodeHighlightOptions are the suspicious characters marked with a box
// (Monaco's unicodeHighlight; unicode.go).
type UnicodeHighlightOptions struct {
	// NonBasicASCII marks every character outside basic ASCII
	// (unicodeHighlight.nonBasicASCII: "inUntrustedWorkspace", off here).
	NonBasicASCII UnicodeHighlightSetting
	// InvisibleCharacters marks characters that take no visible space
	// (unicodeHighlight.invisibleCharacters: true).
	InvisibleCharacters bool
	// AmbiguousCharacters marks characters that look like basic ASCII
	// ones, as Cyrillic "а" looks like "a" (unicodeHighlight
	// .ambiguousCharacters: true).
	AmbiguousCharacters bool
	// IncludeComments also marks characters in comments
	// (unicodeHighlight.includeComments: "inUntrustedWorkspace", off here).
	IncludeComments UnicodeHighlightSetting
	// IncludeStrings also marks characters in strings
	// (unicodeHighlight.includeStrings: true).
	IncludeStrings bool
	// AllowedCharacters are never marked (unicodeHighlight
	// .allowedCharacters, a set of characters in Monaco; here a string of
	// them).
	AllowedCharacters string
	// AllowedLocales are the locales whose ambiguous characters tables
	// apply (unicodeHighlight.allowedLocales: {_os: true, _vscode: true},
	// the zero LocaleSet; "_os" and "_vscode" are SetSystemLocales'
	// locales). A character ambiguous in one of these locales' own
	// alphabets is not marked: AllowLocales("ru") stops marking the
	// Cyrillic "а" (unicode_locales.go). A set, not a map, so Options
	// stays comparable.
	AllowedLocales LocaleSet
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
		StickyScroll:        StickyScrollOptions{Enabled: true, MaxLineCount: 5},
		Minimap:             MinimapOptions{Enabled: true, Side: MinimapRight, Size: MinimapProportional, MaxColumn: 120, RenderCharacters: true, ShowSlider: MinimapSliderMouseOver},
		OverviewRulerLanes:  3,
		OverviewRulerBorder: true,
		FontSize:            DefaultFontSize,
		LineNumbers:         LineNumbersOn,
		Comments:            CommentsOptions{InsertSpace: true, IgnoreEmptyLines: true},

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

		Folding:             true,
		FoldingStrategy:     FoldingAuto,
		FoldingHighlight:    true,
		ShowFoldingControls: FoldingControlsMouseOver,

		RenderWhitespace:        RenderWhitespaceSelection,
		RenderControlCharacters: true,
		Guides: GuidesOptions{Indentation: true, HighlightActiveIndentation: true,
			BracketPairs: BracketGuidesOff, BracketPairsHorizontal: BracketGuidesActive, HighlightActiveBracketPair: true},
		BracketPairColorization: true,
		MatchBrackets:           MatchBracketsAlways,
		RenderLineHighlight:     RenderLineHighlightLine,

		OccurrencesHighlight:      OccurrencesSingleFile,
		OccurrencesHighlightDelay: DefaultOccurrencesHighlightDelay,
		SelectionHighlight:        true,
		UnicodeHighlight: UnicodeHighlightOptions{
			NonBasicASCII:       UnicodeHighlightInUntrustedWorkspace,
			InvisibleCharacters: true,
			AmbiguousCharacters: true,
			IncludeComments:     UnicodeHighlightInUntrustedWorkspace,
			IncludeStrings:      true,
		},
		Links:           true,
		ColorDecorators: true,
	}
}

// DefaultOccurrencesHighlightDelay is DefaultOptions'
// OccurrencesHighlightDelay, in milliseconds.
const DefaultOccurrencesHighlightDelay = 0

// lineNumbersMode returns LineNumbers, "on" for unset or unknown values.
func (o *Options) lineNumbersMode() LineNumbersMode {
	switch o.LineNumbers {
	case LineNumbersOff, LineNumbersRelative, LineNumbersInterval:
		return o.LineNumbers
	}
	return LineNumbersOn
}

// renderWhitespace returns RenderWhitespace, "selection" for unset or
// unknown values.
func (o *Options) renderWhitespace() RenderWhitespaceMode {
	switch o.RenderWhitespace {
	case RenderWhitespaceNone, RenderWhitespaceBoundary, RenderWhitespaceTrailing, RenderWhitespaceAll:
		return o.RenderWhitespace
	}
	return RenderWhitespaceSelection
}

// matchBrackets returns MatchBrackets, "always" for unset or unknown
// values.
func (o *Options) matchBrackets() MatchBracketsMode {
	switch o.MatchBrackets {
	case MatchBracketsNear, MatchBracketsNever:
		return o.MatchBrackets
	}
	return MatchBracketsAlways
}

// bracketGuides returns Guides.BracketPairs and
// Guides.BracketPairsHorizontal in effect: "" is off, an unknown value
// Monaco's default (off; "active").
func (o *Options) bracketGuides() (pairs, horizontal BracketGuideMode) {
	pairs, horizontal = o.Guides.BracketPairs, o.Guides.BracketPairsHorizontal
	switch pairs {
	case BracketGuidesOn, BracketGuidesActive:
	default:
		pairs = BracketGuidesOff
	}
	switch horizontal {
	case BracketGuidesOn, BracketGuidesActive, BracketGuidesOff:
	case "":
		horizontal = BracketGuidesOff
	default:
		horizontal = BracketGuidesActive
	}
	return pairs, horizontal
}

// lineHighlight returns whether the caret's line is highlighted in the
// gutter and in the text area (RenderLineHighlight; unset or unknown:
// "line").
func (o *Options) lineHighlight() (gutter, line bool) {
	switch o.RenderLineHighlight {
	case RenderLineHighlightNone:
		return false, false
	case RenderLineHighlightGutter:
		return true, false
	case RenderLineHighlightAll:
		return true, true
	}
	return false, true
}

// foldingStrategy returns FoldingStrategy, "auto" for unset or unknown
// values.
func (o *Options) foldingStrategy() FoldingStrategy {
	if o.FoldingStrategy == FoldingIndentation {
		return FoldingIndentation
	}
	return FoldingAuto
}

// foldingControls returns ShowFoldingControls, "mouseover" for unset or
// unknown values.
func (o *Options) foldingControls() ShowFoldingControls {
	switch o.ShowFoldingControls {
	case FoldingControlsAlways, FoldingControlsNever:
		return o.ShowFoldingControls
	}
	return FoldingControlsMouseOver
}

// fontSize returns FontSize, or the default when unset, scaled by the
// zoom level in effect.
func (o *Options) fontSize() float32 {
	sz := float32(DefaultFontSize)
	if o.FontSize > 0 {
		sz = o.FontSize
	}
	if o.zoom != 0 {
		sz *= float32(1 + o.zoom*0.1) // Monaco's EditorZoom multiplier
	}
	return sz
}

// occurrences reports whether the word at the caret is highlighted.
func (o *Options) occurrences() bool { return o.OccurrencesHighlight != OccurrencesOff }

// lineNumbers reports whether the gutter shows line numbers.
func (o *Options) lineNumbers() bool { return o.lineNumbersMode() != LineNumbersOff }

// wordWrap returns WordWrap, "off" for unset or unknown values.
func (o *Options) wordWrap() WrapMode {
	switch o.WordWrap {
	case WrapOn, WrapColumn, WrapBounded:
		return o.WordWrap
	}
	return WrapOff
}
