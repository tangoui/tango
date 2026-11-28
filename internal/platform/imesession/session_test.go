package imesession

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

type testSink struct {
	platform.InputMethodSink
	modes []string
}

type candidateSink struct {
	platform.InputMethodSink
	picks []int
}

func (*candidateSink) IMEActivate(uint64) {}

func (s *candidateSink) IMECandidateSelected(_ uint64, index int) {
	s.picks = append(s.picks, index)
}

func TestCandidatePickRejectsReplacedList(t *testing.T) {
	sink := &candidateSink{}
	mux := New(sink, nil, nil)
	var sent []proto.Message
	client := mux.Connect("fcitx5", func(batch []proto.Message) { sent = append(sent, batch...) })
	session := mux.Open(client, 1)
	mux.Focus(session, true)
	publish := func(text string) uint64 {
		session.Post(proto.Candidates{Session: 1, Items: []proto.Candidate{{Text: text}}})
		return sent[len(sent)-1].Body.(proto.Candidates).Revision
	}
	first := publish("你")
	second := publish("泥")
	if first == 0 || second == 0 || first == second {
		t.Fatalf("candidate list revisions = %d, %d", first, second)
	}
	for _, revision := range []uint64{0, first, second} {
		mux.Dispatch(client, proto.Message{
			Seq: revision + 1, Body: proto.CandidatePick{Session: 1, Revision: revision},
		})
	}
	session.Post(proto.Commit{Session: 1, Text: "泥"})
	mux.Dispatch(client, proto.Message{
		Seq: second + 2, Body: proto.CandidatePick{Session: 1, Revision: second},
	})
	if !slices.Equal(sink.picks, []int{0}) {
		t.Fatalf("selected candidate indexes = %v, want only the current list's first index", sink.picks)
	}
}

func (s *testSink) IMEModeChanged(id string) { s.modes = append(s.modes, id) }

func TestSessionMuxTwoTransports(t *testing.T) {
	sink := &testSink{}
	m := New(sink, []proto.Mode{{ID: "zh"}, {ID: "en"}}, nil)
	var states [2][]string
	var sessions [2]*Session
	for i := range states {
		client := m.Connect("editor", func(batch []proto.Message) {
			for _, message := range batch {
				if mode, ok := message.Body.(proto.Mode); ok {
					states[i] = append(states[i], mode.ID)
				}
			}
		})
		sessions[i] = m.Open(client, 1)
	}
	if sessions[0].ID() == 0 || sessions[0].ID() == sessions[1].ID() {
		t.Fatal("transport session IDs collided")
	}
	if err := m.SetMode("en", true); err != nil {
		t.Fatal(err)
	}
	for _, got := range states {
		if !slices.Equal(got, []string{"zh", "en"}) {
			t.Errorf("mode broadcast = %v", got)
		}
	}
	if !slices.Equal(sink.modes, []string{"en"}) {
		t.Fatalf("mode callbacks = %v", sink.modes)
	}
}

func TestLongCommit(t *testing.T) {
	first := strings.Repeat("a", 16*1024-1)
	long := first + "😀" + strings.Repeat("字", 40000)
	m := New(&testSink{}, nil, nil)
	var batches [][]proto.Message
	c := m.Connect("editor", func(batch []proto.Message) { batches = append(batches, batch) })
	s := m.Open(c, 1)
	commit := proto.Commit{Session: s.RemoteID(), Text: long}
	s.Post(commit)
	m.reply(c, s.RemoteID(), 42, s, func() bool {
		s.Post(commit)
		return true
	})
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want one per commit", len(batches))
	}
	for i, seq := range []uint64{0, 42} {
		batch := batches[i]
		if seq != 0 {
			last := batch[len(batch)-1]
			if last.Seq != seq || last.Body != (proto.KeyResult{Session: 1, Eaten: true}) {
				t.Fatalf("last synchronous message = %#v", last)
			}
			batch = batch[:len(batch)-1]
		}
		var parts []string
		for j, msg := range batch {
			if _, err := proto.Marshal(msg); err != nil {
				t.Fatalf("part %d in batch %d does not encode: %v", j, i, err)
			}
			part, ok := msg.Body.(proto.Commit)
			if !ok || msg.Seq != seq || part.Session != s.RemoteID() {
				t.Fatalf("unexpected message at part %d in batch %d", j, i)
			}
			parts = append(parts, part.Text)
		}
		if len(parts) != 4 || strings.Join(parts, "") != long {
			t.Fatalf("batch %d contains %d parts and must rejoin to the original text", i, len(parts))
		}
		if parts[0] != first || !strings.HasPrefix(parts[1], "😀") {
			t.Fatalf("batch %d split inside the UTF-16 surrogate pair", i)
		}
	}
}

func TestLongCommitEditLimit(t *testing.T) {
	part := strings.Repeat("字", 16*1024)
	text := strings.Repeat(part, 33)
	m := New(&testSink{}, nil, nil)
	var batch []proto.Message
	c := m.Connect("editor", func(messages []proto.Message) { batch = messages })
	s := m.Open(c, 1)
	s.Post(proto.Commit{Session: 1, Text: text})
	if len(batch) != 32 {
		t.Fatalf("long commit produced %d frames, want 32", len(batch))
	}
	for i, msg := range batch {
		if msg.Body != (proto.Commit{Session: 1, Text: part}) {
			t.Fatalf("part %d does not match the expected text", i)
		}
	}
}

func TestLongComposition(t *testing.T) {
	first := strings.Repeat("a", 16*1024-1)
	m := New(&testSink{}, nil, nil)
	var batches [][]proto.Message
	c := m.Connect("editor", func(batch []proto.Message) { batches = append(batches, batch) })
	s := m.Open(c, 1)
	s.Post(proto.Compose{Session: 1, Text: first + "😀" + strings.Repeat("字", 40000), Cursor: 7})
	s.Post(proto.Compose{Session: 1})
	for i, want := range []proto.Compose{{Session: 1, Text: first, Cursor: 7}, {Session: 1}} {
		batch := batches[i]
		if len(batch) != 1 || batch[0].Body != want {
			t.Fatalf("composition %d must send one frame with the expected text and cursor", i)
		}
		if _, err := proto.Marshal(batch[0]); err != nil {
			t.Fatalf("composition %d does not encode: %v", i, err)
		}
	}
}
