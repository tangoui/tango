//go:build darwin

package metal

import "slices"

const asyncInflightLimit = 2

type submission struct {
	id     uint64
	cb     id
	retire func() error
}

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

func (q *submissionQueue) waitOldest() error {
	if len(q.items) == 0 {
		return nil
	}
	s := q.items[0]
	q.items = q.items[1:]
	if s.retire == nil {
		return nil
	}
	return s.retire()
}

func (q *submissionQueue) waitAll() error {
	var first error
	for len(q.items) > 0 {
		if err := q.waitOldest(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (q *submissionQueue) ids() []uint64 {
	ids := make([]uint64, len(q.items))
	for i, s := range q.items {
		ids[i] = s.id
	}
	return slices.Clone(ids)
}
