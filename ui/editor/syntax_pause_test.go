package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/monarch"
)

// A scheduler pause before the first few lines must not turn a short
// syntax-dependent command into a no-op. The expired deadline makes the
// pause deterministic, without sleeps or a busy-loop load generator.
func TestSyntaxClassificationSurvivesPauseBeforeShortInput(t *testing.T) {
	reg := monarch.NewRegistry()
	if err := reg.Add("ruby", []byte(`{"tokenizer":{"root":[["#.*","comment"],["[a-z]+",""]]}}`)); err != nil {
		t.Fatal(err)
	}
	s := NewView(NewModel("a.rb", "# comment\ndef a\nb\nend", reg), nil)
	lt := s.newLineTokens(s.Snapshot())
	lt.deadline = time.Now().Add(-time.Hour)
	if toks := lt.tokens(0); lt.failed || tokenAt(toks, 0) != "comment" {
		t.Fatalf("short input was abandoned after a pause: failed=%v tokens=%v", lt.failed, toks)
	}
	lt.tokens(3)
	if lt.failed || lt.next != 4 {
		t.Fatalf("short command incomplete: next=%d failed=%v", lt.next, lt.failed)
	}
}

func TestSyntaxClassificationPauseStillBoundsWork(t *testing.T) {
	reg := monarch.NewRegistry()
	if err := reg.Add("ruby", []byte(`{"tokenizer":{"root":[[".*",""]]}}`)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{strings.Repeat("x\n", 1000), strings.Repeat(strings.Repeat("x", 1024)+"\n", 20)} {
		s := NewView(NewModel("a.rb", text, reg), nil)
		lt := s.newLineTokens(s.Snapshot())
		lt.deadline = time.Now().Add(-time.Hour)
		lt.tokens(19)
		if !lt.failed || lt.next == 0 || lt.next > 8 {
			t.Fatalf("unbounded or no progress after pause: next=%d failed=%v", lt.next, lt.failed)
		}
	}
}
