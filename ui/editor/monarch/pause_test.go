package monarch

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/regex"
)

func TestSchedulerPauseDoesNotDegrade(t *testing.T) {
	lx := addLexer(t, "pause", `{"tokenPostfix":"","tokenizer":{"root":[["x","x"],["y","y"]]}}`)
	line := strings.Repeat("xy", 80)
	want, wantState, _ := lx.TokenizeLineInfo(line, lx.Initial(), nil)
	now := time.Now()
	calls := 0
	clock := func() time.Time {
		calls++
		// Every check simulates another scheduler/GC pause, not just one lucky retry.
		now = now.Add(time.Second)
		return now
	}
	got, state, info := lx.tokenizeLineInfo(line, lx.Initial(), nil, clock)
	if info.Degraded || state != wantState || !equalTokenSlices(got, want) {
		t.Fatalf("pause changed tokens/state: degraded=%v state=%d tokens=%+v", info.Degraded, state, got)
	}
	if calls < 2 {
		t.Fatal("clock was not checked during tokenization")
	}
}

func TestWorkBudgetStillBoundsSlowLine(t *testing.T) {
	lx := addLexer(t, "slow-clock", `{"tokenPostfix":"","tokenizer":{"root":[["[^z]*z","z"],[".","c"]]}}`)
	now := time.Now()
	clock := func() time.Time { now = now.Add(time.Second); return now }
	_, _, info := lx.tokenizeLineInfo(strings.Repeat("a", MaxLineLength-1), lx.Initial(), nil, clock)
	if !info.Degraded {
		t.Fatal("expensive line did not degrade")
	}
}

// Review I1: hundreds of individually sub-timeout matches must not bypass the
// line budget just because the line is only 400 bytes long.
func TestCumulativeShortBacktrackingDegrades(t *testing.T) {
	lx := addLexer(t, "short-backtracking", `{"tokenPostfix":"","tokenizer":{"root":[["(a+)+(?=b)","a"],[".","c"]]}}`)
	line := strings.Repeat(strings.Repeat("a", 19)+"c", 20)
	start := time.Now()
	_, _, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
	if !info.Degraded {
		t.Fatal("cumulative expensive short line did not degrade")
	}
	assertBudget(t, "cumulative short backtracking", time.Since(start), 500*time.Millisecond)
}

func TestBacktrackingPauseIsNotCharged(t *testing.T) {
	re, err := regex.Compile(`x(?=y)`, "")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	times := []time.Time{base, base.Add(10 * time.Second), base.Add(10 * time.Second), base.Add(10*time.Second + time.Microsecond)}
	calls := 0
	b := lineBudget{start: base, now: func() time.Time {
		if calls >= len(times) {
			t.Fatal("unexpected extra confirmation")
		}
		now := times[calls]
		calls++
		return now
	}}
	got, err := b.match(re, "xy", nil, 0)
	if err != nil || len(got) != 2 || got[0] != 0 || got[1] != 1 || b.backtrackSpent != time.Microsecond || calls != 4 {
		t.Fatalf("match=%v err=%v charged=%v clock calls=%d", got, err, b.backtrackSpent, calls)
	}
}

func TestCumulativeShortGuardBacktrackingDegrades(t *testing.T) {
	lx := addLexer(t, "short-guard", `{"tokenPostfix":"","tokenizer":{"root":[["a+c",{"cases":{"~(a+)+(?=b)":"bad","@default":"ok"}}]]}}`)
	line := strings.Repeat(strings.Repeat("a", 19)+"c", 20)
	start := time.Now()
	_, _, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
	if !info.Degraded {
		t.Fatal("cumulative expensive guards escaped the line budget")
	}
	assertBudget(t, "cumulative guard backtracking", time.Since(start), 500*time.Millisecond)
}

func TestWorkBudgetHardCap(t *testing.T) {
	b := lineBudget{start: time.Now(), now: time.Now, work: maxLineWork}
	if b.check(42, true) == nil {
		t.Fatal("hard work cap ignored")
	}
}
