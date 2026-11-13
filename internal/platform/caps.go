package platform

import "errors"

// ErrUnsupported is returned by core feature packages when the backend does
// not implement the capability interface.
var ErrUnsupported = errors.New("tango: not supported on this platform")

// ErrCanceled is returned by dialogs the user dismissed.
var ErrCanceled = errors.New("tango: canceled")

// Optional capability interfaces. A backend implements the ones it supports;
// the core discovers them with a type assertion on the Backend value.
// All methods are called on the main thread.

type Clipboard interface {
	ClipboardReadText() (string, error)
	ClipboardWriteText(text string) error
}

type MessageKind int

const (
	MessageInfo MessageKind = iota
	MessageWarning
	MessageError
	MessageQuestion
)

type MessageOptions struct {
	Kind    MessageKind
	Title   string
	Message string
	Buttons []string // empty means a single "OK"
}

// Dismissal rule (all platforms): if the user dismisses a message dialog
// without choosing a button (Esc, close box, tap outside), done receives
// (-1, ErrCanceled) — except a single-button dialog, which reports (0, nil).

type FileFilter struct {
	Name     string   // "Images"
	Patterns []string // "*.png", "*.jpg"
}

type FileOptions struct {
	Title         string
	Directory     string // initial directory
	Filename      string // save dialog default name
	Filters       []FileFilter
	Multiple      bool // open: allow multiple selection
	PickFolders   bool // open: choose directories instead of files
	CanCreateDirs bool // open/save: allow "New Folder"
	ShowHidden    bool
}

// Dialogs are asynchronous: the backend shows the dialog and calls done on
// the main thread exactly once. It must not block the main thread.
type Dialogs interface {
	MessageDialog(parent SurfaceID, opts MessageOptions, done func(button int, err error))
	OpenFileDialog(parent SurfaceID, opts FileOptions, done func(paths []string, err error))
	SaveFileDialog(parent SurfaceID, opts FileOptions, done func(path string, err error))
}

// MenuItem describes a menu tree. Role items are platform standard items
// (Quit, Copy, Paste, ...) whose label/action the backend supplies.
type MenuItem struct {
	ID          uint32 // core-allocated, nonzero for clickable items
	Label       string
	Accelerator string // "CmdOrCtrl+Shift+N"
	Role        string // "", "quit", "about", "hide", "copy", "paste", "cut", "selectAll", "undo", "redo", "minimize", "close", "appMenu", "editMenu", "windowMenu"
	Separator   bool
	Disabled    bool
	Checked     bool
	Checkable   bool   // render as a check item even when !Checked
	RadioGroup  string // non-empty: radio item; items sharing a group are exclusive
	Icon        []byte // PNG, optional; ignored where unsupported
	Submenu     []MenuItem
}

// MenuItem.ID is 32-bit in the contract. Backends with narrower native IDs
// (Win32 WM_COMMAND: 16 bits) keep their own mapping table.

// AppMenu sets the application menu bar (macOS) or the given surface's
// window menu (Windows/Linux; parent==0 means all current and future
// surfaces). Clicks call onClick(id) on the main thread.
type AppMenu interface {
	SetAppMenu(parent SurfaceID, items []MenuItem, onClick func(id uint32))
}
