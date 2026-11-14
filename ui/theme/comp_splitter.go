package theme

// SplitterToken styles Splitter.
type SplitterToken struct {
	Alias
	SplitTriggerSize      float32 // hit width of a bar (the space between panes)
	SplitBarSize          float32 // thickness of the drag handle (spinner)
	SplitBarDraggableSize float32 // length of the drag handle
	BarColor              Color   // the line between panes
	BarLineSize           float32 // its thickness
	BarHoverColor         Color   // the line under the pointer (zero: BarColor)
	BarActiveColor        Color   // the line while dragging (zero: BarHoverColor)
	HandleColor           Color   // drag handle shown at rest (zero: only on hover)
	HandleHoverColor      Color   `contrast:"nontext:@Page"` // drag handle under the pointer
	HandleActiveColor     Color   `contrast:"nontext:@Page"` // drag handle while dragging
	CollapseBg            Color   // collapse arrow buttons
	CollapseHoverBg       Color
	CollapseColor         Color `contrast:"nontext:CollapseBg"`
	CollapseRadius        float32
	CollapseWidth         float32 // across the bar
	CollapseHeight        float32 // along the bar
	PreviewColor          Color   `contrast:"nontext:@Page"` // Lazy mode: the line showing where the bar will land
	PreviewSize           float32 // its thickness

	// WithHandle grip box (shadcn ResizableHandle withHandle).
	GripWidth    float32 // across the bar
	GripHeight   float32 // along the bar
	GripRadius   float32
	GripBg       Color
	GripBorder   Color
	GripColor    Color
	GripIconSize float32
}

func deriveSplitter(t *Theme) SplitterToken {
	// antd 6.7 es/splitter/style/index.js: the bar line splitBarSize 2 in
	// controlItemBgHover (colorFillTertiary), hover controlItemBgActive
	// (colorPrimaryBg), dragging controlItemBgActiveHover
	// (colorPrimaryBgHover); the 20x2 spinner in colorFill always shown;
	// collapse buttons fontSizeSM x controlHeightSM, controlItemBgHover,
	// borderRadiusXS, colorText; the lazy preview colorPrimary.
	k := SplitterToken{
		SplitTriggerSize: 6, SplitBarSize: 2, SplitBarDraggableSize: 20,
		BarColor: t.FillTertiary, BarLineSize: 2, BarHoverColor: t.Primary.Bg, BarActiveColor: t.Primary.BgHover,
		HandleColor: t.Fill, HandleHoverColor: t.Fill, HandleActiveColor: t.Fill,
		CollapseBg: t.FillTertiary, CollapseHoverBg: t.Primary.Bg, CollapseColor: t.Text,
		CollapseRadius: t.RadiusXS, CollapseWidth: t.FontSizeSM, CollapseHeight: t.ControlHeightSM,
		PreviewColor: t.Primary.Base, PreviewSize: 2,
		GripWidth: 12, GripHeight: 16, GripRadius: 2, GripBg: t.Border, GripBorder: t.Border, GripColor: t.Text, GripIconSize: 10,
	}
	switch t.Flavor {
	case Element:
		// el-splitter: 2px border-color-light line, hover primary-light-5,
		// dragging primary-light-3, no spinner; collapse icons 16x24 in
		// border-color-light, radius 2, hover primary-light-5.
		k.BarColor, k.BarHoverColor, k.BarActiveColor = t.Split, t.Primary.Border, t.Primary.Hover
		k.HandleColor, k.HandleHoverColor, k.HandleActiveColor = Color{}, Color{}, Color{}
		k.CollapseBg, k.CollapseHoverBg, k.CollapseColor = t.Split, t.Primary.Border, t.TextSecondary
		k.CollapseRadius, k.CollapseWidth, k.CollapseHeight = 2, 16, 24
	case StyleShadcn:
		// resizable.tsx ResizableHandle: w-px bg-border, focus ring, no
		// hover state; withHandle h-4 w-3 rounded-xs border bg-border
		// around a GripVertical size-2.5.
		k.BarColor, k.BarLineSize, k.BarHoverColor, k.BarActiveColor = t.Border, 1, Color{}, Color{}
		k.HandleColor, k.HandleHoverColor, k.HandleActiveColor = Color{}, t.Fill, t.Primary.Base
		k.CollapseBg, k.CollapseHoverBg, k.CollapseColor = t.FillSecondary, t.Fill, t.TextSecondary
		k.CollapseRadius, k.CollapseWidth, k.CollapseHeight = t.RadiusSM, 16, 20
	}
	return k
}
