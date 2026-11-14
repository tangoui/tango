package lane

// The scheme lanes' HTTP side: down:scheme is a long-lived streaming GET
// response, up:post a POST per packet, both on the asset origin. The core
// routes /__tango/lane/down and /__tango/lane/up here from its asset
// handler after its own origin check (tango.go, host.ServeAsset).

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
)

// MaxUpBody is the largest up:post request body accepted: a packet holding
// one frame of MaxPostPacket bytes (2 bytes of packet header, 20 of frame
// header). runtime-js keeps whole packets within MaxPostPacket.
const MaxUpBody = MaxPostPacket + 2 + 20

// laneParams reads the gen and tok query parameters.
func laneParams(r *http.Request) (gen uint64, tok string, ok bool) {
	q := r.URL.Query()
	gen, err := strconv.ParseUint(q.Get("gen"), 10, 64)
	tok = q.Get("tok")
	return gen, tok, err == nil && tok != ""
}

// offeredLocked reports whether name is a lane of the current generation
// that has not failed. s.mu held.
func (s *State) offeredLocked(name string) bool {
	st, ok := s.st[name]
	return ok && st != failed
}

// canFlush reports whether w, or a ResponseWriter it wraps (Unwrap, as
// http.ResponseController follows), can flush.
func canFlush(w http.ResponseWriter) bool {
	for w != nil {
		switch u := w.(type) {
		case http.Flusher, interface{ FlushError() error }:
			return true
		case interface{ Unwrap() http.ResponseWriter }:
			w = u.Unwrap()
		default:
			return false
		}
	}
	return false
}

// ServeDown serves GET /__tango/lane/down?gen=&tok=, the down:scheme
// response of the generation gen: an endless application/octet-stream
// body of packets, each flushed as soon as it is written. It starts with
// the empty packet {0, 0} so that the response headers go out at once.
//
// It answers 403 unless gen and tok are the current generation's and it
// offers down:scheme (not failed), 405 to other methods, and 500 when w
// cannot flush (the lane then fails). Only one response serves a
// generation: a later request ends the earlier one. The response ends
// when the request's context is done, the generation ends (Hello, Reset,
// SuspendDown, Close) or down:scheme fails; a write error, and the client
// going away while the response still served the generation, fail
// down:scheme, so that Send stops queueing packets nobody will read (the
// frames then go as messages). w must be usable from the calling goroutine
// for as long as ServeDown runs.
func (s *State) ServeDown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "tango: lane: GET only", http.StatusMethodNotAllowed)
		return
	}
	gen, tok, ok := laneParams(r)
	s.mu.Lock()
	if !ok || !s.tokenLocked(gen, tok) || !s.offeredLocked(DownScheme) || s.down.ended {
		s.mu.Unlock()
		http.Error(w, "tango: lane: refused", http.StatusForbidden)
		return
	}
	if !canFlush(w) {
		s.failLocked(DownScheme)
		s.mu.Unlock()
		http.Error(w, "tango: lane: the response cannot stream", http.StatusInternalServerError)
		return
	}
	q := s.down
	if q.conn != nil {
		close(q.conn) // the previous response ends
	}
	mine := make(chan struct{})
	q.conn = mine
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if q.conn == mine {
			q.conn = nil
		}
		s.mu.Unlock()
	}()

	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte{0, 0}); err != nil {
		s.downBroken(q, mine)
		return
	}
	if rc.Flush() != nil {
		s.downBroken(q, mine)
		return
	}
	ctx := r.Context()
	for {
		s.mu.Lock()
		if q.ended || q.conn != mine {
			s.mu.Unlock()
			return
		}
		// Detach only the packet whose write is now attempted. The tail stays
		// ahead of newer sends, and size includes every in-flight write.
		var p []byte
		if len(q.pkts) > 0 {
			p = q.pkts[0]
			q.pkts[0] = nil
			q.pkts = q.pkts[1:]
		}
		s.mu.Unlock()
		if p == nil {
			select {
			case <-q.wake:
			case <-q.done:
				return
			case <-mine:
				return
			case <-ctx.Done():
				s.downBroken(q, mine)
				return
			}
			continue
		}
		_, err := w.Write(p)
		if err == nil {
			err = rc.Flush()
		}
		s.mu.Lock()
		if !q.ended {
			q.size -= len(p)
		}
		s.mu.Unlock()
		if err != nil {
			s.downBroken(q, mine)
			return
		}
	}
}

// downBroken fails down:scheme when the response that could not write was
// still the one serving the current generation; a response that was
// replaced or whose generation ended leaves the lane alone.
func (s *State) downBroken(q *downQueue, mine chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if q == s.down && q.conn == mine && !q.ended {
		s.failLocked(DownScheme)
	}
}

// ServeUp serves POST /__tango/lane/up?gen=&tok=, one up:post packet: it
// answers 403 unless gen and tok are the current generation's and it
// offers up:post, 413 when the body is longer than MaxUpBody
// (refused before it is read past the limit), 503 when the upstream lane
// is not usable (UpAllowed; runtime-js then fails up:post), and otherwise
// calls deliver with the body, on the calling goroutine, and answers 204.
// deliver must not keep the packet. Packets are not checked here; the
// receiver validates them. Reusable document connections must use
// ServeUpSession and Router.HandleBinarySession instead of this legacy adapter.
func (s *State) ServeUp(w http.ResponseWriter, r *http.Request, deliver func(packet []byte)) {
	s.ServeUpSession(w, r, func(_ uint64, p []byte) { deliver(p) })
}

// ServeUpSession delivers the immutable authorized generation, even if Hello
// rotates the State while the callback is delayed. No callback runs under mu.
func (s *State) ServeUpSession(w http.ResponseWriter, r *http.Request, deliver func(uint64, []byte)) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "tango: lane: POST only", http.StatusMethodNotAllowed)
		return
	}
	gen, tok, ok := laneParams(r)
	s.mu.Lock()
	_, offered := s.st[UpPost] // failed: 503 below
	ok = ok && s.tokenLocked(gen, tok) && offered
	s.mu.Unlock()
	if !ok {
		http.Error(w, "tango: lane: refused", http.StatusForbidden)
		return
	}
	if r.ContentLength > MaxUpBody {
		http.Error(w, "tango: lane: packet too large", http.StatusRequestEntityTooLarge)
		return
	}
	var buf bytes.Buffer
	if r.ContentLength > 0 {
		buf.Grow(int(r.ContentLength))
	}
	if _, err := buf.ReadFrom(io.LimitReader(r.Body, MaxUpBody+1)); err != nil {
		http.Error(w, "tango: lane: reading the packet failed", http.StatusBadRequest)
		return
	}
	if buf.Len() > MaxUpBody {
		http.Error(w, "tango: lane: packet too large", http.StatusRequestEntityTooLarge)
		return
	}
	// The generation may have ended while the body was read. An empty
	// packet (the up:post start probe) is checked but not counted for
	// fault injection.
	s.mu.Lock()
	ok = s.tokenLocked(gen, tok) && s.upAllowedLocked(!emptyPacket(buf.Bytes()))
	s.mu.Unlock()
	if !ok {
		http.Error(w, "tango: lane: upstream lane not available", http.StatusServiceUnavailable)
		return
	}
	deliver(gen, buf.Bytes())
	w.WriteHeader(http.StatusNoContent)
}
