package updateui

import (
	"time"

	"github.com/tangoui/tango/uihost"
)

// Default check timing.
const (
	DefaultInterval   = 24 * time.Hour   // between background checks
	DefaultFirstDelay = 30 * time.Second // after Start, before the first one
)

// Options configures Start and CheckNow.
type Options struct {
	// Interval is the time between background checks: 0 means
	// DefaultInterval, a negative value turns background checks off
	// (CheckNow still works).
	Interval time.Duration
	// FirstDelay is how long after Start the first background check
	// waits at least (0 means DefaultFirstDelay), so it does not slow
	// down the app's start; a check is never sooner than Interval after
	// the previous one, across launches.
	FirstDelay time.Duration
	// Window sets the window's theme, design flavor and locale like for
	// uihost.Open; the title and size are updateui's unless Surface sets
	// them.
	Window uihost.Options
	// Text replaces every text of the window; nil uses the built-in
	// English, Simplified Chinese, Traditional Chinese or Japanese texts
	// for i18n.Locale(), following locale changes.
	Text *Strings
}

func (o Options) interval() time.Duration {
	if o.Interval == 0 {
		return DefaultInterval
	}
	return o.Interval
}

func (o Options) firstDelay() time.Duration {
	if o.FirstDelay <= 0 {
		return DefaultFirstDelay
	}
	return o.FirstDelay
}
