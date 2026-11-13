package jumplist

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

func TestSetAndClick(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	got := make(chan string, 4)
	OnClick(a, func(id string) { got <- id })
	err := Set(a, []Item{
		{ID: "new", Label: "New Window"},
		Separator(),
		{ID: "sync", Label: "Sync", Description: "Fetch", Icon: `C:\a.ico,1`, Disabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	items, n := fb.Shortcuts()
	if n != 1 || len(items) != 3 || !items[1].Separator || items[2].IconPath != `C:\a.ico,1` || !items[2].Disabled {
		t.Fatalf("%d %+v", n, items)
	}
	if !fb.ClickShortcut("sync") {
		t.Fatal("click")
	}
	if id := <-got; id != "sync" {
		t.Fatal(id)
	}
	if fb.ClickShortcut("nope") {
		t.Fatal("unknown id clicked")
	}
	if err := Clear(a); err != nil {
		t.Fatal(err)
	}
	if items, _ := fb.Shortcuts(); items != nil {
		t.Fatal(items)
	}
	boom := errors.New("shell")
	fb.SetShortcutErr(boom)
	if err := Set(a, []Item{{ID: "a", Label: "A"}}); err != boom {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	for _, items := range [][]Item{
		{{Label: "no id"}},
		{{ID: "has space", Label: "x"}},
		{{ID: `quote"`, Label: "x"}},
		{{ID: "a"}},
		{{ID: "a", Label: "A"}, {ID: "a", Label: "B"}},
	} {
		if _, err := convert(items); !errors.Is(err, ErrBadItem) {
			t.Errorf("%+v: %v", items, err)
		}
	}
	if _, err := convert([]Item{{ID: "open-recent_1.x:y", Label: "ok"}, Separator(), Separator()}); err != nil {
		t.Fatal(err)
	}
}

func TestArgs(t *testing.T) {
	if Arg("new") != "--tango-shortcut=new" {
		t.Fatal(Arg("new"))
	}
	if id, ok := IDFromArgs([]string{"-v", "--tango-shortcut=new", "file.txt"}); !ok || id != "new" {
		t.Fatal(id, ok)
	}
	if _, ok := IDFromArgs([]string{"--tango-shortcut=", "x"}); ok {
		t.Fatal("empty id accepted")
	}
}

func TestLaunchArgument(t *testing.T) {
	old := launchArgs
	launchArgs = func() []string { return []string{"--tango-shortcut=compose"} }
	defer func() { launchArgs = old }()
	a, fb := faketest.NewApp(tango.Options{})
	got := make(chan string, 2)
	OnClick(a, func(id string) { got <- id })
	OnClick(a, func(id string) { got <- "second:" + id })
	faketest.Start(t, a, fb)
	for _, want := range []string{"compose", "second:compose"} {
		select {
		case id := <-got:
			if id != want {
				t.Fatal(id)
			}
		case <-time.After(faketest.Timeout):
			t.Fatal("launch shortcut not reported")
		}
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if err := Set(a, []Item{{ID: "a", Label: "A"}}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
