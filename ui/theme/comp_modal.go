package theme

import "github.com/tangoui/tango/ui/paint"

// ModalToken styles Modal and the Confirm / MessageBox dialogs.
type ModalToken struct {
	Alias
	ContentBg          Color
	HeaderBg           Color // zero: none
	FooterBg           Color // zero: none
	TitleColor         Color `contrast:"text:ContentBg"`
	TitleFontSize      float32
	TitleFontWeight    int     // zero: regular
	TitleLineHeight    float32 // zero: the theme line height of TitleFontSize
	Padding            float32 // inline padding of the panel
	PaddingBlock       float32
	HeaderMarginBottom float32
	FooterMarginTop    float32
	CloseIconColor     Color // zero: the icon button default
	Radius             float32
	BorderColor        Color // zero: none
	MaskBg             Color // mask color including opacity
	Shadow             []paint.Shadow

	// Default placement and size (Modal.Width / Top / Centered override).
	Width      float32 // default width
	WidthRatio float64 // nonzero: at least this share of the window width (Element 50%)
	Top        float32 // distance from the window top
	TopRatio   float64 // nonzero: Top is this share of the window height (Element 15vh)
	Centered   bool    // centered vertically by default (shadcn)
	// Close button: box and glyph size, distance from the panel's top and
	// inline-end edges (absolutely placed over the header).
	CloseBtnSize  float32
	CloseIconSize float32
	CloseInsetTop float32
	CloseInsetEnd float32
	// CloseNoHoverBg: no fill under the close button on hover (Element
	// headerbtn background transparent, shadcn); CloseHoverColor, when
	// set, is the glyph color on hover (Element primary, shadcn
	// opacity-100).
	CloseNoHoverBg  bool
	CloseHoverColor Color

	// Confirm / MessageBox / Prompt.
	ConfirmWidth         float32
	ConfirmBtnsMarginTop float32 // Ant body: buttons below the text
}

// DialogMask sets the mask color and alpha for Modal (including Confirm)
// and Drawer. Pass it to WithComponents, Theme.Override or
// ConfigProvider.Components. A zero color gives a fully transparent mask.
// Element Plus 2.14.8's .el-overlay uses Hex("#00000080").
func DialogMask(c Color) Components {
	if c == (Color{}) {
		c = Transparent
	}
	return Components{Modal: ModalToken{MaskBg: c}, Drawer: DrawerToken{MaskBg: c}}
}

func deriveModal(t *Theme) ModalToken {
	k := ModalToken{
		ContentBg: t.BgElevated, TitleColor: t.Text, TitleFontSize: t.FontSizeLG, TitleFontWeight: t.FontWeightStrong,
		Padding: 24, PaddingBlock: 20, HeaderMarginBottom: 8, FooterMarginTop: 12,
		Radius: t.RadiusLG, BorderColor: t.ModalBorder, MaskBg: t.BgMask, Shadow: t.ShadowSecondary,
		// antd 6.7 es/modal/Modal.js width 520, style/index.js top 100;
		// close: modalCloseBtnSize = controlHeight 32, inset
		// (modalHeaderHeight 56 - 32) / 2 = 12, icon fontSizeLG.
		Width: 520, Top: 100,
		CloseBtnSize: 32, CloseIconSize: 16, CloseInsetTop: 12, CloseInsetEnd: 12,
		// ConfirmDialog.js width 416; style confirmBtnsMarginTop = marginSM.
		ConfirmWidth: 416, ConfirmBtnsMarginTop: 12,
	}
	switch t.Flavor {
	case Element:
		k.TitleFontSize, k.TitleFontWeight, k.TitleLineHeight = 18, 0, 24
		k.Padding, k.PaddingBlock, k.HeaderMarginBottom, k.FooterMarginTop = 16, 16, 16, 16
		k.Radius = t.Radius
		// el-dialog width 50% (min 320 here), margin-top 15vh; headerbtn
		// 48x48 at top 0 / right 0, icon --el-message-close-size 16.
		k.Width, k.WidthRatio, k.Top, k.TopRatio = 320, .5, 0, .15
		k.CloseBtnSize, k.CloseInsetTop, k.CloseInsetEnd = 48, 0, 0
		// .el-dialog__close color-info, primary on hover / focus.
		k.CloseNoHoverBg, k.CloseIconColor, k.CloseHoverColor = true, t.Info.Base, t.Primary.Base
		k.ConfirmWidth = 420 // --el-messagebox-width
		if t.elementControls {
			// dialog.scss and common/var.scss $dialog; overlay.scss uses
			// --el-overlay-color-lighter in both light and dark themes.
			k.ContentBg, k.MaskBg = t.BgContainer, RGBA(0, 0, 0, .5)
			k.Width, k.Shadow = 0, t.Shadow
		}
	case StyleShadcn:
		// DialogContent p-6, DialogTitle text-lg font-semibold.
		k.TitleFontSize, k.TitleFontWeight = 18, 600
		k.Padding, k.PaddingBlock, k.HeaderMarginBottom, k.FooterMarginTop = 24, 24, 16, 16
		k.CloseIconColor = Fade(t.Text, .7)
		// dialog.tsx: sm:max-w-lg, centered, title leading-none, close
		// absolute top-4 right-4 with a size-4 icon and no padding.
		k.Width, k.Top, k.Centered, k.TitleLineHeight = 512, 0, true, 18
		k.CloseBtnSize, k.CloseInsetTop, k.CloseInsetEnd = 16, 16, 16
		k.CloseNoHoverBg, k.CloseHoverColor = true, t.Text // opacity-70 -> hover:opacity-100
		k.ConfirmWidth = 512                               // alert-dialog.tsx sm:max-w-lg
		k.MaskBg = RGBA(0, 0, 0, .5)                       // DialogOverlay / AlertDialogOverlay bg-black/50
	}
	return k
}
