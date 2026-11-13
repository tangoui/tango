package push_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/push"
)

func TestPush(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	recv := make(chan push.Message, 1)
	open := make(chan push.Message, 1)
	tok := make(chan string, 1)
	push.OnReceive(a, func(m push.Message) { recv <- m })
	push.OnOpen(a, func(m push.Message) { open <- m })
	push.OnToken(a, func(s string) { tok <- s })
	faketest.Start(t, a, fb)

	if s, err := push.Register(a); s != "fake-push-token" || err != nil {
		t.Fatal(s, err)
	}
	fb.PushNotification(platform.PushPayload{Title: "T", Body: "B", Data: map[string]any{"id": "7"}}, false)
	fb.PushNotification(platform.PushPayload{ID: "n2", Title: "T2", Action: "reply"}, true)
	fb.PushToken("new")
	if m := recvOne(t, recv); m.Title != "T" || m.Body != "B" || m.Data["id"] != "7" {
		t.Fatalf("%+v", m)
	}
	if m := recvOne(t, open); m.Title != "T2" || m.Action != "reply" || m.ID != "n2" {
		t.Fatalf("%+v", m)
	}
	if s := recvOne(t, tok); s != "new" {
		t.Fatal(s)
	}
	fb.SetPushToken("", push.ErrUnsupported)
	if _, err := push.Register(a); !errors.Is(err, push.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestWebViewRestored(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s, _ := a.NewSurface(tango.SurfaceOptions{})
	got := make(chan string, 1)
	a.OnWebViewRestored(func(rs *tango.Surface, url string) {
		if rs == s {
			got <- url
		}
	})
	fb.WebViewRestored(s.ID(), "tango://app/")
	select {
	case u := <-got:
		if u != "tango://app/" {
			t.Fatal(u)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no event")
	}
}

func recvOne[T any](t *testing.T, c chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(faketest.Timeout):
		t.Fatal("event missing")
	}
	panic("unreachable")
}
