//go:build darwin && !ios

package darwin

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform/rolenames"
)

// NSEventModifierFlags.
const (
	modShift   = 1 << 17
	modControl = 1 << 18
	modOption  = 1 << 19
	modCommand = 1 << 20
)

// namedKeys maps accelerator key names to AppKit key equivalents
// (NSFunctionKey private-use code points for function keys).
var namedKeys = map[string]string{
	"enter": "\r", "return": "\r",
	"tab":       "\t",
	"space":     " ",
	"backspace": "\b",
	"delete":    "", "del": "",
	"escape": "\x1b", "esc": "\x1b",
	"up": "", "down": "", "left": "", "right": "",
	"home": "", "end": "",
	"pageup": "", "pagedown": "",
	"insert": "",
	"plus":   "+",
}

// parseAccelerator turns "CmdOrCtrl+Shift+N" into an AppKit key equivalent
// and modifier mask. The key is lowercased; Shift is carried in the mask.
func parseAccelerator(acc string) (key string, mods uint, err error) {
	acc = strings.TrimSpace(acc)
	if acc == "" {
		return "", 0, nil
	}
	parts := strings.Split(acc, "+")
	// "CmdOrCtrl++" means the plus key.
	if strings.HasSuffix(acc, "++") {
		parts = append(parts[:len(parts)-2], "+")
	}
	for i, p := range parts {
		p = strings.TrimSpace(p)
		last := i == len(parts)-1
		switch strings.ToLower(p) {
		case "cmd", "command", "super", "meta", "cmdorctrl", "commandorcontrol":
			mods |= modCommand
			continue
		case "ctrl", "control":
			mods |= modControl
			continue
		case "alt", "option", "opt":
			mods |= modOption
			continue
		case "shift":
			mods |= modShift
			continue
		}
		if !last {
			return "", 0, fmt.Errorf("tango/darwin: bad accelerator %q: unknown modifier %q", acc, p)
		}
		lp := strings.ToLower(p)
		if k, ok := namedKeys[lp]; ok {
			key = k
		} else if len(lp) >= 2 && lp[0] == 'f' {
			var n int
			if _, e := fmt.Sscanf(lp, "f%d", &n); e == nil && n >= 1 && n <= 35 {
				key = string(rune(0xF704 + n - 1)) // NSF1FunctionKey...
			}
		}
		if key == "" {
			if utf8.RuneCountInString(p) != 1 {
				return "", 0, fmt.Errorf("tango/darwin: bad accelerator %q: unknown key %q", acc, p)
			}
			key = strings.ToLower(p)
		}
	}
	if key == "" {
		return "", 0, fmt.Errorf("tango/darwin: bad accelerator %q: no key", acc)
	}
	return key, mods, nil
}

// roleInfo describes a standard menu item.
type roleInfo struct {
	selector string // sent to the first responder (target nil)
	label    string // rolenames key; "%s" in its text is the app name
	accel    string
}

var menuRoles = map[string]roleInfo{
	"about":            {"orderFrontStandardAboutPanel:", "mac.about", ""},
	"hide":             {"hide:", "mac.hide", "Cmd+H"},
	"hideOthers":       {"hideOtherApplications:", "mac.hideOthers", "Cmd+Alt+H"},
	"unhide":           {"unhideAllApplications:", "mac.unhide", ""},
	"quit":             {"terminate:", "mac.quit", "Cmd+Q"},
	"undo":             {"undo:", "mac.undo", "Cmd+Z"},
	"redo":             {"redo:", "mac.redo", "Cmd+Shift+Z"},
	"cut":              {"cut:", "mac.cut", "Cmd+X"},
	"copy":             {"copy:", "mac.copy", "Cmd+C"},
	"paste":            {"paste:", "mac.paste", "Cmd+V"},
	"delete":           {"delete:", "mac.delete", ""},
	"selectAll":        {"selectAll:", "mac.selectAll", "Cmd+A"},
	"minimize":         {"performMiniaturize:", "mac.minimize", "Cmd+M"},
	"zoom":             {"performZoom:", "mac.zoom", ""},
	"close":            {"performClose:", "mac.close", "Cmd+W"},
	"front":            {"arrangeInFront:", "mac.front", ""},
	"togglefullscreen": {"toggleFullScreen:", "mac.togglefullscreen", "Ctrl+Cmd+F"},
}

// roleMenus are the submenu roles the backend fills itself when the item
// has no Submenu of its own.
var roleMenus = map[string]struct {
	title string   // rolenames key
	items []string // "" is a separator
}{
	"appMenu":    {"mac.appMenu", []string{"about", "", "hide", "hideOthers", "unhide", "", "quit"}},
	"editMenu":   {"mac.editMenu", []string{"undo", "redo", "", "cut", "copy", "paste", "selectAll"}},
	"windowMenu": {"mac.windowMenu", []string{"minimize", "zoom", "", "front", "", "close"}},
}

// roleLabel localizes a rolenames key (any other text is used as is) and
// substitutes the app name.
func roleLabel(format, appName string) string {
	format = rolenames.Text(format)
	if strings.Contains(format, "%s") {
		return strings.ReplaceAll(format, "%s", appName)
	}
	return format
}
