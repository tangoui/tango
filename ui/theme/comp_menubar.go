package theme

import "github.com/tangoui/tango/ui/paint"

// MenubarToken styles Menubar: the bar and its top-level triggers. The
// drop-down panels are the Dropdown panels of the same theme (Dropdown
// token); the Indicator* fields add the check mark and radio dot of
// checkable items there.
type MenubarToken struct {
	Alias
	Bg            Color          // bar background (zero: none)
	BorderColor   Color          // outline of the bar (zero: none)
	DividerColor  Color          // rule under the bar (zero: none)
	Radius        float32        // bar corners
	Shadow        []paint.Shadow // bar shadow (nil: none)
	Height        float32        // bar height
	PaddingInline float32        // bar padding at both ends
	Gap           float32        // between triggers

	TriggerHeight        float32
	TriggerPaddingInline float32
	TriggerRadius        float32
	TriggerFontSize      float32
	TriggerFontWeight    int   // zero: regular
	TriggerColor         Color `contrast:"text:Bg"`
	TriggerHoverBg       Color // zero: none
	TriggerHoverColor    Color `contrast:"text:TriggerHoverBg"`
	TriggerOpenBg        Color // trigger of the open menu (zero: none)
	TriggerOpenColor     Color `contrast:"text:TriggerOpenBg"`

	// Checkable items in the panels: the indicator column at the item's
	// inline start (IndicatorWidth wide, labels of the panel move past it).
	IndicatorColor Color   `contrast:"nontext:@Elevated"`
	IndicatorSize  float32 // check mark size; the radio dot is half of it
	IndicatorWidth float32
}

func deriveMenubar(t *Theme) MenubarToken {
	k := MenubarToken{
		Bg: t.BgContainer, DividerColor: t.Split,
		Height: 36, PaddingInline: 4, Gap: 2,
		TriggerHeight: 28, TriggerPaddingInline: 10, TriggerRadius: t.RadiusSM,
		TriggerFontSize: t.FontSize, TriggerColor: t.Text,
		TriggerHoverBg: t.ItemHoverBg, TriggerHoverColor: t.Text,
		TriggerOpenBg: t.ItemHoverBg, TriggerOpenColor: t.Text,
		IndicatorColor: t.Primary.Base, IndicatorSize: 14, IndicatorWidth: 22,
	}
	switch t.Flavor {
	case Element:
		// Element: a flat strip; the open menu's trigger takes the brand
		// tint like el-dropdown items under the pointer.
		k.Height, k.Gap = 40, 0
		k.TriggerHeight, k.TriggerPaddingInline = 32, 12
		k.TriggerColor, k.TriggerHoverColor = t.TextSecondary, t.Primary.Base
		k.TriggerHoverBg = t.Primary.Bg
		k.TriggerOpenBg, k.TriggerOpenColor = t.Primary.Bg, t.Primary.Base
		k.IndicatorSize, k.IndicatorWidth = 12, 20
	case StyleShadcn:
		// Menubar: h-9 rounded-md border bg-background p-1 shadow-xs, gap-1;
		// MenubarTrigger: rounded-sm px-2 py-1 font-medium, accent when open;
		// check / radio items: pl-8 with a size-4 indicator at left-2.
		k.BorderColor, k.DividerColor = t.Border, Color{}
		k.Radius, k.Shadow = t.Radius, t.InputShadow
		k.Gap = 4
		k.TriggerPaddingInline, k.TriggerFontWeight = 8, 500
		k.TriggerHoverColor, k.TriggerOpenColor = t.ItemSelected, t.ItemSelected
		k.IndicatorColor, k.IndicatorSize, k.IndicatorWidth = t.Text, 16, 24
	}
	return k
}
