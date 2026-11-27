package windows

import "testing"

func TestOverlayExStyle(t *testing.T) {
	const want = 0x08000000 | 0x8 | 0x80
	if style := overlayExStyle; style&want != want || style&0x40000 != 0 {
		t.Fatalf("extended overlay style = %#x", style)
	}
}

func TestOverlayShowCmd(t *testing.T) {
	if cmd := overlayShowCmd; cmd != 4 {
		t.Fatalf("overlay show command = %d", cmd)
	}
}

func TestOverlayMouseActivate(t *testing.T) {
	if result := overlayMouseActivate; result != 3 {
		t.Fatalf("overlay mouse activate result = %d", result)
	}
}
