package menu_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/menu"
)

// A panicking OnClick goes to OnPanic as "menu item"; the app keeps
// running and later clicks still work.
func TestClickPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.Run(t, tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	ok := 0
	m := menu.New()
	file := m.Submenu("File")
	file.Add("Boom", func() { panic("boom") })
	file.Add("Ok", func() { ok++ })
	if err := menu.SetAppMenu(a, m); err != nil {
		t.Fatal(err)
	}
	items := fb.Menu(0)
	if !fb.ClickMenu(0, find(items, "Boom").ID) || !fb.ClickMenu(0, find(items, "Ok").ID) {
		t.Fatal("clicks not delivered")
	}
	n := tango.Await(a, func() int { return ok })
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"menu item"}) || n != 1 {
		t.Fatalf("where %q, ok %d", where, n)
	}
}
