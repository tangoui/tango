package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Backdrop blur in the ui layer: the Runner's damage for blurs and the
// frosted FooterToolbar.

func damages(l *paint.List) []geom.Rect {
	var out []geom.Rect
	for _, op := range l.Ops {
		if d, ok := op.(paint.Damage); ok {
			out = append(out, d.Rect)
		}
	}
	return out
}

// TestAddBlurDamage: a blur is damaged when a hint reaches it (its rect
// grown by its reach), also through another blur's damage and a scroll
// region, under a translate; not for hints out of reach, nor under a
// rotation (where it draws nothing).
func TestAddBlurDamage(t *testing.T) {
	bar := geom.R(0, 400, 300, 40)
	reach := paint.BackdropBlurReach(8, 2)
	if reach < 24 || reach > 80 {
		t.Fatalf("reach %v", reach)
	}
	mk := func(hint paint.Op) *paint.List {
		l := &paint.List{}
		l.Fill(geom.R(0, 0, 300, 500), geom.Radii{}, color.NRGBA{200, 0, 0, 255})
		l.BackdropBlur(bar, geom.Rad(6), 8)
		l.Add(hint)
		return l
	}
	// Out of reach.
	l := mk(paint.Damage{Rect: geom.R(0, 0, 300, 400-reach-2)})
	addBlurDamage(l, 2)
	if d := damages(l); len(d) != 1 {
		t.Fatalf("damage out of reach added %v", d)
	}
	// Within reach above the bar.
	l = mk(paint.Damage{Rect: geom.R(10, 400-reach+2, 20, 10)})
	addBlurDamage(l, 2)
	if d := damages(l); len(d) != 2 || d[1] != bar {
		t.Fatalf("damage within reach: %v", d)
	}
	// A scroll region under it.
	l = mk(paint.ScrollRegion{Rect: geom.R(0, 0, 300, 420), Delta: geom.Pt(0, -10)})
	addBlurDamage(l, 2)
	if d := damages(l); len(d) != 1 || d[0] != bar {
		t.Fatalf("scroll under the blur: %v", d)
	}
	// Chained: the second blur is reached only through the first's damage.
	l = mk(paint.Damage{Rect: geom.R(10, 430, 5, 5)})
	l.Transform(geom.Translate(0, 40+reach-2))
	l.BackdropBlur(bar, geom.Radii{}, 8)
	l.Pop()
	addBlurDamage(l, 2)
	if d := damages(l); len(d) != 3 || d[2] != bar.Offset(0, 40+reach-2) {
		t.Fatalf("chained blurs: %v", d)
	}
	// Under a rotation the blur draws nothing and is not damaged.
	l = &paint.List{}
	l.Transform(geom.RotateAround(0.3, geom.Pt(150, 420)))
	l.BackdropBlur(bar, geom.Radii{}, 8)
	l.Pop()
	l.Damage(bar)
	addBlurDamage(l, 2)
	if d := damages(l); len(d) != 1 {
		t.Fatalf("rotated blur damaged: %v", d)
	}
}

// footerBlurs returns the backdrop blurs of the driver's last frame.
func footerBlurs(d *Driver) []paint.BackdropBlur {
	var out []paint.BackdropBlur
	for _, op := range d.r.List().Ops {
		if b, ok := op.(paint.BackdropBlur); ok {
			out = append(out, b)
		}
	}
	return out
}

func stickyFooterPage(dir Direction) func(c *Context) {
	return func(c *Context) {
		ConfigProvider(c).Direction(dir).Body(func() {
			Scroll(c, func() {
				Column(c, nil, layout.H(1000))
				FooterToolbar(c).Sticky().Body(func() { Button(c, "Pinned") })
			}, layout.Grow(1))
		})
	}
}

// TestFooterToolbarFrosted: pinned, the bar is frosted with its design
// system's blur radius (in right-to-left layouts too, over the bar's own
// rectangle); in flow, in a high-contrast theme and on a renderer without
// backdrop blur it is not.
func TestFooterToolbarFrosted(t *testing.T) {
	for name, mk := range map[string]func() *theme.Theme{
		"ant": func() *theme.Theme { return theme.AntDesign() }, "element": func() *theme.Theme { return theme.ElementPlus() }, "shadcn": func() *theme.Theme { return theme.Shadcn() },
	} {
		for _, dir := range []Direction{LTR, RTL} {
			d := pro2Driver(mk(), 600, 300, stickyFooterPage(dir))
			d.Settle()
			bs := footerBlurs(d)
			tk := mk().Components.FooterToolbar
			if len(bs) != 1 || bs[0].Radius != tk.BlurRadius || tk.BlurRadius <= 0 {
				t.Fatalf("%s %v: blurs %v, token radius %v", name, dir, bs, tk.BlurRadius)
			}
			pinned := pro2MustFind(t, d, "Pinned")
			if r := bs[0].Rect; r.W < 500 || r.Y > pinned.Y || r.Bottom() < pinned.Bottom() {
				t.Fatalf("%s %v: blur rect %v around the button at %v", name, dir, r, pinned)
			}
		}
	}
	// In flow: no frosting.
	d := pro2Driver(theme.AntDesign(), 600, 300, func(c *Context) {
		Scroll(c, func() { Text(c, "Content") }, layout.Grow(1))
		FooterToolbar(c).Body(func() { Button(c, "InFlow") })
	})
	if bs := footerBlurs(d); len(bs) != 0 {
		t.Fatalf("bar in flow frosted: %v", bs)
	}
	// High contrast: opaque, no blur.
	d = pro2Driver(theme.HighContrast(theme.AntDesign()), 600, 300, stickyFooterPage(LTR))
	d.Settle()
	if bs := footerBlurs(d); len(bs) != 0 {
		t.Fatalf("high-contrast bar frosted: %v", bs)
	}
	opaque := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.W > 500 && f.Rect.H > 40 && f.Rect.H < 80 && f.Color.A == 255 {
			opaque = true
		}
	}
	if !opaque {
		t.Fatal("high-contrast bar has no opaque background")
	}
}

// noBlurRenderer is a renderer that says it cannot draw backdrop blurs.
type noBlurRenderer struct{ *paint.CPU }

func (noBlurRenderer) BackdropBlur() bool { return false }

func TestFooterToolbarNoBlurRenderer(t *testing.T) {
	if !rendererBlurs(paint.NewCPU()) || rendererBlurs(noBlurRenderer{paint.NewCPU()}) {
		t.Fatal("rendererBlurs")
	}
	r := NewRunner(stickyFooterPage(LTR), RunnerOptions{Renderer: noBlurRenderer{paint.NewCPU()}, NoAnimations: true})
	if !r.c.noBlur {
		t.Fatal("runner does not see the renderer's missing blur")
	}
}

type footerThemeCase struct {
	name string
	make func(dark bool) *theme.Theme
}

func footerThemeCases() []footerThemeCase {
	return []footerThemeCase{
		{"ant", func(dark bool) *theme.Theme {
			if dark {
				return theme.AntDesign(theme.Dark())
			}
			return theme.AntDesign()
		}},
		{"element", func(dark bool) *theme.Theme {
			if dark {
				return theme.ElementPlus(theme.Dark())
			}
			return theme.ElementPlus()
		}},
		{"shadcn", func(dark bool) *theme.Theme {
			if dark {
				return theme.Shadcn(theme.Dark())
			}
			return theme.Shadcn()
		}},
	}
}

func footerDriver(page func(*Context), th *theme.Theme, noBlur bool) *Driver {
	if !noBlur {
		return NewDriver(page, DriverOptions{Width: 600, Height: 300, Theme: th})
	}
	w := &driverWindow{size: geom.Size{W: 600, H: 300}, scale: 1,
		now: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}
	r := NewRunner(page, RunnerOptions{Theme: th, Renderer: noBlurRenderer{paint.NewCPU()}, NoAnimations: true})
	r.Attach(w)
	return &Driver{r: r, w: w}
}

func footerMatrixPage(target bool, ftOut **FooterToolbarW, sbOut **ScrollBox) func(*Context) {
	return func(c *Context) {
		Column(c, func() {
			sb := Scroll(c, nil, layout.Grow(1))
			*sbOut = sb
			sb.Body(func() {
				Column(c, nil, layout.H(900))
				ft := FooterToolbar(c)
				if target {
					ft.Target(sb.ID())
				} else {
					ft.Sticky()
				}
				*ftOut = ft.Body(func() { Button(c, "Pinned") })
			})
		}, layout.Grow(1), layout.Pad(10))
	}
}

func footerFlowPage(ftOut **FooterToolbarW) func(*Context) {
	return func(c *Context) {
		Column(c, func() {
			Column(c, nil, layout.H(100))
			ft := FooterToolbar(c)
			*ftOut = ft.Body(func() { Button(c, "InFlow") })
		}, layout.Grow(1), layout.Pad(10))
	}
}

func footerSurfaceFill(t *testing.T, d *Driver, ft *FooterToolbarW) paint.FillRect {
	t.Helper()
	bar, ok := d.r.c.RectOf(ft.ID())
	if !ok {
		t.Fatal("footer bar has no painted rectangle")
	}
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect == bar {
			return f
		}
	}
	t.Fatalf("no display-list FillRect for footer bar %v", bar)
	return paint.FillRect{}
}

func assertFooterFrame(t *testing.T, d *Driver, ft *FooterToolbarW, th *theme.Theme, blurred bool) {
	t.Helper()
	bs := footerBlurs(d)
	if blurred {
		tk := th.Components.FooterToolbar
		bar, ok := d.r.c.RectOf(ft.ID())
		if len(bs) != 1 || !ok || bs[0].Radius != tk.BlurRadius || bs[0].Rect != bar {
			t.Fatalf("blurred footer: blurs=%v bar=%v token=%v", bs, bar, tk.BlurRadius)
		}
	} else if len(bs) != 0 {
		t.Fatalf("unblurred footer has backdrop blur: %v", bs)
	}
	fill := footerSurfaceFill(t, d, ft)
	want := th.Components.FooterToolbar.Bg
	if blurred {
		want = th.Components.FooterToolbar.FrostedBg
	}
	if fill.Color != want {
		t.Fatalf("footer FillRect color=%v, want %v (blurred=%v)", fill.Color, want, blurred)
	}
}

func assertOpaqueFooterFrame(t *testing.T, d *Driver, ft *FooterToolbarW, th *theme.Theme) {
	t.Helper()
	if bs := footerBlurs(d); len(bs) != 0 {
		t.Fatalf("opaque footer has backdrop blur: %v", bs)
	}
	bar, ok := d.r.c.RectOf(ft.ID())
	if !ok {
		t.Fatal("opaque footer has no painted rectangle")
	}
	fill := footerSurfaceFill(t, d, ft)
	if fill.Rect != bar {
		t.Fatalf("opaque footer FillRect rect=%v, want bar=%v", fill.Rect, bar)
	}
	if fill.Color != th.BgContainer || fill.Color.A != 255 {
		t.Fatalf("opaque footer FillRect color=%v, want BgContainer=%v alpha 255", fill.Color, th.BgContainer)
	}
}

func assertFooterGeometry(t *testing.T, d *Driver, ft *FooterToolbarW, sb *ScrollBox, target bool) {
	t.Helper()
	bar, ok := d.r.c.RectOf(ft.ID())
	if !ok {
		t.Fatal("footer has no painted rectangle")
	}
	viewport, ok := d.r.c.RectOf(sb.ID())
	if !ok {
		t.Fatal("scroll viewport has no painted rectangle")
	}
	wantBottom := d.w.size.H
	if target {
		wantBottom = viewport.Bottom()
	}
	if bar.W != viewport.W || bar.Y != wantBottom-bar.H || bar.Bottom() != wantBottom {
		t.Fatalf("footer geometry=%v, want width=%v y=%v bottom=%v (viewport=%v target=%v)", bar, viewport.W, wantBottom-bar.H, wantBottom, viewport, target)
	}
}

func assertFooterAffixed(t *testing.T, ft *FooterToolbarW, want bool) {
	t.Helper()
	got := ft.affix != nil && ft.affix.Affixed()
	if got != want {
		t.Fatalf("footer affixed=%v, want %v", got, want)
	}
}

// TestFooterToolbarStateMatrix checks each design language in light, dark and
// high-contrast variants through the first flow frame, affixing, scroll-away,
// return, and resize. Target repeats the same sequence inside a ScrollBox.
func TestFooterToolbarStateMatrix(t *testing.T) {
	for _, tc := range footerThemeCases() {
		for _, dark := range []bool{false, true} {
			for _, highContrast := range []bool{false, true} {
				name := tc.name
				if dark {
					name += "-dark"
				}
				if highContrast {
					name += "-hc"
				}
				t.Run(name, func(t *testing.T) {
					th := tc.make(dark)
					if highContrast {
						th = theme.HighContrast(th)
					}
					for _, target := range []bool{false, true} {
						t.Run(map[bool]string{false: "window", true: "target"}[target], func(t *testing.T) {
							var ft *FooterToolbarW
							var sb *ScrollBox
							d := footerDriver(footerMatrixPage(target, &ft, &sb), th, false)

							// The first recording is still in flow; the overlay marks
							// the affix for the following frame.
							d.Frame()
							if len(footerBlurs(d)) != 0 {
								t.Fatal("first footer frame is already blurred")
							}
							d.Frame()
							assertFooterAffixed(t, ft, true)
							if highContrast {
								assertOpaqueFooterFrame(t, d, ft, th)
								if len(footerBlurs(d)) != 0 {
									t.Fatal("high-contrast footer blurred")
								}
							} else {
								assertFooterFrame(t, d, ft, th, true)
							}
							assertFooterGeometry(t, d, ft, sb, target)

							// Scroll beyond the toolbar and then return to the top.
							sb.ScrollTo(0, 800)
							d.Frame()
							d.Frame()
							assertFooterAffixed(t, ft, false)
							if len(footerBlurs(d)) != 0 {
								r, _ := d.r.c.RectOf(ft.ID())
								x, y := sb.Offset()
								t.Fatalf("footer remained blurred after scrolling past it: rect=%v offset=(%v,%v) affixed=%v", r, x, y, ft.affix != nil && ft.affix.Affixed())
							}
							sb.ScrollTo(0, 0)
							d.Frame()
							d.Frame()
							assertFooterAffixed(t, ft, true)
							if highContrast {
								assertOpaqueFooterFrame(t, d, ft, th)
							} else {
								assertFooterFrame(t, d, ft, th, true)
							}

							d.Resize(520, 220)
							d.Frame()
							assertFooterAffixed(t, ft, true)
							assertFooterGeometry(t, d, ft, sb, target)
							if highContrast {
								assertOpaqueFooterFrame(t, d, ft, th)
							} else {
								assertFooterFrame(t, d, ft, th, true)
							}
						})
					}
				})
			}
		}
	}
}

func TestFooterToolbarOpaqueFallbacks(t *testing.T) {
	for _, tc := range footerThemeCases() {
		for _, dark := range []bool{false, true} {
			for _, highContrast := range []bool{false, true} {
				name := tc.name
				if dark {
					name += "-dark"
				}
				if highContrast {
					name += "-hc"
				}
				t.Run(name, func(t *testing.T) {
					th := tc.make(dark)
					if highContrast {
						th = theme.HighContrast(th)
					}
					var ft *FooterToolbarW
					d := footerDriver(footerFlowPage(&ft), th, false)
					d.Settle()
					fill := footerSurfaceFill(t, d, ft)
					if fill.Color != th.Components.FooterToolbar.Bg {
						t.Fatalf("flow fallback=%v, want token Bg=%v", fill.Color, th.Components.FooterToolbar.Bg)
					}

					for _, target := range []bool{false, true} {
						var sticky *FooterToolbarW
						var sb *ScrollBox
						d := footerDriver(footerMatrixPage(target, &sticky, &sb), th, true)
						d.Frame()
						d.Frame()
						assertFooterAffixed(t, sticky, true)
						assertOpaqueFooterFrame(t, d, sticky, th)
						assertFooterGeometry(t, d, sticky, sb, target)

						// Leaving the footer must remove the affixed surface; returning
						// to the top must rebuild the opaque fallback.
						sb.ScrollTo(0, 800)
						d.Frame()
						d.Frame()
						assertFooterAffixed(t, sticky, false)
						if len(footerBlurs(d)) != 0 {
							t.Fatal("no-blur footer emitted BackdropBlur after scroll-away")
						}
						sb.ScrollTo(0, 0)
						d.Frame()
						d.Frame()
						assertFooterAffixed(t, sticky, true)
						assertOpaqueFooterFrame(t, d, sticky, th)
						assertFooterGeometry(t, d, sticky, sb, target)

						d.Resize(520, 220)
						d.Frame()
						assertFooterAffixed(t, sticky, true)
						assertOpaqueFooterFrame(t, d, sticky, th)
						assertFooterGeometry(t, d, sticky, sb, target)
					}
				})
			}
		}
	}
}
