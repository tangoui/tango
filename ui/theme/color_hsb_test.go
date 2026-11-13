package theme

import "testing"

func TestHSBText(t *testing.T) {
	c := rgba(51, 102, 204, 255) // hsb(220, 75%, 80%)
	if s := HSBString(c); s != "hsb(220, 75%, 80%)" {
		t.Fatalf("HSBString = %q", s)
	}
	if s := HSVString(rgba(51, 102, 204, 128)); s != "hsva(220, 75%, 80%, 0.5)" {
		t.Fatalf("HSVString = %q", s)
	}
	if s := HSBAString("hsb", HSVA{H: 200, S: 0, V: .5, A: 1}); s != "hsb(200, 0%, 50%)" {
		t.Fatalf("HSBAString keeps the hue of greys: %q", s)
	}
	for _, in := range []string{"hsb(220, 75%, 80%)", "HSV(220 75% 80%)", "hsb(220deg,75,80)", "hsba(220, 75%, 80%, 1)"} {
		got, err := ParseColor(in)
		if err != nil || got != c {
			t.Fatalf("ParseColor(%q) = %v, %v", in, got, err)
		}
	}
	h, err := ParseHSB("hsva(-30, 50%, 100% / 40%)")
	if err != nil || h.H != 330 || h.S != .5 || h.V != 1 || h.A != .4 {
		t.Fatalf("ParseHSB = %+v, %v", h, err)
	}
	for _, in := range []string{"hsb(1, 2)", "hsb(10, 120%, 5%)", "hsb(x, 1%, 1%)", "hsl(1, 1%, 1%)", "hsb(1, 1%, 1%"} {
		if _, err := ParseHSB(in); err == nil {
			t.Fatalf("ParseHSB(%q) should fail", in)
		}
	}
}
