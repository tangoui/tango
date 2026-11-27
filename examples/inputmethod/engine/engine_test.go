package engine

import (
	"slices"
	"testing"
)

func typeString(c *Composer, s string) Result {
	var r Result
	for _, ch := range s {
		r = c.Feed(Key{Rune: ch})
	}
	return r
}

func TestLettersComposeAndSpaceCommitsCandidate(t *testing.T) {
	c := New()
	r := typeString(c, "nihao")
	if !r.Handled || r.Preedit != "nihao" || r.Commit != "" {
		t.Fatalf("typing: %+v", r)
	}
	if !slices.Contains(r.Candidates, "你好") {
		t.Fatalf("candidates %v lack 你好", r.Candidates)
	}
	r = c.Feed(Key{Special: Space})
	if !r.Handled || r.Commit != "你好" || r.Preedit != "" {
		t.Fatalf("space: %+v", r)
	}
}

func TestSpaceWithoutCandidateCommitsLetters(t *testing.T) {
	c := New()
	typeString(c, "xyz")
	if r := c.Feed(Key{Special: Space}); r.Commit != "xyz" {
		t.Fatalf("got %+v", r)
	}
}

func TestEnterCommitsRawLetters(t *testing.T) {
	c := New()
	typeString(c, "nihao")
	if r := c.Feed(Key{Special: Enter}); !r.Handled || r.Commit != "nihao" || r.Preedit != "" {
		t.Fatalf("got %+v", r)
	}
}

func TestEscapeCancels(t *testing.T) {
	c := New()
	typeString(c, "ni")
	if r := c.Feed(Key{Special: Escape}); !r.Handled || r.Commit != "" || r.Preedit != "" {
		t.Fatalf("got %+v", r)
	}
	if r := c.Feed(Key{Special: Escape}); r.Handled {
		t.Fatalf("escape on empty buffer must pass through: %+v", r)
	}
}

func TestBackspace(t *testing.T) {
	c := New()
	typeString(c, "nih")
	if r := c.Feed(Key{Special: Backspace}); !r.Handled || r.Preedit != "ni" {
		t.Fatalf("got %+v", r)
	}
	c.Feed(Key{Special: Backspace})
	if r := c.Feed(Key{Special: Backspace}); !r.Handled || r.Preedit != "" {
		t.Fatalf("deleting last letter: %+v", r)
	}
	if r := c.Feed(Key{Special: Backspace}); r.Handled {
		t.Fatalf("backspace on empty buffer must pass through: %+v", r)
	}
}

func TestEmptyBufferPassesThrough(t *testing.T) {
	c := New()
	for _, k := range []Key{{Special: Space}, {Special: Enter}, {Rune: '1'}, {Rune: 'A'}, {Rune: ','}} {
		if r := c.Feed(k); r.Handled {
			t.Fatalf("%+v should pass through, got %+v", k, r)
		}
	}
}

func TestPunctuationCommitsCandidateThenItself(t *testing.T) {
	c := New()
	typeString(c, "nihao")
	if r := c.Feed(Key{Rune: ','}); !r.Handled || r.Commit != "你好，" || r.Preedit != "" {
		t.Fatalf("got %+v", r)
	}
}

func TestFlush(t *testing.T) {
	c := New()
	typeString(c, "shijie")
	if got := c.Flush(); got != "shijie" {
		t.Fatalf("flush = %q", got)
	}
	if c.Composing() {
		t.Fatal("still composing after flush")
	}
	if got := c.Flush(); got != "" {
		t.Fatalf("second flush = %q", got)
	}
}
