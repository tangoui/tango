package highlight

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

const fixtureDir = "../langs/testdata/tokenize"

// tsLines 返回 Monaco typescript 分词用例里的所有行（按用例顺序）。
func tsLines(t testing.TB) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, "typescript.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases [][]struct {
		Line string `json:"line"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range cases {
		for _, l := range c {
			out = append(out, l.Line)
		}
	}
	return out
}

// tsDoc 把 TS 用例行循环拼成 n 行。
func tsDoc(t testing.TB, n int) string {
	src := tsLines(t)
	lines := make([]string, n)
	for i := range lines {
		lines[i] = src[i%len(src)]
	}
	return strings.Join(lines, "\n")
}

func tsLexer(t testing.TB) *monarch.Lexer {
	t.Helper()
	lx, err := monarch.NewRegistry().Lexer("typescript")
	if err != nil {
		t.Fatal(err)
	}
	return lx
}

func grammarLexer(t testing.TB, id, g string) *monarch.Lexer {
	t.Helper()
	r := monarch.NewRegistry()
	if err := r.Add(id, json.RawMessage(g)); err != nil {
		t.Fatal(err)
	}
	lx, err := r.Lexer(id)
	if err != nil {
		t.Fatal(err)
	}
	return lx
}

// fullTokenize 从头全量分词，作为对照。降级只因墙钟预算（负载很高的共享机器上偶发），
// 对照结果不能降级：降级行重算，直到得到完整结果。
func fullTokenize(lx *monarch.Lexer, s buffer.Snapshot) [][]monarch.Token {
	out := make([][]monarch.Token, s.LineCount())
	st := lx.Initial()
	for i := range out {
		for try := 0; ; try++ {
			toks, end, info := lx.TokenizeLineInfo(s.Line(i), st, nil)
			if !info.Degraded || try == 10 {
				out[i], st = toks, end
				break
			}
		}
	}
	return out
}

// highlightAll 反复调用 Highlight 直到全部就绪。
func highlightAll(t testing.TB, h *Highlighter, s buffer.Snapshot) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second * raceFactor)
	for !h.Highlight(s, s.LineCount(), 50*time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("Highlight never became ready")
		}
	}
}

func checkAllFinal(t testing.TB, h *Highlighter, lx *monarch.Lexer, s buffer.Snapshot, ctx string) {
	t.Helper()
	want := fullTokenize(lx, s)
	var dst []monarch.Token
	for i := range want {
		var final bool
		dst, final = h.Tokens(s, i, dst[:0])
		if !final {
			t.Fatalf("%s: line %d not final", ctx, i)
		}
		if !slices.Equal(dst, want[i]) {
			t.Fatalf("%s: line %d %q\n got  %+v\n want %+v", ctx, i, s.Line(i), dst, want[i])
		}
	}
}

// 随机编辑 200 次（含跨行删除、插入换行、注释/字符串/模板定界符等改变行尾状态的文本，
// 偶尔一次多处编辑）后，所有行的 token 等于从头全量分词。窗口很小，以覆盖 token 丢弃与重算。
func TestHighlightMatchesFullTokenize(t *testing.T) {
	for _, bg := range []bool{false, true} {
		name := "foreground"
		if bg {
			name = "background"
		}
		t.Run(name, func(t *testing.T) {
			lx := tsLexer(t)
			seed, _ := strconv.ParseUint(os.Getenv("HLSEED"), 10, 64) // 本地扫种子：HLSEED=n；HLDEBUG=1 逐次校验并打印编辑
			rng := rand.New(rand.NewPCG(1+seed, 2))
			s := buffer.NewSnapshot(tsDoc(t, 400))
			h := New(lx)
			h.TokenWindow = 30
			h.MaxRetries = 0
			var snaps chan buffer.Snapshot
			if bg {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				snaps = make(chan buffer.Snapshot, 1)
				h.Start(ctx, snaps, func() {})
			}
			send := func(s buffer.Snapshot) {
				if snaps == nil {
					return
				}
				for { // 通道满时丢弃未读的旧快照，只留最新（后台可能同时取走，故都用非阻塞操作）
					select {
					case snaps <- s:
						return
					default:
					}
					select {
					case <-snaps:
					default:
					}
				}
			}
			h.Highlight(s, s.LineCount(), time.Hour)
			send(s)
			inserts := []string{"x", "/*", "*/", "`", "${", "}", "'", "\"", "\n", "\n\n", "// c\n", "a\nb\nc", " ", "\r\n", "é", "😀"}
			for n := range 200 {
				var edits []buffer.Edit
				k := 1 + rng.IntN(3)*rng.IntN(2) // 多数是单处编辑
				pos := 0
				for range k {
					if pos > s.Len() {
						break
					}
					start := pos + rng.IntN(s.Len()-pos+1)
					end := start
					if rng.IntN(3) == 0 {
						end = min(s.Len(), start+rng.IntN(80))
					}
					txt := ""
					if end == start || rng.IntN(2) == 0 {
						txt = inserts[rng.IntN(len(inserts))]
					}
					edits = append(edits, buffer.Edit{Start: start, End: end, Text: txt})
					pos = end + 1
				}
				ns, err := s.Apply(edits)
				if err != nil {
					continue // 边界落在多字节字符中间：换一组
				}
				if os.Getenv("HLDEBUG") != "" {
					t.Logf("edit %d: %+v lines %d->%d", n, edits, s.LineCount(), ns.LineCount())
				}
				h.Edit(s, ns, edits)
				s = ns
				send(s)
				if rng.IntN(4) == 0 {
					h.Highlight(s, rng.IntN(s.LineCount()+1), time.Duration(rng.IntN(200))*time.Microsecond)
				}
				if rng.IntN(5) == 0 {
					line := rng.IntN(s.LineCount())
					h.Tokens(s, line, nil) // 移动 token 窗口
				}
				if n%25 == 24 || os.Getenv("HLDEBUG") != "" {
					highlightAll(t, h, s)
					checkAllFinal(t, h, lx, s, "after edit "+strconv.Itoa(n))
				}
			}
			highlightAll(t, h, s)
			checkAllFinal(t, h, lx, s, "end")
		})
	}
}

// 改一行内的字符（行尾状态不变）只重新分词该行。
func TestHighlightStopsEarly(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 300))
	h := New(lx)
	highlightAll(t, h, s)
	// 选一行普通代码（"var x = function() { };" 类）在行首插入一个标识符字符。
	line := -1
	for i := 100; i < s.LineCount(); i++ {
		if strings.HasPrefix(s.Line(i), "var ") {
			line = i
			break
		}
	}
	if line < 0 {
		t.Fatal("no var line")
	}
	off := s.LineStart(line) + len("var ")
	edits := []buffer.Edit{{Start: off, End: off, Text: "q"}}
	ns, err := s.Apply(edits)
	if err != nil {
		t.Fatal(err)
	}
	h.Edit(s, ns, edits)
	// 编辑行之上、之下的行仍是最终结果。
	if _, final := h.Tokens(ns, line-1, nil); !final {
		t.Fatal("line above edit lost final")
	}
	if _, final := h.Tokens(ns, line, nil); final {
		t.Fatal("edited line still final before Highlight")
	}
	n0 := h.tokenized.Load()
	if !h.Highlight(ns, ns.LineCount(), time.Hour) {
		t.Fatal("not ready")
	}
	if d := h.tokenized.Load() - n0; d != 1 {
		t.Fatalf("re-tokenized %d lines, want 1", d)
	}
	checkAllFinal(t, h, lx, ns, "after edit")
}

// 预算 2 ms：Highlight 在预算内（加上最多一行的超出）返回，并如实报告未就绪。
func TestHighlightBudget(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 20000))
	h := New(lx)
	const budget = 2 * time.Millisecond
	start := time.Now()
	ready := h.Highlight(s, s.LineCount(), budget)
	d := time.Since(start)
	if ready {
		t.Fatal("2ms budget reported all 20k lines ready")
	}
	limit := budget + 5*time.Millisecond*raceFactor
	// Shared CI runners (TANGO_SMOKE_TIMEOUT_SCALE, which CI sets) get
	// descheduled for tens of milliseconds; an unbudgeted pass over 20k
	// lines still misses this gate by far more.
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		limit = time.Duration(float64(limit) * 4 * f)
	}
	if d > limit {
		t.Fatalf("Highlight took %v, budget %v (limit %v)", d, budget, limit)
	}
	if _, final := h.Tokens(s, s.LineCount()-1, nil); final {
		t.Fatal("last line final after budgeted pass")
	}
	if _, final := h.Tokens(s, 0, nil); !final {
		t.Fatal("first line not final after budgeted pass")
	}
	// 预算 0 不分词，只报告状态。
	n0 := h.tokenized.Load()
	if h.Highlight(s, s.LineCount(), 0) || h.tokenized.Load() != n0 {
		t.Fatal("zero budget did work or reported ready")
	}
	// 只要求前面若干行：已就绪即返回 true。
	if !h.Highlight(s, 1, budget) {
		t.Fatal("line 0 not ready")
	}
	highlightAll(t, h, s)
	checkAllFinal(t, h, lx, s, "end")
}

// 100 万行文件末尾编辑后，前 100 行的 token 仍是最终结果、未被作废，也不需要重新分词。
func TestHighlightEditFarAway(t *testing.T) {
	lx := tsLexer(t)
	const n = 1_000_000
	src := tsLines(t)[:40]
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(src[i%len(src)])
	}
	s := buffer.NewSnapshot(b.String())
	h := New(lx)
	if !h.Highlight(s, 100, time.Hour) {
		t.Fatal("first 100 lines not ready")
	}
	want := make([][]monarch.Token, 100)
	for i := range want {
		want[i], _ = h.Tokens(s, i, nil)
	}
	for _, txt := range []string{"x", "\n/* open comment\n", "`"} {
		edits := []buffer.Edit{{Start: s.Len(), End: s.Len(), Text: txt}}
		ns, err := s.Apply(edits)
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		h.Edit(s, ns, edits)
		if d := time.Since(start); d > 200*time.Millisecond*raceFactor {
			t.Errorf("Edit on 1M lines took %v", d)
		}
		s = ns
		for i := range 100 {
			got, final := h.Tokens(s, i, nil)
			if !final || !slices.Equal(got, want[i]) {
				t.Fatalf("after %q: line %d final=%v tokens %+v want %+v", txt, i, final, got, want[i])
			}
		}
		n0 := h.tokenized.Load()
		if !h.Highlight(s, 100, time.Millisecond) {
			t.Fatal("first 100 lines not ready after far edit")
		}
		if d := h.tokenized.Load() - n0; d != 0 {
			t.Fatalf("far edit re-tokenized %d lines near the top", d)
		}
	}
	// 末尾行尚未就绪（中间的行从未分词）。
	if _, final := h.Tokens(s, s.LineCount()-1, nil); final {
		t.Fatal("last line final without tokenizing")
	}
}

// 病态语法使一行 Degraded：返回的输入状态恰好等于旧的行尾状态，但不能据此停止——下一行要重新分词，
// 下面的行不算最终；后续 Highlight 会重试该行，直到重试次数用完才接受降级结果。
func TestDegradedLineRetried(t *testing.T) {
	lx := grammarLexer(t, "patho", `{"tokenPostfix":"","tokenizer":{"root":[
		["x","x"],
		["\\{",{"token":"open","next":"@inner"}],
		["\\}","close"]
	],"inner":[["x","x"],["(a+)+(?=b)","a"],["\\}",{"token":"close","next":"@pop"}]]}}`)
	s := buffer.NewSnapshot("{\nxx\n}\nx")
	h := New(lx)
	h.RetryDelay = 0
	h.MaxRetries = 1
	highlightAll(t, h, s)
	// 把第 1 行改成病态行：降级时返回输入状态 inner，与旧行尾状态（"xx" 留在 inner）相同。
	patho := "xx" + strings.Repeat("a", 200) + "c"
	edits := []buffer.Edit{{Start: s.LineStart(1), End: s.LineStart(1) + 2, Text: patho}}
	ns, err := s.Apply(edits)
	if err != nil {
		t.Fatal(err)
	}
	h.Edit(s, ns, edits)
	s = ns
	_, _, deg0 := monarch.Stats()
	n0 := h.tokenized.Load()
	if h.Highlight(s, s.LineCount(), time.Hour) {
		t.Fatal("ready with a degraded line pending retry")
	}
	if _, _, deg := monarch.Stats(); deg-deg0 != 1 {
		t.Fatalf("degraded delta = %d, want 1", deg-deg0)
	}
	if d := h.tokenized.Load() - n0; d < 2 {
		t.Fatalf("tokenized %d lines: the line after a degraded line must be re-tokenized", d)
	}
	for _, l := range []int{1, 2, 3} {
		if _, final := h.Tokens(s, l, nil); final {
			t.Fatalf("line %d final while line 1 is degraded and pending retry", l)
		}
	}
	if _, final := h.Tokens(s, 0, nil); !final {
		t.Fatal("line 0 lost final")
	}
	// 重试：再次降级（第 2 次尝试），重试次数用完，接受结果。
	if !h.Highlight(s, s.LineCount(), time.Hour) {
		t.Fatal("not ready after retries are exhausted")
	}
	if _, _, deg := monarch.Stats(); deg-deg0 != 2 {
		t.Fatalf("degraded delta = %d, want 2 (one retry)", deg-deg0)
	}
	toks, final := h.Tokens(s, 1, nil)
	if !final || len(toks) == 0 || toks[len(toks)-1].Type != "" {
		t.Fatalf("degraded line tokens %+v final=%v", toks, final)
	}
	// 把病态行改回正常行：重试记录清除，结果与全量分词一致。
	edits = []buffer.Edit{{Start: s.LineStart(1), End: s.LineStart(1) + len(patho), Text: "x"}}
	ns, err = s.Apply(edits)
	if err != nil {
		t.Fatal(err)
	}
	h.Edit(s, ns, edits)
	highlightAll(t, h, ns)
	checkAllFinal(t, h, lx, ns, "fixed")
}

// 后台 goroutine 在快照更新后补齐剩余行并调用 ready。
func TestBackgroundCatchUp(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 5000))
	h := New(lx)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	snaps := make(chan buffer.Snapshot, 1)
	var readyN atomic.Int64
	readyCh := make(chan struct{}, 1)
	h.Start(ctx, snaps, func() {
		readyN.Add(1)
		select {
		case readyCh <- struct{}{}:
		default:
		}
	})
	waitFinal := func(s buffer.Snapshot) {
		t.Helper()
		deadline := time.After(30 * time.Second * raceFactor)
		for {
			if _, final := h.Tokens(s, s.LineCount()-1, nil); final {
				return
			}
			select {
			case <-readyCh:
			case <-deadline:
				t.Fatal("background never caught up")
			}
		}
	}
	// 前台只做可见区（前 50 行），其余由后台补齐。
	h.Highlight(s, 50, time.Hour)
	snaps <- s
	waitFinal(s)
	if readyN.Load() == 0 {
		t.Fatal("ready never called")
	}
	checkAllFinal(t, h, lx, s, "initial")

	// 顶部打开块注释：直到下一个 */ 的行尾状态都改变，后台在新快照上重新补齐。
	edits := []buffer.Edit{{Start: 0, End: 0, Text: "/*\n"}}
	ns, err := s.Apply(edits)
	if err != nil {
		t.Fatal(err)
	}
	h.Edit(s, ns, edits)
	r0 := readyN.Load()
	h.Highlight(ns, 1, time.Hour) // 前台只做第 0 行，其余（状态全变）都留给后台
	snaps <- ns
	waitFinal(ns)
	if readyN.Load() == r0 {
		t.Fatal("ready not called after update")
	}
	checkAllFinal(t, h, lx, ns, "after edit")

	// 快照通道关闭后 goroutine 退出（不再调用 ready）。
	close(snaps)
}

// 无语言（nil Lexer）：所有行最终、没有 token。
func TestNilLexer(t *testing.T) {
	h := New(nil)
	s := buffer.NewSnapshot("a\nb")
	if !h.Highlight(s, 2, 0) {
		t.Fatal("nil lexer not ready")
	}
	toks, final := h.Tokens(s, 1, nil)
	if !final || len(toks) != 0 {
		t.Fatalf("got %+v %v", toks, final)
	}
}

// Reset 换 Lexer（Registry.Add 之后旧 State 全部作废）：所有行重新分词。
func TestResetLexer(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 200))
	h := New(lx)
	highlightAll(t, h, s)
	lx2 := tsLexer(t)
	h.Reset(lx2)
	if _, final := h.Tokens(s, 0, nil); final {
		t.Fatal("final after Reset")
	}
	highlightAll(t, h, s)
	checkAllFinal(t, h, lx2, s, "after reset")
}

// Highlight 收到与 Edit 不连续的快照（未经 Edit）：整篇重新分词而不是给出错误结果。
func TestUnrelatedSnapshotResets(t *testing.T) {
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 100))
	h := New(lx)
	highlightAll(t, h, s)
	ns, err := s.Apply([]buffer.Edit{{Start: 0, End: 0, Text: "/*"}})
	if err != nil {
		t.Fatal(err)
	}
	highlightAll(t, h, ns)
	checkAllFinal(t, h, lx, ns, "after unannounced edit")
}
