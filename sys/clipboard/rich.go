package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"path/filepath"
	"slices"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

// ErrNotPNG is returned by WriteImage when the bytes are not a PNG image.
var ErrNotPNG = errors.New("clipboard: image is not a PNG")

func rich(a *tango.App) (platform.ClipboardRich, error) {
	c, ok := a.Backend().(platform.ClipboardRich)
	if !ok {
		return nil, ErrUnsupported
	}
	return c, nil
}

type richResult struct {
	data  []byte
	paths []string
	err   error
	ran   bool
}

func awaitRich(a *tango.App, fn func(platform.ClipboardRich) richResult) richResult {
	c, err := rich(a)
	if err != nil {
		return richResult{err: err, ran: true}
	}
	r := tango.Await(a, func() richResult {
		r := fn(c)
		r.ran = true
		return r
	})
	if !r.ran {
		return richResult{err: tango.ErrNotRunning}
	}
	return r
}

// ReadImage returns the clipboard's image encoded as PNG, converting from
// the platform's native image format when needed. It returns nil (and no
// error) when the clipboard holds no image.
func ReadImage(a *tango.App) ([]byte, error) {
	r := awaitRich(a, func(c platform.ClipboardRich) richResult {
		b, err := c.ClipboardReadImage()
		return richResult{data: b, err: err}
	})
	if len(r.data) == 0 {
		r.data = nil
	}
	return r.data, r.err
}

// WriteImage replaces the clipboard's content with a PNG image. Backends
// also offer it in the platform's native image format so other apps can
// paste it.
func WriteImage(a *tango.App, pngData []byte) error {
	if _, err := png.DecodeConfig(bytes.NewReader(pngData)); err != nil {
		return ErrNotPNG
	}
	return awaitRich(a, func(c platform.ClipboardRich) richResult {
		return richResult{err: c.ClipboardWriteImage(pngData)}
	}).err
}

// ReadFiles returns the paths of files copied in the file manager (Finder,
// Explorer, Files), or nil when the clipboard holds none.
func ReadFiles(a *tango.App) ([]string, error) {
	r := awaitRich(a, func(c platform.ClipboardRich) richResult {
		p, err := c.ClipboardReadFiles()
		return richResult{paths: p, err: err}
	})
	if len(r.paths) == 0 {
		r.paths = nil
	}
	return r.paths, r.err
}

// ErrNotAbsolute is returned by WriteFiles for a path that is not
// absolute.
var ErrNotAbsolute = errors.New("clipboard: file paths must be absolute")

// WriteFiles replaces the clipboard's content with a list of files, as
// copying them in the file manager does; pasting in Finder, Explorer or
// Files then copies them. Paths must be absolute; an empty list clears the
// clipboard.
func WriteFiles(a *tango.App, paths []string) error {
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("%w: %q", ErrNotAbsolute, p)
		}
	}
	paths = slices.Clone(paths)
	return awaitRich(a, func(c platform.ClipboardRich) richResult {
		return richResult{err: c.ClipboardWriteFiles(paths)}
	}).err
}
