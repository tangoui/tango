package linux

import "sync"

// handler is the Go side of a C callback. a and b are the C arguments that
// precede user_data (0 when the callback has fewer); the return value goes
// back to C (gboolean for most GTK signals, ignored for void ones).
type handler func(a, b uintptr) uintptr

// dispatchTable maps the ids passed to C as user_data to Go handlers. All
// native callbacks go through a few fixed purego trampolines that look the
// handler up here, so the number of purego callbacks never grows with the
// number of windows, signals or requests.
//
// Ids start at 1 (0 means "no handler") and are never reused within a
// process. The table is safe for concurrent use; handlers run outside the
// lock so they may add or remove entries.
type dispatchTable struct {
	mu   sync.Mutex
	next uintptr
	m    map[uintptr]handler
}

func newDispatchTable() *dispatchTable {
	return &dispatchTable{m: map[uintptr]handler{}}
}

// add registers h and returns its id.
func (t *dispatchTable) add(h handler) uintptr {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	t.m[t.next] = h
	return t.next
}

// lookup returns the handler for id, or nil.
func (t *dispatchTable) lookup(id uintptr) handler {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[id]
}

// call runs the handler for id; an unknown id (already removed, e.g. a late
// signal after teardown) returns 0 and does nothing.
func (t *dispatchTable) call(id, a, b uintptr) uintptr {
	if h := t.lookup(id); h != nil {
		return h(a, b)
	}
	return 0
}

// remove forgets id. Removing an unknown id is a no-op.
func (t *dispatchTable) remove(id uintptr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, id)
}

// size reports the number of live entries (tests, leak checks).
func (t *dispatchTable) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.m)
}
