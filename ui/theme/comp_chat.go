package theme

import "github.com/tangoui/tango/ui/paint"

// ChatToken styles the AI chat components: Bubble, BubbleList, Sender,
// ThoughtChain, ToolCall, Approval, Conversations and Prompts.
type ChatToken struct {
	Alias

	// Bubble.
	BubbleRadius       float32
	BubblePaddingBlock float32
	BubblePaddingInl   float32
	BubbleMaxWidth     float32 // fraction of the row a filled bubble may take (0..1)
	BubbleGap          float32 // avatar to bubble
	AvatarSize         float32
	UserBg             Color // filled user bubble
	UserColor          Color `contrast:"text:UserBg"`
	AssistantBg        Color // filled assistant bubble (Variant BubbleFilled)
	AssistantColor     Color `contrast:"text:AssistantBg"`
	OutlinedBorder     Color // Variant BubbleOutlined
	ShadowBubble       []paint.Shadow
	NameColor          Color `contrast:"text:@Page"` // sender name above a bubble
	MetaColor          Color `contrast:"text:@Page"` // footer text (time, model)
	SystemBg           Color // system notice pill (zero: none)
	SystemColor        Color `contrast:"text:SystemBg"`
	CaretColor         Color // typing caret at the end of a streaming bubble
	DotColor           Color // loading dots
	AssistantAvatarBg  Color
	AssistantAvatarFg  Color `contrast:"nontext:AssistantAvatarBg"`
	ActionColor        Color `contrast:"nontext:@Page"` // copy / retry / edit icons
	ScrollButtonBg     Color // "back to bottom" button of BubbleList
	ScrollButtonBorder Color // zero: none
	ScrollButtonShadow []paint.Shadow

	// Sender.
	SenderBg          Color
	SenderBorder      Color
	SenderFocusBorder Color
	SenderRadius      float32
	SenderPadding     float32
	SenderShadow      []paint.Shadow // zero: none
	SendSize          float32        // send / stop button
	AttachmentBg      Color
	AttachmentBorder  Color // zero: none
	AttachmentRadius  float32
	AttachmentThumb   float32 // image attachment thumbnail size

	// ThoughtChain / ToolCall.
	CardBg          Color
	CardBorder      Color
	CardRadius      float32
	CardHeaderHover Color
	CodeBg          Color // arguments / result blocks
	LineColor       Color // rail between the steps of a chain
	StepTitleColor  Color `contrast:"text:@Page"`
	StepDescColor   Color `contrast:"text:@Page"`
	DurationColor   Color `contrast:"text:@Page"`
	PendingColor    Color `contrast:"nontext:@Page"` // icon of a step that has not started

	// Approval.
	ApprovalBg     Color
	ApprovalBorder Color
	ApprovalIcon   Color `contrast:"nontext:ApprovalBg"`
	DangerBg       Color // risk "error"
	DangerBorder   Color
	DangerIcon     Color `contrast:"nontext:DangerBg"`

	// Conversations.
	ConvItemHeight  float32
	ConvItemRadius  float32
	ConvHoverBg     Color
	ConvActiveBg    Color
	ConvActiveColor Color `contrast:"text:ConvActiveBg"`
	ConvGroupColor  Color `contrast:"text:@Page"`

	// Prompts.
	PromptBg        Color
	PromptBorder    Color // zero: none
	PromptHoverBg   Color
	PromptRadius    float32
	PromptIconColor Color `contrast:"nontext:PromptBg"`
	PromptDescColor Color `contrast:"text:PromptBg"`
}

func deriveChat(t *Theme) ChatToken {
	k := ChatToken{
		BubbleRadius: t.RadiusLG, BubblePaddingBlock: 8, BubblePaddingInl: 12,
		BubbleMaxWidth: 0.8, BubbleGap: 12, AvatarSize: 32,
		UserBg: t.Primary.Bg, UserColor: t.Text,
		AssistantBg: t.FillTertiary, AssistantColor: t.Text,
		OutlinedBorder: t.BorderSecondary, ShadowBubble: t.ShadowTertiary,
		NameColor: t.TextSecondary, MetaColor: t.TextTertiary,
		SystemBg: t.FillQuaternary, SystemColor: t.TextSecondary,
		CaretColor: t.Primary.Base, DotColor: t.Primary.Base,
		AssistantAvatarBg: t.Primary.Bg, AssistantAvatarFg: t.Primary.Base,
		ActionColor:    t.TextTertiary,
		ScrollButtonBg: t.BgElevated, ScrollButtonBorder: t.BorderSecondary, ScrollButtonShadow: t.Shadow,

		SenderBg: t.BgContainer, SenderBorder: t.Border, SenderFocusBorder: t.Primary.Base,
		SenderRadius: t.RadiusLG + 4, SenderPadding: 10, SenderShadow: t.ShadowTertiary, SendSize: 32,
		AttachmentBg: t.FillQuaternary, AttachmentBorder: t.BorderSecondary,
		AttachmentRadius: t.Radius, AttachmentThumb: 40,

		CardBg: t.BgContainer, CardBorder: t.BorderSecondary, CardRadius: t.RadiusLG,
		CardHeaderHover: t.FillQuaternary, CodeBg: t.FillQuaternary, LineColor: t.Split,
		StepTitleColor: t.Text, StepDescColor: t.TextSecondary, DurationColor: t.TextTertiary,
		PendingColor: t.TextQuaternary,

		ApprovalBg: t.Warning.Bg, ApprovalBorder: t.Warning.Border, ApprovalIcon: t.Warning.Base,
		DangerBg: t.Error.Bg, DangerBorder: t.Error.Border, DangerIcon: t.Error.Base,

		ConvItemHeight: 40, ConvItemRadius: t.Radius,
		ConvHoverBg: t.ItemHoverBg, ConvActiveBg: t.ItemSelectedBg, ConvActiveColor: t.ItemSelected,
		ConvGroupColor: t.TextTertiary,

		PromptBg: t.BgContainer, PromptBorder: t.BorderSecondary, PromptHoverBg: t.FillQuaternary,
		PromptRadius: t.RadiusLG, PromptIconColor: t.Primary.Base, PromptDescColor: t.TextTertiary,
	}
	switch t.Flavor {
	case Element:
		k.BubbleRadius, k.CardRadius, k.PromptRadius = t.Radius, t.Radius, t.Radius
		k.SenderRadius = t.Radius + 4
		k.SenderShadow = nil
		k.CardBorder, k.PromptBorder = t.BorderSecondary, t.BorderSecondary
		k.ConvItemRadius = t.RadiusSM
	case StyleShadcn:
		// shadcn chat blocks: user messages bg-primary text-primary-
		// foreground rounded-lg px-3 py-2; cards rounded-xl border; the
		// input is a rounded-xl bordered textarea with shadow-xs.
		k.UserBg, k.UserColor = t.Primary.Base, t.TextOnPrimary
		k.AssistantBg = t.FillSecondary
		k.OutlinedBorder = t.Border
		k.AssistantAvatarBg, k.AssistantAvatarFg = t.FillSecondary, t.Text
		k.CaretColor, k.DotColor = t.Text, t.TextTertiary
		k.SenderBorder, k.SenderFocusBorder = t.Border, t.FocusOutline
		k.SenderShadow = t.InputShadow
		k.SenderRadius = t.RadiusLG + 2
		k.CardBorder, k.PromptBorder, k.AttachmentBorder = t.Border, t.Border, t.Border
		k.ScrollButtonBorder = t.Border
		k.CardRadius, k.PromptRadius = t.RadiusLG+2, t.RadiusLG+2
		k.ApprovalBg, k.ApprovalBorder, k.ApprovalIcon = t.BgContainer, t.Border, t.Warning.Base
		k.DangerBg, k.DangerBorder = t.BgContainer, t.Error.Base
		k.ConvActiveBg, k.ConvActiveColor = t.FillSecondary, t.Text
		k.ConvHoverBg = Fade(t.FillSecondary, .6)
		k.PromptIconColor = t.Text
	}
	return k
}
