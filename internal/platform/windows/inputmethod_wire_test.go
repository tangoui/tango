package windows

import (
	"encoding/binary"
	"slices"
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
	lost := session.pending
	lost.Flags |= tipHandled
	session.pending = tipResponse{}
	session.commit("好")
	state.requeue(token, lost)
	got := session.pending
	if got.Flags != tipCommit || text(got.Committed[:got.CommittedLength]) != "你好" {
		t.Fatalf("requeued commit: flags %d, committed %q", got.Flags, text(got.Committed[:got.CommittedLength]))
	}

	// Older marked text comes back when nothing newer is queued.
	session.pending = tipResponse{}
	session.mark("ni", 2)
	lost = session.pending
	session.pending = tipResponse{}
	state.requeue(token, lost)
	if got := session.pending; got.Flags != tipMarked || text(got.Marked[:got.MarkedLength]) != "ni" || got.Cursor != 2 {
		t.Fatalf("requeued mark: %+v", got.Flags)
	}

	// A session that ended meanwhile drops the response.
	state.dispatch(tipRequest{Version: tipVersion, Action: tipEnd, Token: token})
	state.requeue(token, lost)
	if len(state.byToken) != 0 {
		t.Fatal("requeue revived an ended session")
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
