//go:build tango_dev

package uihost

import (
	"errors"
	"testing"
	"time"

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
	if err := c.subscribeUI(); !errors.Is(err, errAutoOffUI) {
		t.Fatalf("tap attached off the UI thread: %v", err)
	}
	var err error
	var taps int
	th.Do(func() {
		err = c.subscribeUI()
		taps = len(b.taps)
	})
	if err != nil || taps != 1 {
		t.Fatalf("subscribe on the UI thread: %v, %d taps", err, taps)
	}
}

// Only windows opened on their own UI thread take the off-main path; a
// main-thread window is driven on main as before (panics report CodeQuit,
// no busy cap: internal/automation TestServerPanicCodeByThread).
func TestAutomationOffMainOnlyForThreadedWindows(t *testing.T) {
	r := ui.NewRunner(func(c *ui.Context) {}, ui.RunnerOptions{})
	if newAutoCanvas(r, &host{r: r}).OffMain() {
		t.Fatal("main-thread window driven off the main thread")
	}
	if newAutoCanvas(r, nil).OffMain() {
		t.Fatal("tan dev window driven off the main thread")
	}
	if !newAutoCanvas(r, &host{r: r, t: &threadedHost{}}).OffMain() {
		t.Fatal("threaded window driven on the main thread")
	}
}

// A request for a threaded window while the app quits reports CodeQuit at
// once instead of waiting for the busy limit.
func TestAutomationThreadedQuitReportsQuit(t *testing.T) {
	r := ui.NewRunner(func(c *ui.Context) { ui.Button(c, "OK") }, ui.RunnerOptions{})
	a, fb := faketest.NewApp(tango.Options{})
	b := newA11y(a, nil, r, ui.RunnerOptions{})
	t.Cleanup(b.close)
	faketest.Start(t, a, fb)
	th := uithread.Start("automation test")
	t.Cleanup(th.Stop)
	c := newAutoCanvas(r, &host{app: a, r: r, t: &threadedHost{th: th}})
	c.wait = 5 * time.Second
	a.Quit()
	<-fb.Done()
	start := time.Now()
	_, _, err := c.Tree()
	var e *automation.Error
	if !errors.As(err, &e) || e.Code != automation.CodeQuit {
		t.Fatalf("Tree after quit: %v, want CodeQuit", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Tree after quit took %v", d)
	}
}
