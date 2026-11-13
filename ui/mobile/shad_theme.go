package mobile

import (
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shadcn derives the mobile tokens from the shadcn/ui variables of the
// ui theme (or of the stock neutral shadcn theme when the ui theme is
// another design system). shadcn has no mobile components; the values
// carry its desktop proportions over to touch sizes: neutral surfaces,
// thin borders, rounded-lg / rounded-xl, muted foregrounds, a near-black
// (near-white in dark mode) primary and h-10 / h-11 controls.
func (t *Theme) shadcn() {
	src := t.Base
	if src.Shadcn == nil {
		if t.Dark {
			src = theme.Shadcn(theme.Dark())
		} else {
			src = theme.Shadcn()
		}
	}
	v := src.Shadcn

	t.Primary, t.OnPrimary = v.Primary, v.PrimaryForeground
	t.Success, t.Warning, t.Danger = src.Success.Base, src.Warning.Base, v.Destructive
	t.PrimaryLight = v.Accent
	if v.Options.Primary.A > 0 {
		t.PrimaryLight = theme.Fade(v.Primary, 0.12)
	}
	t.OnSolid = theme.Hex("#ffffff")

	t.Text, t.TextSecondary, t.TextWeak, t.TextLight = v.Foreground, src.TextSecondary, v.MutedForeground, src.TextDisabled
	t.Border = v.Border
	// Light: white cards on a zinc-50-like page. Dark: the page is the
	// background and cards are lifted half way to muted (shadcn v4 cards).
	t.PageBg, t.Box = theme.Mix(v.Background, v.Muted, 0.5), v.Card
	if t.Dark {
		t.PageBg, t.Box = v.Background, theme.Mix(v.Background, v.Muted, 0.5)
	}
	t.Fill = v.Muted
	t.Press = theme.Fade(v.Foreground, 0.06)
	t.Mask = theme.RGBA(0, 0, 0, .5)
	t.ToastBg, t.ToastText, t.ToastBorder = v.Popover, v.PopoverForeground, v.Border
	if t.Dark {
		t.ToastBg = t.Box
	}
	t.NoticeBg, t.NoticeText, t.NoticeBorder = theme.Mix(t.Box, v.Muted, 0.6), v.Foreground, v.Border
	t.BadgeColor = t.Danger
	t.CardBorder = v.Border
	t.DrawerHandle = v.Border
	t.Ring = src.FocusOutline
	t.SegmentThumb = v.Background
	if t.Dark {
		t.SegmentThumb = theme.Mix(v.Muted, v.Foreground, 0.12)
	}

	// shadcn Switch (h-5 w-9) scaled to a thumb-sized 28×48.
	t.SwitchW, t.SwitchH = 48, 28
	t.SwitchOff = v.Input
	t.SwitchThumb, t.SwitchThumbOn = v.Background, v.Background
	if t.Dark {
		t.SwitchThumb, t.SwitchThumbOn = v.Foreground, v.PrimaryForeground
	}

	t.FontTiny, t.FontSmall, t.FontMain, t.FontBody, t.FontTitle, t.FontLarge = 11, 13, 14, 16, 17, 20
	t.TitleWeight = text.Semibold
	t.ButtonWeight = text.Medium
	t.PadH = 16
	t.Radius, t.RadiusLarge, t.PopupRad = src.Radius, src.RadiusLG+4, src.RadiusLG+4
	t.NavBarHeight, t.TabBarHeight = 52, 56
	t.CellPadV, t.CellMinHeight = 12, 52
	t.ButtonHeight = [4]float32{32, 36, 44, 48} // h-8, h-9, h-11, h-12
	t.ButtonFont = [4]float32{12, 14, 15, 16}
	t.ButtonRadius = src.Radius
	t.PickerItemHeight, t.PickerVisible = 40, 5
	t.TabsHeight, t.TabsLineWidth, t.TabsLineHeight = 52, 0, 2
	t.DialogWidth = 328
	t.ActionHeight = 52
	t.ToastPad = 16
	t.SearchHeight, t.SearchRadius = 40, src.RadiusLG
	t.StepperHeight, t.StepperInputW = 40, 48
	t.FormLabelWidth = 96
}

// onColor returns the text color on a solid fill of status name.
func (t *Theme) onColor(name string) theme.Color {
	switch name {
	case "primary", "info", "processing":
		return t.OnPrimary
	}
	return t.OnSolid
}
