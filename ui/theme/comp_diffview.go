package theme

// DiffViewToken styles the diff editor (ui/editor/diffview): the
// backgrounds of inserted and removed lines and characters, the merged
// overview ruler's marks, the diagonal fill of the alignment padding and
// the bar of a folded unchanged region. The diff view uses them when the
// editor theme does not set the matching Monaco colors itself
// (diffEditor.*, diffEditorOverview.*), see docs/specs/diff-view.md 3.11.
// They derive from the global tokens only, so dark themes need nothing of
// their own.
type DiffViewToken struct {
	Alias
	InsertedLineBg Color // diffEditor.insertedLineBackground
	InsertedTextBg Color // diffEditor.insertedTextBackground, over InsertedLineBg
	RemovedLineBg  Color // diffEditor.removedLineBackground
	RemovedTextBg  Color // diffEditor.removedTextBackground, over RemovedLineBg

	OverviewInserted Color // diffEditorOverview.insertedForeground
	OverviewRemoved  Color // diffEditorOverview.removedForeground
	DiagonalFill     Color // diffEditor.diagonalFill: stripes of the padding

	RegionBg       Color // the bar of a folded region (diffEditor.unchangedRegionBackground)
	RegionColor    Color `contrast:"text:RegionBg"` // its "N hidden lines" (unchangedRegionForeground)
	LinkColor      Color `contrast:"text:RegionBg"` // its "show more" buttons and the hunk actions
	LinkHoverColor Color `contrast:"text:RegionBg"`
}

func deriveDiffView(t *Theme) DiffViewToken {
	// The buttons are Ant's link colors (colorLink, colorLinkHover = the
	// primary palette's base and hover), lifted towards the text color
	// where they miss 4.5:1 on the bar (light Element, dark Ant).
	bar := Over(t.FillTertiary, t.BgContainer)
	return DiffViewToken{
		// The tints come from the status palettes' border steps, the
		// lines translucent over the editor, the changed characters
		// stronger (Ant's Bg / BgHover steps are too faint for a diff:
		// colorErrorBgHover #fff1f0 is colorErrorBg #fff2f0 to the eye,
		// and colorSuccessBg barely differs from white).
		InsertedLineBg:   Fade(t.Success.Border, 0.35),
		InsertedTextBg:   Fade(t.Success.BorderHover, 0.45),
		RemovedLineBg:    Fade(t.Error.Border, 0.35),
		RemovedTextBg:    Fade(t.Error.BorderHover, 0.45),
		OverviewInserted: t.Success.Base,
		OverviewRemoved:  t.Error.Base,
		DiagonalFill:     t.Split,
		RegionBg:         t.FillTertiary,
		RegionColor:      t.TextSecondary,
		LinkColor:        EnsureContrast(t.Primary.Base, bar, 4.5, t.Text),
		LinkHoverColor:   EnsureContrast(t.Primary.Hover, bar, 4.5, t.Text),
	}
}
