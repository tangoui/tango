package monarch

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/regex"
)

type countedGuardRegexp struct {
	regex.Regexp
	calls *int
	err   error
}

func (r countedGuardRegexp) Engine() regex.Engine { return regex.Std }
func (r countedGuardRegexp) TryMatchStart(string, []rune) ([]int, error) {
	*r.calls++
	return nil, r.err
}

func TestCasesStopAfterGuardTimeout(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, negated := range []bool{false, true} {
			t.Run(fmt.Sprintf("nested=%v/negated=%v", nested, negated), func(t *testing.T) {
				lx := addLexer(t, "stop", `{"tokenPostfix":"","tokenizer":{"root":[["x ","prefix"],["[a-z]+","word"]]}}`)
				first, later := 0, 0
				failed := testRe{re: countedGuardRegexp{calls: &first, err: regex.ErrTimeout}}
				next := testRe{re: countedGuardRegexp{calls: &later}}
				leaf := &action{kind: actString, token: "wrong"}
				tail := &action{kind: actCases, cases: []caseArm{{test: func(c *matchCtx) bool { return next.test(c.matched, c) }, value: leaf}, {value: leaf}}}
				branch := &action{kind: actCases, cases: []caseArm{
					{test: func(c *matchCtx) bool {
						hit := failed.test(c.matched, c)
						if negated {
							return !hit
						}
						return hit
					}, value: tail},
					{test: func(c *matchCtx) bool { return next.test(c.matched, c) }, value: tail},
					{value: tail},
				}}
				if nested {
					branch = &action{kind: actCases, cases: []caseArm{{value: branch}}}
				}
				lx.g.tokenizer["root"][1].action = branch
				toks, state, info := lx.TokenizeLineInfo("x abc", lx.Initial(), nil)
				if !info.Degraded || state != lx.Initial() || len(toks) != 2 || toks[1].ByteStart != 2 || toks[1].Type != "" {
					t.Fatalf("tokens=%+v state=%v info=%+v", toks, state, info)
				}
				if first != 1 || later != 0 {
					t.Fatalf("guard calls first=%d later=%d, want 1/0", first, later)
				}
			})
		}
	}
}

func TestBudgetExhaustionRejectsLaterPaths(t *testing.T) {
	first, later := 0, 0
	b := lineBudget{now: time.Now}
	failed := countedGuardRegexp{calls: &first, err: regex.ErrTimeout}
	next := countedGuardRegexp{calls: &later}
	if _, err := b.match(failed, "abc", nil, 3); err == nil || !b.exhausted {
		t.Fatal("timeout did not latch exhaustion")
	}
	if _, err := b.match(next, "abc", nil, 4); err == nil {
		t.Fatal("later match allowed")
	}
	if b.check(4, false) == nil {
		t.Fatal("throttled budget check missed exhaustion")
	}
	ctx := &matchCtx{budget: &b} // no timedOut flag: the line latch must suffice
	guard := testRe{re: next}
	if guard.test("abc", ctx) {
		t.Fatal("exhausted guard matched")
	}
	def := &action{kind: actString}
	cases := &action{kind: actCases, cases: []caseArm{{test: func(*matchCtx) bool { later++; return true }, value: def}}}
	if cases.test(ctx, def) != def {
		t.Fatal("exhausted cases selected a branch")
	}
	// A search's fast RE2 path must not bypass exhaustion either.
	search := compiledRe{re: next}
	if _, err := search.searchIndex("abc", 4, &b); err == nil {
		t.Fatal("search allowed after exhaustion")
	}
	c := collector{budget: b}
	if _, err := c.nested("abc", true, embedded{}, 4); err == nil {
		t.Fatal("nested fallback allowed after exhaustion")
	}
	lx := addLexer(t, "entry", `{"tokenizer":{"root":[[".","x"]]}}`)
	if _, err := lx.tokenizeLS("abc", true, lx.mustState(lx.Initial()), 4, &c); err == nil {
		t.Fatal("recursive lexer entry allowed after exhaustion")
	}
	if first != 1 || later != 0 {
		t.Fatalf("engine/guard calls first=%d later=%d", first, later)
	}
}

func TestGuardNonMatchStillFallsThrough(t *testing.T) {
	lx := addLexer(t, "fallback", `{"tokenPostfix":"","tokenizer":{"root":[["[a-z]+",{"cases":{"~z(?=z)":"wrong","~a(?=b)bc":{"cases":{"~z(?=z)":"wrong","@default":"right"}},"@default":"wrong"}}]]}}`)
	toks, _, info := lx.TokenizeLineInfo("abc", lx.Initial(), nil)
	if info.Degraded || len(toks) != 1 || toks[0].Type != "right" {
		t.Fatalf("tokens=%+v info=%+v", toks, info)
	}
}

func TestTenPathologicalGuardsStopAtFirstTimeout(t *testing.T) {
	var grammar strings.Builder
	grammar.WriteString(`{"tokenPostfix":"","tokenizer":{"root":[["[a-z]+",{"cases":{`)
	for i := range 10 {
		fmt.Fprintf(&grammar, `"~(a+)+(?=b%d)":"wrong",`, i)
	}
	grammar.WriteString(`"@default":"fallback"}}]]}}`)
	lx := addLexer(t, "ten-guards", grammar.String())
	_, _, before := regex.Stats()
	start := time.Now()
	toks, _, info := lx.TokenizeLineInfo(strings.Repeat("a", 60)+"c", lx.Initial(), nil)
	elapsed := time.Since(start)
	t.Logf("10-guard line: %v", elapsed)
	if !info.Degraded || len(toks) != 1 || toks[0].ByteStart != 0 || toks[0].Type != "" {
		t.Fatalf("tokens=%+v info=%+v", toks, info)
	}
	if _, _, after := regex.Stats(); after-before != 1 {
		t.Fatalf("confirmed timeouts=%d, want exactly one", after-before)
	}
	// The exactly-one confirmed timeout above and the counter tests pin that
	// later guards never run; this wall-clock ceiling only catches the
	// seconds-scale accumulation (ten guards took ~2.1s before the fix).
	assertBudget(t, "ten pathological guards", elapsed, 2*regex.MatchTimeout+300*time.Millisecond)
}
