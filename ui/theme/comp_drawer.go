package theme

import "github.com/tangoui/tango/ui/paint"

// DrawerToken styles Drawer (shadcn: Sheet).
type DrawerToken struct {
	Alias
	Bg                  Color
	TitleColor          Color `contrast:"text:Bg"`
	TitleFontSize       float32
	TitleFontWeight     int // zero: regular
	HeaderPaddingTop    float32
	HeaderPaddingBottom float32
	HeaderPaddingInline float32
	HeaderBorderColor   Color   // divider under the header (zero: none)
	Padding             float32 // body padding
	BodyPaddingTop      float32
	FooterPaddingTop    float32
	FooterPaddingBlock  float32 // bottom padding of the footer
	FooterPaddingInline float32
	FooterBorderColor   Color // divider above the footer (zero: none)
	CloseIconColor      Color // zero: the icon button default
	BorderColor         Color // edge facing the page (zero: none)
	MaskBg              Color
	Shadow              []paint.Shadow
	ResizeHandleSize    float32 // Resizable: hit width of the edge handle
	ResizeLineColor     Color   `contrast:"nontext:Bg"` // Resizable: edge line under the pointer / while dragging
	ResizeLineSize      float32
}

func deriveDrawer(t *Theme) DrawerToken {
	k := DrawerToken{
		Bg: t.BgElevated, TitleColor: t.Text, TitleFontSize: t.FontSizeLG, TitleFontWeight: t.FontWeightStrong,
		HeaderPaddingTop: 16, HeaderPaddingBottom: 16, HeaderPaddingInline: 24, HeaderBorderColor: t.Split,
		Padding: 24, BodyPaddingTop: 24,
		FooterPaddingTop: 8, FooterPaddingBlock: 8, FooterPaddingInline: 16, FooterBorderColor: t.Split,
		MaskBg: t.BgMask, Shadow: t.ShadowSecondary,
		ResizeHandleSize: 8, ResizeLineColor: t.Primary.Base, ResizeLineSize: 2,
	}
	switch t.Flavor {
	case Element:
		k.TitleColor, k.TitleFontWeight = t.TextSecondary, 0
		k.HeaderPaddingTop, k.HeaderPaddingBottom, k.HeaderPaddingInline, k.HeaderBorderColor = 20, 0, 20, Color{}
		k.Padding, k.BodyPaddingTop = 20, 20
		k.FooterPaddingTop, k.FooterPaddingBlock, k.FooterPaddingInline, k.FooterBorderColor = 10, 20, 20, Color{}
	case StyleShadcn:
		// SheetHeader p-6 with a text-lg semibold title, no dividers.
		k.TitleFontSize, k.TitleFontWeight = 18, 600
		k.HeaderPaddingTop, k.HeaderPaddingBottom, k.HeaderPaddingInline, k.HeaderBorderColor = 24, 0, 24, Color{}
		k.BodyPaddingTop = 16
		k.FooterPaddingTop, k.FooterPaddingBlock, k.FooterPaddingInline, k.FooterBorderColor = 24, 24, 24, Color{}
		k.CloseIconColor, k.BorderColor = Fade(t.Text, .7), t.ModalBorder
	}
	return k
}
