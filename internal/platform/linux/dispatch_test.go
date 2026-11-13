package linux

import (
	"sync"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestDispatchTable(t *testing.T) {
	d := newDispatchTable()
	var gotA, gotB uintptr
	id := d.add(func(a, b uintptr) uintptr { gotA, gotB = a, b; return 1 })
	if id == 0 {
		t.Fatal("id 0 is reserved")
	}
	if r := d.call(id, 5, 6); r != 1 || gotA != 5 || gotB != 6 {
		t.Errorf("call = %d (%d, %d)", r, gotA, gotB)
	}
	id2 := d.add(func(a, b uintptr) uintptr { return 2 })
	if id2 == id {
		t.Fatal("ids must be unique")
	}
	d.remove(id)
	if r := d.call(id, 0, 0); r != 0 {
		t.Errorf("removed id returned %d", r)
	}
	d.remove(id) // twice: no-op
	if d.call(9999, 0, 0) != 0 {
		t.Error("unknown id")
	}
	if d.size() != 1 {
		t.Errorf("size = %d", d.size())
	}
	// A handler may modify the table (one-shot sources remove themselves).
	var self uintptr
	self = d.add(func(a, b uintptr) uintptr { d.remove(self); d.add(nil); return 3 })
	if d.call(self, 0, 0) != 3 || d.lookup(self) != nil {
		t.Error("reentrant handler")
	}
}

func TestDispatchTableConcurrent(t *testing.T) {
	d := newDispatchTable()
	var wg sync.WaitGroup
	ids := make(chan uintptr, 800)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := d.add(func(a, b uintptr) uintptr { return a + b })
				if d.call(id, 1, 2) != 3 {
					t.Error("bad call")
				}
				ids <- id
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[uintptr]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = true
		d.remove(id)
	}
	if d.size() != 0 {
		t.Errorf("leaked %d entries", d.size())
	}
}

func TestFocusTracker(t *testing.T) {
	var f focusTracker
	if _, ok := f.settle(); ok {
		t.Error("settle before any focus")
	}
	if ev, ok := f.in(1); !ok || ev != platform.LifecycleForeground {
		t.Fatalf("first focus = %v %v", ev, ok)
	}
	// Moving focus between windows: out(1), in(2) -> no lifecycle change.
	if !f.out(1) {
		t.Error("out of the only focused window should ask for a settle")
	}
	if _, ok := f.in(2); ok {
		t.Error("in(2) while foreground")
	}
	if _, ok := f.settle(); ok {
		t.Error("settle with a focused window")
	}
	// Leaving the app.
	if !f.out(2) {
		t.Error("want settle")
	}
	if ev, ok := f.settle(); !ok || ev != platform.LifecycleBackground {
		t.Errorf("settle = %v %v", ev, ok)
	}
	if _, ok := f.settle(); ok {
		t.Error("Background twice")
	}
	if f.out(2) {
		t.Error("out while background")
	}
	if ev, ok := f.in(1); !ok || ev != platform.LifecycleForeground {
		t.Errorf("refocus = %v %v", ev, ok)
	}
}
