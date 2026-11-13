//go:build !race

package monarch

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/regex"
)

// assertBudget 检查墙钟预算。-race 下检测器把执行拖慢数倍，预算断言在 budget_race_test.go 里是空操作。
func assertBudget(t testing.TB, what string, d, limit time.Duration) {
	t.Helper()
	if d > limit {
		t.Fatalf("%s took %v (> %v)", what, d, limit)
	}
}

// TestTS20kBudget 在普通测试里跑一次 2 万行全量分词，守住规格 6.6 的 2 秒预算。
// 墙钟预算：本文件带 !race 构建标签，-race 下不编译（检测器使执行慢数倍）。
func TestTS20kBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	lines := ts20k(t)
	lx := mustLexer(t, "typescript")
	start := time.Now()
	st := lx.Initial()
	var toks []Token
	for _, l := range lines {
		toks, st = lx.TokenizeLine(l, st, toks[:0])
	}
	d := time.Since(start)
	s, bt, to := regex.Stats()
	t.Logf("20k TS lines: %v; regex.Stats std=%d backtrack=%d timeouts=%d", d, s, bt, to)
	assertBudget(t, "20k TS lines", d, 2*time.Second)
}
