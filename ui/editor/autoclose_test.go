package editor

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
)

// Test cases for the typing assistance. Monaco 0.56's npm package ships no
// unit tests, so the cases are derived from the behaviour of its sources
// (cursorTypeEditOperations, cursorDeleteOperations, onEnter, enterAction,
// autoIndent, moveLinesCommand, cursorWordOperations) with the language
// configurations exported from it.

// seqCase runs ops on in (marked text, see lineops_test.go): an op starting
// with "#" executes that command, any other is typed (State.Type).
type seqCase struct {
	name string
	file string
	in   string
	ops  []string
	want string
	opts func(*State)
}

func runSeq(t *testing.T, cases []seqCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := seqState(t, c.file, c.in, c.opts)
			for _, op := range c.ops {
				if id, ok := strings.CutPrefix(op, "#"); ok {
					st.Execute(id, nil)
				} else {
					st.Type(op)
				}
			}
			if got := marked(st); got != c.want {
				t.Errorf("%q:\n got %q\nwant %q", c.ops, got, c.want)
			}
		})
	}
}

func seqState(t *testing.T, file, in string, opts func(*State)) *State {
	t.Helper()
	if file == "" {
		file = "a.txt"
	}
	text, sels := parseMarked(t, in)
	st := NewState(text, file, theme.Base("vs"))
	if opts != nil {
		opts(st)
	}
	st.SetSelections(sels)
	// Highlight the document first: the token checks are skipped while the
	// lines above the caret are not final, and the short synchronous
	// catch-up may not finish on a loaded machine (-race).
	if hl := st.Highlighter(); hl != nil {
		hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), time.Minute)
	}
	return st
}

func TestAutoCloseRules(t *testing.T) {
	never := func(st *State) { st.Options.AutoClosingBrackets = AutoCloseNever }
	ws := func(st *State) { st.Options.AutoClosingBrackets = AutoCloseBeforeWhitespace }
	always := func(st *State) { st.Options.AutoClosingBrackets = AutoCloseAlways }
	overAlways := func(st *State) { st.Options.AutoClosingOvertype = AutoClosingEditAlways }
	delAlways := func(st *State) { st.Options.AutoClosingDelete = AutoClosingEditAlways }
	noSurround := func(st *State) { st.Options.AutoSurround = AutoSurroundNever }
	quotesOnly := func(st *State) { st.Options.AutoSurround = AutoSurroundQuotes }
	runSeq(t, []seqCase{
		// Auto-closing: before the line end, a blank or an autoCloseBefore
		// character, not before a word.
		{"bracket at the line end", "a.go", "f|", []string{"("}, "f(|)", nil},
		{"bracket before a blank", "a.go", "f| x", []string{"("}, "f(|) x", nil},
		{"bracket before a word", "a.go", "|foo", []string{"("}, "(|foo", nil},
		{"bracket before a closing bracket", "a.go", "a(|)", []string{"("}, "a((|))", nil},
		{"bracket before ;", "a.go", "|;", []string{"("}, "(|);", nil},
		{"every bracket", "a.go", "|", []string{"[", "{"}, "[{|}]", nil},
		{"never", "a.go", "|", []string{"("}, "(|", never},
		{"beforeWhitespace before ;", "a.go", "|;", []string{"("}, "(|;", ws},
		{"beforeWhitespace before a blank", "a.go", "| x", []string{"("}, "(|) x", ws},
		{"always before a word", "a.go", "|foo", []string{"("}, "(|)foo", always},
		{"plain text has no pairs", "a.txt", "|", []string{"("}, "(|", nil},
		// Quotes: not after a word character, not in a string (notIn).
		{"quote", "a.go", "x := |", []string{"\""}, "x := \"|\"", nil},
		{"quote after a word", "a.go", "a|", []string{"\""}, "a\"|", nil},
		{"single quote in a string", "a.go", "x := \"a | b\"", []string{"'"}, "x := \"a '| b\"", nil},
		{"single quote outside a string", "a.go", "x := |", []string{"'"}, "x := '|'", nil},
		{"single quote in a comment", "a.go", "// |", []string{"'"}, "// '|", nil},
		{"double quote in a comment", "a.go", "// |", []string{"\""}, "// \"|\"", nil},
		{"quote ending a string", "a.go", "x := \"ab|", []string{"\""}, "x := \"ab\"|", nil},
		{"quote in an unterminated string", "a.go", "x := \"a |", []string{"\""}, "x := \"a \"|", nil},
		// Only the neutral-character check stops this one: the token before
		// the caret is the template's "}", but a character typed there would
		// be in the template string.
		{"quote after a template interpolation", "a.ts", "x = `${a}| `", []string{"\""}, "x = `${a}\"| `", nil},
		{"quote after an interpolation outside", "a.ts", "x = (a)| ", []string{"\""}, "x = (a)\"|\" ", nil},
		// A longer opening sequence: TypeScript's /** */ (in a comment).
		{"doc comment", "a.ts", "/*|", []string{"*"}, "/**| */", nil},
		{"doc comment typed", "a.ts", "|", []string{"/**"}, "/**| */", nil},
		// Several cursors.
		{"carets", "a.go", "a|\nb|", []string{"("}, "a(|)\nb(|)", nil},
		// Overtype: only over a character the editor inserted, while the
		// caret stays inside the pair.
		{"overtype", "a.go", "f|", []string{"(", ")"}, "f()|", nil},
		{"overtype after typing inside", "a.go", "f|", []string{"(", "x", ")"}, "f(x)|", nil},
		{"overtype a quote", "a.go", "x := |", []string{"\"", "a", "\""}, "x := \"a\"|", nil},
		{"no overtype of a typed close", "a.go", "f(|)", []string{")"}, "f()|)", nil},
		{"overtype always", "a.go", "f(|)", []string{")"}, "f()|", overAlways},
		{"leaving the pair ends overtype", "a.go", "f|", []string{"(", "#cursorRight", "#cursorLeft", ")"}, "f()|)", nil},
		{"overtype at carets", "a.go", "a|\nb|", []string{"(", ")"}, "a()|\nb()|", nil},
		{"nested overtype", "a.go", "|", []string{"(", "[", "]", ")"}, "([])|", nil},
		// Backspace deletes an auto-closed pair.
		{"paired delete", "a.go", "f|", []string{"(", "#deleteLeft"}, "f|", nil},
		{"paired delete of quotes", "a.go", "x := |", []string{"\"", "#deleteLeft"}, "x := |", nil},
		{"no paired delete of a typed pair", "a.go", "f(|)", []string{"#deleteLeft"}, "f|)", nil},
		{"paired delete always", "a.go", "f(|)", []string{"#deleteLeft"}, "f|", delAlways},
		{"deleteWordLeft deletes the pair", "a.go", "f|", []string{"(", "#deleteWordLeft"}, "f|", nil},
		// Surround.
		{"surround", "a.go", "a[foo]b", []string{"("}, "a([foo])b", nil},
		{"surround reversed selection", "a.go", "a]foo[b", []string{"("}, "a([foo])b", nil},
		{"surround with quotes", "a.go", "[foo]", []string{"\""}, "\"[foo]\"", nil},
		{"surround lines", "a.go", "[a\nb]", []string{"{"}, "{[a\nb]}", nil},
		{"surround selections", "a.go", "[a] [b]", []string{"["}, "[[a]] [[b]]", nil},
		{"no surround of blanks", "a.go", "[  ]", []string{"("}, "(|", nil},
		{"no surround of a quote by a quote", "a.go", "[\"]", []string{"'"}, "'|", nil},
		{"surround never", "a.go", "[foo]", []string{"("}, "(|", noSurround},
		{"surround quotes only", "a.go", "[foo]", []string{"("}, "(|", quotesOnly},
		{"surround quotes only, quote", "a.go", "[foo]", []string{"'"}, "'[foo]'", quotesOnly},
	})
}

// TestAutoCloseUndo: auto-closing starts an undo step that the typing after
// it continues; surround is a step of its own.
func TestAutoCloseUndo(t *testing.T) {
	st := seqState(t, "a.go", "x|", nil)
	st.Type("y(z")
	if got := marked(st); got != "xy(z|)" {
		t.Fatalf("typed: %q", got)
	}
	st.Undo()
	if got := marked(st); got != "xy|" {
		t.Errorf("undo: %q, want %q", got, "xy|")
	}
	st.Undo()
	if got := marked(st); got != "x|" {
		t.Errorf("second undo: %q", got)
	}
	st = seqState(t, "a.go", "a[b]c", nil)
	st.Type("(")
	st.Type("x")
	st.Undo()
	if got := marked(st); got != "a([b])c" {
		t.Errorf("undo after surround: %q", got)
	}
	st.Undo()
	if got := marked(st); got != "a[b]c" {
		t.Errorf("undo of surround: %q", got)
	}
}

// TestAutoClosedTracking: the auto-closed characters follow edits of
// other views, and undo ends them when the caret leaves the pair.
func TestAutoClosedTracking(t *testing.T) {
	st := seqState(t, "a.go", "f|", nil)
	other := NewView(st.Model, nil)
	defer other.Close()
	st.Type("(")
	other.SetSelections([]Selection{{Anchor: 0, Head: 0}})
	other.Insert("xx") // before the pair: the tracked close moves with it
	if got := marked(st); got != "xxf(|)" {
		t.Fatalf("after the other view's edit: %q", got)
	}
	st.Type(")")
	if got := marked(st); got != "xxf()|" {
		t.Errorf("overtype after another view's edit: %q", got)
	}
	// Selecting all leaves the pair: no overtype afterwards.
	st = seqState(t, "a.go", "f|", nil)
	st.Type("(")
	st.Execute("editor.action.selectAll", nil)
	st.SetSelections([]Selection{{Anchor: 2, Head: 2}})
	st.Type(")")
	if got := marked(st); got != "f()|)" {
		t.Errorf("after leaving: %q", got)
	}
}

// TestTypeCommandAndView: the type command and committed text from the
// View go through the typing assistance; IME commits do not.
func TestTypeCommandAndView(t *testing.T) {
	st := seqState(t, "a.go", "f|", nil)
	st.Execute("type", map[string]any{"text": "("})
	if got := marked(st); got != "f(|)" {
		t.Errorf("type command: %q", got)
	}
	st = seqState(t, "a.go", "f|", nil)
	st.insertStep("(") // an IME commit
	if got := marked(st); got != "f(|" {
		t.Errorf("IME commit: %q", got)
	}
}

// TestTypingAssistUndoExact: auto-closing at several cursors, a comment
// toggle and Enter, then undo step by step and redo step by step, each
// restoring the text and every cursor exactly.
func TestTypingAssistUndoExact(t *testing.T) {
	st := seqState(t, "a.go", "func a() {\n\tx|\n\ty|\n}", func(st *State) { st.Model.SetInsertSpaces(false) })
	type snap struct {
		text string
		sels []Selection
	}
	now := func() snap { return snap{st.Snapshot().String(), slices.Clone(st.Sels)} }
	steps := []snap{now()}
	st.Type("(")
	st.Type("b") // continues the auto-closing step
	steps = append(steps, now())
	if got := marked(st); got != "func a() {\n\tx(b|)\n\ty(b|)\n}" {
		t.Fatalf("auto-closed: %q", got)
	}
	st.Execute("editor.action.commentLine", nil)
	steps = append(steps, now())
	if got := marked(st); got != "func a() {\n\t// x(b|)\n\t// y(b|)\n}" {
		t.Fatalf("commented: %q", got)
	}
	st.Type("\n")
	steps = append(steps, now())
	if got := marked(st); got != "func a() {\n\t// x(b\n\t|)\n\t// y(b\n\t|)\n}" {
		t.Fatalf("enter: %q", got)
	}
	check := func(ctx string, want snap) {
		t.Helper()
		got := now()
		if got.text != want.text || !slices.Equal(got.sels, want.sels) {
			t.Fatalf("%s: got %q %v, want %q %v", ctx, got.text, got.sels, want.text, want.sels)
		}
	}
	for i := len(steps) - 2; i >= 0; i-- {
		if !st.Undo() {
			t.Fatalf("undo to step %d did nothing", i)
		}
		check("undo", steps[i])
	}
	if st.Undo() {
		t.Fatal("an extra undo step")
	}
	for i := 1; i < len(steps); i++ {
		if !st.Redo() {
			t.Fatalf("redo to step %d did nothing", i)
		}
		check("redo", steps[i])
	}
}
