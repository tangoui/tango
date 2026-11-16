//go:build !race

package monarch

import (
	"slices"
	"strings"
	"testing"
)

// A long line matched entirely by RE2 has no need for regexp2's rune/index
// buffers. Reintroducing eager newLineRunes makes long allocate more than
// short, even with identical token/state results.
//
// Exact counts are tested without -race: sync.Pool.Put randomly discards 1/4
// of objects in race builds, including regexp's pooled work buffers. Their
// length-dependent allocations then dwarf our three eliminated allocations.
// The paired long/short token behavior is tested under both builds below.
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

// TestBacktrackingUnicodeReusesRuneBuffers checks that the Lexer's numeric
// rune/index scratch is pooled across adjacent tokenizations. The exact
// assertion is !race-only because sync.Pool may discard entries in race builds.
func TestBacktrackingUnicodeReusesRuneBuffers(t *testing.T) {
	lx := runePerfLexer(t, `{"tokenizer":{"root":[["(?=世)世","special"],[".","text"]]}}`)
	line := strings.Repeat("a", 40) + "世" + strings.Repeat("b", 40)
	initial := lx.Initial()
	dst := make([]Token, 0, 8)
	allocs := testing.AllocsPerRun(30, func() {
		dst, _, _ = lx.TokenizeLineInfo(line, initial, dst[:0])
	})
	if allocs > 7 {
		t.Fatalf("Unicode backtracking tokenization allocates %.0f times; want <=7 after scratch pooling", allocs)
	}
}
