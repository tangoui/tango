package monarch

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRuntimeTokenTypesPreserveSubstitutionAndBrackets(t *testing.T) {
	lx := addLexer(t, "runtime-types", `{
		"tokenPostfix":".p_>","defaultToken":"default_<",
		"brackets":[["(",")","paren_>"]],
		"tokenizer":{"root":[
			["(a)(b)",["group_<",{"token":"dynamic_$1"}]],
			["c",{"cases":{"@eos":"tail_<","@default":""}}],
			["d","literal_$1"],["e",{"token":"dynamic_$0"}],
			["f",""],["[()]","@brackets.extra_<"]
		]}}`)
	for _, tc := range []struct {
		line string
		want []Token
	}{
		{"ab", []Token{{Type: "group--.p--"}, {Start: 1, ByteStart: 1, Type: "dynamic-a.p--"}}},
		{"c", []Token{{Type: "tail--.p--"}}},
		{"cc", []Token{{}, {Start: 1, ByteStart: 1, Type: "tail--.p--"}}},
		{"d", []Token{{Type: "literal-$1.p--"}}},
		{"e", []Token{{Type: "dynamic-e.p--"}}},
		{"f", []Token{{}}},
		{"(", []Token{{Type: "paren--.p--.extra--"}}},
		{"x", []Token{{Type: "default--.p--"}}},
	} {
		got, state, info := lx.TokenizeLineInfo(tc.line, lx.Initial(), nil)
		if !reflect.DeepEqual(got, tc.want) || state != lx.Initial() || info.Degraded {
			t.Fatalf("line %q: got %v/%v/%v want %v/root/complete", tc.line, got, state, info, tc.want)
		}
	}
}

func TestRuntimeTypesRemainOwnedByTheirGrammar(t *testing.T) {
	r := NewRegistry()
	makeLexer := func(postfix string) *Lexer {
		t.Helper()
		if err := r.Add("replace-types", json.RawMessage(`{"tokenPostfix":"`+postfix+`","tokenizer":{"root":[["a","word"]]}}`)); err != nil {
			t.Fatal(err)
		}
		lx, err := r.Lexer("replace-types")
		if err != nil {
			t.Fatal(err)
		}
		return lx
	}
	old := makeLexer(".one")
	first, _ := old.TokenizeLine("a", old.Initial(), nil)
	newLexer := makeLexer(".two")
	second, _ := newLexer.TokenizeLine("a", newLexer.Initial(), nil)
	again, _ := old.TokenizeLine("a", old.Initial(), nil)
	if first[0].Type != "word.one" || again[0].Type != "word.one" || second[0].Type != "word.two" {
		t.Fatalf("grammar replacement changed token types: %v, %v, %v", first, again, second)
	}
}

func TestShortRuneViewsPreserveUnicodeAndConcurrentResults(t *testing.T) {
	lx := addLexer(t, "rune-view", `{"tokenizer":{"root":[["(?=a)a","hit"],[".","other"]]}}`)
	var wg sync.WaitGroup
	for worker := range 12 {
		wg.Go(func() {
			for _, line := range []string{"a!a", "éa", "😀a", "\xffa", strings.Repeat("a", 255), strings.Repeat("a", 257)} {
				got, state, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
				if info.Degraded || state != lx.Initial() {
					t.Errorf("worker %d line %q: state %v info %v", worker, line, state, info)
					return
				}
				if strings.HasSuffix(line, "a") && got[len(got)-1].Type != "hit.rune-view" {
					t.Errorf("worker %d lost final hit: %v", worker, got)
					return
				}
				switch line {
				case "éa":
					if !reflect.DeepEqual(got, []Token{{Type: "other.rune-view"}, {Start: 1, ByteStart: 2, Type: "hit.rune-view"}}) {
						t.Errorf("Unicode byte/rune offsets changed: %v", got)
					}
				case "😀a":
					if !reflect.DeepEqual(got, []Token{{Type: "other.rune-view"}, {Start: 2, ByteStart: 4, Type: "hit.rune-view"}}) {
						t.Errorf("UTF-16 surrogate offsets changed: %v", got)
					}
				case "\xffa":
					if !reflect.DeepEqual(got, []Token{{Type: "other.rune-view"}, {Start: 1, ByteStart: 1, Type: "hit.rune-view"}}) {
						t.Errorf("invalid UTF-8 offsets changed: %v", got)
					}
				}
			}
		})
	}
	wg.Wait()
}

// Run on baseline and optimized fixed binaries with the output path set, then
// compare the JSON streams. Normal tests use the fixtures' literal expectations.
func TestRuntimeDifferentialDump(t *testing.T) {
	output := os.Getenv("TANGO_MONARCH_TEST_DUMP")
	if output == "" {
		t.Skip("differential output not requested")
	}
	f, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	reg := NewRegistry()
	dump := func(id string, lx *Lexer, cases [][]string) {
		t.Helper()
		for ci, lines := range cases {
			state := lx.Initial()
			for li, line := range lines {
				tokens, next, info := lx.TokenizeLineInfo(line, state, []Token{{Start: 9, ByteStart: 9, Type: "prefix"}})
				if err := enc.Encode(struct {
					Language   string
					Case, Line int
					Input      string
					Tokens     []Token
					State      State
					Info       LineInfo
				}{id, ci, li, line, tokens, next, info}); err != nil {
					t.Fatal(err)
				}
				state = next
			}
		}
	}
	for _, file := range fixtureFiles(t) {
		id := fixtureID(file)
		lx, err := reg.Lexer(id)
		if err != nil {
			t.Fatal(err)
		}
		var cases [][]string
		for _, lines := range loadFixture(t, file) {
			var c []string
			for _, line := range lines {
				c = append(c, line.Line)
			}
			cases = append(cases, c)
		}
		dump(id, lx, cases)
	}
	parts := []string{"a", "const ", "// comment", "/* open", "*/", "\"x\\n\"", "`a${x}`", "(", ")", "+", "0x1f", "😀", "é", "\xff", " ", "<div>", "</div>"}
	var seed uint32 = 719
	var generated [][]string
	for n := 0; n < 2000; n++ {
		line := ""
		for range n % 12 {
			seed = seed*1664525 + 1013904223
			line += parts[int(seed>>8)%len(parts)]
		}
		generated = append(generated, []string{line, "tail := 7", "*/ ` \""})
	}
	for _, id := range []string{"go", "typescript", "html"} {
		lx, err := reg.Lexer(id)
		if err != nil {
			t.Fatal(err)
		}
		dump("generated-"+id, lx, generated)
	}
}
