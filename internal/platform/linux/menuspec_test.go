package linux

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestMnemonicLabel(t *testing.T) {
	cases := map[string]string{
		"File":       "File",
		"&File":      "_File",
		"Save &As…":  "Save _As…",
		"Fish && Co": "Fish & Co",
		"snake_case": "snake__case",
		"trailing&":  "trailing",
	}
	for in, want := range cases {
		if got := mnemonicLabel(in); got != want {
			t.Errorf("mnemonicLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildMenuEntriesRoles(t *testing.T) {
	items := []platform.MenuItem{
		{Role: "appMenu"},
		{Role: "editMenu"},
		{Label: "&View", Submenu: []platform.MenuItem{
			{ID: 7, Label: "Reload", Accelerator: "CmdOrCtrl+R"},
			{Separator: true},
			{Role: "hide"}, // dropped on Linux
			{Separator: true},
			{ID: 8, Label: "Dark", Checked: true, Disabled: true},
			{Separator: true},
		}},
		{Role: "windowMenu", Label: "Win", Submenu: []platform.MenuItem{{Role: "close", Label: "Shut"}}},
	}
	got := buildMenuEntries(items, "My_App")
	if len(got) != 4 {
		t.Fatalf("got %d top-level entries, want 4: %+v", len(got), got)
	}

	app := got[0]
	if app.Label != "My__App" || app.Action != actNone || len(app.Children) != 3 {
		t.Fatalf("appMenu = %+v", app)
	}
	if q := app.Children[2]; q.Action != actQuit || q.Accel != "<Control>q" || q.AccelShow {
		t.Errorf("quit = %+v", q)
	}
	if a := app.Children[0]; a.Action != actAbout {
		t.Errorf("about = %+v", a)
	}

	edit := got[1]
	if edit.Label != "_Edit" || len(edit.Children) != 8 {
		t.Fatalf("editMenu = %+v", edit)
	}
	copyItem := edit.Children[4]
	if copyItem.Action != actEdit || copyItem.Command != "Copy" || copyItem.Accel != "<Control>c" || !copyItem.AccelShow {
		t.Errorf("copy = %+v", copyItem)
	}
	if redo := edit.Children[1]; redo.Accel != "<Control><Shift>z" {
		t.Errorf("redo accel = %q", redo.Accel)
	}

	view := got[2]
	want := []menuEntry{
		{ID: 7, Label: "Reload", Accel: "<Control>r", Action: actClick},
		{Separator: true},
		{ID: 8, Label: "Dark", Action: actClick, Checked: true, Check: true, Disabled: true},
	}
	if view.Label != "_View" || !reflect.DeepEqual(view.Children, want) {
		t.Errorf("view = %+v\nwant children %+v", view, want)
	}

	win := got[3]
	if win.Label != "Win" || len(win.Children) != 1 || win.Children[0].Label != "Shut" || win.Children[0].Action != actClose {
		t.Errorf("windowMenu = %+v", win)
	}
}

func TestBuildMenuEntriesAppMenuNoName(t *testing.T) {
	got := buildMenuEntries([]platform.MenuItem{{Role: "appMenu"}}, "")
	if got[0].Label != "_File" {
		t.Errorf("label = %q", got[0].Label)
	}
}

func TestBuildMenuEntriesUnknownRoleAndBadAccel(t *testing.T) {
	got := buildMenuEntries([]platform.MenuItem{{ID: 3, Label: "X", Role: "frobnicate", Accelerator: "Hyper+X"}}, "")
	if len(got) != 1 || got[0].Action != actClick || got[0].ID != 3 || got[0].Accel != "" {
		t.Errorf("got %+v", got)
	}
}

func TestBuildMenuEntriesCheckRadio(t *testing.T) {
	items := []platform.MenuItem{
		{ID: 1, Label: "Plain"},
		{ID: 2, Label: "Wrap", Checkable: true},
		{ID: 3, Label: "On", Checked: true},
		{ID: 4, Label: "Small", RadioGroup: "size", Checked: true},
		{ID: 5, Label: "Large", RadioGroup: "size"},
		{Label: "Sub", Checkable: true, RadioGroup: "x", Submenu: []platform.MenuItem{
			{ID: 6, Label: "Inner", RadioGroup: "y"},
		}},
	}
	got := buildMenuEntries(items, "")
	want := []menuEntry{
		{ID: 1, Label: "Plain", Action: actClick},
		{ID: 2, Label: "Wrap", Action: actClick, Check: true},
		{ID: 3, Label: "On", Action: actClick, Checked: true, Check: true},
		{ID: 4, Label: "Small", Action: actClick, Checked: true, Check: true, Radio: true},
		{ID: 5, Label: "Large", Action: actClick, Check: true, Radio: true},
		// Submenu parents are never check items.
		{Label: "Sub", Children: []menuEntry{
			{ID: 6, Label: "Inner", Action: actClick, Check: true, Radio: true},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}
