//go:build linux && !android

package linux

// Printing (platform/print_ext.go), written against the WebKitGTK and
// GTK 3 API references:
//
//   - PrintPage: webkit_print_operation_new + webkit_print_operation_run_dialog
//     (WebKitGTK 2.x). run_dialog is modal (a nested main loop) and tells
//     whether the user chose Print; done then waits for the operation's
//     "finished" / "failed" signals. Copies preset with
//     gtk_print_settings_set_n_copies; Title becomes the "output-basename"
//     of the Print to File printer (WebKit names the job after the page).
//   - PrintDocument: GtkPrintUnixDialog (shown without a nested loop,
//     answered through "response") picks the printer and settings, then a
//     GtkPrintJob sends a temporary copy of the PDF as its source file
//     (gtk_print_job_set_source_file, gtk_print_job_send). Printers that do
//     not take PDF (gtk_printer_accepts_pdf false) get ErrUnsupported; CUPS
//     printers and Print to File take it.
//
// The symbols are bound on first use with the package's binder, so a
// missing one only turns printing off (ErrUnsupported).

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.PagePrinter     = (*backend)(nil)
	_ platform.DocumentPrinter = (*backend)(nil)
)

const (
	gtkResponseOK                 = -5
	gtkPrintCapabilityGeneratePDF = 1 << 5 // GTK_PRINT_CAPABILITY_GENERATE_PDF
)

var pr struct {
	once   sync.Once
	pageOK bool // webkit print operation with settings
	docOK  bool // GtkPrintUnixDialog + GtkPrintJob

	opNew        func(v ptr) ptr
	opRunDialog  func(op, parent ptr) int32
	opSetSetting func(op, settings ptr)
	psNew        func() ptr
	psSet        func(s ptr, key, value string)
	psSetCopies  func(s ptr, n int32)

	dlgNew        func(title string, parent ptr) ptr
	dlgSetCaps    func(d ptr, caps uint32)
	dlgSetSetting func(d, settings ptr)
	dlgGetSetting func(d ptr) ptr // new reference
	dlgPrinter    func(d ptr) ptr // not owned
	dlgPageSetup  func(d ptr) ptr // not owned
	windowModal   func(w ptr, modal bool)
	transientFor  func(w, parent ptr)
	acceptsPDF    func(p ptr) bool
	jobNew        func(title string, printer, settings, setup ptr) ptr
	jobSetSource  func(job ptr, filename string, gerr *ptr) bool
	jobSend       func(job, cb, data, notify ptr)
	errorFree     func(e ptr)

	cbJob ptr // GtkPrintJobCompleteFunc (job, user_data, error)
	jobs  map[ptr]func(gerr ptr)
	seq   ptr
}

func bindPrint() {
	pr.once.Do(func() {
		var bd binder
		t, w, g := libGTK, libWebKit, libGLib
		psOK := bd.opt(&pr.psNew, "gtk_print_settings_new", t) &&
			bd.opt(&pr.psSet, "gtk_print_settings_set", t) &&
			bd.opt(&pr.psSetCopies, "gtk_print_settings_set_n_copies", t)
		pr.pageOK = psOK &&
			bd.opt(&pr.opNew, "webkit_print_operation_new", w) &&
			bd.opt(&pr.opRunDialog, "webkit_print_operation_run_dialog", w) &&
			bd.opt(&pr.opSetSetting, "webkit_print_operation_set_print_settings", w)
		pr.docOK = psOK &&
			bd.opt(&pr.dlgNew, "gtk_print_unix_dialog_new", t) &&
			bd.opt(&pr.dlgSetCaps, "gtk_print_unix_dialog_set_manual_capabilities", t) &&
			bd.opt(&pr.dlgSetSetting, "gtk_print_unix_dialog_set_settings", t) &&
			bd.opt(&pr.dlgGetSetting, "gtk_print_unix_dialog_get_settings", t) &&
			bd.opt(&pr.dlgPrinter, "gtk_print_unix_dialog_get_selected_printer", t) &&
			bd.opt(&pr.dlgPageSetup, "gtk_print_unix_dialog_get_page_setup", t) &&
			bd.opt(&pr.windowModal, "gtk_window_set_modal", t) &&
			bd.opt(&pr.transientFor, "gtk_window_set_transient_for", t) &&
			bd.opt(&pr.acceptsPDF, "gtk_printer_accepts_pdf", t) &&
			bd.opt(&pr.jobNew, "gtk_print_job_new", t) &&
			bd.opt(&pr.jobSetSource, "gtk_print_job_set_source_file", t) &&
			bd.opt(&pr.jobSend, "gtk_print_job_send", t) &&
			bd.opt(&pr.errorFree, "g_error_free", g)
		if pr.docOK {
			pr.jobs = map[ptr]func(ptr){}
			pr.cbJob = purego.NewCallback(func(_, data, gerr ptr) {
				fn := pr.jobs[data]
				delete(pr.jobs, data)
				if fn != nil {
					fn(gerr)
				}
			})
		}
	})
}

// printSettings returns a new GtkPrintSettings with opts applied, or 0
// when opts presets nothing.
func printSettings(opts platform.PrintOptions) ptr {
	if opts.Copies <= 0 && opts.Title == "" {
		return 0
	}
	s := pr.psNew()
	if opts.Copies > 0 {
		pr.psSetCopies(s, int32(opts.Copies))
	}
	if opts.Title != "" {
		pr.psSet(s, "output-basename", opts.Title)
	}
	return s
}

func (b *backend) PrintPage(sid platform.SurfaceID, opts platform.PrintOptions, done func(error)) {
	bindPrint()
	s := b.surfaces[sid]
	if s == nil || s.closed || s.web == 0 || !pr.pageOK {
		done(platform.ErrUnsupported)
		return
	}
	op := pr.opNew(s.web)
	if op == 0 {
		done(errors.New("tango/linux: webkit_print_operation_new failed"))
		return
	}
	if ps := printSettings(opts); ps != 0 {
		pr.opSetSetting(op, ps) // the operation keeps its own reference
		gObjectUnref(ps)
	}
	fired, failMsg := false, ""
	finish := func(err error) {
		if fired {
			return
		}
		fired = true
		gObjectUnref(op)
		done(err)
	}
	connect(op, "failed", 1, func(_, gerr ptr) ptr {
		if failMsg = gErrorText(gerr); failMsg == "" {
			failMsg = "printing failed"
		}
		return 0
	})
	connect(op, "finished", 0, func(_, _ ptr) ptr {
		later(0, func() { // not from inside the emission
			if failMsg != "" {
				finish(errors.New("tango/linux: " + failMsg))
				return
			}
			finish(nil)
		})
		return 0
	})
	if pr.opRunDialog(op, s.win) != webkitPrintResponsePrint {
		finish(platform.ErrCanceled)
	}
}

func (b *backend) PrintDocument(parent platform.SurfaceID, pdf []byte, opts platform.PrintOptions, done func(error)) {
	bindPrint()
	if !pr.docOK {
		done(platform.ErrUnsupported)
		return
	}
	title := opts.Title
	if title == "" {
		title = "Document"
	}
	dlg := pr.dlgNew(title, b.parentWindow(parent))
	if dlg == 0 {
		done(errors.New("tango/linux: gtk_print_unix_dialog_new failed"))
		return
	}
	pr.dlgSetCaps(dlg, gtkPrintCapabilityGeneratePDF)
	if ps := printSettings(opts); ps != 0 {
		pr.dlgSetSetting(dlg, ps)
		gObjectUnref(ps)
	}
	if p := b.parentWindow(parent); p != 0 {
		pr.transientFor(dlg, p)
	}
	pr.windowModal(dlg, true)

	fired := false
	connect(dlg, "response", 1, func(_, resp ptr) ptr {
		if fired {
			return 0
		}
		fired = true // before gtk_widget_destroy, which emits "destroy"
		if int32(resp) != gtkResponseOK {
			gtkWidgetDestroy(dlg)
			done(platform.ErrCanceled)
			return 0
		}
		printer := pr.dlgPrinter(dlg)
		if printer == 0 || !pr.acceptsPDF(printer) {
			gtkWidgetDestroy(dlg)
			done(fmt.Errorf("the selected printer does not accept PDF: %w", platform.ErrUnsupported))
			return 0
		}
		settings := pr.dlgGetSetting(dlg)
		job := pr.jobNew(title, printer, settings, pr.dlgPageSetup(dlg)) // takes its own references
		gObjectUnref(settings)
		gtkWidgetDestroy(dlg)
		sendJob(job, pdf, done)
		return 0
	})
	connect(dlg, "destroy", 0, func(_, _ ptr) ptr {
		if !fired { // destroyed with its parent
			fired = true
			done(platform.ErrCanceled)
		}
		return 0
	})
	gtkWindowPresent(dlg)
}

// sendJob spools pdf through a temporary file and calls done when GTK has
// handed the job to the printing system.
func sendJob(job ptr, pdf []byte, done func(error)) {
	if job == 0 {
		done(errors.New("tango/linux: gtk_print_job_new failed"))
		return
	}
	f, err := os.CreateTemp("", "tango-print-*.pdf")
	if err != nil {
		gObjectUnref(job)
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
		gObjectUnref(job)
		done(err)
		return
	}
	var gerr ptr
	if !pr.jobSetSource(job, path, &gerr) {
		msg := gErrorText(gerr)
		if gerr != 0 {
			pr.errorFree(gerr)
		}
		os.Remove(path)
		gObjectUnref(job)
		done(errors.New("tango/linux: " + msg))
		return
	}
	pr.seq++
	key := pr.seq
	pr.jobs[key] = func(gerr ptr) {
		msg := gErrorText(gerr) // owned by GTK
		later(0, func() {
			os.Remove(path)
			gObjectUnref(job)
			if msg != "" {
				done(errors.New("tango/linux: " + msg))
				return
			}
			done(nil)
		})
	}
	pr.jobSend(job, pr.cbJob, key, 0)
}
