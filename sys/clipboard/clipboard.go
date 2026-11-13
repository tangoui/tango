// Package clipboard reads and writes the system clipboard: text, images
// (PNG bytes) and file lists.
//
// Functions are safe from any goroutine once the app runs. They return
// platform.ErrUnsupported (errors.Is(err, clipboard.ErrUnsupported)) when the
// backend has no clipboard, and tango.ErrNotRunning after the app quit.
package clipboard

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is platform.ErrUnsupported, re-exported for callers
// outside the module.
var ErrUnsupported = platform.ErrUnsupported

func backend(a *tango.App) (platform.Clipboard, error) {
	c, ok := a.Backend().(platform.Clipboard)
	if !ok {
		return nil, ErrUnsupported
	}
	return c, nil
}

type result struct {
	text string
	err  error
	ran  bool
}

// ReadText returns the clipboard's text ("" if it holds none).
func ReadText(a *tango.App) (string, error) {
	c, err := backend(a)
	if err != nil {
		return "", err
	}
	r := tango.Await(a, func() result {
		s, err := c.ClipboardReadText()
		return result{s, err, true}
	})
	if !r.ran {
		return "", tango.ErrNotRunning
	}
	return r.text, r.err
}

// WriteText replaces the clipboard's content with text.
func WriteText(a *tango.App, text string) error {
	c, err := backend(a)
	if err != nil {
		return err
	}
	r := tango.Await(a, func() result { return result{err: c.ClipboardWriteText(text), ran: true} })
	if !r.ran {
		return tango.ErrNotRunning
	}
	return r.err
}
