package linux

import (
	"path/filepath"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/rolenames"
)

// GTK enum values used by the dialogs (gtkmessagedialog.h, gtkdialog.h,
// gtkfilechooser.h).
const (
	gtkMessageInfo     = 0
	gtkMessageWarning  = 1
	gtkMessageQuestion = 2
	gtkMessageError    = 3

	gtkResponseDeleteEvent = -4
	gtkResponseAccept      = -3

	gtkFileChooserOpen         = 0
	gtkFileChooserSave         = 1
	gtkFileChooserSelectFolder = 2
)

func gtkMessageType(k platform.MessageKind) int32 {
	switch k {
	case platform.MessageWarning:
		return gtkMessageWarning
	case platform.MessageError:
		return gtkMessageError
	case platform.MessageQuestion:
		return gtkMessageQuestion
	default:
		return gtkMessageInfo
	}
}

// messageButtons returns the button labels in GTK mnemonic syntax, in
// order; button i answers with response id i.
func messageButtons(o platform.MessageOptions) []string {
	if len(o.Buttons) == 0 {
		return []string{rolenames.Text("gtk.ok")}
	}
	out := make([]string, len(o.Buttons))
	for i, b := range o.Buttons {
		out[i] = escapeMnemonic(b)
	}
	return out
}

// messageResult maps a GtkDialog response id to the button index. Negative
// ids (Escape, the window's close button, destroy) mean the dialog was
// dismissed: (-1, ErrCanceled), or (0, nil) for a single-button dialog.
func messageResult(resp int32, buttons int) (int, error) {
	if resp >= 0 && int(resp) < buttons {
		return int(resp), nil
	}
	if buttons <= 1 {
		return 0, nil
	}
	return -1, platform.ErrCanceled
}

func fileChooserAction(o platform.FileOptions, save bool) int32 {
	switch {
	case save:
		return gtkFileChooserSave
	case o.PickFolders:
		return gtkFileChooserSelectFolder
	default:
		return gtkFileChooserOpen
	}
}

// saveLocation splits FileOptions into the chooser's initial folder and
// name: an absolute Filename sets both.
func saveLocation(o platform.FileOptions) (dir, name string) {
	dir, name = o.Directory, o.Filename
	if filepath.IsAbs(name) {
		if dir == "" {
			dir = filepath.Dir(name)
		}
		name = filepath.Base(name)
	}
	return dir, name
}
