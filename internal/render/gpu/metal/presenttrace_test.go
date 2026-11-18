package metal

import "testing"

func TestPresentationTraceRecordsIdentityAndLate(t *testing.T) {
	q := make(chan presentationRecord, 8)
	tr := newPresentationTrace(9, q)
	a := tr.begin(7, 11, 100)
	b := tr.begin(7, 12, 110) // same render frame, different present
	tr.finish(b, 0, 130)      // skipped must not become a timestamp
	tr.close()
	tr.finish(a, 120, 140) // late callback cannot touch a closed Device
	first, second, late := <-q, <-q, <-q
	if first.Present != 2 || first.Frame != 7 || first.Drawable != 12 || first.PresentedNS != 0 || first.Status != "skipped" {
		t.Fatalf("skipped identity changed: %+v", first)
	}
	if second.Status != "closed" || late.Status != "late" || late.Session != 9 || late.PresentedNS != 120 || late.Present != 1 {
		t.Fatalf("close/late: %+v %+v", second, late)
	}
}

func TestPresentationTraceBoundedAndCloseIdempotent(t *testing.T) {
	q := make(chan presentationRecord, 1)
	tr := newPresentationTrace(2, q)
	a := tr.begin(1, 0, 10)
	tr.finish(a, 20, 25)
	tr.finish(a, 0, 26) // overflow cannot block the callback
	if tr.dropped.Load() != 1 {
		t.Fatal("overflow not counted")
	}
	<-q
	tr.close()
	if r := <-q; r.Dropped != 1 || r.Status != "closed" {
		t.Fatalf("%+v", r)
	}
	tr.close()
	if len(q) != 0 {
		t.Fatal("Close emitted twice")
	}
}
