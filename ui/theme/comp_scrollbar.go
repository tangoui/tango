package theme

// ScrollbarToken styles the overlay scrollbars used by a scroll container.
type ScrollbarToken struct {
	Alias
	// TrackInset insets both ends of the track. Zero uses Tango's edge strip.
	TrackInset float32
	// Thickness and ThumbRadius are logical pixels.
	Thickness, ThumbRadius float32
	MinThumbLength         float32
	Thumb, ThumbHover      Color
}

func deriveScrollbar(t *Theme) ScrollbarToken {
	if t.elementControls {
		// element-plus@2.14.8: scrollbar.scss and $scrollbar in common/var.scss.
		// The 20px minimum is the component's minSize prop (scrollbar.mjs).
		return ScrollbarToken{TrackInset: 2, Thickness: 6, ThumbRadius: 4, MinThumbLength: 20,
			Thumb: Fade(t.TextSecondary, .3), ThumbHover: Fade(t.TextSecondary, .5)}
	}
	return ScrollbarToken{Thickness: 6, ThumbRadius: 3, MinThumbLength: 24, Thumb: t.TextQuaternary, ThumbHover: t.TextTertiary}
}
