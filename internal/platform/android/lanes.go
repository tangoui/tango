package android

// IPC fast lanes, the pure-Go part (no build tag, so its tests run on the
// host too); the surface methods and JNI wrappers are in lanes_android.go.
//
// down:android and up:android carry the core's lane packets
// (count:u16 | (streamID:u64 seq:u64 len:u32 payload)*) as ArrayBuffers
// over the page's WebMessageListener channel (Page.java), next to the
// string messages of the bridge.
//
// Up (page → Java → Go, main thread): token[16] | gen:u64 LE | packet. The
// token is the document's page token in its 16-byte form (rawToken,
// token.go); it must be the current or pending token and have been issued
// to the listener's source origin. gen is the lane generation of the
// page's hello; the core drops packets of any other generation. A packet
// whose token or origin does not match is dropped without a reply.
//
// The probe is exactly token | gen | 00 00 (26 bytes). When the core
// accepts it, Java keeps the probe's reply proxy for the downstream lane
// and answers [0]; Go records gen as the proxy's generation (laneState).
// A packet or probe the core refuses is answered [1]. Each probe gets
// exactly one reply.
//
// Down (Go → Java → page): the core's SendLanePacket queues (gen, p)
// (laneState.push, any goroutine, up to laneQueueMax unsent bytes) and
// wakes the main thread, whose drain posts the packets of the recorded
// proxy generation through the proxy (Page.laneSend) and drops the rest
// (laneState.flush).

import (
	"encoding/binary"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/lane"
)

const (
	laneQueueMax  = 8 << 20                   // unsent down bytes per page
	laneUpHeader  = tokenLen/2 + 8            // token[16] | gen:u64
	laneProbeSize = laneUpHeader + 2          // header | 00 00
	laneSwitchEnv = "TANGO_IPC_LANES_ANDROID" // read when a page is created
)

// laneCapsFromEnv returns the lanes a new page reports: down:android and
// up:android when the WebView can carry ArrayBuffers over a
// WebMessageListener (supported: Tango.laneSupported) and
// TANGO_IPC_LANES_ANDROID, read through getenv, is on. Unset, empty, "0",
// "false" and "off" (any case, surrounding spaces ignored) are off; any
// other value is on. TANGO_IPC_LANES=0 and Options.DisableFastLanes are
// the core's.
func laneCapsFromEnv(getenv func(string) string, supported bool) []string {
	if !supported {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(getenv(laneSwitchEnv))) {
	case "", "0", "false", "off":
		return nil
	}
	return []string{lane.DownAndroid, lane.UpAndroid}
}

// parseUp checks an upstream lane message from a main-frame
// WebMessageListener message that the WebView says came from sourceOrigin.
// ok is false when the message is too short, its token is neither the
// current nor the pending one, or the token was issued to another origin.
// origin is the token's document's; packet is the lane packet after the
// header (it aliases data); probe says whether the message is a probe.
func parseUp(d *docTokens, sourceOrigin string, data []byte) (origin string, gen uint64, packet []byte, probe bool, ok bool) {
	if len(data) < laneProbeSize {
		return "", 0, nil, false, false
	}
	origin, ok = d.originRaw(data[:tokenLen/2])
	if !ok || !sameOrigin(origin, sourceOrigin) {
		return "", 0, nil, false, false
	}
	gen = binary.LittleEndian.Uint64(data[tokenLen/2 : laneUpHeader])
	packet = data[laneUpHeader:]
	probe = len(data) == laneProbeSize && packet[0] == 0 && packet[1] == 0
	return origin, gen, packet, probe, true
}

// handleUp handles an upstream lane message (Tango.nativeLanePacket, main
// thread): deliver hands the packet to the core (LaneHost.OnLanePacket)
// and reports whether it took it. The result tells Java what to do:
//
//	0: token or origin mismatch, dropped; no reply
//	1: delivered; for a probe Go recorded its generation, and Java keeps
//	   the reply proxy and answers [0]
//	2: the core refused it; Java answers [1]
func handleUp(st *laneState, d *docTokens, src string, data []byte, deliver func(origin string, gen uint64, p []byte) bool) (code int) {
	laneCount(&laneStats.upPackets, 1)
	origin, gen, p, probe, ok := parseUp(d, src, data)
	if !ok {
		laneCount(&laneStats.upBadToken, 1)
		return 0
	}
	if probe {
		laneCount(&laneStats.upProbes, 1)
	}
	if !deliver(origin, gen, p) {
		laneCount(&laneStats.upRefused, 1)
		return 2
	}
	if probe {
		st.probedBy(gen, data[:tokenLen/2])
	}
	return 1
}

// beginPage starts a new main-frame document at rawURL
// (Tango.nativeBeginPage): docTokens.begin, then laneState.begin. keep
// tells Java whether to keep its reply proxy.
func beginPage(d *docTokens, st *laneState, rawURL string) (inject string, keep bool) {
	inject, promoted := d.beginDoc(rawURL)
	p := rawToken(promoted)
	return inject, st.begin(p != nil, p != nil && st.probedWith(p))
}

// queued is one down packet and the lane generation it was sent in.
type queued struct {
	gen uint64
	p   []byte
}

// laneState is one page's lane state: the down queue and the generation of
// the reply proxy Java keeps (that of the last accepted probe). Its methods
// may be called from any goroutine; the core calls push under its lane
// lock, so laneState never calls out while holding mu.
type laneState struct {
	mu       sync.Mutex
	q        []queued
	bytes    int  // unsent bytes in q
	closed   bool // the page is gone (release)
	hasProxy bool // a probe was accepted since the document started
	proxyGen uint64
	probeTok []byte // the token of that probe (16 bytes)
}

func newLaneState() *laneState { return &laneState{} }

// push queues p, sent in generation gen, for the main thread. It is false
// (p not kept) once the page is gone or when the queue would exceed
// laneQueueMax unsent bytes.
func (st *laneState) push(gen uint64, p []byte) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed || st.bytes+len(p) > laneQueueMax {
		return false
	}
	st.q = append(st.q, queued{gen, p})
	st.bytes += len(p)
	laneCount(&laneStats.downQueued, 1)
	return true
}

// take removes and returns the queued packets.
func (st *laneState) take() []queued {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.takeLocked()
}

func (st *laneState) takeLocked() []queued {
	q := st.q
	st.q, st.bytes = nil, 0
	laneCount(&laneStats.downQueued, -int64(len(q)))
	return q
}

// flush posts the queued packets of the proxy generation with send (which
// reports whether Java posted it) and drops the others. Main thread; send
// runs without mu held.
func (st *laneState) flush(send func(p []byte) bool) {
	st.mu.Lock()
	q := st.takeLocked()
	gen, ok := st.proxyGen, st.hasProxy
	st.mu.Unlock()
	var sent, dropped int64
	for _, e := range q {
		if ok && e.gen == gen && send(e.p) {
			sent++
		} else {
			dropped++
		}
	}
	laneCount(&laneStats.downPackets, sent)
	laneCount(&laneStats.downDropped, dropped)
}

// pending reports whether packets are queued.
func (st *laneState) pending() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.q) > 0
}

// probed records gen as the generation of Java's reply proxy: a probe of
// generation gen was accepted.
func (st *laneState) probed(gen uint64) { st.probedBy(gen, nil) }

// probedBy is probed for a probe that carried token tok (16 bytes).
func (st *laneState) probedBy(gen uint64, tok []byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.hasProxy, st.proxyGen = true, gen
	st.probeTok = append(st.probeTok[:0], tok...)
}

// probedWith reports whether the accepted probe carried token tok.
func (st *laneState) probedWith(tok []byte) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.hasProxy && len(tok) > 0 && string(st.probeTok) == string(tok)
}

// begin is called when a main-frame document starts: the proxy generation
// is forgotten (Java drops the proxy too) unless this begin promoted the
// pending token to current (promotedPending) and the accepted probe was
// made with that token (pendingProbed): the new document's probe reached
// the main thread before its own onPageStarted. It reports whether the
// proxy is kept. Queued packets stay; flush drops those of other
// generations.
func (st *laneState) begin(promotedPending bool, pendingProbed bool) (keep bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if promotedPending && pendingProbed && st.hasProxy {
		return true
	}
	st.hasProxy, st.proxyGen, st.probeTok = false, 0, nil
	return false
}

// close drops the queue; push fails from now on. Any goroutine.
func (st *laneState) close() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.closed = true
	st.takeLocked()
	st.hasProxy, st.proxyGen, st.probeTok = false, 0, nil
}

// ---------------------------------------------------------------------------
// Lane counters (test hook): counted only in builds with the self-test
// hooks (testHooks, testhooks_on.go); always zero otherwise.

// LaneStats are the Android lane counters, process-wide.
type LaneStats struct {
	UpPackets   int64 // upstream lane messages that reached Go (all of them)
	UpProbes    int64 // of them, probes with a valid token
	UpRefused   int64 // of them, refused by the core (answered [1])
	UpBadToken  int64 // of them, dropped for a token or origin mismatch
	DownPackets int64 // down packets posted to the page
	DownDropped int64 // down packets dropped: other generation, no proxy, or the post failed
	DownQueued  int64 // down packets queued now, not yet posted or dropped
}

// LaneStatsSnapshot returns the lane counters (all zero in builds without
// the self-test hooks). Any goroutine.
func LaneStatsSnapshot() LaneStats {
	c := &laneStats
	return LaneStats{
		UpPackets: c.upPackets.Load(), UpProbes: c.upProbes.Load(), UpRefused: c.upRefused.Load(),
		UpBadToken: c.upBadToken.Load(), DownPackets: c.downPackets.Load(), DownDropped: c.downDropped.Load(),
		DownQueued: c.downQueued.Load(),
	}
}

var laneStats struct {
	upPackets, upProbes, upRefused, upBadToken, downPackets, downDropped, downQueued atomic.Int64
}

func laneCount(c *atomic.Int64, n int64) {
	if testHooks && n != 0 {
		c.Add(n)
	}
}
