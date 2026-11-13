package notify_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/notify"
)

func TestSendAndClick(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	clicked := make(chan string, 1)
	notify.OnClick(a, func(id string) { clicked <- id })
	id, err := notify.Send(a, notify.Message{Title: "T", Subtitle: "S", Body: "B", Silent: true})
	if err != nil || !strings.HasPrefix(id, "tango-") {
		t.Fatalf("%q %v", id, err)
	}
	id2, _ := notify.Send(a, notify.Message{Title: "second"})
	if id2 == id {
		t.Fatal("ids must differ")
	}
	ns := fb.Notifications()
	if len(ns) != 2 || ns[0].ID != id || ns[0].Title != "T" || ns[0].Subtitle != "S" || ns[0].Body != "B" || !ns[0].Silent {
		t.Fatalf("%+v", ns)
	}
	if !fb.ClickNotification(id) {
		t.Fatal("click not delivered")
	}
	select {
	case got := <-clicked:
		if got != id {
			t.Fatalf("clicked %q", got)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no click")
	}
}

// A click on a notification of an earlier run that launched the app
// reaches OnClick handlers registered before Run.
func TestLaunchClick(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	clicked := make(chan string, 1)
	notify.OnClick(a, func(id string) { clicked <- id })
	faketest.Start(t, a, fb)
	fb.NotificationOpened("tango-old-1")
	select {
	case got := <-clicked:
		if got != "tango-old-1" {
			t.Fatalf("clicked %q", got)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no launch click")
	}
}

// On a platform that cannot pump native events while the main thread
// waits, a main-thread request fails at once instead of hanging.
func TestPermissionNoNestedLoop(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.NoNestedLoop = true
	faketest.Start(t, a, fb)
	err := tango.Await(a, func() error { _, err := notify.RequestPermission(a); return err })
	if !errors.Is(err, tango.ErrNestedWait) {
		t.Fatal(err)
	}
	if _, err := notify.RequestPermission(a); err != nil { // off the main thread it works
		t.Fatal(err)
	}
}

func TestPermission(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.SetNotifyPermission(false, nil)
	faketest.Start(t, a, fb)
	ok, err := notify.RequestPermission(a)
	if ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
	// From the main thread too (AwaitAsync pumps events).
	got := tango.Await(a, func() bool { ok, _ := notify.RequestPermission(a); return ok })
	if got || fb.NotifyPermissionRequests() != 2 {
		t.Fatalf("%v %d", got, fb.NotifyPermissionRequests())
	}
}

func TestErrors(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	boom := errors.New("not bundled")
	fb.SetNotifyErr(boom)
	if _, err := notify.Send(a, notify.Message{Title: "x"}); err != boom {
		t.Fatal(err)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := notify.Send(m, notify.Message{}); !errors.Is(err, notify.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := notify.RequestPermission(m); !errors.Is(err, notify.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestAfterQuit(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	a.Quit()
	_ = a.Run()
	<-fb.Done()
	if _, err := notify.Send(a, notify.Message{}); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
	if _, err := notify.RequestPermission(a); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
