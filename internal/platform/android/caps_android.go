//go:build android

package android

/*
#include "jni_android.h"
*/
import "C"

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
)

// ClipboardReadText reads the primary clip as text. Android only lets the
// focused foreground app read the clipboard.
func (b *Backend) ClipboardReadText() (string, error) {
	var s string
	var err error
	ok := withEnv(func(e env) {
		var o C.jobject
		o, err = e.callStaticObject(mClipboardRead)
		s = e.gostr(C.jstring(o))
	})
	if !ok && err == nil {
		err = errJava
	}
	return s, err
}

func (b *Backend) ClipboardWriteText(text string) error {
	var err error
	if !withEnv(func(e env) { err = e.callStaticVoid(mClipboardWrite, jvObj(e.str(text))) }) && err == nil {
		err = errJava
	}
	return err
}

// addCallback registers a dialog completion; Java answers through
// nativeDialogDone with the same id. Main thread.
func (b *Backend) addCallback(fn func(button int, paths []string, errMsg string)) int {
	b.nextCB++
	if b.nextCB <= 0 {
		b.nextCB = 1
	}
	b.callbacks[b.nextCB] = fn
	return b.nextCB
}

// MessageDialog shows an AlertDialog. Up to three buttons map to the
// dialog's positive, negative and neutral buttons (in Buttons order); more
// are shown as a list. Dismissing it with back or a tap outside reports
// ErrCanceled.
func (b *Backend) MessageDialog(parent platform.SurfaceID, opts platform.MessageOptions, done func(button int, err error)) {
	buttons := opts.Buttons
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}
	cb := b.addCallback(func(button int, _ []string, errMsg string) {
		switch {
		case errMsg != "":
			done(-1, errors.New(errMsg))
		case button < 0:
			done(-1, platform.ErrCanceled)
		default:
			done(button, nil)
		}
	})
	var err error
	if !withEnv(func(e env) {
		err = e.callStaticVoid(mMessageDialog, jvInt(cb), jvInt(int(opts.Kind)),
			jvObj(e.str(opts.Title)), jvObj(e.str(opts.Message)), jvObj(e.strs(buttons)))
	}) && err == nil {
		err = errJava
	}
	if err != nil {
		delete(b.callbacks, cb)
		done(-1, err)
	}
}

// OpenFileDialog opens the system document picker (ACTION_OPEN_DOCUMENT).
// The chosen documents are copied into the app's cache directory and their
// copies' paths returned, so os.Open works on them. Folders cannot be
// picked (the picker yields tree URIs, not paths): PickFolders reports
// ErrUnsupported.
func (b *Backend) OpenFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(paths []string, err error)) {
	if opts.PickFolders {
		done(nil, platform.ErrUnsupported)
		return
	}
	var patterns []string
	for _, f := range opts.Filters {
		patterns = append(patterns, f.Patterns...)
	}
	cb := b.addCallback(func(_ int, paths []string, errMsg string) {
		switch {
		case errMsg != "":
			done(nil, errors.New(errMsg))
		case paths == nil:
			done(nil, platform.ErrCanceled)
		default:
			done(paths, nil)
		}
	})
	var err error
	if !withEnv(func(e env) {
		err = e.callStaticVoid(mOpenDocuments, jvInt(cb), jvObj(e.str(opts.Title)), jvObj(e.strs(patterns)), jvBool(opts.Multiple))
	}) && err == nil {
		err = errJava
	}
	if err != nil {
		delete(b.callbacks, cb)
		done(nil, err)
	}
}

// SaveFileDialog is not supported: Android's ACTION_CREATE_DOCUMENT yields
// a content URI, not a path. Write into the app's files directory and
// share it instead.
func (b *Backend) SaveFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(path string, err error)) {
	done("", platform.ErrUnsupported)
}
