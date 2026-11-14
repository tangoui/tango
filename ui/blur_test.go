package ui

import (
	"image/color"
	"testing"

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
