package theme

import "testing"

func TestParseOKLCH(t *testing.T) {
	cases := []struct{ in, want string }{
		{"oklch(1 0 0)", "#ffffff"},
		{"oklch(0.145 0 0)", "#0a0a0a"},
		{"oklch(0.205 0 0)", "#171717"},
		{"oklch(0.985 0 0)", "#fafafa"},
		{"oklch(0.922 0 0)", "#e5e5e5"},
		{"oklch(0.708 0 0)", "#a1a1a1"},
		{"oklch(0.577 0.245 27.325)", "#e7000b"},
		{"oklch(0.488 0.243 264.376)", "#1447e6"},
		{"oklch(0.555 0.163 48.998)", "#bb4d00"},
		{"oklch(98.5% 0 none)", "#fafafa"}, // Tailwind v4 theme.css form
	}
	for _, c := range cases {
		got, err := ParseOKLCH(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if HexString(got) != c.want || got.A != 255 {
			t.Errorf("%s = %s a=%d, want %s", c.in, HexString(got), got.A, c.want)
		}
	}
}

func TestParseOKLCHAlpha(t *testing.T) {
	c, err := ParseOKLCH("oklch(1 0 0 / 10%)")
	if err != nil || c.R != 255 || c.G != 255 || c.B != 255 || c.A != 26 {
		t.Errorf("10%%: %v %v", c, err)
	}
	c, err = ParseOKLCH("oklch(1 0 0 / 0.15)")
	if err != nil || c.A != 38 {
		t.Errorf("0.15: %v %v", c, err)
	}
}

func TestParseOKLCHClamp(t *testing.T) {
	if _, err := ParseOKLCH("oklch(0.7 0.4 145)"); err != nil {
		t.Fatal(err)
	}
	// Linear sRGB is about (-0.25, 0.64, -0.12): red and blue are below
	// the gamut and must clamp to 0, not wrap around.
	if c := OKLCH(0.7, 0.4, 145, 1); c.R != 0 || c.B != 0 || c.G != 210 {
		t.Errorf("out-of-gamut channels should clamp to 0, got %v", c)
	}
	// Lightness above 1 is above the gamut on every channel.
	if c := OKLCH(1.2, 0, 0, 1); c != Hex("#ffffff") {
		t.Errorf("l>1 should clamp to white, got %v", c)
	}
}

func TestParseOKLCHErrors(t *testing.T) {
	for _, s := range []string{"oklch(1 0)", "oklch(a b c)", "oklch(1 0 0", "rgb(1 0 0)"} {
		if _, err := ParseOKLCH(s); err == nil {
			t.Errorf("%q: expected an error", s)
		}
	}
}

func TestParseColorOKLCH(t *testing.T) {
	c, err := ParseColor("oklch(0.205 0 0)")
	if err != nil || c != Hex("#171717") {
		t.Errorf("ParseColor = %v %v", c, err)
	}
}
