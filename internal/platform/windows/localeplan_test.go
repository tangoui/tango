package windows

import (
	"slices"
	"testing"
	"unicode/utf16"
)

func TestSplitMultiSZ(t *testing.T) {
	buf := utf16.Encode([]rune("zh-CN\x00en-US\x00\x00"))
	if got := splitMultiSZ(buf); !slices.Equal(got, []string{"zh-CN", "en-US"}) {
		t.Errorf("got %v", got)
	}
	if got := splitMultiSZ([]uint16{0, 0}); len(got) != 0 {
		t.Errorf("empty: %v", got)
	}
	if got := splitMultiSZ(nil); len(got) != 0 {
		t.Errorf("nil: %v", got)
	}
}
