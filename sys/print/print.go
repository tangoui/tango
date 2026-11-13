// Package print prints through the system print dialog: the page shown in
// a WebView surface, PDF documents and images. It also saves a page as a
// PDF file without any dialog.
//
//	err := print.Page(s, print.Options{Title: "Invoice 42"})
//	if errors.Is(err, print.ErrCanceled) { ... }
//	err = print.PDF(app, pdfBytes, print.Options{Parent: s, Copies: 2})
//	err = print.SavePDF(s, "/tmp/page.pdf")
//
// Every function blocks until the dialog is closed and the job was handed
// to the printing system (not until paper comes out). Called on the main
// thread it keeps the app's event loop running meanwhile (not on Android,
// see docs/mobile.md). Errors: ErrUnsupported where the platform cannot
// do it (see docs/print.md for the matrix), ErrCanceled when the user
// dismissed the dialog and the platform reports it, tango.ErrNotRunning
// after the app quit.
//
// The package name shadows the print builtin in files that import it;
// import it under another name (printing "…/sys/print") if that matters.
package print

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // File and Image accept GIF, JPEG and PNG
	_ "image/jpeg"
	_ "image/png"
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

// Options are the initial values of the print dialog; the user can change
// them there. Fields a platform cannot preset are ignored.
type Options struct {
	// Title names the print job; "" keeps the page title or app name.
	Title string
	// Copies is the initial number of copies; 0 keeps the system default.
	Copies int
	// Parent attaches the dialog of PDF, File and Image to this surface's
	// window where the platform does; nil shows it on its own. Page always
	// uses its own surface.
	Parent *tango.Surface
}

func (o Options) platform() (platform.PrintOptions, error) {
	if o.Copies < 0 {
		return platform.PrintOptions{}, errors.New("tango/print: negative Copies")
	}
	return platform.PrintOptions{Title: o.Title, Copies: o.Copies}, nil
}

// Page prints the page shown in WebView surface s. Canvas surfaces (and
// tango/ui windows) have no page: they get ErrUnsupported, see
// docs/print.md for how to print them.
func Page(s *tango.Surface, o Options) error {
	if s == nil {
		return errors.New("tango/print: nil surface")
	}
	po, err := o.platform()
	if err != nil {
		return err
	}
	a := s.App()
	p, err := capcall.Backend[platform.PagePrinter](a)
	if err != nil {
		return err
	}
	id := s.ID()
	_, err = capcall.Async(a, func(done func(struct{}, error)) {
		p.PrintPage(id, po, func(err error) { done(struct{}{}, err) })
	})
	return err
}

// PDF prints a PDF document given as its bytes.
func PDF(a *tango.App, data []byte, o Options) error {
	if !IsPDF(data) {
		return errors.New("tango/print: data is not a PDF document")
	}
	po, err := o.platform()
	if err != nil {
		return err
	}
	p, err := capcall.Backend[platform.DocumentPrinter](a)
	if err != nil {
		return err
	}
	var parent platform.SurfaceID
	if o.Parent != nil {
		parent = o.Parent.ID()
	}
	_, err = capcall.Async(a, func(done func(struct{}, error)) {
		p.PrintDocument(parent, data, po, func(err error) { done(struct{}{}, err) })
	})
	return err
}

// File prints a PDF document or a GIF, JPEG or PNG image file. Images
// are printed as one page each through PDF (see ImagePDF).
func File(a *tango.App, path string, o Options) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("tango/print: %w", err)
	}
	if IsPDF(data) {
		return PDF(a, data, o)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("tango/print: %s is neither a PDF nor a supported image: %w", filepath.Base(path), err)
	}
	if o.Title == "" {
		o.Title = filepath.Base(path)
	}
	return Image(a, img, o)
}

// Image prints img on one page, scaled to fit the paper by the system.
func Image(a *tango.App, img image.Image, o Options) error {
	doc, err := ImagePDF(img)
	if err != nil {
		return err
	}
	return PDF(a, doc, o)
}

// SavePDF saves the page shown in WebView surface s as a PDF file at path,
// without a dialog. It is tango.Surface.PrintToPDF with default options
// plus writing the file; use PrintToPDF directly for paper size,
// orientation and backgrounds.
func SavePDF(s *tango.Surface, path string) error {
	if s == nil {
		return errors.New("tango/print: nil surface")
	}
	if path == "" {
		return errors.New("tango/print: empty path")
	}
	data, err := s.PrintToPDF(tango.PDFOptions{})
	if err != nil {
		return err
	}
	if !IsPDF(data) {
		return errors.New("tango/print: the WebView returned no PDF")
	}
	return os.WriteFile(path, data, 0o644)
}

// IsPDF reports whether data starts with the PDF header "%PDF-".
func IsPDF(data []byte) bool { return bytes.HasPrefix(data, []byte("%PDF-")) }
