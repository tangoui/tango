package ui

// ShortcutCaptureWindow is an optional host capability for shortcut recorders.
// While capture is on, window shortcuts (including those enabled in text
// fields) and native menu accelerators must yield to the focused canvas.
// This is not text input: it must not enable IME or a software keyboard.
// System-reserved keys and global OS hotkeys are outside this capability.
// The runner releases capture on completion, cancel, blur or removal.
// Calls are made on the UI thread, only when the requested state changes.
type ShortcutCaptureWindow interface {
	SetShortcutCapture(on bool)
}

func (r *Runner) sendShortcutCapture() {
	c := r.c
	on := c.shortcutCaptureNext != 0 && c.shortcutCaptureNext == c.focus && !c.winBlurred
	c.shortcutCapture = 0
	if on {
		c.shortcutCapture = c.shortcutCaptureNext
	}
	if on != r.shortcutCaptureOn {
		r.shortcutCaptureOn = on
		if w, ok := r.win.(ShortcutCaptureWindow); ok {
			w.SetShortcutCapture(on)
		}
	}
}
