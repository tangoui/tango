// Package dialog shows native message and file dialogs.
//
// Each function blocks its caller until the user answers. Called on the
// main thread (from a callback) it keeps the app's event loop running while
// it waits. Errors: ErrCanceled when the user dismissed a file dialog (or
// a message dialog with several buttons),
// ErrUnsupported when the backend has no dialogs, tango.ErrNotRunning after
// the app quit (or if it quits while the dialog is open).
package dialog

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

var (
	// ErrUnsupported is platform.ErrUnsupported.
	ErrUnsupported = platform.ErrUnsupported
	// ErrCanceled is platform.ErrCanceled.
	ErrCanceled = platform.ErrCanceled
)

// Kind is a message dialog's icon / severity.
type Kind = platform.MessageKind

const (
	Info     = platform.MessageInfo
	Warning  = platform.MessageWarning
	Error    = platform.MessageError
	Question = platform.MessageQuestion
)

// FileFilter restricts the files a file dialog offers, e.g.
// {Name: "Images", Patterns: []string{"*.png", "*.jpg"}}.
type FileFilter = platform.FileFilter

// MessageOptions configures Message.
type MessageOptions struct {
	Parent  *tango.Surface // attach to this surface (sheet / owner); nil for none
	Kind    Kind
	Title   string
	Message string
	Buttons []string // empty means a single "OK"
}

// FileOptions configures Open and Save.
type FileOptions struct {
	Parent      *tango.Surface
	Title       string
	Directory   string // initial directory
	Filename    string // Save: default file name
	Filters     []FileFilter
	Multiple    bool // Open: allow several files
	PickFolders bool // Open: choose directories instead of files
	// CanCreateDirs offers a "New Folder" button where the platform
	// dialog lets the app choose (macOS, GTK); Windows always offers it.
	CanCreateDirs bool
	// ShowHidden lists hidden files (dot files, hidden attribute).
	ShowHidden bool
}

func parentID(s *tango.Surface) platform.SurfaceID {
	if s == nil {
		return 0
	}
	return s.ID()
}

func dialogs(a *tango.App) (platform.Dialogs, error) {
	d, ok := a.Backend().(platform.Dialogs)
	if !ok {
		return nil, ErrUnsupported
	}
	return d, nil
}

func (o FileOptions) toPlatform() platform.FileOptions {
	return platform.FileOptions{
		Title:         o.Title,
		Directory:     o.Directory,
		Filename:      o.Filename,
		Filters:       o.Filters,
		Multiple:      o.Multiple,
		PickFolders:   o.PickFolders,
		CanCreateDirs: o.CanCreateDirs,
		ShowHidden:    o.ShowHidden,
	}
}

type result[T any] struct {
	v   T
	err error
}

func await[T any](a *tango.App, start func(done func(T, error))) (T, error) {
	r, err := tango.TryAwaitAsync(a, func(done func(result[T])) {
		start(func(v T, err error) { done(result[T]{v: v, err: err}) })
	})
	if err != nil {
		return r.v, err
	}
	return r.v, r.err
}

// Message shows a message dialog and returns the index of the button the
// user chose (0 for the default "OK").
//
// If the user dismisses the dialog without choosing a button (Esc, the
// close box) Message returns (-1, ErrCanceled), except for a dialog with a
// single button, which reports that button: (0, nil).
func Message(a *tango.App, opts MessageOptions) (int, error) {
	d, err := dialogs(a)
	if err != nil {
		return 0, err
	}
	po := platform.MessageOptions{Kind: opts.Kind, Title: opts.Title, Message: opts.Message, Buttons: opts.Buttons}
	parent := parentID(opts.Parent)
	n, err := await(a, func(done func(int, error)) { d.MessageDialog(parent, po, done) })
	return normalizeDismissal(n, err, len(opts.Buttons))
}

// normalizeDismissal enforces the contract's dismissal rule whatever the
// backend reported: a dismissed multi-button dialog is (-1, ErrCanceled), a
// dismissed single-button dialog is (0, nil).
func normalizeDismissal(n int, err error, buttons int) (int, error) {
	if buttons <= 1 && (err == ErrCanceled || err == nil && n < 0) {
		return 0, nil
	}
	if err == ErrCanceled || err == nil && (n < 0 || n >= buttons) {
		return -1, ErrCanceled
	}
	return n, err
}

// Open shows an open dialog and returns the chosen paths (one unless
// Multiple). It returns ErrCanceled if the user dismissed it.
func Open(a *tango.App, opts FileOptions) ([]string, error) {
	d, err := dialogs(a)
	if err != nil {
		return nil, err
	}
	po, parent := opts.toPlatform(), parentID(opts.Parent)
	return await(a, func(done func([]string, error)) { d.OpenFileDialog(parent, po, done) })
}

// Save shows a save dialog and returns the chosen path. It returns
// ErrCanceled if the user dismissed it.
func Save(a *tango.App, opts FileOptions) (string, error) {
	d, err := dialogs(a)
	if err != nil {
		return "", err
	}
	po, parent := opts.toPlatform(), parentID(opts.Parent)
	return await(a, func(done func(string, error)) { d.SaveFileDialog(parent, po, done) })
}
