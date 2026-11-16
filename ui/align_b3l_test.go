package ui

// Spec alignment batch 3, group L (feedback and status): behaviour tests
// for the gaps closed against antd 6.7 / Element Plus 2.14.8 / shadcn v4
// (docs/specs/align/3-feedback.md).

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/theme"
)

// b3lFind returns the first node (depth first) that matches pred.
func b3lFind(d *Driver, pred func(n *node) bool) *node {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if found != nil {
			return
		}
		if pred(n) {
			found = n
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.Runner().Context().root)
	return found
}

func b3lLabel(d *Driver, label string) *node {
	return b3lFind(d, func(n *node) bool { return n.label == label })
}

func b3lCenter(r geom.Rect) geom.Point { return geom.Pt(r.X+r.W/2, r.Y+r.H/2) }

// ---------------------------------------------------------------------------
// Alert

// TestAlertShowIconDefault: antd showIcon and EP show-icon default to
// false; a banner shows the icon unless told otherwise.
func TestAlertShowIconDefault(t *testing.T) {
	cases := []struct {
		name string
		mod  func(a *AlertW) *AlertW
		want bool
	}{
		{"default", func(a *AlertW) *AlertW { return a }, false},
		{"ShowIcon", func(a *AlertW) *AlertW { return a.ShowIcon(true) }, true},
		{"Banner", func(a *AlertW) *AlertW { return a.Banner() }, true},
		{"Banner ShowIcon(false)", func(a *AlertW) *AlertW { return a.Banner().ShowIcon(false) }, false},
		{"Banner NoIcon", func(a *AlertW) *AlertW { return a.Banner().NoIcon() }, false},
	}
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		for _, tc := range cases {
			d := NewDriver(func(c *Context) { tc.mod(Alert(c, "warning", "t")).Show() }, DriverOptions{Theme: th})
			d.Frame()
			got := b3lLabel(d, "warning") != nil // the status icon node
			if got != tc.want {
				t.Errorf("%v %s: icon %v, want %v", th.Flavor, tc.name, got, tc.want)
			}
		}
	}
	// Defaults "showIcon" still turns it on everywhere.
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Alert", Props{"showIcon": true}).Body(func() { Alert(c, "info", "t").Show() })
	}, DriverOptions{})
	d.Frame()
	if b3lLabel(d, "info") == nil {
		t.Error(`Defaults("Alert", showIcon: true) lost the icon`)
	}
}

// TestAlertCustomIcon: Icon replaces the status icon and shows it.
func TestAlertCustomIcon(t *testing.T) {
	d := NewDriver(func(c *Context) { Alert(c, "info", "t").Icon(icon.Star).Show() }, DriverOptions{})
	d.Frame()
	if b3lFind(d, func(n *node) bool { return n.paint != nil && n.label == "" && n.aHidden && n.ln.Width == 14 }) == nil {
		t.Error("custom 14px icon not built")
	}
	if b3lLabel(d, "info") != nil {
		t.Error("status icon drawn next to the custom icon")
	}
}

// TestAlertVariantFilled: antd 6.4 variant "filled" drops the status
// border; the default "outlined" keeps it (Defaults key "variant").
func TestAlertVariantFilled(t *testing.T) {
	var out, fill, def alertLook
	d := NewDriver(func(c *Context) {
		out = Alert(c, "success", "t").frame()
		fill = Alert(c, "success", "t").Variant("filled").frame()
		ConfigProvider(c).Defaults("Alert", Props{"variant": "filled"}).Body(func() { def = Alert(c, "success", "t").frame() })
	}, DriverOptions{})
	d.Frame()
	if out.border.A == 0 {
		t.Error("outlined alert has no border")
	}
	if fill.border.A != 0 || def.border.A != 0 {
		t.Errorf("filled alert border %v / Defaults %v, want none", fill.border, def.border)
	}
	if fill.bg != out.bg {
		t.Error("filled changed the background")
	}
}

// TestAlertCloseTextAndIcon: CloseText (EP close-text) and CloseIcon make
// the alert closable; a click reports Closed.
func TestAlertCloseTextAndIcon(t *testing.T) {
	textClosed, iconClosed := 0, 0
	d := NewDriver(func(c *Context) {
		if Alert(c, "info", "one").CloseText("知道了").Show().Closed() { // i18n:ok test data
			textClosed++
		}
		if Alert(c, "info", "two").CloseIcon(icon.CloseCircle).Show().Closed() {
			iconClosed++
		}
	}, DriverOptions{})
	d.Frame()
	r, ok := d.FindText("知道了") // i18n:ok test data
	if !ok {
		t.Fatal("close text not drawn")
	}
	d.Click(b3lCenter(r))
	if textClosed != 1 {
		t.Errorf("click on the close text: Closed %d times, want 1", textClosed)
	}
	r, ok = d.Find(ZhCN.Close)
	if !ok {
		t.Fatal("close icon button missing")
	}
	d.Click(b3lCenter(r))
	if iconClosed != 1 {
		t.Errorf("click on the custom close icon: Closed %d times, want 1", iconClosed)
	}
}

// TestAlertCloseDisabled: antd closable.disabled and
// ConfigProvider.ComponentDisabled: the close button does not respond, by
// pointer or keyboard, and is exposed as disabled.
func TestAlertCloseDisabled(t *testing.T) {
	for _, viaConfig := range []bool{false, true} {
		closed := 0
		d := NewDriver(func(c *Context) {
			build := func() {
				if Alert(c, "info", "t").Closable().CloseDisabled(!viaConfig).Show().Closed() {
					closed++
				}
			}
			if viaConfig {
				ConfigProvider(c).ComponentDisabled(true).Body(build)
			} else {
				build()
			}
		}, DriverOptions{})
		d.EnableAccessibility()
		d.Frame()
		n := b3lLabel(d, ZhCN.Close)
		if n == nil {
			t.Fatal("close button missing")
		}
		d.Click(b3lCenter(n.ln.Rect))
		if closed != 0 {
			t.Errorf("config=%v: disabled close button closed the alert", viaConfig)
		}
		if nd, ok := d.A11yFind(a11y.RoleButton, ZhCN.Close); !ok || nd.States&a11y.Disabled == 0 {
			t.Errorf("config=%v: close button not exposed as disabled", viaConfig)
		}
	}
}

// TestAlertA11yRoles: errors and warnings are alerts, the rest status; the
// title names the box and the description describes it.
func TestAlertA11yRoles(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Alert(c, "error", "坏了").Description("细节").Show() // i18n:ok test data
		Alert(c, "success", "好了").Show()                 // i18n:ok test data
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	if n, ok := d.A11yFind(a11y.RoleAlert, "坏了"); !ok || n.Description != "细节" { // i18n:ok test data
		t.Errorf("error alert: %v %+v", ok, n)
	}
	if _, ok := d.A11yFind(a11y.RoleStatus, "好了"); !ok { // i18n:ok test data
		t.Error("success alert is not a status")
	}
}

// ---------------------------------------------------------------------------
// Message

func b3lMsgDriver(fn func(c *Context)) *Driver {
	return NewDriver(fn, DriverOptions{Width: 600, Height: 400})
}

// TestMessagePauseOnHover: antd 6.7 message pauseOnHover defaults to
// true: a hovered message does not time out; leaving resumes the rest.
func TestMessagePauseOnHover(t *testing.T) {
	var c0 *Context
	open := true
	d := b3lMsgDriver(func(c *Context) {
		c0 = c
		if open {
			open = false
			Message(c, "info", "悬停暂停").Duration(time.Second) // i18n:ok test data
		}
	})
	d.Frame()
	d.Frame()
	r, ok := d.FindText("悬停暂停") // i18n:ok test data
	if !ok {
		t.Fatal("message not shown")
	}
	d.Move(b3lCenter(r))
	d.Frame()
	d.Advance(3 * time.Second)
	d.Frame()
	d.Frame()
	if len(c0.toasts().msgs) != 1 || c0.toasts().msgs[0].leaving(d.Now()) {
		t.Fatal("hovered message timed out")
	}
	d.Move(geom.Pt(5, 390))
	d.Frame()
	d.Advance(500 * time.Millisecond)
	d.Frame()
	if c0.toasts().msgs[0].leaving(d.Now()) {
		t.Error("message left before the rest of its time")
	}
	d.Advance(2 * time.Second)
	d.Frame()
	d.Frame()
	if len(c0.toasts().msgs) != 0 {
		t.Error("message did not close after the pointer left")
	}
}

// TestMessagePauseOnHoverOff: PauseOnHover(false) and the Defaults key
// keep counting while hovered.
func TestMessagePauseOnHoverOff(t *testing.T) {
	for _, viaDefaults := range []bool{false, true} {
		var c0 *Context
		open := true
		d := b3lMsgDriver(func(c *Context) {
			c0 = c
			if !open {
				return
			}
			open = false
			if viaDefaults {
				ConfigProvider(c).Defaults("Message", Props{"pauseOnHover": false}).Body(func() {
					Message(c, "info", "不停").Duration(time.Second) // i18n:ok test data
				})
			} else {
				Message(c, "info", "不停").Duration(time.Second).PauseOnHover(false) // i18n:ok test data
			}
		})
		d.Frame()
		d.Frame()
		r, _ := d.FindText("不停") // i18n:ok test data
		d.Move(b3lCenter(r))
		d.Frame()
		d.Advance(2 * time.Second)
		d.Frame()
		d.Frame()
		if len(c0.toasts().msgs) != 0 {
			t.Errorf("defaults=%v: message paused although pauseOnHover is off", viaDefaults)
		}
	}
}

// TestMessageOnCloseOnClick: onClose runs once after the message closed;
// onClick on a click.
func TestMessageOnCloseOnClick(t *testing.T) {
	closed, clicked := 0, 0
	open := true
	d := b3lMsgDriver(func(c *Context) {
		if open {
			open = false
			Message(c, "success", "回调").Duration(time.Second).OnClose(func() { closed++ }).OnClick(func() { clicked++ }) // i18n:ok test data
		}
	})
	d.Frame()
	d.Frame()
	r, _ := d.FindText("回调") // i18n:ok test data
	d.Click(b3lCenter(r))
	if clicked != 1 {
		t.Errorf("onClick ran %d times", clicked)
	}
	d.Move(geom.Pt(5, 390))
	d.Frame()
	d.Advance(3 * time.Second)
	d.Frame()
	d.Frame()
	d.Advance(time.Second)
	d.Frame()
	if closed != 1 {
		t.Errorf("onClose ran %d times, want 1", closed)
	}
}

// TestMessageGrouping: EP grouping merges equal content into one message
// with a repeat counter and takes the latest type.
func TestMessageGrouping(t *testing.T) {
	var c0 *Context
	n := 0
	d := b3lMsgDriver(func(c *Context) {
		c0 = c
		if n < 3 {
			n++
			st := "info"
			if n == 3 {
				st = "error"
			}
			Message(c, st, "重复").Grouping() // i18n:ok test data
		}
	})
	for range 4 {
		d.Frame()
	}
	ms := c0.toasts().msgs
	if len(ms) != 1 || ms[0].repeat != 3 || ms[0].status != "error" {
		t.Fatalf("grouping: %d messages, repeat %d status %q", len(ms), ms[0].repeat, ms[0].status)
	}
}

// TestMessageGroupingMaxCount: grouping happens before maxCount (EP
// method.mjs), so a repeated message merges even at the limit; a message
// pushed out by maxCount fades out and runs OnClose.
func TestMessageGroupingMaxCount(t *testing.T) {
	var c0 *Context
	step, closed := 1, 0
	d := b3lMsgDriver(func(c *Context) {
		c0 = c
		ConfigProvider(c).Defaults("Message", Props{"maxCount": 1}).Body(func() {
			switch step {
			case 1:
				Message(c, "info", "A").Duration(0).OnClose(func() { closed++ })
			case 2:
				Message(c, "info", "A").Grouping().Duration(0) // merges into the first A
			case 3:
				Message(c, "info", "B").Duration(0) // pushes A out
			}
			step = -step
		})
	})
	d.Frame()
	step = 2
	d.Frame()
	ms := c0.toasts().msgs
	if len(ms) != 1 || ms[0].repeat != 2 {
		t.Fatalf("after grouping at maxCount 1: %d messages, repeat %d", len(ms), ms[0].repeat)
	}
	step = 3
	d.Frame()
	d.Advance(time.Second)
	d.Frame()
	if ms := c0.toasts().msgs; len(ms) != 1 || ms[0].title != "B" {
		t.Errorf("maxCount kept %d messages", len(ms))
	}
	if closed != 1 {
		t.Errorf("message pushed out by maxCount: OnClose ran %d times, want 1", closed)
	}
}

// TestMessageGroupingPlacement: messages of different placements do not
// merge (EP keeps one list per placement).
func TestMessageGroupingPlacement(t *testing.T) {
	var c0 *Context
	step := 1
	d := b3lMsgDriver(func(c *Context) {
		c0 = c
		switch step {
		case 1:
			Message(c, "info", "A").Duration(0)
		case 2:
			Message(c, "info", "A").Grouping().Placement(MessageBottom).Duration(0)
		}
		step = 0
	})
	d.Frame()
	step = 2
	d.Frame()
	if n := len(c0.toasts().msgs); n != 2 {
		t.Errorf("messages %d, want 2 (different placements)", n)
	}
}

// TestMessageOutOfRangeDuration: a zero or negative duration keeps the
// message until destroyed (antd duration 0, EP duration 0).
func TestMessageOutOfRangeDuration(t *testing.T) {
	var c0 *Context
	open := true
	d := b3lMsgDriver(func(c *Context) {
		c0 = c
		if open {
			open = false
			Message(c, "info", "常驻").Duration(0)            // i18n:ok test data
			Message(c, "info", "负数").Duration(-time.Second) // i18n:ok test data
		}
	})
	d.Frame()
	d.Advance(time.Hour)
	d.Frame()
	if len(c0.toasts().msgs) != 2 {
		t.Fatalf("messages %d, want 2", len(c0.toasts().msgs))
	}
	MessageDestroy(c0, "")
	d.Advance(time.Second)
	d.Frame()
	d.Frame()
	if len(c0.toasts().msgs) != 0 {
		t.Error("destroy did not close the persistent messages")
	}
}

// TestMessageA11yLive: messages are live regions (status, alert for errors).
func TestMessageA11yLive(t *testing.T) {
	open := true
	d := b3lMsgDriver(func(c *Context) {
		if open {
			open = false
			Message(c, "error", "失败").Duration(0)   // i18n:ok test data
			Message(c, "success", "成功").Duration(0) // i18n:ok test data
		}
	})
	d.EnableAccessibility()
	d.Frame()
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleAlert, "失败"); !ok { // i18n:ok test data
		t.Error("error message is not an alert")
	}
	if _, ok := d.A11yFind(a11y.RoleStatus, "成功"); !ok { // i18n:ok test data
		t.Error("success message is not a status")
	}
}

// ---------------------------------------------------------------------------
// Notification

// TestNotificationClosable: Closable(false) hides the close button (antd
// closable, EP showClose); CloseIcon replaces it.
func TestNotificationClosable(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.Shadcn()} {
		open := true
		d := NewDriver(func(c *Context) {
			if open {
				open = false
				Notification(c, "info", "a").Closable(false).Duration(0)
			}
		}, DriverOptions{Width: 800, Height: 600, Theme: th})
		d.Frame()
		d.Frame()
		if _, ok := d.Find(ZhCN.Close); ok {
			t.Errorf("%v: close button shown with Closable(false)", th.Flavor)
		}
	}
	var ns *navNotes
	open := true
	d := NewDriver(func(c *Context) {
		ns = c.navNotes()
		if open {
			open = false
			Notification(c, "info", "b").CloseIcon(icon.CloseCircle).Duration(0)
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.Frame()
	r, ok := d.Find(ZhCN.Close)
	if !ok {
		t.Fatal("close button missing")
	}
	d.Click(b3lCenter(r))
	if len(ns.list) == 1 && ns.list[0].closing.IsZero() {
		t.Error("custom close icon did not close the card")
	}
}

// TestNotificationMaxCount: Defaults maxCount closes the oldest cards.
func TestNotificationMaxCount(t *testing.T) {
	var ns *navNotes
	open := true
	d := NewDriver(func(c *Context) {
		ns = c.navNotes()
		if open {
			open = false
			ConfigProvider(c).Defaults("Notification", Props{"maxCount": 2}).Body(func() {
				for _, s := range []string{"1", "2", "3"} {
					Notification(c, "info", s).Duration(0)
				}
			})
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.Advance(time.Second)
	d.Frame()
	d.Frame()
	var titles []string
	for _, n := range ns.list {
		if n.closing.IsZero() {
			titles = append(titles, n.cfg.Title)
		}
	}
	if len(titles) != 2 || titles[0] != "2" || titles[1] != "3" {
		t.Errorf("open cards %v, want [2 3]", titles)
	}
}

// TestNotificationRoleOnClose: Role overrides the live role; OnClose runs
// once after the card left.
func TestNotificationRoleOnClose(t *testing.T) {
	closed := 0
	open := true
	d := NewDriver(func(c *Context) {
		if open {
			open = false
			Notification(c, "error", "x").Role("status").Duration(time.Second).OnClose(func() { closed++ })
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.EnableAccessibility()
	d.Frame()
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleStatus, "x"); !ok {
		t.Error("Role(status) ignored")
	}
	d.Move(geom.Pt(5, 590))
	d.Advance(2 * time.Second)
	d.Frame()
	d.Frame()
	d.Advance(time.Second)
	d.Frame()
	if closed != 1 {
		t.Errorf("OnClose ran %d times", closed)
	}
}

// TestNotificationOutOfRangeDuration: Duration(0) keeps the card.
func TestNotificationOutOfRangeDuration(t *testing.T) {
	var ns *navNotes
	open := true
	d := NewDriver(func(c *Context) {
		ns = c.navNotes()
		if open {
			open = false
			Notification(c, "info", "stay").Duration(0).ShowProgress()
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.Advance(time.Hour)
	d.Frame()
	if len(ns.list) != 1 || !ns.list[0].closing.IsZero() {
		t.Error("Duration(0) card closed")
	}
}

// ---------------------------------------------------------------------------
// Spin / Loading

// TestSpinDescription: antd 6 description (tip deprecated) and the
// Defaults key "description".
func TestSpinDescription(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Spin(c).Description("载入")                                                               // i18n:ok test data
		ConfigProvider(c).Defaults("Spin", Props{"description": "默认"}).Body(func() { Spin(c) }) // i18n:ok test data
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	for _, s := range []string{"载入", "默认"} { // i18n:ok test data
		if _, ok := d.A11yFind(a11y.RoleStatus, s); !ok {
			t.Errorf("spin %q not found", s)
		}
	}
}

// TestSpinTokenSizes: indicator sizes come from SpinToken (antd 20/14/32,
// EP 42/28/50, shadcn size-4 = 16, small size-3, large size-6).
func TestSpinTokenSizes(t *testing.T) {
	cases := []struct {
		th   *theme.Theme
		want [3]float32
	}{
		{theme.AntDesign(), [3]float32{20, 14, 32}},
		{theme.ElementPlus(), [3]float32{42, 28, 50}},
		{theme.Shadcn(), [3]float32{16, 12, 24}}, // size-4, size-3 / size-6
	}
	for _, tc := range cases {
		var got [3]float32
		d := NewDriver(func(c *Context) {
			got[0] = Spin(c).px()
			got[1] = Spin(c).Size(Small).px()
			got[2] = Spin(c).Size(Large).px()
		}, DriverOptions{Theme: tc.th})
		d.Frame()
		if got != tc.want {
			t.Errorf("%v: sizes %v, want %v", tc.th.Flavor, got, tc.want)
		}
	}
}

// TestSpinDelayHidesAtOnce: delay postpones showing; stopping hides at
// once (antd debounce only on the way in).
func TestSpinDelayHidesAtOnce(t *testing.T) {
	on := true
	d := NewDriver(func(c *Context) {
		Spin(c).Description("等").Delay(300 * time.Millisecond).Spinning(on) // i18n:ok test data
	}, DriverOptions{Width: 200, Height: 200, Animations: true})
	d.Frame()
	if _, ok := navFindNode(d, "等"); ok { // i18n:ok test data
		t.Fatal("shown before the delay")
	}
	d.Advance(400 * time.Millisecond)
	d.Frame()
	if _, ok := navFindNode(d, "等"); !ok { // i18n:ok test data
		t.Fatal("not shown after the delay")
	}
	on = false
	d.Frame()
	if _, ok := navFindNode(d, "等"); ok { // i18n:ok test data
		t.Error("still shown after spinning stopped")
	}
}

// TestSpinPercentAuto: percent "auto" grows over time, slows down and
// never reaches 100 (antd usePercent).
func TestSpinPercentAuto(t *testing.T) {
	var s *SpinW
	d := NewDriver(func(c *Context) { s = Spin(c).PercentAuto() }, DriverOptions{Width: 200, Height: 200, Animations: true})
	d.Frame()
	if s.percent != 0 {
		t.Fatalf("start %v", s.percent)
	}
	d.Advance(time.Second) // 5 steps of 5%
	d.Frame()
	want := 100 - 100*0.95*0.95*0.95*0.95*0.95
	if s.percent < want-0.01 || s.percent > want+0.01 {
		t.Errorf("after 1s %v, want %v", s.percent, want)
	}
	d.Advance(10 * time.Minute)
	d.Frame()
	if s.percent >= 100 || s.percent < 96 {
		t.Errorf("after 10min %v, want in [96, 100)", s.percent)
	}
}

// TestSpinA11y: the indicator is a busy status named "loading" by the
// locale (shadcn Spinner role="status" aria-label="Loading"); Percent
// makes it a progress bar.
func TestSpinA11y(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Spin(c)
		Spin(c).Description("p").Percent(30)
	}, DriverOptions{Theme: theme.Shadcn()})
	d.EnableAccessibility()
	d.Frame()
	if n, ok := d.A11yFind(a11y.RoleStatus, ZhCN.A11y.Loading); !ok || n.States&a11y.Busy == 0 {
		t.Error("spinner not a busy status named Loading")
	}
	if n, ok := d.A11yFind(a11y.RoleProgressBar, "p"); !ok || n.Value != "30%" {
		t.Errorf("percent spin: %v %+v", ok, n)
	}
}

// TestLoadingTextAndSpinner: EP text / spinner options; the mask blocks
// the content while loading.
func TestLoadingTextAndSpinner(t *testing.T) {
	clicks := 0
	on := true
	d := NewDriver(func(c *Context) {
		Loading(c, on).Text("读取").Spinner(icon.Reload).Body(func() { // i18n:ok test data
			if Button(c, "btn").Clicked() {
				clicks++
			}
		})
	}, DriverOptions{Width: 300, Height: 200})
	d.EnableAccessibility()
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleStatus, "读取"); !ok { // i18n:ok test data
		t.Error("loading text not exposed")
	}
	_ = d.ClickLabel("btn")
	if clicks != 0 {
		t.Error("click went through the loading mask")
	}
	on = false
	d.Frame()
	d.Settle()
	_ = d.ClickLabel("btn")
	if clicks != 1 {
		t.Errorf("clicks after loading %d", clicks)
	}
}

// ---------------------------------------------------------------------------
// Skeleton throttle

// TestSkeletonThrottle: EP throttle: the skeleton appears only after
// loading lasted the leading delay and stays for the trailing delay.
func TestSkeletonThrottle(t *testing.T) {
	loading := true
	d := NewDriver(func(c *Context) {
		Skeleton(c).Loading(loading).Throttle(500*time.Millisecond, 300*time.Millisecond).Body(func() { Text(c, "内容") }) // i18n:ok test data
	}, DriverOptions{Width: 300, Height: 200, Animations: true})
	d.Frame()
	if _, ok := d.FindText("内容"); !ok { // i18n:ok test data
		t.Fatal("content hidden before the leading delay")
	}
	d.Advance(600 * time.Millisecond)
	d.Frame()
	if _, ok := d.FindText("内容"); ok { // i18n:ok test data
		t.Fatal("skeleton not shown after the leading delay")
	}
	loading = false
	d.Frame()
	if _, ok := d.FindText("内容"); ok { // i18n:ok test data
		t.Fatal("skeleton left before the trailing delay")
	}
	d.Advance(400 * time.Millisecond)
	d.Frame()
	if _, ok := d.FindText("内容"); !ok { // i18n:ok test data
		t.Error("content not back after the trailing delay")
	}
}

// TestSkeletonThrottleInitial: ThrottleInitial(true) starts on the
// skeleton.
func TestSkeletonThrottleInitial(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Skeleton(c).Loading(true).Throttle(time.Second, 0).ThrottleInitial(true).Body(func() { Text(c, "内容") }) // i18n:ok test data
	}, DriverOptions{Width: 300, Height: 200, Animations: true})
	d.Frame()
	if _, ok := d.FindText("内容"); ok { // i18n:ok test data
		t.Error("ThrottleInitial(true) showed the content first")
	}
}

// TestAlignB3LEmptyPictureToken: the Element illustration follows
// EmptyToken.Picture (square picture), not the flavor.
func TestAlignB3LEmptyPictureToken(t *testing.T) {
	th := theme.AntDesign()
	th.Components.Empty.Picture = theme.EmptyPictureElement
	for _, tc := range []struct {
		th   *theme.Theme
		want bool // square (Element illustration)
	}{{theme.AntDesign(), false}, {th, true}, {theme.ElementPlus(), true}} {
		d := NewDriver(func(c *Context) { Empty(c).Body(nil) }, DriverOptions{Theme: tc.th})
		d.Frame()
		n := b3lFind(d, func(n *node) bool {
			return n.label == "" && n.paint != nil && n.ln.Height > 0 && n.ln.Width > 0 && (n.ln.Width == n.ln.Height) == tc.want && n.ln.Height >= 100
		})
		if n == nil {
			t.Errorf("Picture %v: square=%v picture not found", tc.th.Components.Empty.Picture, tc.want)
		}
	}
}
