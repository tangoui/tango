package monarch

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/regex"
)

func addLexer(t *testing.T, id, g string) *Lexer {
	t.Helper()
	r := NewRegistry()
	if err := r.Add(id, json.RawMessage(g)); err != nil {
		t.Fatal(err)
	}
	lx, err := r.Lexer(id)
	if err != nil {
		t.Fatal(err)
	}
	return lx
}

// 病态回溯（regexp2 超时）：一行内第一次超时即降级——其余部分一个 "" token，
// 返回输入状态（不把可能错误的行尾状态当作"未变化"缓存），LineInfo.Degraded=true。
func TestPathologicalLineDegrades(t *testing.T) {
	lx := addLexer(t, "patho", `{"tokenPostfix":"","tokenizer":{"root":[
		["x","x"],
		["\\{",{"token":"open","next":"@inner"}],
		["(a+)+(?=b)","a"]
	],"inner":[["x","x"],["(a+)+(?=b)","a"],["\\}",{"token":"close","next":"@pop"}]]}}`)
	_, st1 := lx.TokenizeLine("{", lx.Initial(), nil)
	if st1 == lx.Initial() {
		t.Fatal("{ must push a state")
	}
	_, _, deg0 := Stats()
	line := "xx" + strings.Repeat("a", 200) + "c"
	start := time.Now()
	toks, st, info := lx.TokenizeLineInfo(line, st1, nil)
	d := time.Since(start)
	// Full 100ms engine allowance plus one confirmation, with clock slop.
	assertBudget(t, "pathological line", d, 2*regex.MatchTimeout+100*time.Millisecond)
	if !info.Degraded {
		t.Fatal("Degraded = false, want true")
	}
	if st != st1 {
		t.Fatalf("state = %d, want input state %d", st, st1)
	}
	want := []Token{{Start: 0, ByteStart: 0, Type: "x"}, {Start: 2, ByteStart: 2, Type: ""}}
	if !equalTokenSlices(toks, want) {
		t.Fatalf("tokens = %+v, want %+v", toks, want)
	}
	if _, _, deg := Stats(); deg-deg0 != 1 {
		t.Fatalf("degraded lines delta = %d, want 1", deg-deg0)
	}
	// TokenizeLine 委托给 TokenizeLineInfo，结果相同。
	toks2, st2 := lx.TokenizeLine(line, st1, nil)
	if st2 != st1 || !equalTokenSlices(toks2, want) {
		t.Fatalf("TokenizeLine: %+v %d", toks2, st2)
	}
	// 正常行不降级。
	if _, _, info := lx.TokenizeLineInfo("xx}", st1, nil); info.Degraded {
		t.Fatal("normal line degraded")
	}
}

// cases 里 ~regex 守卫超时同样降级。
func TestGuardTimeoutDegrades(t *testing.T) {
	lx := addLexer(t, "guard", `{"tokenPostfix":"","tokenizer":{"root":[
		["\\w+",{"cases":{"~(a+)+(?=b)":"bad","@default":"ok"}}],
		[" ","sp"]
	]}}`)
	line := "ok " + strings.Repeat("a", 200)
	toks, st, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
	want := []Token{{Start: 0, Type: "ok"}, {Start: 2, ByteStart: 2, Type: "sp"}, {Start: 3, ByteStart: 3, Type: ""}}
	if !info.Degraded || st != lx.Initial() || !equalTokenSlices(toks, want) {
		t.Fatalf("got %+v state %d info %+v", toks, st, info)
	}
}

// 不超时、但累计匹配耗时超过行预算（50ms）时也降级：这里每个位置的锚定匹配都要扫到行尾（平方级）。
func TestSlowLineDegrades(t *testing.T) {
	lx := addLexer(t, "slow", `{"tokenPostfix":"","tokenizer":{"root":[["[^z]*z","z"],[".","c"]]}}`)
	line := strings.Repeat("a", MaxLineLength-1)
	start := time.Now()
	toks, st, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
	assertBudget(t, "slow line", time.Since(start), 200*time.Millisecond)
	if !info.Degraded || st != lx.Initial() {
		t.Fatalf("info %+v state %d", info, st)
	}
	last := toks[len(toks)-1]
	if last.Type != "" || last.ByteStart == 0 && len(toks) != 1 {
		t.Fatalf("last token %+v", last)
	}
}

// 内置语法的全部 Monaco 用例行都不应降级。
func TestFixturesNotDegraded(t *testing.T) {
	files := fixtureFiles(t)
	reg := NewRegistry()
	for _, file := range files {
		id := fixtureID(file)
		lx, err := reg.Lexer(id)
		if err != nil {
			t.Fatal(err)
		}
		for ci, c := range loadFixture(t, file) {
			st := lx.Initial()
			var toks []Token
			for li, l := range c {
				var info LineInfo
				toks, st, info = lx.TokenizeLineInfo(l.Line, st, toks[:0])
				if info.Degraded {
					t.Errorf("%s case %d line %d %q degraded", id, ci, li, l.Line)
				}
			}
		}
	}
}

// 第一次进入嵌入语言时编译它的语法；编译耗时不计入本行的匹配预算。
func TestEmbeddedCompileNotCounted(t *testing.T) {
	old := compileGrammarFunc
	t.Cleanup(func() { compileGrammarFunc = old })
	compileGrammarFunc = func(id string, n *node) (*grammar, error) {
		if id == "inner" {
			time.Sleep(2 * lineBudgetDur)
		}
		return old(id, n)
	}
	r := NewRegistry()
	if err := r.Add("inner", json.RawMessage(`{"tokenPostfix":"","tokenizer":{"root":[["y","y"]]}}`)); err != nil {
		t.Fatal(err)
	}
	if err := r.Add("outer", json.RawMessage(`{"tokenPostfix":"","tokenizer":{"root":[
		["<",{"token":"open","next":"@emb","nextEmbedded":"inner"}]],
		"emb":[[">",{"token":"close","next":"@pop","nextEmbedded":"@pop"}]]}}`)); err != nil {
		t.Fatal(err)
	}
	lx, err := r.Lexer("outer")
	if err != nil {
		t.Fatal(err)
	}
	toks, _, info := lx.TokenizeLineInfo("<"+strings.Repeat("y", 40)+">", lx.Initial(), nil)
	if info.Degraded || types(toks) != "open y close" {
		t.Fatalf("got %q info %+v", types(toks), info)
	}
}
