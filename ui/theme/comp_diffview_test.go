package theme

import "testing"

// TestDiffViewContrast: in every built-in theme, light and dark, code in
// the default text color stays readable on the diff backgrounds (4.5:1 on
// the changed lines and on the changed characters over them), the
// tints stand out (the lines from the page, the characters from their
// line), the
// placeholder bar's texts pass their tagged pairs, and the overview marks
// are visible on the page (3:1 against the ruler's container would be
// stricter than Monaco's translucent marks; 1.5:1 keeps them apart).
func TestDiffViewContrast(t *testing.T) {
	for _, th := range BuiltinThemes() {
		name := themeLabel(th)
		k := th.For(CompDiffView).Components.DiffView
		page := th.BgContainer
		for _, p := range []struct {
			what       string
			line, text Color
		}{{"inserted", k.InsertedLineBg, k.InsertedTextBg}, {"removed", k.RemovedLineBg, k.RemovedTextBg}} {
			line := Over(p.line, page)
			chars := Over(p.text, line)
			if r := ContrastRatio(th.Text, line); r < 4.5 {
				t.Errorf("%s: text on the %s lines %.2f:1", name, p.what, r)
			}
			if r := ContrastRatio(th.Text, chars); r < 4.5 {
				t.Errorf("%s: text on the %s characters %.2f:1", name, p.what, r)
			}
			if r := ContrastRatio(chars, line); r < 1.15 {
				t.Errorf("%s: %s characters on their line %.2f:1", name, p.what, r)
			}
			if r := ContrastRatio(line, page); r < 1.03 {
				t.Errorf("%s: %s lines on the page %.2f:1", name, p.what, r)
			}
		}
		for _, c := range []Color{k.OverviewInserted, k.OverviewRemoved} {
			if r := ContrastRatio(c, page); r < 1.5 {
				t.Errorf("%s: overview mark %v on the page %.2f:1", name, c, r)
			}
		}
		// The tagged pairs (CheckContrast reports component pairs only
		// for overrides).
		bar := Over(k.RegionBg, page)
		for _, p := range []struct {
			what string
			c    Color
		}{{"RegionColor", k.RegionColor}, {"LinkColor", k.LinkColor}, {"LinkHoverColor", k.LinkHoverColor}} {
			if r := ContrastRatio(p.c, bar); r < 4.5 {
				t.Errorf("%s: %s on RegionBg %.2f:1", name, p.what, r)
			}
		}
	}
}
