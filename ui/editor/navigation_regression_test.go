package editor

import (
	"encoding/json"
	"github.com/tangoui/tango/ui/editor/monarch"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
)

func TestMinimapPreparedBeforeLayout(t *testing.T) {
	goText(t)
	st := NewState("var x = 1\n", "a.go", theme.Base("vs"))
	defer st.Close()
	st.Highlighter().HighlightFor(st.hlKey, st.Snapshot(), 2, time.Second)
	d, vw := viewDriver(st, 640, 300, nil)
	old := shapeHook
	defer func() { shapeHook = old }()
	checked := false
	shapeHook = func(i int, _ string) {
		if !checked {
			checked = true
			if (*vw).vs.mini.cache[0] == nil {
				t.Error("minimap token work runs after document shaping/drawing, spending the token budget on unrelated paint work")
			}
		}
	}
	d.Frame()
	if !checked {
		t.Fatal("no document layout observed")
	}
}

func TestMinimapWrapCaretFits(t *testing.T) {
	line := strings.Repeat("ab\t你好世界😀x👩‍💻é̂ word ", 12)
	st, ed := wrapEditor(t, "first\n"+line+"\nlast line\n", 300, 400)
	defer st.Close()
	st.MoveCaret(st.Snapshot().LineStart(1)+384, false)
	ed.Frame()
	r := rowRect(t, ed, st, st.Primary().Head, false)
	w := *ed.vw
	if r.Right() > w.vs.text.Right() {
		t.Fatalf("caret %v outside text %v; minimap %v", r, w.vs.text, w.vs.minimapRect)
	}
}

func TestStickyProviderCached(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("text\n", 100), "a.txt", theme.Base("vs"))
	defer st.Close()
	p := &testProvider{rs: []FoldingRange{{Start: 0, End: 90}, {Start: 2, End: 60}}}
	d, vw := viewDriver(st, 640, 300, func(w *ViewW) { w.FoldingProvider(p) })
	d.Frame()
	st.Viewport.TopLine = 10
	d.Frame()
	calls := p.calls
	for range 5 {
		d.Frame()
	}
	if p.calls != calls || len((*vw).vs.stickyLines) != 2 {
		t.Fatalf("provider calls %d -> %d headers %v", calls, p.calls, (*vw).vs.stickyLines)
	}
}

func TestMinimapLanguageAndTokenState(t *testing.T) {
	goText(t)
	st := NewState("/*\nvar x = 1\n*/\nvar y = 2\n", "a.go", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	for range 5 {
		d.Frame()
	}
	before := (*vw).vs.mini.rebuilt
	st.Model.SetLanguage("plaintext")
	d.Frame()
	if (*vw).vs.mini.rebuilt == before {
		t.Fatal("language did not invalidate minimap")
	}
	st.Model.SetLanguage("go")
	st.Highlighter().Highlight(st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	d.Frame()
	ln := (*vw).vs.mini.cache[1]
	if ln == nil || len(miniRowBlocks(ln, st)) == 0 || miniRowBlocks(ln, st)[0].color != st.Theme.Style("comment.go").Foreground {
		t.Fatal("start state recolor missing")
	}
}

func TestMinimapLexerReplacement(t *testing.T) {
	goText(t)
	reg := monarch.NewRegistry()
	st := NewStateWithRegistry(strings.Repeat("var x = 1\n", 200), "a.go", theme.Base("vs"), reg)
	defer st.Close()
	hl := st.Highlighter()
	hl.TokenWindow = 20
	hl.Highlight(st.Snapshot(), 201, time.Second)
	d, vw := viewDriver(st, 640, 300, nil)
	for range 20 {
		d.Frame()
	}
	before := (*vw).vs.mini.cache[100]
	if before == nil {
		t.Fatal("minimap line not cached")
	}
	old := miniRowBlocks(before, st)[0].color
	if err := reg.Add("go", json.RawMessage(`{"tokenizer":{"root":[[".+","string"]]}}`)); err != nil {
		t.Fatal(err)
	}
	lx, err := reg.Lexer("go")
	if err != nil {
		t.Fatal(err)
	}
	hl.Reset(lx)
	hl.Highlight(st.Snapshot(), 201, time.Second)
	for range 20 {
		d.Frame()
	}
	after := (*vw).vs.mini.cache[100]
	if after == nil || miniRowBlocks(after, st)[0].color == old {
		t.Fatal("offscreen minimap retained old lexer colors")
	}
	lo, hi := hl.WindowRangeFor(st.hlKey)
	if lo > 20 || hi > 70 {
		t.Fatalf("minimap thrashed visible token window: %d..%d", lo, hi)
	}
}

func TestNavigationLargeFile(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("x\n", LargeFileLines+1), "a.go", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	if !(*vw).vs.minimapRect.Empty() || !(*vw).vs.stickyRect.Empty() {
		t.Fatal("large-file navigation did not degrade")
	}
	if st.Highlighter() == nil {
		t.Fatal("large-file highlighter disabled")
	}
}

// miniRowBlocks returns the token-colored blocks of a miniature row: what
// the block minimap draws for it, made from the row's text and tokens
// (rows made while characters are on keep none).
func miniRowBlocks(ln *miniLine, st *State) []miniBlock {
	if ln.blocksDone {
		return ln.blocks
	}
	return miniBlocksAt(ln.text, ln.base, ln.tokens, st.Theme, max(1, st.tabSize()))
}
