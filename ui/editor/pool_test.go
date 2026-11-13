package editor

import "testing"

func TestPoolOpenShares(t *testing.T) {
	p := NewModelPool(DefaultRegistry())
	var created, disposed []*Model
	p.OnCreate(func(m *Model) { created = append(created, m) })
	p.OnWillDispose(func(m *Model) {
		if p.Get(m.URI()) != m {
			t.Error("model left the pool before OnWillDispose")
		}
		disposed = append(disposed, m)
	})

	const uri = "file:///src/main.go"
	m1 := p.Open(uri, "package main\n")
	m2 := p.Open(uri, "ignored")
	if m1 != m2 || p.Get(uri) != m1 {
		t.Fatal("two Opens of one URI returned different models")
	}
	if got := m1.Snapshot().String(); got != "package main\n" {
		t.Fatalf("text %q: the second Open's text was not ignored", got)
	}
	if m1.URI() != uri || m1.Language() != "go" || m1.Highlighter() == nil || m1.Conf() == nil {
		t.Fatalf("uri %q language %q", m1.URI(), m1.Language())
	}
	if len(created) != 1 || created[0] != m1 {
		t.Fatalf("OnCreate called %d times", len(created))
	}

	p.Release(m1)
	if len(disposed) != 0 || p.Get(uri) != m1 {
		t.Fatal("disposed with a reference left")
	}
	p.Release(m2)
	if len(disposed) != 1 || disposed[0] != m1 || p.Get(uri) != nil {
		t.Fatalf("after the last Release: %d disposals, Get %p", len(disposed), p.Get(uri))
	}
	p.Release(m1) // no longer held: nothing
	if len(disposed) != 1 {
		t.Fatal("releasing a disposed model disposed it again")
	}

	m3 := p.Open(uri, "new")
	if m3 == m1 || m3.Snapshot().String() != "new" || len(created) != 2 {
		t.Fatal("Open after disposal did not create a new model")
	}
	if p.Get("file:///other.go") != nil {
		t.Fatal("Get of an unopened URI")
	}
}

func TestPoolLanguageEvents(t *testing.T) {
	p := NewModelPool(DefaultRegistry())
	type ev struct {
		m   *Model
		old string
	}
	var evs []ev
	cancel := p.OnLanguageChange(func(m *Model, old string) { evs = append(evs, ev{m, old}) })
	m := p.Open("notes.txt", "let x = 1")
	if m.Language() != PlainText || m.Highlighter() != nil || m.Conf() != nil {
		t.Fatalf("language %q", m.Language())
	}
	var changes int
	m.Subscribe(func(ch ModelChange) {
		if ch.Language && ch.Old.Version() == ch.New.Version() {
			changes++
		}
	})

	m.SetLanguage("typescript")
	hl := m.Highlighter()
	if m.Language() != "typescript" || hl == nil || m.Conf() == nil {
		t.Fatalf("after SetLanguage: %q", m.Language())
	}
	m.SetLanguage("typescript") // unchanged: no event
	m.SetLanguage("javascript")
	if m.Highlighter() != hl {
		t.Fatal("SetLanguage replaced the highlighter instead of resetting it")
	}
	m.SetLanguage("no-such-language")
	if m.Language() != PlainText || m.Highlighter() != nil || m.Conf() != nil {
		t.Fatalf("unknown language: %q", m.Language())
	}
	want := []ev{{m, PlainText}, {m, "typescript"}, {m, "javascript"}}
	if len(evs) != len(want) || changes != len(want) {
		t.Fatalf("events %v (%d changes), want %v", evs, changes, want)
	}
	for i := range want {
		if evs[i] != want[i] {
			t.Fatalf("event %d = %v, want %v", i, evs[i], want[i])
		}
	}

	cancel()
	m.SetLanguage("go")
	if len(evs) != len(want) {
		t.Fatal("cancelled listener still called")
	}
	// A model outside a pool has no pool events but still notifies.
	solo := NewModel("a.txt", "", DefaultRegistry())
	solo.SetLanguage("go")
	if solo.Language() != "go" || solo.Highlighter() == nil {
		t.Fatal("SetLanguage on a model outside a pool")
	}
}
