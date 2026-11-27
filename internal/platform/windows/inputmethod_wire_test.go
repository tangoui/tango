package windows

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/tangoui/tango/internal/platform"
)

type tipTestSink struct {
	events []string
	onKey  func(uint64, platform.IMEKey) bool
}

func (s *tipTestSink) IMEActivate(uint64)          { s.events = append(s.events, "activate") }
func (s *tipTestSink) IMEDeactivate(uint64)        { s.events = append(s.events, "deactivate") }
func (s *tipTestSink) IMECommitComposition(uint64) { s.events = append(s.events, "reset") }
func (s *tipTestSink) IMEEnd(uint64)               { s.events = append(s.events, "end") }
func (s *tipTestSink) IMEKey(session uint64, key platform.IMEKey) bool {
	return s.onKey(session, key)
}

func TestTIPWireSizes(t *testing.T) {
	if req, res := binary.Size(tipRequest{}), binary.Size(tipResponse{}); req != 68 || res != 4116 {
		t.Fatalf("wire sizes %d/%d, want Windows bridge layout 68/4116", req, res)
	}
}

func TestTIPSessionRoundTrip(t *testing.T) {
	sink := &tipTestSink{}
	state := newTIPSessions(sink, func(uint32) string { return "Editor.exe" })
	request := tipRequest{Token: [16]byte{1}, PID: 135, VK: 'N', Text: [8]uint16{'n'}, X: 17, Y: 23, H: 20}
	request.Action = tipActivate
	state.dispatch(request)
	sink.onKey = func(id uint64, k platform.IMEKey) bool {
		if k.Code != "KeyN" || k.Text != "n" {
			t.Errorf("unexpected key %+v", k)
		}
		session := state.byID[id]
		if session.client != "Editor.exe" || session.caret.X != 17 || session.caret.Height != 20 {
			t.Errorf("unexpected session %+v", session)
		}
		session.mark("a😀b", 2)
		return true
	}
	request.Action = tipKey
	response := state.dispatch(request)
	if response.Flags != tipHandled|tipMarked || response.Cursor != 3 ||
		string(utf16.Decode(response.Marked[:response.MarkedLength])) != "a😀b" {
		t.Fatalf("key response has flags %d, cursor %d and length %d", response.Flags, response.Cursor, response.MarkedLength)
	}
	state.byToken[request.Token].commit("你好")
	request.Action = tipPoll
	response = state.dispatch(request)
	if response.Flags != tipCommit || string(utf16.Decode(response.Committed[:response.CommittedLength])) != "你好" {
		t.Fatalf("commit response has flags %d and length %d", response.Flags, response.CommittedLength)
	}
	request.Action = tipEnd
	state.dispatch(request)
	if !slices.Equal(sink.events, []string{"activate", "deactivate", "end"}) || len(state.byID) != 0 || len(state.byToken) != 0 {
		t.Fatalf("ended session events = %v, state = %+v", sink.events, state.byToken)
	}
}

func TestTIPRequeue(t *testing.T) {
	text := func(units []uint16) string { return string(utf16.Decode(units)) }
	sink := &tipTestSink{}
	state := newTIPSessions(sink, func(uint32) string { return "" })
	token := [16]byte{7}
	state.dispatch(tipRequest{Version: tipVersion, Action: tipActivate, Token: token})
	session := state.byToken[token]

	// The lost response committed "你" and marked "h"; "好" came since.
	session.commit("你")
	session.mark("h", 1)
	session.pending.Flags |= tipHandled
	lost := session.take()
	session.commit("好")
	state.requeue(token, lost)
	got := session.take()
	if got.Flags != tipCommit || text(got.Committed[:got.CommittedLength]) != "你好" {
		t.Fatalf("requeued commit: flags %d, committed %q", got.Flags, text(got.Committed[:got.CommittedLength]))
	}

	// Older marked text comes back when nothing newer is queued.
	session.mark("ni", 2)
	lost = session.take()
	state.requeue(token, lost)
	if got := session.take(); got.Flags != tipMarked || text(got.Marked[:got.MarkedLength]) != "ni" || got.Cursor != 2 {
		t.Fatalf("requeued mark: %+v", got.Flags)
	}

	// A session that ended meanwhile drops the response.
	state.dispatch(tipRequest{Version: tipVersion, Action: tipEnd, Token: token})
	state.requeue(token, lost)
	if len(state.byToken) != 0 {
		t.Fatal("requeue revived an ended session")
	}
}

// Committed text longer than one response goes out in parts. Marked text
// waits for the last part, and a surrogate pair is never split.
func TestTIPLongCommit(t *testing.T) {
	sink := &tipTestSink{}
	state := newTIPSessions(sink, func(uint32) string { return "" })
	token := [16]byte{9}
	state.dispatch(tipRequest{Version: tipVersion, Action: tipActivate, Token: token})
	session := state.byToken[token]

	long := strings.Repeat("a", 1023) + "😀" + strings.Repeat("字", 1500)
	session.commit(long)
	session.mark("zi", 1)
	var got []uint16
	var last tipResponse
	for i := 0; ; i++ {
		if i > 4 {
			t.Fatal("commit did not drain")
		}
		last = state.dispatch(tipRequest{Version: tipVersion, Action: tipPoll, Token: token})
		if last.Flags&tipCommit == 0 {
			t.Fatalf("part %d has flags %d", i, last.Flags)
		}
		got = append(got, last.Committed[:last.CommittedLength]...)
		if last.Flags&tipMore == 0 {
			break
		}
		if last.Flags&tipMarked != 0 {
			t.Fatalf("part %d carries marked text before the commit drained", i)
		}
	}
	if string(utf16.Decode(got)) != long {
		t.Fatalf("reassembled %d units, want %d", len(got), len(utf16.Encode([]rune(long))))
	}
	if last.Flags&tipMarked == 0 || string(utf16.Decode(last.Marked[:last.MarkedLength])) != "zi" {
		t.Fatalf("last part flags %d", last.Flags)
	}
	if poll := state.dispatch(tipRequest{Version: tipVersion, Action: tipPoll, Token: token}); poll.Flags != 0 {
		t.Fatalf("drained session flags %d", poll.Flags)
	}

	session.commit(strings.Repeat("b", tipMaxCommitted+10))
	if len(session.committed) != tipMaxCommitted {
		t.Fatalf("queued %d units, want the %d cap", len(session.committed), tipMaxCommitted)
	}
}

// Text committed while a session ends stays reachable by Poll until drained.
func TestTIPEndDrainsCommit(t *testing.T) {
	sink := &tipTestSink{}
	state := newTIPSessions(sink, func(uint32) string { return "" })
	token := [16]byte{11}
	state.dispatch(tipRequest{Version: tipVersion, Action: tipActivate, Token: token})
	id := state.byToken[token].id
	long := strings.Repeat("末", 1500)
	state.byID[id].commit(long)

	end := state.dispatch(tipRequest{Version: tipVersion, Action: tipEnd, Token: token})
	if end.Flags != tipCommit|tipMore || len(state.byID) != 0 ||
		!slices.Equal(sink.events, []string{"activate", "deactivate", "end"}) {
		t.Fatalf("end flags %d, events %v", end.Flags, sink.events)
	}
	got := end.Committed[:end.CommittedLength]
	poll := state.dispatch(tipRequest{Version: tipVersion, Action: tipPoll, Token: token})
	got = append(got, poll.Committed[:poll.CommittedLength]...)
	if poll.Flags != tipCommit || string(utf16.Decode(got)) != long {
		t.Fatalf("drain flags %d, %d units", poll.Flags, len(got))
	}
	if len(state.byToken) != 0 || len(sink.events) != 3 {
		t.Fatalf("drained session left tokens %d, events %v", len(state.byToken), sink.events)
	}
}

func TestTIPKeyCodes(t *testing.T) {
	for vk, code := range map[uint32]string{
		'Q': "KeyQ", '7': "Digit7", 0xbc: "Comma", 0xbe: "Period", 0xbd: "Minus", 0xbb: "Equal",
		0xba: "Semicolon", 0xdb: "BracketLeft", 0x60: "Numpad0", 0x69: "Numpad9", 0x6e: "NumpadDecimal",
		0x70: "F1", 0x7b: "F12", 0x25: "ArrowLeft", 0x10: "",
	} {
		if got := tipKeyEvent(tipRequest{VK: vk}).Code; got != code {
			t.Errorf("VK %#x: Code %q, want %q", vk, got, code)
		}
	}
}

func TestTIPNumpadDigit(t *testing.T) {
	if got := tipKeyEvent(tipRequest{VK: 0x62, Text: [8]uint16{'2'}}); got.Code != "Numpad2" || got.Text != "2" {
		t.Fatalf("key = %+v", got)
	}
}
