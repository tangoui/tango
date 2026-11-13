package theme

import (
	"strings"
	"testing"
)

func TestParseHexForms(t *testing.T) {
	cases := map[string]Color{
		"#f50":      rgba(0xff, 0x55, 0x00, 0xff),
		"f508":      rgba(0xff, 0x55, 0x00, 0x88),
		" #1677FF ": rgba(0x16, 0x77, 0xff, 0xff),
		"#1677ff80": rgba(0x16, 0x77, 0xff, 0x80),
	}
	for in, want := range cases {
		got, err := ParseHex(in)
		if err != nil || got != want {
			t.Errorf("ParseHex(%q) = %v, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "#12", "#12345", "#ggg", "rgb(1,2,3)"} {
		if _, err := ParseHex(in); err == nil {
			t.Errorf("ParseHex(%q) should fail", in)
		}
	}
	if HexString(rgba(0x16, 0x77, 0xff, 0xff)) != "#1677ff" || HexString(rgba(1, 2, 3, 4)) != "#01020304" {
		t.Fatal("HexString")
	}
	if strings.ToUpper(HexString(rgba(0x16, 0x77, 0xff, 0x80))) != "#1677FF80" {
		t.Fatal("upper hex")
	}
}

func TestRGBStrings(t *testing.T) {
	if s := RGBString(rgba(22, 119, 255, 255)); s != "rgb(22, 119, 255)" {
		t.Fatal(s)
	}
	if s := RGBString(rgba(22, 119, 255, 128)); s != "rgba(22, 119, 255, 0.5)" {
		t.Fatal(s)
	}
	cases := map[string]Color{
		"rgb(22, 119, 255)":       rgba(22, 119, 255, 255),
		"RGBA(22,119,255,0.5)":    rgba(22, 119, 255, 128),
		"rgb(22 119 255 / 50%)":   rgba(22, 119, 255, 128),
		"rgba(0, 0, 0, 2)":        rgba(0, 0, 0, 255),
		" rgba(1, 2, 3, 0) ":      rgba(1, 2, 3, 0),
		"rgba(255, 255, 255, 1)":  rgba(255, 255, 255, 255),
		"rgba(10, 20, 30, 0.04)":  rgba(10, 20, 30, 10),
		"rgb(0, 0, 0)":            rgba(0, 0, 0, 255),
		"rgba(255,0,0,100%)":      rgba(255, 0, 0, 255),
		"rgb( 1 , 2 , 3 )":        rgba(1, 2, 3, 255),
		"rgba(12, 34, 56, 0.25 )": rgba(12, 34, 56, 64),
	}
	for in, want := range cases {
		got, err := ParseRGB(in)
		if err != nil || got != want {
			t.Errorf("ParseRGB(%q) = %v, %v", in, got, err)
		}
	}
	for _, in := range []string{"rgb(256,0,0)", "rgb(1,2)", "rgb(a,b,c)", "#fff", "rgba(1,2,3,x)"} {
		if _, err := ParseRGB(in); err == nil {
			t.Errorf("ParseRGB(%q) should fail", in)
		}
	}
	for in, want := range map[string]Color{"#fff": rgba(255, 255, 255, 255), "rgb(1,2,3)": rgba(1, 2, 3, 255), "transparent": {}, "00ff0080": rgba(0, 255, 0, 128)} {
		if got, err := ParseColor(in); err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseColor("nope"); err == nil {
		t.Fatal("ParseColor should fail")
	}
}

func TestHSVA(t *testing.T) {
	for _, c := range []Color{rgba(22, 119, 255, 255), rgba(255, 0, 0, 128), rgba(0, 0, 0, 0), rgba(250, 173, 20, 77), rgba(128, 128, 128, 255)} {
		h := ToHSVA(c)
		if got := FromHSVA(h); got != c {
			t.Errorf("round trip %v -> %+v -> %v", c, h, got)
		}
	}
	if h := ToHSVA(rgba(255, 0, 0, 255)); h.H != 0 || h.S != 1 || h.V != 1 || h.A != 1 {
		t.Fatalf("red %+v", h)
	}
	if c := FromHSVA(HSVA{H: 120, S: 1, V: 1, A: 2}); c != (rgba(0, 255, 0, 255)) {
		t.Fatalf("clamped alpha %v", c)
	}
	if Luma(rgba(255, 255, 255, 255)) != 1 || Luma(rgba(0, 0, 0, 0)) != 0 {
		t.Fatal("luma")
	}
}

func TestSegmentedTokens(t *testing.T) {
	a := AntDesign()
	if HexString(a.SegmentedBg) != "#f5f5f5" || a.SegmentedSelectedBg != a.BgElevated {
		t.Fatalf("ant light %s", HexString(a.SegmentedBg))
	}
	ad := AntDesign(Dark())
	if ad.SegmentedBg != ad.BgLayout || ad.SegmentedSelectedBg != ad.BgElevated {
		t.Fatal("ant dark")
	}
	for _, e := range []*Theme{ElementPlus(), ElementPlus(Dark())} {
		if e.SegmentedBg != e.FillTertiary || e.SegmentedSelectedBg != e.Primary.Base {
			t.Fatal("element")
		}
	}
}

func rgba(r, g, b, a uint8) Color { return Color{R: r, G: g, B: b, A: a} }
