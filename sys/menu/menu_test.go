package menu_test

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/menu"
)

func find(items []platform.MenuItem, label string) *platform.MenuItem {
	for i := range items {
		if items[i].Label == label {
			return &items[i]
		}
		if f := find(items[i].Submenu, label); f != nil {
			return f
		}
	}
	return nil
}

func TestAppMenuClicks(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	clicks := make(chan string, 4)
	m := menu.New()
	app := m.Submenu("App")
	app.Role(menu.About)
	app.Separator()
	app.Role(menu.Quit)
	file := m.Submenu("File")
	file.Add("New", func() { clicks <- "new" }).Accelerator = "CmdOrCtrl+N"
	open := file.Add("Open", func() { clicks <- "open" })
	open.Disabled = true
	file.Add("Checked", nil).Checked = true
	m.Submenu("Edit").Role(menu.EditMenu)

	if err := menu.SetAppMenu(a, m); err != nil {
		t.Fatal(err)
	}
	items := fb.Menu(0)
	if len(items) != 3 || items[0].Label != "App" || len(items[0].Submenu) != 3 {
		t.Fatalf("tree %+v", items)
	}
	if r := items[0].Submenu; r[0].Role != "about" || !r[1].Separator || r[2].Role != "quit" || r[0].ID != 0 || r[1].ID != 0 {
		t.Fatalf("roles %+v", r)
	}
	if items[0].ID != 0 {
		t.Fatal("submenu got an id")
	}
	nw, op, ck := find(items, "New"), find(items, "Open"), find(items, "Checked")
	if nw.ID == 0 || op.ID == 0 || ck.ID == 0 || nw.ID == op.ID || nw.Accelerator != "CmdOrCtrl+N" || !op.Disabled || !ck.Checked {
		t.Fatalf("items %+v %+v %+v", nw, op, ck)
	}
	fb.ClickMenu(0, nw.ID)
	fb.ClickMenu(0, op.ID)
	fb.ClickMenu(0, ck.ID)  // nil OnClick: ignored
	fb.ClickMenu(0, 999999) // unknown: ignored
	if got := <-clicks; got != "new" {
		t.Fatal(got)
	}
	if got := <-clicks; got != "open" {
		t.Fatal(got)
	}
	select {
	case got := <-clicks:
		t.Fatalf("extra click %q", got)
	default:
	}

	// Setting again allocates fresh ids; old ones no longer fire.
	called := make(chan struct{}, 1)
	m2 := menu.New()
	m2.Add("Only", func() { called <- struct{}{} })
	if err := menu.SetAppMenu(a, m2); err != nil {
		t.Fatal(err)
	}
	only := fb.Menu(0)[0]
	if only.ID == nw.ID {
		t.Fatal("ids reused")
	}
	fb.ClickMenu(0, nw.ID)
	fb.ClickMenu(0, only.ID)
	<-called
}

func TestClickOnMainThread(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	onMain := make(chan bool, 1)
	m := menu.New()
	m.Add("X", func() { onMain <- fb.IsMainThread() })
	if err := menu.SetAppMenu(a, m); err != nil {
		t.Fatal(err)
	}
	fb.ClickMenu(0, fb.Menu(0)[0].ID)
	if !<-onMain {
		t.Fatal("click off main thread")
	}
}

func TestSurfaceMenuAndCycle(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m := menu.New()
	sub := m.Submenu("Loop")
	sub.Items = append(sub.Items, &menu.Item{Label: "Back", Submenu: m}) // cycle
	if err := menu.SetSurfaceMenu(a, s, m); err != nil {
		t.Fatal(err)
	}
	items := fb.Menu(s.ID())
	if len(items) != 1 || items[0].Submenu[0].Label != "Back" || items[0].Submenu[0].Submenu != nil {
		t.Fatalf("%+v", items)
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if err := menu.SetAppMenu(a, menu.New()); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestAfterQuit(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	a.Quit()
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	<-fb.Done()
	if err := menu.SetAppMenu(a, menu.New()); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
