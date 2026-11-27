package windows

import (
	"slices"
	"strconv"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/tangoui/tango/internal/platform"
)

// This wire format is mirrored by examples/inputmethod/windows/tip.cpp.
// Each connection carries one request and one fixed-size response.
const (
	tipVersion  = 0x31454d49 // "IME1" in little-endian order
	tipActivate = 1
	tipKey      = 2
	tipReset    = 3
	tipEnd      = 4
	tipPoll     = 5

	tipHandled = 1
	tipMarked  = 2
	tipCommit  = 4
	// tipMore means committed text is still queued: the bridge polls again
	// before applying marked text, which arrives with the last part.
	tipMore = 8

	// tipMaxCommitted bounds the text queued for one session, in UTF-16 units.
	tipMaxCommitted = 64 * 1024
)

type tipRequest struct {
	Version, Action uint32
	Token           [16]byte
	PID             uint32
	X, Y, W, H      int32
	VK, Modifiers   uint32
	Text            [8]uint16
}

type tipResponse struct {
	Version, Flags, Cursor, MarkedLength, CommittedLength uint32
	Marked, Committed                                     [1024]uint16
}

type tipSessions struct {
	sink        platform.InputMethodSink
	processName func(uint32) string
	next        uint64
	byToken     map[[16]byte]*tipSession
	byID        map[uint64]*tipSession
}

func newTIPSessions(sink platform.InputMethodSink, processName func(uint32) string) *tipSessions {
	return &tipSessions{sink: sink, processName: processName,
		byToken: make(map[[16]byte]*tipSession), byID: make(map[uint64]*tipSession)}
}

type tipSession struct {
	id      uint64
	caret   platform.Rect
	client  string
	updated time.Time
	// pending holds the Handled and Commit flags and the marked text; the
	// committed text waits in committed and goes out in response-sized parts.
	pending   tipResponse
	committed []uint16
	// ended marks a session whose End response left text queued. Only Poll
	// reaches it, until the queue drains.
	ended bool
}

func (s *tipSessions) dispatch(r tipRequest) tipResponse {
	now := time.Now()
	response := tipResponse{Version: tipVersion}
	if r.Token == ([16]byte{}) || r.Action < tipActivate || r.Action > tipPoll {
		return response
	}
	session := s.byToken[r.Token]
	if session != nil && session.ended {
		session.updated = now
		response = session.take()
		if response.Flags&tipMore == 0 {
			delete(s.byToken, r.Token)
		}
		return response
	}
	if session == nil {
		if r.Action == tipReset || r.Action == tipEnd || r.Action == tipPoll {
			return response
		}
		// A process that terminates without sending End leaves a session
		// behind. Bound those entries and reclaim them before opening more.
		for token, stale := range s.byToken {
			if now.Sub(stale.updated) > time.Minute {
				if !stale.ended {
					delete(s.byID, stale.id)
					s.sink.IMEEnd(stale.id)
				}
				delete(s.byToken, token)
			}
		}
		if len(s.byToken) >= 256 {
			return response
		}
		s.next++
		session = &tipSession{id: s.next, client: s.processName(r.PID)}
		s.byToken[r.Token], s.byID[session.id] = session, session
		s.sink.IMEActivate(session.id)
	}
	session.updated = now
	if r.H > 0 {
		session.caret = platform.Rect{X: int(r.X), Y: int(r.Y), Width: int(r.W), Height: int(r.H)}
	}
	switch r.Action {
	case tipKey:
		if s.sink.IMEKey(session.id, tipKeyEvent(r)) {
			session.pending.Flags |= tipHandled
		}
	case tipReset:
		s.sink.IMECommitComposition(session.id)
	case tipEnd:
		s.sink.IMEDeactivate(session.id)
		delete(s.byID, session.id)
		s.sink.IMEEnd(session.id)
		session.ended = true
	}
	response = session.take()
	if session.ended && response.Flags&tipMore == 0 {
		delete(s.byToken, r.Token)
	}
	return response
}

// take removes the next response from the session's queue.
func (s *tipSession) take() tipResponse {
	pending := &s.pending
	r := tipResponse{Version: tipVersion, Flags: pending.Flags & tipHandled}
	pending.Flags &^= tipHandled
	if pending.Flags&tipCommit != 0 {
		n := min(len(s.committed), len(r.Committed))
		if n < len(s.committed) && n > 0 && utf16.IsSurrogate(rune(s.committed[n-1])) && s.committed[n-1] < 0xdc00 {
			n--
		}
		r.Flags |= tipCommit
		r.CommittedLength = uint32(copy(r.Committed[:], s.committed[:n]))
		s.committed = s.committed[n:]
		if len(s.committed) != 0 {
			r.Flags |= tipMore
			return r
		}
		s.committed = nil
	}
	if pending.Flags&tipMarked != 0 {
		r.Flags |= tipMarked
		r.Marked, r.MarkedLength, r.Cursor = pending.Marked, pending.MarkedLength, pending.Cursor
	}
	*pending = tipResponse{}
	return r
}

func tipKeyEvent(r tipRequest) platform.IMEKey {
	result := platform.IMEKey{
		Shift: r.Modifiers&1 != 0, Ctrl: r.Modifiers&2 != 0,
		Alt: r.Modifiers&4 != 0, Meta: r.Modifiers&8 != 0,
	}
	switch vk := r.VK; {
	case vk >= 'A' && vk <= 'Z':
		result.Code = "Key" + string(rune(vk))
	case vk >= '0' && vk <= '9':
		result.Code = "Digit" + string(rune(vk))
	case vk >= 0x60 && vk <= 0x69: // VK_NUMPAD0 through VK_NUMPAD9
		result.Code = "Numpad" + string(rune('0'+vk-0x60))
	case vk == 0x0d && r.Modifiers&16 != 0:
		result.Code = "NumpadEnter"
	default:
		result.Code = tipKeyCodes[vk]
	}
	n := slices.Index(r.Text[:], 0)
	if n < 0 {
		n = len(r.Text)
	}
	text := utf16.Decode(r.Text[:n])
	if len(text) == 1 && unicode.IsPrint(text[0]) {
		result.Text = string(text[0])
	}
	return result
}

var tipKeyCodes = func() map[uint32]string {
	codes := map[uint32]string{
		0x08: "Backspace", 0x09: "Tab", 0x0d: "Enter", 0x1b: "Escape", 0x20: "Space",
		0x21: "PageUp", 0x22: "PageDown", 0x23: "End", 0x24: "Home",
		0x25: "ArrowLeft", 0x26: "ArrowUp", 0x27: "ArrowRight", 0x28: "ArrowDown",
		0x2d: "Insert", 0x2e: "Delete",
		0x6a: "NumpadMultiply", 0x6b: "NumpadAdd", 0x6d: "NumpadSubtract", 0x6e: "NumpadDecimal", 0x6f: "NumpadDivide",
		// VK_OEM_*, named as on a U.S. keyboard like Code on the other platforms.
		0xba: "Semicolon", 0xbb: "Equal", 0xbc: "Comma", 0xbd: "Minus", 0xbe: "Period", 0xbf: "Slash",
		0xc0: "Backquote", 0xdb: "BracketLeft", 0xdc: "Backslash", 0xdd: "BracketRight", 0xde: "Quote",
	}
	for i := range uint32(10) {
		codes[0x60+i] = "Numpad" + string(rune('0'+i))
	}
	for i := range uint32(12) {
		codes[0x70+i] = "F" + strconv.Itoa(int(i)+1)
	}
	return codes
}()

func tipCopyUTF16(dst []uint16, text string) int {
	all := utf16.Encode([]rune(text))
	n := copy(dst, all)
	if n < len(all) && n > 0 && all[n-1] >= 0xd800 && all[n-1] <= 0xdbff {
		n--
	}
	return n
}

func (s *tipSession) mark(text string, cursor int) {
	pending := &s.pending
	pending.Flags |= tipMarked
	pending.MarkedLength = uint32(tipCopyUTF16(pending.Marked[:], text))
	pending.Cursor = 0
	for _, char := range text {
		if cursor <= 0 {
			break
		}
		pending.Cursor += uint32(utf16.RuneLen(char))
		cursor--
	}
	pending.Cursor = min(pending.Cursor, pending.MarkedLength)
}

func (s *tipSession) commit(text string) {
	s.pending.Flags = s.pending.Flags&^tipMarked | tipCommit
	units := utf16.Encode([]rune(text))
	if room := tipMaxCommitted - len(s.committed); len(units) > room {
		units = units[:max(room, 0)]
		if n := len(units); n > 0 && utf16.IsSurrogate(rune(units[n-1])) && units[n-1] < 0xdc00 {
			units = units[:n-1]
		}
	}
	s.committed = append(s.committed, units...)
}

// requeue returns an undelivered response's text to its session, ahead of
// anything queued since. The bridge has already let the key through, so
// the Handled flag is dropped.
func (s *tipSessions) requeue(token [16]byte, r tipResponse) {
	session := s.byToken[token]
	if session == nil || r.Flags&(tipMarked|tipCommit) == 0 {
		return
	}
	pending := &session.pending
	// Text marked or committed since supersedes the older marked text.
	newer := pending.Flags&(tipMarked|tipCommit) != 0
	if r.Flags&tipCommit != 0 {
		session.committed = append(slices.Clip(r.Committed[:r.CommittedLength]), session.committed...)
		pending.Flags |= tipCommit
	}
	if !newer && r.Flags&tipMarked != 0 {
		pending.Flags |= tipMarked
		pending.Marked, pending.MarkedLength, pending.Cursor = r.Marked, r.MarkedLength, r.Cursor
	}
}
