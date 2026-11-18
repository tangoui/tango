//go:build darwin

package text

import "testing"

func TestCoreTextLightweightRejectsWrappedText(t *testing.T) {
	t.Setenv("TANGO_TEXT_ENGINE", "coretext")
	if got := coreTextShape("long paragraph", Style{Size: 13}, Options{MaxWidth: 20}); got != nil {
		t.Fatal("wrapped CoreText path must fall back")
	}
}
