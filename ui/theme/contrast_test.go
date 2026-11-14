package theme

import (
	"math"
	"strings"
	"testing"
)

func TestContrastRatio(t *testing.T) {
	black, white := Hex("#000000"), Hex("#ffffff")
	if r := ContrastRatio(black, white); math.Abs(r-21) > 0.01 {
		t.Errorf("black on white %.2f", r)
	}
	if r := ContrastRatio(white, white); math.Abs(r-1) > 1e-9 {
		t.Errorf("white on white %.2f", r)
	}
	// #767676 is the classic lightest gray passing 4.5:1 on white.
	if r := ContrastRatio(Hex("#767676"), white); r < 4.5 || r > 4.6 {
		t.Errorf("#767676 %.3f", r)
	}
	// Translucent text is composited first: 45% black on white = #8c8c8c.
	if a, b := ContrastRatio(RGBA(0, 0, 0, .45), white), ContrastRatio(Hex("#8c8c8c"), white); math.Abs(a-b) > 0.02 {
		t.Errorf("alpha compositing %.2f vs %.2f", a, b)
	}
	if ContrastRatio(white, black) != ContrastRatio(black, white) {
		t.Error("ratio must be symmetric")
	}
}

func TestEnsureContrast(t *testing.T) {
	white := Hex("#ffffff")
	lift := EnsureContrast(Hex("#91caff"), white, 3, Hex("#1677ff"))
	if r := ContrastRatio(lift, white); r < 3 {
		t.Errorf("lifted ring %.2f", r)
	}
	if got := EnsureContrast(Hex("#000000"), white, 3); got != Hex("#000000") {
		t.Error("passing color changed")
	}
}

// TestBuiltinThemeContrast checks the six built-in themes: no required
// pair may fail; the design-system defaults below their minimum (placeholder,
// borders …) are reported as DesignDefault with the HighContrast value, not
// hidden; body text and keyboard focus indicators always pass.
func TestBuiltinThemeContrast(t *testing.T) {
	mustPass := map[string]bool{"Text": true, "TextSecondary": true, "TextHeading": true, "FocusOutline": true, "FocusRing": true, "ControlFocusBorder": true, "TextOnSpotlight": true, "MarkText": true}
	var n [4]int
	for _, th := range BuiltinThemes() {
		checks := CheckContrast(th)
		if len(checks) < 20 {
			t.Fatalf("%s: only %d checks", themeLabel(th), len(checks))
		}
		for _, c := range checks {
			n[c.Status]++
			if c.Ratio < 1 || c.Ratio > 21.001 {
				t.Errorf("%s: ratio out of range", c)
			}
			switch c.Status {
			case ContrastStatusFail:
				t.Errorf("required pair fails: %s", c)
			case ContrastStatusDesignDefault:
				if mustPass[c.Token] || c.Spec == "" {
					t.Errorf("not eligible as design default: %s", c)
				}
				if c.SuggestRatio+1e-9 < c.Kind.Min() {
					t.Errorf("suggestion below minimum: %s", c)
				}
			case ContrastStatusExempt:
				if c.Kind != ContrastExempt {
					t.Errorf("exempt status on a required pair: %s", c)
				}
			}
			if (c.Status == ContrastStatusPass) != (c.Kind != ContrastExempt && c.Pass()) {
				t.Errorf("status %s inconsistent with ratio: %s", c.Status, c)
			}
		}
		if len(ContrastFailures(th)) != 0 {
			t.Errorf("%s: ContrastFailures not empty", themeLabel(th))
		}
	}
	// The 35 design defaults left of the 37 pairs the stock themes missed
	// (the two input focus borders are lifted, see designSpec), plus the 23
	// of the data-entry / switch thumb / rate pairs (see designSpec), plus
	// the 50 xterm.js ANSI colors below 4.5:1 on the terminal background
	// (TerminalToken.Palette: 11 in each light theme, 6 in antd-dark and
	// element-plus-dark, 5 in shadcn-dark), minus the 3 light shadcn danger
	// pairs shadcn/ui v4's --destructive (#e7000b, 4.5:1+) now passes.
	if n[ContrastStatusDesignDefault] != 105 {
		t.Errorf("%d design defaults, want 105", n[ContrastStatusDesignDefault])
	}
	rep := ContrastReport(BuiltinThemes()...)
	if strings.Count(rep, "\nDesignDefault ") != n[ContrastStatusDesignDefault] || strings.Contains(rep, "\nFAIL") {
		t.Errorf("report does not match the checks")
	}
	t.Logf("%d fail, %d design-default, %d exempt, %d ok:\n%s", n[ContrastStatusFail], n[ContrastStatusDesignDefault], n[ContrastStatusExempt], n[ContrastStatusPass], rep)
}

// TestDesignDefaultNeedsStockColors: a brand override is the app's choice,
// so its shortfalls fail instead of passing as design defaults.
func TestDesignDefaultNeedsStockColors(t *testing.T) {
	th := AntDesign(WithPrimary(Hex("#ffcc00")))
	var fails []string
	for _, c := range ContrastFailures(th) {
		fails = append(fails, c.Token)
	}
	got := strings.Join(fails, ",")
	if !strings.Contains(got, "TextOnPrimary") || !strings.Contains(got, "TextLink") {
		t.Errorf("yellow brand failures = %s", got)
	}
	for _, c := range ContrastFailures(th) {
		if c.SuggestRatio+1e-9 < c.Kind.Min() {
			t.Errorf("suggestion below minimum: %s", c)
		}
	}
}

// TestHighContrast: every required pair passes in the high-contrast variant
// of each built-in theme (and of brand / base-color variants), exempt pairs
// are untouched, and the style tokens are kept.
func TestHighContrast(t *testing.T) {
	themes := append(BuiltinThemes(),
		AntDesign(WithPrimary(Hex("#ffcc00"))), AntDesign(WithPrimary(Hex("#13c2c2")), Dark()),
		ElementPlus(WithPrimary(Hex("#67c23a"))), Shadcn(ShadcnOptions{Primary: Hex("#2563eb")}),
		Shadcn(ShadcnOptions{Base: "slate"}, Dark()), Shadcn(ShadcnOptions{Base: "stone", Primary: Hex("#f97316")}, Dark()))
	for _, th := range themes {
		hc := HighContrast(th)
		if !hc.HighContrast || th.HighContrast || hc.Flavor != th.Flavor || hc.Dark != th.Dark {
			t.Fatalf("%s: flags", themeLabel(th))
		}
		for _, c := range CheckContrast(hc) {
			if c.Status == ContrastStatusFail || c.Status == ContrastStatusDesignDefault {
				t.Errorf("high contrast: %s", c)
			}
		}
		if hc.Radius != th.Radius || hc.ControlHeight != th.ControlHeight || hc.FontSize != th.FontSize || hc.BgContainer != th.BgContainer ||
			hc.BorderSecondary != th.BorderSecondary || hc.TextDisabled != th.TextDisabled || len(hc.Shadow) != len(th.Shadow) {
			t.Errorf("%s: high contrast changed style or exempt tokens", themeLabel(th))
		}
		// Hue is kept: the brand color stays on the same side of the wheel.
		if a, b := ToHSV(th.Primary.Base), ToHSV(hc.Primary.Base); th.Primary.Base != hc.Primary.Base && ToHSV(th.Primary.Base).S > .2 {
			d := math.Abs(a.H - b.H)
			if d > 180 {
				d = 360 - d
			}
			if d > 8 {
				t.Errorf("%s: primary hue %.0f → %.0f", themeLabel(th), a.H, b.H)
			}
		}
		if again := HighContrast(hc); len(ContrastIssues(again)) != 0 || again.Primary.Base != hc.Primary.Base || again.Border != hc.Border {
			t.Errorf("%s: HighContrast is not idempotent", themeLabel(th))
		}
		if th.Shadcn != nil && (hc.Shadcn == th.Shadcn || hc.Shadcn.Input != hc.Border || hc.Shadcn.Destructive != hc.Error.Base) {
			t.Errorf("%s: shadcn variables not synced", themeLabel(th))
		}
	}
	// The source theme is not modified.
	th := ElementPlus()
	HighContrast(th)
	if th.Primary.Base != Hex("#409eff") || th.Border != Hex("#dcdfe6") || th.HighContrast {
		t.Error("HighContrast modified its argument")
	}
	if HighContrast(nil) != nil {
		t.Error("nil theme")
	}
}
