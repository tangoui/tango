package theme

// SplitterToken styles Splitter.
type SplitterToken struct {
	Alias
	SplitTriggerSize      float32 // hit width of a bar (the space between panes)
	SplitBarSize          float32 // thickness of the drag handle shown on hover
	SplitBarDraggableSize float32 // length of the drag handle
	BarColor              Color   // the 1px line between panes
	HandleHoverColor      Color   `contrast:"nontext:@Page"` // drag handle under the pointer
	HandleActiveColor     Color   `contrast:"nontext:@Page"` // drag handle while dragging
	CollapseBg            Color   // collapse arrow buttons
	CollapseHoverBg       Color
	CollapseColor         Color `contrast:"nontext:CollapseBg"`
	CollapseRadius        float32
	PreviewColor          Color   `contrast:"nontext:@Page"` // Lazy mode: the line showing where the bar will land
	PreviewSize           float32 // its thickness
}

func deriveSplitter(t *Theme) SplitterToken {
	return SplitterToken{
		SplitTriggerSize: 6, SplitBarSize: 2, SplitBarDraggableSize: 20,
		BarColor: t.Split, HandleHoverColor: t.Fill, HandleActiveColor: t.Primary.Base,
		CollapseBg: t.FillSecondary, CollapseHoverBg: t.Fill, CollapseColor: t.TextSecondary,
		CollapseRadius: t.RadiusSM,
		PreviewColor:   t.Primary.Base, PreviewSize: 2,
	}
}
