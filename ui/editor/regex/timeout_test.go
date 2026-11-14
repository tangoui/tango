package regex

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TryMatchStart 把回溯超时报告为 ErrTimeout，调用方据此区分"不匹配"与"放弃匹配"。
func TestTryMatchStartReportsTimeout(t *testing.T) {
	re := mustCompile(t, "(a+)+(?=b)", "")
	s := strings.Repeat("a", 60) + "c"
	start := time.Now()
	m, err := re.TryMatchStart(s, nil)
	d := time.Since(start)
	if m != nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, %v; want nil, ErrTimeout", m, err)
	}
	if d < 2*MatchTimeout {
		t.Fatalf("returned after %v, before two full MatchTimeout allowances %v", d, 2*MatchTimeout)
	}
	// Upper bound only catches seconds-scale regressions (per-guard or
	// per-retry accumulation); the exact attempt count is pinned without a
	// wall clock by TestRepeatedTimeoutIsCountedOnce. Two confirmed attempts
	// take ~200ms, so a tighter ceiling fails on scheduler stalls under load.
	assertBudget(t, "timed-out match with confirmation", d, 2*MatchTimeout+300*time.Millisecond)

	// 正常匹配与不匹配都没有错误；rs 传入时结果相同。
	for _, src := range []string{`x(?=y)`, `[a-z]+`} {
		re := mustCompile(t, src, "")
		if m, err := re.TryMatchStart("xy", nil); m == nil || err != nil {
			t.Errorf("%s on xy: %v, %v", src, m, err)
		}
		if m, err := re.TryMatchStart("1", []rune("1")); m != nil || err != nil {
			t.Errorf("%s on 1: %v, %v", src, m, err)
		}
	}
}

func TestMatchTimeoutIs100ms(t *testing.T) {
	if MatchTimeout != 100*time.Millisecond {
		t.Fatalf("MatchTimeout = %v, want 100ms", MatchTimeout)
	}
}
