package theme

import "testing"

func TestTextOn(t *testing.T) {
	white := Hex("#ffffff")
	light, dark := AntDesign(), Shadcn(Dark())
	if got := light.TextOn(light.Primary.Base, white); got != white {
		t.Errorf("antd primary: %v, want white", got)
	}
	if got := light.TextOn(Hex("#fafafa"), white); got != light.Text {
		t.Errorf("light fill in a light theme: %v, want Text %v", got, light.Text)
	}
	// shadcn dark: primary is near white and Text is light, so marks on the
	// primary use the dark container color.
	if got := dark.TextOn(dark.Primary.Base, white); got != dark.BgContainer {
		t.Errorf("shadcn dark primary: %v, want BgContainer %v", got, dark.BgContainer)
	}
	if Luma(dark.TextOn(dark.Primary.Base, white)) > 0.5 {
		t.Error("mark on shadcn dark primary is not dark")
	}
}
