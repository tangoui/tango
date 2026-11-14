package ui

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
)

// ProFormCaptcha (Pro Components v3 es/form/components/Captcha) is the
// captcha value type of ProField: ProField{ValueType: ValueCaptcha} with
// OnCaptcha (onGetCaptcha), CaptchaSeconds (countDown, default 60) and
// Captcha for the phone field, the button text and the timing callback.
// LoginForm's mobile tab uses the same button.

// ProFieldCaptcha configures a captcha field beyond ProField.OnCaptcha and
// ProField.CaptchaSeconds (Pro ProFormCaptcha phoneName,
// captchaTextRender, onTiming).
type ProFieldCaptcha struct {
	_ [0]func()

	// PhoneKey is the key of the phone field: inside a form that field is
	// validated first and nothing is sent while it is invalid (Pro
	// phoneName). OnCaptcha receives all values, the phone included.
	PhoneKey string
	// Text returns the button text while counting down (timing true, count
	// = seconds left) or idle (count = the full countdown); nil uses the
	// locale's "get code" / "resend in %d s" (Pro captchaTextRender).
	Text func(timing bool, count int) string
	// OnTiming receives the count whenever it changes: the full countdown
	// when idle, then every second left while counting down (Pro onTiming).
	OnTiming func(count int)
}

// proCaptchaSecs returns the countdown length (Pro countDown, default 60).
func proCaptchaSecs(n int) int {
	if n <= 0 {
		return 60
	}
	return n
}

// proCaptcha is the state of a "get code" button: the end of the
// countdown, a send in flight and its result.
type proCaptcha struct {
	until      time.Time
	sending    bool
	validating bool
	phone      string
	counting   bool // countdown announced as started, not yet as ended
	lastCount  int  // last count passed to OnTiming (0: none yet)
	res        pro3Async[error]
}

// remain returns the whole seconds left in the countdown (0: idle).
func (st *proCaptcha) remain(now time.Time) int { return pro3Remain(st.until, now) }

// start begins the countdown of secs seconds.
func (st *proCaptcha) start(c *Context, secs int) {
	st.until = c.Now().Add(time.Duration(secs) * time.Second)
	c.invalid = true
}

// send calls fn with a done callback; the button spins until done runs
// (from any goroutine). A nil error starts the countdown, an error leaves
// the button ready again (Pro: onGetCaptcha rejecting does not time).
func (st *proCaptcha) send(c *Context, fn func(done func(error))) {
	st.sending = true
	ch, inv := st.res.chInit(), c.Invalidator()
	fn(func(err error) { ch <- err; inv() })
	c.invalid = true
}

// poll applies delivered send results: it reports whether one arrived and
// its error; a success starts the countdown.
func (st *proCaptcha) poll(c *Context, secs int) (got bool, err error) {
	for {
		e, ok := st.res.poll()
		if !ok {
			return got, err
		}
		got, err = true, e
		st.sending = false
		if e == nil {
			st.start(c, secs)
		}
		c.invalid = true
	}
}

// button builds the "get code" button: the locale text (or text) with
// the seconds left, disabled while counting down, spinning while a send
// is in flight. It wakes the frame at every second of the countdown,
// announces its start and end, and calls onTiming when the count changes.
// It reports a click that should send a code.
func (st *proCaptcha) button(c *Context, secs int, size ControlSize, text func(bool, int) string, onTiming func(int)) bool {
	loc := c.Locale()
	now := c.Now()
	n := st.remain(now)
	count := secs
	if n > 0 {
		count = n
		left := st.until.Sub(now)
		c.WakeAt(now.Add(left - time.Duration(n-1)*time.Second))
	}
	var label string
	switch {
	case text != nil:
		label = text(n > 0, count)
	case n > 0:
		label = locFmt(loc.LoginForm.ResendFmt, n)
	default:
		label = loc.LoginForm.GetCode
	}
	if onTiming != nil && count != st.lastCount {
		st.lastCount = count
		onTiming(count)
	}
	// The countdown is announced when it starts and when it ends, not
	// every second (the label still counts down for sighted users).
	switch {
	case n > 0 && !st.counting:
		st.counting = true
		c.Announce(locFmt(loc.LoginForm.CodeSentFmt, n), a11y.LivePolite)
	case n == 0 && st.counting:
		st.counting = false
		c.Announce(loc.LoginForm.CodeReady, a11y.LivePolite)
	}
	b := Button(c, label).Size(size).Disabled(n > 0).Loading(st.sending || st.validating).Layout(layout.MinW(120))
	return b.Clicked() && !st.sending && !st.validating && n == 0
}
