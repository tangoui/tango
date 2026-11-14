package theme

import "github.com/tangoui/tango/ui/paint"

// NotificationToken styles Notification (shadcn: Sonner toasts).
type NotificationToken struct {
	Alias
	Bg                 Color
	TitleColor         Color `contrast:"text:Bg"`
	DescriptionColor   Color `contrast:"text:Bg"`
	TitleFontSize      float32
	TitleFontWeight    int     // zero: regular
	TitleLineHeight    float32 // zero: the theme line height of TitleFontSize
	Width              float32
	Padding            float32 // inline padding (end side)
	PaddingInlineStart float32
	PaddingBlock       float32
	Radius             float32
	BorderColor        Color // zero: none
	Shadow             []paint.Shadow
	// DescriptionFontSize and DescriptionLineHeight size the description
	// (zero: the theme font size / line height).
	DescriptionFontSize   float32
	DescriptionLineHeight float32
	// TitleMarginBottom is the space between title and description,
	// IconGap the space between status icon and text, ActionsMarginTop
	// the space above Actions.
	TitleMarginBottom float32
	IconGap           float32
	ActionsMarginTop  float32
	// MarginEdge is the distance from the window edges to the cards and
	// Gap the space between cards.
	MarginEdge float32
	Gap        float32
	// CloseColor colors the close icon; CloseGap is the space between
	// text and close button (antd: the title keeps paddingLG 24 from the
	// 22px button's end edge, so 2).
	CloseColor Color
	CloseGap   float32
	// ProgressColorStart and ProgressColor are the ends of the
	// ShowProgress bar gradient (equal: solid), ProgressRail its track
	// (zero: none); ProgressInset keeps the bar off the card's rounded
	// corners.
	ProgressColorStart Color
	ProgressColor      Color
	ProgressRail       Color
	ProgressInset      float32
	ProgressHeight     float32
	// ProgressStatusColor colors the bar with the card's status (EP
	// el-progress status) instead of ProgressColor.
	ProgressStatusColor bool
}

func deriveNotification(t *Theme) NotificationToken {
	k := NotificationToken{
		Bg: t.BgElevated, TitleColor: t.Text, DescriptionColor: t.Text, TitleFontSize: t.FontSizeLG, TitleLineHeight: 24,
		Width: 384, Padding: 24, PaddingInlineStart: 24, PaddingBlock: 20,
		Radius: t.RadiusLG, Shadow: t.ShadowSecondary,
		// antd es/notification/style: section gap marginXS 8, wrapper gap
		// marginSM 12, actions marginTop marginSM 12, marginEdge marginLG
		// 24, list gap margin 16, close colorIcon; progress
		// linear-gradient(90deg, colorPrimaryBorderHover, colorPrimary)
		// on rgba(0,0,0,.04), inset borderRadiusLG.
		TitleMarginBottom: 8, IconGap: 12, ActionsMarginTop: 12, MarginEdge: 24, Gap: 16,
		CloseColor: t.TextTertiary, CloseGap: 2,
		ProgressColorStart: t.Primary.BorderHover, ProgressColor: t.Primary.Base, ProgressRail: t.FillTertiary, ProgressInset: t.RadiusLG,
		ProgressHeight: 2,
	}
	switch t.Flavor {
	case Element:
		k.DescriptionColor, k.TitleFontWeight = t.TextSecondary, 700
		k.Width, k.Padding, k.PaddingInlineStart, k.PaddingBlock = 330, 15, 13, 14
		k.BorderColor, k.Shadow = t.BorderSecondary, t.Shadow
		// theme-chalk/src/notification.scss: content margin 6px 0 0,
		// group margin-left 13px, right/left 16px, GAP_SIZE 16
		// (notify.mjs), close --el-text-color-secondary.
		k.TitleMarginBottom, k.IconGap, k.MarginEdge = 6, 13, 16
		k.CloseColor, k.CloseGap = t.TextSecondary, 3 // close at right 15px, content padding-right 26px
		// notification.vue: ElProgress strokeWidth 3, status from the type,
		// on the --el-border-color-lighter rail.
		k.ProgressColorStart, k.ProgressRail, k.ProgressInset, k.ProgressHeight = t.Primary.Base, t.BorderSecondary, 0, 3
		k.ProgressStatusColor = true
	case StyleShadcn:
		// Sonner: 356px cards, p-4, font-medium title, muted description.
		// Sonner 2.0.7 dist/index.mjs: 13px text, title font-weight 500
		// line-height 1.5, description line-height 1.4, content gap 2px,
		// shadow 0 4px 12px rgba(0,0,0,.1).
		k.DescriptionColor, k.TitleFontSize, k.TitleFontWeight, k.TitleLineHeight = t.TextTertiary, 13, 500, 19.5
		k.DescriptionFontSize, k.DescriptionLineHeight = 13, 18.2
		k.Width, k.Padding, k.PaddingInlineStart, k.PaddingBlock = 356, 16, 16, 16
		k.BorderColor = t.PopupBorder
		k.Shadow = []paint.Shadow{{Y: 4, Blur: 12, Color: RGBA(0, 0, 0, .1)}}
		k.TitleMarginBottom, k.ActionsMarginTop = 2, 8
		k.ProgressColorStart, k.ProgressColor, k.ProgressRail, k.ProgressInset = Fade(t.Text, .25), Fade(t.Text, .25), Color{}, 0
	}
	return k
}
