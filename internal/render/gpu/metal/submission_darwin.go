//go:build darwin

package metal

import "time"

// asyncInflightLimit bounds the command buffers (frames and presents
// together) the Device keeps committed but not yet retired.
const asyncInflightLimit = 2

// submissionKind tells what a committed command buffer does, so Execute
// retires exactly the work whose CPU-written resources it reuses.
type submissionKind uint8

const (
	submitOther   submissionKind = iota // readback, test references: retired before the next frame
	submitFrame                         // Execute: reads the shared instance/staging buffers
	submitPresent                       // present: reads the target only (GPU-ordered on the queue)
)

func (k submissionKind) String() string {
	switch k {
	case submitFrame:
		return "frame"
	case submitPresent:
		return "present"
	}
	return "other"
}

// submission is one committed, not yet retired command buffer.
type submission struct {
	id     uint64
	kind   submissionKind
	frame  uint64 // Execute frame it belongs to (a present: the frame it shows)
	cb     id     // retained until retirement
	retire func() error

	// Diagnostics (filled only while a timeline callback is installed).
	commit       time.Time
	drawableWait time.Duration
	presents     bool // a frame whose command buffer also presents (deferFrameCommit)
}

// submissionQueue holds submissions in commit order; retirement is FIFO.
type submissionQueue struct {
	limit int
	items []submission
}

func newSubmissionQueue(limit int) *submissionQueue {
	return &submissionQueue{limit: max(limit, 1)}
}

func (q *submissionQueue) push(s submission) { q.items = append(q.items, s) }
func (q *submissionQueue) len() int          { return len(q.items) }
func (q *submissionQueue) needOldest() bool  { return len(q.items) >= q.limit }

// waitOldest retires the oldest submission and returns its error.
func (q *submissionQueue) waitOldest() error {
	if len(q.items) == 0 {
		return nil
	}
	s := q.items[0]
	q.items[0] = submission{}
	q.items = q.items[1:]
	if s.retire == nil {
		return nil
	}
	return s.retire()
}

// waitAll retires every submission in order and returns the first error.
func (q *submissionQueue) waitAll() error {
	var first error
	for len(q.items) > 0 {
		if err := q.waitOldest(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// hasNonPresent reports whether a frame or other submission is in flight.
func (q *submissionQueue) hasNonPresent() bool {
	for _, s := range q.items {
		if s.kind != submitPresent {
			return true
		}
	}
	return false
}

// kinds lists the in-flight kinds in commit order (tests).
func (q *submissionQueue) kinds() []submissionKind {
	ks := make([]submissionKind, len(q.items))
	for i, s := range q.items {
		ks[i] = s.kind
	}
	return ks
}
