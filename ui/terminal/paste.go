package terminal

import "github.com/tangoui/tango/ui/terminal/vt"

// OnPaste intercepts nonempty text from native Paste, keyboard paste
// shortcuts and [ViewW.Paste]. The handler runs on the UI thread with
// the emulator unlocked. Call submit once on the UI thread, immediately
// or after confirmation, with the original or rewritten text. Omitting
// the call cancels the paste. A nil handler submits the original text.
func (w *ViewW) OnPaste(fn func(text string, submit func(string))) *ViewW {
	w.onPaste = fn
	return w
}

// Paste requests a paste through the view's OnPaste handler. The accepted
// text is encoded using the terminal's bracketed-paste mode at submission.
func (w *ViewW) Paste(text string) {
	if text == "" {
		return
	}
	submitted := false
	submit := func(value string) {
		if submitted {
			return
		}
		submitted = true
		if value == "" {
			return
		}
		if exited, _ := w.t.Exited(); exited {
			return
		}
		v := w.t.Lock()
		b := vt.EncodePaste(value, v.Modes())
		if w.t.sendInputLocked(b, false).AcceptedBytes > 0 {
			v.Screen().SetViewOffset(0)
			if w.vs.term == w.t {
				w.vs.sel.clear()
			}
		}
		w.t.Unlock()
		w.c.Invalidator()()
	}
	if w.onPaste != nil {
		w.onPaste(text, submit)
	} else {
		submit(text)
	}
}
