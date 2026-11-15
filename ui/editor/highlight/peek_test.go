package highlight

import (
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"runtime"
	"strings"
	"testing"
)

func TestPeekTokenRangeBounded(t *testing.T) {
	lx := grammarLexer(t, "preview", `{"tokenizer":{"root":[[".+","text"]]}}`)
	h := New(lx)
	snap := buffer.NewSnapshot(strings.Repeat("x", 1<<20))
	h.Highlight(snap, 1, 0)
	sl := h.st.slotFor(DefaultView, 0)
	sl.ok = true
	sl.start = lx.Initial()
	sl.toks = make([]monarch.Token, 1<<20)
	for i := range sl.toks {
		sl.toks[i] = monarch.Token{Start: i, ByteStart: i, Type: "text"}
	}
	lo, hi := h.WindowRange()
	before := h.tokenized.Load()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for range 10 {
		toks, ok := h.PeekTokenRange(snap, 0, 100, 108, nil)
		if !ok || len(toks) != 8 || toks[0].ByteStart != 100 || toks[7].ByteStart != 107 || toks[0].Start != 100 {
			t.Fatalf("range %v %v", ok, toks)
		}
	}
	runtime.ReadMemStats(&b)
	if b.TotalAlloc-a.TotalAlloc > 64<<10 {
		t.Fatalf("copied whole cached token list: %d bytes", b.TotalAlloc-a.TotalAlloc)
	}
	lo2, hi2 := h.WindowRange()
	if lo2 != lo || hi2 != hi || h.tokenized.Load() != before {
		t.Fatal("peek moved or tokenized window")
	}
}
