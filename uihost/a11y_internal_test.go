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
