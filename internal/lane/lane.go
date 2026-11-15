// Package lane holds the per-page state of IPC fast lanes: side channels
// that carry stream byte frames as binary packets next to the message
// channel (see the ipc package comment, "Fast lanes", and
// docs/specs/2026-11-13-ipc-duplex-streams.md §6.1).
//
// A State belongs to one surface. Every hello from runtime-js starts a new
// generation (Hello): a fresh token, the lanes the page may use, all of them
// pending until the page reports them ready (Control). The core's
// *tango.Surface wraps a State to implement ipc.BinaryConn.
//
// All methods are safe from any goroutine. platformSend and pollSend are
// called without the State's lock held.
package lane

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"sync"
)

// Lane names, as reported by backends (platform.BinaryLane.LaneCaps), listed
// in the hello reply and named in control messages.
const (
	UpPost      = "up:post"      // POST /__tango/lane/up on the asset origin
	DownAndroid = "down:android" // Android WebMessagePort / ArrayBuffer bridge
	UpAndroid   = "up:android"
	DownShm     = "down:shm" // WebView2 shared buffers
	UpShm       = "up:shm"
	// DownPoll is the core's downstream transport: GET /__tango/lane/poll
	// on the asset origin (the root package and internal/downq). Its
	// packets go to New's pollSend.
	DownPoll = "down:poll"
)

const (
	// MaxPostPacket is the largest packet runtime-js puts in one up:post
	// request; frames that do not fit go on the message channel.
	MaxPostPacket = 1 << 20
	// PostBatchBytes is the size at which runtime-js sends an up:post batch
	// without waiting for the batch delay.
	PostBatchBytes = 16 << 10
)

// downPref and upPref are the order in which ready lanes are chosen:
// platform lanes before down:poll and up:post. runtime-js keeps at most one
// lane ready per direction, so the order only matters if a page misbehaves.
var (
	downPref = []string{DownShm, DownAndroid, DownPoll}
	upPref   = []string{UpShm, UpAndroid, UpPost}
)

func known(name string) bool {
	switch name {
	case DownPoll, UpPost, DownAndroid, UpAndroid, DownShm, UpShm:
		return true
	}
	return false
}

func isUp(name string) bool { return name == UpPost || name == UpAndroid || name == UpShm }

type status int

const (
	pending status = iota // offered in the hello, not reported ready yet
	ready
	failed // reported failed, or faulted; stays failed for the generation
)

// State is the fast lane state of one page. Create it with New.
type State struct {
	platformSend func([]byte) bool
	pollSend     func([]byte) bool

	// Fault injection (tango_dev builds; fault.go): the lane faultLane
	// fails after faultN packets in a generation.
	faultLane string
	faultN    int

	mu     sync.Mutex
	closed bool
	gen    uint64
	tok    string            // "" when the generation has no lanes
	caps   []string          // lanes offered in this generation, hello order
	st     map[string]status // per offered lane
	count  int               // packets counted for faultLane in this generation
	// downOff suspends this generation's downstream lanes (SuspendDown).
	downOff bool
}

// New returns the state of a page whose platform lanes (down:android,
// down:shm) send packets with platformSend and whose down:poll lane sends
// them with pollSend (the core's downstream queue). Both are non-blocking,
// called from any goroutine without the State's lock held, and return
// false when the packet was not taken; either may be nil (the lane then
// takes nothing). TANGO_IPC_LANE_FAULT is read here.
func New(platformSend, pollSend func([]byte) bool) *State {
	s := &State{platformSend: platformSend, pollSend: pollSend}
	// down:poll's faults are injected where its responses are written
	// (internal/downq's fault.go), so that they exercise the real recovery.
	if name, n, ok := loadFault(); ok && name != DownPoll {
		s.faultLane, s.faultN = name, n
	}
	return s
}

type helloLanes struct {
	Gen  uint64   `json:"gen"`
	Tok  string   `json:"tok"`
	List []string `json:"list"`
}

// Hello starts a new generation for a hello from the page: the generation
// number goes up, a new token is made and every lane is pending. It
// returns the hello reply's "lanes" field for caps (unknown and repeated
// names dropped), or nil when enabled is false, no known lane is left, or
// the State is closed; the previous generation ends in every case.
func (s *State) Hello(caps []string, enabled bool) json.RawMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.gen++
	s.tok, s.caps, s.st, s.count = "", nil, nil, 0
	s.downOff = false
	if !enabled {
		return nil
	}
	st := map[string]status{}
	var list []string
	for _, c := range caps {
		if _, dup := st[c]; dup || !known(c) {
			continue
		}
		st[c] = pending
		list = append(list, c)
	}
	if len(list) == 0 {
		return nil
	}
	var b [16]byte
	rand.Read(b[:]) // never fails (crypto/rand)
	s.tok, s.caps, s.st = hex.EncodeToString(b[:]), list, st
	raw, _ := json.Marshal(helloLanes{Gen: s.gen, Tok: s.tok, List: list})
	return raw
}

type control struct {
	Op   string `json:"op"`
	Lane string `json:"lane"`
	Gen  uint64 `json:"gen"`
}

// Control applies a control message from the page (the payload of the
// core's internal "lane" channel): {"op":"ready"|"fail","lane":name,
// "gen":n}. Messages for another generation, for a lane not offered in
// this one, and malformed ones are dropped. A failed lane stays failed
// until the next generation.
func (s *State) Control(payload string) {
	var m control
	if json.Unmarshal([]byte(payload), &m) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || m.Gen != s.gen {
		return
	}
	cur, ok := s.st[m.Lane]
	if !ok {
		return
	}
	switch m.Op {
	case "ready":
		if cur == pending {
			s.st[m.Lane] = ready
		}
	case "fail":
		s.failLocked(m.Lane)
	}
}

// failLocked marks name failed. s.mu held.
func (s *State) failLocked(name string) {
	s.st[name] = failed
}

// downLocked returns the ready downstream lane, or "". s.mu held.
func (s *State) downLocked() string {
	if s.closed || s.downOff {
		return ""
	}
	for _, name := range downPref {
		if s.st[name] == ready {
			return name
		}
	}
	return ""
}

// faultLocked counts one data packet on name for fault injection and
// reports whether the lane has faulted (it is then failed). An empty packet
// (count false: no frames, such as the up:post start probe) is not counted
// and never faults the lane; whether the lane is still usable is the
// caller's check. s.mu held.
func (s *State) faultLocked(name string, count bool) bool {
	if !count || name != s.faultLane || s.faultLane == "" {
		return false
	}
	if s.count >= s.faultN {
		s.failLocked(name)
		return true
	}
	s.count++
	return false
}

// Send hands packet to the ready downstream lane: a platform lane's
// platformSend, or down:poll's pollSend. It reports false when no lane is
// ready or the lane refused it; the caller then sends the frames as
// messages. On true the lane owns packet; on false it was
// neither kept nor modified (ipc.BinaryConn.SendBinary).
func (s *State) Send(packet []byte) bool { ok, _ := s.send(0, false, packet); return ok }

// SendOrdered is Send that also reports whether the lane that took packet
// is down:poll, whose packets share one sequenced queue with the page's
// messages (ipc.OrderedBinaryConn). ordered is false when sent is false.
func (s *State) SendOrdered(packet []byte) (sent, ordered bool) { return s.send(0, false, packet) }

// SendGeneration is Send for the session of generation gen: false when
// gen is not the current generation. The check is made under s.mu but the
// lane's send (platformSend, pollSend) runs after it is released, so a new
// Hello in between does not stop the packet; the lane's own generation
// (down:poll's page generation, a platform lane's) is what drops it then.
func (s *State) SendGeneration(gen uint64, packet []byte) bool {
	ok, _ := s.send(gen, true, packet)
	return ok
}

// SendGenerationOrdered is SendGeneration that also reports ordered, as
// SendOrdered does.
func (s *State) SendGenerationOrdered(gen uint64, packet []byte) (sent, ordered bool) {
	return s.send(gen, true, packet)
}

func (s *State) send(genID uint64, scoped bool, packet []byte) (sent, ordered bool) {
	s.mu.Lock()
	if scoped && genID != s.gen {
		s.mu.Unlock()
		return false, false
	}
	name := s.downLocked()
	counted := !emptyPacket(packet)
	if name == "" || s.faultLocked(name, counted) {
		s.mu.Unlock()
		return false, false
	}
	gen := s.gen
	s.mu.Unlock()
	send := s.platformSend
	if name == DownPoll {
		send = s.pollSend
	}
	if send != nil && send(packet) {
		return true, name == DownPoll
	}
	if counted {
		s.mu.Lock()
		s.uncountLocked(name, gen)
		s.mu.Unlock()
	}
	return false, false
}

// emptyPacket reports whether packet carries no frames (count 0), like the
// up:post start probe [0,0]. Fault injection does not count such packets.
func emptyPacket(packet []byte) bool {
	return len(packet) >= 2 && packet[0] == 0 && packet[1] == 0
}

// uncountLocked takes back a packet faultLocked counted that the lane did
// not take. s.mu held.
func (s *State) uncountLocked(name string, gen uint64) {
	if name == s.faultLane && gen == s.gen && s.count > 0 {
		s.count--
	}
}

// MinSize is ipc.BinaryConn.BinaryMinSize: 0 while a downstream lane is
// ready (every lane takes byte frames of any size), -1 when none is.
func (s *State) MinSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.downLocked() == "" {
		return -1
	}
	return 0
}

// CheckToken reports whether gen and tok are the current generation's (in
// constant time for tok). Always false for a generation without lanes and
// after Close.
func (s *State) CheckToken(gen uint64, tok string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenLocked(gen, tok)
}

// tokenLocked is CheckToken. s.mu held.
func (s *State) tokenLocked(gen uint64, tok string) bool {
	return !s.closed && s.tok != "" && gen == s.gen &&
		subtle.ConstantTimeCompare([]byte(tok), []byte(s.tok)) == 1
}

// UpAllowed is asked for every upstream packet that arrives (on a platform
// lane or up:post) before it is delivered: true while the page's upstream
// lane is usable. That is the ready upstream lane or, before the page has
// reported one ready, the first upstream lane of the hello that has not
// failed (a packet can overtake the ready message: the message channel and
// the lane are not ordered). Packets counted for fault injection are
// counted here.
func (s *State) UpAllowed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upAllowedLocked(true)
}

// upAllowedLocked is UpAllowed. count is false for a packet known to be
// empty (no frames: the up:post start probe), which is checked for
// usability but not counted for fault injection. s.mu held.
func (s *State) upAllowedLocked(count bool) bool {
	if s.closed {
		return false
	}
	name := ""
	for _, n := range upPref {
		if s.st[n] == ready {
			name = n
			break
		}
	}
	if name == "" {
		for _, n := range s.caps {
			if isUp(n) && s.st[n] == pending {
				name = n
				break
			}
		}
	}
	return name != "" && !s.faultLocked(name, count)
}

// Reset ends the current generation without starting one with lanes, as
// a disabled Hello does: the page has left (navigated away from the asset
// origin). Send and UpAllowed refuse, MinSize is -1 and the token is dead,
// until the next Hello.
func (s *State) Reset() { s.Hello(nil, false) }

// SuspendDown stops the downstream lanes of the current generation until
// the next Hello: Send refuses and MinSize is -1. The page is navigating
// and its successor must not receive this page's bytes on a platform lane
// (down:android, down:shm; down:poll's own generation ends in the core's
// downstream queue); a later ready for a downstream lane of this
// generation does not lift it. The upstream lane and the token stay valid:
// a navigation may leave the page in place (a download, a failed load),
// and its uploads in flight must still arrive.
func (s *State) SuspendDown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downOff = true
}

// Close ends the State for good (the surface closed): no lane is usable
// and Hello returns nil.
func (s *State) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.tok, s.caps, s.st = "", nil, nil
}

// Generation is an immutable identity for the current document session.
func (s *State) Generation() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.gen }

// AllowUpSession authorizes native ingress and captures its session atomically.
// Native backends must additionally bind originating document identity.
func (s *State) AllowUpSession() (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen, s.upAllowedLocked(true)
}
