package theme

import "github.com/tangoui/tango/ui/paint"

// CommandToken styles Command (shadcn command.tsx): the palette box, the
// search row, group headings, items and the empty state.
type CommandToken struct {
	Alias
	Bg          Color
	BorderColor Color // zero: none
	Radius      float32
	Shadow      []paint.Shadow // Shadow(): the floating look

	// Search row (CommandInput wrapper): height, padding at both ends,
	// the search icon and the rule under the row.
	InputHeight        float32
	InputPaddingInline float32
	InputIconSize      float32
	InputIconColor     Color
	InputBorderColor   Color

	ListMaxHeight float32 // default MaxHeight
	ListPadding   float32 // around the groups (CommandGroup p-1)

	GroupTitleHeight     float32
	GroupTitlePadding    float32 // inline padding of a heading
	GroupTitleFontSize   float32
	GroupTitleFontWeight int
	GroupTitleColor      Color `contrast:"text:Bg"`
	SeparatorColor       Color

	ItemHeight        float32
	ItemPaddingInline float32
	ItemGap           float32 // between icon and label
	ItemRadius        float32
	ItemColor         Color `contrast:"text:Bg"`
	ItemActiveBg      Color
	ItemActiveColor   Color `contrast:"text:ItemActiveBg"`
	ItemIconSize      float32
	ItemIconColor     Color
	DisabledOpacity   float32

	ShortcutColor         Color `contrast:"text:Bg"`
	ShortcutFontSize      float32
	ShortcutLetterSpacing float32

	EmptyPaddingBlock float32
	EmptyColor        Color `contrast:"text:Bg"`
}

func deriveCommand(t *Theme) CommandToken {
	// shadcn new-york-v4 command.tsx (2026-11-15): Command rounded-md
	// bg-popover; CommandInput wrapper h-9 gap-2 border-b px-3 with a size-4
	// opacity-50 SearchIcon; CommandList max-h-[300px]; CommandGroup p-1,
	// heading px-2 py-1.5 text-xs font-medium text-muted-foreground;
	// CommandItem gap-2 rounded-sm px-2 py-1.5 text-sm, selected bg-accent
	// text-accent-foreground, disabled opacity-50, size-4 muted icons;
	// CommandShortcut text-xs tracking-widest text-muted-foreground;
	// CommandEmpty py-6 text-center text-sm; CommandSeparator h-px bg-border.
	// Ant Design and Element Plus have no command palette: they keep the
	// previous look (44px search row, bordered RadiusLG box).
	k := CommandToken{
		Bg: t.BgElevated, BorderColor: t.Split, Radius: t.RadiusLG, Shadow: t.Shadow,
		InputHeight: 44, InputPaddingInline: 12, InputIconSize: 16, InputIconColor: Fade(t.Text, .5), InputBorderColor: t.Split,
		ListMaxHeight: 300, ListPadding: 4,
		GroupTitleHeight: 28, GroupTitlePadding: 8, GroupTitleFontSize: t.FontSizeSM, GroupTitleFontWeight: 500,
		GroupTitleColor: t.TextTertiary, SeparatorColor: t.Split,
		ItemHeight: 32, ItemPaddingInline: 8, ItemGap: 8, ItemRadius: t.RadiusSM,
		ItemColor: t.Text, ItemActiveBg: t.ItemHoverBg, ItemActiveColor: t.ItemSelected,
		ItemIconSize: 16, ItemIconColor: t.TextTertiary, DisabledOpacity: .5,
		ShortcutColor: t.TextTertiary, ShortcutFontSize: t.FontSizeSM,
		EmptyPaddingBlock: 24, EmptyColor: t.Text,
	}
	if t.Flavor == StyleShadcn {
		k.BorderColor, k.Radius = t.Border, t.Radius
		k.InputHeight, k.InputBorderColor, k.SeparatorColor = 36, t.Border, t.Border
		k.ShortcutLetterSpacing = t.FontSizeSM / 10
	}
	return k
}
