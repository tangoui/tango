package monarch

import (
	"strings"
	"testing"
)

func mygoCompareGoChunk() (string, []string) {
	src := strings.Repeat("func (s *server) handle(w http.ResponseWriter, r *http.Request) error {\n\tif err := s.check(r.Context(), \"x\\n\", 0x1f); err != nil { // why\n\t\treturn fmt.Errorf(`bad: %w`, err)\n\t}\n\treturn nil\n}\n", 200)
	return src, strings.Split(src, "\n")
}

func BenchmarkMyGoCompareGoChunk(b *testing.B) {
	src, lines := mygoCompareGoChunk()
	lx, err := NewRegistry().Lexer("go")
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for range b.N {
		st := lx.Initial()
		var toks []Token
		for _, line := range lines {
			toks, st = lx.TokenizeLine(line, st, toks[:0])
		}
	}
}

func TestMyGoCompareGoChunkAllocBudget(t *testing.T) {
	_, lines := mygoCompareGoChunk()
	lx, err := NewRegistry().Lexer("go")
	if err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(3, func() {
		st := lx.Initial()
		var toks []Token
		for _, line := range lines {
			toks, st = lx.TokenizeLine(line, st, toks[:0])
		}
	})
	if allocs >= 60_000 {
		t.Fatalf("Go chunk tokenizer allocations = %.0f, want < 60000", allocs)
	}
}
