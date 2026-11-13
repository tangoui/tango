package devhost

import (
	"errors"
	"fmt"
	"image"
	"sync"
	"time"
)

// Frame transport. Each window has one segment per worker holding two
// slots (double buffering). The worker writes a frame into a slot the host
// does not hold, then sends FrameReady; the host presents it and sends
// FrameAck for the slot it held before (it keeps the slot on screen until
// a newer frame replaces it, so it can redraw the error banner over it).
// When a frame no longer fits, the worker creates a bigger segment and
// sends ShmAttach first; the old one stays mapped in the host until it is
// done with it, so growing never waits.

// SlotTimeout bounds how long Write waits for the host to release a slot.
var SlotTimeout = time.Second

// ErrNoSlot is returned by Write when the host held both slots too long.
var ErrNoSlot = errors.New("devhost: no free frame slot")

// FrameWriter is the worker side of one window's frame transport.
type FrameWriter struct {
	win  int
	dir  string
	send func(any) error

	mu       sync.Mutex
	cond     *sync.Cond
	seg      *Segment
	gen      int
	slotSize int
	busy     [2]bool
	next     int
	closed   bool
}

// NewFrameWriter returns a writer for window win; segments are created in
// dir and announced with send.
func NewFrameWriter(win int, dir string, send func(any) error) *FrameWriter {
	w := &FrameWriter{win: win, dir: dir, send: send}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// Write copies img (premultiplied RGBA, device pixels) into a free slot and
// sends FrameReady.
func (w *FrameWriter) Write(img *image.RGBA, scale float32) error {
	r := img.Bounds()
	width, height := r.Dx(), r.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	stride := width * 4
	need := stride * height
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("devhost: frame writer closed")
	}
	if w.seg == nil || need > w.slotSize {
		slot := roundUp(need+need/4, 1<<16) // headroom for live resize
		seg, err := CreateSegment(w.dir, 2*slot)
		if err != nil {
			return err
		}
		if w.seg != nil {
			w.seg.Close()
			w.seg.Unlink()
		}
		w.gen++
		w.seg, w.slotSize, w.busy, w.next = seg, slot, [2]bool{}, 0
		if err := w.send(ShmAttach{Win: w.win, Seg: w.gen, Name: seg.Name, Size: 2 * slot}); err != nil {
			return err
		}
	}
	s := w.next
	if w.busy[s] {
		s ^= 1
	}
	if w.busy[s] {
		deadline := time.Now().Add(SlotTimeout)
		for w.busy[0] && w.busy[1] && !w.closed {
			if !w.waitUntil(deadline) {
				return ErrNoSlot
			}
		}
		if w.closed {
			return errors.New("devhost: frame writer closed")
		}
		s = w.next
		if w.busy[s] {
			s ^= 1
		}
	}
	dst := w.seg.Bytes()[s*w.slotSize : s*w.slotSize+need]
	off := img.PixOffset(r.Min.X, r.Min.Y)
	if img.Stride == stride {
		copy(dst, img.Pix[off:off+need])
	} else {
		for y := range height {
			o := off + y*img.Stride
			copy(dst[y*stride:(y+1)*stride], img.Pix[o:o+stride])
		}
	}
	w.busy[s] = true
	w.next = s ^ 1
	return w.send(FrameReady{Win: w.win, Seg: w.gen, Slot: s, W: width, H: height, Stride: stride, Scale: scale})
}

// waitUntil waits for a signal or the deadline; it reports false when the
// deadline passed. w.mu held.
func (w *FrameWriter) waitUntil(deadline time.Time) bool {
	d := time.Until(deadline)
	if d <= 0 {
		return false
	}
	t := time.AfterFunc(d, func() { w.mu.Lock(); w.cond.Broadcast(); w.mu.Unlock() })
	w.cond.Wait()
	t.Stop()
	return time.Now().Before(deadline)
}

// Ack releases a slot. Any goroutine.
func (w *FrameWriter) Ack(seg, slot int) {
	w.mu.Lock()
	if seg == w.gen && slot >= 0 && slot < 2 {
		w.busy[slot] = false
		w.cond.Broadcast()
	}
	w.mu.Unlock()
}

// Close unmaps the current segment.
func (w *FrameWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	if w.seg != nil {
		w.seg.Close()
		w.seg.Unlink()
		w.seg = nil
	}
	w.cond.Broadcast()
}

// FrameImage returns the frame f inside seg (no copy). The slot size is
// half the segment.
func FrameImage(seg *Segment, f FrameReady) (*image.RGBA, error) {
	data := seg.Bytes()
	slot := len(data) / 2
	n := f.Stride * f.H
	if f.Slot < 0 || f.Slot > 1 || f.W <= 0 || f.H <= 0 || f.Stride < f.W*4 || n > slot {
		return nil, fmt.Errorf("devhost: bad frame %+v for segment of %d bytes", f, len(data))
	}
	pix := data[f.Slot*slot : f.Slot*slot+n]
	return &image.RGBA{Pix: pix, Stride: f.Stride, Rect: image.Rect(0, 0, f.W, f.H)}, nil
}

func roundUp(n, to int) int { return (n + to - 1) / to * to }
