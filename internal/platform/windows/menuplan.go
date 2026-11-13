package windows

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/rolenames"
)

// A menuPlan is a platform.MenuItem tree translated for Win32: labels with
// accelerator text, 16-bit command IDs (WM_COMMAND carries only the low
// word), and the accelerator table. Building HMENUs from it is mechanical
// (menu_windows.go).

type entryKind int

const (
	entryCommand entryKind = iota
	entrySeparator
	entryPopup
)

type menuEntry struct {
	Kind     entryKind
	Label    string // includes "\t<accelerator>" when there is one
	Cmd      uint16 // 0 when not clickable
	Disabled bool
	Checked  bool
	// Checkable marks a check item even when unchecked. Win32 menus reserve
	// the check column anyway, so it changes nothing visible today.
	Checkable bool
	Radio     bool   // MFT_RADIOCHECK: a bullet instead of a check mark
	Icon      []byte // PNG for hbmpItem; nil = none
	Children  []menuEntry
}

// menuAction is what a command does: call onClick(ID), or run a role.
type menuAction struct {
	ID   uint32
	Role string
}

type menuPlan struct {
	Entries []menuEntry
	Actions map[uint16]menuAction
	Accels  []accelEntry
}

// Command IDs stay below 0xF000, where SC_* system commands start.
const (
	firstMenuCmd = 1
	lastMenuCmd  = 0xEFFF
)

type roleInfo struct {
	label string // rolenames key
	accel string
	// nativeKey: the WebView handles the key itself (editing keys), so the
	// accelerator is shown but not registered.
	nativeKey bool
}

var leafRoles = map[string]roleInfo{
	"quit":      {label: "win.quit"},
	"about":     {label: "win.about"},
	"hide":      {label: "win.hide"},
	"minimize":  {label: "win.minimize"},
	"close":     {label: "win.close", accel: "Ctrl+W"},
	"undo":      {label: "win.undo", accel: "Ctrl+Z", nativeKey: true},
	"redo":      {label: "win.redo", accel: "Ctrl+Y", nativeKey: true},
	"cut":       {label: "win.cut", accel: "Ctrl+X", nativeKey: true},
	"copy":      {label: "win.copy", accel: "Ctrl+C", nativeKey: true},
	"paste":     {label: "win.paste", accel: "Ctrl+V", nativeKey: true},
	"selectAll": {label: "win.selectAll", accel: "Ctrl+A", nativeKey: true},
}

// roleEditCommand returns the Chromium editing command of an edit role.
func roleEditCommand(role string) (string, bool) {
	switch role {
	case "undo", "redo", "cut", "copy", "paste", "selectAll":
		return role, true
	}
	return "", false
}

// defaultSubmenu returns the label and default items of a submenu role.
func defaultSubmenu(role, appName string) (string, []platform.MenuItem, bool) {
	sep := platform.MenuItem{Separator: true}
	switch role {
	case "appMenu":
		if appName == "" {
			appName = "App"
		}
		return appName, []platform.MenuItem{{Role: "about"}, sep, {Role: "quit"}}, true
	case "editMenu":
		return rolenames.Text("win.editMenu"), []platform.MenuItem{
			{Role: "undo"}, {Role: "redo"}, sep,
			{Role: "cut"}, {Role: "copy"}, {Role: "paste"}, sep,
			{Role: "selectAll"},
		}, true
	case "windowMenu":
		return rolenames.Text("win.windowMenu"), []platform.MenuItem{{Role: "minimize"}, {Role: "close"}}, true
	}
	return "", nil, false
}

// buildMenuPlan translates items. appName labels the "appMenu" role and
// the "about" item.
func buildMenuPlan(items []platform.MenuItem, appName string) *menuPlan {
	p := &menuPlan{Actions: map[uint16]menuAction{}}
	next := uint16(firstMenuCmd)
	var build func([]platform.MenuItem) []menuEntry
	build = func(items []platform.MenuItem) []menuEntry {
		out := make([]menuEntry, 0, len(items))
		for _, it := range items {
			if it.Separator {
				out = append(out, menuEntry{Kind: entrySeparator})
				continue
			}
			label, children := it.Label, it.Submenu
			if dl, dc, ok := defaultSubmenu(it.Role, appName); ok {
				if label == "" {
					label = dl
				}
				if len(children) == 0 {
					children = dc
				}
			}
			if len(children) > 0 {
				out = append(out, menuEntry{
					Kind: entryPopup, Label: label, Disabled: it.Disabled, Icon: it.Icon,
					Children: build(children),
				})
				continue
			}
			role, isRole := leafRoles[it.Role]
			if label == "" && isRole {
				label = rolenames.Text(role.label)
				if it.Role == "about" && appName != "" {
					label = rolenames.Format("win.aboutApp", appName)
				}
			}
			accelText := it.Accelerator
			native := false
			if accelText == "" && isRole {
				accelText = role.accel
			}
			if isRole {
				native = role.nativeKey
			}
			e := menuEntry{
				Kind: entryCommand, Label: label, Disabled: it.Disabled, Checked: it.Checked,
				Checkable: it.Checkable || it.Checked || it.RadioGroup != "",
				Radio:     it.RadioGroup != "",
				Icon:      it.Icon,
			}
			clickable := it.ID != 0 || isRole
			if clickable && next <= lastMenuCmd {
				e.Cmd = next
				next++
				act := menuAction{ID: it.ID}
				if isRole {
					act.Role = it.Role
				}
				p.Actions[e.Cmd] = act
			}
			if accelText != "" {
				if a, err := parseAccelerator(accelText); err == nil {
					e.Label += "\t" + a.String()
					if e.Cmd != 0 && !native && !it.Disabled {
						p.Accels = append(p.Accels, accelEntry{Accel: a, Cmd: e.Cmd})
					}
				}
			}
			out = append(out, e)
		}
		return out
	}
	p.Entries = build(items)
	return p
}
