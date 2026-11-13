//go:build darwin && !ios

package darwin

import "errors"

const pasteboardTypeString = "public.utf8-plain-text" // NSPasteboardTypeString

func (b *backend) ClipboardReadText() (string, error) {
	var s string
	withPool(func() {
		pb := send(class("NSPasteboard"), "generalPasteboard")
		s = goString(send(pb, "stringForType:", uintptr(nsString(pasteboardTypeString))))
	})
	return s, nil
}

func (b *backend) ClipboardWriteText(text string) error {
	ok := false
	withPool(func() {
		pb := send(class("NSPasteboard"), "generalPasteboard")
		send(pb, "clearContents")
		ok = sendBool(pb, "setString:forType:", uintptr(nsString(text)), uintptr(nsString(pasteboardTypeString)))
	})
	if !ok {
		return errors.New("tango/darwin: cannot write to the pasteboard")
	}
	return nil
}
