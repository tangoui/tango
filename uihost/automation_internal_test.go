//go:build tango_dev

package uihost

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"

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

// The accessibility bridge of a threaded window belongs to its UI thread:
// automation attaches its tap there and refuses from any other thread
// (frame publication and close would race with it).
func TestAutomationTapOnlyOnUIThread(t *testing.T) {
	r := ui.NewRunner(func(c *ui.Context) { ui.Button(c, "OK") }, ui.RunnerOptions{})
	a, _ := faketest.NewApp(tango.Options{})
	b := newA11y(a, nil, r, ui.RunnerOptions{})
	t.Cleanup(b.close)
	th := uithread.Start("automation test")
	t.Cleanup(th.Stop)
	c := newAutoCanvas(r, &host{app: a, r: r, t: &threadedHost{th: th}})
	if err := c.subscribe(); !errors.Is(err, errAutoOffUI) {
		t.Fatalf("tap attached off the UI thread: %v", err)
	}
	var err error
	var taps int
	th.Do(func() {
		err = c.subscribe()
		taps = len(b.taps)
	})
	if err != nil || taps != 1 {
		t.Fatalf("subscribe on the UI thread: %v, %d taps", err, taps)
	}
}
