//go:build darwin

package text

import (
	"os"
	"testing"
)

func TestCoreTextLightweightShapeOptIn(t *testing.T) {
	old := os.Getenv("TANGO_TEXT_ENGINE")
	t.Setenv("TANGO_TEXT_ENGINE", "coretext")
	defer os.Setenv("TANGO_TEXT_ENGINE", old)
	l := coreTextShape("重置", Style{Size: 13}, Options{NoWrap: true})
	if l == nil || len(l.Glyphs) == 0 || l.Width <= 0 || l.Height <= 0 {
		t.Fatalf("invalid CoreText layout: %+v", l)
	}
	for _, g := range l.Glyphs {
		if g.Src == nil {
			t.Fatal("nil CoreText glyph source")
		}
	}
}

func TestCoreTextLightweightRejectsWrappedText(t *testing.T) {
	t.Setenv("TANGO_TEXT_ENGINE", "coretext")
	if got := coreTextShape("long paragraph", Style{Size: 13}, Options{MaxWidth: 20}); got != nil {
		t.Fatal("wrapped CoreText path must fall back")
	}
}
