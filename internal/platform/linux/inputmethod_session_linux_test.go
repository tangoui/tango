package linux

import (
	"fmt"
	"slices"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/imesession"
)

type ibusTestSink struct{ events []string }

func (s *ibusTestSink) add(event string, id uint64) {
	s.events = append(s.events, fmt.Sprint(event, id))
}

func (s *ibusTestSink) IMEActivate(id uint64)               { s.add("activate", id) }
func (s *ibusTestSink) IMEDeactivate(id uint64)             { s.add("deactivate", id) }
func (s *ibusTestSink) IMECommitComposition(id uint64)      { s.add("reset", id) }
func (s *ibusTestSink) IMEEnd(id uint64)                    { s.add("end", id) }
func (s *ibusTestSink) IMEKey(uint64, platform.IMEKey) bool { return false }

// Every session is activated once, before its first key, including one
// that a key opens after disable and enable without a focus change.
func TestIBusSessionsActivateOnce(t *testing.T) {
	sink := &ibusTestSink{}
	s := &ibusServer{mux: imesession.New(sink, nil, nil), byEngine: make(map[ptr]*ibusSession), byID: make(map[uint64]*ibusSession)}
	const engine = ptr(1)
	key := func() { sink.add("key", s.session(engine).id) }
	s.focusIn(engine, "/client/1", "editor")
	key()
	s.focusOut(engine, "") // disable
	key()                  // after enable, with no focus change
	s.focusIn(engine, "/client/1", "editor")
	want := []string{"activate1", "key1", "deactivate1", "end1", "activate2", "key2"}
	if !slices.Equal(sink.events, want) {
		t.Fatalf("events = %v, want %v", sink.events, want)
	}
	if got := s.byEngine[engine]; got.path != "/client/1" || got.routed.ClientApp() != "editor" {
		t.Fatalf("focus-in after a key left session %+v", got)
	}
}
