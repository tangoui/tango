package tango

// The core's downstream transport (docs/superpowers/specs/
// 2026-11-14-down-poll-design.md §5): Surface.Send and Surface.Eval go
// through the page's internal/downq queue. While the page polls
// (/__tango/lane/poll, the down:poll lane), messages wait there for its
// poll requests and an Eval script becomes a barrier item; otherwise
// (before the first poll, after a failure, when the watchdog fires, or
// when down:poll is off) the queue hands the items to the fallback, which
// delivers them with one Eval of window.__tango._recvSeq per batch. Every
// item carries a sequence number, so the page delivers them in Send order
// whichever way they came (downShim, downshim.go).

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/downq"
	"github.com/tangoui/tango/internal/lane"
)

// downChannel is the internal channel of the page's down:poll reports
// (corehook.InternalPrefix + "down:" + JSON; downShim).
const downChannel = "down"

// surfaceDown is a surface's side of the downstream transport.
//
// sendMu orders Send, Eval and the page's load state: loaded and pending
// change, and items are pushed, only under it. That is what keeps the
// Sends of one goroutine in call order across a page load (a Send that
// finds the page not loaded queues in pending, which the load pushes
// before any later Send).
type surfaceDown struct {
	sendMu  sync.Mutex
	pending []string // Send messages waiting for a trusted page; sendMu

	mu     sync.Mutex
	gen    uint64      // the generation started last; 0 once it ended
	out    []downBatch // fallback batches waiting for the main thread
	posted bool        // a drainDown is posted for out
	nextID uint64      // last barrier id given out
	// barrier holds the scripts of gen's barriers that have not run, by
	// id. Ids are never reused, so a missing id is one that ran (or
	// belongs to an ended generation): each script runs at most once.
	barrier map[uint64]string
}

// downBatch is one entry of the outbox: a fallback call's items of
// generation gen, or (script) the js of an Eval that is not a barrier.
type downBatch struct {
	gen    uint64
	items  []downq.Item
	script bool
	js     string
}

// initDown sets up s's downstream queue. Before the native surface exists.
func (s *Surface) initDown() {
	s.downq = downq.New(s.downFallback)
}

// downEnabled reports whether the page that just loaded gets the
// down:poll lane: it comes from the asset origin, fast lanes are not
// turned off, and the backend reports the lane. It does not depend on
// OnBinary handlers: string messages do not need them.
func (s *Surface) downEnabled() bool {
	a := s.app
	bl, origin := s.laneNative()
	return bl != nil && !a.lanesOff() && a.isAssetOrigin(origin) &&
		slices.Contains(bl.LaneCaps(), lane.DownPoll)
}

// pageReady marks the page trusted and loaded: a new downstream generation
// starts (opening the poll lane in the page when enabled) and the messages
// sent while no page was loaded are pushed in order. They are delivered by
// the time it returns unless the page polls. Main thread.
func (s *Surface) pageReady() {
	if s.closed.Load() {
		return
	}
	if s.ns == nil { // loaded while being created: addSurface calls again
		s.readyEarly = true
		return
	}
	d := &s.down
	d.sendMu.Lock()
	enabled := s.downEnabled()
	gen, tok := s.downq.Start(enabled)
	d.mu.Lock()
	d.gen = gen
	d.barrier = nil
	d.mu.Unlock()
	if tok != "" {
		o, _ := json.Marshal(struct {
			Gen  uint64 `json:"gen"`
			Tok  string `json:"tok"`
			Path string `json:"path"`
		}{gen, tok, lanePathPrefix + "poll"})
		s.ns.Eval("window.__tango._open(" + lineSepEscaper.Replace(string(o)) + ")")
	}
	s.loaded.Store(true)
	pending := d.pending
	d.pending = nil
	for _, msg := range pending {
		s.downq.Push(downq.Msg, []byte(msg))
	}
	d.sendMu.Unlock()
	s.drainDown()
}

// pageGone marks the page not loaded: the downstream generation ends,
// dropping what it had not delivered, and later Sends wait in pending for
// the next trusted page. Main thread (and handleClosed).
func (s *Surface) pageGone() {
	d := &s.down
	d.sendMu.Lock()
	s.loaded.Store(false)
	s.downq.End()
	d.mu.Lock()
	d.gen = 0 // drainDown drops the outbox's items of the ended generation
	d.barrier = nil
	d.mu.Unlock()
	d.sendMu.Unlock()
}

// sendDown is Send: any goroutine. Messages pushed here are delivered in
// the order of the calls that pushed them. On the main thread a message
// that goes to the fallback is evaluated before sendDown returns, as Send
// always did there (a Navigate right after it must not drop it).
func (s *Surface) sendDown(msg string) {
	d := &s.down
	d.sendMu.Lock()
	if !s.loaded.Load() {
		d.pending = append(d.pending, msg)
		d.sendMu.Unlock()
		return
	}
	s.downq.Push(downq.Msg, []byte(msg))
	d.sendMu.Unlock()
	if d.outboxed() && s.app.isMain() {
		s.drainDown()
	}
}

// evalDown is Eval: any goroutine. While a page is loaded js always
// becomes a barrier item with a sequence number (design 5.9): while the
// page polls it runs when the page reaches it (handleDown) or when the
// fallback takes it over; otherwise the item goes to the fallback at once
// and the script runs at its place there, so the page cannot deliver a
// later message before it even if polling starts meanwhile. With no page
// loaded js joins the outbox as is. On the main thread it has run by the
// time evalDown returns, as Eval always did there (unless the page polls).
func (s *Surface) evalDown(js string) {
	d := &s.down
	d.sendMu.Lock()
	if s.loaded.Load() {
		d.mu.Lock()
		d.nextID++
		id := d.nextID
		if d.barrier == nil {
			d.barrier = map[uint64]string{}
		}
		// Recorded before Push: the item may go to the fallback at once.
		d.barrier[id] = js
		d.mu.Unlock()
		s.downq.Push(downq.Barrier, binary.BigEndian.AppendUint64(nil, id))
		d.sendMu.Unlock()
		if d.outboxed() && s.app.isMain() {
			s.drainDown()
		}
		return
	}
	s.toOutbox(downBatch{script: true, js: js})
	d.sendMu.Unlock()
	if s.app.isMain() {
		s.drainDown()
	}
}

// outboxed reports whether the outbox holds batches. Send and Eval drain
// it on the main thread only then: an item the poll lane queued needs no
// main thread, and isMain (a goroutine id lookup) costs microseconds,
// several times the rest of a Send.
func (d *surfaceDown) outboxed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.out) > 0
}

// downFallback is downq's fallback. downq's lock is held and the caller
// may be any goroutine: the items join the outbox, in order, and the main
// thread delivers them (drainDown).
func (s *Surface) downFallback(items []downq.Item) {
	// The items are of the generation started last (toOutbox): downq
	// hands over only its current generation's, and d.gen changes right
	// after downq.State.Start and End.
	s.toOutbox(downBatch{items: slices.Clone(items)})
}

// toOutbox appends b, stamped with the current generation, to the outbox
// and makes sure a drainDown is posted. Any goroutine; it does not block.
func (s *Surface) toOutbox(b downBatch) {
	d := &s.down
	d.mu.Lock()
	b.gen = d.gen
	d.out = append(d.out, b)
	post := !d.posted
	d.posted = true
	d.mu.Unlock()
	if post {
		s.app.loop.post(s.drainDown)
	}
}

// drainDown delivers the outbox in order (recvSeq): items of an ended
// generation are dropped; scripts run. Main thread.
func (s *Surface) drainDown() {
	d := &s.down
	d.mu.Lock()
	out := d.out
	d.out = nil
	d.posted = false
	gen := d.gen
	d.mu.Unlock()
	if s.closed.Load() || s.ns == nil {
		return
	}
	// The batches of one round between scripts go in one Eval: the
	// fallback's calls of a generation are contiguous in sequence.
	var items []downq.Item
	for _, b := range out {
		if b.script {
			s.recvSeq(gen, items)
			items = items[:0]
			s.ns.Eval(b.js) // whatever the page: Eval never waited for one
			continue
		}
		if b.gen == gen && gen != 0 {
			items = append(items, b.items...)
		}
	}
	s.recvSeq(gen, items)
}

// recvSeq delivers fallback items of generation gen: one Eval of
// _recvSeq, except that a barrier whose script has not run yet runs at its
// place, in an Eval of its own (so that a script that throws or does not
// parse leaves the items around it alone). Main thread, s.ns set.
func (s *Surface) recvSeq(gen uint64, items []downq.Item) {
	start := 0
	for i, it := range items {
		if it.Kind != downq.Barrier {
			continue
		}
		js, ok := s.takeBarrier(barrierID(it.Data))
		if !ok {
			continue
		}
		if i > start {
			s.ns.Eval(recvSeqJS(gen, items[start:i]))
		}
		s.ns.Eval(js)
		start = i // the barrier item itself goes next: the page advances past it
	}
	if start < len(items) {
		s.ns.Eval(recvSeqJS(gen, items[start:]))
	}
}

// takeBarrier returns the script of barrier id for running it and forgets
// it; false if it ran already or is not of the current generation.
func (s *Surface) takeBarrier(id uint64) (string, bool) {
	d := &s.down
	d.mu.Lock()
	defer d.mu.Unlock()
	js, ok := d.barrier[id]
	delete(d.barrier, id)
	return js, ok
}

// barrierID decodes a barrier item's payload.
func barrierID(p []byte) uint64 {
	if len(p) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(p)
}

// recvSeqJS returns the script handing items of generation gen to the
// page's __tango._recvSeq: [seq, kind, data] with data the message (a JSON
// string literal, jsString), the packet in base64, or the
// barrier id.
func recvSeqJS(gen uint64, items []downq.Item) string {
	var b strings.Builder
	b.WriteString("window.__tango._recvSeq(")
	b.WriteString(strconv.FormatUint(gen, 10))
	b.WriteString(",[")
	for i, it := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('[')
		b.WriteString(strconv.FormatUint(it.Seq, 10))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(int(it.Kind)))
		b.WriteByte(',')
		switch it.Kind {
		case downq.Msg:
			b.WriteString(jsString(string(it.Data)))
		case downq.Packet:
			b.WriteByte('"')
			b.WriteString(base64.StdEncoding.EncodeToString(it.Data))
			b.WriteByte('"')
		default:
			b.WriteString(strconv.FormatUint(barrierID(it.Data), 10))
		}
		b.WriteByte(']')
	}
	b.WriteString("])")
	return b.String()
}

// downReport is an internal message of downChannel.
type downReport struct {
	Op    string `json:"op"`
	Gen   uint64 `json:"gen"`
	ID    uint64 `json:"id"`
	Acked uint64 `json:"acked"`
}

// handleDown handles the page's down:poll reports. Main thread.
func (s *Surface) handleDown(payload string) {
	var r downReport
	if json.Unmarshal([]byte(payload), &r) != nil || s.ns == nil {
		return
	}
	d := &s.down
	d.mu.Lock()
	gen := d.gen
	d.mu.Unlock()
	if r.Gen != gen || gen == 0 {
		return
	}
	switch r.Op {
	case "poll":
		// The page starts polling; the first request makes the queue
		// active (downq.State.Serve). Nothing to do.
	case "fail":
		s.downq.Fail(r.Gen, r.Acked)
		s.drainDown()
	case "barrier":
		// A copy of this barrier the fallback holds goes first: it runs
		// the script itself, at its place.
		s.drainDown()
		if js, ok := s.takeBarrier(r.ID); ok {
			s.ns.Eval(js)
		}
		// Released by an Eval of its own, after the script's: one that
		// throws or does not parse still releases the page.
		s.ns.Eval("window.__tango._release(" + strconv.FormatUint(r.Gen, 10) + "," + strconv.FormatUint(r.ID, 10) + ")")
	}
}
