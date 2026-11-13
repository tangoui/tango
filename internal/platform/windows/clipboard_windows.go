//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"time"
	"unsafe"
)

// Plain-text clipboard access through CF_UNICODETEXT. The clipboard is a
// shared resource that one window at a time may open; the app window is
// the owner on this side.

// openClipboard opens the clipboard for the app window. Another process
// may be holding it for a moment, so it tries ten times, 5 ms apart.
func (b *Backend) openClipboard() error {
	var lastErr error
	for range 10 {
		ok, _, err := procOpenClipboard.Call(b.appHwnd.Load())
		if ok != 0 {
			return nil
		}
		lastErr = err
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("tango: OpenClipboard: %v", lastErr)
}

// withClipboard runs fn with the clipboard open and closes it afterwards.
func (b *Backend) withClipboard(fn func() error) error {
	if err := b.openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	return fn()
}

func (b *Backend) ClipboardReadText() (text string, err error) {
	err = b.withClipboard(func() error {
		if avail, _, _ := procIsClipboardFormatAvailable.Call(cfUnicodeText); avail == 0 {
			return nil // no text on the clipboard: ""
		}
		mem, _, callErr := procGetClipboardData.Call(cfUnicodeText)
		if mem == 0 {
			return fmt.Errorf("tango: GetClipboardData: %v", callErr)
		}
		// The clipboard owns mem; lock it only for as long as it is read.
		p, _, callErr := procGlobalLock.Call(mem)
		if p == 0 {
			return fmt.Errorf("tango: GlobalLock: %v", callErr)
		}
		defer procGlobalUnlock.Call(mem)
		bytes, _, _ := procGlobalSize.Call(mem)
		text = wstrN(p, int(bytes/2))
		return nil
	})
	return text, err
}

// globalText copies text, NUL-terminated, into a new movable global
// memory block, the form SetClipboardData expects.
func globalText(text string) (uintptr, error) {
	units := utf16z(text)
	mem, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(2*len(units)))
	if mem == 0 {
		return 0, fmt.Errorf("tango: GlobalAlloc: %v", err)
	}
	p, _, err := procGlobalLock.Call(mem)
	if p == 0 {
		procGlobalFree.Call(mem)
		return 0, fmt.Errorf("tango: GlobalLock: %v", err)
	}
	copy(unsafe.Slice((*uint16)(native(p)), len(units)), units)
	procGlobalUnlock.Call(mem)
	return mem, nil
}

func (b *Backend) ClipboardWriteText(text string) error {
	mem, err := globalText(text)
	if err != nil {
		return err
	}
	// Until SetClipboardData succeeds the block is still ours to free.
	if err := b.openClipboard(); err != nil {
		procGlobalFree.Call(mem)
		return err
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	if ok, _, err := procSetClipboardData.Call(cfUnicodeText, mem); ok == 0 {
		procGlobalFree.Call(mem)
		return fmt.Errorf("tango: SetClipboardData: %v", err)
	}
	return nil
}
