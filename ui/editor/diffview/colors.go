package diffview

import (
	"image/color"

	"github.com/tangoui/tango/ui/editor/theme"
)

// colors are the diff view's colors (docs/specs/diff-view.md, 3.11): the
// editor theme's own value of each Monaco key first (a VS Code theme the
// user installed), then the design theme's CompDiffView token, then
// Monaco's default for the key.
type colors struct {
	insertedLine, insertedText, removedLine, removedText color.NRGBA
	insertedBorder, removedBorder                        color.NRGBA
	gutterInserted, gutterRemoved                        color.NRGBA
	overviewInserted, overviewRemoved                    color.NRGBA
	diagonal                                             color.NRGBA
	regionBg, regionFg, regionShadow                     color.NRGBA
	sign, lineNumber, link, linkHover                    color.NRGBA
	slider, sliderHover, revert, revertHover, border     color.NRGBA
	background                                           color.NRGBA // editor.background
	hash                                                 uint64
}

// tokenColors are the design theme's colors for the keys (zero: none).
type tokenColors struct {
	insertedLine, insertedText, removedLine, removedText color.NRGBA
	overviewInserted, overviewRemoved                    color.NRGBA
	diagonal, regionBg, regionFg, link, linkHover        color.NRGBA
}

func resolveColors(th *theme.Theme, tk *tokenColors) colors {
	if tk == nil {
		tk = &tokenColors{}
	}
	pick := func(key string, tok color.NRGBA) color.NRGBA {
		if th != nil {
			if c, ok := th.OwnColor(key); ok {
				return c
			}
		}
		if tok.A > 0 {
			return tok
		}
		if th != nil {
			if c, ok := th.Color(key); ok {
				return c
			}
		}
		return color.NRGBA{}
	}
	key := func(key string) color.NRGBA { return pick(key, color.NRGBA{}) }
	var c colors
	c.insertedLine = pick("diffEditor.insertedLineBackground", tk.insertedLine)
	c.insertedText = pick("diffEditor.insertedTextBackground", tk.insertedText)
	c.removedLine = pick("diffEditor.removedLineBackground", tk.removedLine)
	c.removedText = pick("diffEditor.removedTextBackground", tk.removedText)
	c.insertedBorder = key("diffEditor.insertedTextBorder")
	c.removedBorder = key("diffEditor.removedTextBorder")
	// Monaco's CSS falls back to the line backgrounds for the gutter.
	c.gutterInserted = or(key("diffEditorGutter.insertedLineBackground"), c.insertedLine)
	c.gutterRemoved = or(key("diffEditorGutter.removedLineBackground"), c.removedLine)
	c.overviewInserted = or(pick("diffEditorOverview.insertedForeground", tk.overviewInserted), opaqueish(c.insertedText))
	c.overviewRemoved = or(pick("diffEditorOverview.removedForeground", tk.overviewRemoved), opaqueish(c.removedText))
	fg := key("editor.foreground")
	c.diagonal = or(pick("diffEditor.diagonalFill", tk.diagonal), alpha(fg, 0x33))
	c.regionBg = or(pick("diffEditor.unchangedRegionBackground", tk.regionBg), alpha(fg, 0x0f))
	c.regionFg = or(pick("diffEditor.unchangedRegionForeground", tk.regionFg), alpha(fg, 0xb0))
	c.regionShadow = alpha(or(key("diffEditor.unchangedRegionShadow"), fg), 0x40)
	c.sign = alpha(fg, 0xb0)
	c.lineNumber = or(key("editorLineNumber.foreground"), alpha(fg, 0x80))
	c.link = or(pick("textLink.foreground", tk.link), fg)
	c.linkHover = or(pick("textLink.activeForeground", tk.linkHover), c.link)
	c.slider = or(key("scrollbarSlider.background"), alpha(fg, 0x33))
	c.sliderHover = or(key("scrollbarSlider.hoverBackground"), alpha(fg, 0x55))
	c.revert = alpha(fg, 0x99)
	c.revertHover = fg
	c.border = or(key("editorOverviewRuler.border"), alpha(fg, 0x20))
	c.background = key("editor.background")
	c.hash = hashOf(
		pack(c.insertedLine), pack(c.insertedText), pack(c.removedLine), pack(c.removedText),
		pack(c.insertedBorder), pack(c.removedBorder), pack(c.gutterInserted), pack(c.gutterRemoved),
		pack(c.overviewInserted), pack(c.overviewRemoved), pack(c.diagonal), pack(c.regionBg),
		pack(c.regionFg), pack(c.regionShadow), pack(c.sign), pack(c.lineNumber), pack(c.link), pack(c.linkHover), pack(c.background))
	return c
}

func pack(c color.NRGBA) uint64 {
	return uint64(c.R)<<24 | uint64(c.G)<<16 | uint64(c.B)<<8 | uint64(c.A)
}

func or(c, fallback color.NRGBA) color.NRGBA {
	if c.A == 0 {
		return fallback
	}
	return c
}

func alpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = uint8(uint16(c.A) * uint16(a) / 255)
	return c
}

// opaqueish returns c strong enough for an overview ruler mark.
func opaqueish(c color.NRGBA) color.NRGBA {
	if c.A < 0x99 {
		c.A = 0x99
	}
	return c
}
