package theme

import (
	"math"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// flatOnly strips the component tokens so two themes can be compared on
// their global (flat) tokens.
func flatOnly(t *Theme) Theme {
	cp := *t
	cp.Components, cp.comp = Components{}, nil
	return cp
}

type ctor struct {
	name string
	mk   func(...Option) *Theme
}

var ctors = []ctor{
	{"antd", AntDesign},
	{"element", ElementPlus},
	{"shadcn", func(o ...Option) *Theme {
		args := make([]ShadcnArg, len(o))
		for i, f := range o {
			args[i] = f
		}
		return Shadcn(args...)
	}},
}

func TestComponentOrder(t *testing.T) {
	// A token field must not shadow a promoted Alias field.
	for i := range int(numComps) {
		ft := componentsType.Field(i).Type
		for j := range aliasType.NumField() {
			if f, _ := ft.FieldByName(aliasType.Field(j).Name); len(f.Index) != 2 {
				t.Errorf("%s.%s shadows the Alias field", ft.Name(), aliasType.Field(j).Name)
			}
		}
	}
	if componentsType.NumField() != int(numComps) {
		t.Fatalf("Components has %d fields, %d Comp values", componentsType.NumField(), int(numComps))
	}
	want := []string{"Button", "Input", "Select", "Checkbox", "Radio", "Switch", "Slider", "Tag", "Tabs", "Menu", "Table",
		"Card", "Modal", "Drawer", "Tooltip", "Popover", "Notification", "Message", "Pagination", "Segmented", "Avatar", "Skeleton", "Layout",
		"Tree", "Cascader", "DatePicker", "Calendar", "Steps", "Timeline", "Descriptions", "List", "Badge", "Progress", "Upload", "Transfer", "Collapse", "Alert", "Breadcrumb", "Dropdown", "Rate", "InputNumber", "Statistic", "Divider", "Typography", "Image", "Carousel", "ColorPicker", "Tour", "Splitter", "Result", "Empty", "FloatButton", "Anchor", "Form", "Menubar", "Item", "Terminal", "CodeBlock", "JSONView", "LogView", "StatusBar", "TitleBar", "NotificationCenter", "HotkeyInput", "Dock", "Chat", "FooterToolbar"}
	for i, n := range want {
		if Comp(i).String() != n {
			t.Errorf("Comp(%d) = %s, want %s", i, Comp(i), n)
		}
		f := componentsType.Field(i)
		if f.Type.Name() != n+"Token" || f.Type.Field(0).Type != aliasType || !f.Type.Field(0).Anonymous {
			t.Errorf("%s: want %sToken embedding Alias first", f.Name, n)
		}
	}
	if CompLayout.String() != "Layout" || Comp(200).String() != "Comp(?)" {
		t.Error("Comp.String")
	}
}

// TestZeroValueInheritance: component tokens leave the global tokens alone
// (with or without overrides that have no Alias) and come from them.
func TestZeroValueInheritance(t *testing.T) {
	over := Components{
		Button: ButtonToken{PaddingInline: 20, DefaultBg: Hex("#fafafa")},
		Table:  TableToken{HeaderBg: Hex("#f0f5ff"), CellPaddingBlock: 12},
	}
	for _, c := range ctors {
		for _, dark := range []bool{false, true} {
			var opts []Option
			if dark {
				opts = append(opts, Dark())
			}
			a := c.mk(opts...)
			b := c.mk(append(opts, WithComponents(over))...)
			if !reflect.DeepEqual(flatOnly(a), flatOnly(b)) {
				t.Errorf("%s dark=%v: overrides changed the global tokens", c.name, dark)
			}
			// Re-deriving with no overrides is idempotent and touches no flat field.
			cp := *a
			cp.deriveComponents(Components{})
			if !reflect.DeepEqual(cp.Components, a.Components) || !reflect.DeepEqual(flatOnly(&cp), flatOnly(a)) {
				t.Errorf("%s: deriveComponents(Components{}) not idempotent", c.name)
			}
			k := a.Components
			if k.Button.PaddingInline != a.ButtonPaddingH || k.Button.FontWeight != a.ButtonFontWeight ||
				k.Switch.TrackHeight != a.SwitchHeight || k.Checkbox.ControlInteractiveSize != a.CheckboxSize ||
				k.Table.HeaderBg != a.TableHeaderBg || k.Table.CellPaddingBlock != a.TablePaddingV ||
				k.Menu.ItemHeight != a.MenuItemHeight || k.Segmented.TrackBg != a.SegmentedBg ||
				k.Slider.RailSize != a.SliderRailHeight || k.Layout.SiderWidth != a.SiderWidth {
				t.Errorf("%s: defaults not derived from the flat tokens", c.name)
			}
			if k.Button.Alias != aliasOf(a) || k.Layout.Alias != aliasOf(a) {
				t.Errorf("%s: resolved Alias", c.name)
			}
			for i := range int(numComps) {
				if a.For(Comp(i)) != a {
					t.Errorf("%s: For(%s) without Alias must return the theme", c.name, Comp(i))
				}
			}
			kb := b.Components
			if kb.Button.PaddingInline != 20 || kb.Button.DefaultBg != Hex("#fafafa") || kb.Table.HeaderBg != Hex("#f0f5ff") ||
				kb.Table.CellPaddingBlock != 12 || kb.Button.PaddingInlineSM != k.Button.PaddingInlineSM || kb.Table.HeaderColor != k.Table.HeaderColor {
				t.Errorf("%s: overrides not merged", c.name)
			}
			if !reflect.DeepEqual(b.Overrides(), over) || !reflect.DeepEqual(a.Overrides(), Components{}) {
				t.Errorf("%s: Overrides", c.name)
			}
		}
	}
}

func TestSentinels(t *testing.T) {
	if !math.Signbit(float64(ZeroPx)) || ZeroPx != 0 {
		t.Fatal("ZeroPx must be negative zero")
	}
	if Transparent == (Color{}) || Transparent.A != 0 || NoShadow == nil || len(NoShadow) != 0 || Instant <= 0 {
		t.Fatal("sentinels must differ from zero values")
	}
	th := AntDesign(WithComponents(Components{
		Button: ButtonToken{PaddingInline: ZeroPx, DefaultBg: Transparent, PrimaryShadow: NoShadow, Alias: Alias{BorderRadius: ZeroPx}},
		Card:   CardToken{BodyPadding: ZeroPx},
	}))
	b := th.Components.Button
	if b.PaddingInline != 0 || math.Signbit(float64(b.PaddingInline)) {
		t.Errorf("ZeroPx resolves to +0, got %v", b.PaddingInline)
	}
	if b.DefaultBg != Transparent || b.PrimaryShadow == nil || len(b.PrimaryShadow) != 0 {
		t.Error("Transparent / NoShadow not kept")
	}
	if th.Components.Card.BodyPadding != 0 {
		t.Error("Card ZeroPx")
	}
	// The zero-radius Alias gives Button its own theme.
	if bt := th.For(CompButton); bt == th || bt.Radius != 0 || b.BorderRadius != 0 || th.Radius != 6 {
		t.Errorf("ZeroPx alias radius: theme %v, token %v, global %v", bt.Radius, b.BorderRadius, th.Radius)
	}
	// Plain zeros inherit.
	plain := AntDesign(WithComponents(Components{Button: ButtonToken{PaddingInline: 0}}))
	if plain.Components.Button.PaddingInline != 15 || len(plain.Components.Button.PrimaryShadow) == 0 || plain.For(CompButton) != plain {
		t.Error("zero values must inherit")
	}
	// Layered options keep the sentinel until resolution.
	lay := AntDesign(WithComponents(Components{Button: ButtonToken{PaddingInline: ZeroPx}}), WithComponents(Components{Button: ButtonToken{FontWeight: 600}}))
	if lay.Components.Button.PaddingInline != 0 || lay.Components.Button.FontWeight != 600 {
		t.Error("layered WithComponents")
	}
	if u := lay.Overrides().Button.PaddingInline; !math.Signbit(float64(u)) {
		t.Error("stored override lost the sentinel")
	}
	if (Components{Button: ButtonToken{PaddingInline: ZeroPx}}).Hash() == (Components{}).Hash() {
		t.Error("Hash must tell ZeroPx from zero")
	}
	if (Components{Button: ButtonToken{PrimaryShadow: NoShadow}}).Hash() == (Components{}).Hash() {
		t.Error("Hash must tell NoShadow from nil")
	}
}

// TestAliasRederive: a component ColorPrimary regenerates the palette with
// each design system's own algorithm, only for that component.
func TestAliasRederive(t *testing.T) {
	green := Hex("#00b96b")
	for _, c := range ctors {
		for _, dark := range []bool{false, true} {
			var opts []Option
			if dark {
				opts = append(opts, Dark())
			}
			th := c.mk(append(opts, WithComponents(Components{Button: ButtonToken{Alias: Alias{ColorPrimary: green, ControlHeight: 40}}}))...)
			ref := c.mk(append(opts, WithPrimary(green))...)
			plain := c.mk(opts...)
			bt := th.For(CompButton)
			if bt == th {
				t.Fatalf("%s: no alias theme", c.name)
			}
			if bt.Primary != ref.Primary {
				t.Errorf("%s dark=%v: button palette %s/%s, want %s/%s", c.name, dark,
					HexString(bt.Primary.Base), HexString(bt.Primary.Hover), HexString(ref.Primary.Base), HexString(ref.Primary.Hover))
			}
			for _, f := range []struct {
				n    string
				a, b Color
			}{{"TextLink", bt.TextLink, ref.TextLink}, {"ItemSelected", bt.ItemSelected, ref.ItemSelected},
				{"FocusOutline", bt.FocusOutline, ref.FocusOutline}, {"ControlFocusBorder", bt.ControlFocusBorder, ref.ControlFocusBorder},
				{"TextOnPrimary", bt.TextOnPrimary, ref.TextOnPrimary}} {
				if f.a != f.b {
					t.Errorf("%s dark=%v: %s %s, want %s", c.name, dark, f.n, HexString(f.a), HexString(f.b))
				}
			}
			if bt.ControlHeight != 40 || th.ControlHeight == 40 {
				t.Errorf("%s: alias geometry", c.name)
			}
			if th.Primary != plain.Primary || th.For(CompInput) != th {
				t.Errorf("%s: alias leaked into the global theme", c.name)
			}
			bk := th.Components.Button
			if bk.ColorPrimary != bt.Primary.Base || bk.ControlHeight != 40 || bk.Alias != aliasOf(bt) {
				t.Errorf("%s: resolved button Alias", c.name)
			}
			// Hover follows the algorithm (Ant: shade 5 of the new scale).
			if c.name == "antd" && bt.Primary.Hover != AntPalette(green, dark, Hex("#141414")).Hover {
				t.Errorf("antd hover")
			}
			if !reflect.DeepEqual(bt.Components.Button, bk) || bt.For(CompButton) != bt {
				t.Errorf("%s: alias theme components", c.name)
			}
			if th.Shadcn != nil && (bt.Shadcn == th.Shadcn || bt.Shadcn.Primary != green || th.Shadcn.Primary == green) {
				t.Errorf("shadcn vars not copied / synced")
			}
		}
	}
	// ColorError and the neutral aliases.
	th := ElementPlus(WithComponents(Components{Tag: TagToken{Alias: Alias{ColorError: Hex("#cf1322"), ColorText: Hex("#111111"), ColorBgContainer: Hex("#fffbe6")}}}))
	tg := th.For(CompTag)
	if tg.Error != ElementPalette(Hex("#cf1322"), false, Hex("#141414")) || tg.ButtonDangerBg != tg.Error.Base {
		t.Error("ColorError alias")
	}
	if tg.Text != Hex("#111111") || tg.TextHeading != Hex("#111111") || tg.BgContainer != Hex("#fffbe6") || tg.ControlBg != Hex("#fffbe6") {
		t.Error("neutral alias")
	}
	if th.Text == tg.Text || th.Error == tg.Error {
		t.Error("alias leaked")
	}
}

func TestOverrideScoped(t *testing.T) {
	outer := AntDesign(WithComponents(Components{Button: ButtonToken{PaddingInline: 20}}))
	in := outer.Override(Components{Card: CardToken{HeaderBg: Hex("#eeeeee")}, Button: ButtonToken{FontWeight: 600}})
	if in == outer || in.Components.Card.HeaderBg != Hex("#eeeeee") || in.Components.Button.PaddingInline != 20 || in.Components.Button.FontWeight != 600 {
		t.Error("Override must layer on the outer overrides")
	}
	if outer.Components.Card.HeaderBg != (Color{}) || outer.Components.Button.FontWeight == 600 {
		t.Error("Override modified its receiver")
	}
	hc := HighContrast(AntDesign()).Override(Components{Button: ButtonToken{DefaultColor: Hex("#dddddd")}})
	if !hc.HighContrast || len(ContrastFailures(hc)) != 0 {
		t.Error("Override of a high-contrast theme must stay high-contrast")
	}
}

// lowContrast is a set of overrides that fails almost every tagged pair.
func lowContrast() Components {
	pale := Hex("#e8e8e8")
	return Components{
		Button:     ButtonToken{DefaultColor: pale, PrimaryColor: Hex("#4096ff"), SecondaryColor: Hex("#f0f0f0"), DangerColor: Hex("#ff9999")},
		Input:      InputToken{PlaceholderColor: pale, ActiveBorderColor: Hex("#f5f5f5")},
		Checkbox:   CheckboxToken{BorderColor: Hex("#fafafa"), CheckedBg: Hex("#f0f8ff")},
		Switch:     SwitchToken{OffBg: Hex("#f7f7f7")},
		Table:      TableToken{HeaderBg: Hex("#222222"), HeaderColor: Hex("#333333")},
		Tabs:       TabsToken{ItemColor: pale, InkBarColor: Hex("#eef")},
		Menu:       MenuToken{ItemSelectedBg: Hex("#1677ff"), ItemSelectedColor: Hex("#3388ff"), DarkItemColor: Hex("#223344")},
		Tooltip:    TooltipToken{Bg: Hex("#777777"), Color: Hex("#888888")},
		Tag:        TagToken{DefaultBg: Hex("#ffd666"), DefaultColor: Hex("#ffe58f"), Alias: Alias{ColorPrimary: Hex("#ffe58f")}},
		Pagination: PaginationToken{ItemActiveColor: Hex("#bae0ff")},
		Avatar:     AvatarToken{Bg: Hex("#cccccc"), Color: Hex("#ffffff")},
		Layout:     LayoutToken{HeaderBg: Hex("#001529"), HeaderColor: Hex("#002140")},
		Slider:     SliderToken{TrackBg: Hex("#e6f4ff"), HandleColor: Hex("#e6f4ff")},
	}
}

// TestHighContrastComponents: deliberately low-contrast overrides fail in
// the normal theme and pass after HighContrast, in every design system.
func TestHighContrastComponents(t *testing.T) {
	for _, c := range ctors {
		for _, dark := range []bool{false, true} {
			opts := []Option{WithComponents(lowContrast())}
			if dark {
				opts = append(opts, Dark())
			}
			th := c.mk(opts...)
			fails := 0
			for _, f := range ContrastFailures(th) {
				if strings.Contains(f.Token, ".") {
					fails++
					if f.SuggestRatio+1e-9 < f.Kind.Min() {
						t.Errorf("suggestion below minimum: %s", f)
					}
				}
			}
			if fails < 5 {
				t.Errorf("%s dark=%v: only %d component failures reported", c.name, dark, fails)
			}
			hc := HighContrast(th)
			for _, f := range ContrastFailures(hc) {
				t.Errorf("%s dark=%v: %s", c.name, dark, f)
			}
			if n := len(componentPairs(hc, true)); n < 40 {
				t.Errorf("%s: only %d tagged pairs checked", c.name, n)
			}
			// Alias themes go through HighContrast too.
			if tg := hc.For(CompTag); tg == hc || !tg.HighContrast || ContrastRatio(tg.TextOnPrimary, tg.Primary.Base) < 4.5 {
				t.Errorf("%s: alias theme not high-contrast", c.name)
			}
			if !reflect.DeepEqual(hc.For(CompTag).Components.Tag, hc.Components.Tag) {
				t.Errorf("%s: alias theme components out of sync", c.name)
			}
			if th.Components.Button.DefaultColor != Hex("#e8e8e8") {
				t.Error("HighContrast modified its argument")
			}
		}
	}
	// Normal themes without overrides report no component pair, except
	// the always-audited (spec-tagged) terminal palette.
	for _, th := range BuiltinThemes() {
		for _, ch := range CheckContrast(th) {
			if strings.Contains(ch.Token, ".") && !strings.HasPrefix(ch.Token, "Terminal.Palette[") {
				t.Errorf("unexpected component pair in a stock theme: %s", ch)
			}
		}
	}
}

func TestComponentCSSVars(t *testing.T) {
	kebabRe := regexp.MustCompile(`^--tango-[a-z0-9]+(-[a-z0-9]+)*$`)
	th := AntDesign()
	plain := th.CSSVars()
	if got := th.CSSVarsWith(CSSOptions{}); !reflect.DeepEqual(got, plain) {
		t.Error("CSSVarsWith without Components must equal CSSVars")
	}
	all := th.CSSVarsWith(CSSOptions{Components: true})
	comp := map[string]string{}
	th.componentVars(comp, nil)
	// Complete: one variable per field (Alias fields included) per
	// component, one per element of an array field (the terminal palette).
	n := 0
	for i := range int(numComps) {
		ft := componentsType.Field(i).Type
		n += ft.NumField() - 1 + aliasType.NumField()
		for j := range ft.NumField() {
			if ft.Field(j).Type.Kind() == reflect.Array {
				n += ft.Field(j).Type.Len() - 1
			}
		}
	}
	if len(comp) != n {
		t.Errorf("%d component variables, want %d", len(comp), n)
	}
	for k, v := range comp {
		if !kebabRe.MatchString(k) {
			t.Errorf("not kebab: %s", k)
		}
		if _, dup := plain[k]; dup {
			t.Errorf("component variable collides with a global one: %s", k)
		}
		if all[k] != v {
			t.Errorf("%s missing from CSSVarsWith", k)
		}
	}
	for k, want := range map[string]string{
		"--tango-button-padding-inline":    "15px",
		"--tango-button-padding-inline-sm": "7px",
		"--tango-button-color-primary":     "#1677ff",
		"--tango-table-header-bg":          "#fafafa",
		"--tango-table-fixed-shadow-alpha": "0.16",
		"--tango-button-font-weight":       "400",
		"--tango-tooltip-shadow":           cssShadow(th.ShadowSecondary),
		"--tango-notification-width":       "384px",
	} {
		if comp[k] != want {
			t.Errorf("%s = %q, want %q", k, comp[k], want)
		}
	}
	if kebab("PaddingInlineSM") != "padding-inline-sm" || kebab("HeaderBg") != "header-bg" || kebab("ControlHeightLG") != "control-height-lg" {
		t.Error("kebab")
	}
	// OnlyChanged: nothing for a theme without overrides.
	if got := th.CSSVarsWith(CSSOptions{Components: true, OnlyChanged: true}); len(got) != len(plain) {
		t.Errorf("OnlyChanged without overrides adds %d variables", len(got)-len(plain))
	}
	ov := AntDesign(WithComponents(Components{Button: ButtonToken{PaddingInline: 20, Alias: Alias{ColorPrimary: Hex("#00b96b")}}, Table: TableToken{HeaderBg: Hex("#f0f5ff")}}))
	ch := ov.CSSVarsWith(CSSOptions{Components: true, OnlyChanged: true})
	extra := map[string]string{}
	for k, v := range ch {
		if _, ok := plain[k]; !ok {
			extra[k] = v
		}
	}
	if extra["--tango-button-padding-inline"] != "20px" || extra["--tango-table-header-bg"] != "#f0f5ff" || extra["--tango-button-color-primary"] != "#00b96b" {
		t.Errorf("OnlyChanged missed overrides: %v", extra)
	}
	if _, ok := extra["--tango-input-padding-inline"]; ok || len(extra) > 30 {
		t.Errorf("OnlyChanged exported unchanged variables (%d)", len(extra))
	}
	// Ref: identical alias tokens point at the global variable.
	ref := th.CSSVarsWith(CSSOptions{Components: true, Ref: true})
	if ref["--tango-button-color-primary"] != "var(--tango-color-primary)" || ref["--tango-input-border-radius"] != "var(--tango-radius)" {
		t.Errorf("Ref: %s, %s", ref["--tango-button-color-primary"], ref["--tango-input-border-radius"])
	}
	if refOv := ov.CSSVarsWith(CSSOptions{Components: true, Ref: true}); refOv["--tango-button-color-primary"] != "#00b96b" {
		t.Error("Ref must not reference a different value")
	}
}

func TestForZeroAlloc(t *testing.T) {
	th := Shadcn(WithComponents(Components{Button: ButtonToken{Alias: Alias{BorderRadius: 0.5}}}))
	var sink *Theme
	if n := testing.AllocsPerRun(1000, func() {
		sink = th.For(CompTable)
		sink = th.For(CompButton)
	}); n != 0 {
		t.Errorf("For allocates %v times", n)
	}
	_ = sink
}

func BenchmarkFor(b *testing.B) {
	th := AntDesign()
	b.ReportAllocs()
	var sink *Theme
	for b.Loop() {
		sink = th.For(CompButton)
	}
	_ = sink
}

func BenchmarkDeriveComponents(b *testing.B) {
	th := AntDesign()
	b.ReportAllocs()
	for b.Loop() {
		cp := *th
		cp.deriveComponents(Components{})
	}
}

// TestAliasFromBrandTheme: the Alias re-derivation also starts from a
// WithPrimary theme (Ant dark blends the seed, which must be recovered).
func TestAliasFromBrandTheme(t *testing.T) {
	green := Hex("#00b96b")
	for _, c := range ctors {
		th := c.mk(Dark(), WithPrimary(Hex("#13c2c2")), WithComponents(Components{Menu: MenuToken{Alias: Alias{ColorPrimary: green}}}))
		ref := c.mk(Dark(), WithPrimary(green))
		m := th.For(CompMenu)
		if m.Primary != ref.Primary || m.TextLink != ref.TextLink || m.FocusOutline != ref.FocusOutline || m.ItemSelectedBg != ref.ItemSelectedBg {
			t.Errorf("%s: brand theme alias %s, want %s", c.name, HexString(m.FocusOutline), HexString(ref.FocusOutline))
		}
	}
}

func TestComponentsMerge(t *testing.T) {
	m := Components{Button: ButtonToken{PaddingInline: ZeroPx}}.Merge(Components{Button: ButtonToken{FontWeight: 600}, Card: CardToken{Radius: 3}})
	if !math.Signbit(float64(m.Button.PaddingInline)) || m.Button.FontWeight != 600 || m.Card.Radius != 3 {
		t.Error("Merge")
	}
}
