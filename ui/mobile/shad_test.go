package mobile

import (
	"image"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func shadPhone(page func(c *ui.Context), anim, dark bool) *ui.Driver {
	th := theme.Shadcn()
	if dark {
		th = theme.Shadcn(theme.Dark())
	}
	return ui.NewDriver(page, ui.DriverOptions{Width: 390, Height: 844, Animations: anim, Theme: th})
}

// px returns the color of the logical point p in a 1× driver frame.
func px(img *image.RGBA, p geom.Point) theme.Color {
	c := img.RGBAAt(int(p.X), int(p.Y))
	return theme.RGBA(c.R, c.G, c.B, float64(c.A)/255)
}

func near(a, b theme.Color) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= 3 && d(a.G, b.G) <= 3 && d(a.B, b.B) <= 3
}

func TestShadcnThemeResolution(t *testing.T) {
	for _, dark := range []bool{false, true} {
		base := theme.Shadcn()
		if dark {
			base = theme.Shadcn(theme.Dark())
		}
		th := NewTheme(base, AntMobile)
		if th.Style != Shadcn || !th.shad() {
			t.Fatalf("dark=%v: AntMobile under shadcn resolved to %v", dark, th.Style)
		}
		v := base.Shadcn
		if th.Primary != v.Primary || th.OnPrimary != v.PrimaryForeground || th.Text != v.Foreground || th.Border != v.Border {
			t.Fatalf("dark=%v: shadcn colors not taken from the theme variables", dark)
		}
		if th.CardBorder.A == 0 || th.DrawerHandle.A == 0 || th.ToastBorder.A == 0 {
			t.Fatalf("dark=%v: shadcn chrome tokens missing", dark)
		}
		if th.ButtonHeight[2] < 44 || th.TouchMin != 44 || th.SwitchW != 48 || th.SwitchH != 28 {
			t.Fatalf("dark=%v: touch sizes %v %v×%v", dark, th.ButtonHeight, th.SwitchW, th.SwitchH)
		}
		if th.Box == th.PageBg {
			t.Fatalf("dark=%v: cards must differ from the page", dark)
		}
		if NewTheme(base, Vant).Style != Vant {
			t.Fatal("explicit Vant must stay Vant")
		}
	}
	// Explicit Shadcn on another ui theme falls back to stock shadcn colors.
	if th := NewTheme(theme.AntDesign(), Shadcn); th.Primary != theme.Shadcn().Shadcn.Primary {
		t.Fatal("Shadcn on an Ant theme must use the stock shadcn primary")
	}
	// Ant Mobile / Vant keep their values for the new tokens.
	am := NewTheme(theme.AntDesign(), AntMobile)
	if am.OnSolid != am.OnPrimary || am.SwitchW != 51 || am.SwitchOff != theme.Hex("#e5e5e5") || am.CardBorder.A != 0 || am.DrawerHandle.A != 0 {
		t.Fatal("Ant Mobile tokens changed")
	}
	if vt := NewTheme(theme.AntDesign(theme.Dark()), Vant); vt.SwitchW != 52 || vt.SwitchOff != vt.Fill {
		t.Fatal("Vant switch tokens changed")
	}
}

func TestShadcnDrawerHandleDrag(t *testing.T) {
	open := true
	var panel ui.ID
	closed := 0
	d := shadPhone(func(c *ui.Context) {
		p := Popup(c, &open).A11yLabel("sheet").Body(func() {
			ui.Div(c, layout.H(300))
		})
		if p.panel != nil {
			panel = p.panel.ID()
		}
		if p.Closed() {
			closed++
		}
	}, true, false)
	run(d, 500*time.Millisecond)
	// The sheet is 28 (handle) + 300 tall at the bottom.
	top := float32(844 - 328)
	img := d.Frame()
	handle := px(img, geom.Pt(195, top+14))
	if !near(handle, theme.Shadcn().Shadcn.Border) {
		t.Fatalf("no grab handle at the top of the drawer: %v", handle)
	}
	if panel == 0 {
		t.Fatal("panel not built")
	}
	// A short drag springs back.
	f := newFinger(t, d)
	f.down(geom.Pt(195, top+14))
	f.move(geom.Pt(195, top+54), 4, 16*time.Millisecond)
	f.hold(200 * time.Millisecond)
	f.up()
	run(d, 500*time.Millisecond)
	if !open || closed != 0 {
		t.Fatal("a short drag must not close the drawer")
	}
	if c := px(d.Frame(), geom.Pt(195, top+14)); !near(c, theme.Shadcn().Shadcn.Border) {
		t.Fatalf("the drawer did not spring back: %v", c)
	}
	// Dragging past a quarter of the height closes it.
	f.down(geom.Pt(195, top+14))
	f.move(geom.Pt(195, top+214), 8, 16*time.Millisecond)
	f.hold(200 * time.Millisecond)
	f.up()
	run(d, 600*time.Millisecond)
	if open || closed != 1 {
		t.Fatalf("a long drag must close the drawer (open=%v closed=%d)", open, closed)
	}
}

func TestShadcnActionSheetCards(t *testing.T) {
	open := true
	sel := -1
	d := shadPhone(func(c *ui.Context) {
		w := ActionSheet(c, &open, []Action{{Text: "复制"}, {Text: "删除", Danger: true}}).Cancel("")
		if i, ok := w.Selected(); ok {
			sel = i
		}
	}, false, false)
	d.Frame()
	img := d.Frame()
	th := NewTheme(theme.Shadcn(), Shadcn)
	// Cancel card: the last 52px above the bottom margin (8px padding +
	// 8px gap before the safe-area spacer); the gap between the cards
	// shows the mask, not a card.
	cancel := geom.Pt(195, 844-16-26)
	gap := geom.Pt(195, 844-16-52-4)
	if !near(px(img, cancel), th.Box) {
		t.Fatalf("cancel card: %v", px(img, cancel))
	}
	if near(px(img, gap), th.Box) {
		t.Fatal("cancel must be a separate card")
	}
	// Side margin: the sheet floats 8px from the edges.
	if near(px(img, geom.Pt(3, 844-16-26)), th.Box) {
		t.Fatal("cards must be inset from the screen edges")
	}
	d.Tap(geom.Pt(195, 844-16-52-8-52-26))
	d.Frame()
	if sel != 0 {
		t.Fatalf("tap on the first action: sel=%d", sel)
	}
}

func TestShadcnSonnerToast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		show := true
		d := shadPhone(func(c *ui.Context) {
			Toast(c, &show, "已保存").Icon("success").Duration(0).Show()
		}, false, dark)
		d.Frame()
		img := d.Frame()
		base := theme.Shadcn()
		if dark {
			base = theme.Shadcn(theme.Dark())
		}
		th := NewTheme(base, AntMobile)
		// Bottom by default, 16px from the edge; the card is the popover
		// color (not the dark Ant Mobile pill).
		if c := px(img, geom.Pt(360, 844-16-10)); !near(c, th.ToastBg) {
			t.Fatalf("dark=%v: toast card at the bottom: %v want %v", dark, c, th.ToastBg)
		}
		if c := px(img, geom.Pt(195, 422)); near(c, th.ToastBg) && !near(th.ToastBg, theme.Hex("#ffffff")) {
			t.Fatalf("dark=%v: toast must not be centred", dark)
		}
	}
}

func TestShadcnSegmentedTabs(t *testing.T) {
	active := 0
	d := shadPhone(func(c *ui.Context) {
		Tabs(c, []string{"一", "二", "三"}, &active).Header()
	}, false, false)
	d.Frame()
	img := d.Frame()
	th := NewTheme(theme.Shadcn(), AntMobile)
	// Track inset by PadH, thumb on the first segment.
	seg := (390 - 2*th.PadH) / 3
	if c := px(img, geom.Pt(th.PadH+seg/2, 12)); !near(c, th.SegmentThumb) {
		t.Fatalf("thumb: %v", c)
	}
	if c := px(img, geom.Pt(th.PadH+seg*1.5+20, 12)); !near(c, th.Fill) {
		t.Fatalf("track: %v", c)
	}
	d.Tap(geom.Pt(th.PadH+seg*2.5, 26))
	d.Frame()
	if active != 2 {
		t.Fatalf("tap on the third segment: %d", active)
	}
}

func TestShadcnSwitchAndStepper(t *testing.T) {
	on := false
	v := 1.0
	var cell, st ui.ID
	var cr, sr geom.Rect
	d := shadPhone(func(c *ui.Context) {
		cell = Cell(c, "Wi-Fi").Switch(&on).ID()
		ui.Row(c, func() { st = Stepper(c, &v).Max(2).A11yLabel("qty").b.ID() })
		cr, _ = c.RectOf(cell)
		sr, _ = c.RectOf(st)
	}, false, false)
	d.Frame()
	d.Frame()
	th := NewTheme(theme.Shadcn(), Shadcn)
	if sr.W != 2*th.StepperHeight+th.StepperInputW {
		t.Fatalf("stepper width %v", sr.W)
	}
	img := d.Frame()
	// Off track = input color.
	sw := geom.Pt(cr.Right()-th.PadH-6, cr.Y+cr.H/2)
	if c := px(img, sw); !near(c, th.SwitchOff) {
		t.Fatalf("switch off track: %v want %v", c, th.SwitchOff)
	}
	d.Tap(geom.Pt(cr.Right()-th.PadH-20, cr.Y+cr.H/2))
	d.Frame()
	if !on {
		t.Fatal("switch tap")
	}
	if c := px(d.Frame(), geom.Pt(cr.Right()-th.PadH-th.SwitchW+6, cr.Y+cr.H/2)); !near(c, th.Primary) {
		t.Fatalf("switch on track: %v", c)
	}
	// Outlined group: the border color at the left edge of the stepper.
	if c := px(img, geom.Pt(sr.X+0.5, sr.Y+sr.H/2)); near(c, th.Box) {
		t.Fatal("stepper must be outlined")
	}
	d.Tap(geom.Pt(sr.Right()-th.StepperHeight/2, sr.Y+sr.H/2))
	d.Frame()
	if v != 2 {
		t.Fatalf("stepper +: %v", v)
	}
}

func TestMobileConfigDefaults(t *testing.T) {
	var big, small, plain geom.Rect
	var stepV = 5.0
	var step *StepperW
	var nav *NavBarW
	var dlg *DialogW
	open := true
	d := shadPhone(func(c *ui.Context) {
		ui.ConfigProvider(c).
			Defaults("mobile.Button", ui.Props{"size": "large", "shape": "round"}).
			Defaults("mobile.Stepper", ui.Props{"min": 2, "max": 6, "step": 2}).
			Defaults("mobile.NavBar", ui.Props{"bordered": false}).
			Defaults("mobile.Dialog", ui.Props{"confirmText": "好的"}).
			Body(func() {
				b1 := Button(c, "A")
				b2 := Button(c, "B").Size(Small) // explicit wins
				big, _ = c.RectOf(b1.ID())
				small, _ = c.RectOf(b2.ID())
				step = Stepper(c, &stepV)
				nav = NavBar(c, "T")
				dlg = Dialog(c, &open, "标题").Show()
			})
		b3 := Button(c, "C") // outside the provider
		plain, _ = c.RectOf(b3.ID())
	}, false, false)
	d.Frame()
	d.Frame()
	th := NewTheme(theme.Shadcn(), Shadcn)
	if big.H != th.ButtonHeight[3] || small.H != th.ButtonHeight[1] || plain.H != th.ButtonHeight[2] {
		t.Fatalf("button heights %v %v %v", big.H, small.H, plain.H)
	}
	if step.min != 2 || step.max != 6 || step.step != 2 {
		t.Fatalf("stepper defaults %v %v %v", step.min, step.max, step.step)
	}
	if nav.border {
		t.Fatal("NavBar bordered default")
	}
	if dlg.ok != "好的" {
		t.Fatalf("Dialog confirmText: %q", dlg.ok)
	}
}

func TestMobileConfigDefaultsKeepAntLook(t *testing.T) {
	// Defaults apply to Ant Mobile too (no shadcn branch involved).
	var r geom.Rect
	d := phone(func(c *ui.Context) {
		ui.ConfigProvider(c).Defaults("mobile.Button", ui.Props{"size": Mini}).Body(func() {
			r, _ = c.RectOf(Button(c, "A").ID())
		})
	}, false)
	d.Frame()
	d.Frame()
	if r.H != NewTheme(theme.AntDesign(), AntMobile).ButtonHeight[0] {
		t.Fatalf("mini height %v", r.H)
	}
}

func TestPopupDefaultsStayOnPopup(t *testing.T) {
	th := NewTheme(theme.Shadcn(), Shadcn)
	props := ui.Props{"position": "top", "handle": false}
	open := true
	d := shadPhone(func(c *ui.Context) {
		ui.ConfigProvider(c).Defaults("mobile.Popup", props).Body(func() {
			Popup(c, &open).A11yLabel("p").Body(func() { ui.Div(c, layout.H(100)) })
		})
	}, false, false)
	d.Frame()
	d.Frame()
	if c := px(d.Frame(), geom.Pt(195, 50)); !near(c, th.Box) {
		t.Fatalf("popup position default not applied: %v", c)
	}
	// The action sheet (built on a popup) stays at the bottom.
	open = true
	d = shadPhone(func(c *ui.Context) {
		ui.ConfigProvider(c).Defaults("mobile.Popup", props).Body(func() {
			ActionSheet(c, &open, []Action{{Text: "x"}}).Show()
		})
	}, false, false)
	d.Frame()
	d.Frame()
	img := d.Frame()
	if !near(px(img, geom.Pt(60, 844-16-26)), th.Box) || near(px(img, geom.Pt(60, 50)), th.Box) {
		t.Fatal("ActionSheet must ignore the Popup position default")
	}
}
