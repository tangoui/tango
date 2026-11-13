//go:build linux && !android

package linux

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
)

const (
	gtkDialogModal             = 1 << 0
	gtkDialogDestroyWithParent = 1 << 1
	gtkButtonsNone             = 0
)

// Dialogs never run a nested loop (gtk_dialog_run): they are shown and
// answer through their "response" signal, so the main thread is never
// blocked. Each dialog's state lives in its handler closures; done runs
// exactly once, guarded by fired, and late signals after teardown find no
// dispatch entry.

func (b *backend) MessageDialog(parent platform.SurfaceID, o platform.MessageOptions, done func(int, error)) {
	d := gtkMessageDialogNew(b.parentWindow(parent), gtkDialogModal|gtkDialogDestroyWithParent,
		gtkMessageType(o.Kind), gtkButtonsNone, "%s", o.Message)
	if d == 0 {
		done(-1, errors.New("tango/linux: gtk_message_dialog_new failed"))
		return
	}
	if o.Title != "" {
		gtkWindowSetTitle(d, o.Title)
	}
	buttons := messageButtons(o)
	for i, label := range buttons {
		gtkDialogAddButton(d, label, int32(i))
	}
	fired := false
	connect(d, "response", 1, func(_, resp ptr) ptr {
		if fired {
			return 0
		}
		fired = true
		btn, err := messageResult(int32(resp), len(buttons))
		gtkWidgetDestroy(d)
		done(btn, err)
		return 0
	})
	// DESTROY_WITH_PARENT destroys the dialog without a response when the
	// parent window closes.
	connect(d, "destroy", 0, func(_, _ ptr) ptr {
		if !fired {
			fired = true
			done(messageResult(gtkResponseDeleteEvent, len(buttons)))
		}
		return 0
	})
	gtkWindowPresent(d)
}

func (b *backend) OpenFileDialog(parent platform.SurfaceID, o platform.FileOptions, done func([]string, error)) {
	b.fileDialog(parent, o, false, done)
}

func (b *backend) SaveFileDialog(parent platform.SurfaceID, o platform.FileOptions, done func(string, error)) {
	b.fileDialog(parent, o, true, func(paths []string, err error) {
		if err == nil && len(paths) == 0 {
			err = platform.ErrCanceled
		}
		if err != nil {
			done("", err)
			return
		}
		done(paths[0], nil)
	})
}

// fileDialog shows a GtkFileChooserNative (the portal dialog under Flatpak
// / Snap, GTK's own otherwise).
func (b *backend) fileDialog(parent platform.SurfaceID, o platform.FileOptions, save bool, done func([]string, error)) {
	action := fileChooserAction(o, save)
	fc := gtkFileChooserNativeNew(cstr(o.Title), b.parentWindow(parent), action, nil, nil)
	if fc == 0 {
		done(nil, errors.New("tango/linux: gtk_file_chooser_native_new failed"))
		return
	}
	if save {
		gtkFileChooserSetDoOverwriteConf(fc, true)
		dir, name := saveLocation(o)
		if dir != "" {
			gtkFileChooserSetCurrentFolder(fc, dir)
		}
		if name != "" {
			gtkFileChooserSetCurrentName(fc, name)
		}
	} else {
		gtkFileChooserSetSelectMultiple(fc, o.Multiple)
		if o.Directory != "" {
			gtkFileChooserSetCurrentFolder(fc, o.Directory)
		}
	}
	setChooserFlags(fc, o.ShowHidden, o.CanCreateDirs)
	if !o.PickFolders || save {
		for _, f := range o.Filters {
			filter := gtkFileFilterNew()
			gtkFileFilterSetName(filter, f.Name)
			for _, p := range f.Patterns {
				gtkFileFilterAddPattern(filter, p)
			}
			gtkFileChooserAddFilter(fc, filter) // sinks the floating ref
		}
	}
	fired := false
	connect(fc, "response", 1, func(_, resp ptr) ptr {
		if fired {
			return 0
		}
		fired = true
		var paths []string
		var err error = platform.ErrCanceled
		if int32(resp) == gtkResponseAccept {
			paths, err = slistStrings(gtkFileChooserGetFilenames(fc)), nil
		}
		// Drop our reference outside the emission; finalizing disconnects
		// this handler and frees its dispatch entry.
		later(0, func() { gObjectUnref(fc) })
		done(paths, err)
		return 0
	})
	gtkNativeDialogSetModal(fc, true)
	gtkNativeDialogShow(fc)
}
