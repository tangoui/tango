// Package share opens the system share sheet with text, a URL and/or
// files: UIActivityViewController on iOS, an ACTION_SEND chooser on
// Android, NSSharingServicePicker on macOS.
//
//	err := share.Share(a, share.Options{Text: "Look at this", URL: "https://example.com"})
//	if errors.Is(err, share.ErrCanceled) { ... }
//
// Share blocks until the sheet is dismissed (iOS, macOS) or shown
// (Android, which does not report what the user did). It returns
// ErrUnsupported where the backend has no share sheet and
// tango.ErrNotRunning after the app quit. On Android call it from a
// goroutine, not from a main-thread callback (see docs/mobile.md).
package share

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported = platform.ErrUnsupported
	ErrCanceled    = platform.ErrCanceled
)

// Options is what to share. At least one of Text, URL and Files is needed.
type Options struct {
	Text  string
	URL   string
	Files []string // paths of existing files
	Title string   // chooser title (Android) / subject for mail
}

// Share shows the share sheet and waits for the user.
func Share(a *tango.App, o Options) error {
	s, err := capcall.Backend[platform.Sharer](a)
	if err != nil {
		return err
	}
	if o.Text == "" && o.URL == "" && len(o.Files) == 0 {
		return errors.New("tango/share: nothing to share")
	}
	var paths []string
	for _, f := range o.Files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("tango/share: %w", err)
		}
		paths = append(paths, abs)
	}
	opts := platform.ShareOptions{Text: o.Text, URL: o.URL, Paths: paths, Title: o.Title}
	_, err = capcall.Async(a, func(done func(struct{}, error)) {
		s.Share(opts, func(err error) { done(struct{}{}, err) })
	})
	return err
}
