package monarch

import (
	"reflect"
	"testing"
)

func TestGoFastPathMatchesGrammar(t *testing.T) {
	fast := mustLexer(t, "go")
	ref := mustLexer(t, "go")
	ref.fastGo = false
	cases := []string{
		`x := 0x1f`, `x := 077`, `x := 1abc`, `x := 1.2.3`, `x := "x\n"`, `x := "\q"`,
		`/** doc */ var x error`, `x := 1/*comment*/+2`, `x := 1+// comment`, `x := <`, `x := <= 2`,
		`x := "中文😀"`, `变量 := 123`, `x := '\n'`, `x := 'abc'`, `x := [][2]int{}`, `x := 1e+2`,
	}
	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			got, gs := fast.TokenizeLine(line, fast.Initial(), nil)
			want, ws := ref.TokenizeLine(line, ref.Initial(), nil)
			if !reflect.DeepEqual(got, want) || (gs == fast.Initial()) != (ws == ref.Initial()) {
				t.Fatalf("optimized=%v root=%v; grammar=%v root=%v", got, gs == fast.Initial(), want, ws == ref.Initial())
			}
		})
	}
}

func TestBacktrackFilterPreservesAlternativesAndOptionalPrefix(t *testing.T) {
	for _, pat := range []string{`a(?=x)|b`, `a?(?=b)b`, `a*(?=b)b`} {
		t.Run(pat, func(t *testing.T) {
			lx := addLexer(t, "filter-check", `{"tokenizer":{"root":[[{"$regex":"`+pat+`","flags":""},"hit"],[".","fallback"]]}}`)
			got, _ := lx.TokenizeLine("b", lx.Initial(), nil)
			if len(got) == 0 || got[0].Type != "hit.filter-check" {
				t.Fatalf("filter removed valid match: %v", got)
			}
		})
	}
}

func TestCustomGoGrammarCannotSelectBuiltinFastPath(t *testing.T) {
	r := NewRegistry()
	if err := r.Add("go", []byte(`{"defaultToken":"","tokenPostfix":".go","tokenizer":{"root":[[".+","custom"]]}}`)); err != nil {
		t.Fatal(err)
	}
	lx, err := r.Lexer("go")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := lx.TokenizeLine("var x = 1", lx.Initial(), nil)
	if len(got) != 1 || got[0].Type != "custom.go" {
		t.Fatalf("custom grammar bypassed: %v", got)
	}
}

func TestGoFastPathGeneratedEquivalence(t *testing.T) {
	fast := mustLexer(t, "go")
	ref := mustLexer(t, "go")
	ref.fastGo = false
	parts := []string{"var", "error", "true", "x_1", "0x1f", "077", "0b101", "1abc", "1e+2", "1e+", "1.2.3", "1'", ".23", "\"\\x0f\\n\"", "\"\\q\"", "`raw`", "`a\\b`", "'x'", "/** doc */", "/* open", "// tail", "a:[]int{1}", "[[]]", "+//", ":=", "<=", "+!", "&^=", "()", ",.", "\\", "#", "中文😀"}
	var seed uint32 = 451
	for n := 0; n < 3000; n++ {
		line := ""
		for i := 0; i < 5; i++ {
			seed = seed*1664525 + 1013904223
			line += parts[int(seed)%len(parts)]
			if seed&256 != 0 {
				line += " "
			}
		}
		fs, rs := fast.Initial(), ref.Initial()
		prefix := []Token{{Start: 7, ByteStart: 7, Type: "prefix"}}
		for _, s := range []string{line, "tail := 7", "*/ ` \""} {
			got, gst := fast.TokenizeLine(s, fs, append([]Token(nil), prefix...))
			want, wst := ref.TokenizeLine(s, rs, append([]Token(nil), prefix...))
			if !reflect.DeepEqual(got, want) || (gst == fast.Initial()) != (wst == ref.Initial()) {
				t.Fatalf("line=%q current=%q\nfast %v\nref %v", line, s, got, want)
			}
			fs, rs = gst, wst
		}
	}
}

func TestGoFastPathKeywordSetMatchesBuiltinGrammar(t *testing.T) {
	lx := mustLexer(t, "go")
	words, ok := stringArray(lx.g.src.get("keywords"))
	if !ok {
		t.Fatal("missing keywords")
	}
	if len(words) != len(goKeywordTypes) {
		t.Fatalf("keyword set sizes differ: grammar=%d fast=%d", len(words), len(goKeywordTypes))
	}
	for _, word := range words {
		typ, ok := goKeywordTypes[word]
		if !ok || typ != "keyword."+word+".go" {
			t.Fatalf("keyword %q fast type %q", word, typ)
		}
	}
}
