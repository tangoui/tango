package ui

// Batch 4 group B (Pro form) alignment tests: ProFormCaptcha,
// ProFormSegmented and the v3 behaviour of ProForm, ModalForm/DrawerForm,
// QueryFilter, StepsForm, ProFormList, ProFormGroup, LoginForm and
// LightFilter (docs/specs/align/4-form.md).

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// b4bCaptchaForm renders a ProForm with one captcha field whose sends are
// recorded; the done callbacks are kept so the test decides when (and
// how) each send completes — no goroutines, no sleeping.
type b4bCaptcha struct {
	sent  []ProValues
	dones []func(error)
}

func (b *b4bCaptcha) field(fd ProField) ProField {
	fd.ValueType = ValueCaptcha
	if fd.Key == "" {
		fd.Key = "code"
	}
	if fd.Label == "" {
		fd.Label = "Code"
	}
	fd.OnCaptcha = func(v ProValues, done func(error)) {
		b.sent = append(b.sent, v)
		b.dones = append(b.dones, done)
	}
	return fd
}

func TestProFormCaptchaSendCountdown(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var cb b4bCaptcha
		fields := []ProField{cb.field(ProField{})} // CaptchaSeconds 0: Pro countDown default 60
		vals := ProValues{}
		d := pro3Driver(th, 700, 400, func(c *Context) { ProForm(c, fields).Values(&vals).Show() })
		if err := d.ClickLabel(ZhCN.LoginForm.GetCode); err != nil {
			t.Fatal(err)
		}
		if len(cb.sent) != 1 {
			t.Fatalf("sends %d", len(cb.sent))
		}
		// In flight: the button spins and ignores clicks; no countdown yet.
		d.ClickLabel(ZhCN.LoginForm.GetCode)
		if len(cb.sent) != 1 {
			t.Fatal("sent twice while in flight")
		}
		// Success: 60 s countdown, disabled meanwhile.
		cb.dones[0](nil)
		d.Settle()
		if !pro3HasText(d, locFmt(ZhCN.LoginForm.ResendFmt, 60)) {
			t.Fatal("countdown did not start at 60")
		}
		d.ClickLabel(locFmt(ZhCN.LoginForm.ResendFmt, 60))
		if len(cb.sent) != 1 {
			t.Fatal("sent during the countdown")
		}
		d.Advance(time.Second)
		d.Settle()
		if !pro3HasText(d, locFmt(ZhCN.LoginForm.ResendFmt, 59)) {
			t.Fatal("countdown did not tick")
		}
		d.Advance(59 * time.Second)
		d.Settle()
		if !pro3HasText(d, ZhCN.LoginForm.GetCode) {
			t.Fatal("countdown did not end")
		}
		// Ready again.
		d.ClickLabel(ZhCN.LoginForm.GetCode)
		if len(cb.sent) != 2 {
			t.Fatalf("not ready after the countdown: sends %d", len(cb.sent))
		}
	})
}

func TestProFormCaptchaFailureRecovers(t *testing.T) {
	var cb b4bCaptcha
	fields := []ProField{cb.field(ProField{CaptchaSeconds: 5})}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) { ProForm(c, fields).Show() })
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	cb.dones[0](errors.New("sms down"))
	d.Settle()
	// Pro: onGetCaptcha rejecting leaves timing false and loading false.
	if pro3HasText(d, locFmt(ZhCN.LoginForm.ResendFmt, 5)) || !pro3HasText(d, ZhCN.LoginForm.GetCode) {
		t.Fatal("a failed send started the countdown")
	}
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	if len(cb.sent) != 2 {
		t.Fatalf("button not ready after a failure: sends %d", len(cb.sent))
	}
	cb.dones[1](nil)
	d.Settle()
	if !pro3HasText(d, locFmt(ZhCN.LoginForm.ResendFmt, 5)) {
		t.Fatal("retry did not count down")
	}
}

func TestProFormCaptchaDisabled(t *testing.T) {
	var cb b4bCaptcha
	fields := []ProField{cb.field(ProField{Disable: func(ProValues) bool { return true }})}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) { ProForm(c, fields).Show() })
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	if len(cb.sent) != 0 {
		t.Fatal("a disabled captcha field sent a code")
	}
}

func TestProFormCaptchaA11y(t *testing.T) {
	var cb b4bCaptcha
	fields := []ProField{cb.field(ProField{CaptchaSeconds: 3})}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) { ProForm(c, fields).Show() })
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleTextbox, "Code")
	b := mustNode(t, d, a11y.RoleButton, ZhCN.LoginForm.GetCode)
	// Keyboard: focus the button and press Enter.
	d.PerformAction(b.ID, a11y.Focus, "")
	d.Frame()
	d.Key(input.KeyEnter, 0)
	if len(cb.sent) != 1 {
		t.Fatalf("Enter did not send: %d", len(cb.sent))
	}
	cb.dones[0](nil)
	d.Settle()
	n := mustNode(t, d, a11y.RoleButton, locFmt(ZhCN.LoginForm.ResendFmt, 3))
	if n.States&a11y.Disabled == 0 {
		t.Fatal("counting button not reported disabled")
	}
}

func TestProFormCaptchaTextTimingDefaults(t *testing.T) {
	var cb b4bCaptcha
	var counts []int
	fields := []ProField{cb.field(ProField{Captcha: &ProFieldCaptcha{
		Text: func(timing bool, n int) string {
			if timing {
				return locFmt("wait %d", n)
			}
			return "send"
		},
		OnTiming: func(n int) { counts = append(counts, n) },
	}})}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) {
		ConfigProvider(c).Defaults("ProFormCaptcha", Props{"countDown": 3}).Body(func() { ProForm(c, fields).Show() })
	})
	if len(counts) != 1 || counts[0] != 3 {
		t.Fatalf("initial timing %v", counts)
	}
	d.ClickLabel("send")
	cb.dones[0](nil)
	d.Settle()
	if !pro3HasText(d, "wait 3") {
		t.Fatal("custom countdown text missing")
	}
	d.Advance(time.Second)
	d.Settle()
	d.Advance(2 * time.Second)
	d.Settle()
	if !pro3HasText(d, "send") || len(counts) != 3 || counts[1] != 2 || counts[2] != 3 {
		t.Fatalf("timing %v", counts)
	}
}

func TestProFormSegmentedConfiguration(t *testing.T) {
	vals := ProValues{"mode": "a"}
	fields := []ProField{{Key: "mode", Label: "Mode", ValueType: ValueSegmented,
		ValueEnum: []ProEnum{{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta", Disabled: true}, {Value: "c", Label: "Gamma"}},
		Segmented: func(w *SegmentedW) { w.ItemRender(func(o Option, _ bool) { Text(w.c, "custom "+o.Label) }) },
	}}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) { ProForm(c, fields).Values(&vals).Show() })
	d.EnableAccessibility()
	d.Frame()
	g := mustNode(t, d, a11y.RoleRadioGroup, "Mode")
	if !pro3HasText(d, "custom Alpha") {
		t.Fatal("custom content missing")
	}
	d.ClickLabel("custom Beta")
	if vals.String("mode") != "a" {
		t.Fatal("disabled option selected")
	}
	d.PerformAction(g.ID, a11y.Focus, "")
	d.Frame()
	d.Key(input.KeyRight, 0)
	if vals.String("mode") != "c" {
		t.Fatalf("keyboard did not skip disabled: %v", vals)
	}
	vals["mode"] = "a"
	d.Settle()
	if mustNode(t, d, a11y.RoleRadio, "Alpha").States&a11y.Checked == 0 {
		t.Fatal("external value not reflected")
	}
	fields[0].Readonly = true
	d.Settle()
	if !pro3HasText(d, "Alpha") {
		t.Fatal("readonly did not map label")
	}
	vals["mode"] = "unknown"
	d.Settle()
	if !pro3HasText(d, "unknown") {
		t.Fatal("unknown readonly value lost")
	}
}

func TestProFormV3Defaults(t *testing.T) {
	d := NewDriver(func(c *Context) {
		if ModalForm(c, new(bool), "Modal", nil).width != 800 {
			t.Error("ModalForm width must default to 800")
		}
		if DrawerForm(c, new(bool), "Drawer", nil).width != 800 {
			t.Error("DrawerForm width must default to 800")
		}
		for _, tc := range []struct {
			w float32
			n int
		}{{575, 1}, {576, 2}, {767, 2}, {768, 3}, {1599, 3}, {1600, 4}} {
			if got := queryPerRow(tc.w); got != tc.n {
				t.Errorf("queryPerRow(%v)=%d want %d", tc.w, got, tc.n)
			}
		}
	}, DriverOptions{})
	d.Frame()
}

func TestProFormQueryNarrowKeepsOneField(t *testing.T) {
	fields := []ProField{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 420, 600, func(c *Context) { QueryFilter(c, fields).Show() })
	if !pro3HasText(d, ZhCN.WithColon("Alpha")) {
		t.Fatal("narrow collapsed query hides every field")
	}
}

func TestProFormCaptchaPhoneValidation(t *testing.T) {
	var cb b4bCaptcha
	vals := ProValues{}
	fields := []ProField{{Key: "phone", Label: "Phone", Required: true}, cb.field(ProField{Captcha: &ProFieldCaptcha{PhoneKey: "phone"}})}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 500, func(c *Context) { ProForm(c, fields).Values(&vals).Show() })
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	if len(cb.sent) != 0 {
		t.Fatal("sent without valid phone")
	}
	vals["phone"] = "13800000000"
	d.Settle()
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	if len(cb.sent) != 1 || cb.sent[0].String("phone") != "13800000000" {
		t.Fatalf("phone snapshot %v", cb.sent)
	}
}

func TestProFormV3SubmitterOrder(t *testing.T) {
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) { ProForm(c, nil).Show() })
	r, _ := d.FindText(ZhCN.Reset)
	s, _ := d.FindText(ZhCN.Form.Submit)
	if r.X >= s.X {
		t.Fatal("reset must precede submit")
	}
}

func TestProFormModalPreservesValues(t *testing.T) {
	open := true
	vals := ProValues{"a": "original"}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 700, func(c *Context) {
		ModalForm(c, &open, "Edit", []ProField{{Key: "a", Label: "Alpha"}}).Values(&vals).Show()
	})
	if vals.String("a") != "original" {
		t.Fatal("initial controlled values discarded")
	}
	vals["a"] = "edited"
	open = false
	d.Settle()
	open = true
	d.Settle()
	if vals.String("a") != "edited" {
		t.Fatal("reopening discarded values")
	}
}

func TestProFormB4ThemesAndLocales(t *testing.T) {
	for _, th := range []struct {
		name string
		f    func() *theme.Theme
	}{
		{"ant", func() *theme.Theme { return theme.AntDesign() }}, {"ant-dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
		{"element", func() *theme.Theme { return theme.ElementPlus() }}, {"element-dark", func() *theme.Theme { return theme.ElementPlus(theme.Dark()) }},
		{"shadcn", func() *theme.Theme { return theme.Shadcn() }}, {"shadcn-dark", func() *theme.Theme { return theme.Shadcn(theme.Dark()) }},
	} {
		t.Run(th.name, func(t *testing.T) {
			for _, loc := range []*Locale{EnUS, ZhCN, ZhTW, Ja} {
				var cb b4bCaptcha
				vals := ProValues{"mode": "a"}
				fields := []ProField{cb.field(ProField{}), {Key: "mode", Label: "Mode", ValueType: ValueSegmented, ValueEnum: []ProEnum{{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta"}}}}
				d := pro3Driver(th.f, 700, 500, func(c *Context) {
					ConfigProvider(c).Locale(loc).Body(func() { ProForm(c, fields).Values(&vals).Show() })
				})
				d.EnableAccessibility()
				d.Frame()
				mustNode(t, d, a11y.RoleButton, loc.LoginForm.GetCode)
				if err := d.ClickLabel(loc.LoginForm.GetCode); err != nil {
					t.Fatal(err)
				}
				cb.dones[0](nil)
				d.Settle()
				mustNode(t, d, a11y.RoleButton, locFmt(loc.LoginForm.ResendFmt, 60))
			}
		})
	}
}

func TestProFormCaptchaGlobalDisabledAndReadonly(t *testing.T) {
	var cb b4bCaptcha
	disabled, readonly := true, false
	fields := []ProField{cb.field(ProField{})}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 400, func(c *Context) {
		Disabled(c, disabled, func() { ProForm(c, fields).Readonly(readonly).Show() })
	})
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	if len(cb.sent) != 0 {
		t.Fatal("globally disabled captcha sent")
	}
	disabled = false
	readonly = true
	d.Settle()
	if pro3HasText(d, ZhCN.LoginForm.GetCode) {
		t.Fatal("readonly captcha has send button")
	}
}

func TestProFormModalDestroyOnHidden(t *testing.T) {
	open := true
	vals := ProValues{}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 700, func(c *Context) {
		ModalForm(c, &open, "Edit", []ProField{{Key: "a", Label: "Alpha"}}).Values(&vals).DestroyOnHidden(true).Show()
	})
	vals["a"] = "edited"
	open = false
	d.Settle()
	open = true
	d.Settle()
	if vals.String("a") != "" {
		t.Fatal("destroyOnHidden did not reset")
	}
}

func TestProFormCaptchaPhoneAfterCaptcha(t *testing.T) {
	var cb b4bCaptcha
	vals := ProValues{}
	fields := []ProField{cb.field(ProField{Captcha: &ProFieldCaptcha{PhoneKey: "phone"}}), {Key: "phone", Label: "Phone", Required: true}}
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 500, func(c *Context) { ProForm(c, fields).Values(&vals).Show() })
	d.ClickLabel(ZhCN.LoginForm.GetCode)
	if len(cb.sent) != 0 {
		t.Fatal("sent before later phone field validated")
	}
}

func TestProFormCaptchaAsyncPhoneIntent(t *testing.T) {
	for _, mode := range []string{"success", "failure", "changed"} {
		t.Run(mode, func(t *testing.T) {
			var cb b4bCaptcha
			vals := ProValues{"phone": "first"}
			var checks []func(string)
			fields := []ProField{cb.field(ProField{Captcha: &ProFieldCaptcha{PhoneKey: "phone"}}), {Key: "phone", Label: "Phone", AsyncValidate: func(_ any, done func(string)) { checks = append(checks, done) }}}
			d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 500, func(c *Context) { ProForm(c, fields).Values(&vals).Show() })
			d.ClickLabel(ZhCN.LoginForm.GetCode)
			if len(cb.sent) != 0 || len(checks) == 0 {
				t.Fatal("did not await phone validation")
			}
			if mode == "changed" {
				vals["phone"] = "second"
				d.Settle()
			}
			msg := ""
			if mode == "failure" {
				msg = "invalid phone"
			}
			checks[0](msg)
			d.Settle()
			if mode == "success" {
				if len(cb.sent) != 1 || cb.sent[0].String("phone") != "first" {
					t.Fatalf("successful validation lost intent: %v", cb.sent)
				}
			} else if len(cb.sent) != 0 {
				t.Fatal("failed or stale validation sent code")
			}
		})
	}
}

func TestProFormModalDestroyRestoresInitial(t *testing.T) {
	for _, drawer := range []bool{false, true} {
		open := true
		vals := ProValues{"a": "original"}
		d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 700, func(c *Context) {
			var m *ModalFormW
			if drawer {
				m = DrawerForm(c, &open, "Edit", []ProField{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta", Initial: "default"}})
			} else {
				m = ModalForm(c, &open, "Edit", []ProField{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta", Initial: "default"}})
			}
			m.Values(&vals).DestroyOnHidden(true).Show()
		})
		if vals.String("a") != "original" {
			t.Fatal("first open discarded controlled initial values")
		}
		vals["a"] = "edited"
		vals["b"] = "changed"
		open = false
		d.Settle()
		open = true
		d.Settle()
		if vals.String("a") != "original" || vals.String("b") != "default" {
			t.Fatalf("destroy failed to restore initial values: %v", vals)
		}
	}
}
