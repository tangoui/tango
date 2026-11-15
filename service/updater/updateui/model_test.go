package updateui

import (
	"testing"

	"github.com/tangoui/tango/service/updater"
)

func TestModelEventSequence(t *testing.T) {
	m := &model{current: "1.2.0"}
	rel := &updater.Release{Version: "1.3.0"}
	steps := []struct {
		e    updater.Event
		want phase
	}{
		{updater.Event{Kind: updater.EventChecking}, phaseChecking},
		{updater.Event{Kind: updater.EventAvailable, Release: rel}, phaseAvailable},
		{updater.Event{Kind: updater.EventDownloading, Patch: true}, phaseDownloading},
		{updater.Event{Kind: updater.EventProgress, Done: 50, Total: 200}, phaseDownloading},
		{updater.Event{Kind: updater.EventPatchFailed, Error: "bad patch"}, phaseDownloading},
		{updater.Event{Kind: updater.EventDownloading, Patch: false}, phaseDownloading},
		{updater.Event{Kind: updater.EventProgress, Done: 100, Total: 400}, phaseDownloading},
		{updater.Event{Kind: updater.EventVerified}, phaseReady},
		{updater.Event{Kind: updater.EventReady}, phaseReady},
	}
	for i, s := range steps {
		m.applyEvent(s.e)
		v := m.snapshot()
		if v.phase != s.want {
			t.Fatalf("step %d (%s): phase %v, want %v", i, s.e.Kind, v.phase, s.want)
		}
		switch i {
		case 1:
			if v.rel == nil || v.rel.Version != "1.3.0" {
				t.Fatalf("available: rel %+v", v.rel)
			}
		case 2:
			if !v.patch {
				t.Fatal("downloading a patch: patch false")
			}
		case 3:
			if v.progress() != 0.25 {
				t.Fatalf("progress %v, want 0.25", v.progress())
			}
		case 4:
			if v.patch || v.errText != "" {
				t.Fatalf("patch failed: patch %v err %q", v.patch, v.errText)
			}
		case 6:
			if v.progress() != 0.25 || v.patch {
				t.Fatalf("full download: progress %v patch %v", v.progress(), v.patch)
			}
		}
	}
}

func TestModelUnknownTotal(t *testing.T) {
	m := &model{}
	m.applyEvent(updater.Event{Kind: updater.EventDownloading})
	m.applyEvent(updater.Event{Kind: updater.EventProgress, Done: 10, Total: -1})
	if p := m.snapshot().progress(); p != -1 {
		t.Fatalf("unknown total: progress %v, want -1", p)
	}
	m.applyEvent(updater.Event{Kind: updater.EventProgress, Done: 500, Total: 100})
	if p := m.snapshot().progress(); p != 1 {
		t.Fatalf("done beyond total: progress %v, want 1", p)
	}
}

func TestModelProgressOnlyWhileDownloading(t *testing.T) {
	m := &model{}
	m.applyEvent(updater.Event{Kind: updater.EventVerified})
	m.applyEvent(updater.Event{Kind: updater.EventProgress, Done: 10, Total: 20})
	if v := m.snapshot(); v.phase != phaseReady || v.done != 0 {
		t.Fatalf("late progress changed a finished download: %+v", v)
	}
}

func TestModelErrorEvent(t *testing.T) {
	m := &model{}
	m.applyEvent(updater.Event{Kind: updater.EventError, Error: "boom"})
	if v := m.snapshot(); v.phase != phaseError || v.errText != "boom" {
		t.Fatalf("error event: %+v", v)
	}
}

func TestModelOnChangeCalledOutsideLock(t *testing.T) {
	m := &model{}
	calls := 0
	m.onChange = func() {
		_ = m.snapshot() // would deadlock if called under the lock
		calls++
	}
	m.applyEvent(updater.Event{Kind: updater.EventChecking})
	m.set(func(m *model) { m.manual = true })
	if calls != 2 {
		t.Fatalf("onChange calls %d, want 2", calls)
	}
}
