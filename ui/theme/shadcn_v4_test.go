package theme

import "testing"

func mustOK(t *testing.T, s string) Color {
	t.Helper()
	c, err := ParseOKLCH(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestShadcnV4Neutral(t *testing.T) {
	th := Shadcn()
	v := th.Shadcn
	for name, c := range map[string][2]Color{
		"background":  {v.Background, Hex("#ffffff")},
		"foreground":  {v.Foreground, Hex("#0a0a0a")},
		"primary":     {v.Primary, Hex("#171717")},
		"border":      {v.Border, Hex("#e5e5e5")},
		"ring":        {v.Ring, Hex("#a1a1a1")},
		"destructive": {v.Destructive, Hex("#e7000b")},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %s, want %s", name, HexString(c[0]), HexString(c[1]))
		}
	}
	if th.RadiusLG != 10 || th.Radius != 8 || th.RadiusSM != 6 || th.CardRadius != 14 {
		t.Errorf("radius %v/%v/%v card %v", th.RadiusLG, th.Radius, th.RadiusSM, th.CardRadius)
	}
}

func TestShadcnV4DarkTranslucentBorder(t *testing.T) {
	th := Shadcn(Dark())
	v := th.Shadcn
	if v.Border.R != 255 || v.Border.G != 255 || v.Border.B != 255 || v.Border.A != 26 {
		t.Errorf("dark border = %v, want white at 10%%", v.Border)
	}
	if v.Input.A != 38 {
		t.Errorf("dark input alpha = %d, want 38", v.Input.A)
	}
	if want := Over(v.Input, v.Background); th.Border != want {
		t.Errorf("th.Border = %s, want input over background %s", HexString(th.Border), HexString(want))
	}
	if th.Border.A != 255 || th.BorderSecondary.A != 255 || th.CardBorder.A != 255 {
		t.Error("theme border tokens must be opaque")
	}
	if v.SidebarPrimary != Hex("#1447e6") {
		t.Errorf("dark sidebar-primary = %s, want #1447e6", HexString(v.SidebarPrimary))
	}
}

func TestShadcnV4Bases(t *testing.T) {
	for _, b := range []string{"stone", "zinc", "mauve", "olive", "mist", "taupe"} {
		for _, dark := range []bool{false, true} {
			vars := shadcnThemeVars[b].light
			if dark {
				vars = shadcnThemeVars[b].dark
			}
			got := Shadcn(ShadcnOptions{Base: b, Dark: dark}).Shadcn
			if got.Primary != mustOK(t, vars["primary"]) || got.Background != mustOK(t, vars["background"]) {
				t.Errorf("%s dark=%v: primary %s background %s", b, dark, HexString(got.Primary), HexString(got.Background))
			}
		}
	}
}

func TestShadcnV4Color(t *testing.T) {
	v := Shadcn(ShadcnOptions{Color: "amber"}).Shadcn
	if v.Primary != Hex("#bb4d00") || v.Background != Hex("#ffffff") {
		t.Errorf("amber light: primary %s background %s", HexString(v.Primary), HexString(v.Background))
	}
	if v.Chart[0] != mustOK(t, shadcnThemeVars["amber"].light["chart-1"]) {
		t.Error("amber chart-1 not applied")
	}
	d := Shadcn(ShadcnOptions{Color: "amber"}, Dark()).Shadcn
	if d.Primary != mustOK(t, shadcnThemeVars["amber"].dark["primary"]) {
		t.Errorf("amber dark primary = %s", HexString(d.Primary))
	}
	if n := Shadcn(ShadcnOptions{Color: "nope"}).Shadcn; n.Primary != Hex("#171717") {
		t.Error("an unknown color theme must be ignored")
	}
}

func TestShadcnPrimaryOverridesColor(t *testing.T) {
	v := Shadcn(ShadcnOptions{Color: "amber", Primary: Hex("#2563eb")}).Shadcn
	if v.Primary != Hex("#2563eb") || v.Ring != Hex("#2563eb") {
		t.Errorf("primary %s ring %s", HexString(v.Primary), HexString(v.Ring))
	}
}

// TestShadcnLegacyBases: slate and gray left shadcn/ui in v4; they keep the
// v3 colors they had before the v4 switch (values recorded from that code).
func TestShadcnLegacyBases(t *testing.T) {
	for _, c := range []struct {
		base                                string
		dark                                bool
		bg, fg, primary, border, muted, mfg string
	}{
		{"slate", false, "#ffffff", "#020617", "#0f172a", "#e2e8f0", "#f1f5f9", "#64748b"},
		{"slate", true, "#020617", "#f8fafc", "#f8fafc", "#1e293b", "#1e293b", "#94a3b8"},
		{"gray", false, "#ffffff", "#030712", "#111827", "#e5e7eb", "#f3f4f6", "#6b7280"},
		{"gray", true, "#030712", "#f9fafb", "#f9fafb", "#1f2937", "#1f2937", "#9ca3af"},
	} {
		v := Shadcn(ShadcnOptions{Base: c.base, Radius: 8, Dark: c.dark}).Shadcn
		got := [6]string{HexString(v.Background), HexString(v.Foreground), HexString(v.Primary), HexString(v.Border), HexString(v.Muted), HexString(v.MutedForeground)}
		want := [6]string{c.bg, c.fg, c.primary, c.border, c.muted, c.mfg}
		if got != want {
			t.Errorf("%s dark=%v = %v, want %v", c.base, c.dark, got, want)
		}
	}
	if Shadcn(ShadcnOptions{Base: "nope"}).Shadcn.Options.Base != "neutral" {
		t.Error("unknown base should fall back to neutral")
	}
}

func TestShadcnCSSVarsV4(t *testing.T) {
	if r := Shadcn().CSSVars()["--radius"]; r != "0.625rem" {
		t.Errorf("--radius = %q, want 0.625rem", r)
	}
}
