package dialog_test

import (
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/apphook"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/sys/notify"
)

func TestDialogFromUIThreadAwaitsMain(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	th := uithread.Start("dialog test")
	t.Cleanup(th.Stop)
	type observation struct {
		main, waiting bool
		parent        platform.SurfaceID
	}
	seen := make(chan observation, 1)
	fb.OnMessageDialog = func(id platform.SurfaceID, _ platform.MessageOptions) (int, error) {
		seen <- observation{fb.IsMainThread(), th.AwaitingMain(), id}
		return 1, nil
	}
	faketest.Start(t, a, fb)
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan bool, 1)
	th.Post(func() {
		n, err := dialog.Message(a, dialog.MessageOptions{Parent: s, Buttons: []string{"No", "Yes"}})
		result <- n == 1 && err == nil && !th.AwaitingMain()
	})
	select {
	case ok := <-result:
		if !ok {
			t.Error("dialog did not return its result and clear the wait mark")
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("dialog did not return")
	}
	if got := <-seen; got != (observation{true, true, s.ID()}) {
		t.Fatalf("dialog main/waiting/parent = %+v, want true/true/%v", got, s.ID())
	}
}

type observedNotifier struct {
	*fake.Backend
	observe func()
}

func (b observedNotifier) RequestNotifyPermission(done func(bool, error)) {
	b.observe()
	b.Backend.RequestNotifyPermission(done)
}

func TestNotificationPermissionFromUIThreadAwaitsMain(t *testing.T) {
	fb := fake.New()
	th := uithread.Start("notify test")
	t.Cleanup(th.Stop)
	seen := make(chan bool, 1)
	a := apphook.NewApp(observedNotifier{fb, func() {
		seen <- fb.IsMainThread() && th.AwaitingMain()
	}}, tango.Options{}).(*tango.App)
	faketest.Start(t, a, fb)
	result := make(chan bool, 1)
	th.Post(func() {
		ok, err := notify.RequestPermission(a)
		result <- ok && err == nil && !th.AwaitingMain()
	})
	select {
	case ok := <-result:
		if !ok {
			t.Error("permission did not return its result and clear the wait mark")
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("notification permission did not return")
	}
	if !<-seen {
		t.Fatal("notification permission did not wait on the main thread with wait mark set")
	}
}
