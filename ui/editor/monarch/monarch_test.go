package monarch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type fixtureToken struct {
	StartIndex int    `json:"startIndex"`
	Type       string `json:"type"`
}

type fixtureLine struct {
	Line   string         `json:"line"`
	Tokens []fixtureToken `json:"tokens"`
}

const fixtureDir = "../langs/testdata/tokenize"

func loadFixture(t testing.TB, file string) [][]fixtureLine {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var cases [][]fixtureLine
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return cases
}

func fixtureFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(fixtureDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	return files
}

func fixtureID(file string) string { return strings.TrimSuffix(filepath.Base(file), ".json") }

func TestMonacoFixtures(t *testing.T) {
	files := fixtureFiles(t)
	reg := NewRegistry()
	_, panics0, _ := Stats()
	defer func() {
		// TokenizeLine 的 recover 只为不可信插件语法兜底，不能掩盖内置语法上的 bug
		if _, p, _ := Stats(); p != panics0 {
			t.Errorf("built-in grammars hit %d recovered panics on fixtures", p-panics0)
		}
	}()
	type rate struct {
		id          string
		pass, total int
	}
	var rates []rate
	for _, file := range files {
		id := fixtureID(file)
		cases := loadFixture(t, file)
		lx, err := reg.Lexer(id)
		if err != nil {
			t.Errorf("%s: Lexer: %v", id, err)
			rates = append(rates, rate{id, 0, len(cases)})
			continue
		}
		pass := 0
		for ci, c := range cases {
			if msg := runFixtureCase(lx, c); msg != "" {
				t.Errorf("%s case %d: %s", id, ci, msg)
				continue
			}
			pass++
		}
		rates = append(rates, rate{id, pass, len(cases)})
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i].id < rates[j].id })
	var sumPass, sumTotal int
	var sb strings.Builder
	for _, r := range rates {
		sumPass += r.pass
		sumTotal += r.total
		pct := 100.0
		if r.total > 0 {
			pct = 100 * float64(r.pass) / float64(r.total)
		}
		fmt.Fprintf(&sb, "| %s | %d/%d | %.1f%% |\n", r.id, r.pass, r.total, pct)
	}
	fmt.Fprintf(&sb, "| **overall** | %d/%d | %.1f%% |\n", sumPass, sumTotal, 100*float64(sumPass)/float64(sumTotal))
	t.Logf("fixture pass rates (cases):\n%s", sb.String())
}

// runFixtureCase 逐行分词（行间传递状态），返回第一处差异的描述；全部一致返回 ""。
func runFixtureCase(lx *Lexer, c []fixtureLine) string {
	st := lx.Initial()
	var toks []Token
	for li, l := range c {
		toks, st = lx.TokenizeLine(l.Line, st, toks[:0])
		got := make([]fixtureToken, len(toks))
		for i, tk := range toks {
			got[i] = fixtureToken{tk.Start, tk.Type}
		}
		if !equalTokens(got, l.Tokens) {
			return fmt.Sprintf("line %d %q:\n  want %v\n  got  %v", li, l.Line, l.Tokens, got)
		}
	}
	return ""
}

func equalTokens(a, b []fixtureToken) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mustLexer(t testing.TB, id string) *Lexer {
	t.Helper()
	lx, err := NewRegistry().Lexer(id)
	if err != nil {
		t.Fatal(err)
	}
	return lx
}

func TestStateInterning(t *testing.T) {
	lx := mustLexer(t, "typescript")
	st0 := lx.Initial()
	if st0 == 0 {
		t.Fatal("Initial() must not be 0")
	}
	_, a := lx.TokenizeLine("let x = `abc ${", st0, nil)
	_, b := lx.TokenizeLine("let x = `abc ${", st0, nil)
	if a != b {
		t.Fatalf("same input, different states: %d vs %d", a, b)
	}
	if a == st0 {
		t.Fatalf("template literal must push a state")
	}
	_, c := lx.TokenizeLine("/* open comment", st0, nil)
	if c == a || c == st0 {
		t.Fatalf("different stacks must differ: st0=%d a=%d c=%d", st0, a, c)
	}
	_, d := lx.TokenizeLine("let y = 2;", st0, nil)
	if d != st0 {
		t.Fatalf("plain line must keep the root state: %d vs %d", d, st0)
	}
	// 状态 0 视同初始状态。
	_, e := lx.TokenizeLine("let y = 2;", 0, nil)
	if e != st0 {
		t.Fatalf("state 0 should act as Initial: %d vs %d", e, st0)
	}
}

// withoutLineBudget 在测试期间关闭行预算（LineBudget），用于检验与墙钟无关的语义（如长行切分）。
func withoutLineBudget(t *testing.T) {
	old := lineBudgetDur
	lineBudgetDur = time.Hour
	t.Cleanup(func() { lineBudgetDur = old })
}

func TestLongLine(t *testing.T) {
	withoutLineBudget(t) // 两万列的分词在负载高或 -race 下可能超过 50ms 预算而降级；这里只验证切分语义
	lx := mustLexer(t, "javascript")
	unit := "function f(a,b){return a+b*2/3;}var s='x\\'y',r=/ab+c/g;if(a<b&&c>d){x=[1,2,3]}"
	var sb strings.Builder
	for sb.Len() < 1<<20 {
		sb.WriteString(unit)
	}
	line := sb.String()
	st0 := lx.Initial()
	start := time.Now()
	toks, st := lx.TokenizeLine(line, st0, nil)
	assertBudget(t, "1 MB line", time.Since(start), 2*time.Second)
	if st != st0 {
		t.Fatalf("long line must keep the input state")
	}
	if len(toks) < 2 {
		t.Fatalf("expected tokens before the cut, got %v", toks)
	}
	last := toks[len(toks)-1]
	if last.Start != MaxLineLength || last.ByteStart != MaxLineLength || last.Type != "" {
		t.Fatalf("last token = %+v, want {Start:%d Type:\"\"}", last, MaxLineLength)
	}
	for _, tk := range toks[:len(toks)-1] {
		if tk.Start >= MaxLineLength {
			t.Fatalf("token past the cut: %+v", tk)
		}
	}
	if toks[0].Type != "keyword.js" {
		t.Fatalf("first token = %+v, want keyword.js", toks[0])
	}

	// 非 ASCII：切点按 UTF-16 列计算，且不切开代理对。
	line2 := strings.Repeat("中", MaxLineLength-1) + "😀" + strings.Repeat("a", 10)
	start = time.Now()
	toks2, _ := lx.TokenizeLine(line2, st0, nil)
	// 无规则匹配的字符逐个前进，每个位置都尝试全部规则
	assertBudget(t, "CJK line", time.Since(start), 2*time.Second)
	last2 := toks2[len(toks2)-1]
	if last2.Type != "" || last2.Start != MaxLineLength-1 || last2.ByteStart != 3*(MaxLineLength-1) {
		t.Fatalf("non-ASCII cut token = %+v", last2)
	}
}

// 切点前最后一个 token 已是 "" 时不再追加，保持合并。
func TestLongLineMergesEmptyTail(t *testing.T) {
	lx := mustLexer(t, "javascript")
	toks, _ := lx.TokenizeLine("x"+strings.Repeat(" ", MaxLineLength+50), lx.Initial(), nil)
	if len(toks) != 2 || toks[0].Type != "identifier.js" || toks[1] != (Token{Start: 1, ByteStart: 1}) {
		t.Fatalf("got %+v", toks)
	}
}

func TestEmbeddedAcrossLines(t *testing.T) {
	lx := mustLexer(t, "html")
	st := lx.Initial()
	var toks []Token
	toks, st = lx.TokenizeLine("<script>", st, toks[:0])
	if got := types(toks); got != "delimiter.html tag.html delimiter.html" {
		t.Fatalf("line 1: %s", got)
	}
	toks, st = lx.TokenizeLine("let x = 1", st, toks[:0])
	for _, tk := range toks {
		if tk.Type != "" && !strings.HasSuffix(tk.Type, ".js") {
			t.Fatalf("line 2 token %+v is not javascript", tk)
		}
	}
	if got := types(toks); got != "keyword.js  identifier.js  delimiter.js  number.js" {
		t.Fatalf("line 2: %q", got)
	}
	inScript := st
	toks, st = lx.TokenizeLine("</script>", st, toks[:0])
	if got := types(toks); got != "delimiter.html tag.html delimiter.html" {
		t.Fatalf("line 3: %s", got)
	}
	if st != lx.Initial() {
		t.Fatalf("state after </script> = %d, want initial %d", st, lx.Initial())
	}
	if inScript == lx.Initial() {
		t.Fatal("state inside <script> must differ from initial")
	}
	// 嵌入语言的状态跨行保存：多行注释在 script 中延续。
	st = lx.Initial()
	_, st = lx.TokenizeLine("<script>/* a", st, nil)
	toks, st = lx.TokenizeLine("b */ var</script>", st, nil)
	if got := types(toks); got != "comment.js  keyword.js delimiter.html tag.html delimiter.html" {
		t.Fatalf("comment across lines: %q", got)
	}
	if st != lx.Initial() {
		t.Fatal("state not restored after </script>")
	}
}

func TestUnknownLanguage(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Lexer("plaintext"); err == nil {
		t.Fatal("plaintext should have no lexer")
	}
	if _, err := r.Lexer("no-such-language"); err == nil {
		t.Fatal("unknown id should fail")
	}
}

func TestRegistryAdd(t *testing.T) {
	r := NewRegistry()
	g := json.RawMessage(`{
		"tokenPostfix": ".demo",
		"keywords": ["if", "else"],
		"tokenizer": {
			"root": [
				[{"$regex": "[a-z]+", "flags": ""}, {"cases": {"@keywords": "keyword", "@default": "identifier"}}],
				[{"$regex": "\"", "flags": ""}, "string", "@str"],
				[{"$regex": "\\s+", "flags": ""}, "white"]
			],
			"str": [
				[{"$regex": "[^\"]+", "flags": ""}, "string"],
				[{"$regex": "\"", "flags": ""}, "string", "@pop"]
			]
		}
	}`)
	if err := r.Add("demo", g); err != nil {
		t.Fatal(err)
	}
	lx, err := r.Lexer("demo")
	if err != nil {
		t.Fatal(err)
	}
	toks, st := lx.TokenizeLine(`if x "ab`, lx.Initial(), nil)
	if got := types(toks); got != "keyword.demo white.demo identifier.demo white.demo string.demo" {
		t.Fatalf("got %q", got)
	}
	toks, st = lx.TokenizeLine(`c" else`, st, nil)
	if got := types(toks); got != "string.demo white.demo keyword.demo" || st != lx.Initial() {
		t.Fatalf("got %q", got)
	}
	// 同 ID 覆盖。
	if err := r.Add("demo", json.RawMessage(`{"tokenizer":{"root":[[{"$regex":".+","flags":""},"all"]]}}`)); err != nil {
		t.Fatal(err)
	}
	lx2, _ := r.Lexer("demo")
	toks, _ = lx2.TokenizeLine("if x", lx2.Initial(), nil)
	if got := types(toks); got != "all.demo" {
		t.Fatalf("override: %q", got)
	}
	if err := r.Add("bad", json.RawMessage(`{"tokenizer":{"root":[[{"$regex":"a","flags":""},"x","@nowhere"]]}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Lexer("bad"); err == nil {
		t.Fatal("undefined next state must be a compile error")
	}
}

func TestUTF16Columns(t *testing.T) {
	lx := mustLexer(t, "go")
	toks, _ := lx.TokenizeLine(`s := "中😀" + x`, lx.Initial(), nil)
	// s␠:=␠"中😀"␠+␠x：字符串占 UTF-16 列 5..9（中=1，😀=2），之后的 ␠ 在列 10、字节 14。
	var found bool
	for _, tk := range toks {
		if tk.Type == "" && tk.ByteStart == 14 {
			found = true
			if tk.Start != 10 {
				t.Fatalf("token %+v: want Start 10", tk)
			}
		}
	}
	if !found {
		t.Fatalf("no token at byte 14: %+v", toks)
	}
}

func types(toks []Token) string {
	s := make([]string, len(toks))
	for i, tk := range toks {
		s[i] = tk.Type
	}
	return strings.Join(s, " ")
}

// 比对逻辑本身能发现差异（防止 TestMonacoFixtures 因比较写错而"全过"）。
func TestFixtureHarnessDetectsMismatch(t *testing.T) {
	lx := mustLexer(t, "go")
	good := []fixtureLine{{Line: "var x", Tokens: []fixtureToken{{0, "keyword.var.go"}, {3, ""}, {4, "identifier.go"}}}}
	if msg := runFixtureCase(lx, good); msg != "" {
		t.Fatalf("good case failed: %s", msg)
	}
	bad := []fixtureLine{{Line: "var x", Tokens: []fixtureToken{{0, "keyword.var.go"}, {3, ""}, {4, "keyword.go"}}}}
	if runFixtureCase(lx, bad) == "" {
		t.Fatal("mismatch not detected")
	}
}

// 规则执行出错（这里是空匹配且状态不变，Monaco 抛 "no progress"）时，
// 与 Monaco 编辑器的 safeTokenize 一样整行一个 "" token、状态不变。
func TestRuntimeErrorNullTokens(t *testing.T) {
	r := NewRegistry()
	err := r.Add("loop", json.RawMessage(`{"tokenizer":{"root":[
		[{"$regex":"a","flags":""},"a"],
		[{"$regex":"(?=b)","flags":""},"x"]
	]}}`))
	if err != nil {
		t.Fatal(err)
	}
	lx, err := r.Lexer("loop")
	if err != nil {
		t.Fatal(err)
	}
	toks, st := lx.TokenizeLine("aab", lx.Initial(), []Token{{Type: "keep"}})
	if len(toks) != 2 || toks[0].Type != "keep" || toks[1] != (Token{}) || st != lx.Initial() {
		t.Fatalf("got %+v state %d", toks, st)
	}
}

// $Sn 状态引用（正则与 token 里）、@rematch 与 switchTo。
func TestStateReferences(t *testing.T) {
	r := NewRegistry()
	err := r.Add("refs", json.RawMessage(`{
		"tokenPostfix": "",
		"tokenizer": {
			"root": [
				[{"$regex":"([\"'])","flags":""}, {"token":"open", "next":"@str.$1"}],
				[{"$regex":"\\w+","flags":""}, "word"],
				[{"$regex":"\\s+","flags":""}, ""]
			],
			"str": [
				[{"$regex":"$S2","flags":""}, {"token":"close.$S2", "next":"@pop"}],
				[{"$regex":"#","flags":""}, {"token":"@rematch", "switchTo":"@hash.$S2"}],
				[{"$regex":"[^\"'#]+|[\"']","flags":""}, "body"]
			],
			"hash": [
				[{"$regex":"#","flags":""}, {"token":"hash", "switchTo":"@str.$S2"}]
			]
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	lx, _ := r.Lexer("refs")
	toks, st := lx.TokenizeLine(`a "x'#y" b`, lx.Initial(), nil)
	want := "word  open body hash body close.-  word" // sanitize 把 " 换成 -
	if got := types(toks); got != want || st != lx.Initial() {
		t.Fatalf("got %q (state %d), want %q", got, st, want)
	}
}

func TestConcurrentTokenize(t *testing.T) {
	lx := mustLexer(t, "html")
	lines := []string{"<script>", "let x = `a ${b}`", "</script>", "<style>a { color: red }</style>"}
	done := make(chan []Token, 8)
	for range 8 {
		go func() {
			st := lx.Initial()
			var all []Token
			for range 50 {
				for _, l := range lines {
					var toks []Token
					toks, st = lx.TokenizeLine(l, st, nil)
					all = append(all, toks...)
				}
			}
			done <- all
		}()
	}
	first := <-done
	for range 7 {
		if got := <-done; !equalTokenSlices(got, first) {
			t.Fatal("concurrent results differ")
		}
	}
}

func equalTokenSlices(a, b []Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 插件语法里空匹配在两个状态间来回（push/pop 或 switchTo）时不能死循环：
// 超过上限后按"无进展"错误处理（整行一个 "" token、状态不变）。
func TestEmptyMatchCycleTerminates(t *testing.T) {
	grammars := map[string]string{
		"pushpop":  `{"tokenizer":{"root":[["","a","@b"]],"b":[["","b","@pop"]]}}`,
		"switchto": `{"tokenizer":{"root":[["",{"token":"a","switchTo":"@b"}]],"b":[["",{"token":"b","switchTo":"@root"}]]}}`,
	}
	for name, g := range grammars {
		r := NewRegistry()
		if err := r.Add(name, json.RawMessage(g)); err != nil {
			t.Fatal(err)
		}
		lx, err := r.Lexer(name)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var toks []Token
		var st State
		go func() {
			toks, st = lx.TokenizeLine("x", lx.Initial(), nil)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: TokenizeLine did not return", name)
		}
		if len(toks) != 1 || toks[0] != (Token{}) || st != lx.Initial() {
			t.Fatalf("%s: got %+v state %d", name, toks, st)
		}
	}
}

// goBack 回退到本次匹配起点（或更前）时 pos 不前进；插件语法不能借此让分词死循环。
// 与空匹配一样计入"无进展"上限，超过后整行一个 "" token、状态不变。
func TestGoBackNoProgressTerminates(t *testing.T) {
	grammars := map[string]string{
		"self":      `{"tokenizer":{"root":[["a",{"token":"x","goBack":1}]]}}`,
		"overshoot": `{"tokenizer":{"root":[["a",{"token":"x","goBack":5}]]}}`,
		"alternate": `{"tokenizer":{"root":[["a+",{"token":"x","goBack":2,"switchTo":"@b"}]],"b":[["a+",{"token":"y","goBack":2,"switchTo":"@root"}]]}}`,
		"pushpop":   `{"tokenizer":{"root":[["a",{"token":"x","goBack":1,"next":"@b"}]],"b":[["a",{"token":"y","goBack":1,"next":"@pop"}]]}}`,
		"group":     `{"tokenPostfix":"","tokenizer":{"root":[["(a)(a)",["x",{"token":"y","goBack":2}]]]}}`,
	}
	for name, g := range grammars {
		r := NewRegistry()
		if err := r.Add(name, json.RawMessage(g)); err != nil {
			t.Fatal(err)
		}
		lx, err := r.Lexer(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{"a", "aa"} {
			if name == "group" && line == "a" {
				continue
			}
			done := make(chan struct{})
			var toks []Token
			var st State
			go func() {
				toks, st = lx.TokenizeLine(line, lx.Initial(), nil)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s %q: TokenizeLine did not return", name, line)
			}
			if len(toks) > 4 {
				t.Fatalf("%s %q: %d tokens, want <= 4", name, line, len(toks))
			}
			if name == "group" {
				// 分组项 y 的 goBack 只能回到 y 自身的起点（而不是 x 的起点）：整体仍前进；
				// y 没有前进，不输出（零宽）；剩下的 "a" 不再匹配规则，按 defaultToken 输出
				if got := types(toks); got != "x source" || st != lx.Initial() {
					t.Fatalf("%s %q: got %q state %d", name, line, got, st)
				}
				if toks[1].ByteStart != 1 {
					t.Fatalf("%s %q: offsets %+v", name, line, toks)
				}
				continue
			}
			if len(toks) != 1 || toks[0] != (Token{}) || st != lx.Initial() {
				t.Fatalf("%s %q: got %+v state %d", name, line, toks, st)
			}
		}
	}
}

// tokenizeWithin 在 2 秒内分词一行（不能 panic、不能卡住），并要求 token 数有界。
func tokenizeWithin(t *testing.T, lx *Lexer, line string) ([]Token, State) {
	t.Helper()
	type res struct {
		toks []Token
		st   State
		pan  any
	}
	done := make(chan res, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- res{pan: p}
			}
		}()
		toks, st := lx.TokenizeLine(line, lx.Initial(), nil)
		done <- res{toks: toks, st: st}
	}()
	select {
	case r := <-done:
		if r.pan != nil {
			t.Fatalf("%q: panic: %v", line, r.pan)
		}
		if len(r.toks) > len(line)+2 {
			t.Fatalf("%q: %d tokens, want <= %d", line, len(r.toks), len(line)+2)
		}
		return r.toks, r.st
	case <-time.After(2 * time.Second):
		t.Fatalf("%q: TokenizeLine did not return", line)
	}
	return nil, 0
}

// @rematch 与 goBack 同用时，@rematch 从本次匹配起点重来（忽略 goBack）：
// pos 不能退到匹配起点之前（越界 panic），也不能在两处来回振荡（重置无进展计数、token 无界增长）。
func TestRematchGoBackTerminates(t *testing.T) {
	grammars := map[string]string{
		"underflow": `{"tokenizer":{"root":[["a",{"token":"@rematch","goBack":1,"switchTo":"@b"}]],"b":[["a","u"]]}}`,
		"oscillate": `{"tokenizer":{
			"root":[["x","x"],["a",{"token":"@rematch","goBack":1,"switchTo":"@b"}]],
			"b":[["x","x"],["a",{"token":"@rematch","goBack":1,"switchTo":"@root"}]]}}`,
	}
	want := map[string]map[string]string{
		// 回到 a 的起点后由 b 的规则匹配
		"underflow": {"a": "u.underflow", "xa": "source.underflow u.underflow"},
		// a 在 root/b 间 @rematch 互切且永不消耗字符：无进展上限 → 整行 ""
		"oscillate": {"a": "", "xa": ""},
	}
	for name, g := range grammars {
		r := NewRegistry()
		if err := r.Add(name, json.RawMessage(g)); err != nil {
			t.Fatal(err)
		}
		lx, err := r.Lexer(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range []string{"a", "xa"} {
			t.Run(name+"/"+line, func(t *testing.T) {
				toks, _ := tokenizeWithin(t, lx, line)
				if got := types(toks); got != want[name][line] {
					t.Fatalf("got %q (%+v)", got, toks)
				}
			})
		}
	}
}

// goBack 小于匹配长度时保持 Monaco 行为：token 覆盖整个匹配，下次从回退处继续。
func TestGoBackPartial(t *testing.T) {
	r := NewRegistry()
	err := r.Add("gb", json.RawMessage(`{"tokenPostfix":"","tokenizer":{"root":[
		["ab",{"token":"x","goBack":1,"next":"@b"}],
		["c","c"]
	],"b":[["b",{"token":"y","next":"@pop"}]]}}`))
	if err != nil {
		t.Fatal(err)
	}
	lx, _ := r.Lexer("gb")
	toks, st := lx.TokenizeLine("abc", lx.Initial(), nil)
	if got := types(toks); got != "x y c" || st != lx.Initial() {
		t.Fatalf("got %q (%+v) state %d", got, toks, st)
	}
	if toks[1].ByteStart != 1 {
		t.Fatalf("y should start at 1: %+v", toks)
	}
}

// 空正则 + 空分组动作：Monaco 只在还有剩余分组时走分组路径，这里不能越界 panic。
func TestEmptyGroupAction(t *testing.T) {
	r := NewRegistry()
	if err := r.Add("eg", json.RawMessage(`{"tokenizer":{"root":[["",[]]]}}`)); err != nil {
		t.Fatal(err)
	}
	lx, _ := r.Lexer("eg")
	_, panics0, _ := Stats()
	for _, line := range []string{"", "x", "xy"} {
		toks, st := tokenizeWithin(t, lx, line)
		if line == "" {
			// 与 Monaco 相同：展开空分组后行已结束，没有 token
			if len(toks) != 0 || st != lx.Initial() {
				t.Fatalf("%q: got %+v state %d", line, toks, st)
			}
			continue
		}
		// 非空行上会原地反复展开（Monaco 死循环）：无进展上限 → 整行 ""
		if len(toks) != 1 || toks[0] != (Token{}) || st != lx.Initial() {
			t.Fatalf("%q: got %+v state %d", line, toks, st)
		}
	}
	if _, p, _ := Stats(); p != panics0 {
		t.Fatalf("empty group handled via recover (%d panics), want a real fix", p-panics0)
	}
}

// 语法引发的运行时 panic（这里人为破坏编译结果来模拟未知 bug）不能传出 TokenizeLine：
// 转为与规则出错相同的安全结果（整行一个 "" token、状态不变），并计数。
func TestTokenizeLineRecoversPanic(t *testing.T) {
	r := NewRegistry()
	if err := r.Add("boom", json.RawMessage(`{"tokenizer":{"root":[["a","a"]]}}`)); err != nil {
		t.Fatal(err)
	}
	lx, _ := r.Lexer("boom")
	lx.g.defAction = nil // 不匹配 "a" 的字符会解引用 nil 动作
	errs0, panics0, _ := Stats()
	toks, st := tokenizeWithin(t, lx, "ab")
	if len(toks) != 1 || toks[0] != (Token{}) || st != lx.Initial() {
		t.Fatalf("got %+v state %d", toks, st)
	}
	errs, panics, _ := Stats()
	if panics-panics0 != 1 || errs-errs0 != 1 {
		t.Fatalf("Stats delta: errors %d panics %d, want 1 1", errs-errs0, panics-panics0)
	}
	// 后续调用不受影响
	if toks, _ := lx.TokenizeLine("a", lx.Initial(), nil); types(toks) != "a.boom" {
		t.Fatalf("after recover: %+v", toks)
	}
}
