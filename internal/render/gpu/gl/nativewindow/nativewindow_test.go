package nativewindow

import "testing"

type user struct {
	h     *Holder
	calls int
	seen  []uintptr // the holder's window at each DetachWindow
}

func (u *user) DetachWindow() {
	u.calls++
	win, _, _, _ := u.h.Window()
	u.seen = append(u.seen, win)
}

func TestHolderLifecycle(t *testing.T) {
	h := New()
	if h.Handle() == 0 || Lookup(h.Handle()) != h {
		t.Fatalf("handle %d not registered", h.Handle())
	}
	u := &user{h: h}
	h.SetUser(u)

	h.SetWindow(0x1000, 1080, 2400)
	win, w, hh, gen := h.Window()
	if win != 0x1000 || w != 1080 || hh != 2400 || gen != 1 {
		t.Fatalf("Window = %#x %d×%d gen %d", win, w, hh, gen)
	}
	if u.calls != 1 { // detach with no window yet is harmless
		t.Fatalf("detach calls %d, want 1", u.calls)
	}

	// Rotation: same window, new size, no detach, same generation.
	h.SetWindow(0x1000, 2400, 1080)
	if _, w, _, gen = h.Window(); w != 2400 || gen != 1 || u.calls != 1 {
		t.Fatalf("resize: w %d gen %d calls %d", w, gen, u.calls)
	}

	// Background: detached while the old window is still recorded.
	h.ClearWindow()
	if win, _, _, gen = h.Window(); win != 0 || gen != 2 || u.calls != 2 || u.seen[1] != 0x1000 {
		t.Fatalf("clear: win %#x gen %d calls %d seen %v", win, gen, u.calls, u.seen)
	}

	// Foreground with a new window.
	h.SetWindow(0x2000, 1080, 2400)
	if win, _, _, gen = h.Window(); win != 0x2000 || gen != 3 {
		t.Fatalf("new window: %#x gen %d", win, gen)
	}

	// A CPU frame: detaches and marks; a new window clears the mark.
	if !h.UseCPU() || !h.CPU() || u.calls != 4 {
		t.Fatalf("UseCPU: cpu %v calls %d", h.CPU(), u.calls)
	}
	h.SetUser(nil)
	if h.UseCPU() {
		t.Fatal("UseCPU reported a user after SetUser(nil)")
	}
	h.SetWindow(0x3000, 10, 10)
	if h.CPU() {
		t.Fatal("cpu mark kept for a new window")
	}

	h.SetUser(u)
	h.Close()
	if !h.Closed() || Lookup(h.Handle()) != nil || u.calls != 5 {
		t.Fatalf("Close: closed %v lookup %v calls %d", h.Closed(), Lookup(h.Handle()), u.calls)
	}
	h.SetWindow(0x4000, 1, 1)
	h.Close()
	if win, _, _, _ = h.Window(); win != 0 || u.calls != 5 {
		t.Fatalf("after Close: win %#x calls %d", win, u.calls)
	}
}

func TestHandlesNotReused(t *testing.T) {
	a := New()
	a.Close()
	b := New()
	defer b.Close()
	if a.Handle() == b.Handle() || Lookup(a.Handle()) != nil {
		t.Fatalf("handles %d, %d", a.Handle(), b.Handle())
	}
}
