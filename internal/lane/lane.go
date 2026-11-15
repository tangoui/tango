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
// All methods are safe from any goroutine. platformSend is called without
// the State's lock held, with the generation the packet was admitted in.
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
	DownScheme  = "down:scheme"  // streaming GET /__tango/lane/down on the asset origin
	UpPost      = "up:post"      // POST /__tango/lane/up on the asset origin
	DownAndroid = "down:android" // Android WebMessagePort / ArrayBuffer bridge
	UpAndroid   = "up:android"
	DownShm     = "down:shm" // WebView2 shared buffers
	UpShm       = "up:shm"
)

const (
	// MaxPostPacket is the largest packet runtime-js puts in one up:post
	// request; frames that do not fit go on the message channel.
	MaxPostPacket = 1 << 20
	// PostBatchBytes is the size at which runtime-js sends an up:post batch
	// without waiting for the batch delay.
	PostBatchBytes = 16 << 10

	// SchemeMinSize is BinaryMinSize while down:scheme is the downstream
	// lane: smaller frames go as messages, for latency. Platform lanes
	// (down:android, down:shm) take every byte frame (0).
	SchemeMinSize = 4096

	// maxDownQueue bounds the bytes queued for the down:scheme response;
	// past it Send returns false and frames go on the message channel.
	maxDownQueue = 8 << 20
)

// downPref and upPref are the order in which ready lanes are chosen:
// platform lanes before the scheme ones. runtime-js keeps at most one lane
// ready per direction, so the order only matters if a page misbehaves.
var (
	downPref = []string{DownShm, DownAndroid, DownScheme}
	upPref   = []string{UpShm, UpAndroid, UpPost}
)

func known(name string) bool {
	switch name {
	case DownScheme, UpPost, DownAndroid, UpAndroid, DownShm, UpShm:
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

// downQueue holds the packets waiting for one generation's down:scheme
// response. The response handler (ServeDown) takes packets under State.mu,
// waits on wake for more and stops when done is closed: a new generation,
// Close, or down:scheme failing ends the queue.
type downQueue struct {
	pkts  [][]byte
	size  int           // retained bytes: pkts plus in-flight Write/Flush calls
	wake  chan struct{} // capacity 1; signalled when a packet is queued
	done  chan struct{} // closed by end
	ended bool
	// conn is closed to end the response currently serving the queue
	// when another one connects (ServeDown); nil when none is connected.
	conn chan struct{}
}

func newDownQueue() *downQueue {
	return &downQueue{wake: make(chan struct{}, 1), done: make(chan struct{})}
}

// end ends the queue and drops its packets. s.mu held.
func (q *downQueue) end() {
	if q.ended {
		return
	}
	q.ended = true
	q.pkts, q.size = nil, 0
	close(q.done)
}

// State is the fast lane state of one page. Create it with New.
type State struct {
	platformSend func(gen uint64, p []byte) bool

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
	down   *downQueue        // this generation's down:scheme queue
	// downOff suspends this generation's downstream lanes (SuspendDown).
	downOff bool
}

// New returns the state of a page whose platform lanes (down:android,
// down:shm) send packets with platformSend (non-blocking, any goroutine;
// false when the packet was not taken). platformSend gets the generation
// the packet was admitted in; it is called without the State's lock held,
// so by then that generation may have ended, and the backend must deliver
// the packet only to the page lane of that generation (and drop it
// otherwise). It must not call back into the State. platformSend may be
// nil when the backend has no platform lane. TANGO_IPC_LANE_FAULT is read
// here.
func New(platformSend func(gen uint64, p []byte) bool) *State {
	s := &State{platformSend: platformSend, down: newDownQueue()}
	s.down.end() // no generation yet
	if name, n, ok := loadFault(); ok {
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
// number goes up, a new token is made, every lane is pending and the
// previous down:scheme response ends (its queued packets are dropped). It
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
	s.down.end()
	s.down = newDownQueue()
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

// failLocked marks name failed; failing down:scheme ends its response.
// s.mu held.
func (s *State) failLocked(name string) {
	s.st[name] = failed
	if name == DownScheme {
		s.down.end()
	}
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
// platformSend, or the down:scheme queue. It reports false when no lane is
// ready, the lane refused it or the queue is full; the caller then sends
// the frames as messages. On true the lane owns packet; on false it was
// neither kept nor modified (ipc.BinaryConn.SendBinary).
func (s *State) Send(packet []byte) bool { return s.send(0, false, packet) }

// SendGeneration atomically validates session identity with queue admission.
func (s *State) SendGeneration(gen uint64, packet []byte) bool { return s.send(gen, true, packet) }

func (s *State) send(genID uint64, scoped bool, packet []byte) bool {
	s.mu.Lock()
	if scoped && genID != s.gen {
		s.mu.Unlock()
		return false
	}
	name := s.downLocked()
	counted := !emptyPacket(packet)
	if name == "" || s.faultLocked(name, counted) {
		s.mu.Unlock()
		return false
	}
	if name == DownScheme {
		defer s.mu.Unlock()
		q := s.down
		if q.ended || q.size+len(packet) > maxDownQueue {
			if counted {
				s.uncountLocked(name, s.gen)
			}
			return false
		}
		q.pkts = append(q.pkts, packet)
		q.size += len(packet)
		select {
		case q.wake <- struct{}{}:
		default:
		}
		return true
	}
	// platformSend runs without s.mu (no lock order with the backend's own
	// locks); gen lets the backend drop the packet if the generation ends
	// meanwhile.
	gen := s.gen
	s.mu.Unlock()
	if s.platformSend != nil && s.platformSend(gen, packet) {
		return true
	}
	if counted {
		s.mu.Lock()
		s.uncountLocked(name, gen)
		s.mu.Unlock()
	}
	return false
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

// MinSize is ipc.BinaryConn.BinaryMinSize: SchemeMinSize while down:scheme
// is the ready downstream lane, 0 for a platform lane, -1 when none is
// ready.
func (s *State) MinSize() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.downLocked() {
	case "":
		return -1
	case DownScheme:
		return SchemeMinSize
	default:
		return 0
	}
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
// origin). Send and UpAllowed refuse, MinSize is -1, the token is dead and
// the down:scheme response ends, until the next Hello.
func (s *State) Reset() { s.Hello(nil, false) }

// SuspendDown stops the downstream lanes of the current generation until
// the next Hello: Send refuses, MinSize is -1 and the down:scheme response
// ends. The page is navigating and its successor must not receive this
// page's bytes; a later ready for a downstream lane of this generation does
// not lift it. The upstream lane and the token stay valid: a navigation may
// leave the page in place (a download, a failed load), and its uploads in
// flight must still arrive.
func (s *State) SuspendDown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downOff = true
	s.down.end()
}

// Queued returns the packets and bytes waiting in the current generation's
// down:scheme queue (tests, diagnostics).
func (s *State) Queued() (packets, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.down.pkts), s.down.size
}

// Close ends the State for good (the surface closed): no lane is usable,
// the down:scheme response ends and Hello returns nil.
func (s *State) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.down.end()
	s.tok, s.caps, s.st = "", nil, nil
}

// Generation is an immutable identity for the current document session.
func (s *State) Generation() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.gen }

// AllowUpGen is asked for every packet a platform upstream lane (up:android,
// up:shm) received, before it is delivered: true when gen, the generation
// the page sent it in, is the current one and the upstream lane is usable
// (UpAllowed), both checked under one lock so a delayed packet of an old
// generation never counts for a new one. Only data packets are counted for
// fault injection; an empty packet (count 0, such as the up:android probe)
// is checked but not counted.
func (s *State) AllowUpGen(gen uint64, p []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gen == s.gen && s.upAllowedLocked(!emptyPacket(p))
}
