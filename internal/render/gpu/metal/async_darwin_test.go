//go:build darwin

package metal

import (
	"errors"
	"image"
	"slices"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
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

// TestMetalExecuteKeepsPresentInFlight: Execute retires the frame whose
// shared instance/staging buffers it is about to overwrite, but not the
// later present copy (GPU ordering on the queue protects the target).
func TestMetalExecuteKeepsPresentInFlight(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[0].Make(t)
	size := devicePixels(gputest.SceneSize, 1)
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	p.dev.mu.Lock()
	kinds := p.dev.inflight.kinds()
	p.dev.mu.Unlock()
	if !slices.Equal(kinds, []submissionKind{submitPresent, submitFrame}) {
		t.Fatalf("in flight after Render/present/Render = %v, want [present frame]", kinds)
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
}

// TestMetalPresentRetiresOldestOnly: presents without Execute in between
// are bounded by the shared cap and retire only the oldest submission.
func TestMetalPresentRetiresOldestOnly(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	size := devicePixels(gputest.SceneSize, 1)
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := p.present(size, 1); err != nil {
			t.Fatal(err)
		}
		p.dev.mu.Lock()
		n := len(p.dev.inflight.items)
		p.dev.mu.Unlock()
		if n > asyncInflightLimit {
			t.Fatalf("%d submissions in flight, cap %d", n, asyncInflightLimit)
		}
	}
	p.dev.mu.Lock()
	kinds := p.dev.inflight.kinds()
	p.dev.mu.Unlock()
	if !slices.Equal(kinds, []submissionKind{submitPresent, submitPresent}) {
		t.Fatalf("in flight = %v, want the two newest presents", kinds)
	}
}

// TestMetalFailureCountedOncePerFrame: a failed frame and its failed
// present count as one failure toward device loss, as the old batch wait
// did; a later successful frame resets the count, a present does not.
func TestMetalFailureCountedOncePerFrame(t *testing.T) {
	d := &Device{}
	boom := errors.New("boom")
	d.noteRetired(submission{kind: submitFrame, frame: 1}, boom, false)
	d.noteRetired(submission{kind: submitPresent, frame: 1}, boom, false)
	if d.failures != 1 || d.pendingErr != boom || d.hasFrame {
		t.Fatalf("failures=%d pending=%v hasFrame=%v, want 1/boom/false", d.failures, d.pendingErr, d.hasFrame)
	}
	d.noteRetired(submission{kind: submitPresent, frame: 2}, nil, false)
	if d.failures != 1 {
		t.Fatalf("a successful present reset failures to %d", d.failures)
	}
	d.noteRetired(submission{kind: submitFrame, frame: 2}, nil, false)
	if d.failures != 0 {
		t.Fatalf("a successful later frame left failures=%d", d.failures)
	}
}
