package downq

// The poll endpoint: GET /__tango/lane/poll?gen=&tok=&ack= on the asset
// origin, routed here by the core after its own origin check, and the
// batch format of its responses (big-endian):
//
//	batch := firstSeq:u64 | count:u32 | item*count
//	item  := kind:u8 | len:u32 | payload[len]

import (
	"encoding/binary"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const (
	batchHeader = 8 + 4 // firstSeq, count
	itemHeader  = 1 + 4 // kind, len
)

// Serve serves one poll request of the page: GET with the query
// parameters gen and tok (the generation's, from Start) and ack (the
// largest sequence number the page delivered). It answers 405 to other
// methods, 403 unless gen and tok are the current generation's (tok
// compared in constant time; nothing changes then), 400 to a malformed
// ack, and 410 Gone when the generation has ended or failed.
//
// Otherwise the request acknowledges the items up to ack and makes the
// generation poll (see Active; it also stops the watchdog). When items
// are not acknowledged or pending it answers 200 at once with a batch of
// them (unacknowledged ones again first); else it holds until an item is
// pushed (then a batch), HoldMax passes (an empty batch), the generation
// ends or fails (410), or r's context is done (no answer). The response
// has Content-Type application/octet-stream, X-Content-Type-Options
// nosniff and Cache-Control no-store. Items written stay until a later
// ack. w must be usable from the calling goroutine for as long as Serve
// runs.
func (s *State) Serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "tango: downq: GET only", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	gen, genErr := strconv.ParseUint(q.Get("gen"), 10, 64)
	tok := q.Get("tok")
	ack, ackErr := strconv.ParseUint(q.Get("ack"), 10, 64)

	s.mu.Lock()
	if genErr != nil || tok == "" || !s.tokenLocked(gen, tok) {
		s.mu.Unlock()
		http.Error(w, "tango: downq: refused", http.StatusForbidden)
		return
	}
	if ackErr != nil {
		s.mu.Unlock()
		http.Error(w, "tango: downq: bad ack", http.StatusBadRequest)
		return
	}
	if s.ended || s.failed {
		s.mu.Unlock()
		http.Error(w, "tango: downq: generation ended", http.StatusGone)
		return
	}
	s.ackLocked(ack)
	s.active, s.watchdogged = true, false
	s.stopDogLocked()
	s.reqs++
	done := s.done
	s.mu.Unlock()
	defer s.leave()

	var hold *time.Timer
	expired := false
	ctx := r.Context()
	for {
		s.mu.Lock()
		if s.gen != gen || s.ended || s.failed {
			s.mu.Unlock()
			http.Error(w, "tango: downq: generation ended", http.StatusGone)
			return
		}
		first, items := s.takeLocked()
		cut := s.faultCutLocked(items)
		wake := s.wake
		s.mu.Unlock()
		if cut >= 0 {
			writeCutBatch(w, first, items, cut)
			return
		}
		if len(items) > 0 || expired {
			writeBatch(w, first, items)
			return
		}
		if hold == nil {
			hold = time.NewTimer(HoldMax)
			defer hold.Stop()
		}
		select {
		case <-wake:
		case <-hold.C:
			expired = true
		case <-done:
			http.Error(w, "tango: downq: generation ended", http.StatusGone)
			return
		case <-ctx.Done():
			return
		}
	}
}

// leave ends a poll request: once none is in progress, pending items arm
// the watchdog.
func (s *State) leave() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs--
	if s.reqs == 0 && len(s.queue) > 0 && s.pollingLocked() {
		s.armDogLocked()
	}
}

// writeBatch answers 200 with the batch of items starting at first.
func writeBatch(w http.ResponseWriter, first uint64, items []Item) {
	body := EncodeBatch(first, items)
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	w.Write(body) // a failed write leaves the items unacknowledged
}

// writeCutBatch is writeBatch cut off in the middle of items[cut] (fault
// injection, fault.go): the head announces the whole batch, the body ends
// early. The items stay unacknowledged.
func writeCutBatch(w http.ResponseWriter, first uint64, items []Item, cut int) {
	body := EncodeBatch(first, items)
	n := batchHeader
	for _, it := range items[:cut] {
		n += itemHeader + len(it.Data)
	}
	n += itemHeader + len(items[cut].Data)/2
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	w.Write(body[:n])
}

// EncodeBatch returns the batch of items whose first item has sequence
// number firstSeq; the items' Seq fields are not written (the page counts
// from firstSeq).
func EncodeBatch(firstSeq uint64, items []Item) []byte {
	n := batchHeader
	for _, it := range items {
		n += itemHeader + len(it.Data)
	}
	b := make([]byte, 0, n)
	b = binary.BigEndian.AppendUint64(b, firstSeq)
	b = binary.BigEndian.AppendUint32(b, uint32(len(items)))
	for _, it := range items {
		b = append(b, byte(it.Kind))
		b = binary.BigEndian.AppendUint32(b, uint32(len(it.Data)))
		b = append(b, it.Data...)
	}
	return b
}

// DecodeBatch parses a batch made by EncodeBatch: the items get sequence
// numbers firstSeq, firstSeq+1, ... and Data slices of b. It fails on a
// short or over-long batch and on an unknown kind.
func DecodeBatch(b []byte) (firstSeq uint64, items []Item, err error) {
	if len(b) < batchHeader {
		return 0, nil, errors.New("downq: batch header cut short")
	}
	firstSeq = binary.BigEndian.Uint64(b)
	count := binary.BigEndian.Uint32(b[8:])
	b = b[batchHeader:]
	if uint64(count) > uint64(len(b)/itemHeader) {
		return 0, nil, errors.New("downq: batch shorter than its count")
	}
	items = make([]Item, 0, count)
	for i := range uint64(count) {
		if len(b) < itemHeader {
			return 0, nil, errors.New("downq: item header cut short")
		}
		kind := Kind(b[0])
		if kind > Barrier {
			return 0, nil, errors.New("downq: unknown item kind " + strconv.Itoa(int(kind)))
		}
		n := binary.BigEndian.Uint32(b[1:])
		b = b[itemHeader:]
		if uint64(n) > uint64(len(b)) {
			return 0, nil, errors.New("downq: item payload cut short")
		}
		items = append(items, Item{Seq: firstSeq + i, Kind: kind, Data: b[:n:n]})
		b = b[n:]
	}
	if len(b) != 0 {
		return 0, nil, errors.New("downq: trailing bytes after the batch")
	}
	return firstSeq, items, nil
}
