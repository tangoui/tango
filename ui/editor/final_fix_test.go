package editor

import (
	"encoding/json"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/regex"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

func TestFinalQueuedAfterCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name        string
		edits       []buffer.Edit
		after, want []Selection
		text        string
	}{
		{"caret", []buffer.Edit{{Start: 0, End: 0, Text: "YYY"}}, []Selection{{Anchor: 3, Head: 3}}, []Selection{{Anchor: 3, Head: 3}}, "YYYabXcd!"},
		{"replacement reversed", []buffer.Edit{{Start: 0, End: 1, Text: "YYYY"}}, []Selection{{Anchor: 3, Head: 1}}, []Selection{{Anchor: 3, Head: 1}}, "YYYYbXcd!"},
		{"multicursor", []buffer.Edit{{Start: 0, End: 0, Text: "YYY"}, {Start: 3, End: 4, Text: "ZZ"}}, []Selection{{Anchor: 2, Head: 1}, {Anchor: 7, Head: 8}}, []Selection{{Anchor: 2, Head: 1}, {Anchor: 8, Head: 9}}, "YYYabXcZZ!"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel("a.txt", "abcd", nil)
			a, b, c := NewView(m, theme.Base("vs")), NewView(m, theme.Base("vs")), NewView(m, theme.Base("vs"))
			once := false
			m.Subscribe(func(ch ModelChange) {
				if once {
					return
				}
				once = true
				if err := b.Edit([]buffer.Edit{{Start: 2, End: 2, Text: "X"}}, Selection{Anchor: 3, Head: 3}); err != nil {
					t.Fatal(err)
				}
				if err := c.edit(tc.edits, kindOther, tc.after, true); err != nil {
					t.Fatal(err)
				}
			})
			if err := a.Edit([]buffer.Edit{{Start: 4, End: 4, Text: "!"}}, Selection{Anchor: 5, Head: 5}); err != nil {
				t.Fatal(err)
			}
			if got := m.Snapshot().String(); got != tc.text {
				t.Fatalf("text %q want %q", got, tc.text)
			}
			if !slices.Equal(c.Sels, tc.want) {
				t.Fatalf("selections %v want %v", c.Sels, tc.want)
			}
			if !c.Undo() || !c.Redo() || !slices.Equal(c.Sels, tc.want) || m.Snapshot().String() != tc.text {
				t.Fatalf("redo %q %v", m.Snapshot().String(), c.Sels)
			}
		})
	}
}

func TestFinalAutoCloseReentrantMetadata(t *testing.T) {
	for _, del := range []bool{false, true} {
		t.Run(fmt.Sprint(del), func(t *testing.T) {
			m := NewModel("a.go", "", DefaultRegistry())
			a, b := NewView(m, theme.Base("vs")), NewView(m, theme.Base("vs"))
			once := false
			m.Subscribe(func(ch ModelChange) {
				if once {
					return
				}
				once = true
				b.Edit([]buffer.Edit{{Start: 0, End: 0, Text: "x"}}, Selection{Anchor: 1, Head: 1})
			})
			a.Type("(")
			if marked(a) != "x(|)" {
				t.Fatalf("after open %q", marked(a))
			}
			if del {
				a.Execute("deleteLeft", nil)
				if marked(a) != "x|" {
					t.Fatalf("delete %q", marked(a))
				}
			} else {
				a.Type(")")
				if marked(a) != "x()|" {
					t.Fatalf("overtype %q", marked(a))
				}
			}
		})
	}
}

func TestFinalLanguageNotificationOrder(t *testing.T) {
	p := NewModelPool(DefaultRegistry())
	m := p.Open("a.txt", "")
	st := NewView(m, theme.Base("vs"))
	var got []string
	m.Subscribe(func(ch ModelChange) {
		if ch.Language {
			if ch.NewLanguage != m.Language() || ch.OldLanguage == ch.NewLanguage || ch.New.Version() != m.Snapshot().Version() {
				t.Fatalf("inconsistent language event: %+v current=%s", ch, m.Language())
			}
			got = append(got, "model:"+m.Language())
			if m.Language() == "go" {
				m.SetLanguage("python")
			}
		} else {
			got = append(got, "edit:"+m.Language())
		}
	})
	p.OnLanguageChange(func(m *Model, old string) {
		got = append(got, "pool:"+old+">"+m.Language())
		if m.Language() == "go" {
			st.Insert("x")
		}
	})
	m.SetLanguage("go")
	want := []string{"model:go", "pool:plaintext>go", "model:python", "pool:go>python", "edit:python"}
	if !slices.Equal(got, want) || m.Snapshot().String() != "x" {
		t.Fatalf("events %v text %q", got, m.Snapshot().String())
	}
}

func TestFinalPickerCurrentReadOnly(t *testing.T) {
	goText(t)
	for _, persistent := range []bool{false, true} {
		t.Run(fmt.Sprint(persistent), func(t *testing.T) {
			const doc = "color: #ff0000;"
			st := NewState(doc, "a.css", theme.Base("vs"))
			h := newHintHost(t, st, geom.Size{W: 480, H: 300}, 1)
			ro := false
			h.r = ui.NewRunner(func(c *ui.Context) { h.vw = View(c, st).ReadOnly(ro || st.Options.ReadOnly) }, ui.RunnerOptions{Renderer: h.ren, NoAnimations: true, Clock: func() time.Time { return h.now }})
			h.r.Attach(h.w)
			h.focusAt(0)
			sw := h.vw.vs.swatches[0].r
			h.click(geom.Pt(sw.X+sw.W/2, sw.Y+sw.H/2), 0)
			h.frame()
			h.frame()
			pk := h.vw.vs.picker
			if pk == nil {
				t.Fatal("picker not open")
			}
			pk.col = color.NRGBA{0, 255, 0, 255}
			if persistent {
				st.Options.ReadOnly = true
			} else {
				ro = true
			}
			h.frame()
			if st.Snapshot().String() != doc || st.Undo() {
				t.Fatalf("read-only picker changed document or undo: %q", st.Snapshot().String())
			}
		})
	}
}

func TestFinalGrammarBracketRefresh(t *testing.T) {
	goText(t)
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprint(direct), func(t *testing.T) {
			reg := monarch.NewRegistry()
			m := NewModel("a.go", "()", reg)
			st := NewView(m, theme.Base("vs"))
			st.Theme = theme.Base("vs")
			h := newHintHost(t, st, geom.Size{W: 480, H: 300}, 1)
			h.focusAt(0)
			h.frame()
			h.frame()
			if len(h.vw.vs.brc.lines[0]) != 2 || len(h.vw.vs.match) == 0 {
				t.Fatalf("initial colors/match %v %v", h.vw.vs.brc.lines, h.vw.vs.match)
			}
			for _, g := range []string{`{"tokenizer":{"root":[[".*","comment"]]}}`, `{"tokenizer":{"root":[[".",""]]}}`} {
				if err := reg.Add("go", json.RawMessage(g)); err != nil {
					t.Fatal(err)
				}
				if direct {
					lx, err := reg.Lexer("go")
					if err != nil {
						t.Fatal(err)
					}
					m.Highlighter().Reset(lx)
				} else {
					m.SetLanguage("go")
				}
				h.frame()
				h.frame()
				h.frame()
				comment := strings.Contains(g, "comment")
				tokens, final := m.Highlighter().Tokens(m.Snapshot(), 0, nil)
				if !final || (tokenAt(tokens, 0) == "comment") != comment {
					t.Fatalf("stale tokens: comment=%v final=%v tokens=%v", comment, final, tokens)
				}
				n := len(h.vw.vs.brc.lines[0])
				matches := len(h.vw.vs.match)
				if comment && (n != 0 || matches != 0) || !comment && (n != 2 || matches == 0) {
					t.Fatalf("comment=%v colors=%v match=%v", comment, h.vw.vs.brc.lines, h.vw.vs.match)
				}
			}
		})
	}
}

func TestFinalClassificationWorkBound(t *testing.T) {
	reg := monarch.NewRegistry()
	m := NewModel("a.go", strings.Repeat("x\n", 10000), reg)
	s := NewView(m, theme.Base("vs"))
	if s.trimTrailingWhitespace() {
		t.Fatal("clean trim changed text")
	}
	lt := s.newLineTokens(m.Snapshot())
	lt.tokens(9999)
	if lt.next > syntaxCommandLines || !lt.failed {
		t.Fatalf("unbounded classification: %d lines failed=%v", lt.next, lt.failed)
	}
	// A cutoff must not misclassify blanks after a truncated string as code.
	long := "`" + strings.Repeat("x", monarch.MaxLineLength) + "  "
	m = NewModel("a.go", long, reg)
	s = NewView(m, theme.Base("vs"))
	if s.trimTrailingWhitespace() || m.Snapshot().String() != long {
		t.Fatal("trimmed unclassified long string")
	}
}

func TestFinalClassificationSlowGrammarStops(t *testing.T) {
	reg := monarch.NewRegistry()
	if err := reg.Add("ruby", json.RawMessage(`{"tokenizer":{"root":[["(a+)+(?=b)","string"],[".",""]]}}`)); err != nil {
		t.Fatal(err)
	}
	clean := strings.Repeat(strings.Repeat("a", 200)+"c\n", 4)
	m := NewModel("a.rb", clean, reg)
	s := NewView(m, theme.Base("vs"))
	_, _, before := regex.Stats()
	if s.trimTrailingWhitespace() {
		t.Fatal("clean trim changed text")
	}
	_, _, after := regex.Stats()
	if after != before {
		t.Fatal("clean trim forced pathological classification")
	}
	doc := strings.Repeat(strings.Repeat("a", 200)+"c  \n", 4)
	m = NewModel("a.rb", doc, reg)
	s = NewView(m, theme.Base("vs"))
	_, _, before = regex.Stats()
	if s.trimTrailingWhitespace() || m.Snapshot().String() != doc {
		t.Fatal("trim treated degraded tokens as definitive")
	}
	_, _, after = regex.Stats()
	if after-before > 1 {
		t.Fatalf("command repeated pathological attempt across lines: %d", after-before)
	}
	if s.reindentLines() || !s.SyntaxCommandIncomplete() {
		t.Fatal("reindent used unavailable classification or did not report incomplete")
	}
}

func TestFinalCommandTransactionLogsInterveningChanges(t *testing.T) {
	m := NewModel("a.txt", "abcd", nil)
	a, b := NewView(m, nil), NewView(m, nil)
	once := false
	m.Subscribe(func(ch ModelChange) {
		if once {
			return
		}
		once = true
		b.Edit([]buffer.Edit{{Start: 0, End: 0, Text: "Y"}}, Selection{Anchor: 1, Head: 1})
	})
	RegisterCommand(Command{ID: "test.final.multiEdit", Run: func(st *State, _ any) bool {
		st.Edit([]buffer.Edit{{Start: 4, End: 4, Text: "!"}}, Selection{Anchor: 5, Head: 5})
		st.Edit([]buffer.Edit{{Start: 0, End: 0, Text: "z"}}, Selection{Anchor: 1, Head: 1})
		return true
	}})
	a.Execute("test.final.multiEdit", nil)
	if m.Snapshot().String() != "zYabcd!" || b.Primary() != (Selection{Anchor: 2, Head: 2}) {
		t.Fatalf("source changes missing from queued rebase: %q %v", m.Snapshot().String(), b.Sels)
	}
}

func TestFinalQueuedDropObservable(t *testing.T) {
	m := NewModel("a.txt", "x", nil)
	a, b := NewView(m, nil), NewView(m, nil)
	once := false
	m.Subscribe(func(ch ModelChange) {
		if !once {
			once = true
			b.Edit([]buffer.Edit{{Start: 99, End: 100}}, Selection{})
		}
	})
	a.Insert("!")
	if m.DroppedEdits() != 1 {
		t.Fatalf("drops %d", m.DroppedEdits())
	}
}

func TestFinalClassificationIncompleteIsAtomicForReindent(t *testing.T) {
	reg := monarch.NewRegistry()
	doc := "def f\n" + strings.Repeat("x  \n", 200) + "end"
	m := NewModel("a.rb", doc, reg)
	s := NewView(m, nil)
	if s.reindentLines() || !s.SyntaxCommandIncomplete() || m.Snapshot().String() != doc || s.Undo() {
		t.Fatal("reindent applied a partially classified edit or failed to report it")
	}
	lt := s.newLineTokens(m.Snapshot())
	lt.deadline = time.Now().Add(-time.Second)
	lt.tokens(100)
	if !lt.failed || lt.next > 8 {
		t.Fatalf("expired command exceeded its small progress allowance: next=%d failed=%v", lt.next, lt.failed)
	}
}

func TestFinalQueuedTypingCapturesSelection(t *testing.T) {
	m := NewModel("a.txt", "ab", nil)
	a, b := NewView(m, nil), NewView(m, nil)
	once := false
	m.Subscribe(func(ch ModelChange) {
		if once {
			return
		}
		once = true
		b.SetSelections([]Selection{{Anchor: 0, Head: 0}})
		b.Type("X")
		b.SetSelections([]Selection{{Anchor: 2, Head: 2}})
		b.Execute("type", "Y")
	})
	a.Edit([]buffer.Edit{{Start: 2, End: 2, Text: "!"}}, Selection{Anchor: 3, Head: 3})
	if m.Snapshot().String() != "XabY!" || b.Primary() != (Selection{Anchor: 4, Head: 4}) {
		t.Fatalf("queued typing lost requested coordinates: %q %v", m.Snapshot().String(), b.Sels)
	}
}
