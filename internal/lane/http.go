package lane

// The up:post lane's HTTP side: a POST per packet on the asset origin. The
// core routes /__tango/lane/up here from its asset handler after its own
// origin check (tango.go, host.ServeAsset). down:poll's endpoint,
// /__tango/lane/poll, is internal/downq's.

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
