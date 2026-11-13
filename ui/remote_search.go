package ui

// Shared pieces of remote (server-side) search for the searchable inputs:
// Select / MultiSelect / VirtualSelect Remote, AutoComplete Remote and
// Mentions Remote. Each widget keeps a remoteSearch in its retained
// state, schedules it when the query is edited and ticks it every build;
// the application callback runs on the UI thread after the debounce and
// should start the request in the background, then replace the options
// (and clear Loading) when it completes.

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// remoteDebounce is the default delay before a remote search callback.
const remoteDebounce = 300 * time.Millisecond

// remoteSearch debounces a remote search callback.
type remoteSearch struct {
	pending bool
	at      time.Time
}

// schedule (re)starts the debounce: the callback is due d from now.
func (r *remoteSearch) schedule(c *Context, d time.Duration) {
	r.pending, r.at = true, c.now.Add(d)
}

// tick runs fire once the debounce has elapsed (and asks for a rebuild so
// the new options or loading state show); until then it books a wake-up.
func (r *remoteSearch) tick(c *Context, fire func()) {
	if !r.pending {
		return
	}
	if c.now.Before(r.at) {
		c.WakeAt(r.at)
		return
	}
	r.pending = false
	if fire != nil {
		fire()
	}
	c.invalid = true
}

// filterOptions returns the options matching query: fn when set, otherwise
// a case-insensitive label match. An empty query keeps every option.
func filterOptions(opts []Option, query string, fn func(query string, o Option) bool) []Option {
	if query == "" {
		return opts
	}
	q := strings.ToLower(query)
	var out []Option
	for _, o := range opts {
		ok := false
		if fn != nil {
			ok = fn(query, o)
		} else {
			ok = strings.Contains(strings.ToLower(o.Label), q)
		}
		if ok {
			out = append(out, o)
		}
	}
	return out
}

// entLoadingRow adds the busy line of a dropdown waiting for remote
// results: a spinner and s (Locale.Loading) centered in a row of height h,
// colored by the Select token.
func entLoadingRow(c *Context, s string, h float32) *node {
	th := c.ThemeFor(theme.CompSelect)
	tk := &th.Components.Select
	n := c.add("ent-loading", s)
	n.setRole(a11y.RoleStatus).stateIf(true, a11y.Busy)
	n.ln.Height = h
	n.ln.Measure = func(_, _ float32) geom.Size {
		return geom.Size{W: entTextW(c, s, th.FontSize, text.Regular) + 24 + 22, H: h}
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		tw := entTextW(c, s, th.FontSize, text.Regular)
		off := (r.W - tw - 20) / 2 // spinner then text from the inline start
		p2bSpin(c, l, c.inline(r).Rect(off, r.Y+(r.H-14)/2, 14, 14), tk.LoadingIconColor)
		entTextIn(c, l, s, off+20, r, th.FontSize, text.Regular, tk.LoadingColor, 0)
	}
	return n
}
