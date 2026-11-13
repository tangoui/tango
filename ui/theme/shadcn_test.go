package theme

import (
	"math"
	"strings"
	"testing"
)

func relLum(c Color) float64 {
	ch := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

func contrast(a, b Color) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestShadcnDefaults(t *testing.T) {
	th := Shadcn()
	if th.Flavor != StyleShadcn || th.Dark || th.Shadcn == nil {
		t.Fatal("flavor")
	}
	for name, c := range map[string][2]string{
		"background": {HexString(th.Shadcn.Background), "#ffffff"},
		"foreground": {HexString(th.Shadcn.Foreground), "#0a0a0a"},
		"primary":    {HexString(th.Primary.Base), "#171717"},
		"secondary":  {HexString(th.SecondaryBg), "#f5f5f5"},
		"muted-fg":   {HexString(th.TextTertiary), "#737373"},
		"border":     {HexString(th.Border), "#e5e5e5"},
		"accent":     {HexString(th.ItemHoverBg), "#f5f5f5"},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %s, want %s", name, c[0], c[1])
		}
	}
	if th.RadiusLG != 8 || th.Radius != 6 || th.RadiusSM != 4 {
		t.Errorf("radius %v/%v/%v", th.RadiusLG, th.Radius, th.RadiusSM)
	}
	if th.ControlHeight != 36 || th.FontSize != 14 || th.LineHeightFor(14) != 20 || th.LineHeightFor(12) != 16 {
		t.Error("geometry")
	}
	if th.FocusRingWidth != 2 || th.FocusRingOffset != 2 {
		t.Error("ring + offset")
	}
}

func TestShadcnBasesAndContrast(t *testing.T) {
	for _, base := range ShadcnBases {
		for _, dark := range []bool{false, true} {
			th := Shadcn(ShadcnOptions{Base: base, Dark: dark})
			name := base
			if dark {
				name += "-dark"
			}
			v := th.Shadcn
			if v.Options.Base != base || th.Dark != dark {
				t.Errorf("%s: options not applied", name)
			}
			checks := []struct {
				what   string
				fg, bg Color
				min    float64
			}{
				{"foreground", v.Foreground, v.Background, 15},
				{"primary-foreground", v.PrimaryForeground, v.Primary, 12},
				{"muted-foreground", v.MutedForeground, v.Background, 4.5},
				{"muted-foreground on muted", v.MutedForeground, v.Muted, 3.5},
				{"secondary-foreground", v.SecondaryForeground, v.Secondary, 10},
				{"destructive text", v.Destructive, v.Background, 3},
				{"tooltip", th.TextOnSpotlight, th.BgSpotlight, 12},
				{"text on solid", th.TextOnSolid, th.Error.Base, 3},
			}
			for _, c := range checks {
				if got := contrast(c.fg, c.bg); got < c.min {
					t.Errorf("%s: %s contrast %.2f < %.1f", name, c.what, got, c.min)
				}
			}
			// Borders must be visible but quiet.
			if got := contrast(v.Border, v.Background); got < 1.1 || got > 1.8 {
				t.Errorf("%s: border contrast %.2f", name, got)
			}
		}
	}
	if a, b := Shadcn(ShadcnOptions{Base: "slate"}).Border, Shadcn(ShadcnOptions{Base: "stone"}).Border; a == b {
		t.Error("bases share a border color")
	}
	if Shadcn(ShadcnOptions{Base: "nope"}).Shadcn.Options.Base != "neutral" {
		t.Error("unknown base should fall back to neutral")
	}
}

func TestShadcnOptions(t *testing.T) {
	th := Shadcn(ShadcnOptions{Base: "zinc", Radius: 12, Primary: Hex("#2563eb")}, Dark())
	if !th.Dark || th.RadiusLG != 12 || th.Radius != 10 {
		t.Fatalf("dark=%v radius=%v", th.Dark, th.RadiusLG)
	}
	if HexString(th.Primary.Base) != "#2563eb" || th.Shadcn.Ring != Hex("#2563eb") {
		t.Error("custom primary / ring")
	}
	if contrast(th.TextOnPrimary, th.Primary.Base) < 4.5 {
		t.Error("primary foreground contrast")
	}
	th = Shadcn(WithPrimary(Hex("#facc15")))
	if th.TextOnPrimary == Hex("#ffffff") || Luma(th.TextOnPrimary) > 0.5 {
		t.Error("light primary should get a dark foreground")
	}
	if Shadcn(Compact()).ControlHeight != 32 {
		t.Error("compact")
	}
}

func TestShadcnCSSVars(t *testing.T) {
	v := Shadcn(ShadcnOptions{Base: "zinc"}).CSSVars()
	for k, w := range map[string]string{
		"--background":         "#ffffff",
		"--foreground":         "#09090b",
		"--primary":            "#18181b",
		"--primary-foreground": "#fafafa",
		"--muted-foreground":   "#71717a",
		"--border":             "#e4e4e7",
		"--ring":               "#a1a1aa",
		"--radius":             "0.5rem",
		"--chart-1":            "#e76e50",
		"--tango-radius":       "6px",
		"--tango-color-scheme": "light",
	} {
		if v[k] != w {
			t.Errorf("%s = %q, want %q", k, v[k], w)
		}
	}
	for _, k := range []string{"--card", "--popover-foreground", "--secondary", "--accent", "--destructive", "--input", "--chart-5", "--sidebar"} {
		if v[k] == "" {
			t.Errorf("missing %s", k)
		}
	}
	h := Shadcn(ShadcnOptions{Base: "zinc"}).Shadcn.Vars(true)
	if h["--primary"] != "240 5.9% 10%" || h["--background"] != "0 0% 100%" {
		t.Errorf("hsl primary=%q background=%q", h["--primary"], h["--background"])
	}
	if _, ok := AntDesign().CSSVars()["--background"]; ok {
		t.Error("Ant theme must not export shadcn names")
	}
	css := Shadcn(Dark()).CSS()
	if !strings.Contains(css, "--background: #0a0a0a;") || !strings.Contains(css, "--tango-color-scheme: dark;") {
		t.Error("dark css")
	}
}

func TestRecipeTokensKeepAntElement(t *testing.T) {
	a, e := AntDesign(), ElementPlus()
	if a.FocusRingWidth != 4 || a.FocusRingOffset != 1 || a.FocusOutline != a.FocusRing {
		t.Error("ant focus recipe")
	}
	// The Element ring starts from light-3 and is lifted to 3:1 contrast
	// (contrast.go); the recipe (width, offset, hue source) is unchanged.
	if e.FocusRingWidth != 2 || e.FocusRingOffset != 0 || e.FocusOutline != EnsureContrast(e.Primary.Shades[3], e.BgContainer, 3, e.Primary.Active, e.Text) {
		t.Error("element focus recipe")
	}
	if a.LineHeightFor(14) != 22 || e.LineHeightFor(14) != 22 {
		t.Error("ant/element leading changed")
	}
	if a.ButtonFontWeight != 400 || e.ButtonFontWeight != 500 {
		t.Error("button weight")
	}
	for _, th := range []*Theme{a, e, AntDesign(Dark()), ElementPlus(Dark()), Shadcn(), Shadcn(Dark())} {
		if th.TextOnSpotlight.A == 0 || th.SkeletonBg.A == 0 || th.AvatarBg.A == 0 || th.AddonBg.A == 0 || th.Illustration.Line.A == 0 {
			t.Errorf("%s dark=%v: recipe token missing", th.Name, th.Dark)
		}
	}
}
