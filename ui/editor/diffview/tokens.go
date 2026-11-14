package diffview

import (
	"image/color"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	uitheme "github.com/tangoui/tango/ui/theme"
)

// tokensOf returns the design theme's diff colors (theme.CompDiffView)
// for an editor drawn with th, or nil when they do not fit it: an editor
// theme that is dark while the design theme is light (or the other way
// round), or a high-contrast editor theme (whose Monaco defaults are
// borders, not tints), keeps the editor theme's colors.
func tokensOf(c *ui.Context, th *theme.Theme) *tokenColors {
	if c.Theme == nil || th == nil || strings.HasPrefix(th.BaseName(), "hc") {
		return nil
	}
	ut := c.ThemeFor(uitheme.CompDiffView)
	bg, ok := th.Color("editor.background")
	if !ok || dark(bg) != dark(ut.BgContainer) {
		return nil
	}
	k := &ut.Components.DiffView
	return &tokenColors{
		insertedLine: k.InsertedLineBg, insertedText: k.InsertedTextBg,
		removedLine: k.RemovedLineBg, removedText: k.RemovedTextBg,
		overviewInserted: k.OverviewInserted, overviewRemoved: k.OverviewRemoved,
		diagonal: k.DiagonalFill, regionBg: k.RegionBg, regionFg: k.RegionColor,
		link: k.LinkColor, linkHover: k.LinkHoverColor,
	}
}

// dark reports whether c is a dark background.
func dark(c color.NRGBA) bool {
	return 0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B) < 128
}
