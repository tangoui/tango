package imesession

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

type testSink struct {
	platform.InputMethodSink
	modes []string
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
