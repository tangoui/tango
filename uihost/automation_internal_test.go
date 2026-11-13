//go:build tango_dev

package uihost

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// Windows shown by the tan dev host (devOpen) have no accessibility
// bridge: every operation reports hot reload as unsupported instead of
// failing silently.
func TestAutomationHotReloadUnsupported(t *testing.T) {
	r := ui.NewRunner(func(c *ui.Context) { ui.Button(c, "OK") }, ui.RunnerOptions{})
	for _, c := range []*autoCanvas{{r: r}, {r: r, h: &host{r: r}}} { // tan dev; a runner without a bridge
		var e *automation.Error
		if _, _, err := c.Tree(); !errors.As(err, &e) || e.Code != automation.CodeHotReload {
			t.Fatalf("Tree: %v", err)
		}
		if err := c.Perform(1, a11y.Press, ""); !errors.Is(err, automation.ErrHotReload) {
			t.Fatalf("Perform: %v", err)
		}
		if err := c.Input([]input.Event{input.TextInput{Text: "x"}}); !errors.Is(err, automation.ErrHotReload) {
			t.Fatalf("Input: %v", err)
		}
		if _, err := c.Snapshot(); !errors.Is(err, automation.ErrHotReload) {
			t.Fatalf("Snapshot: %v", err)
		}
	}
}
