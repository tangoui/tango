package tray_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/tray"
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

func TestTray(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	tr, err := tray.New(a, tray.Options{Icon: []byte{1}, Template: true, Tooltip: "tip"})
	if err != nil {
		t.Fatal(err)
	}
	ft := fb.Trays()[0]
	if o := ft.Options(); !o.Template || o.Tooltip != "tip" || len(o.Icon) != 1 {
		t.Fatalf("%+v", o)
	}
	tr.SetTitle("T")
	tr.SetTooltip("tip2")
	tr.SetIcon([]byte{1, 2}, false)
	tr.SetMenuOn(tray.MenuOnSecondary)
	waitFor(t, "options", func() bool {
		o := ft.Options()
		return o.Title == "T" && o.Tooltip == "tip2" && len(o.Icon) == 2 && !o.Template && o.MenuOn == tray.MenuOnSecondary
	})

	ev := make(chan string, 8)
	tr.OnClick(func() { ev <- "click" })
	tr.OnRightClick(func() { ev <- "right" })
	tr.OnDoubleClick(func() { ev <- "double" })
	ft.Click()
	ft.RightClick()
	ft.DoubleClick()
	for _, want := range []string{"click", "right", "double"} {
		if got := <-ev; got != want {
			t.Fatal(got, want)
		}
	}

	m := menu.New()
	status := m.Add("Status: idle", nil)
	dark := m.Check("Dark", false, func() { ev <- "dark" })
	m.Role(menu.Quit)
	if err := tr.SetMenu(m); err != nil {
		t.Fatal(err)
	}
	items := ft.Menu()
	if len(items) != 3 || items[2].Role != "quit" {
		t.Fatalf("%+v", items)
	}
	ft.ChooseMenu(items[1].ID)
	if <-ev != "dark" || !dark.IsChecked() || !ft.Menu()[1].Checked {
		t.Fatal("check via tray menu")
	}
	status.SetLabel("Status: busy")
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	if ft.Menu()[0].Label != "Status: busy" {
		t.Fatal("update not pushed")
	}
	if r, ok := tr.Bounds(); !ok || r != fake.TrayBounds {
		t.Fatal(r, ok)
	}
	if err := tr.SetMenu(nil); err != nil {
		t.Fatal(err)
	}
	if ft.Menu() != nil {
		t.Fatal("menu not removed")
	}
	sets := ft.MenuSets()
	status.SetLabel("x")
	_ = m.Update() // detached: no push
	if ft.MenuSets() != sets {
		t.Fatal("detached menu pushed")
	}

	tr.Remove()
	waitFor(t, "remove", ft.Removed)
	if _, ok := tr.Bounds(); ok {
		t.Fatal("bounds after remove")
	}
	if ft.Click() {
		t.Fatal("click after remove")
	}
}

func TestTrayUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := tray.New(a, tray.Options{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestTrayMacIcon(t *testing.T) {
	colour, mono := []byte{1}, []byte{2, 2}
	for _, tc := range []struct {
		os   string
		want int
	}{{"darwin", 2}, {"windows", 1}, {"linux", 1}} {
		restore := tray.SetGOOS(tc.os)
		a, fb := faketest.Run(t, tango.Options{})
		tr, err := tray.New(a, tray.Options{Icon: colour, MacIcon: mono, Template: true})
		if err != nil {
			restore()
			t.Fatal(err)
		}
		ft := fb.Trays()[0]
		if got := len(ft.Options().Icon); got != tc.want {
			t.Errorf("%s: shows icon of %d bytes, want %d", tc.os, got, tc.want)
		}
		if tc.os == "darwin" {
			tr.SetMacIcon(nil) // falls back to Icon
			waitFor(t, "fallback", func() bool { return len(ft.Options().Icon) == 1 })
		}
		restore()
	}
}
