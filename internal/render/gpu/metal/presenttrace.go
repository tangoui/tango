package metal

import "sync/atomic"

// presentationRecord is platform presentation feedback, not command-buffer
// completion and not physical scanout. Host times are uptime nanoseconds.
type presentationRecord struct {
	Session     uint64 `json:"session"`
	Present     uint64 `json:"present"`
	Frame       uint64 `json:"renderFrame"`
	Drawable    uint64 `json:"drawable"`
	SubmittedNS int64  `json:"submittedNS"`
	PresentedNS int64  `json:"presentedNS"`
	CallbackNS  int64  `json:"callbackNS"`
	Status      string `json:"status"`
	Dropped     uint64 `json:"dropped"`
}

// A native block captures only this recorder and a value token, never Device.
// The sink is process-lifetime and bounded; no per-window consumer goroutine is
// kept alive by missing callbacks. Closing doesn't close a callback's channel.
type presentationTrace struct {
	session uint64
	next    atomic.Uint64
	closed  atomic.Bool
	dropped atomic.Uint64
	out     chan<- presentationRecord
}

func newPresentationTrace(session uint64, out chan<- presentationRecord) *presentationTrace {
	return &presentationTrace{session: session, out: out}
}
func (t *presentationTrace) begin(frame, drawable uint64, host int64) presentationRecord {
	return presentationRecord{Session: t.session, Present: t.next.Add(1), Frame: frame, Drawable: drawable, SubmittedNS: host}
}
func (t *presentationTrace) emit(r presentationRecord) {
	r.Dropped = t.dropped.Load()
	select {
	case t.out <- r:
	default:
		t.dropped.Add(1)
	}
}
func (t *presentationTrace) finish(r presentationRecord, presented, callback int64) {
	r.PresentedNS, r.CallbackNS = presented, callback
	r.Status = "presented"
	if presented <= 0 {
		r.Status = "skipped"
	}
	if t.closed.Load() {
		r.Status = "late"
	}
	t.emit(r)
}
func (t *presentationTrace) close() {
	if !t.closed.Swap(true) {
		t.emit(presentationRecord{Session: t.session, Status: "closed"})
	}
}
