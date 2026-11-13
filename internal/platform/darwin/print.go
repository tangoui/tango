//go:build darwin && !ios

package darwin

// Printing (platform/print_ext.go), written against Apple's AppKit, WebKit
// and PDFKit documentation:
//
//   - PrintPage: -[WKWebView printOperationWithPrintInfo:] (macOS 11+)
//     run with -[NSPrintOperation runOperationModalForWindow:delegate:
//     didRunSelector:contextInfo:] as a sheet on the surface's window.
//   - PrintDocument: -[PDFDocument printOperationForPrintInfo:scalingMode:
//     autoRotate:] (PDFKit, macOS 10.4+), as a sheet on the parent's window,
//     or app-modal with -runOperation when there is no usable parent.
//
// The print info is a copy of +[NSPrintInfo sharedPrintInfo] so presets
// (copies) do not leak into later jobs. The did-run selector reports
// success == NO both for Cancel and for a failed job; both become
// platform.ErrCanceled, as AppKit does not tell them apart.

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.PagePrinter     = (*backend)(nil)
	_ platform.DocumentPrinter = (*backend)(nil)
)

// PDFPrintScalingMode (PDFKit/PDFDocument.h).
const pdfPrintPageScaleDownToFit = 2

var (
	printDelegateOnce sync.Once
	printDelegate     id

	// printRuns maps a did-run contextInfo token to its completion. Main
	// thread only.
	printRuns   = map[uintptr]func(success bool){}
	printNextID uintptr

	pdfKitOnce sync.Once
	pdfKitOK   bool
)

func ensurePrintDelegate() id {
	printDelegateOnce.Do(func() {
		// - (void)printOperationDidRun:(NSPrintOperation *)op success:(BOOL)success contextInfo:(void *)ctx
		didRun := func(_ id, _ objc.SEL, _ id, success bool, ctx uintptr) {
			fn := printRuns[ctx]
			delete(printRuns, ctx)
			if fn != nil {
				fn(success)
			}
		}
		classDef("TangoPrintDelegate", "NSObject", nil, []objc.MethodDef{
			method("printOperationDidRun:success:contextInfo:", didRun),
		})
		printDelegate = alloc("TangoPrintDelegate") // kept for the life of the process
	})
	return printDelegate
}

// newPrintInfo returns an autoreleased copy of the shared print info with
// opts applied.
func newPrintInfo(opts platform.PrintOptions) id {
	info := autorelease(send(send(class("NSPrintInfo"), "sharedPrintInfo"), "copy"))
	if opts.Copies > 0 {
		if key := constPtr(libAppKit, "NSPrintCopies"); key != 0 {
			n := send(class("NSNumber"), "numberWithInteger:", uintptr(opts.Copies))
			send(send(info, "dictionary"), "setObject:forKey:", uintptr(n), key)
		}
	}
	return info
}

// runPrint shows op's print panel and calls done when the operation ends:
// as a sheet on win, or app-modal (blocking in a nested modal loop) when
// win is 0. keep is retained until then (the operation's source object).
func runPrint(op, win, keep id, opts platform.PrintOptions, done func(error)) {
	if opts.Title != "" {
		send(op, "setJobTitle:", uintptr(nsString(opts.Title)))
	}
	send(op, "setShowsPrintPanel:", boolArg(true))
	send(op, "setShowsProgressPanel:", boolArg(true))
	finish := func(success bool) {
		if success {
			done(nil)
		} else {
			done(platform.ErrCanceled)
		}
	}
	if win == 0 {
		finish(sendBool(op, "runOperation"))
		return
	}
	retain(op)
	retain(keep)
	printNextID++
	token := printNextID
	printRuns[token] = func(success bool) {
		release(op)
		release(keep)
		finish(success)
	}
	send(op, "runOperationModalForWindow:delegate:didRunSelector:contextInfo:",
		uintptr(win), uintptr(ensurePrintDelegate()), uintptr(sel("printOperationDidRun:success:contextInfo:")), token)
}

func (b *backend) PrintPage(sid platform.SurfaceID, opts platform.PrintOptions, done func(error)) {
	s := b.surfaces[sid]
	if s == nil || s.closed || s.web == 0 || !respondsTo(s.web, "printOperationWithPrintInfo:") {
		done(platform.ErrUnsupported)
		return
	}
	// WebKit's print operation needs a window to run on (-runOperation is
	// not supported for it), and a window shows one sheet at a time.
	win := b.parentWindow(sid)
	if win == 0 {
		done(errors.New("tango/darwin: the window is hidden or already shows a sheet"))
		return
	}
	withPool(func() {
		op := send(s.web, "printOperationWithPrintInfo:", uintptr(newPrintInfo(opts)))
		if op == 0 {
			done(errors.New("tango/darwin: WKWebView returned no print operation"))
			return
		}
		// WebKit's print view starts with a zero frame and prints blank
		// pages unless it is given one.
		if v := send(op, "view"); v != 0 {
			msgSetRect(v, sel("setFrame:"), msgRect(s.web, sel("bounds")))
		}
		runPrint(op, win, s.web, opts, done)
	})
}

func loadPDFKit() bool {
	pdfKitOnce.Do(func() {
		_, err := purego.Dlopen(frameworksDir+"PDFKit.framework/PDFKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		pdfKitOK = err == nil && hasClass("PDFDocument")
	})
	return pdfKitOK
}

func (b *backend) PrintDocument(parent platform.SurfaceID, pdf []byte, opts platform.PrintOptions, done func(error)) {
	if !loadPDFKit() {
		done(platform.ErrUnsupported)
		return
	}
	withPool(func() {
		doc := autorelease(send(send(class("PDFDocument"), "alloc"), "initWithData:", uintptr(nsData(pdf))))
		if doc == 0 || sendInt(doc, "pageCount") == 0 {
			done(errors.New("tango/darwin: PDFKit could not read the document"))
			return
		}
		if !respondsTo(doc, "printOperationForPrintInfo:scalingMode:autoRotate:") {
			done(platform.ErrUnsupported)
			return
		}
		op := send(doc, "printOperationForPrintInfo:scalingMode:autoRotate:",
			uintptr(newPrintInfo(opts)), pdfPrintPageScaleDownToFit, boolArg(true))
		if op == 0 {
			done(errors.New("tango/darwin: PDFKit returned no print operation"))
			return
		}
		runPrint(op, b.parentWindow(parent), doc, opts, done)
	})
}
