//go:build darwin

package metal

import (
	"errors"
	"slices"
	"testing"
)

func TestSubmissionQueueBounded(t *testing.T) {
	var retired []int
	q := newSubmissionQueue(2)
	q.push(submission{id: 1, retire: func() error { retired = append(retired, 1); return nil }})
	q.push(submission{id: 2, retire: func() error { retired = append(retired, 2); return nil }})
	if q.needOldest() == false || q.len() != 2 {
		t.Fatalf("queue len=%d needOldest=%v, want 2/true", q.len(), q.needOldest())
	}
	if err := q.waitOldest(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(retired, []int{1}) || q.len() != 1 {
		t.Fatalf("retired=%v len=%d, want [1]/1", retired, q.len())
	}
}

func TestSubmissionQueueWaitAllRetiresInOrder(t *testing.T) {
	var retired []int
	q := newSubmissionQueue(2)
	for i := 1; i <= 2; i++ {
		i := i
		q.push(submission{id: uint64(i), retire: func() error { retired = append(retired, i); return nil }})
	}
	if err := q.waitAll(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(retired, []int{1, 2}) || q.len() != 0 {
		t.Fatalf("retired=%v len=%d, want [1 2]/0", retired, q.len())
	}
}

func TestSubmissionQueueOldestErrorLeavesLaterWork(t *testing.T) {
	want := errors.New("oldest failed")
	q := newSubmissionQueue(2)
	q.push(submission{id: 1, retire: func() error { return want }})
	q.push(submission{id: 2, retire: func() error { return nil }})
	if err := q.waitOldest(); !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
	if q.len() != 1 {
		t.Fatalf("queue len=%d, want later submission retained", q.len())
	}
	if err := q.waitAll(); err != nil {
		t.Fatalf("later submission: %v", err)
	}
}
