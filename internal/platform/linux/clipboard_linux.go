//go:build linux && !android

package linux

// GDK_SELECTION_CLIPBOARD is _GDK_MAKE_ATOM (69) (gdkselection.h): a
// predefined atom, so no gdk_atom_intern call is needed.
const gdkSelectionClipboard = 69

// ClipboardReadText uses gtk_clipboard_wait_for_text. The contract's read is
// synchronous, so an async gtk_clipboard_request_text would need a nested
// loop anyway; wait_for_text is GTK's own nested GMainLoop, which keeps the
// UI responsive while the owner answers (other events, including Drain,
// can run during it — the same reentrancy as Step). When the app itself
// owns the clipboard GTK answers without a round trip. No text (or a
// non-text clipboard) reads as "".
func (b *backend) ClipboardReadText() (string, error) {
	cb := gtkClipboardGet(gdkSelectionClipboard)
	return takeStr(gtkClipboardWaitForText(cb)), nil
}

// ClipboardWriteText sets the CLIPBOARD selection. gtk_clipboard_store is
// not called: it can block in a nested loop for seconds waiting on a
// clipboard manager, and gtk_main stores all clipboards when it exits.
func (b *backend) ClipboardWriteText(text string) error {
	cb := gtkClipboardGet(gdkSelectionClipboard)
	gtkClipboardSetText(cb, text, int32(len(text)))
	return nil
}
