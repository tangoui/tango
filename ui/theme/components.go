package theme

import (
	"math"
	"reflect"
	"time"

	"github.com/tangoui/tango/ui/paint"
)

// Component tokens (docs/specs/rtl-and-component-tokens.md, section 4).
//
// Every component with visual values of its own has a token struct
// (ButtonToken, TableToken …) in Theme.Components. Field names follow Ant
// Design's component tokens in PascalCase. The values are derived from the
// global tokens when the theme is built (deriveX in comp_<name>.go, where
// the only per-design-system branches live), then the overrides given with
// WithComponents are merged on top, once; components read the resolved
// values and never merge per frame.
//
// In an override a zero field inherits the derived value. Explicit zeros
// use the sentinels ZeroPx, Transparent, NoShadow and Instant. Resolved
// values follow the flat Theme conventions: a zero Color documented as
// "zero: none" draws nothing.
//
// Each token embeds Alias, a subset of the global tokens. Setting an Alias
// field re-derives the whole theme for that component (a new ColorPrimary
// regenerates the palette with the design system's own algorithm); read the
// result through Theme.For.

// Comp names a component with its own tokens (its field in Components;
// String returns the field name).
//
// Append-only: never reorder or insert; the zero value stays CompButton. A
// new component takes the next value and its token field goes at the end
// of Components.
type Comp uint8

const (
	CompButton Comp = iota
	CompInput
	CompSelect
	CompCheckbox
	CompRadio
	CompSwitch
	CompSlider
	CompTag
	CompTabs
	CompMenu
	CompTable
	CompCard
	CompModal
	CompDrawer
	CompTooltip
	CompPopover
	CompNotification
	CompMessage
	CompPagination
	CompSegmented
	CompAvatar
	CompSkeleton
	CompLayout
	CompTree
	CompCascader
	CompDatePicker
	CompCalendar
	CompSteps
	CompTimeline
	CompDescriptions
	CompList
	CompBadge
	CompProgress
	CompUpload
	CompTransfer
	CompCollapse
	CompAlert
	CompBreadcrumb
	CompDropdown
	CompRate
	CompInputNumber
	CompStatistic
	CompDivider
	CompTypography
	CompImage
	CompCarousel
	CompColorPicker
	CompTour
	CompSplitter
	CompResult
	CompEmpty
	CompFloatButton
	CompAnchor
	CompForm
	CompMenubar
	CompItem
	// numComps counts the Comp values. Internally Comp(i) indexes the
	// i-th field of Components, so both lists only ever grow at the end.
	numComps
)

// Components holds the token struct of every component. New components
// are appended at the end (see Comp).
type Components struct {
	Button       ButtonToken
	Input        InputToken
	Select       SelectToken
	Checkbox     CheckboxToken
	Radio        RadioToken
	Switch       SwitchToken
	Slider       SliderToken
	Tag          TagToken
	Tabs         TabsToken
	Menu         MenuToken
	Table        TableToken
	Card         CardToken
	Modal        ModalToken
	Drawer       DrawerToken
	Tooltip      TooltipToken
	Popover      PopoverToken
	Notification NotificationToken
	Message      MessageToken
	Pagination   PaginationToken
	Segmented    SegmentedToken
	Avatar       AvatarToken
	Skeleton     SkeletonToken
	Layout       LayoutToken
	Tree         TreeToken
	Cascader     CascaderToken
	DatePicker   DatePickerToken
	Calendar     CalendarToken
	Steps        StepsToken
	Timeline     TimelineToken
	Descriptions DescriptionsToken
	List         ListToken
	Badge        BadgeToken
	Progress     ProgressToken
	Upload       UploadToken
	Transfer     TransferToken
	Collapse     CollapseToken
	Alert        AlertToken
	Breadcrumb   BreadcrumbToken
	Dropdown     DropdownToken
	Rate         RateToken
	InputNumber  InputNumberToken
	Statistic    StatisticToken
	Divider      DividerToken
	Typography   TypographyToken
	Image        ImageToken
	Carousel     CarouselToken
	ColorPicker  ColorPickerToken
	Tour         TourToken
	Splitter     SplitterToken
	Result       ResultToken
	Empty        EmptyToken
	FloatButton  FloatButtonToken
	Anchor       AnchorToken
	Form         FormToken
	Menubar      MenubarToken
	Item         ItemToken
}

// Alias is the subset of global tokens a component may override for itself
// (Ant: a component token overriding an alias token). Resolved tokens carry
// the effective values. The ref tags name the global CSS variable the
// field mirrors (CSSOptions.Ref).
type Alias struct {
	ColorPrimary     Color   `ref:"color-primary"`
	ColorError       Color   `ref:"color-error"`
	ColorText        Color   `ref:"color-text"`
	ColorBorder      Color   `ref:"color-border"`
	ColorBgContainer Color   `ref:"color-bg-container"`
	BorderRadius     float32 `ref:"radius"`
	BorderRadiusSM   float32 `ref:"radius-sm"`
	BorderRadiusLG   float32 `ref:"radius-lg"`
	ControlHeight    float32 `ref:"control-height"`
	ControlHeightSM  float32 `ref:"control-height-sm"`
	ControlHeightLG  float32 `ref:"control-height-lg"`
	FontSize         float32 `ref:"font-size"`
	FontSizeSM       float32 `ref:"font-size-sm"`
	FontSizeLG       float32 `ref:"font-size-lg"`
	LineWidth        float32 `ref:"line-width"`
}

// Sentinels for "explicitly zero" in overrides (a plain zero inherits).
var (
	// ZeroPx is a zero length (negative zero; it draws like 0 and resolves
	// to 0).
	ZeroPx = float32(math.Copysign(0, -1))
	// Transparent is an invisible color distinct from the zero Color.
	Transparent = Color{R: 255, G: 255, B: 255, A: 0}
	// NoShadow removes a shadow (a non-nil empty slice; nil inherits).
	NoShadow = []paint.Shadow{}
)

// Instant is a zero-length duration for duration tokens.
const Instant = time.Nanosecond

// WithComponents sets component token overrides. Repeated options layer:
// later non-zero fields win. It is also accepted by Shadcn.
func WithComponents(c Components) Option {
	return func(o *options) { o.components = mergeOverrides(o.components, c) }
}

// compState is the per-theme component bookkeeping: the overrides the
// theme was built with and the themes of components with an Alias.
type compState struct {
	user  Components
	alias [numComps]*Theme
}

// For returns the theme a component reads: t with the component's Alias
// applied, or t itself (no allocation) when the component has none.
func (t *Theme) For(k Comp) *Theme {
	if t.comp != nil && k < numComps {
		if a := t.comp.alias[k]; a != nil {
			return a
		}
	}
	return t
}

// Overrides returns the component overrides t was built with.
func (t *Theme) Overrides() Components {
	if t.comp == nil {
		return Components{}
	}
	return t.comp.user
}

// Override returns a copy of t with c layered over its component overrides
// and every component token re-derived (scoped themes of ConfigProvider).
// A HighContrast theme stays high-contrast. t is not modified.
func (t *Theme) Override(c Components) *Theme {
	return t.withOverrides(mergeOverrides(t.Overrides(), c))
}

// withOverrides rebuilds the component tokens of a copy of t from user,
// replacing t's overrides.
func (t *Theme) withOverrides(user Components) *Theme {
	cp := *t
	n := &cp
	if t.Shadcn != nil {
		v := *t.Shadcn
		n.Shadcn = &v
	}
	n.deriveComponents(user)
	if t.HighContrast {
		n = HighContrast(n)
	}
	return n
}

var (
	colorType      = reflect.TypeFor[Color]()
	aliasType      = reflect.TypeFor[Alias]()
	componentsType = reflect.TypeFor[Components]()
)

// compNames holds the Components field names ("Button" …).
var compNames = func() (n [numComps]string) {
	for i := range n {
		n[i] = componentsType.Field(i).Name
	}
	return n
}()

// String returns the component name ("Button").
func (k Comp) String() string {
	if k < numComps {
		return compNames[k]
	}
	return "Comp(?)"
}

// deriveComponents resolves t.Components: for every component, its Alias
// theme (when user sets one), the defaults derived from that theme, then
// user's non-zero fields. Called last by the theme constructors, and before
// HighContrast.
func (t *Theme) deriveComponents(user Components) {
	st := &compState{user: user}
	var res Components
	rv := reflect.ValueOf(&res).Elem()
	uv := reflect.ValueOf(user)
	for k := range numComps {
		tk := t
		if ua := uv.Field(int(k)).Field(0); !isUnset(ua) {
			tk = t.withAlias(ua.Interface().(Alias))
			st.alias[k] = tk
		}
		dst := rv.Field(int(k))
		dst.Set(reflect.ValueOf(deriveComp(tk, k)))
		overlay(dst, uv.Field(int(k)), false)
		dst.Field(0).Set(reflect.ValueOf(aliasOf(tk)))
	}
	t.Components, t.comp = res, st
	for _, a := range st.alias {
		if a != nil {
			a.Components = res
		}
	}
}

// deriveComp returns the default token of component k for t.
func deriveComp(t *Theme, k Comp) any {
	switch k {
	case CompButton:
		return deriveButton(t)
	case CompInput:
		return deriveInput(t)
	case CompSelect:
		return deriveSelect(t)
	case CompCheckbox:
		return deriveCheckbox(t)
	case CompRadio:
		return deriveRadio(t)
	case CompSwitch:
		return deriveSwitch(t)
	case CompSlider:
		return deriveSlider(t)
	case CompTag:
		return deriveTag(t)
	case CompTabs:
		return deriveTabs(t)
	case CompMenu:
		return deriveMenu(t)
	case CompTable:
		return deriveTable(t)
	case CompCard:
		return deriveCard(t)
	case CompModal:
		return deriveModal(t)
	case CompDrawer:
		return deriveDrawer(t)
	case CompTooltip:
		return deriveTooltip(t)
	case CompPopover:
		return derivePopover(t)
	case CompNotification:
		return deriveNotification(t)
	case CompMessage:
		return deriveMessage(t)
	case CompPagination:
		return derivePagination(t)
	case CompSegmented:
		return deriveSegmented(t)
	case CompAvatar:
		return deriveAvatar(t)
	case CompSkeleton:
		return deriveSkeleton(t)
	case CompLayout:
		return deriveLayout(t)
	case CompTree:
		return deriveTree(t)
	case CompCascader:
		return deriveCascader(t)
	case CompDatePicker:
		return deriveDatePicker(t)
	case CompCalendar:
		return deriveCalendar(t)
	case CompSteps:
		return deriveSteps(t)
	case CompTimeline:
		return deriveTimeline(t)
	case CompDescriptions:
		return deriveDescriptions(t)
	case CompList:
		return deriveList(t)
	case CompBadge:
		return deriveBadge(t)
	case CompProgress:
		return deriveProgress(t)
	case CompUpload:
		return deriveUpload(t)
	case CompTransfer:
		return deriveTransfer(t)
	case CompCollapse:
		return deriveCollapse(t)
	case CompAlert:
		return deriveAlert(t)
	case CompBreadcrumb:
		return deriveBreadcrumb(t)
	case CompDropdown:
		return deriveDropdown(t)
	case CompRate:
		return deriveRate(t)
	case CompInputNumber:
		return deriveInputNumber(t)
	case CompStatistic:
		return deriveStatistic(t)
	case CompDivider:
		return deriveDivider(t)
	case CompTypography:
		return deriveTypography(t)
	case CompImage:
		return deriveImage(t)
	case CompCarousel:
		return deriveCarousel(t)
	case CompColorPicker:
		return deriveColorPicker(t)
	case CompTour:
		return deriveTour(t)
	case CompSplitter:
		return deriveSplitter(t)
	case CompResult:
		return deriveResult(t)
	case CompEmpty:
		return deriveEmpty(t)
	case CompFloatButton:
		return deriveFloatButton(t)
	case CompAnchor:
		return deriveAnchor(t)
	case CompForm:
		return deriveForm(t)
	case CompMenubar:
		return deriveMenubar(t)
	case CompItem:
		return deriveItem(t)
	}
	panic("theme: unknown component")
}

// aliasOf returns the effective Alias values of t.
func aliasOf(t *Theme) Alias {
	return Alias{
		ColorPrimary: t.Primary.Base, ColorError: t.Error.Base, ColorText: t.Text,
		ColorBorder: t.Border, ColorBgContainer: t.BgContainer,
		BorderRadius: t.Radius, BorderRadiusSM: t.RadiusSM, BorderRadiusLG: t.RadiusLG,
		ControlHeight: t.ControlHeight, ControlHeightSM: t.ControlHeightSM, ControlHeightLG: t.ControlHeightLG,
		FontSize: t.FontSize, FontSizeSM: t.FontSizeSM, FontSizeLG: t.FontSizeLG,
		LineWidth: t.LineWidth,
	}
}

// isUnset reports whether every field of an override value inherits (zero
// and not a sentinel).
func isUnset(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == colorType {
			return v.Interface().(Color) == Color{}
		}
		for i := range v.NumField() {
			if !isUnset(v.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		return f == 0 && !math.Signbit(f)
	case reflect.Slice, reflect.Map, reflect.Pointer:
		return v.IsNil()
	}
	return v.IsZero()
}

// overlay copies the set fields of override u onto dst. With keep false
// (resolving) ZeroPx becomes 0; with keep true (layering overrides) the
// sentinel is kept so it still means "explicit zero" when resolved later.
func overlay(dst, u reflect.Value, keep bool) {
	switch u.Kind() {
	case reflect.Struct:
		if u.Type() == colorType {
			if u.Interface().(Color) != (Color{}) {
				dst.Set(u)
			}
			return
		}
		for i := range u.NumField() {
			overlay(dst.Field(i), u.Field(i), keep)
		}
	case reflect.Float32, reflect.Float64:
		f := u.Float()
		switch {
		case f != 0:
			dst.SetFloat(f)
		case math.Signbit(f) && keep:
			dst.Set(u)
		case math.Signbit(f):
			dst.SetFloat(0)
		}
	case reflect.Slice, reflect.Map, reflect.Pointer:
		if !u.IsNil() {
			dst.Set(u)
		}
	default:
		if !u.IsZero() {
			dst.Set(u)
		}
	}
}

// Merge returns c with the set fields of o layered on top (sentinels kept),
// e.g. for repeated ConfigProvider.Components calls.
func (c Components) Merge(o Components) Components { return mergeOverrides(c, o) }

// mergeOverrides layers override b over a.
func mergeOverrides(a, b Components) Components {
	overlay(reflect.ValueOf(&a).Elem(), reflect.ValueOf(b), true)
	return a
}

// setLen applies an Alias length: unset keeps *dst, ZeroPx gives 0.
func setLen(dst *float32, v float32) {
	if v != 0 {
		*dst = v
	} else if math.Signbit(float64(v)) {
		*dst = 0
	}
}

// withAlias returns a copy of t with the Alias a applied. A new
// ColorPrimary rebuilds every primary-derived token with the design
// system's algorithm (palette, links, selection, focus rings, shadcn
// foregrounds); ColorError regenerates the error palette and the tokens
// copied from it; the neutral colors replace the tokens that carry the same
// value; lengths replace their global token.
func (t *Theme) withAlias(a Alias) *Theme {
	cp := *t
	n := &cp
	n.comp, n.Components = nil, Components{}
	if t.Shadcn != nil {
		v := *t.Shadcn
		n.Shadcn = &v
	}
	if a.ColorPrimary != (Color{}) && a.ColorPrimary != t.Primary.Base {
		n.adoptPrimary(a.ColorPrimary)
	}
	if a.ColorError != (Color{}) && a.ColorError != t.Error.Base {
		old := n.Error
		n.Error = n.flavorPalette(a.ColorError)
		moves := paletteLeafMoves(old, n.Error)
		walkColors(reflect.ValueOf(n).Elem(), func(name string, c *Color) {
			if name == "TextOnPrimary" || name == "TextOnSolid" {
				return
			}
			if m, ok := moves[*c]; ok {
				*c = m
			}
		})
		if v := n.Shadcn; v != nil && v.Destructive == old.Base {
			v.Destructive = n.Error.Base
		}
	}
	follow := func(old, nw Color, fields ...*Color) {
		if nw == (Color{}) || nw == old {
			return
		}
		for _, f := range fields {
			if *f == old {
				*f = nw
			}
		}
	}
	var sv ShadcnVars
	if n.Shadcn != nil {
		sv = *n.Shadcn
	}
	follow(t.Text, a.ColorText, &n.Text, &n.TextHeading, &n.SecondaryText, &n.AddonText, &n.TableHeaderText, &n.ItemSelected, &n.MarkText,
		&sv.Foreground, &sv.CardForeground, &sv.PopoverForeground, &sv.AccentForeground, &sv.SecondaryForeground)
	follow(t.Border, a.ColorBorder, &n.Border, &n.SwitchOffBg, &n.SwitchOffBgHover, &n.RateStarEmpty, &sv.Input)
	follow(t.BgContainer, a.ColorBgContainer, &n.BgContainer, &n.BgElevated, &n.ControlBg, &n.ControlBgHover, &n.HeaderBg,
		&n.SiderLightBg, &n.TableHeaderBg, &n.SegmentedSelectedBg, &sv.Background, &sv.Card, &sv.Popover)
	if n.Shadcn != nil {
		*n.Shadcn = sv
	}
	setLen(&n.Radius, a.BorderRadius)
	setLen(&n.RadiusSM, a.BorderRadiusSM)
	setLen(&n.RadiusLG, a.BorderRadiusLG)
	setLen(&n.ControlHeight, a.ControlHeight)
	setLen(&n.ControlHeightSM, a.ControlHeightSM)
	setLen(&n.ControlHeightLG, a.ControlHeightLG)
	setLen(&n.FontSize, a.FontSize)
	setLen(&n.FontSizeSM, a.FontSizeSM)
	setLen(&n.FontSizeLG, a.FontSizeLG)
	setLen(&n.LineWidth, a.LineWidth)
	return n
}

// flavorPalette derives a semantic palette from c with the algorithm of t's
// design system.
func (t *Theme) flavorPalette(c Color) Palette {
	bgDark := Hex("#141414")
	switch t.Flavor {
	case Element:
		return ElementPalette(c, t.Dark, bgDark)
	case StyleShadcn:
		bg := t.BgContainer
		if t.Shadcn != nil {
			bg = t.Shadcn.Background
		}
		return shadcnPalette(c, bg, t.Dark)
	}
	return AntPalette(c, t.Dark, bgDark)
}

// stockTheme builds the stock theme of t's design system (same dark mode,
// shadcn base and radius) with the given brand color; nil keeps the
// design system's own (shadcn: the neutral primary).
func (t *Theme) stockTheme(primary *Color) *Theme {
	var opts []Option
	if t.Dark {
		opts = append(opts, Dark())
	}
	if primary != nil {
		opts = append(opts, WithPrimary(*primary))
	}
	switch t.Flavor {
	case Element:
		return ElementPlus(opts...)
	case StyleShadcn:
		o := ShadcnOptions{Dark: t.Dark}
		if t.Shadcn != nil {
			o.Base, o.Radius = t.Shadcn.Options.Base, t.Shadcn.Options.Radius
		}
		if primary != nil {
			o.Primary = *primary
		}
		return Shadcn(o)
	}
	return AntDesign(opts...)
}

// adoptPrimary moves t to brand color c the way the constructor would have
// derived it: every color that equals the stock theme built with t's
// primary takes the value of the stock theme built with c. Colors the app
// set itself (no match) are kept.
func (t *Theme) adoptPrimary(c Color) {
	old, nw := t.stockTheme(t.primarySeed()), t.stockTheme(&c)
	adopt := func(dst, o, n reflect.Value) {
		var walk func(d, o, n reflect.Value)
		walk = func(d, o, n reflect.Value) {
			switch {
			case d.Type() == colorType:
				if d.Interface() == o.Interface() && o.Interface() != n.Interface() {
					d.Set(n)
				}
			case d.Kind() == reflect.Struct:
				for i := range d.NumField() {
					if d.Type().Field(i).IsExported() && d.Type().Field(i).Name != "Components" {
						walk(d.Field(i), o.Field(i), n.Field(i))
					}
				}
			case d.Kind() == reflect.Array:
				for i := range d.Len() {
					walk(d.Index(i), o.Index(i), n.Index(i))
				}
			}
		}
		walk(dst, o, n)
	}
	adopt(reflect.ValueOf(t).Elem(), reflect.ValueOf(old).Elem(), reflect.ValueOf(nw).Elem())
	if t.Shadcn != nil && old.Shadcn != nil && nw.Shadcn != nil {
		adopt(reflect.ValueOf(t.Shadcn).Elem(), reflect.ValueOf(old.Shadcn).Elem(), reflect.ValueOf(nw.Shadcn).Elem())
		t.Shadcn.Options.Primary = c
	}
	// The palette itself always follows, even when the app replaced it.
	if t.Primary.Base != c {
		infoIsPrimary := t.Info == t.Primary
		t.Primary = nw.Primary
		if infoIsPrimary {
			t.Info = t.Primary
		}
	}
}

// primarySeed returns the brand color t was built from (nil: the design
// system's own), so that stockTheme reproduces t's primary-derived tokens.
// Light themes, Element and shadcn keep the seed as Primary.Base; Ant's
// dark algorithm blends it onto the page (85%), so the seed is recovered
// by inverting the blend and checking the neighbors of the estimate.
func (t *Theme) primarySeed() *Color {
	if t.Flavor == StyleShadcn {
		if t.Shadcn != nil && t.Shadcn.Options.Primary.A > 0 {
			p := t.Shadcn.Options.Primary
			return &p
		}
		return nil
	}
	if t.stockTheme(nil).Primary == t.Primary {
		return nil
	}
	p := t.Primary.Base
	if t.Flavor != Ant || !t.Dark {
		return &p
	}
	bg := Hex("#141414")
	inv := func(d, b uint8) float64 { return (float64(d) - 0.15*float64(b)) / 0.85 }
	est := [3]float64{inv(p.R, bg.R), inv(p.G, bg.G), inv(p.B, bg.B)}
	ch := func(v float64, d int) uint8 { return uint8(max(0, min(255, math.Round(v)+float64(d)))) }
	for dr := -1; dr <= 1; dr++ {
		for dg := -1; dg <= 1; dg++ {
			for db := -1; db <= 1; db++ {
				s := Color{R: ch(est[0], dr), G: ch(est[1], dg), B: ch(est[2], db), A: 255}
				if AntPalette(s, true, bg).Base == p {
					return &s
				}
			}
		}
	}
	return &p
}

// paletteLeafMoves maps every color of old to the color at the same place
// in q.
func paletteLeafMoves(old, q Palette) map[Color]Color {
	m := map[Color]Color{}
	o, n := reflect.ValueOf(old), reflect.ValueOf(q)
	var walk func(o, n reflect.Value)
	walk = func(o, n reflect.Value) {
		switch {
		case o.Type() == colorType:
			if a, b := o.Interface().(Color), n.Interface().(Color); a != b {
				m[a] = b
			}
		case o.Kind() == reflect.Struct:
			for i := range o.NumField() {
				walk(o.Field(i), n.Field(i))
			}
		case o.Kind() == reflect.Array:
			for i := range o.Len() {
				walk(o.Index(i), n.Index(i))
			}
		}
	}
	walk(o, n)
	return m
}

// walkColors calls f for every top-level Color field of the struct v (an
// addressable value), with the field name.
func walkColors(v reflect.Value, f func(name string, c *Color)) {
	for i := range v.NumField() {
		sf := v.Type().Field(i)
		if sf.Type == colorType && sf.IsExported() {
			f(sf.Name, v.Field(i).Addr().Interface().(*Color))
		}
	}
}

// Hash returns a hash of the overrides (sentinels included), for caches of
// derived themes keyed by override set.
func (c Components) Hash() uint64 {
	// Inline FNV-1a over 64-bit words: no hasher allocation, so a
	// ConfigProvider that re-hashes its overrides every frame stays
	// allocation-free.
	const prime = 1099511628211
	h := uint64(14695981039346656037)
	put := func(x uint64) {
		for i := 0; i < 8; i++ {
			h ^= x & 0xff
			h *= prime
			x >>= 8
		}
	}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i))
			}
		case reflect.Float32, reflect.Float64:
			put(math.Float64bits(v.Float()))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			put(uint64(v.Int()))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			put(v.Uint())
		case reflect.Bool:
			if v.Bool() {
				put(1)
			} else {
				put(0)
			}
		case reflect.String:
			str := v.String()
			put(uint64(len(str)))
			for i := 0; i < len(str); i++ {
				h ^= uint64(str[i])
				h *= prime
			}
		case reflect.Slice:
			if v.IsNil() {
				put(math.MaxUint64)
				return
			}
			put(uint64(v.Len()))
			for i := range v.Len() {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(&c).Elem())
	return h
}

// shadcnComponents collects the WithComponents overrides among Shadcn's
// arguments.
func shadcnComponents(args []ShadcnArg) Components {
	var o options
	for _, a := range args {
		if f, ok := a.(Option); ok && f != nil {
			f(&o)
		}
	}
	return o.components
}
