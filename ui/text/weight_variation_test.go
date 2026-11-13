package text_test

import (
	"testing"

	"github.com/tangoui/tango/ui/text"
)

// Weights of one variable font (SF Pro on macOS, ...) must measure
// differently: each weight needs its own *font.Font because go-text's
// HarfbuzzShaper caches shaping fonts per *font.Font. Regression test for
// all weights shaping with the first-shaped weight's coordinates.
func TestVariableWeightsMeasureDifferently(t *testing.T) {
	for _, eng := range []bool{false, true} {
		prev := text.UseSystemEngine(eng)
		light := text.Shape("Hello World", text.Style{Size: 16, Weight: text.Light}, text.Options{}).Width
		regular := text.Shape("Hello World", text.Style{Size: 16, Weight: text.Regular}, text.Options{}).Width
		bold := text.Shape("Hello World", text.Style{Size: 16, Weight: text.Bold}, text.Options{}).Width
		text.UseSystemEngine(prev)
		if !(light < regular && regular < bold) {
			t.Errorf("system engine %v: widths light=%.2f regular=%.2f bold=%.2f, want strictly increasing (the default sans is variable on macOS; elsewhere skip if the font has a single weight)", eng, light, regular, bold)
		}
	}
}
