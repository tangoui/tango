//go:build race

package monarch

import (
	"testing"
	"time"
)

// assertBudget 在 -race 下不检查墙钟预算（见 budget_test.go）；只记录耗时。
func assertBudget(t testing.TB, what string, d, limit time.Duration) {
	t.Helper()
	t.Logf("%s took %v (budget %v not enforced under -race)", what, d, limit)
}
