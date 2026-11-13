package theme

// TreeToken styles Tree and VirtualTree, and the tree rows of the
// TreeSelect panel (its trigger follows Select).
type TreeToken struct {
	Alias
	TitleHeight            float32 // row height
	NodeGap                float32 // space under each row
	DropdownNodeHeight     float32 // TreeSelect panel row height
	IndentSize             float32 // indent per level
	SwitcherSize           float32 // width of the expand switcher column
	NodeBorderRadius       float32 // title highlight (Element fills the whole row, square)
	TitleColor             Color   `contrast:"text:@Page"`
	NodeHoverBg            Color
	NodeSelectedBg         Color
	NodeSelectedColor      Color `contrast:"text:NodeSelectedBg"`
	NodeSelectedFontWeight int   // title weight of the selected node
	SwitcherColor          Color `contrast:"nontext:@Page"` // expand caret / chevron
	LineColor              Color // ShowLine connectors
	SearchHighlightColor   Color `contrast:"text:SearchHighlightBg"` // Tree.Search match (zero: the title color)
	SearchHighlightBg      Color // mark behind the match (zero: none)
	FilterHighlightColor   Color `contrast:"text:@Page"` // VirtualTree.Filter match
	DropIndicatorColor     Color `contrast:"nontext:@Page"`
	DraggingNodeBg         Color // title of the node being dragged
}

func deriveTree(t *Theme) TreeToken {
	k := TreeToken{
		TitleHeight: 24, NodeGap: 4, DropdownNodeHeight: 28, IndentSize: 24, SwitcherSize: 24,
		NodeBorderRadius: t.RadiusSM,
		TitleColor:       t.Text, NodeHoverBg: t.ItemHoverBg, NodeSelectedBg: t.ItemSelectedBg, NodeSelectedColor: t.Text,
		NodeSelectedFontWeight: 400,
		SwitcherColor:          t.Text, LineColor: t.Border,
		SearchHighlightColor: Hex("#ff5500"), FilterHighlightColor: t.Warning.Base,
		DropIndicatorColor: t.Primary.Base, DraggingNodeBg: Fade(t.Primary.Bg, 0.9),
	}
	switch t.Flavor {
	case Element:
		// el-tree: 26px rows without gaps, 18px indent, the current row
		// tinted with the brand, placeholder-colored carets.
		k.TitleHeight, k.NodeGap, k.DropdownNodeHeight, k.IndentSize = 26, 0, 26, 18
		k.NodeBorderRadius = 0
		k.NodeSelectedBg = t.Primary.Bg
		k.SwitcherColor = t.TextQuaternary
		k.SearchHighlightColor, k.FilterHighlightColor = t.Primary.Base, t.Primary.Base
	case StyleShadcn:
		// Sidebar file tree: h-7 rows with a 2px gap, 16px indent, a 20px
		// chevron column; the active item in medium weight; a soft mark
		// behind search matches.
		k.TitleHeight, k.NodeGap, k.DropdownNodeHeight, k.IndentSize, k.SwitcherSize = 28, 2, 32, 16, 20
		k.NodeBorderRadius = t.Radius
		k.NodeSelectedFontWeight = 500
		k.SwitcherColor = t.TextTertiary
		k.SearchHighlightColor, k.SearchHighlightBg = Color{}, t.Warning.Border
		k.FilterHighlightColor = t.Primary.Base
	}
	return k
}
