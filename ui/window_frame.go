package ui

// WindowFrame is implemented by windows whose frame the page can drive:
// a frameless window with a self-drawn TitleBar asks it to move, to change
// state and to close. uihost windows implement it; TitleBar and
// WindowControls find it on Context.Window, or take one explicitly
// (TitleBarW.Frame) for other hosts and for tests.
//
// All methods run on the UI thread.
type WindowFrame interface {
	// FrameInfo describes the window's current state and buttons.
	FrameInfo() WindowFrameInfo
	// SetWindowState minimizes, maximizes, makes full screen or restores
	// the window.
	SetWindowState(s WindowState)
	// CloseWindow asks the window to close, as its system close button
	// would (close handlers may still veto it).
	CloseWindow()
	// StartWindowMove hands the primary-button press that is in progress
	// to the window manager, which moves the window until the button is
	// released. Call it while handling the press.
	StartWindowMove()
}

// WindowState is the state of a window.
//
// Append-only: never reorder or insert; the zero value stays WindowNormal.
type WindowState uint8

const (
	WindowNormal WindowState = iota
	WindowMinimized
	WindowMaximized
	WindowFullscreen
)

// WindowFrameInfo is what a WindowFrame reports about its window.
type WindowFrameInfo struct {
	_ [0]func()

	State WindowState
	// SystemButtons is true when the platform draws the window buttons
	// over the page itself (macOS traffic lights over a hidden title bar):
	// a title bar leaves room for them and draws none.
	SystemButtons bool
	// SystemTitleBar is true when the window keeps its system title bar
	// (and buttons): a TitleBar in the page draws no window buttons.
	SystemTitleBar bool
	// NoMinimize, NoMaximize and NoClose disable those buttons (the
	// window's appearance forbids the action).
	NoMinimize, NoMaximize, NoClose bool
}

// windowFrame returns the frame of the window the context draws into, or
// nil.
func (c *Context) windowFrame() WindowFrame {
	if f, ok := c.win.(WindowFrame); ok {
		return f
	}
	return nil
}
