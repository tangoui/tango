package ui

// ConfigProvider.Defaults wiring of the components that had none (see
// docs/specs/ui-coverage.md §6.1). Keys follow the Ant Design prop names,
// Element Plus where Ant has none; every key is read with a constant
// component and key so that it is listed in api/go/ui.txt (R11). Each
// constructor applies these before returning, so explicit modifiers win.
//
//	Addon            size
//	BackTop          visibilityHeight (px), duration (time.Duration or ms)
//	Compact          block
//	Flex             gap (ControlSize, "small" | "middle" | "large", or px)
//	FloatButtonGroup shape ("square"), trigger ("click" | "hover"), type ("primary")
//	SplitterPanel    collapsible, resizable
//	Command          placeholder, emptyText
//	Checkbox         size (Checkbox, CheckboxGroup, CheckAll)
//	Radio            size (Radio; RadioGroup already)
//	MarkdownEditor   mode ("edit" | "split" | "preview"), placeholder
//	AvatarGroup      maxCount
//	BadgeRibbon      placement ("start" | "end"; when the argument is "")
//	Countdown        format (Countdown and StatisticTimer)
//	InfiniteScroll   distance (px)
//	Listy            split, rowHoverable
//	Tour             type ("primary"), mask, arrow
//	VirtualTree      checkStrictly, expandOnClickNode, checkOnClickNode
//	Item             variant (ItemVariant or its string), size (ItemSize or its string)
//	ItemGroup        split, bordered
//	ButtonGroup      size, type (a Button type / variant name)
//	Button           autoInsertSpace
//	Message          maxCount
//
// The panels reuse the keys of their pickers: CascaderPanel "Cascader"
// (expandTrigger, changeOnSelect, showCheckedStrategy, strategyValue),
// ColorPickerPanel "ColorPicker" (size, allowClear, disabledAlpha,
// format), DatePickerPanel "DatePicker" (picker), TimePanel "TimePicker"
// (format, use12Hours, hourStep, minuteStep, secondStep,
// hideDisabledOptions); MultiDatePicker reads "DatePicker" (format,
// placeholder, picker, allowClear, size, width) and Loading "Spin" (size,
// tip).

import (
	"strings"
	"time"
)

// numProp converts a ConfigProp result holding any Go number.
func numProp(v any, ok bool) (float64, bool) {
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// sizeProp converts a ConfigProp result holding a ControlSize or an Ant /
// Element size name.
func sizeProp(v any, ok bool) (ControlSize, bool) {
	if !ok {
		return Middle, false
	}
	return shad2aParseSize(v)
}

// durProp converts a ConfigProp result holding a time.Duration or a
// number of milliseconds.
func durProp(v any, ok bool) (time.Duration, bool) {
	if !ok {
		return 0, false
	}
	if d, ok := v.(time.Duration); ok {
		return d, true
	}
	if ms, ok := numProp(v, true); ok {
		return time.Duration(ms * float64(time.Millisecond)), true
	}
	return 0, false
}

func (a *AddonW) configDefaults() {
	if s, ok := sizeProp(a.c.ConfigProp("Addon", "size")); ok {
		a.size = s
	}
}

func (b *BackTopW) configDefaults() {
	c := b.c
	if n, ok := numProp(c.ConfigProp("BackTop", "visibilityHeight")); ok {
		b.visible = float32(n)
	}
	if d, ok := durProp(c.ConfigProp("BackTop", "duration")); ok {
		b.dur = d
	}
}

func (w *CompactW) configDefaults() {
	w.block = Prop(w.c, "Compact", "block", w.block)
}

func (f *FlexW) configDefaults() {
	v, ok := f.c.ConfigProp("Flex", "gap")
	if !ok {
		return
	}
	if s, ok := sizeProp(v, true); ok {
		f.Gap(s)
	} else if px, ok := numProp(v, true); ok {
		f.GapPx(float32(px))
	}
}

func (g *FloatButtonGroupW) configDefaults() {
	c := g.c
	g.square = Prop(c, "FloatButtonGroup", "shape", "") == "square"
	g.trigger = Prop(c, "FloatButtonGroup", "trigger", g.trigger)
	g.primary = Prop(c, "FloatButtonGroup", "type", "") == "primary"
}

func (p *SplitterPanelW) configDefaults() {
	c := p.c
	p.collapsible = Prop(c, "SplitterPanel", "collapsible", p.collapsible)
	p.fixed = !Prop(c, "SplitterPanel", "resizable", !p.fixed)
}

func (w *CommandW) configDefaults() {
	c := w.c
	w.place = Prop(c, "Command", "placeholder", w.place)
	w.empty = Prop(c, "Command", "emptyText", w.empty)
}

// configDefaults applies the "Cascader" keys that concern the columns.
func (p *CascaderPanelW) configDefaults() {
	c, w := p.c, p.cas
	if v, ok := p2bStr(c, "Cascader", "expandTrigger"); ok {
		w.hoverExpand = v == "hover"
	}
	if v, ok := p2bBool(c, "Cascader", "changeOnSelect"); ok {
		w.changeOnSelect = v
	}
	if v, ok := p2bBool(c, "Cascader", "strategyValue"); ok {
		w.stratVal = v
	}
	w.strategy = p2bStrategy(c, "Cascader", w.strategy)
}

func (w *CheckboxW) configDefaults() {
	if s, ok := sizeProp(w.c.ConfigProp("Checkbox", "size")); ok {
		w.size = s
	}
}

func (g *CheckboxGroupW) configDefaults() {
	if s, ok := sizeProp(g.c.ConfigProp("Checkbox", "size")); ok {
		g.size = s
	}
}

func (w *RadioW) configDefaults() {
	if s, ok := sizeProp(w.c.ConfigProp("Radio", "size")); ok {
		w.size = s
	}
}

func (w *DatePickerPanelW) configDefaults() {
	if v, ok := w.c.ConfigProp("DatePicker", "picker"); ok {
		if p, ok := v.(PickerType); ok {
			w.picker = p
		}
	}
}

func (w *TimePanelW) configDefaults() {
	def := p2aDefaults(w.c, "TimePicker")
	def.str("format", &w.format)
	def.flag("use12Hours", &w.clock.use12)
	def.integer("hourStep", &w.steps[0])
	def.integer("minuteStep", &w.steps[1])
	def.integer("secondStep", &w.steps[2])
	def.flag("hideDisabledOptions", &w.clock.hide)
}

func (w *MultiDatePickerW) configDefaults() {
	def := p2aDefaults(w.c, "DatePicker")
	w.formatSet = def.str("format", &w.format)
	w.phSet = def.str("placeholder", &w.box.placeholder)
	if v, ok := w.c.ConfigProp("DatePicker", "picker"); ok {
		if p, ok := v.(PickerType); ok {
			w.picker = p
		}
	}
	def.flag("allowClear", &w.box.allowClear)
	def.px("width", &w.box.n.ln.Width)
	def.size(&w.box.size)
}

func (w *MarkdownEditorW) configDefaults() {
	c := w.c
	w.mode = Prop(c, "MarkdownEditor", "mode", w.mode)
	w.placeholder = Prop(c, "MarkdownEditor", "placeholder", w.placeholder)
}

func (g *AvatarGroupW) configDefaults() {
	if n, ok := numProp(g.c.ConfigProp("AvatarGroup", "maxCount")); ok {
		g.max = int(n)
	}
}

// badgeRibbonPlacement is the placement of a BadgeRibbon called with "".
func badgeRibbonPlacement(c *Context, placement string) string {
	if placement == "" {
		return Prop(c, "BadgeRibbon", "placement", "")
	}
	return placement
}

func (w *CountdownW) configDefaults() {
	w.format = Prop(w.c, "Countdown", "format", w.format)
}

func (w *InfiniteScrollW) configDefaults() {
	if n, ok := numProp(w.c.ConfigProp("InfiniteScroll", "distance")); ok {
		w.distance = float32(n)
	}
}

func (w *ListyW) configDefaults() {
	c := w.c
	w.split = Prop(c, "Listy", "split", w.split)
	w.hoverable = Prop(c, "Listy", "rowHoverable", w.hoverable)
}

func (t *TourW) configDefaults() {
	c := t.c
	t.primary = Prop(c, "Tour", "type", "") == "primary"
	t.mask = Prop(c, "Tour", "mask", t.mask)
	t.arrow = Prop(c, "Tour", "arrow", t.arrow)
}

func (w *VirtualTreeW) configDefaults() {
	c := w.c
	w.strict = Prop(c, "VirtualTree", "checkStrictly", w.strict)
	// Set only when given (the Element theme turns it on otherwise): a bool
	// that both defaults agree on.
	if on := Prop(c, "VirtualTree", "expandOnClickNode", true); on == Prop(c, "VirtualTree", "expandOnClickNode", false) {
		w.ExpandOnClick(on)
	}
	w.chkClick = Prop(c, "VirtualTree", "checkOnClickNode", w.chkClick)
}

func (m *LoadingW) configDefaults() {
	c := m.c
	if sz, ok := s2cSize(c, "Spin"); ok {
		m.size, m.sizeSet = sz, true
	}
	if t, ok := p2bStr(c, "Spin", "tip"); ok {
		m.tip = t
	}
}

func (w *ItemW) configDefaults() {
	c := w.c
	if v, ok := c.ConfigProp("Item", "variant"); ok {
		switch x := v.(type) {
		case ItemVariant:
			w.variant = x
		case string:
			switch ItemVariant(x) {
			case ItemVariantDefault, ItemVariantOutline, ItemVariantMuted:
				w.variant = ItemVariant(x)
			}
		}
	}
	if v, ok := c.ConfigProp("Item", "size"); ok {
		switch x := v.(type) {
		case ItemSize:
			w.size = x
		case string:
			switch strings.ToLower(x) {
			case "sm", "small":
				w.size = ItemSizeSmall
			case "default", "middle":
				w.size = ItemSizeDefault
			}
		}
	}
}

func (g *ItemGroupW) configDefaults() {
	c := g.c
	g.sep = Prop(c, "ItemGroup", "split", g.sep)
	g.bordered = Prop(c, "ItemGroup", "bordered", g.bordered)
}

func (g *ButtonGroupW) configDefaults() {
	c := g.c
	if s, ok := sizeProp(c.ConfigProp("ButtonGroup", "size")); ok {
		g.size, g.sizeSet = s, true
	}
	if t := Prop(c, "ButtonGroup", "type", ""); t != "" {
		const unknown ButtonVariant = 255
		if v := buttonVariantOf(t, unknown); v != unknown {
			g.variant, g.varSet = v, true
		}
	}
}

// messageMaxCount closes the oldest messages beyond the "Message"
// maxCount default of the scope (0 or unset: no limit).
func messageMaxCount(c *Context, t *toasts) {
	n, ok := numProp(c.ConfigProp("Message", "maxCount"))
	if !ok || n < 1 {
		return
	}
	if k := int(n); len(t.msgs) > k {
		t.msgs = append([]*toast(nil), t.msgs[len(t.msgs)-k:]...)
	}
}
