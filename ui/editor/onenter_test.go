package editor

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/langs"
)

func tabs(st *State) { st.Model.SetInsertSpaces(false) }

// TestOnEnterJSDoc: TypeScript's onEnter rules continue JSDoc comments.
func TestOnEnterJSDoc(t *testing.T) {
	runSeq(t, []seqCase{
		{"after /**", "a.ts", "/**|", []string{"\n"}, "/**\n * |", nil},
		{"between /** and */", "a.ts", "/**| */", []string{"\n"}, "/**\n * |\n */", nil},
		{"typed /** then Enter", "a.ts", "|", []string{"/**", "\n"}, "/**\n * |\n */", nil},
		{"continues * lines", "a.ts", "/**\n * foo|", []string{"\n"}, "/**\n * foo\n * |", nil},
		{"after */ removes the space", "a.ts", "/**\n * a\n */|", []string{"\n"}, "/**\n * a\n */\n|", nil},
		{"indented", "a.ts", "\t/**| */", []string{"\n"}, "\t/**\n\t * |\n\t */", tabs},
		{"indented with spaces", "a.ts", "    /**|", []string{"\n"}, "    /**\n     * |", nil},
		{"not after /**/", "a.ts", "/**/|", []string{"\n"}, "/**/\n|", nil},
		{"javascript too", "a.js", "/**|", []string{"\n"}, "/**\n * |", nil},
	})
}

// TestOnEnterIndentRules: Enter by the onEnter rules, the brackets and the
// indentation rules; a closing bracket typed at the start of a line moves
// it to its opening bracket's indentation.
func TestOnEnterIndentRules(t *testing.T) {
	none := func(st *State) { st.Options.AutoIndent = AutoIndentNone }
	keep := func(st *State) { st.Options.AutoIndent = AutoIndentKeep }
	brackets := func(st *State) { st.Options.AutoIndent = AutoIndentBrackets; tabs(st) }
	runSeq(t, []seqCase{
		// Python: the onEnter rule indents after a line ending in ":".
		{"python def", "a.py", "def f():|", []string{"\n"}, "def f():\n    |", nil},
		{"python if", "a.py", "    if x:|", []string{"\n"}, "    if x:\n        |", nil},
		{"python keeps", "a.py", "    return x|", []string{"\n"}, "    return x\n    |", nil},
		{"python no rule below advanced", "a.py", "def f():|", []string{"\n"}, "def f():\n|", brackets},
		// Go: brackets indent and split a pair over three lines.
		{"go open brace", "a.go", "func a() {|", []string{"\n"}, "func a() {\n\t|", tabs},
		{"go between braces", "a.go", "func a() {|}", []string{"\n"}, "func a() {\n\t|\n}", tabs},
		{"go between parens", "a.go", "\tf(|)", []string{"\n"}, "\tf(\n\t\t|\n\t)", tabs},
		{"go brackets level", "a.go", "func a() {|}", []string{"\n"}, "func a() {\n\t|\n}", brackets},
		{"go spaces", "a.go", "func a() {|", []string{"\n"}, "func a() {\n    |", nil},
		{"go selection replaced", "a.go", "f([x])", []string{"\n"}, "f(\n    |\n)", nil},
		{"go carets", "a.go", "{|}\n{|}", []string{"\n"}, "{\n\t|\n}\n{\n\t|\n}", tabs},
		{"no auto indent", "a.go", "\tfoo{|", []string{"\n"}, "\tfoo{\n|", none},
		{"keep", "a.go", "\tfoo{|", []string{"\n"}, "\tfoo{\n\t|", func(st *State) { keep(st); tabs(st) }},
		{"keep normalises", "a.go", "\tfoo|", []string{"\n"}, "\tfoo\n    |", keep},
		{"brace in a string", "a.go", "x := \"{|", []string{"\n"}, "x := \"{\n|", nil},
		// Go: a closing brace typed at the start of a line outdents it to
		// the line of its opening brace (electric character).
		{"go } outdents", "a.go", "func a() {\n\tx\n\t|", []string{"}"}, "func a() {\n\tx\n}|", tabs},
		{"go } nested", "a.go", "func a() {\n\tif b {\n\t\tx\n\t\t|", []string{"}"}, "func a() {\n\tif b {\n\t\tx\n\t}|", tabs},
		{"go } skips braces in strings", "a.go", "func a() {\n\ts := \"}\"\n\t\t|", []string{"}"}, "func a() {\n\ts := \"}\"\n}|", tabs},
		{"go } after text stays", "a.go", "func a() {\n\tx|", []string{"}"}, "func a() {\n\tx}|", tabs},
		{"go } in a raw string", "a.go", "x := `\n\t|", []string{"}"}, "x := `\n\t}|", tabs},
		{"go ) outdents", "a.go", "f(\n\t\ta,\n\t\t|", []string{")"}, "f(\n\t\ta,\n)|", tabs},
		{"go } same line", "a.go", "{ |", []string{"}"}, "{ }|", tabs},
		// Ruby: the indentation rules (autoIndent "full").
		{"ruby def", "a.rb", "def foo|", []string{"\n"}, "def foo\n    |", nil},
		{"ruby end outdents when typed", "a.rb", "def foo\n    x\n    en|", []string{"d"}, "def foo\n    x\nend|", nil},
		{"ruby end at the right indentation", "a.rb", "def foo\n    x\nen|", []string{"d"}, "def foo\n    x\nend|", nil},
		{"ruby enter before end", "a.rb", "def foo\n    x|end", []string{"\n"}, "def foo\n    x\n|end", nil},
		{"ruby enter in the indentation", "a.rb", "def foo\n|  x", []string{"\n"}, "def foo\n\n|    x", nil},
		// No valid line above (getPrecedingValidLine returns -1): the
		// indentation rules give nothing and Enter keeps the indentation.
		{"ruby enter under blank lines", "a.rb", "        \n  |", []string{"\n"}, "        \n  \n  |", nil},
		// Verilog's word brackets match as whole words: "end" in "append"
		// is not a bracket.
		{"verilog end outdents past append", "a.v", "begin\n  append x;\n  |", []string{"end"}, "begin\n  append x;\nend|", nil},
		{"verilog endcase is not end", "a.v", "begin\n  case x\n  endcase\n  |", []string{"end"}, "begin\n  case x\n  endcase\nend|", nil},
	})
}

// TestAutoIndentCommands: Tab on a blank line, insertLineBefore/After and
// moving lines use the rules too.
func TestAutoIndentCommands(t *testing.T) {
	tab, before, after := "#tab", "#editor.action.insertLineBefore", "#editor.action.insertLineAfter"
	up, down := "#editor.action.moveLinesUpAction", "#editor.action.moveLinesDownAction"
	runSeq(t, []seqCase{
		// Tab on a blank line: the indentation the rules give it.
		{"tab after a brace", "a.go", "func a() {\n|", []string{tab}, "func a() {\n    |", nil},
		{"tab after a brace, tabs", "a.go", "func a() {\n|", []string{tab}, "func a() {\n\t|", tabs},
		{"tab blank line keeps going", "a.go", "func a() {\n    |", []string{tab}, "func a() {\n        |", nil},
		{"tab without a rule", "a.go", "\t\tx\n|", []string{tab}, "\t\tx\n    |", nil},
		{"tab replaces blanks", "a.go", "func a() {\n  |", []string{tab}, "func a() {\n    |", nil},
		{"tab ruby", "a.rb", "def foo\n|", []string{tab}, "def foo\n    |", nil},
		{"tab ruby after end", "a.rb", "def foo\n    x\nend\n|", []string{tab}, "def foo\n    x\nend\n    |", nil},
		{"tab plain text", "a.txt", "  x\n|", []string{tab}, "  x\n    |", nil},
		{"tab ruby under blank lines", "a.rb", "        \n|", []string{tab}, "        \n    |", nil},
		// insertLineBefore / After: Enter at the end of the line above.
		{"insert after a brace", "a.go", "func a() {|", []string{after}, "func a() {\n\t|", tabs},
		{"insert before under a brace", "a.go", "func a() {\n|x\n}", []string{before}, "func a() {\n\t|\nx\n}", tabs},
		{"insert after python def", "a.py", "def f():|", []string{after}, "def f():\n    |", nil},
		{"insert before first line", "a.go", "x|", []string{before}, "|\nx", nil},
		{"insert after jsdoc", "a.ts", "/**|", []string{after}, "/**\n * |", nil},
		// Moving lines into and out of a Ruby block re-indents them.
		{"ruby move up into a block", "a.rb", "def a\n    y\nend\nx|", []string{up}, "def a\n    y\n    x|\nend", nil},
		{"ruby move down out of a block", "a.rb", "def a\n    |x\nend\nz", []string{down}, "def a\nend\n|x\nz", nil},
		{"ruby move down into a block", "a.rb", "|x\ndef a\n    y\nend", []string{down}, "def a\n    |x\n    y\nend", nil},
		{"ruby move a block", "a.rb", "def a\nend\n[x\ny]", []string{up}, "def a\n    [x\n    y]\nend", nil},
		{"ruby move keeps aligned", "a.rb", "def a\n    y\n    x|\nend", []string{up}, "def a\n    x|\n    y\nend", nil},
		{"go move keeps indentation", "a.go", "func a() {\n}\nx|", []string{up}, "func a() {\nx|\n}", nil},
	})
}

// TestAutoIndentShiftJSDoc: indenting JSDoc continuation lines keeps the
// space before their "*" (Monaco's ShiftCommand extraSpaces).
func TestAutoIndentShiftJSDoc(t *testing.T) {
	runSeq(t, []seqCase{
		{"indent", "a.ts", "[/**\n * a\n */]", []string{"#editor.action.indentLines"}, "[    /**\n     * a\n     */]", nil},
		{"outdent", "a.ts", "[    /**\n     * a\n     */]", []string{"#editor.action.outdentLines"}, "[/**\n * a\n */]", nil},
	})
}

// TestOnEnterRulesCompile: every built-in language's onEnter rules compile
// (a rule that does not is dropped), and so do its bracket patterns.
func TestOnEnterRulesCompile(t *testing.T) {
	for _, l := range langs.Builtin() {
		o := newOnEnterSupport(&l.Conf)
		if o == nil {
			if len(l.Conf.OnEnterRules) > 0 || l.Conf.Brackets != nil {
				t.Errorf("%s: no onEnter support", l.ID)
			}
			continue
		}
		if len(o.rules) != len(l.Conf.OnEnterRules) {
			t.Errorf("%s: %d of %d onEnter rules compile", l.ID, len(o.rules), len(l.Conf.OnEnterRules))
		}
		want := len(l.Conf.Brackets)
		if l.Conf.Brackets == nil {
			want = 3
		}
		if len(o.brackets) != want {
			t.Errorf("%s: %d of %d bracket patterns compile", l.ID, len(o.brackets), want)
		}
	}
}

// TestBracketRegexp: the brackets removed from strings and comments for the
// indentation rules match case-insensitively, word brackets as whole words
// only (Monaco's createBracketOrRegExp).
func TestBracketRegexp(t *testing.T) {
	st := seqState(t, "a.v", "// subclass Class x (y)|", nil)
	snap := st.Snapshot()
	if got := st.processedLine(snap, 0, st.typing()); got != "// subclass  x y" {
		t.Errorf("processed: %q", got)
	}
	re := bracketRegexp([][2]string{{"begin", "end"}, {"(", ")"}})
	if got := re.ReplaceAllString("append (End) BEGIN ending", ""); got != "append   ending" {
		t.Errorf("replaced: %q", got)
	}
	if bracketRegexp(nil) != nil {
		t.Error("no brackets, a regexp")
	}
}

// TestEnterManyCursorsStale: Enter at 1000 cursors across a 10 000-line
// file the Highlighter has not tokenized stays bounded: the synchronous
// catch-up is spent once per command, not per cursor.
func TestEnterManyCursorsStale(t *testing.T) {
	var b strings.Builder
	for i := range 10000 {
		if i%10 == 0 {
			b.WriteString("\tf(a) {\n")
		} else {
			b.WriteString("\tx := f(a)\n")
		}
	}
	st := NewState(b.String(), "a.go", nil)
	snap := st.Snapshot()
	var sels []Selection
	for i := 0; i < 10000; i += 10 {
		sels = append(sels, carets(lineEnd(snap, i))...)
	}
	st.SetSelections(sels)
	start := time.Now()
	st.Type("\n")
	d := time.Since(start)
	t.Logf("Enter at %d cursors in %v", len(sels), d)
	if lim := gateLimit(50 * time.Millisecond); d > lim {
		t.Errorf("Enter took %v, want ≤ %v", d, lim)
	}
	if got := st.Snapshot().LineCount(); got != 10001+1000 {
		t.Fatalf("%d lines", got)
	}
}

// TestBracketsNonASCII: lines whose lower case is shorter than themselves
// (İ, the Kelvin sign, the Ohm sign) or that hold emoji, between the
// brackets, keep the byte offsets of the bracket searches right.
func TestBracketsNonASCII(t *testing.T) {
	runSeq(t, []seqCase{
		{"electric } past İ", "a.go", "func a() {\n\t// İstanbul\n\t\t|", []string{"}"}, "func a() {\n\t// İstanbul\n}|", tabs},
		{"electric } past K Ω emoji", "a.go", "func a() {\n\tx := \"KΩ😀İİİ\" // ΩΩ\n\t\t|", []string{"}"}, "func a() {\n\tx := \"KΩ😀İİİ\" // ΩΩ\n}|", tabs},
		{"electric } after İ on the line", "a.go", "func a() {\n\tİ := 1; f(\"İİ\")\n\t\t|", []string{"}"}, "func a() {\n\tİ := 1; f(\"İİ\")\n}|", tabs},
		{"verilog end past İ", "a.v", "begin\n  // İİİİ append\n  x = 1; // KΩ\n  |", []string{"end"}, "begin\n  // İİİİ append\n  x = 1; // KΩ\nend|", nil},
		{"verilog END upper case", "a.v", "BEGIN\n  İ\n  |", []string{"end"}, "BEGIN\n  İ\nend|", nil},
	})
	// The bracket regexp folds ASCII only: the Kelvin sign is not a k.
	re := bracketRegexp([][2]string{{"task", "endtask"}})
	if got := re.ReplaceAllString("TASK tasK", ""); got != " tasK" {
		t.Errorf("replaced: %q", got)
	}
}

// TestMatchBracketUpFuzz: matchBracketUp and electric typing never panic
// on random lines of non-ASCII text, brackets and word brackets.
func TestMatchBracketUpFuzz(t *testing.T) {
	pieces := []string{"İ", "K", "Ω", "😀", "ß", "é", "a", "Z", " ", "\t", "{", "}", "(", ")", "begin", "end", "BEGIN", "End", "append", "\"", "//", "/*", "*/"}
	rng := rand.New(rand.NewPCG(1, 2))
	line := func() string {
		var b strings.Builder
		for range rng.IntN(12) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		return b.String()
	}
	for _, file := range []string{"a.go", "a.v"} {
		for range 200 {
			var lines []string
			for range 1 + rng.IntN(6) {
				lines = append(lines, line())
			}
			text := strings.Join(lines, "\n")
			st := NewState(text, file, nil)
			snap := st.Snapshot()
			tc := st.typing()
			for _, p := range tc.brackets {
				l := rng.IntN(snap.LineCount())
				col := rng.IntN(len(snap.Line(l)) + 1)
				st.matchBracketUp(snap, p, l, col)
			}
			// Electric typing at the end of a new blank line.
			st.SetSelections(carets(snap.Len()))
			st.Type("\n")
			for _, ch := range []string{"}", ")", "end"} {
				st.Type(ch)
			}
		}
	}
}
