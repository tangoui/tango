package editor

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
)

func BenchmarkEditorInput(b *testing.B) {
	const line = "The quick brown fox jumps over the lazy dog. This deterministic input exercises text layout in Tango."
	payload := strings.Repeat("## Heading\n\n"+line+"\n\n", 1800)
	committed := input.TextInput{Text: payload}
	for _, tc := range []struct {
		name string
		edit func(*ViewW)
	}{
		{"CommittedText", func(w *ViewW) { w.applyEvents([]input.Event{committed}) }},
		{"Keystrokes", func(w *ViewW) {
			for _, ch := range payload {
				if ch == '\n' {
					w.applyEvents([]input.Event{input.KeyEvent{Key: input.KeyEnter, Down: true}})
				} else {
					w.applyEvents([]input.Event{input.TextInput{Text: string(ch)}})
				}
			}
		}},
		{"IME", func(w *ViewW) { w.applyEvents([]input.Event{imeCommit{committed}}) }},
		{"Paste", func(w *ViewW) {
			w.applyEvents([]input.Event{input.KeyEvent{Key: input.KeyV, Down: true, Mods: input.ModMeta}})
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			st := NewState("", "", nil)
			defer st.Close()
			ed := newEditDriver(st, 500, 300, pinMac)
			w := *ed.vw
			w.c.Window().WriteClipboard(payload)
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			var undo time.Duration
			for b.Loop() {
				tc.edit(w)
				b.StopTimer()
				if st.Snapshot().Len() != len(payload) {
					b.Fatal("input did not insert the full payload")
				}
				start := time.Now()
				for st.Undo() {
				}
				undo += time.Since(start)
				if st.Snapshot().Len() != 0 {
					b.Fatal("undo did not restore the empty buffer")
				}
				b.StartTimer()
			}
			b.ReportMetric(float64(undo.Nanoseconds())/float64(b.N), "undo-ns/op")
		})
	}
}

func TestCommittedTextPreservesObserverEdits(t *testing.T) {
	st := NewState("", "", nil)
	defer st.Close()
	stop := st.Model.Subscribe(func(ch ModelChange) {
		if ch.New.String() == "a" {
			st.Insert("!")
		}
	})
	defer stop()
	(&ViewW{st: st}).commitText("abc", false)
	if got := st.Snapshot().String(); got != "a!bc" {
		t.Fatalf("commit with observer = %q", got)
	}
}

func TestCommittedTextBatchesOrdinaryRuns(t *testing.T) {
	st := NewState("", "", nil)
	defer st.Close()
	w := &ViewW{st: st}
	const text = "ordinary text with 汉字 and e\u0301"
	before := st.Snapshot().Version()
	w.commitText(text, false)
	if got := st.Snapshot(); got.String() != text || got.Version() != before+1 {
		t.Fatalf("commit = %q at version %d, want one edit after %d", got.String(), got.Version(), before)
	}
}

func TestCommittedTextMatchesTypingAndHistory(t *testing.T) {
	for _, tc := range []struct {
		name, lang, original, text string
		carets                     []Selection
		indent                     AutoIndentStrategy
	}{
		{"plain", "", "  old\nlast", "prefix e\u0301\r\n\tx 👩‍💻\u0007bye\rwhatever", []Selection{sel(2, 5)}, AutoIndentFull},
		{"pairs", "a.go", "", "x:= (word and more) + \"quoted text\"\n", []Selection{sel(0, 0)}, AutoIndentKeep},
		{"indent", "a.go", "func main() {", "\nvalue := (a)\n}", []Selection{sel(13, 13)}, AutoIndentFull},
		{"surround", "a.go", "wrap me", "( plus more)", []Selection{sel(0, 7)}, AutoIndentKeep},
		{"multiple", "", "ab cd", "xx and yy\n", []Selection{sel(1, 1), sel(4, 4)}, AutoIndentFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			batched := NewState(tc.original, tc.lang, nil)
			defer batched.Close()
			typed := NewState(tc.original, tc.lang, nil)
			defer typed.Close()
			for _, st := range []*State{batched, typed} {
				st.Options.AutoIndent = tc.indent
				st.SetSelections(tc.carets)
			}
			check := func() {
				t.Helper()
				if got, want := batched.Snapshot().String(), typed.Snapshot().String(); got != want {
					t.Fatalf("committed %q, typed %q", got, want)
				}
				if !slices.Equal(batched.Sels, typed.Sels) {
					t.Fatalf("committed selections %v, typed %v", batched.Sels, typed.Sels)
				}
			}
			(&ViewW{st: batched}).commitText(tc.text, false)
			typed.Type(tc.text)
			check()
			for _, redo := range []bool{false, true} {
				for {
					got, want := batched.history(redo), typed.history(redo)
					if got != want {
						t.Fatalf("history(%v) = %v, want %v", redo, got, want)
					}
					if !got {
						break
					}
					check()
				}
			}
		})
	}
}
