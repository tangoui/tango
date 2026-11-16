//go:build !race

package monarch

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestLongTokenTypesDoNotMultiplyCompileMemory(t *testing.T) {
	postfix := "." + strings.Repeat("p", 512<<10)
	rules := make([][]string, 128)
	for i := range rules {
		rules[i] = []string{"a", "word"}
	}
	data, err := json.Marshal(map[string]any{
		"tokenPostfix": postfix,
		"tokenizer":    map[string]any{"root": rules},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := r.Add("long-types", data); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	lx, err := r.Lexer("long-types")
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Fatalf("compiling a 512 KiB postfix with 128 rules allocated %d bytes, budget 32 MiB", allocated)
	}
	got, _ := lx.TokenizeLine("a", lx.Initial(), nil)
	if len(got) != 1 || got[0].Type != "word"+postfix {
		t.Fatal("long token type changed")
	}
}

func TestStaticTokenTypeAllocationBudget(t *testing.T) {
	lx := addLexer(t, "static-alloc", `{"tokenPostfix":".static","tokenizer":{"root":[["a","word"]]}}`)
	assertLineAllocationBudget(t, lx, strings.Repeat("a", 100), "word.static", 125)
}

func TestShortASCIIBacktrackAllocationBudget(t *testing.T) {
	lx := addLexer(t, "rune-alloc", `{"tokenizer":{"root":[["(?=z)z",""],["a",""]]}}`)
	assertLineAllocationBudget(t, lx, strings.Repeat("a", 100), "", 125)
}

func TestMatchFrameAllocationBudget(t *testing.T) {
	lx := addLexer(t, "frame-alloc", `{"tokenizer":{"root":[["a",""]]}}`)
	assertLineAllocationBudget(t, lx, "a", "", 3)
}

func assertLineAllocationBudget(t *testing.T, lx *Lexer, line, wantType string, max float64) {
	t.Helper()
	dst := make([]Token, 0, 1)
	allocs := testing.AllocsPerRun(100, func() {
		got, st, info := lx.TokenizeLineInfo(line, lx.Initial(), dst[:0])
		if info.Degraded || st != lx.Initial() || len(got) != 1 || got[0] != (Token{Type: wantType}) {
			panic("incorrect token output or state")
		}
	})
	if allocs > max {
		t.Fatalf("line allocates %.0f times, budget %.0f", allocs, max)
	}
}
