package monarch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func runePerfLexer(tb testing.TB, grammar string) *Lexer {
	tb.Helper()
	r := NewRegistry()
	if err := r.Add("rune-perf", json.RawMessage(grammar)); err != nil {
		tb.Fatal(err)
	}
	lx, err := r.Lexer("rune-perf")
	if err != nil {
		tb.Fatal(err)
	}
	return lx
}

// A long line matched entirely by RE2 has no need for regexp2's rune/index
// buffers. Reintroducing eager newLineRunes makes long allocate more than
// short, even with identical token/state results.
func TestRE2LongLineDoesNotAllocateRuneIndex(t *testing.T) {
	for _, grammar := range []string{
		`{"tokenizer":{"root":[["[a-z]+","identifier"]]}}`,
		// A filtered-out backtracking rule is not a reason to build runes.
		`{"tokenizer":{"root":[["z(?=x)","special"],["[a-z]+","identifier"]]}}`,
	} {
		lx := runePerfLexer(t, grammar)
		short, long := strings.Repeat("a", 100), strings.Repeat("a", 4096)
		initial := lx.Initial()
		dst := make([]Token, 0, 8)
		measure := func(line string) float64 {
			return testing.AllocsPerRun(30, func() {
				dst, _, _ = lx.TokenizeLineInfo(line, initial, dst[:0])
			})
		}
		small, large := measure(short), measure(long)
		if large > small {
			t.Fatalf("RE2 long line allocated %.0f times, short %.0f; rune/index buffers must be lazy", large, small)
		}
		if !slices.Equal(dst, []Token{{Start: 0, ByteStart: 0, Type: "identifier.rune-perf"}}) {
			t.Fatalf("tokens changed: %v", dst)
		}
	}
}

// The first actual backtracking rule can appear after a long RE2 prefix.
// Its rune window must start at the current byte, not the line's start.
func TestLazyRunesBacktrackAfterRE2Prefix(t *testing.T) {
	lx := runePerfLexer(t, `{"tokenizer":{"root":[["[a-z]+","identifier"],["(?=世)世😀","special"]]}}`)
	line := strings.Repeat("a", 300) + "世😀" + strings.Repeat("b", 600)
	tokens, end, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
	want := []Token{
		{Start: 0, ByteStart: 0, Type: "identifier.rune-perf"},
		{Start: 300, ByteStart: 300, Type: "special.rune-perf"},
		{Start: 303, ByteStart: 307, Type: "identifier.rune-perf"},
	}
	if !slices.Equal(tokens, want) || end != lx.Initial() || info.Degraded {
		t.Fatalf("tokens=%v state=%v info=%+v; want %v, initial state, no degradation", tokens, end, info, want)
	}
}

func BenchmarkRE2LongLine(b *testing.B) {
	lx := runePerfLexer(b, `{"tokenizer":{"root":[["[a-z]+","identifier"]]}}`)
	line := strings.Repeat("a", 4096)
	initial := lx.Initial()
	dst := make([]Token, 0, 8)
	b.SetBytes(int64(len(line)))
	b.ReportAllocs()
	for b.Loop() {
		dst, _, _ = lx.TokenizeLineInfo(line, initial, dst[:0])
	}
}
