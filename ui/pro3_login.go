package ui

import (
	"image"
	"math"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Login types of LoginForm (the "loginType" value).
const (
	LoginAccount = "account"
	LoginMobile  = "mobile"
)

// LoginFormW is a login form (Ant Design Pro LoginForm): logo, title and
// subtitle, account / mobile tabs, a verification code button with a
// countdown, "remember me", a forgot-password link, a slot for other
// login methods and a submit button with a loading state.
//
// OnFinish receives ProValues with the keys loginType ("account" or
// "mobile"), username, password, mobile, captcha and autoLogin (bool).
//
//	ui.LoginForm(c).Title("Tango").SubTitle("...").
//		OnSendCode(func(mobile string, done func(error)) { go sms(mobile, done) }).
//		OnFinish(func(v ui.ProValues, done func(error)) { go login(v, done) }).
//		Show()
type LoginFormW struct {
	_ [0]func()

	c          *Context
	logo       func()
	title      string
	subTitle   string
	noMobile   bool
	noAccount  bool
	onSendCode func(mobile string, done func(error))
	onFinish   func(v ProValues, done func(error))
	onForgot   func()
	actions    func()
	submitText string
	width      float32
	interval   int
	mobileRe   string

	page    bool
	bg      theme.Color
	bgImg   image.Image
	bgSet   bool
	side    func()
	place   string
	id      ID
	st      *pro3LoginState
	form    *FormW
	done    bool
	forgot  bool
	sentNow bool
}

type pro3LoginState struct {
	tab                string
	username, password string
	mobile, captcha    string
	remember           bool
	remInit            bool
	codeUntil          time.Time
	counting           bool // countdown announced as started, not yet as ended
	sending            bool
	submitting         bool
	err                error
	codeRes            pro3Async[error]
	subRes             pro3Async[error]
}

// LoginForm starts a login form; chain options, then Show.
func LoginForm(c *Context) *LoginFormW {
	w := &LoginFormW{c: c, width: 328, interval: 60, mobileRe: `^1\d{10}$`}
	if shad2d(c) {
		w.width = 384 // login block: max-w-sm
	}
	// ConfigProvider.Defaults("LoginForm", ...) keys: "title", "subTitle",
	// "submitText" (string), "width" (number).
	w.title = Prop(c, "LoginForm", "title", "")
	w.subTitle = Prop(c, "LoginForm", "subTitle", "")
	w.submitText = Prop(c, "LoginForm", "submitText", "")
	if v, ok := p2bNum(c, "LoginForm", "width"); ok && v > 0 {
		w.width = float32(v)
	}
	return w
}

// LoginFormPage is LoginForm on a full page with a background color or
// image and an optional side area (Ant Design Pro LoginFormPage).
func LoginFormPage(c *Context) *LoginFormW {
	w := LoginForm(c)
	// ConfigProvider.Defaults("LoginFormPage", ...) keys: "placement"
	// ("left" | "center" | "right"), plus the "LoginForm" keys.
	w.page, w.place = true, Prop(c, "LoginFormPage", "placement", "right")
	return w
}

// Logo builds the logo left of the title.
func (w *LoginFormW) Logo(fn func()) *LoginFormW { w.logo = fn; return w }

// Title sets the title.
func (w *LoginFormW) Title(s string) *LoginFormW { w.title = s; return w }

// SubTitle sets the line under the title.
func (w *LoginFormW) SubTitle(s string) *LoginFormW { w.subTitle = s; return w }

// AccountOnly hides the tabs and the mobile login.
func (w *LoginFormW) AccountOnly() *LoginFormW { w.noMobile, w.noAccount = true, false; return w }

// MobileOnly hides the tabs and the account login.
func (w *LoginFormW) MobileOnly() *LoginFormW { w.noAccount, w.noMobile = true, false; return w }

// OnSendCode is called by the "get code" button with a valid mobile
// number; call done from any goroutine. A nil error starts the countdown.
func (w *LoginFormW) OnSendCode(fn func(mobile string, done func(error))) *LoginFormW {
	w.onSendCode = fn
	return w
}

// CodeInterval sets the countdown in seconds (default 60).
func (w *LoginFormW) CodeInterval(sec int) *LoginFormW { w.interval = max(1, sec); return w }

// MobilePattern replaces the mobile number pattern (default: 11 digits
// starting with 1).
func (w *LoginFormW) MobilePattern(re string) *LoginFormW { w.mobileRe = re; return w }

// OnFinish receives the values after validation; call done from any
// goroutine. The submit button spins meanwhile and an error is shown
// above the form.
func (w *LoginFormW) OnFinish(fn func(v ProValues, done func(error))) *LoginFormW {
	w.onFinish = fn
	return w
}

// OnForgot shows the forgot-password link and calls fn on click.
func (w *LoginFormW) OnForgot(fn func()) *LoginFormW { w.onForgot = fn; return w }

// Actions builds the other login methods (icons) under the form, in a
// group named "Other login methods". Icon-only buttons have no visible
// text: give each one an accessible name, e.g.
// ui.Button(c, "").Icon(icon.Wechat).Circle().A11yLabel("WeChat").
func (w *LoginFormW) Actions(fn func()) *LoginFormW { w.actions = fn; return w }

// SubmitText sets the submit button's text.
func (w *LoginFormW) SubmitText(s string) *LoginFormW { w.submitText = s; return w }

// Width sets the form width (default 328).
func (w *LoginFormW) Width(px float32) *LoginFormW { w.width = px; return w }

// Background sets the page background color (LoginFormPage).
func (w *LoginFormW) Background(col theme.Color) *LoginFormW { w.bg, w.bgSet = col, true; return w }

// BackgroundImage sets the page background image, scaled to cover
// (LoginFormPage).
func (w *LoginFormW) BackgroundImage(img image.Image) *LoginFormW { w.bgImg = img; return w }

// Side builds content beside the form card (LoginFormPage), such as an
// activity banner.
func (w *LoginFormW) Side(fn func()) *LoginFormW { w.side = fn; return w }

// Placement puts the form card "left", "center" or "right" (default) of
// the page (LoginFormPage).
func (w *LoginFormW) Placement(s string) *LoginFormW { w.place = s; return w }

// Finished reports a completed login (OnFinish called done(nil)), once.
func (w *LoginFormW) Finished() bool { return w.done }

// ForgotClicked reports a click on the forgot-password link.
func (w *LoginFormW) ForgotClicked() bool { return w.forgot }

// CodeSent reports that a verification code was sent in the last Show.
func (w *LoginFormW) CodeSent() bool { return w.sentNow }

// Submitting reports validation or OnFinish in progress.
func (w *LoginFormW) Submitting() bool {
	return w.st != nil && (w.st.submitting || (w.form != nil && w.form.Submitting()))
}

// Err returns the last OnFinish or OnSendCode error.
func (w *LoginFormW) Err() error {
	if w.st == nil {
		return nil
	}
	return w.st.err
}

// LoginType returns the active tab (LoginAccount or LoginMobile).
func (w *LoginFormW) LoginType() string {
	if w.st == nil {
		return ""
	}
	return w.st.tab
}

// Countdown returns the seconds left before a new code can be sent.
func (w *LoginFormW) Countdown() int {
	if w.st == nil {
		return 0
	}
	return pro3Remain(w.st.codeUntil, w.c.Now())
}

// Submit validates and submits the form.
func (w *LoginFormW) Submit() {
	if w.form != nil {
		w.form.Submit()
	}
}

// ID returns the widget ID (valid after Show).
func (w *LoginFormW) ID() ID { return w.id }

func pro3Remain(until, now time.Time) int {
	if until.IsZero() || !now.Before(until) {
		return 0
	}
	return int(math.Ceil(until.Sub(now).Seconds()))
}

// Values returns the form values (see OnFinish).
func (w *LoginFormW) Values() ProValues {
	st := w.st
	if st == nil {
		return ProValues{}
	}
	return ProValues{"loginType": st.tab, "username": st.username, "password": st.password,
		"mobile": st.mobile, "captcha": st.captcha, "autoLogin": st.remember}
}

// Show builds the form (or the page).
func (w *LoginFormW) Show() *LoginFormW {
	if w.page {
		w.showPage()
		return w
	}
	w.build()
	return w
}

func (w *LoginFormW) poll() {
	st := w.st
	for {
		err, ok := st.codeRes.poll()
		if !ok {
			break
		}
		st.sending = false
		if err != nil {
			st.err = err
		} else {
			st.codeUntil = w.c.Now().Add(time.Duration(w.interval) * time.Second)
			w.sentNow = true
		}
		w.c.invalid = true
	}
	for {
		err, ok := st.subRes.poll()
		if !ok {
			break
		}
		st.submitting, st.err = false, err
		w.done = err == nil
		w.c.invalid = true
	}
}

// build is the form itself.
func (w *LoginFormW) build() {
	c := w.c
	loc := c.Locale()
	root := c.box("login-form", layout.Column, []layout.Option{layout.W(w.width), layout.Grow(0)})
	w.id = root.n.id
	st := State[pro3LoginState](c, w.id, func() pro3LoginState { return pro3LoginState{remember: true} })
	w.st = st
	switch {
	case w.noMobile:
		st.tab = LoginAccount
	case w.noAccount:
		st.tab = LoginMobile
	case st.tab == "":
		st.tab = LoginAccount
	}
	w.poll()
	root.Semantics(a11y.RoleForm, w.title)
	root.Body(func() {
		w.header()
		if !w.noMobile && !w.noAccount {
			Row(c, func() {
				tb := Tabs(c, &st.tab, []TabItem{
					{Key: LoginAccount, Label: loc.LoginForm.AccountTab},
					{Key: LoginMobile, Label: loc.LoginForm.MobileTab},
				}).Show()
				if tb.Changed() {
					st.err = nil
					c.invalid = true
				}
			}, layout.Center(), layout.Margin(geom.Insets{Bottom: 8}))
		}
		if st.err != nil {
			Row(c, func() { Alert(c, "error", st.err.Error()).Show() }, layout.Margin(geom.Insets{Bottom: 24}))
		}
		w.fields()
	})
	if w.form != nil && w.form.Finished() {
		if w.onFinish == nil {
			w.done = true
		} else {
			st.submitting, st.err = true, nil
			ch, inv := st.subRes.chInit(), c.Invalidator()
			w.onFinish(w.Values(), func(err error) { ch <- err; inv() })
			w.poll()
		}
		c.invalid = true
	}
	if n := pro3Remain(st.codeUntil, c.Now()); n > 0 {
		// Wake at the next whole second of the countdown.
		left := st.codeUntil.Sub(c.Now())
		step := left - time.Duration(n-1)*time.Second
		c.WakeAt(c.Now().Add(step))
	}
}

// header is the logo, title and subtitle.
func (w *LoginFormW) header() {
	c, th := w.c, w.c.Theme
	if w.logo == nil && w.title == "" && w.subTitle == "" {
		return
	}
	if shad2d(c) {
		w.shadHeader()
		return
	}
	Column(c, func() {
		if w.logo != nil || w.title != "" {
			Row(c, func() {
				if w.logo != nil {
					Row(c, w.logo, layout.H(44), layout.Middle())
				}
				if w.title != "" {
					Text(c, w.title).Size(33).Weight(text.Medium).Color(th.TextHeading).NoWrap()
				}
			}, layout.Gap(12), layout.Middle(), layout.Center())
		}
		if w.subTitle != "" {
			Text(c, w.subTitle).Color(th.TextSecondary).Align(AlignCenter).
				Layout(layout.Margin(geom.Insets{Top: 12}))
		}
	}, layout.AlignItems(layout.AlignCenter), layout.Margin(geom.Insets{Bottom: 32}))
}

// fields builds the inputs of the active tab, the options row and the
// buttons.
func (w *LoginFormW) fields() {
	c, th, st := w.c, w.c.Theme, w.st
	loc := c.Locale()
	f := Form(c).Vertical()
	w.form = f
	grow := []layout.Option{layout.Grow(1), layout.MinW(0)}
	shad := shad2d(c)
	size := Large
	// item labels the field above its input in shadcn (login block
	// Field + Label); Pro hides the label and uses icon prefixes.
	item := func(label, name string) *FormItemW {
		if shad {
			return f.Item(label).Name(name)
		}
		return f.Item("").NoLabel().Name(name)
	}
	prefix := func(in *InputW, ic *icon.Icon) *InputW {
		if !shad {
			in.Prefix(ic)
		}
		return in
	}
	if shad {
		size = Middle
	}
	f.Body(func() {
		if st.tab == LoginAccount {
			item(loc.LoginForm.Username, "username").Rules(&st.username,
				Required(loc.LoginForm.UsernameRequired)).Body(func() {
				if prefix(Input(c, &st.username).Size(size), icon.User).AllowClear().
					Placeholder(loc.LoginForm.Username).Layout(grow...).Submitted() {
					f.Submit()
				}
			})
			item(loc.LoginForm.Password, "password").Rules(&st.password,
				Required(loc.LoginForm.PasswordRequired)).Body(func() {
				if prefix(Input(c, &st.password).Size(size), icon.Lock).Password().
					Placeholder(loc.LoginForm.Password).Layout(grow...).Submitted() {
					f.Submit()
				}
			})
		} else {
			mobileRules := []Rule{
				Required(loc.LoginForm.MobileRequired),
				Pattern(w.mobileRe, loc.LoginForm.MobileInvalid),
			}
			item(loc.LoginForm.Mobile, "mobile").Rules(&st.mobile, mobileRules...).Body(func() {
				prefix(Input(c, &st.mobile).Size(size), icon.Message).AllowClear().
					Placeholder(loc.LoginForm.Mobile).Layout(grow...)
			})
			item(loc.LoginForm.Captcha, "captcha").Rules(&st.captcha,
				Required(loc.LoginForm.CaptchaRequired)).Body(func() {
				Row(c, func() {
					if prefix(Input(c, &st.captcha).Size(size), icon.Mail).
						Placeholder(loc.LoginForm.Captcha).Layout(grow...).Submitted() {
						f.Submit()
					}
					w.codeButton(f, mobileRules, size)
				}, layout.Gap(8), layout.Grow(1), layout.MinW(0))
			})
		}
		Row(c, func() {
			Checkbox(c, loc.LoginForm.Remember, &st.remember)
			Spacer(c)
			if w.onForgot != nil && Link(c, loc.LoginForm.Forgot).Clicked() {
				w.forgot = true
				w.onForgot()
			}
		}, layout.Middle(), layout.Margin(geom.Insets{Bottom: 24}))
		sub := w.submitText
		if sub == "" {
			sub = loc.LoginForm.Submit
		}
		if Button(c, sub).Primary().Size(size).Block().Loading(w.Submitting()).Clicked() {
			f.Submit()
		}
	})
	if w.actions != nil && shad {
		w.shadOthers()
	} else if w.actions != nil {
		Row(c, func() {
			Text(c, loc.LoginForm.Others).Color(th.TextTertiary)
			Row(c, w.actions, layout.Gap(12), layout.Middle())
		}, layout.Gap(8), layout.Middle(), layout.Margin(geom.Insets{Top: 24})).
			Semantics(a11y.RoleGroup, loc.LoginForm.Others)
	}
}

// codeButton is the "get code" button with its countdown.
func (w *LoginFormW) codeButton(f *FormW, rules []Rule, size ControlSize) {
	c, st := w.c, w.st
	loc := c.Locale()
	n := pro3Remain(st.codeUntil, c.Now())
	label := loc.LoginForm.GetCode
	if n > 0 {
		label = locFmt(loc.LoginForm.ResendFmt, n)
	}
	b := Button(c, label).Size(size).Disabled(n > 0).Loading(st.sending).Layout(layout.MinW(120))
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
	if !b.Clicked() {
		return
	}
	if msg := pro3Check(rules, st.mobile); msg != "" {
		f.st.touched["mobile"] = true
		c.invalid = true
		return
	}
	if w.onSendCode == nil {
		st.codeUntil = c.Now().Add(time.Duration(w.interval) * time.Second)
		w.sentNow = true
		c.invalid = true
		return
	}
	st.sending, st.err = true, nil
	ch, inv := st.codeRes.chInit(), c.Invalidator()
	w.onSendCode(st.mobile, func(err error) { ch <- err; inv() })
	w.poll()
	c.invalid = true
}

// showPage wraps the form in a full page with a background.
func (w *LoginFormW) showPage() {
	c, th := w.c, w.c.Theme
	img := w.bgImg
	bg := th.BgLayout
	shad := shad2d(c)
	if shad {
		bg, _ = shad2dMuted(th) // login block page: bg-muted
	}
	if w.bgSet {
		bg = w.bg
	}
	page := Row(c, func() {
		card := func() {
			if shad {
				// Card: rounded-xl border shadow-sm, p-6.
				Column(c, func() { w.build() }, layout.Pad(24)).
					Bg(th.BgContainer).Radius(shad2dCardRadius(th)).Border(1, th.BorderSecondary).Shadow(th.CardShadow)
				return
			}
			Column(c, func() { w.build() }, layout.PadVH(32, 40)).
				Bg(th.BgContainer).Radius(th.RadiusLG).Shadow(th.ShadowSecondary)
		}
		switch w.place {
		case "left":
			card()
			if w.side != nil {
				Column(c, w.side, layout.Grow(1), layout.MinW(0), layout.Center())
			} else {
				Spacer(c)
			}
		case "center":
			Spacer(c)
			Column(c, func() {
				card()
				if w.side != nil {
					Column(c, w.side, layout.Margin(geom.Insets{Top: 24}))
				}
			}, layout.AlignItems(layout.AlignCenter))
			Spacer(c)
		default:
			if w.side != nil {
				Column(c, w.side, layout.Grow(1), layout.MinW(0), layout.Center())
			} else {
				Spacer(c)
			}
			card()
		}
	}, layout.Grow(1), layout.MinH(560), layout.Pad(48), layout.Gap(48), layout.Middle())
	page.Paint(func(l *paint.List, r geom.Rect) {
		if img == nil {
			if w.bgSet || shad {
				l.Fill(r, geom.Radii{}, bg)
				return
			}
			l.LinearStops(r, geom.Radii{}, geom.Pt(r.X, r.Y), geom.Pt(r.Right(), r.Bottom()),
				paint.Stop{Offset: 0, Color: th.Primary.Bg}, paint.Stop{Offset: 1, Color: bg})
			return
		}
		b := img.Bounds()
		iw, ih := float32(b.Dx()), float32(b.Dy())
		if iw <= 0 || ih <= 0 {
			return
		}
		k := max(r.W/iw, r.H/ih)
		dw, dh := iw*k, ih*k
		l.Clip(r, geom.Radii{})
		l.DrawImage(img, geom.Rect{X: r.X + (r.W-dw)/2, Y: r.Y + (r.H-dh)/2, W: dw, H: dh}, geom.Radii{})
		l.Pop()
	})
}
