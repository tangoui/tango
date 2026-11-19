//go:build darwin && (ios || tango_gotext)

package text

import (
	"os"
	"testing"
)

// go-text build only (-tags tango_gotext): the opt-in short-text CoreText
// path (coreTextShape with TANGO_TEXT_ENGINE=coretext) exists only there;
// the default macOS build shapes everything with CoreText (shapeItems) and its coreTextShape
// always declines.

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
