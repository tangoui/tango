package terminal

import (
	"bytes"
	"time"

	"github.com/tangoui/tango/ui/terminal/vt"
)

const echoLimit = 32
const echoTimeout = time.Second

// echoState tracks a short run of printable input at the end of a line.
// The first byte must be echoed by the program before we show later bytes.
// Authoritative output always goes directly to the emulator.
type echoState struct {
	x, y      int
	queue     [echoLimit]byte
	n         int
	confirmed bool
	deadline  time.Time
}

func (e *echoState) expire(now time.Time) {
	if e.n != 0 && !now.Before(e.deadline) {
		*e = echoState{}
	}
}

func (e *echoState) input(v *vt.Terminal, b []byte, typed bool, now time.Time) {
	e.expire(now)
	x, y, ok := v.EchoCursor()
	if !typed || !ok || len(b) != 1 || b[0] < ' ' || b[0] > '~' ||
		e.n == len(e.queue) || (e.confirmed || e.n != 0) && (e.x != x || e.y != y) {
		*e = echoState{}
		return
	}
	cells := v.Screen().Line(y).Cells
	// Stay away from the wrap boundary and occupied cells: applications
	// perform their own line editing and may redraw rather than append.
	if x+e.n+1 >= len(cells) || cells[x+e.n].Rune != 0 || cells[x+e.n].Width != 1 {
		*e = echoState{}
		return
	}
	e.x, e.y = x, y
	if e.n == 0 {
		e.deadline = now.Add(echoTimeout)
	}
	e.queue[e.n] = b[0]
	e.n++
}

func (e *echoState) output(v *vt.Terminal, b []byte, now time.Time) {
	e.expire(now)
	if e.n == 0 || len(b) > e.n || !bytes.Equal(b, e.queue[:len(b)]) {
		*e = echoState{}
		return
	}
	x, y, ok := v.EchoCursor()
	if !ok || x != e.x+len(b) || y != e.y {
		*e = echoState{}
		return
	}
	e.confirmed, e.x = true, x
	e.n = copy(e.queue[:], e.queue[len(b):e.n])
}

type echoPreview struct {
	text string
	x, y int
}

func (e *echoState) preview(v *vt.Terminal, now time.Time) (echoPreview, time.Duration) {
	e.expire(now)
	x, y, ok := v.EchoCursor()
	if !ok || x != e.x || y != e.y {
		*e = echoState{}
	}
	if e.n == 0 || !e.confirmed {
		return echoPreview{}, 0
	}
	return echoPreview{text: string(e.queue[:e.n]), x: x, y: y}, e.deadline.Sub(now)
}

// SetPredictiveEcho enables or disables speculative display for this
// terminal. It is off by default. After a real echo confirms input on the
// current line, the view can preview single printable ASCII keystrokes
// until the program echoes them. Other input, unexpected output and a
// timeout discard the preview. The setting is safe to change concurrently.
func (t *Terminal) SetPredictiveEcho(on bool) {
	t.mu.Lock()
	if t.echoEnabled == on {
		t.mu.Unlock()
		return
	}
	t.echo = echoState{}
	t.echoEnabled = on
	invalidate := t.invalidate
	t.mu.Unlock()
	if invalidate != nil {
		invalidate()
	}
}

func (t *Terminal) predicting() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.echoEnabled
}

func (t *Terminal) sendInputLocked(b []byte, typed bool) SendResult {
	result := t.queueInput(b)
	if t.echoEnabled {
		t.dropStaleEcho()
		if result.AcceptedBytes == len(b) {
			t.echo.input(t.vt, b, typed, time.Now())
		} else {
			t.echo = echoState{}
		}
	}
	return result
}

func (t *Terminal) dropStaleEcho() {
	if t.echoReset.Swap(false) {
		t.echo = echoState{}
	}
}

func (w *ViewW) sendInput(b []byte, typed bool) {
	w.t.Lock()
	w.t.sendInputLocked(b, typed)
	w.t.Unlock()
}
