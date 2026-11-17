//go:build darwin

package metal

import (
	"errors"
	"image"
	"slices"
	"testing"
	"time"

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

// TestMetalPresentSharesFrameCommandBuffer: a presenter Device leaves the
// frame's command buffer open after Execute, and present appends the
// drawable copy to it and commits it, so a presented frame is one command
// buffer (one submission the next Execute retires before reusing the
// frame's instance and staging buffers).
func TestMetalPresentSharesFrameCommandBuffer(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[0].Make(t)
	size := devicePixels(gputest.SceneSize, 1)
	state := func() (open bool, kinds []submissionKind, presents []bool, commits int) {
		p.dev.mu.Lock()
		defer p.dev.mu.Unlock()
		for _, s := range p.dev.inflight.items {
			presents = append(presents, s.presents)
		}
		return p.dev.openCB != 0, p.dev.inflight.kinds(), presents, p.dev.commits
	}
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	p.dev.mu.Lock()
	p.dev.drawables = drawableBackoff{} // the common case: drawables are not scarce
	p.dev.mu.Unlock()
	open, kinds, _, commits := state()
	if !open || len(kinds) != 0 || commits != 0 {
		t.Fatalf("after Render: open=%v in flight=%v commits=%d, want an open frame, nothing committed", open, kinds, commits)
	}
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	open, kinds, presents, commits := state()
	if open || !slices.Equal(kinds, []submissionKind{submitFrame}) || !slices.Equal(presents, []bool{true}) || commits != 1 || p.presented != 1 {
		t.Fatalf("after present: open=%v in flight=%v presents=%v commits=%d presented=%d, want one presenting frame",
			open, kinds, presents, commits, p.presented)
	}
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if open, kinds, _, _ = state(); !open || len(kinds) != 0 {
		t.Fatalf("after Render/present/Render: open=%v in flight=%v, want the presented frame retired and a new open frame", open, kinds)
	}
	// A frame nobody presents is committed by the next Execute.
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if open, _, _, commits = state(); !open || commits != 2 {
		t.Fatalf("after an unpresented frame and Render: open=%v commits=%d, want the old frame committed", open, commits)
	}
	// Wait submits the open frame.
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
	if open, kinds, _, commits = state(); open || len(kinds) != 0 || commits != 3 {
		t.Fatalf("after Wait: open=%v in flight=%v commits=%d", open, kinds, commits)
	}
	// present with nothing open (presenting the same frame again) uses a
	// command buffer of its own.
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	if _, kinds, presents, _ = state(); !slices.Equal(kinds, []submissionKind{submitPresent}) || !slices.Equal(presents, []bool{false}) {
		t.Fatalf("re-present: in flight=%v presents=%v, want one present", kinds, presents)
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
}

// TestDrawableBackoff: short nextDrawable waits keep the shared command
// buffer; a long one splits the next splitPresents presents, and another
// long wait restarts the count.
func TestDrawableBackoff(t *testing.T) {
	var b drawableBackoff
	b.observe(0)
	b.observe(drawableWaitSplit) // not over the threshold
	if b.split() {
		t.Fatal("split without a long drawable wait")
	}
	b.observe(drawableWaitSplit + time.Microsecond)
	for i := 0; i < splitPresents-10; i++ {
		if !b.split() {
			t.Fatalf("present %d after a long wait not split", i)
		}
	}
	b.observe(5 * time.Millisecond) // still starved: restart
	n := 0
	for b.split() {
		n++
	}
	if n != splitPresents {
		t.Fatalf("%d presents split after the second long wait, want %d", n, splitPresents)
	}
	b.observe(100 * time.Microsecond)
	if b.split() {
		t.Fatal("split after the count ran out with short waits")
	}
}

// TestMetalPresentSplitsAfterDrawableWait: after a long nextDrawable wait
// present commits the open frame first and presents in a command buffer
// of its own; when the count runs out it shares the frame's again.
func TestMetalPresentSplitsAfterDrawableWait(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[0].Make(t)
	size := devicePixels(gputest.SceneSize, 1)
	state := func() (kinds []submissionKind, presents []bool) {
		p.dev.mu.Lock()
		defer p.dev.mu.Unlock()
		for _, s := range p.dev.inflight.items {
			presents = append(presents, s.presents)
		}
		return p.dev.inflight.kinds(), presents
	}
	p.dev.mu.Lock()
	p.dev.drawables.observe(5 * time.Millisecond)
	p.dev.mu.Unlock()
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	if kinds, presents := state(); !slices.Equal(kinds, []submissionKind{submitFrame, submitPresent}) || !slices.Equal(presents, []bool{false, false}) || p.presented != 1 {
		t.Fatalf("split present: in flight %v presents %v presented %d, want [frame present], no shared buffer", kinds, presents, p.presented)
	}
	p.dev.mu.Lock()
	p.dev.drawables = drawableBackoff{} // the count ran out
	p.dev.mu.Unlock()
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(size, 1); err != nil {
		t.Fatal(err)
	}
	kinds, presents := state()
	if n := len(kinds); n == 0 || kinds[n-1] != submitFrame || !presents[n-1] {
		t.Fatalf("after the backoff: in flight %v presents %v, want the shared frame last", kinds, presents)
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
}

// TestMetalPresentWithoutDrawableCommitsFrame: every present that shows
// nothing (empty size, no drawable) still submits the open frame.
func TestMetalPresentWithoutDrawableCommitsFrame(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	l := gputest.Scenes[0].Make(t)
	open := func() bool {
		p.dev.mu.Lock()
		defer p.dev.mu.Unlock()
		return p.dev.openCB != 0
	}
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(image.Point{}, 1); err != nil {
		t.Fatal(err)
	}
	if open() {
		t.Fatal("a zero-size present left the frame uncommitted")
	}
	if err := p.r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	msgSetID(layer, sel("setDevice:"), 0) // nextDrawable returns nil
	if err := p.present(devicePixels(gputest.SceneSize, 1), 1); err != nil {
		t.Fatal(err)
	}
	if open() || p.presented != 0 {
		t.Fatalf("present without a drawable: open=%v presented=%d", open(), p.presented)
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
}

// TestMetalSharedFrameTimeline: a presented frame's single command buffer
// reports a frame Timing (with GPU times) and then a present Timing.
func TestMetalSharedFrameTimeline(t *testing.T) {
	layer := newTestLayer(t, image.Pt(10, 10))
	p := startPresenter(t, layer)
	var got []Timing
	p.dev.SetTimeline(func(t Timing) { got = append(got, t) })
	if err := p.r.Render(gputest.Scenes[0].Make(t), gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if err := p.present(devicePixels(gputest.SceneSize, 1), 1); err != nil {
		t.Fatal(err)
	}
	if err := p.dev.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != "frame" || got[1].Kind != "present" {
		t.Fatalf("timeline %+v, want frame then present", got)
	}
	if got[0].Drawable || !got[1].Drawable || got[0].Frame != got[1].Frame || got[0].Commit != got[1].Commit {
		t.Fatalf("timeline %+v: frame without drawable, present with it, same frame and commit", got)
	}
	if !got[0].GPUKnown {
		t.Errorf("frame Timing without GPU times: %+v", got[0])
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
// did; the count resets only once a frame has retired with every one of
// its submissions completed (known when the next frame's first submission
// retires).
func TestMetalFailureCountedOncePerFrame(t *testing.T) {
	d := &Device{frames: 1}
	boom := errors.New("boom")
	d.noteRetired(submission{kind: submitFrame, frame: 1}, boom, false)
	d.noteRetired(submission{kind: submitPresent, frame: 1}, boom, false)
	if d.failures != 1 || d.pendingErr != boom || d.hasFrame {
		t.Fatalf("failures=%d pending=%v hasFrame=%v, want 1/boom/false", d.failures, d.pendingErr, d.hasFrame)
	}
	d.frames = 2
	d.noteRetired(submission{kind: submitFrame, frame: 2}, nil, false)
	if d.failures != 1 {
		t.Fatalf("frame 2's draw alone reset failures to %d", d.failures)
	}
	d.noteRetired(submission{kind: submitPresent, frame: 2}, nil, false)
	d.frames = 3
	d.noteRetired(submission{kind: submitFrame, frame: 3}, nil, false)
	if d.failures != 0 {
		t.Fatalf("a fully successful frame 2 left failures=%d", d.failures)
	}
}

// TestMetalAlternatingFailuresLoseDevice (review Important 1): presents
// that keep failing while every draw succeeds must still add up to a lost
// device. Steady-state retirement order is F(k-1), P(k-1), F(k), ... with
// P(k-1) retired after F(k) was committed.
func TestMetalAlternatingFailuresLoseDevice(t *testing.T) {
	d := &Device{}
	boom := errors.New("present failed")
	for k := uint64(1); k <= maxFailures+1 && d.lost == nil; k++ {
		d.frames = k
		d.noteRetired(submission{kind: submitFrame, frame: k}, nil, false)
		d.frames = k + 1 // the next frame is committed before P(k) retires
		d.noteRetired(submission{kind: submitPresent, frame: k}, boom, false)
	}
	if !errors.Is(d.lost, ErrDeviceLost) {
		t.Fatalf("failures=%d lost=%v: failing presents never lost the device", d.failures, d.lost)
	}
}

// TestMetalStaleFailureKeepsNewFrame (review Important 2): once frame N's
// failure was counted and frame N+1 committed, a late failure of frame N's
// present neither counts again nor clears frame N+1's hasFrame/pendingErr.
func TestMetalStaleFailureKeepsNewFrame(t *testing.T) {
	d := &Device{frames: 1}
	boom := errors.New("boom")
	d.noteRetired(submission{kind: submitFrame, frame: 1}, boom, false)
	d.pendingErr = nil             // Execute reported it
	d.frames, d.hasFrame = 2, true // frame 2 committed
	d.noteRetired(submission{kind: submitPresent, frame: 1}, boom, false)
	if d.failures != 1 || !d.hasFrame || d.pendingErr != nil {
		t.Fatalf("stale failure: failures=%d hasFrame=%v pending=%v, want 1/true/nil", d.failures, d.hasFrame, d.pendingErr)
	}
}
