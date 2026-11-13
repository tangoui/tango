package windows

import "testing"

func TestAccentConversions(t *testing.T) {
	if got := accentFromDWM(0xff_d7_78_00); got != 0x0078d7ff {
		t.Fatalf("%#x", got)
	}
	if got := accentFromDWM(0x00_d7_78_00); got != 0x0078d7ff {
		t.Fatalf("zero alpha: %#x", got)
	}
	if got := accentFromColorization(0xc4_00_78_d7); got != 0x0078d7ff {
		t.Fatalf("%#x", got)
	}
}

func TestBadgeText(t *testing.T) {
	for in, want := range map[string]string{"3": "3", "42": "42", "123": "9+", "9+": "9+", "12+": "9+", "": ""} {
		if got := badgeText(in); got != want {
			t.Errorf("badgeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBadgePixels(t *testing.T) {
	const size = 16
	pix := badgePixels("3", size)
	at := func(x, y int) []byte { i := (y*size + x) * 4; return pix[i : i+4] }
	if a := at(0, 0)[3]; a != 0 {
		t.Fatalf("corner alpha %d", a)
	}
	white, red := 0, 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			p := at(x, y)
			switch {
			case p[0] == 0xff && p[1] == 0xff && p[2] == 0xff:
				white++
			case p[2] == 0xe8 && p[3] == 0xff:
				red++
			}
		}
	}
	// "3" at scale 2: 11 lit cells of 2×2 px.
	if white != 11*4 || red < 100 {
		t.Fatalf("white %d red %d", white, red)
	}
	if n := len(badgePixels("99", 32)); n != 32*32*4 {
		t.Fatal(n)
	}
}
