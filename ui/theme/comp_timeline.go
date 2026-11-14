package theme

// TimelineToken styles Timeline.
type TimelineToken struct {
	Alias
	TailColor         Color   // the axis between two dots
	TailWidth         float32 // axis thickness
	DotBg             Color   // fill inside hollow dots and the ring cutting the axis
	DotColor          Color   `contrast:"nontext:DotBg"` // dot of an item without Color
	DotSize           float32 // dot diameter; also the width of the axis column
	DotSizeLG         float32 // Element large / icon dot
	DotBorderWidth    float32 // ring of a hollow dot
	DotOffsetTop      float32 // dot top inside the item's first line
	IconDotSize       float32 // DotIcon size (Element: the icon sits in a DotSizeLG dot)
	PendingColor      Color   // spinner of the pending item
	ItemPaddingBottom float32 // space under every item but the last
	ContentGap        float32 // axis to content / label
	ContentColor      Color   `contrast:"text:@Page"`
	LabelColor        Color   `contrast:"text:@Page"` // Ant label on the opposite side
	TimeColor         Color   `contrast:"text:@Page"` // timestamp
	TimeFontSize      float32

	// Horizontal timelines (Timeline.Horizontal).
	HorizontalGap         float32 // axis to the content / label above and below
	HorizontalItemPadding float32 // inline padding of every event's texts
}

func deriveTimeline(t *Theme) TimelineToken {
	k := TimelineToken{
		TailColor: t.Split, TailWidth: 2, DotBg: t.BgContainer, DotColor: t.Primary.Base,
		DotSize: 10, DotSizeLG: 10, DotBorderWidth: 3, DotOffsetTop: 5.5, IconDotSize: 16,
		PendingColor: t.Primary.Base, ItemPaddingBottom: 20, ContentGap: 16,
		ContentColor: t.Text, LabelColor: t.Text, TimeColor: t.TextTertiary, TimeFontSize: t.FontSize,
		HorizontalGap: 8, HorizontalItemPadding: 8,
	}
	switch t.Flavor {
	case Element:
		k.DotColor = t.Split // --el-timeline-node-color
		k.DotSize, k.DotSizeLG, k.DotBorderWidth, k.DotOffsetTop, k.IconDotSize = 12, 14, 2, 4, 14
		k.TimeFontSize = 13
	case StyleShadcn:
		// 1px border axis, filled size-2.5 dots centred on the 20px line.
		k.TailColor, k.TailWidth, k.DotOffsetTop = t.Border, 1, 5
		k.TimeFontSize = t.FontSizeSM
	}
	return k
}
