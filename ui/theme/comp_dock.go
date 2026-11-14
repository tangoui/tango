package theme

// DockToken styles Dock (docking layout of tab groups). The split bars
// reuse the Splitter tokens; the tab strip of each group is a Tabs bar and
// reads the Tabs tokens.
type DockToken struct {
	Alias
	Bg            Color   // behind the groups (shows in the split bars)
	GroupBg       Color   // content area of a group
	HeaderBg      Color   // tab strip of a group (zero: none)
	HeaderBorder  Color   // line under the tab strip (zero: none; Ant and Element card tabs draw their own)
	ActiveBorder  Color   `contrast:"nontext:HeaderBg"` // top edge of the focused group's strip (zero: none)
	GutterSize    float32 // hit size of a split bar (the gap between groups)
	MinPaneSize   float32 // smallest size a split bar leaves to a group
	DropBg        Color   // drop preview fill
	DropBorder    Color   `contrast:"nontext:@Page"` // drop preview outline and tab insertion mark
	DropRadius    float32
	GhostBg       Color // dragged tab following the pointer
	GhostText     Color `contrast:"text:GhostBg"`
	GhostRadius   float32
	EmptyText     Color   `contrast:"text:GroupBg"` // hint of an empty group
	ActionBoxSize float32 // header buttons (split, maximize)
}

func deriveDock(t *Theme) DockToken {
	k := DockToken{
		Bg: t.BgLayout, GroupBg: t.BgContainer, HeaderBg: t.FillAlter,
		ActiveBorder: t.Primary.Base,
		GutterSize:   4, MinPaneSize: 80,
		DropBg: Fade(t.Primary.Base, 0.12), DropBorder: t.Primary.Base, DropRadius: t.RadiusSM,
		GhostBg: t.BgElevated, GhostText: t.Text, GhostRadius: t.RadiusSM,
		EmptyText: t.TextTertiary, ActionBoxSize: 24,
	}
	switch t.Flavor {
	case StyleShadcn:
		k.Bg, k.HeaderBg, k.HeaderBorder = t.Border, t.BgContainer, t.Border
		k.DropBg, k.ActiveBorder = Fade(t.Text, 0.08), t.Text
		k.DropBorder = t.TextTertiary
	}
	return k
}
