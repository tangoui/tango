package monarch

import (
	"strings"
	"testing"
)

func shortBacktrackLexer(tb testing.TB) *Lexer {
	tb.Helper()
	reg := NewRegistry()
	if err := reg.Add("perf", []byte(`{"tokenizer":{"root":[[{"$regex":"[a-z](?=[0-9])"},"keyword"],[{"$regex":"."},"other"]]}}`)); err != nil {
		tb.Fatal(err)
	}
	lx, err := reg.Lexer("perf")
	if err != nil {
		tb.Fatal(err)
	}
	return lx
}

func TestShortBacktrackRuneAllocationBudget(t *testing.T) {
	if raceFactor != 1 {
		// The race build randomly drops sync.Pool entries, including the
		// regex runners. Measure allocations on the normal build; all
		// token/Unicode/budget correctness tests still run under -race.
		t.Skip("allocation budget excludes race-instrumented pool drops")
	}
	lx := shortBacktrackLexer(t)
	line := strings.Repeat("a", 128)
	var toks []Token
	allocs := testing.AllocsPerRun(30, func() { toks, _ = lx.TokenizeLine(line, lx.Initial(), toks[:0]) })
	if len(toks) != 1 || toks[0].ByteStart != 0 || toks[0].Type != "other.perf" {
		t.Fatalf("unexpected tokens: %v", toks)
	}
	if allocs > 280 {
		t.Fatalf("128-character backtracking line uses %.0f allocations, want <=280 with shared line runes", allocs)
	}
}

func BenchmarkShortBacktrackLine(b *testing.B) {
	lx := shortBacktrackLexer(b)
	line := strings.Repeat("a", 128)
	var toks []Token
	b.ReportAllocs()
	for b.Loop() {
		toks, _ = lx.TokenizeLine(line, lx.Initial(), toks[:0])
	}
}

func TestShortBacktrackBoundaryTokens(t *testing.T) {
	lx := addLexer(t, "boundary", `{"tokenPostfix":"","tokenizer":{"root":[[{"$regex":"(é|😀|�)(?=x)"},"marked"],[{"$regex":"."},"other"]]}}`)
	got, end, info := lx.TokenizeLineInfo("aéx😀x"+string([]byte{0xff})+"x", lx.Initial(), nil)
	want := []Token{{Start: 0, ByteStart: 0, Type: "other"}, {Start: 1, ByteStart: 1, Type: "marked"}, {Start: 2, ByteStart: 3, Type: "other"}, {Start: 3, ByteStart: 4, Type: "marked"}, {Start: 5, ByteStart: 8, Type: "other"}, {Start: 6, ByteStart: 9, Type: "marked"}, {Start: 7, ByteStart: 10, Type: "other"}}
	if info.Degraded || end != lx.Initial() || !equalTokenSlices(got, want) {
		t.Fatalf("got %v state=%v info=%v, want %v", got, end, info, want)
	}
}
