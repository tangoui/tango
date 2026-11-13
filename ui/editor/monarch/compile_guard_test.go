package monarch

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// blowupGrammar 返回 k 个引用 × levels 层的 @attr 链：展开后长度约 k^levels。
func blowupGrammar(k, levels int) string {
	var sb strings.Builder
	sb.WriteString(`{"a0":"x"`)
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&sb, `,"a%d":"%s"`, i, strings.Repeat(fmt.Sprintf("@a%d", i-1), k))
	}
	fmt.Fprintf(&sb, `,"tokenizer":{"root":[["@a%d","t"]]}}`, levels)
	return sb.String()
}

// @attr 指数展开：展开后的正则源码超过 maxExpandedRegex 时编译报错，且要快（不能先展开到 OOM）。
func TestAttrExpansionBlowupErrors(t *testing.T) {
	for _, k := range []int{12, 64} {
		g := blowupGrammar(k, 5)
		r := NewRegistry()
		if err := r.Add("blowup", json.RawMessage(g)); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		_, err := r.Lexer("blowup")
		assertBudget(t, fmt.Sprintf("k=%d blow-up compile", k), time.Since(start), 50*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "too long") {
			t.Fatalf("k=%d (%d-byte grammar): err = %v, want 'too long' error", k, len(g), err)
		}
	}
	// 合理大小的展开不受影响。
	r := NewRegistry()
	if err := r.Add("ok", json.RawMessage(blowupGrammar(3, 3))); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Lexer("ok"); err != nil {
		t.Fatal(err)
	}
}

// 编译阶段的 panic（不可信插件语法触发的未知 bug）转为 Lexer 的错误，且被缓存。
func TestCompilePanicBecomesError(t *testing.T) {
	old := compileGrammarFunc
	t.Cleanup(func() { compileGrammarFunc = old })
	calls := 0
	compileGrammarFunc = func(id string, n *node) (*grammar, error) {
		calls++
		panic("boom")
	}
	r := NewRegistry()
	if err := r.Add("p", json.RawMessage(`{"tokenizer":{"root":[["a","a"]]}}`)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		lx, err := r.Lexer("p")
		if lx != nil || err == nil || !strings.Contains(err.Error(), "panic") {
			t.Fatalf("got %v, %v; want panic error", lx, err)
		}
	}
	if calls != 1 {
		t.Fatalf("compiled %d times, want 1 (result cached)", calls)
	}
}

// 一个慢编译不阻塞其他语言的 Lexer()；同一语言的并发调用只编译一次。
func TestSlowCompileDoesNotBlockRegistry(t *testing.T) {
	old := compileGrammarFunc
	t.Cleanup(func() { compileGrammarFunc = old })
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	var slowCalls int
	compileGrammarFunc = func(id string, n *node) (*grammar, error) {
		if id == "slow" {
			slowCalls++
			entered <- struct{}{}
			<-release
		}
		return old(id, n)
	}
	r := NewRegistry()
	if err := r.Add("slow", json.RawMessage(`{"tokenizer":{"root":[["a","a"]]}}`)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := r.Lexer("slow"); done <- err }()
	}
	<-entered
	fast := make(chan error, 1)
	go func() { _, err := r.Lexer("go"); fast <- err }()
	select {
	case err := <-fast:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Lexer(go) blocked by a slow compile of another language")
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if slowCalls != 1 {
		t.Fatalf("slow grammar compiled %d times, want 1", slowCalls)
	}
}
