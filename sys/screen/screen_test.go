package screen_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/screen"
)

var twoDisplays = []platform.Display{
	{ID: "left", Bounds: platform.Rect{X: -1920, Y: 0, Width: 1920, Height: 1080}, Scale: 1},
	{ID: "main", Bounds: platform.Rect{Width: 1440, Height: 900}, Scale: 2, Primary: true},
}

func TestAllPrimaryAt(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetDisplays(twoDisplays)
	ds, err := screen.All(a)
	if err != nil || len(ds) != 2 || ds[0].ID != "main" || ds[1].ID != "left" || ds[1].Primary {
		t.Fatalf("%+v %v", ds, err)
	}
	p, err := screen.Primary(a)
	if err != nil || p.ID != "main" {
		t.Fatalf("%+v %v", p, err)
	}
	for _, c := range []struct {
		x, y int
		id   string
	}{
		{10, 10, "main"}, {-5, 500, "left"}, {1439, 899, "main"}, {1440, 0, "main"},
		{-3000, 10, "left"}, {5000, 5000, "main"}, {-100, 2000, "left"},
	} {
		d, err := screen.At(a, c.x, c.y)
		if err != nil || d.ID != c.id {
			t.Errorf("At(%d,%d) = %s %v, want %s", c.x, c.y, d.ID, err, c.id)
		}
	}
}

func TestNoPrimaryFlag(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetDisplays([]platform.Display{{ID: "a"}, {ID: "b"}})
	p, err := screen.Primary(a)
	if err != nil || p.ID != "a" || !p.Primary {
		t.Fatalf("%+v %v", p, err)
	}
	fb.SetDisplays(nil)
	if _, err := screen.Primary(a); !errors.Is(err, screen.ErrNoDisplay) {
		t.Fatal(err)
	}
	if _, err := screen.At(a, 0, 0); !errors.Is(err, screen.ErrNoDisplay) {
		t.Fatal(err)
	}
}

func TestOnChange(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	got := make(chan int, 4)
	screen.OnChange(a, func() {
		ds, _ := screen.All(a) // main thread: Await runs directly
		got <- len(ds)
	})
	fb.SetTheme(platform.Theme{Dark: true}) // not a display event
	fb.SetDisplays(twoDisplays)
	select {
	case n := <-got:
		if n != 2 {
			t.Fatal(n)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no change event")
	}
	if len(got) != 0 {
		t.Fatal("extra events")
	}
}

func TestUnsupportedAndQuit(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := screen.All(a); !errors.Is(err, screen.ErrUnsupported) {
		t.Fatal(err)
	}
	b, fb := faketest.NewApp(tango.Options{})
	b.Quit()
	if err := b.Run(); err != nil {
		t.Fatal(err)
	}
	<-fb.Done()
	if _, err := screen.Primary(b); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
