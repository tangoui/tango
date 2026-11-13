package single

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

// A second instance reaches every handler even when one panics; each
// panic goes to OnPanic as "event handler".
func TestSecondInstancePanic(t *testing.T) {
	if mobile {
		t.Skip("no single instance on mobile")
	}
	var mu sync.Mutex
	var where []string
	a, _ := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	got := make(chan []string, 1)
	OnSecondInstance(a, func([]string, string) { panic("boom") })
	OnSecondInstance(a, func(args []string, _ string) { got <- args })
	key := uniqueKey(t)
	primary, err := Claim(a, Options{
		Key:              key,
		NoFocus:          true,
		Args:             []string{},
		OnSecondInstance: func([]string, string) { panic("option") },
	})
	if err != nil || !primary {
		t.Fatal(primary, err)
	}
	if err := send(name(key), []byte(`{"args":["x"],"cwd":"/"}`), timeout); err != nil {
		t.Fatal(err)
	}
	select {
	case args := <-got:
		if !slices.Equal(args, []string{"x"}) {
			t.Fatal(args)
		}
	case <-time.After(timeout):
		t.Fatal("last handler not called")
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"event handler", "event handler"}) {
		t.Fatalf("where %q", where)
	}
}
