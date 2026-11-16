package monarch

import (
	"slices"
	"strings"
	"testing"
)

// Preserve both lazy long-line windows and the short ASCII cache when a
// backtracking rule is reached only after a long RE2 prefix.
func TestMergedRuneWindowBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, grammar, tail string
	}{
		{"ascii", `{"tokenPostfix":"","tokenizer":{"root":[["a+","prefix"],["x(?=!)!","tail"]]}}`, "x!"},
		{"unicode", `{"tokenPostfix":"","tokenizer":{"root":[["a+","prefix"],["(?=世)世😀","tail"]]}}`, "世😀"},
		{"capture", `{"tokenPostfix":"","tokenizer":{"root":[["a+","prefix"],["((?=世)世😀)",["tail"]]]}}`, "世😀"},
		{"invalid", `{"tokenPostfix":"","tokenizer":{"root":[["a+","prefix"],["(?=.)..","tail"]]}}`, "\xff!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lx := runePerfLexer(t, tc.grammar)
			for _, size := range []int{256, 257, 1024} {
				prefix := size - len(tc.tail)
				line := strings.Repeat("a", prefix) + tc.tail
				tokens, state, info := lx.TokenizeLineInfo(line, lx.Initial(), nil)
				want := []Token{
					{Type: "prefix"},
					{Start: prefix, ByteStart: prefix, Type: "tail"},
				}
				if !slices.Equal(tokens, want) || state != lx.Initial() || info.Degraded {
					t.Fatalf("%d bytes: tokens=%v state=%v info=%+v; want %v and initial state", size, tokens, state, info, want)
				}
			}
		})
	}
}
