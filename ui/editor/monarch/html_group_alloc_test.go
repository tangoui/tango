//go:build !race

package monarch

import (
	"strings"
	"testing"
)

func TestSmallGroupActionAllocationBudget(t *testing.T) {
	for _, tc := range []struct{ pattern, actions, chunk string }{
		{`(a)(b)`, `["word","word"]`, "ab"},
		{`(a)(b)(c)(d)`, `["word","word","word","word"]`, "abcd"},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			lx := addLexer(t, "group-alloc", `{"tokenPostfix":"","tokenizer":{"root":[["`+tc.pattern+`",`+tc.actions+`]]}}`)
			assertLineAllocationBudget(t, lx, strings.Repeat(tc.chunk, 100), "word", 225)
		})
	}
}

func TestFallbackCaptureAllocationBudget(t *testing.T) {
	lx := addLexer(t, "fallback-alloc", `{"defaultToken":"fallback","tokenPostfix":"","tokenizer":{"root":[["z","z"]]}}`)
	for _, line := range []string{strings.Repeat("a", 100), strings.Repeat("é😀\xff", 40)} {
		assertLineAllocationBudget(t, lx, line, "fallback", 3)
	}
}

func TestEmptyFallbackCaptureAllocationBudget(t *testing.T) {
	lx := addLexer(t, "empty-alloc", `{"defaultToken":"fallback","tokenPostfix":"","tokenizer":{"root":[["z","z"]]}}`)
	prefix := Token{Start: 7, ByteStart: 9, Type: "prefix"}
	dst := []Token{prefix}
	allocs := testing.AllocsPerRun(100, func() {
		got, state, info := lx.TokenizeLineInfo("", lx.Initial(), dst)
		if info.Degraded || state != lx.Initial() || len(got) != 1 || got[0] != prefix {
			panic("empty line changed prefix, tokens or state")
		}
	})
	if allocs > 2 {
		t.Fatalf("empty line allocates %.0f times, budget 2", allocs)
	}
}
