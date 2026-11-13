package theme

import "testing"

func TestAntPrimaryPalette(t *testing.T) {
	want := []string{"#e6f4ff", "#bae0ff", "#91caff", "#69b1ff", "#4096ff", "#1677ff", "#0958d9", "#003eb3", "#002c8c", "#001d66"}
	got := AntShades(Hex("#1677ff"), false, Color{})
	for i, w := range want {
		if HexString(got[i]) != w {
			t.Errorf("shade %d = %s, want %s", i+1, HexString(got[i]), w)
		}
	}
}

func TestAntSemanticTokens(t *testing.T) {
	th := AntDesign()
	checks := map[string][2]string{
		"error hover":   {HexString(th.Error.Hover), "#ff7875"},
		"error active":  {HexString(th.Error.Active), "#d9363e"},
		"success":       {HexString(th.Success.Base), "#52c41a"},
		"primary bg":    {HexString(th.Primary.Bg), "#e6f4ff"},
		"warning hover": {HexString(th.Warning.Hover), "#ffc53d"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %s, want %s", name, c[0], c[1])
		}
	}
	if th.ControlHeight != 32 || th.Radius != 6 || th.FontSize != 14 {
		t.Fatal("geometry tokens")
	}
}

func TestAntDarkPrimary(t *testing.T) {
	th := AntDesign(Dark())
	if got := HexString(th.Primary.Base); got != "#1668dc" {
		t.Errorf("dark primary = %s, want #1668dc", got)
	}
	if !th.Dark || th.BgContainer != Hex("#141414") {
		t.Error("dark neutrals")
	}
}

func TestElementPalette(t *testing.T) {
	th := ElementPlus()
	for name, c := range map[string][2]string{
		"light-3": {HexString(th.Primary.Hover), "#79bbff"},
		"dark-2":  {HexString(th.Primary.Active), "#337ecc"},
		"light-9": {HexString(th.Primary.Bg), "#ecf5ff"},
		"light-5": {HexString(th.Primary.Border), "#a0cfff"},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %s, want %s", name, c[0], c[1])
		}
	}
	if th.Radius != 4 || th.Flavor != Element {
		t.Error("element geometry")
	}
}

func TestWithPrimaryAndHelpers(t *testing.T) {
	th := AntDesign(WithPrimary(Hex("#722ed1")))
	if HexString(th.Primary.Base) != "#722ed1" || HexString(th.Primary.Bg) != "#f9f0ff" {
		t.Errorf("purple palette base=%s bg=%s", HexString(th.Primary.Base), HexString(th.Primary.Bg))
	}
	if HexString(Mix(Hex("#000000"), Hex("#ffffff"), 0.5)) != "#808080" {
		t.Error("mix")
	}
	if Lerp(Hex("#000000"), Hex("#ffffff"), 1) != Hex("#ffffff") {
		t.Error("lerp end")
	}
	if _, err := ParseHex("#12"); err == nil {
		t.Error("bad hex accepted")
	}
}
