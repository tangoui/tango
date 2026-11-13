package menu_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/menu"
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

func TestRadioAndCheck(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	clicks := make(chan bool, 4)
	m := menu.New()
	view := m.Submenu("View")
	small := view.Radio("size", "Small", true, nil)
	large := view.Radio("size", "Large", false, nil)
	other := view.Radio("other", "Other", true, nil)
	var wrap *menu.Item
	wrap = view.Check("Wrap", false, func() { clicks <- wrap.Checked }) // main thread
	if err := menu.SetAppMenu(a, m); err != nil {
		t.Fatal(err)
	}
	items := fb.Menu(0)
	if l := find(items, "Large"); l.RadioGroup != "size" || l.Checked {
		t.Fatalf("%+v", l)
	}
	if w := find(items, "Wrap"); !w.Checkable || w.Checked {
		t.Fatalf("%+v", w)
	}
	fb.ClickMenu(0, find(items, "Large").ID)
	waitFor(t, "radio", func() bool { return large.IsChecked() })
	if small.IsChecked() || !other.IsChecked() {
		t.Fatal("radio exclusivity")
	}
	// The click pushed the new state to the backend.
	items = fb.Menu(0)
	if !find(items, "Large").Checked || find(items, "Small").Checked || !find(items, "Other").Checked {
		t.Fatalf("not refreshed: %+v", items)
	}
	fb.ClickMenu(0, find(items, "Wrap").ID)
	if !<-clicks {
		t.Fatal("check not toggled before OnClick")
	}
	fb.ClickMenu(0, find(fb.Menu(0), "Wrap").ID)
	if <-clicks {
		t.Fatal("check not toggled back")
	}
	// SetChecked on a radio item applies the group rule.
	small.SetChecked(true)
	if large.IsChecked() {
		t.Fatal("SetChecked radio")
	}
}

func TestUpdate(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := menu.New()
	sub := m.Submenu("File")
	it := sub.Add("Save", nil)
	if err := menu.SetAppMenu(a, m); err != nil {
		t.Fatal(err)
	}
	if err := menu.SetSurfaceMenu(a, s, m); err != nil {
		t.Fatal(err)
	}
	it.SetLabel("Save As")
	it.SetDisabled(true)
	it.SetIcon([]byte{1, 2})
	if find(fb.Menu(0), "Save") == nil {
		t.Fatal("updated before Update")
	}
	if err := sub.Update(); err != nil { // a submenu updates its roots
		t.Fatal(err)
	}
	for _, p := range []platform.SurfaceID{0, s.ID()} {
		got := find(fb.Menu(p), "Save As")
		if got == nil || !got.Disabled || len(got.Icon) != 2 {
			t.Fatalf("parent %d: %+v", p, fb.Menu(p))
		}
	}
	// Unattached menus update nothing.
	if err := menu.New().Update(); err != nil {
		t.Fatal(err)
	}
	// Replacing the app menu detaches m from it.
	if err := menu.SetAppMenu(a, menu.New()); err != nil {
		t.Fatal(err)
	}
	it.SetLabel("Again")
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	if find(fb.Menu(0), "Again") != nil || find(fb.Menu(s.ID()), "Again") == nil {
		t.Fatal("detach")
	}
}

func TestPopupBlocks(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	clicked := false
	m := menu.New()
	c := m.Check("Pin", false, func() { clicked = true })
	done := make(chan error, 1)
	go func() { done <- menu.Popup(s, 10, 20, m) }()
	waitFor(t, "popup", func() bool { return len(fb.Popups()) == 1 })
	p := fb.Popups()[0]
	if p.Surface != s.ID() || p.X != 10 || p.Y != 20 || p.Items[0].Label != "Pin" {
		t.Fatalf("%+v", p)
	}
	select {
	case <-done:
		t.Fatal("Popup returned while open")
	case <-time.After(20 * time.Millisecond):
	}
	p.Choose(p.Items[0].ID)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !clicked || !c.IsChecked() {
		t.Fatal("click not delivered before return")
	}

	// From the main thread (nested): pumps events until dismissed.
	res := make(chan error, 1)
	a.Post(func() {
		go func() {
			waitFor(t, "nested popup", func() bool { return len(fb.Popups()) == 2 })
			fb.Popups()[1].Dismiss()
		}()
		res <- menu.Popup(s, 0, 0, m)
	})
	if err := <-res; err != nil {
		t.Fatal(err)
	}
}

func TestContextMenuMessage(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	var got []menu.Context
	m := menu.New()
	hit := make(chan string, 2)
	m.Add("Delete", func() { hit <- got[len(got)-1].Data })
	if err := menu.RegisterContextFunc(a, "row", func(c menu.Context) *menu.Menu { got = append(got, c); return m }); err != nil {
		t.Fatal(err)
	}
	raw := make(chan string, 4)
	a.OnMessage(func(_ *tango.Surface, msg string) { raw <- msg })
	faketest.Start(t, a, fb)
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bs := fb.Surface(s.ID()).Options().BridgeScript; !strings.Contains(bs, "data-tango-menu") || !strings.Contains(bs, "--tango-contextmenu") {
		t.Fatal("context script not injected")
	}
	fb.Message(s.ID(), "\x00tango-int:ctxmenu:"+`{"name":"row","x":5,"y":6,"data":"42"}`)
	fb.Message(s.ID(), "\x00tango-int:ctxmenu:"+`{"name":"nope","x":1,"y":1}`)
	fb.Message(s.ID(), "\x00tango-int:other:x") // unknown channel: dropped
	fb.Message(s.ID(), "hello")
	if msg := <-raw; msg != "hello" {
		t.Fatalf("internal message leaked to OnMessage: %q", msg)
	}
	ps := fb.Popups()
	if len(ps) != 1 || ps[0].X != 5 || ps[0].Y != 6 || ps[0].Surface != s.ID() {
		t.Fatalf("%+v", ps)
	}
	if len(got) != 1 || got[0].Name != "row" || got[0].Surface != s {
		t.Fatalf("%+v", got)
	}
	ps[0].Choose(ps[0].Items[0].ID)
	if d := <-hit; d != "42" {
		t.Fatal(d)
	}
	// Unregister.
	if err := menu.RegisterContext(a, "row", nil); err != nil {
		t.Fatal(err)
	}
	fb.Message(s.ID(), "\x00tango-int:ctxmenu:"+`{"name":"row","x":5,"y":6}`)
	if len(fb.Popups()) != 1 {
		t.Fatal("unregistered menu shown")
	}
}

func TestPopupUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := menu.Popup(s, 0, 0, menu.New()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := menu.RegisterContext(a, "x", menu.New()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
}
