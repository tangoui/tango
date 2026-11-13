package ui

import (
	"image"
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// probe is a leaf recording the scope seen by its measure and paint.
type probe struct {
	measDark, paintDark bool
	paintRTL, measRTL   bool
	loc                 *Locale
	th                  *theme.Theme
}

func addProbe(c *Context, p *probe) {
	Custom(c, func(_, _ float32) geom.Size {
		p.measDark, p.measRTL = c.Theme.Dark, c.IsRTL()
		return geom.Size{W: 10, H: 10}
	}, func(_ *paint.List, _ geom.Rect) {
		p.paintDark, p.paintRTL, p.loc, p.th = c.Theme.Dark, c.IsRTL(), c.Locale(), c.Theme
	})
}

// TestEnvNestedProviderIsLocal: a nested ConfigProvider themes, localizes
// and orients only the widgets inside it, at paint and measure time too.
func TestEnvNestedProviderIsLocal(t *testing.T) {
	var before, inner, after probe
	var innerNode *node
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			addProbe(c, &before)
			ConfigProvider(c).Dark(true).Locale(EnUS).Direction(RTL).Body(func() {
				addProbe(c, &inner)
				innerNode = c.cur.kids[len(c.cur.kids)-1]
			})
			addProbe(c, &after)
		})
	}, DriverOptions{Width: 200, Height: 200})
	d.Settle()
	if before.paintDark || after.paintDark || before.measDark || after.measDark {
		t.Errorf("outer nodes painted dark: before=%+v after=%+v", before, after)
	}
	if !inner.paintDark || !inner.measDark {
		t.Errorf("inner node not dark: paint=%v measure=%v", inner.paintDark, inner.measDark)
	}
	if inner.loc != EnUS {
		t.Errorf("inner paint locale = %v, want en-US", inner.loc.Code)
	}
	if after.loc != defaultLocaleNow() || before.loc != defaultLocaleNow() {
		t.Errorf("outer paint locale = %v / %v", before.loc.Code, after.loc.Code)
	}
	if !inner.paintRTL || !inner.measRTL || before.paintRTL || after.paintRTL {
		t.Errorf("direction: inner=%v/%v before=%v after=%v", inner.paintRTL, inner.measRTL, before.paintRTL, after.paintRTL)
	}
	if innerNode == nil || !innerNode.ln.RTL {
		t.Error("inner node's layout style not marked RTL")
	}
	if win := d.Runner().Context().Theme; win.Dark {
		t.Error("nested provider changed the window theme")
	}
}

// TestEnvRootProviderSetsWindowTheme keeps Q7 (b): a provider at the page
// root still themes the window, including widgets built before it.
func TestEnvRootProviderSetsWindowTheme(t *testing.T) {
	var before, inner probe
	d := NewDriver(func(c *Context) {
		addProbe(c, &before)
		ConfigProvider(c).Dark(true).Body(func() { addProbe(c, &inner) })
	}, DriverOptions{Width: 200, Height: 200})
	d.Settle()
	if !d.Runner().Context().Theme.Dark || !before.paintDark || !inner.paintDark {
		t.Fatalf("root provider: window=%v before=%v inner=%v", d.Runner().Context().Theme.Dark, before.paintDark, inner.paintDark)
	}
	if px := d.Image().RGBAAt(150, 150); px.R > 80 {
		t.Errorf("window background not dark: %v", px)
	}
}

// TestEnvNestedThemePixels: inner box painted with a dark token, the page
// background stays light.
func TestEnvNestedThemePixels(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Dark(true).Body(func() {
				Custom(c, nil, func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, c.Theme.BgContainer) }, layout.Size(50, 50))
			})
		})
	}, DriverOptions{Width: 200, Height: 200})
	img := d.Settle()
	if in := img.RGBAAt(25, 25); in.R > 80 {
		t.Errorf("inner fill not dark: %v", in)
	}
	if out := img.RGBAAt(150, 150); out.R < 180 {
		t.Errorf("page background not light: %v", out)
	}
}

// TestEnvReuseNoAlloc: a scope without overrides reuses the parent env, and
// a cached override env is reused without allocating.
func TestEnvReuseNoAlloc(t *testing.T) {
	c := newContext(theme.AntDesign())
	root := c.env
	if n := testing.AllocsPerRun(100, func() {
		if c.deriveEnv(ID(1), c.Theme, c.IsRTL(), nil) != root {
			t.Fatal("no-override scope did not reuse the parent env")
		}
	}); n != 0 {
		t.Errorf("no-override scope allocates %v", n)
	}
	dark := theme.AntDesign(theme.Dark())
	e := c.deriveEnv(ID(2), dark, true, EnUS)
	if n := testing.AllocsPerRun(100, func() {
		if c.deriveEnv(ID(2), dark, true, EnUS) != e {
			t.Fatal("cached env not reused")
		}
	}); n != 0 {
		t.Errorf("cached override scope allocates %v", n)
	}
	if e2 := c.deriveEnv(ID(2), dark, false, EnUS); e2 == e || e.rtl != true {
		t.Error("changed values must give a fresh env and leave the old one intact")
	}
}

// TestEnvProviderNoAllocAcrossFrames: frames of a page with a plain
// provider do not allocate envs (the env pointer stays the same).
func TestEnvProviderStableAcrossFrames(t *testing.T) {
	var seen []*env
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Locale(EnUS).Body(func() { seen = append(seen, c.env) })
		})
	}, DriverOptions{Width: 100, Height: 100})
	d.Frame()
	d.Frame()
	if len(seen) < 2 || seen[0] != seen[len(seen)-1] {
		t.Fatalf("env not cached across frames: %v", seen)
	}
}

func TestLocaleRTL(t *testing.T) {
	cases := map[string]bool{
		"ar": true, "ar-EG": true, "AR_sa": true, "he-IL": true, "iw": true, "fa_IR.UTF-8": true,
		"ur-PK": true, "ps": true, "ckb": true, "dv": true, "yi": true, "ug-CN": true, "syr": true, "nqo": true,
		"az-Arab": true, "pa-Arab-PK": true, "ff-Adlm": true, "sd-Arab@x": true,
		"ku-Latn": false, "ug-Cyrl": false, "az-Cyrl": false, "ar-Latn": false,
		"en-US": false, "zh-CN": false, "zh-Hant-TW": false, "ja": false, "": false, "arabic": false, "art": false,
	}
	for code, want := range cases {
		if got := localeRTL(code); got != want {
			t.Errorf("localeRTL(%q) = %v, want %v", code, got, want)
		}
	}
	if n := testing.AllocsPerRun(50, func() { localeRTL("pa-Arab-PK.UTF-8") }); n != 0 {
		t.Errorf("localeRTL allocates %v", n)
	}
}

func TestResolveDirection(t *testing.T) {
	defer SetDefaultDirection(DirAuto)
	ar := &Locale{Code: "ar-EG"}
	forced := &Locale{Code: "ar-EG", Dir: LTR}
	heb := &Locale{Code: "en-US", Dir: RTL}
	cases := []struct {
		explicit Direction
		loc      *Locale
		def      Direction
		want     bool
	}{
		{DirAuto, nil, DirAuto, defaultLocaleNow().IsRTL()},
		{DirAuto, nil, RTL, true},
		{DirAuto, ar, DirAuto, true},
		{DirAuto, ar, LTR, true}, // the scope's locale beats the window default
		{DirAuto, forced, DirAuto, false},
		{DirAuto, heb, DirAuto, true},
		{DirAuto, EnUS, RTL, false},
		{LTR, ar, RTL, false},
		{RTL, EnUS, LTR, true},
	}
	for i, tc := range cases {
		SetDefaultDirection(tc.def)
		if got := resolveRTL(tc.explicit, tc.loc); got != tc.want {
			t.Errorf("case %d: resolveRTL = %v, want %v", i, got, tc.want)
		}
	}
}

// TestDirectionScopes: provider chain resolution as seen by c.Direction().
func TestDirectionScopes(t *testing.T) {
	defer SetDefaultDirection(DirAuto)
	ar := &Locale{Code: "ar"}
	got := map[string]Direction{}
	page := func(c *Context) {
		got["root"] = c.Direction()
		ConfigProvider(c).Locale(ar).Body(func() {
			got["ar"] = c.Direction()
			ConfigProvider(c).Direction(LTR).Body(func() {
				got["ltr"] = c.Direction()
				ConfigProvider(c).Locale(EnUS).Body(func() { got["inherit-explicit"] = c.Direction() })
				ConfigProvider(c).Direction(DirAuto).Body(func() { got["auto-again"] = c.Direction() })
			})
			got["ar-after"] = c.Direction()
		})
		got["root-after"] = c.Direction()
	}
	d := NewDriver(page, DriverOptions{Width: 100, Height: 100})
	d.Frame()
	want := map[string]Direction{"root": LTR, "ar": RTL, "ltr": LTR, "inherit-explicit": LTR, "auto-again": RTL, "ar-after": RTL, "root-after": LTR}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %v, want %v", k, got[k], w)
		}
	}
	SetDefaultDirection(RTL)
	d.Frame()
	if got["root"] != RTL || got["root-after"] != RTL {
		t.Errorf("SetDefaultDirection(RTL): root=%v", got["root"])
	}
	if d.Runner().Context().root.ln.RTL != true {
		t.Error("root node not marked RTL")
	}
}

// TestRovingStepRTL: Left/Right swap in an RTL scope, Up/Down do not.
func TestRovingStepRTL(t *testing.T) {
	for _, tc := range []struct {
		dir  Direction
		key  input.Key
		want int
	}{
		{LTR, input.KeyRight, 2}, {LTR, input.KeyLeft, 0},
		{RTL, input.KeyRight, 0}, {RTL, input.KeyLeft, 2},
		{RTL, input.KeyDown, 2}, {RTL, input.KeyUp, 0},
	} {
		got := -1
		focused := false
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Direction(tc.dir).Body(func() {
				Row(c, func() {
					ids := make([]ID, 3)
					for i := range ids {
						n := c.add("item", strconv.Itoa(i))
						n.ln.Width, n.ln.Height = 10, 10
						c.interact(n, true, input.CursorDefault)
						ids[i] = n.id
					}
					if !focused {
						c.SetFocus(ids[1])
						focused = true
					}
					if to := rovingStep(c, ids, nil, rovingBoth); to >= 0 && got < 0 {
						got = to
					}
				})
			})
		}, DriverOptions{Width: 100, Height: 100})
		d.Settle()
		d.Key(tc.key, 0)
		if got != tc.want {
			t.Errorf("%v %v: moved to %d, want %d", tc.dir, tc.key, got, tc.want)
		}
	}
}

func TestInlineSpan(t *testing.T) {
	r := geom.Rect{X: 10, Y: 5, W: 100, H: 20}
	l, rt := inlineSpan{r: r}, inlineSpan{r: r, rtl: true}
	if l.X(8, 14) != 18 || rt.X(8, 14) != 88 {
		t.Errorf("X: %v %v", l.X(8, 14), rt.X(8, 14))
	}
	if got := rt.Rect(0, 7, 20, 10); got != (geom.Rect{X: 90, Y: 7, W: 20, H: 10}) {
		t.Errorf("Rect: %v", got)
	}
	if l.Start() != 10 || l.End() != 110 || rt.Start() != 110 || rt.End() != 10 {
		t.Error("Start/End")
	}
	if l.Off(30) != 20 || rt.Off(30) != 80 || l.Sign() != 1 || rt.Sign() != -1 {
		t.Error("Off/Sign")
	}
	p := geom.Pt(35, 0)
	if l.Frac(p) != 0.25 || rt.Frac(p) != 0.75 || rt.Frac(geom.Pt(-50, 0)) != 1 || (inlineSpan{}).Frac(p) != 0 {
		t.Errorf("Frac: %v %v", l.Frac(p), rt.Frac(p))
	}
	box := geom.Rect{X: 10, W: 100, H: 50}
	if got := mirrorRect(geom.Rect{X: 20, Y: 3, W: 30, H: 4}, box); got != (geom.Rect{X: 70, Y: 3, W: 30, H: 4}) {
		t.Errorf("mirrorRect: %v", got)
	}
	// Mirroring an element placed with the LTR span gives the RTL span's rect.
	if mirrorRect(l.Rect(8, 0, 14, 1), r) != rt.Rect(8, 0, 14, 1) {
		t.Error("mirrorRect disagrees with inlineSpan")
	}
}

func TestTextOptsAndAlign(t *testing.T) {
	c := newContext(theme.AntDesign())
	if o := c.textOpts(text.Options{}); o.Direction != text.DirAuto {
		t.Error("LTR scope must keep DirAuto")
	}
	c.dir.rtl = true
	if o := c.textOpts(text.Options{}); o.Direction != text.DirRTL {
		t.Error("RTL scope must set DirRTL")
	}
	if o := c.textOpts(text.Options{Direction: text.DirLTR}); o.Direction != text.DirLTR {
		t.Error("explicit direction overridden")
	}
	if prev, next := c.inlineKey(input.KeyLeft); prev || !next {
		t.Error("RTL Left must be next")
	}
	if AlignLeft != AlignStart || AlignStart != 0 || AlignRight != AlignEnd {
		t.Error("transitional aliases changed")
	}
	cases := []struct {
		a    TextAlign
		rtl  bool
		want float32
	}{
		{AlignStart, false, 0}, {AlignStart, true, 60}, {AlignEnd, false, 60}, {AlignEnd, true, 0},
		{AlignCenter, true, 30}, {AlignPhysicalLeft, true, 0}, {AlignPhysicalRight, false, 60},
	}
	for _, tc := range cases {
		if got := alignLine(tc.a, 100, 40, tc.rtl); got != tc.want {
			t.Errorf("alignLine(%d, rtl=%v) = %v, want %v", tc.a, tc.rtl, got, tc.want)
		}
		if !tc.rtl && alignShift(tc.a, 100, 40) != tc.want {
			t.Errorf("alignShift(%d) differs from LTR alignLine", tc.a)
		}
	}
}

// TestLinkTypeAndColor: Link honours Type and Color; paint does not
// overwrite them, and the default colors are unchanged.
func TestLinkTypeAndColor(t *testing.T) {
	th := theme.AntDesign()
	custom := theme.Hex("#00aa00")
	for _, tc := range []struct {
		name string
		mod  func(*LinkW)
		want theme.Color
	}{
		{"default", func(*LinkW) {}, th.TextLink},
		{"danger", func(l *LinkW) { l.Type("danger") }, th.Error.Base},
		{"success", func(l *LinkW) { l.Type("success") }, th.Success.Base},
		{"secondary", func(l *LinkW) { l.Secondary() }, th.TextSecondary},
		{"color", func(l *LinkW) { l.Color(custom) }, custom},
	} {
		var lw *LinkW
		d := NewDriver(func(c *Context) {
			lw = Link(c, "HHHHHH")
			lw.Size(24)
			tc.mod(lw)
		}, DriverOptions{Width: 300, Height: 60, Theme: th})
		d.Frame()
		img := d.Frame() // paint twice: the choice must survive
		r, ok := d.Find("HHHHHH")
		if !ok {
			t.Fatalf("%s: link not found", tc.name)
		}
		if !hasColor(img, r, over(tc.want, th.BgLayout), 24) {
			t.Errorf("%s: no pixel near %v in %v", tc.name, tc.want, r)
		}
	}
}

// over composites col on an opaque background.
func over(col, bg theme.Color) theme.Color {
	a := int(col.A)
	mix := func(f, b uint8) uint8 { return uint8((int(f)*a + int(b)*(255-a)) / 255) }
	return theme.Color{R: mix(col.R, bg.R), G: mix(col.G, bg.G), B: mix(col.B, bg.B), A: 255}
}

func hasColor(img *image.RGBA, r geom.Rect, want theme.Color, tol int) bool {
	wr, wg, wb := int(want.R), int(want.G), int(want.B)
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	for y := int(r.Y); y < int(r.Bottom()); y++ {
		for x := int(r.X); x < int(r.Right()); x++ {
			p := img.RGBAAt(x, y)
			if abs(int(p.R)-wr) <= tol && abs(int(p.G)-wg) <= tol && abs(int(p.B)-wb) <= tol {
				return true
			}
		}
	}
	return false
}

// TestEnvNestedComponents: component token overrides of a nested provider
// apply only inside it, at build and paint time, and are cached.
func TestEnvNestedComponents(t *testing.T) {
	var outerB, innerB, outerP, innerP, rootTok float32
	var innerTh []*theme.Theme
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			outerB = c.Token().Button.PaddingInline
			Custom(c, nil, func(*paint.List, geom.Rect) { outerP = c.Token().Button.PaddingInline }, layout.Size(10, 10))
			ConfigProvider(c).Components(theme.Components{Button: theme.ButtonToken{PaddingInline: 30}}).Body(func() {
				innerB = c.ThemeFor(theme.CompButton).Components.Button.PaddingInline
				innerTh = append(innerTh, c.Theme)
				Custom(c, nil, func(*paint.List, geom.Rect) { innerP = c.Token().Button.PaddingInline }, layout.Size(10, 10))
			})
		})
		rootTok = c.Token().Button.PaddingInline
	}, DriverOptions{Width: 200, Height: 200})
	d.Frame()
	d.Frame()
	if innerB != 30 || innerP != 30 {
		t.Errorf("inner PaddingInline build=%v paint=%v, want 30", innerB, innerP)
	}
	if outerP == 0 {
		t.Error("outer probe not painted")
	}
	if outerB == 30 || outerP == 30 || rootTok == 30 || d.Runner().Context().Theme.Components.Button.PaddingInline == 30 {
		t.Errorf("override leaked: outer=%v/%v root=%v", outerB, outerP, rootTok)
	}
	if len(innerTh) < 2 || innerTh[0] != innerTh[len(innerTh)-1] {
		t.Error("scoped component theme not cached across frames")
	}
}

// TestEnvRootComponentsStable: a root provider with component overrides
// themes the window and settles (no rebuild every frame).
func TestEnvRootComponentsStable(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Dark(true).Components(theme.Components{Button: theme.ButtonToken{PaddingInline: 30}}).Body(func() { Text(c, "x") })
	}, DriverOptions{Width: 100, Height: 100})
	d.Frame()
	d.Frame()
	c := d.Runner().Context()
	th := c.Theme
	d.Frame()
	if c.Theme != th || !th.Dark || th.Components.Button.PaddingInline != 30 {
		t.Fatalf("root window theme: stable=%v dark=%v pad=%v", c.Theme == th, th.Dark, th.Components.Button.PaddingInline)
	}
}

// TestScopedComponentsNoAlloc: an unchanged override set reuses the
// cached scoped theme.
func TestScopedComponentsNoAlloc(t *testing.T) {
	c := newContext(theme.AntDesign())
	p := ConfigProvider(c).Components(theme.Components{Button: theme.ButtonToken{PaddingInline: 30}})
	first := p.scopedComponents(ID(7), c.Theme)
	n := testing.AllocsPerRun(50, func() {
		if p.scopedComponents(ID(7), c.Theme) != first {
			t.Fatal("not cached")
		}
	})
	t.Logf("allocs per cached lookup: %v", n)
	// theme.Components.Hash allocates its hasher once per call (theme
	// package); the scoped theme itself is not rebuilt.
	if n > 1 {
		t.Errorf("cached lookup allocates %v", n)
	}
}

// TestEnvOverlayRootScoped: an overlay opened inside a nested provider
// paints its root (and children) with that provider's theme and direction.
func TestEnvOverlayRootScoped(t *testing.T) {
	var ovNode *node
	var kid probe
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Dark(true).Direction(RTL).Body(func() {
			b := Overlay(c, OverlayOptions{}, func() { addProbe(c, &kid) })
			ovNode = b.n
		})
	}, DriverOptions{Width: 200, Height: 200})
	d.Settle()
	if ovNode == nil || ovNode.env == nil || !ovNode.env.th.Dark || !ovNode.env.rtl {
		t.Fatalf("overlay root env not stamped: %+v", ovNode)
	}
	if !kid.paintDark || !kid.paintRTL {
		t.Errorf("overlay child: dark=%v rtl=%v", kid.paintDark, kid.paintRTL)
	}
}

// TestEnvRootProviderMirrorsWindow: a root provider's RTL direction mirrors
// the page's top-level nodes, not only nodes nested in a container.
func TestEnvRootProviderMirrorsWindow(t *testing.T) {
	var id ID
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			b := Row(c, func() {}, layout.Size(50, 20))
			id = b.ID()
		})
	}, DriverOptions{Width: 200, Height: 100})
	d.Settle()
	r := d.Runner().Context().rects[id]
	if r.X != 150 || r.W != 50 {
		t.Fatalf("top-level node in RTL root = %+v, want x=150 w=50", r)
	}
}
