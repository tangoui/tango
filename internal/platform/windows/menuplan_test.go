package windows

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestBuildMenuPlan(t *testing.T) {
	items := []platform.MenuItem{
		{Label: "&File", Submenu: []platform.MenuItem{
			{ID: 70000, Label: "New", Accelerator: "CmdOrCtrl+N"},
			{ID: 11, Label: "Open…", Accelerator: "Super+O"}, // invalid on Windows: no accel
			{Separator: true},
			{ID: 12, Label: "Autosave", Checked: true},
			{ID: 13, Label: "Disabled", Disabled: true, Accelerator: "Ctrl+D"},
			{Label: "Inert"},
			{Role: "quit"},
		}},
		{Role: "editMenu"},
		{Role: "windowMenu", Label: "Fenster"},
		{Role: "appMenu"},
		{ID: 99, Label: "Help"},
	}
	p := buildMenuPlan(items, "Demo")
	if len(p.Entries) != 5 {
		t.Fatalf("top entries = %d", len(p.Entries))
	}
	file := p.Entries[0]
	if file.Kind != entryPopup || file.Label != "&File" || len(file.Children) != 7 {
		t.Fatalf("file = %+v", file)
	}
	newItem := file.Children[0]
	if newItem.Label != "New\tCtrl+N" || newItem.Cmd == 0 {
		t.Errorf("new = %+v", newItem)
	}
	// Core IDs above 16 bits are mapped to WM_COMMAND-sized commands.
	if act := p.Actions[newItem.Cmd]; act.ID != 70000 || act.Role != "" {
		t.Errorf("new action = %+v", act)
	}
	if open := file.Children[1]; open.Label != "Open…" || open.Cmd == 0 {
		t.Errorf("open = %+v", open)
	}
	if file.Children[2].Kind != entrySeparator {
		t.Error("separator lost")
	}
	if !file.Children[3].Checked {
		t.Error("checked lost")
	}
	if d := file.Children[4]; !d.Disabled || d.Label != "Disabled\tCtrl+D" {
		t.Errorf("disabled = %+v", d)
	}
	if inert := file.Children[5]; inert.Cmd != 0 {
		t.Errorf("item without ID got command %d", inert.Cmd)
	}
	quit := file.Children[6]
	if quit.Label != "Exit" || p.Actions[quit.Cmd].Role != "quit" {
		t.Errorf("quit = %+v %+v", quit, p.Actions[quit.Cmd])
	}

	edit := p.Entries[1]
	if edit.Label != "Edit" || len(edit.Children) != 8 {
		t.Fatalf("edit = %+v", edit)
	}
	if c := edit.Children[4]; c.Label != "Copy\tCtrl+C" || p.Actions[c.Cmd].Role != "copy" {
		t.Errorf("copy = %+v", c)
	}
	win := p.Entries[2]
	if win.Label != "Fenster" || len(win.Children) != 2 || win.Children[1].Label != "Close\tCtrl+W" {
		t.Errorf("window = %+v", win)
	}
	app := p.Entries[3]
	if app.Label != "Demo" || app.Children[0].Label != "About Demo" {
		t.Errorf("app = %+v", app)
	}
	help := p.Entries[4]
	if help.Kind != entryCommand || p.Actions[help.Cmd].ID != 99 {
		t.Errorf("top-level command = %+v", help)
	}

	// Accelerators: Ctrl+N and the close role's Ctrl+W are registered;
	// editing keys are left to the WebView, disabled and invalid ones are
	// skipped.
	got := map[string]uint16{}
	for _, a := range p.Accels {
		got[a.Accel.String()] = a.Cmd
	}
	if len(got) != 2 || got["Ctrl+N"] != newItem.Cmd || got["Ctrl+W"] != win.Children[1].Cmd {
		t.Errorf("accels = %v", got)
	}

	// Commands are unique and in range.
	seen := map[uint16]bool{}
	var walk func([]menuEntry)
	walk = func(es []menuEntry) {
		for _, e := range es {
			if e.Cmd != 0 {
				if seen[e.Cmd] {
					t.Errorf("duplicate command %d", e.Cmd)
				}
				if e.Cmd > lastMenuCmd {
					t.Errorf("command %d out of range", e.Cmd)
				}
				seen[e.Cmd] = true
			}
			walk(e.Children)
		}
	}
	walk(p.Entries)
	if len(seen) != len(p.Actions) {
		t.Errorf("%d commands, %d actions", len(seen), len(p.Actions))
	}
}

func TestMenuPlanCustomRoleSubmenu(t *testing.T) {
	p := buildMenuPlan([]platform.MenuItem{{Role: "editMenu", Submenu: []platform.MenuItem{{Role: "paste", Label: "Einfügen"}}}}, "")
	e := p.Entries[0]
	if e.Label != "Edit" || len(e.Children) != 1 || e.Children[0].Label != "Einfügen\tCtrl+V" {
		t.Errorf("custom edit = %+v", e)
	}
	if app := buildMenuPlan([]platform.MenuItem{{Role: "appMenu"}}, "").Entries[0]; app.Label != "App" || app.Children[0].Label != "About" {
		t.Errorf("unnamed app menu = %+v", app)
	}
	if len(buildMenuPlan(nil, "x").Entries) != 0 {
		t.Error("empty plan")
	}
}

func TestRoleEditCommand(t *testing.T) {
	for _, r := range []string{"undo", "redo", "cut", "copy", "paste", "selectAll"} {
		if c, ok := roleEditCommand(r); !ok || c != r {
			t.Errorf("%s: %q %v", r, c, ok)
		}
	}
	if _, ok := roleEditCommand("quit"); ok {
		t.Error("quit is not an edit command")
	}
}

func TestBuildMenuPlanCheckRadioIcon(t *testing.T) {
	icon := []byte{0x89, 'P', 'N', 'G'}
	p := buildMenuPlan([]platform.MenuItem{
		{ID: 1, Label: "Plain"},
		{ID: 2, Label: "Check", Checkable: true},
		{ID: 3, Label: "Small", RadioGroup: "size", Checked: true},
		{ID: 4, Label: "Large", RadioGroup: "size"},
		{ID: 5, Label: "Icon", Icon: icon},
		{Label: "Sub", Icon: icon, Submenu: []platform.MenuItem{{ID: 6, Label: "X"}}},
	}, "")
	e := p.Entries
	if e[0].Checkable || e[0].Radio || e[0].Icon != nil {
		t.Errorf("plain = %+v", e[0])
	}
	if !e[1].Checkable || e[1].Checked || e[1].Radio {
		t.Errorf("check = %+v", e[1])
	}
	if !e[2].Radio || !e[2].Checked || !e[2].Checkable {
		t.Errorf("small = %+v", e[2])
	}
	if !e[3].Radio || e[3].Checked {
		t.Errorf("large = %+v", e[3])
	}
	if string(e[4].Icon) != string(icon) || string(e[5].Icon) != string(icon) {
		t.Errorf("icons = %v / %v", e[4].Icon, e[5].Icon)
	}
}
