package theme

// TextOn returns the color for text or marks drawn on a solid fill bg:
// onDark (the usual white of solid controls, e.g. TextOnSolid or a
// component's text token) unless bg is light (Luma above 0.75, such as
// shadcn's near-white primary in dark mode), where a dark color reads
// better — the theme's Text when it is dark, else BgContainer (dark
// themes, whose Text is light).
func (t *Theme) TextOn(bg, onDark Color) Color {
	if Luma(bg) <= 0.75 {
		return onDark
	}
	if Luma(t.Text) <= 0.5 {
		return t.Text
	}
	return t.BgContainer
}
