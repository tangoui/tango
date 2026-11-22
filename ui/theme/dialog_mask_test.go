package theme

import "testing"

func TestDialogMask(t *testing.T) {
	plain := ElementPlus(Dark())
	want := Hex("#00000080")
	th := ElementPlus(Dark(), WithComponents(DialogMask(want)))
	if th.Components.Modal.MaskBg != want || th.Components.Drawer.MaskBg != want || th.BgMask != plain.BgMask {
		t.Fatalf("mask tokens: modal %v, drawer %v, global %v", th.Components.Modal.MaskBg, th.Components.Drawer.MaskBg, th.BgMask)
	}
	clear := th.Override(DialogMask(Color{}))
	if clear.Components.Modal.MaskBg.A != 0 || clear.Components.Drawer.MaskBg.A != 0 {
		t.Fatal("transparent mask inherited the default")
	}
	if th.Components.Modal.MaskBg != want || plain.Components.Modal.MaskBg != Hex("#000000cc") {
		t.Fatal("theme override modified the original theme")
	}
}
