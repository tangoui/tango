package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/highlight"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
)

// Removing the complete-cache fast path must restore the lexer's allocations.
// The .perf counting overlay additionally verifies actual Lexer invocation counts.
func TestBracketTokensCompleteCacheAllocs(t *testing.T) {
	st := NewState("s := \"}\" // ]\n", "a.go", theme.Base("vs"))
	defer st.Close()
	snap, h := st.Snapshot(), st.Highlighter()
	if !h.Highlight(snap, snap.LineCount(), time.Second) {
		t.Fatal("setup not final")
	}
	src, text := modelBrackets{st.Model}, snap.Line(0)
	want := h.TokenizeSpan(snap, 0, text, nil)
	dst := make([]monarch.Token, 0, len(want))
	allocs := testing.AllocsPerRun(20, func() { dst = src.tokens(snap, 0, text, dst) })
	if !slices.Equal(dst, want) {
		t.Fatalf("cached classifications changed: got %v want %v", dst, want)
	}
	if allocs != 0 {
		t.Fatalf("complete cached line allocated %.0f times per query, want 0 (no repeated lexing)", allocs)
	}
}

// A miss must preserve the old independent TokenizeSpan behavior, including its
// initial-state hint for unknown snapshots and its stored-state hint when dirty.
func TestBracketTokensCompleteCacheFallback(t *testing.T) {
	for _, name := range []string{"unknown", "dirty", "old-snapshot", "other-buffer", "reset", "evicted"} {
		t.Run(name, func(t *testing.T) {
			st := NewState("/* open\n} ]\n*/ f()\n", "a.go", theme.Base("vs"))
			defer st.Close()
			snap, h := st.Snapshot(), st.Highlighter()
			if name != "unknown" && !h.Highlight(snap, snap.LineCount(), time.Second) {
				t.Fatal("setup not final")
			}
			switch name {
			case "dirty", "old-snapshot":
				st.MoveCaret(0, false)
				st.Insert("//")
				if name == "dirty" {
					snap = st.Snapshot()
				}
			case "other-buffer":
				snap = buffer.NewSnapshot("x := 1\n} ]\n// f()\n")
			case "reset":
				h.Reset(h.LexerIdentity())
			case "evicted":
				h.DropView(highlight.DefaultView)
			}
			text := snap.Line(1)
			want := h.TokenizeSpan(snap, 1, text, nil)
			got := (modelBrackets{st.Model}).tokens(snap, 1, text, []monarch.Token{{Type: "old-dst"}})
			if !slices.Equal(got, want) {
				t.Fatalf("fallback differs from independent lexing: got %v want %v", got, want)
			}
		})
	}
}

// Compare every token and bracket query with a separately rebuilt Model whose
// token windows are removed, forcing the original independent lexing path.
func TestBracketTokensCompleteCacheEdits(t *testing.T) {
	st := NewState("func f() {\n s := \"} ]\" // (\n /* } */\n x := g(1)\n}\n", "a.go", theme.Base("vs"))
	defer st.Close()
	rng := rand.New(rand.NewPCG(91, 37))
	_, _, degradedBefore := monarch.Stats()
	check := func(step int) {
		t.Helper()
		snap := st.Snapshot()
		if h := st.Highlighter(); h != nil && !h.Highlight(snap, snap.LineCount(), time.Second) {
			t.Fatalf("step %d: incremental setup not final", step)
		}
		ref := NewState(snap.String(), "a.go", theme.Base("vs"))
		defer ref.Close()
		ref.Model.SetLanguage(st.Model.Language())
		rs := ref.Snapshot()
		if h := ref.Highlighter(); h != nil {
			if !h.Highlight(rs, rs.LineCount(), time.Second) {
				t.Fatalf("step %d: reference setup not final", step)
			}
			h.DropView(highlight.DefaultView)
		}
		for line := 0; line < snap.LineCount(); line++ {
			text := snap.Line(line)
			got := (modelBrackets{st.Model}).tokens(snap, line, text, nil)
			var want []monarch.Token
			if h := ref.Highlighter(); h != nil {
				want = h.TokenizeSpan(rs, line, text, nil)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("step %d language %s line %d tokens differ:\ngot %v\nwant %v", step, st.Model.Language(), line, got, want)
			}
		}
		deadline := time.Now().Add(time.Hour)
		for off := 0; off <= snap.Len(); off++ {
			got, ok := st.matchBracket(off, deadline)
			want, rok := ref.matchBracket(off, deadline)
			if got != want || ok != rok {
				t.Fatalf("step %d off %d: bracket %v/%v want %v/%v", step, off, got, ok, want, rok)
			}
			got, ok = st.enclosingBrackets(off, deadline)
			want, rok = ref.enclosingBrackets(off, deadline)
			if got != want || ok != rok {
				t.Fatalf("step %d off %d: enclosing %v/%v want %v/%v", step, off, got, ok, want, rok)
			}
		}
	}
	check(0)
	// These are literal false brackets inside a string and a comment, not
	// expectations obtained from the implementation under test.
	for _, off := range []int{strings.Index(st.Snapshot().String(), "} ]"), strings.Index(st.Snapshot().String(), "} */")} {
		if p, ok := st.matchBracket(off, time.Now().Add(time.Second)); ok {
			t.Fatalf("string/comment byte %d incorrectly matched %v", off, p)
		}
	}
	fragments := []string{"x", "\"", "/*", "*/", "//", "(", ")", "[", "]", "{", "}", "\n"}
	for step := 1; step <= 48; step++ {
		snap := st.Snapshot()
		start := rng.IntN(snap.Len() + 1)
		end := min(snap.Len(), start+rng.IntN(3))
		if err := st.edit([]buffer.Edit{{Start: start, End: end, Text: fragments[rng.IntN(len(fragments))]}}, kindOther, []Selection{{}}, false); err != nil {
			t.Fatal(err)
		}
		check(step)
		if step%8 == 0 {
			if !st.Undo() {
				t.Fatal("edit not undoable")
			}
			check(-step)
			if !st.Redo() {
				t.Fatal("edit not redoable")
			}
			check(step)
		}
		if step%12 == 0 {
			for _, language := range []string{"typescript", PlainText, "go"} {
				st.Model.SetLanguage(language)
				check(step)
			}
		}
	}
	if _, _, degradedAfter := monarch.Stats(); degradedAfter != degradedBefore {
		t.Fatal("short-line oracle or candidate degraded; comparison would not prove complete tokens")
	}
}
