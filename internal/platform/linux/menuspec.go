package linux

import (
	"strings"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/rolenames"
)

// menuAction is what activating a menu item does.
type menuAction int

const (
	actNone     menuAction = iota // submenu parent / separator
	actClick                      // report MenuItem.ID to onClick
	actQuit                       // Backend.Quit
	actAbout                      // GtkAboutDialog
	actEdit                       // webkit_web_view_execute_editing_command(Command)
	actMinimize                   // gtk_window_iconify
	actClose                      // gtk_window_close (asks Host.ShouldClose)
)

// menuEntry is a MenuItem resolved for GTK: roles expanded, labels in GTK
// mnemonic syntax, accelerators in gtk_accelerator_parse syntax.
type menuEntry struct {
	ID        uint32
	Label     string // GTK mnemonic label ("_File")
	Accel     string // "<Control>q", "" for none
	AccelShow bool   // show Accel in the label but do not bind it (see below)
	Action    menuAction
	Command   string // WebKit editing command for actEdit
	Separator bool
	Disabled  bool
	Checked   bool
	// Check renders the item as a GtkCheckMenuItem (MenuItem.Checkable,
	// Checked or RadioGroup); Radio additionally draws it as a radio
	// button. Radio items are check items drawn as radios rather than
	// GtkRadioMenuItems: GTK would flip the group's state on its own,
	// whereas the core owns the state and re-sends the menu after a click.
	Check    bool
	Radio    bool
	Children []menuEntry
}

// roleDef is the backend-supplied label/accelerator/action of a role.
type roleDef struct {
	label   string // rolenames key
	accel   string // Tango syntax
	action  menuAction
	command string
	skip    bool // no Linux equivalent: the item is left out
}

// Editing roles only display their shortcut: WebKit already handles
// Ctrl+C/V/X/A/Z in the page, and binding them to the menu item would steal
// the keys from the page's own keydown handlers.
var roleDefs = map[string]roleDef{
	"quit":      {label: "gtk.quit", accel: "Ctrl+Q", action: actQuit},
	"about":     {label: "gtk.about", action: actAbout},
	"hide":      {skip: true},
	"copy":      {label: "gtk.copy", accel: "Ctrl+C", action: actEdit, command: "Copy"},
	"paste":     {label: "gtk.paste", accel: "Ctrl+V", action: actEdit, command: "Paste"},
	"cut":       {label: "gtk.cut", accel: "Ctrl+X", action: actEdit, command: "Cut"},
	"selectAll": {label: "gtk.selectAll", accel: "Ctrl+A", action: actEdit, command: "SelectAll"},
	"undo":      {label: "gtk.undo", accel: "Ctrl+Z", action: actEdit, command: "Undo"},
	"redo":      {label: "gtk.redo", accel: "Ctrl+Shift+Z", action: actEdit, command: "Redo"},
	"minimize":  {label: "gtk.minimize", accel: "Ctrl+M", action: actMinimize},
	"close":     {label: "gtk.close", accel: "Ctrl+W", action: actClose},
}

// Submenu roles and their default contents.
var submenuRoles = map[string]func(appName string) (string, []platform.MenuItem){
	"appMenu": func(appName string) (string, []platform.MenuItem) {
		label := rolenames.Text("gtk.fileMenu")
		if appName != "" {
			label = escapeMnemonic(appName)
		}
		return label, []platform.MenuItem{{Role: "about"}, {Separator: true}, {Role: "quit"}}
	},
	"editMenu": func(string) (string, []platform.MenuItem) {
		return rolenames.Text("gtk.editMenu"), []platform.MenuItem{
			{Role: "undo"}, {Role: "redo"}, {Separator: true},
			{Role: "cut"}, {Role: "copy"}, {Role: "paste"}, {Separator: true},
			{Role: "selectAll"},
		}
	},
	"windowMenu": func(string) (string, []platform.MenuItem) {
		return rolenames.Text("gtk.windowMenu"), []platform.MenuItem{{Role: "minimize"}, {Role: "close"}}
	},
}

// buildMenuEntries resolves items for GTK. Unknown roles are treated as
// plain items; roles with no Linux equivalent are dropped, and separators
// left leading, trailing or doubled by dropping are removed.
func buildMenuEntries(items []platform.MenuItem, appName string) []menuEntry {
	out := make([]menuEntry, 0, len(items))
	for _, it := range items {
		e, ok := buildMenuEntry(it, appName)
		if !ok {
			continue
		}
		if e.Separator && (len(out) == 0 || out[len(out)-1].Separator) {
			continue
		}
		out = append(out, e)
	}
	for len(out) > 0 && out[len(out)-1].Separator {
		out = out[:len(out)-1]
	}
	return out
}

func buildMenuEntry(it platform.MenuItem, appName string) (menuEntry, bool) {
	if it.Separator {
		return menuEntry{Separator: true}, true
	}
	e := menuEntry{ID: it.ID, Disabled: it.Disabled, Checked: it.Checked}
	label, accel := it.Label, it.Accelerator
	children := it.Submenu

	if mk, ok := submenuRoles[it.Role]; ok {
		defLabel, defChildren := mk(appName)
		if len(children) == 0 {
			children = defChildren
		}
		if label == "" {
			e.Label = defLabel
		}
	} else if def, ok := roleDefs[it.Role]; ok {
		if def.skip {
			return menuEntry{}, false
		}
		e.Action, e.Command = def.action, def.command
		e.AccelShow = def.action == actEdit
		if label == "" {
			e.Label = rolenames.Text(def.label)
		}
		if accel == "" {
			accel = def.accel
		}
	} else if len(children) == 0 {
		e.Action = actClick
	}
	if e.Label == "" {
		e.Label = mnemonicLabel(label)
	}
	if a, err := gtkAccelerator(accel); err == nil {
		e.Accel = a
	}
	if len(children) > 0 {
		e.Action, e.Command, e.Accel, e.AccelShow = actNone, "", "", false
		e.Children = buildMenuEntries(children, appName)
	} else {
		e.Radio = it.RadioGroup != ""
		e.Check = it.Checkable || it.Checked || e.Radio
	}
	return e, true
}

// mnemonicLabel converts a Tango label to GTK mnemonic syntax. Tango labels
// follow the Windows/Electron convention: "&File" underlines F and "&&" is a
// literal ampersand. GTK uses "_" for the mnemonic, so literal underscores
// are doubled.
func mnemonicLabel(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '_':
			b.WriteString("__")
		case '&':
			if i+1 < len(s) && s[i+1] == '&' {
				b.WriteByte('&')
				i++
			} else if i+1 < len(s) {
				b.WriteByte('_')
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeMnemonic makes s display literally in a label parsed for mnemonics.
func escapeMnemonic(s string) string { return strings.ReplaceAll(s, "_", "__") }
