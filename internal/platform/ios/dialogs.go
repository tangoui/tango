//go:build ios

package ios

/*
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"strings"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

func (b *backend) ClipboardReadText() (string, error) {
	p := C.tg_clipboard_read()
	if p == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), nil
}

func (b *backend) ClipboardWriteText(text string) error {
	t, free := cstring(text)
	defer free()
	C.tg_clipboard_write(t)
	return nil
}

var errNoPresenter = errors.New("tango/ios: no page on screen to present the dialog from")

// Pending dialog results, by token. Main thread only.
var (
	alertDone  = map[int]func(int){}
	pickerDone = map[int]func([]string, bool){}
)

// addDialog registers cancel (which must call the caller's done with
// ErrCanceled) and returns the token and a function ending the dialog.
func (b *backend) addDialog(cancel func()) (int, func()) {
	b.nextTok++
	tok := b.nextTok
	b.dialogs[tok] = cancel
	return tok, func() {
		delete(b.dialogs, tok)
		delete(alertDone, tok)
		delete(pickerDone, tok)
	}
}

// cancelDialogs answers every showing dialog with ErrCanceled and takes it
// off screen, so each done runs exactly once even when the app quits.
func (b *backend) cancelDialogs() {
	if len(b.dialogs) == 0 {
		return
	}
	for len(b.dialogs) > 0 {
		for _, cancel := range b.dialogs {
			cancel()
		}
	}
	C.tg_dismiss_dialogs()
}

// MessageDialog shows a UIAlertController. Kind has no visual counterpart
// on iOS. Alerts cannot be dismissed without a button, so ErrCanceled only
// comes from the app quitting.
func (b *backend) MessageDialog(parent platform.SurfaceID, opts platform.MessageOptions, done func(int, error)) {
	if b.quitting {
		done(-1, platform.ErrCanceled)
		return
	}
	buttons := opts.Buttons
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	var tok int
	var end func()
	tok, end = b.addDialog(func() { end(); done(-1, platform.ErrCanceled) })
	alertDone[tok] = func(i int) { end(); done(i, nil) }

	title, f1 := cstring(opts.Title)
	defer f1()
	msg, f2 := cstring(opts.Message)
	defer f2()
	btns, f3 := cstring(strings.Join(buttons, "\n"))
	defer f3()
	if C.tg_alert(C.int(tok), title, msg, btns) != 0 {
		end()
		done(-1, errNoPresenter)
	}
}

// OpenFileDialog shows a UIDocumentPickerViewController. Picked files are
// imported as copies into the app's sandbox (tmp/), and the copies' paths
// are returned. With PickFolders the folder's real path is returned; it is
// security-scoped outside the sandbox. Title is not shown on iOS.
func (b *backend) OpenFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func([]string, error)) {
	if b.quitting {
		done(nil, platform.ErrCanceled)
		return
	}
	var tok int
	var end func()
	tok, end = b.addDialog(func() { end(); done(nil, platform.ErrCanceled) })
	pickerDone[tok] = func(paths []string, canceled bool) {
		end()
		if canceled {
			done(nil, platform.ErrCanceled)
			return
		}
		done(paths, nil)
	}

	exts, f1 := cstring(strings.Join(extensions(opts.Filters), "\n"))
	defer f1()
	dir, f2 := cstring(opts.Directory)
	defer f2()
	if C.tg_pick(C.int(tok), exts, cbool(opts.Multiple), cbool(opts.PickFolders), dir) != 0 {
		end()
		done(nil, errNoPresenter)
	}
}

// SaveFileDialog is unsupported: iOS has no "choose where to save" panel
// (UIDocumentPickerViewController can only export a file that already
// exists). Write into the app's sandbox instead.
func (b *backend) SaveFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(string, error)) {
	done("", platform.ErrUnsupported)
}

//export tangoIOSAlertDone
func tangoIOSAlertDone(tok, button C.int) {
	if fn := alertDone[int(tok)]; fn != nil {
		fn(int(button))
	}
}

//export tangoIOSPickerDone
func tangoIOSPickerDone(tok C.int, buf *C.char, n, canceled C.int) {
	fn := pickerDone[int(tok)]
	if fn == nil {
		return
	}
	var paths []string
	if n > 0 {
		for _, p := range bytes.Split(C.GoBytes(unsafe.Pointer(buf), n), []byte{0}) {
			if len(p) > 0 {
				paths = append(paths, string(p))
			}
		}
	}
	fn(paths, canceled != 0)
}

// extensions flattens filters to extensions without dots; nil (any file)
// when a filter accepts everything.
func extensions(filters []platform.FileFilter) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range filters {
		for _, p := range f.Patterns {
			p = strings.TrimSpace(p)
			if p == "*" || p == "*.*" {
				return nil
			}
			e := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(p, "*"), "."))
			if e == "" || strings.ContainsAny(e, "*?/") || seen[e] {
				continue
			}
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func cbool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
