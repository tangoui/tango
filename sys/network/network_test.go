package network_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/network"
)

func TestNetwork(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	ch := make(chan network.Status, 8)
	network.OnChange(a, func(s network.Status) { ch <- s })
	faketest.Start(t, a, fb)

	st, err := network.Get(a)
	if err != nil || !st.Online || st.Kind != network.Wired {
		t.Fatal(st, err)
	}
	deadline := time.Now().Add(faketest.Timeout)
	wifi := network.Status{Online: true, Kind: network.WiFi, Metered: true}
	for !fb.SetNetwork(wifi) {
		if time.Now().After(deadline) {
			t.Fatal("handler never installed")
		}
		time.Sleep(time.Millisecond)
	}
	if s := <-ch; s != wifi {
		t.Fatal(s)
	}
	fb.SetNetwork(wifi) // unchanged: coalesced
	off := network.Status{Kind: network.None}
	fb.SetNetwork(off)
	if s := <-ch; s != off {
		t.Fatal("got", s)
	}
	boom := errors.New("nm down")
	fb.SetNetworkErr(boom)
	if _, err := network.Get(a); err != boom {
		t.Fatal(err)
	}
}

func TestKindString(t *testing.T) {
	for k, want := range map[network.Kind]string{
		network.Unknown: "unknown", network.None: "none", network.Wired: "wired",
		network.WiFi: "wifi", network.Cellular: "cellular", network.Other: "other",
	} {
		if got := network.KindString(k); got != want {
			t.Errorf("%d: %q", k, got)
		}
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := network.Get(a); !errors.Is(err, network.ErrUnsupported) {
		t.Fatal(err)
	}
	network.OnChange(a, func(network.Status) {})
}
