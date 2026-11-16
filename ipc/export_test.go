package ipc

import "time"

// setJSSendWindow overrides the send window Go uses toward runtime-js (which
// never advertises one; production always uses DefaultWindowBytes) so tests
// can exercise blocking with small frames.
func setJSSendWindow(r *Router, n int) {
	r.mu.Lock()
	r.jsSendWin = n
	r.mu.Unlock()
}

// setGapTimeout overrides how long a stream created on r waits for a
// missing frame before failing with CodeTransport (production always uses
// GapTimeout). Streams already open keep their value.
func setGapTimeout(r *Router, d time.Duration) {
	r.mu.Lock()
	r.gapTimeout = d
	r.mu.Unlock()
}

// gapTimerActive reports whether s has a gap timer armed.
func gapTimerActive(s *Stream) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gapTimer != nil
}

// pageWindow returns the send window new streams on c would get toward the
// page (DefaultWindowBytes when c has no state).
func pageWindow(r *Router, c Conn) int {
	r.mu.Lock()
	cs := r.conns[c]
	r.mu.Unlock()
	if cs == nil {
		return DefaultWindowBytes
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.pageWinLocked()
}
