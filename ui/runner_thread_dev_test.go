//go:build tango_dev

package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

const offThreadText = "ui: called off the window's UI thread; use Runner.Post"

type threadEntry struct {
	name string
	call func()
}

// threadEntries lists every entry point that checks the caller's thread.
func threadEntries(r *Runner) []threadEntry {
	c := r.Context()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	return []threadEntry{
		{"Runner.SetReduceMotion", func() { r.SetReduceMotion(true) }},
		{"Runner.EnableAccessibility", func() { r.EnableAccessibility(true) }},
		{"Runner.SetTheme", func() { r.SetTheme(theme.AntDesign()) }},
		{"Runner.HandleEvent", func() { r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Point{X: 1, Y: 1}}) }},
		{"Runner.Frame", func() { _ = r.Frame() }},
		{"Runner.PerformAction", func() { _ = r.PerformAction(1, a11y.Press, "") }},
		{"Runner.SetKeyboardInset", func() { r.SetKeyboardInset(geom.Insets{Bottom: 10}) }},
		{"Runner.ClearKeyboardInset", func() { r.ClearKeyboardInset() }},
		{"Runner.SetSafeArea", func() { r.SetSafeArea(geom.Insets{Top: 10}) }},
		{"Runner.ClearSafeArea", func() { r.ClearSafeArea() }},
		{"Runner.FileDragOver", func() { r.FileDragOver(geom.Point{X: 1, Y: 1}) }},
		{"Runner.FileDragLeave", func() { r.FileDragLeave() }},
		{"Runner.DropFiles", func() { r.DropFiles(geom.Point{X: 1, Y: 1}, []string{"a.txt"}) }},
		{"Runner.TrimCaches", func() { r.TrimCaches() }},
		{"Runner.SetTouchPointerEmulation", func() { r.SetTouchPointerEmulation(true) }},
		{"State", func() { State(c, 1, func() int { return 0 }) }},
		{"Context.Invalidate", func() { c.Invalidate() }},
		{"Context.WakeAt", func() { c.WakeAt(time.Unix(2000, 0)) }},
		{"Context.WakeAtDamaged", func() { c.WakeAtDamaged(time.Unix(2000, 0)) }},
		{"Context.SetFocus", func() { c.SetFocus(1) }},
		{"Context.InvalidateImage", func() { c.InvalidateImage(img) }},
	}
}

// panicOn runs fn on a new goroutine and returns what it panicked with
// ("" if it returned normally).
func panicOn(fn func()) string {
	res := make(chan string)
	go func() {
		msg := ""
		defer func() {
			if v := recover(); v != nil {
				msg = fmt.Sprint(v)
			}
			res <- msg
		}()
		fn()
	}()
	return <-res
}

func newBoundRunner(t *testing.T) (*Runner, func(func()) bool) {
	t.Helper()
	th := startUIThread(t)
	r := NewRunner(func(c *Context) { Div(c) }, RunnerOptions{})
	r.BindThread(th)
	r.Attach(newThreadWin())
	return r, th.Do
}

func TestStateOffThreadPanics(t *testing.T) {
	r, _ := newBoundRunner(t)
	for _, e := range threadEntries(r) {
		msg := panicOn(e.call)
		if !strings.Contains(msg, offThreadText) {
			t.Errorf("%s off the UI thread: panic %q, want it to contain %q", e.name, msg, offThreadText)
		}
	}
}

func TestStateOnThreadOK(t *testing.T) {
	r, do := newBoundRunner(t)
	for _, e := range threadEntries(r) {
		msg := ""
		do(func() {
			defer func() {
				if v := recover(); v != nil {
					msg = fmt.Sprint(v)
				}
			}()
			e.call()
		})
		if msg != "" {
			t.Errorf("%s on the UI thread panicked: %s", e.name, msg)
		}
	}
}

func TestUnboundRunnerNeverChecks(t *testing.T) {
	r := NewRunner(func(c *Context) { Div(c) }, RunnerOptions{})
	r.Attach(newThreadWin())
	for _, e := range threadEntries(r) {
		if msg := panicOn(e.call); msg != "" {
			t.Errorf("%s on an unbound runner panicked: %s", e.name, msg)
		}
	}
}
