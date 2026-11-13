package power_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/power"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStatusAndEvents(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	events := make(chan string, 16)
	statuses := make(chan power.Status, 16)
	// Registered before Run.
	power.OnSuspend(a, func() { events <- "suspend" })
	power.OnResume(a, func() { events <- "resume" })
	power.OnChange(a, func(s power.Status) { statuses <- s })
	faketest.Start(t, a, fb)

	st, err := power.Get(a)
	if err != nil || st.Source != power.AC || st.Battery != -1 {
		t.Fatal(st, err)
	}
	waitFor(t, "handler", func() bool { return fb.EmitPower(platform.PowerSuspending) })
	if e := <-events; e != "suspend" {
		t.Fatal(e)
	}
	bat := platform.PowerStatus{Source: power.Battery, HasBattery: true, Battery: 0.5, LowPower: true}
	fb.SetPower(bat)
	if s := <-statuses; s != bat {
		t.Fatal(s)
	}
	// Same status again: coalesced.
	fb.SetPower(bat)
	fb.EmitPower(platform.PowerResumed)
	if e := <-events; e != "resume" {
		t.Fatal(e)
	}
	select {
	case s := <-statuses:
		t.Fatal("duplicate status reported", s)
	case <-time.After(20 * time.Millisecond):
	}
	// Resume re-reads the status; a new one is reported.
	ac := platform.PowerStatus{Source: power.AC, HasBattery: true, Battery: 0.6, Charging: true}
	fb.SetPower(ac)
	if s := <-statuses; s != ac {
		t.Fatal(s)
	}
}

func TestBlocks(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{Name: "Player"})
	b1, err := power.KeepDisplayOn(a, "video")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := power.KeepAwake(a, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := fb.SleepBlocks(); len(got) != 2 {
		t.Fatal(got)
	}
	b1.Release()
	b1.Release()
	if b1.Active() || !b2.Active() {
		t.Fatal("active")
	}
	waitFor(t, "release", func() bool { return len(fb.SleepBlocks()) == 1 && fb.SleepBlocks()[0] == platform.BlockSystemSleep })
	b2.Release()
	waitFor(t, "release all", func() bool { return len(fb.SleepBlocks()) == 0 })
	var nilBlock *power.Block
	nilBlock.Release()

	boom := errors.New("no iokit")
	fb.SetPowerErr(boom)
	if _, err := power.KeepAwake(a, "x"); err != boom {
		t.Fatal(err)
	}
	if _, err := power.Get(a); err != boom {
		t.Fatal(err)
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := power.Get(a); !errors.Is(err, power.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := power.KeepAwake(a, ""); !errors.Is(err, power.ErrUnsupported) {
		t.Fatal(err)
	}
	power.OnChange(a, func(power.Status) {}) // no panic
}
