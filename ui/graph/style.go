package graph

import (
	"github.com/tangoui/tango/ui/chart"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Style holds the colours and metrics of the graph widgets. StyleOf
// derives it from the theme tokens, so it follows the design system and
// its dark variant; tweak it per widget with the Style modifiers.
type Style struct {
	Canvas  theme.Color // diagram background
	GridDot theme.Color // background grid dots

	NodeBg     theme.Color
	NodeBorder theme.Color
	NodeText   theme.Color
	NodeDetail theme.Color // second line, ER column types
	NodeRadius float32
	NodeShadow []paint.Shadow
	Font       float32 // node label size at zoom 1
	FontSM     float32 // detail / column / label size at zoom 1

	Selection theme.Color // outline of selected nodes and edges
	Hover     theme.Color // border of the hovered node
	Edge      theme.Color
	EdgeWidth float32 // screen pixels
	LabelBg   theme.Color
	LabelText theme.Color

	Marquee       theme.Color // marquee fill
	MarqueeBorder theme.Color

	MiniBg         theme.Color
	MiniBorder     theme.Color
	MiniNode       theme.Color
	MiniView       theme.Color // visible-area fill
	MiniViewBorder theme.Color
	ToolbarBg      theme.Color
	ToolbarShadow  []paint.Shadow

	// ER tables.
	HeaderBg   theme.Color
	HeaderText theme.Color
	RowHover   theme.Color
	KeyPK      theme.Color // primary-key mark
	KeyFK      theme.Color // foreign-key mark

	// GitGraph.
	Lanes      []theme.Color // branch colours, cycled
	RowSel     theme.Color   // selected row background
	RowSelText theme.Color
	Hash       theme.Color
	Subject    theme.Color
	Meta       theme.Color // author / date
}

// StyleOf derives the graph style from th.
func StyleOf(th *theme.Theme) *Style {
	cs := chart.StyleOf(th)
	s := &Style{
		Canvas:     th.BgContainer,
		GridDot:    th.Border,
		NodeBg:     th.BgElevated,
		NodeBorder: th.Border,
		NodeText:   th.Text,
		NodeDetail: th.TextSecondary,
		NodeRadius: th.Radius,
		NodeShadow: th.ShadowTertiary,
		Font:       th.FontSize,
		FontSM:     th.FontSizeSM,

		Selection: th.Primary.Base,
		Hover:     th.Primary.BorderHover,
		Edge:      th.TextTertiary,
		EdgeWidth: 1.5,
		LabelBg:   th.BgContainer,
		LabelText: th.TextSecondary,

		Marquee:       theme.Fade(th.Primary.Base, 0.08),
		MarqueeBorder: th.Primary.Base,

		MiniBg:         th.BgElevated,
		MiniBorder:     th.BorderSecondary,
		MiniNode:       th.Fill,
		MiniView:       theme.Fade(th.Primary.Base, 0.08),
		MiniViewBorder: th.Primary.Base,
		ToolbarBg:      th.BgElevated,
		ToolbarShadow:  th.Shadow,

		HeaderBg:   th.FillAlter,
		HeaderText: th.TextHeading,
		RowHover:   th.ItemHoverBg,
		KeyPK:      th.Warning.Base,
		KeyFK:      th.Primary.Base,

		Lanes:      cs.Colors,
		RowSel:     th.ItemSelectedBg,
		RowSelText: th.ItemSelected,
		Hash:       th.TextTertiary,
		Subject:    th.Text,
		Meta:       th.TextSecondary,
	}
	if s.HeaderText.A == 0 {
		s.HeaderText = th.Text
	}
	if s.HeaderBg.A == 0 {
		s.HeaderBg = th.FillQuaternary
	}
	if len(s.Lanes) == 0 {
		s.Lanes = []theme.Color{th.Primary.Base, th.Success.Base, th.Warning.Base, th.Error.Base, th.Info.Base}
	}
	return s
}

// lane returns the colour of lane colour index i.
func (s *Style) lane(i int) theme.Color { return s.Lanes[((i%len(s.Lanes))+len(s.Lanes))%len(s.Lanes)] }
