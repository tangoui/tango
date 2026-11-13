package fake

import (
	"errors"
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Printing (platform/print_ext.go). The fake records every print request
// and answers with the error set by SetPrintErr. Like sysx.go its state
// lives beside Backend, keyed by it.

var (
	_ platform.PagePrinter     = (*Backend)(nil)
	_ platform.DocumentPrinter = (*Backend)(nil)
)

// PrintJob records one PrintPage or PrintDocument call. PDF is nil for a
// page.
type PrintJob struct {
	Surface platform.SurfaceID // the page's surface, or the document's parent
	PDF     []byte
	Opts    platform.PrintOptions
}

type printState struct {
	mu   sync.Mutex
	jobs []PrintJob
	err  error
}

var printStates sync.Map // *Backend -> *printState

func (b *Backend) prints() *printState {
	p, _ := printStates.LoadOrStore(b, &printState{})
	return p.(*printState)
}

// SetPrintErr sets what later print requests answer (nil: printed;
// platform.ErrCanceled: the user dismissed the dialog).
func (b *Backend) SetPrintErr(err error) {
	p := b.prints()
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()
}

// PrintJobs returns every print request, in order.
func (b *Backend) PrintJobs() []PrintJob {
	p := b.prints()
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.jobs)
}

func (b *Backend) recordPrint(j PrintJob) error {
	p := b.prints()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.jobs = append(p.jobs, j)
	return p.err
}

func (b *Backend) PrintPage(id platform.SurfaceID, opts platform.PrintOptions, done func(error)) {
	b.mustMain("PrintPage")
	s := b.Surface(id)
	if s == nil || s.printClosed() || b.printCanvas(id) {
		done(platform.ErrUnsupported)
		return
	}
	err := b.recordPrint(PrintJob{Surface: id, Opts: opts})
	b.enqueue(func() { done(err) }) // asynchronous, like a dialog
}

func (b *Backend) PrintDocument(parent platform.SurfaceID, pdf []byte, opts platform.PrintOptions, done func(error)) {
	b.mustMain("PrintDocument")
	if len(pdf) == 0 {
		done(errors.New("fake: empty document"))
		return
	}
	err := b.recordPrint(PrintJob{Surface: parent, PDF: slices.Clone(pdf), Opts: opts})
	b.enqueue(func() { done(err) })
}

func (s *Surface) printClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (b *Backend) printCanvas(id platform.SurfaceID) bool {
	cfg := b.canvasCfg()
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	_, ok := cfg.canvases[id]
	return ok
}
