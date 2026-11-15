package ui

import "github.com/tangoui/tango/ui/input"

// TerminalEvents declares an experimental fast input path for a focused
// terminal-like control. The handler runs when Runner.HandleEvent receives an
// event, before the event is queued for the next frame. Returning true claims
// the event; returning false leaves it for ordinary frame processing.
//
// The handler must not mutate retained UI state from another goroutine. It
// may enqueue work into a thread-safe application queue such as a terminal's
// input writer.
func (c *Context) TerminalEvents(id ID, fn func(input.Event) bool) {
	if id == 0 {
		return
	}
	c.terminalEventID = id
	c.terminalEvent = fn
}
