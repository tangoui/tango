package uihost

import (
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
)

// A closed host (surface closed, canvas detached) refuses actions without
// reaching the runner.
func TestA11yActionAfterClose(t *testing.T) {
	r := ui.NewRunner(func(c *ui.Context) { ui.Button(c, "OK") }, ui.RunnerOptions{})
	h := &host{r: r}
	h.closed = true
	if err := h.AccessibilityAction(1, a11y.Press, ""); err != errA11yClosed {
		t.Fatalf("closed host: err %v", err)
	}
	if err := (&host{}).AccessibilityAction(1, a11y.Press, ""); err != errA11yClosed {
		t.Fatalf("host without runner: err %v", err)
	}
	var b *a11yBridge // nil-safe helpers
	b.close()
	b.tap(func(*a11y.Tree, []a11y.NodeID) {})()
	b.force()()
}

func TestThreadedNativeAXPublishesInOneMainTask(t *testing.T) {
	first := &a11y.Tree{}
	var tasks []func()
	b := a11yBridge{postNative: func(fn func()) { tasks = append(tasks, fn) }}
	seen := false
	b.publishNative(first, func() {
		seen = true
		if b.published.Load() != first {
			t.Error("action validation was not published before native notification")
		}
	})
	if seen || b.published.Load() != nil || len(tasks) != 1 {
		t.Fatal("publication escaped its main task")
	}
	tasks[0]()
	b.publishNative(nil, func() {
		if b.published.Load() != nil {
			t.Error("withdrawal left stale action validation")
		}
	})
	if !seen || b.published.Load() != first || len(tasks) != 2 {
		t.Fatal("withdrawal escaped its own main task")
	}
	tasks[1]()
}
