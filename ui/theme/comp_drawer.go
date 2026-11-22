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
	MaskBg              Color // mask color including opacity
	Shadow              []paint.Shadow
	ResizeHandleSize    float32 // Resizable: hit width of the edge handle
	ResizeLineColor     Color   `contrast:"nontext:Bg" spec:"dragger hover: antd 6.7 drawer/style colorPrimary opacity .2; el-drawer primary"` // Resizable: edge line under the pointer / while dragging
	ResizeLineSize      float32
	// ResizeLineActiveColor colors the edge while dragging (zero:
	// ResizeLineColor).
	ResizeLineActiveColor Color `contrast:"nontext:Bg" spec:"dragger dragging: antd 6.7 drawer/style colorPrimary opacity .3; el-drawer primary"`

	// Default size: Size, or SizeRatio of the window extent when Size is
	// zero; both set: the smaller (shadcn w-3/4 sm:max-w-sm).
	Size      float32
	SizeRatio float64
	// CloseAtEnd puts the close button after the title and extra
	// (closable.placement "end"); false: before the title (Ant "start").
	CloseAtEnd    bool
	CloseBtnSize  float32
	CloseIconSize float32
	HeaderGap     float32 // between close button, title and extra
}

func deriveDrawer(t *Theme) DrawerToken {
	k := DrawerToken{
		Bg: t.BgElevated, TitleColor: t.Text, TitleFontSize: t.FontSizeLG, TitleFontWeight: t.FontWeightStrong,
		HeaderPaddingTop: 16, HeaderPaddingBottom: 16, HeaderPaddingInline: 24, HeaderBorderColor: t.Split,
		Padding: 24, BodyPaddingTop: 24,
		FooterPaddingTop: 8, FooterPaddingBlock: 8, FooterPaddingInline: 16, FooterBorderColor: t.Split,
		MaskBg: t.BgMask, Shadow: t.ShadowSecondary,
		// antd 6.7 es/drawer/style/index.js: draggerSize 4, hover
		// colorPrimary at opacity .2, dragging .3; close fontSizeLG +
		// paddingXS = 24 box, marginInlineEnd marginXS; Drawer.js
		// DEFAULT_SIZE 378.
		ResizeHandleSize: 4, ResizeLineSize: 4,
		ResizeLineColor: Fade(t.Primary.Base, .2), ResizeLineActiveColor: Fade(t.Primary.Base, .3),
		Size: 378, CloseBtnSize: 24, CloseIconSize: 16, HeaderGap: 8,
	}
	switch t.Flavor {
	case Element:
		// el-drawer header color text-color-primary, title 16px;
		// close-btn font-size-extra-large 20, color inherit; size 30%;
		// dragger 8px, primary on hover.
		k.TitleColor, k.TitleFontWeight = t.Text, 0
		k.Size, k.SizeRatio, k.CloseAtEnd = 0, .3, true
		k.CloseBtnSize, k.CloseIconSize, k.CloseIconColor = 20, 20, t.Text
		k.ResizeHandleSize, k.ResizeLineSize = 8, 2
		k.ResizeLineColor, k.ResizeLineActiveColor = t.Primary.Base, t.Primary.Base
		k.HeaderPaddingTop, k.HeaderPaddingBottom, k.HeaderPaddingInline, k.HeaderBorderColor = 20, 0, 20, Color{}
		k.Padding, k.BodyPaddingTop = 20, 20
		k.FooterPaddingTop, k.FooterPaddingBlock, k.FooterPaddingInline, k.FooterBorderColor = 10, 20, 20, Color{}
		if t.elementControls {
			// drawer.scss: --el-drawer-bg-color, --el-box-shadow-dark and
			// color-primary on the close button's hover and focus states.
			k.Bg, k.MaskBg = t.BgContainer, RGBA(0, 0, 0, .5)
			k.Shadow = []paint.Shadow{sh(0, 16, 48, 16, RGBA(0, 0, 0, .08)), sh(0, 12, 32, 0, RGBA(0, 0, 0, .12)), sh(0, 8, 16, -8, RGBA(0, 0, 0, .16))}
		}
	case StyleShadcn:
		// sheet.tsx: SheetHeader p-4 gap-1.5 with a font-semibold
		// title (1rem), content gap-4, SheetFooter p-4, no dividers;
		// w-3/4 sm:max-w-sm; close top-4 right-4, size-4 icon,
		// opacity-70.
		k.TitleFontSize, k.TitleFontWeight = 16, 600
		k.HeaderPaddingTop, k.HeaderPaddingBottom, k.HeaderPaddingInline, k.HeaderBorderColor = 16, 0, 16, Color{}
		k.Padding, k.BodyPaddingTop = 16, 16
		k.FooterPaddingTop, k.FooterPaddingBlock, k.FooterPaddingInline, k.FooterBorderColor = 16, 16, 16, Color{}
		k.CloseIconColor, k.BorderColor = Fade(t.Text, .7), t.ModalBorder
		k.Size, k.SizeRatio, k.CloseAtEnd = 384, .75, true
		k.MaskBg = RGBA(0, 0, 0, .5) // SheetOverlay bg-black/50
		k.CloseBtnSize, k.CloseIconSize = 16, 16
		k.ResizeHandleSize, k.ResizeLineSize = 8, 2
		k.ResizeLineColor, k.ResizeLineActiveColor = t.Primary.Base, t.Primary.Base
	}
	return k
}
