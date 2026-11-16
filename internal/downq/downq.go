// Package downq holds the per-page downstream queue of the down:poll lane
// (docs/superpowers/specs/2026-11-14-down-poll-design.md): every message,
// byte packet and barrier the core sends to a page gets a sequence number,
// and while the page polls, the items wait here until a poll request
// (Serve) takes them in a batch. Items the page has not acknowledged are
// kept and sent again; when polling is not possible (before the first
// poll, after the page reports a failure, when the page stops polling)
// items go to a fallback, the core's Eval path, still numbered, so that
// the page can deliver everything in sequence order.
//
// A State belongs to one surface. Every page load starts a new generation
// (Start): numbering restarts at 1 and a fresh token guards the poll
// endpoint. All methods are safe from any goroutine.
package downq

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

// Kind is the kind of a downstream item.
type Kind uint8

const (
	// Msg is a string message (UTF-8), the msg of Surface.Send.
	Msg Kind = 0
	// Packet is a byte packet in the fast lane packet format
	// (count:u16 | frame*), for runtime-js's downstream lane.
	Packet Kind = 1
	// Barrier marks the place of a Surface.Eval script in the sequence;
	// its payload is the barrier id (u64, big-endian).
	Barrier Kind = 2
)

// Item is one downstream item.
type Item struct {
	Seq  uint64 // 1, 2, ... within a generation
	Kind Kind
	Data []byte
}

// Limits of the poll lane. They are read when they take effect (a poll
// request starts holding, a batch is cut, the watchdog is armed); tests
// change them.
var (
	// HoldMax is how long a poll request waits for items before it ends
	// with an empty batch.
	HoldMax = 25 * time.Second
	// BatchBytes and BatchItems bound one batch, whichever is reached
	// first. An item counts its payload plus its 5-byte header; an item
	// larger than BatchBytes goes alone in a batch.
	BatchBytes = 4 << 20
	BatchItems = 4096
	// WatchdogAfter is how long items may wait with no poll request in
	// progress before they go to the fallback instead.
	WatchdogAfter = time.Second
	// IdleEvalBytes and IdleEvalItems are the latency rule of the design's
	// §6: while a poll request waits on an idle lane (nothing pending or
	// unacknowledged), an item of at most IdleEvalBytes goes to fallback
	// instead, since one Eval reaches the page sooner than a poll response
	// (measured on WKWebView: about 40 µs less per round trip). At most
	// IdleEvalItems items go that way until the core takes them (Taken);
	// later ones queue for the poll, so a busy stream stays on the lane.
	// IdleEvalItems 0 turns the rule off.
	IdleEvalBytes = 1024
	IdleEvalItems = 8
)

// State is the downstream queue of one page. Create it with New.
type State struct {
	fallback func([]Item)

	mu          sync.Mutex
	closed      bool
	ended       bool   // the generation ended (End, Start, Close): Push drops
	gen         uint64 // current generation, 0 before the first Start
	tok         string // "" when the generation does not poll
	seq         uint64 // last sequence number given out
	active      bool   // a poll request of the generation arrived
	failed      bool   // the page reported failure (Fail)
	watchdogged bool   // the watchdog fired; cleared by the next poll
	queue       []Item // pending: not yet written to a response
	unacked     []Item // written to a response, not acknowledged; before queue
	reqs        int    // poll requests in progress (Serve)
	idleEval    int    // items the idle rule gave fallback since Taken
	wake        chan struct{}
	done        chan struct{} // closed when the generation ends or fails
	dog         *time.Timer   // the armed watchdog, nil when off

	// Fault injection (fault.go, development builds only): cut the
	// response carrying the generation's faultN+1th byte packet.
	faultOn      bool
	faultN       int
	faultPackets int  // byte packets written in this generation
	faulted      bool // this generation's cut was made
}

// New returns the queue of a page. fallback receives the items that do not
// go through polling, in sequence order: the sequence numbers within a
// call are contiguous and keep increasing from call to call within a
// generation. fallback must not be nil. It is called with the State's
// lock held, so it must not block and must not call the State; the core
// hands the items to its main thread. Items given to fallback are not
// kept.
func New(fallback func(items []Item)) *State {
	s := &State{
		fallback: fallback,
		ended:    true, // no generation yet
		wake:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	s.faultN, s.faultOn = loadFault()
	close(s.done)
	return s
}

// Start begins a new generation and returns its number and token: the
// previous generation ends (see End), numbering restarts so that the next
// Push gets 1. With enabled the generation polls once its first poll
// request arrives; without it tok is "" and every item goes to fallback.
// After Close, Start returns the current generation and "" and changes
// nothing.
func (s *State) Start(enabled bool) (gen uint64, tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.gen, ""
	}
	s.endLocked()
	s.gen++
	s.seq = 0
	s.ended, s.active, s.failed, s.watchdogged = false, false, false, false
	s.idleEval = 0
	s.faultPackets, s.faulted = 0, false
	s.done = make(chan struct{})
	s.tok = ""
	if enabled {
		var b [16]byte
		rand.Read(b[:]) // never fails (crypto/rand)
		s.tok = hex.EncodeToString(b[:])
	}
	return s.gen, s.tok
}

// End ends the current generation: pending and unacknowledged items are
// dropped (not given to fallback), poll requests in progress end with
// 410 Gone, and later Push calls drop their item and return 0 until the
// next Start.
func (s *State) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endLocked()
}

// Close ends the current generation for good (see End); Start no longer
// begins one.
func (s *State) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.endLocked()
}

// endLocked is End. s.mu held.
func (s *State) endLocked() {
	s.ended = true
	s.queue, s.unacked = nil, nil
	s.stopDogLocked()
	traceEnd(s)
	closeOnce(s.done)
}

// Push gives the next sequence number of the generation to an item of
// kind with payload data and returns it. While the generation polls
// (Active) the item waits for a poll request and wakes a waiting one,
// unless the idle rule (IdleEvalBytes) gives it to fallback; otherwise it
// goes to fallback before Push returns. After End, and before
// the first Start, the item is dropped and Push returns 0. Push keeps
// data: the caller must not change it afterwards.
func (s *State) Push(kind Kind, data []byte) (seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return 0
	}
	return s.pushLocked(kind, data)
}

// PushActive is Push for an item that may only be taken while the
// generation polls (Active), such as a down:poll byte packet: it pushes the
// item only if the generation polls and current (when not nil) reports
// true, both checked under the State's lock atomically with the push, and
// otherwise drops it and returns 0. current is called with the lock held;
// it must not call the State. The core uses it to bind a packet to the
// fast lane generation it was admitted in (lane.State.DownCurrent).
func (s *State) PushActive(kind Kind, data []byte, current func() bool) (seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.pollingLocked() || (current != nil && !current()) {
		return 0
	}
	return s.pushLocked(kind, data)
}

// pushLocked is Push for a generation that has not ended. s.mu held.
func (s *State) pushLocked(kind Kind, data []byte) (seq uint64) {
	s.seq++
	it := Item{Seq: s.seq, Kind: kind, Data: data}
	if !s.pollingLocked() || s.idleLocked(len(data)) {
		s.fallback([]Item{it})
		return it.Seq
	}
	tracePush(s, it.Seq)
	s.queue = append(s.queue, it)
	close(s.wake)
	s.wake = make(chan struct{})
	if s.reqs == 0 {
		s.armDogLocked()
	}
	return it.Seq
}

// idleLocked applies the idle rule (IdleEvalBytes) to an item of n bytes
// of a polling generation, counting it when it goes to fallback. s.mu
// held.
func (s *State) idleLocked(n int) bool {
	if s.reqs == 0 || len(s.queue) > 0 || len(s.unacked) > 0 ||
		n > IdleEvalBytes || s.idleEval >= IdleEvalItems {
		return false
	}
	s.idleEval++
	return true
}

// Taken tells the queue that the core took the items fallback got so far
// for delivery: the idle rule may hand IdleEvalItems more to fallback.
func (s *State) Taken() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleEval = 0
}

// Active reports whether Push now queues items for polling: the
// generation polls, its first poll request arrived, the page has not
// reported failure, and the watchdog has not sent items to fallback since
// the last poll request.
func (s *State) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pollingLocked()
}

// pollingLocked is Active. s.mu held.
func (s *State) pollingLocked() bool {
	return !s.ended && s.tok != "" && s.active && !s.failed && !s.watchdogged
}

// Fail handles the page's report that polling failed in generation gen
// after it delivered every item up to acked: those items are acknowledged,
// the remaining unacknowledged and pending items go to fallback in one
// call, poll requests in progress end with 410 Gone, and every later Push
// of the generation goes to fallback. A report for another generation, or
// a repeated one, is ignored.
func (s *State) Fail(gen, acked uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.gen || s.ended || s.tok == "" || s.failed {
		return
	}
	s.ackLocked(acked)
	s.failed = true
	s.stopDogLocked()
	closeOnce(s.done)
	if items := s.drainLocked(); len(items) > 0 {
		s.fallback(items)
	}
}

// ackLocked drops the unacknowledged items up to seq ack. s.mu held.
func (s *State) ackLocked(ack uint64) {
	n := 0
	for n < len(s.unacked) && s.unacked[n].Seq <= ack {
		n++
	}
	clear(s.unacked[:n])
	s.unacked = s.unacked[n:]
	if len(s.unacked) == 0 {
		s.unacked = nil
	}
	traceAck(s, ack)
}

// drainLocked removes and returns every unacknowledged and pending item,
// in sequence order. s.mu held.
func (s *State) drainLocked() []Item {
	items := make([]Item, 0, len(s.unacked)+len(s.queue))
	items = append(items, s.unacked...)
	items = append(items, s.queue...)
	s.unacked, s.queue = nil, nil
	return items
}

// takeLocked returns the next batch: the unacknowledged items (sent again,
// since the request's ack did not cover them) followed by pending items,
// within BatchBytes and BatchItems. The pending items taken become
// unacknowledged. first is the sequence number of the batch's first item,
// or of the next item to come when the batch is empty. s.mu held.
func (s *State) takeLocked() (first uint64, items []Item) {
	var size int
	fits := func(it Item) bool {
		n := itemHeader + len(it.Data)
		if len(items) > 0 && (len(items) >= BatchItems || size+n > BatchBytes) {
			return false
		}
		size += n
		items = append(items, it)
		return true
	}
	for _, it := range s.unacked {
		if !fits(it) {
			return items[0].Seq, items
		}
	}
	n := 0
	for n < len(s.queue) && fits(s.queue[n]) {
		n++
	}
	if n > 0 {
		s.unacked = append(s.unacked, s.queue[:n]...)
		clear(s.queue[:n])
		s.queue = s.queue[n:]
		if len(s.queue) == 0 {
			s.queue = nil
		}
	}
	if len(items) == 0 {
		return s.seq + 1, nil
	}
	return items[0].Seq, items
}

// faultCutLocked returns the index of the item of the batch items at
// which an injected fault cuts the response off, or -1 (always -1 unless
// fault injection is on; fault.go). s.mu held.
func (s *State) faultCutLocked(items []Item) int {
	if !s.faultOn || s.faulted {
		return -1
	}
	for i, it := range items {
		if it.Kind != Packet {
			continue
		}
		if s.faultPackets >= s.faultN {
			s.faulted = true
			noteFault(s.gen, it.Seq)
			return i
		}
		s.faultPackets++
	}
	return -1
}

// armDogLocked starts the watchdog unless it runs. s.mu held.
func (s *State) armDogLocked() {
	if s.dog != nil {
		return
	}
	var t *time.Timer
	t = time.AfterFunc(WatchdogAfter, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.dog != t { // stopped, or replaced, while this waited for mu
			return
		}
		s.dog = nil
		if !s.pollingLocked() || s.reqs > 0 || len(s.queue) == 0 && len(s.unacked) == 0 {
			return
		}
		// The page stopped polling (in the background, throttled): hand
		// everything not acknowledged to fallback, so that the sequence
		// it gets stays contiguous, until the next poll request.
		s.watchdogged = true
		s.fallback(s.drainLocked())
	})
	s.dog = t
}

// stopDogLocked stops the watchdog. s.mu held.
func (s *State) stopDogLocked() {
	if s.dog != nil {
		s.dog.Stop()
		s.dog = nil
	}
}

// tokenLocked reports whether gen and tok are the current generation's (in
// constant time for tok). Always false for a generation that does not
// poll. s.mu held.
func (s *State) tokenLocked(gen uint64, tok string) bool {
	return s.tok != "" && gen == s.gen &&
		subtle.ConstantTimeCompare([]byte(tok), []byte(s.tok)) == 1
}

// closeOnce closes ch unless it is closed.
func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
