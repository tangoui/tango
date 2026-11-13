package background_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/background"
)

func waitFinished(t *testing.T, done func() (bool, bool)) bool {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for time.Now().Before(deadline) {
		if ok, finished := done(); finished {
			return ok
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task not finished")
	return false
}

func TestScheduleAndRun(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	ran := make(chan background.Task, 4)
	background.OnTask(a, func(ctx context.Context, tk background.Task) error {
		ran <- tk
		if tk.Kind == background.Processing {
			return errors.New("failed")
		}
		return nil
	})
	faketest.Start(t, a, fb)

	req := background.Request{Kind: background.Refresh, Earliest: 15 * time.Minute, RequiresNetwork: true}
	if err := background.Schedule(a, req); err != nil {
		t.Fatal(err)
	}
	if r, ok := fb.Scheduled(background.Refresh); !ok || r != req {
		t.Fatal(r, ok)
	}
	if err := background.Schedule(a, background.Request{Kind: background.Processing}); err != nil {
		t.Fatal(err)
	}
	if err := background.Cancel(a, background.Processing); err != nil {
		t.Fatal(err)
	}
	if _, ok := fb.Scheduled(background.Processing); ok {
		t.Fatal("not canceled")
	}

	id := fb.RunBackgroundTask(background.Refresh, time.Minute)
	select {
	case tk := <-ran:
		if tk.Kind != background.Refresh || time.Until(tk.Deadline) <= 0 {
			t.Fatalf("%+v", tk)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("task did not run")
	}
	if !waitFinished(t, func() (bool, bool) { return fb.BackgroundFinished(id) }) {
		t.Fatal("refresh reported failure")
	}
	id2 := fb.RunBackgroundTask(background.Processing, time.Minute)
	<-ran
	if waitFinished(t, func() (bool, bool) { return fb.BackgroundFinished(id2) }) {
		t.Fatal("processing reported success")
	}
}

func TestExpire(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	started := make(chan struct{})
	background.OnTask(a, func(ctx context.Context, tk background.Task) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	faketest.Start(t, a, fb)
	id := fb.RunBackgroundTask(background.Refresh, time.Hour)
	<-started
	fb.ExpireBackgroundTask(id, background.Refresh)
	if waitFinished(t, func() (bool, bool) { return fb.BackgroundFinished(id) }) {
		t.Fatal("expired task reported success")
	}
}

// A task started before OnTask was registered (a background launch) is
// replayed to it.
func TestLateHandler(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	id := fb.RunBackgroundTask(background.Refresh, time.Minute)
	background.OnTask(a, func(ctx context.Context, tk background.Task) error { return nil })
	if !waitFinished(t, func() (bool, bool) { return fb.BackgroundFinished(id) }) {
		t.Fatal("failure")
	}
}

func TestUnsupported(t *testing.T) {
	m, _ := faketest.RunMinimal(t, tango.Options{})
	background.OnTask(m, func(context.Context, background.Task) error { return nil })
	if err := background.Schedule(m, background.Request{Kind: background.Refresh}); !errors.Is(err, background.ErrUnsupported) {
		t.Fatal(err)
	}
}
