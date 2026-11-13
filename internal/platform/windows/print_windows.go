//go:build windows && (amd64 || arm64)

package windows

// Printing (platform/print_ext.go), written against Microsoft's WebView2
// and Shell documentation:
//
//   - PrintPage: ICoreWebView2_16::ShowPrintUI(COREWEBVIEW2_PRINT_DIALOG_KIND_SYSTEM)
//     (WebView2 runtime 1.0.1518+). It opens the dialog and returns at
//     once without reporting what the user chose, so done receives nil
//     right away and PrintOptions are not applied (the dialog has no
//     presets). Older runtimes fall back to the page's window.print(),
//     which shows WebView2's own print preview.
//   - PrintDocument: ShellExecuteW with the "print" verb on a temporary
//     copy of the PDF. That asks the app registered for PDF files to print
//     it; whether it shows a dialog is up to that app, and PrintOptions
//     are not passed. Without a registered "print" verb (the default for
//     Microsoft Edge as the PDF viewer) the result is ErrUnsupported.

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.PagePrinter     = (*Backend)(nil)
	_ platform.DocumentPrinter = (*Backend)(nil)
)

var iidICoreWebView2_16 = guid("0EB34DC9-9F91-41E1-8639-95CD5943906B")

const (
	// ICoreWebView2_16 (WebView2.h): Print = 113, ShowPrintUI = 114,
	// PrintToPdfStream = 115; ICoreWebView2_15 ends with GetFavicon = 112.
	wv16ShowPrintUI = 114

	printDialogKindSystem = 1 // COREWEBVIEW2_PRINT_DIALOG_KIND_SYSTEM

	seErrNoAssoc = 31 // SE_ERR_NOASSOC: no application for the verb

	// printFileLifetime is how long a temporary PDF handed to another app
	// is kept: ShellExecute returns before that app has read it.
	printFileLifetime = 10 * time.Minute
)

func (b *Backend) PrintPage(sid platform.SurfaceID, _ platform.PrintOptions, done func(error)) {
	s := b.byID[sid]
	if s == nil || s.closed || s.canvas != nil {
		done(platform.ErrUnsupported)
		return
	}
	s.webCall(done, func(claim func() bool) {
		if !claim() {
			return
		}
		wv16 := queryInterface(s.webview, &iidICoreWebView2_16)
		if wv16 == 0 {
			s.Eval("window.print()")
			done(nil)
			return
		}
		defer release(wv16)
		if hr := comCall(wv16, wv16ShowPrintUI, printDialogKindSystem); failed(hr) {
			done(hresultError("ICoreWebView2_16::ShowPrintUI", hr))
			return
		}
		done(nil)
	})
}

func (b *Backend) PrintDocument(parent platform.SurfaceID, pdf []byte, _ platform.PrintOptions, done func(error)) {
	f, err := os.CreateTemp("", "tango-print-*.pdf")
	if err != nil {
		done(err)
		return
	}
	path := f.Name()
	_, err = f.Write(pdf)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		done(err)
		return
	}
	verb, file := u16("print"), u16(path)
	r, _, _ := ixShellExecuteW.Call(b.ownerOf(parent), uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swShowNormal)
	switch {
	case r == seErrNoAssoc:
		os.Remove(path)
		done(fmt.Errorf("no app registered to print PDF files: %w", platform.ErrUnsupported))
	case r <= 32: // HINSTANCE > 32 means success
		os.Remove(path)
		done(fmt.Errorf("tango: ShellExecuteW(print) failed (%d)", r))
	default:
		time.AfterFunc(printFileLifetime, func() { os.Remove(path) })
		done(nil)
	}
}
