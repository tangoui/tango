package platform

// Contract extension: printing. Like ext.go everything here is OPTIONAL;
// the core (sys/print) finds support with a type assertion on the Backend
// and reports ErrUnsupported otherwise. Nothing changes what the minimum
// Backend must do.
//
// Threading is the backend.go rule: methods are called on the main thread
// and done must be called exactly once, on the main thread, either before
// the method returns or later.
//
// Exporting a page as a PDF file is not part of this file: it is WebPDF
// (webview_ext.go), which sys/print.SavePDF uses.

// PrintOptions are the initial values of the system print dialog. The user
// can change them there. Fields a platform cannot preset are ignored.
type PrintOptions struct {
	// Title names the print job (the print queue, "Print to File" default
	// name). "" keeps the platform's choice: the page title or app name.
	Title string
	// Copies is the initial number of copies; 0 keeps the system default.
	Copies int
}

// PagePrinter prints the page shown in a WebView surface through the
// system print dialog.
//
// done receives nil once the job was handed to the printing system (not
// when paper comes out), ErrCanceled if the user dismissed the dialog and
// the platform tells, or another error. Canvas surfaces, closed and
// unknown surfaces get ErrUnsupported.
type PagePrinter interface {
	PrintPage(surface SurfaceID, opts PrintOptions, done func(error))
}

// DocumentPrinter prints a PDF document through the system print dialog.
// pdf is a complete PDF file (the core checks the %PDF- header and turns
// images into PDF before calling). The dialog is attached to parent where
// the platform attaches dialogs to windows; parent 0 or an unknown id
// shows it on its own. done reports as for PagePrinter.
//
// The backend must not keep pdf after done; it copies what it needs (to a
// temporary file, for example) and removes such copies itself.
type DocumentPrinter interface {
	PrintDocument(parent SurfaceID, pdf []byte, opts PrintOptions, done func(error))
}
