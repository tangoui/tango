package background_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/background"
)

// A panicking task still fails the task towards the system and is
// reported to OnPanic as "background task"; the app keeps running.
func TestTaskPanic(t *testing.T) {
	var mu sync.Mutex
	var where []string
	a, fb := faketest.NewApp(tango.Options{OnPanic: func(p tango.Panic) bool {
		mu.Lock()
		where = append(where, p.Where)
		mu.Unlock()
		return true
	}})
	background.OnTask(a, func(context.Context, background.Task) error { panic("boom") })
	faketest.Start(t, a, fb)

	id := fb.RunBackgroundTask(background.Refresh, time.Minute)
	if waitFinished(t, func() (bool, bool) { return fb.BackgroundFinished(id) }) {
		t.Fatal("panicked task reported success")
	}
	if !tango.Await(a, func() bool { return true }) {
		t.Fatal("app stopped")
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(where, []string{"background task"}) {
		t.Fatalf("where %q", where)
	}
}
