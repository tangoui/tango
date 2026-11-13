//go:build !race

package regex

import (
	"testing"
	"time"
)

// assertBudget 检查墙钟预算。-race 下检测器把执行拖慢数倍，预算断言在 budget_race_test.go 里是空操作。
func assertBudget(t testing.TB, what string, d, limit time.Duration) {
	t.Helper()
	if d > limit {
		t.Fatalf("%s took %v (> %v)", what, d, limit)
	}
}
