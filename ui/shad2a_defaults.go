package ui

// ConfigProvider Defaults of the basic data-entry controls (Button,
// Checkbox/Radio/Switch, Input family, Select family, OTP, Rate,
// Segmented, ColorPicker, Mentions, CheckableTag). Keys mirror the Ant
// Design prop names:
//
//	ui.ConfigProvider(c).Defaults("Button", ui.Props{"size": "small", "shape": "round"}).Body(page)
//
// Each constructor applies its defaults before returning, so explicit
// modifiers always win.

import "strings"

// shad2aParseSize converts a size default (a ControlSize, or an Ant / Element
// size name) to a ControlSize.
func shad2aParseSize(v any) (ControlSize, bool) {
	switch x := v.(type) {
	case ControlSize:
		return x, true
	case string:
		switch strings.ToLower(x) {
		case "small", "sm", "mini":
			return Small, true
		case "middle", "default", "medium", "md":
			return Middle, true
		case "large", "lg":
			return Large, true
		}
	}
	return Middle, false
}

// shad2aSize applies the "size" default of comp to *dst.
func shad2aSize(c *Context, comp string, dst *ControlSize) {
	if v, ok := c.ConfigProp(comp, "size"); ok {
		if s, ok := shad2aParseSize(v); ok {
			*dst = s
		}
	}
}

// buttonDefaults: size, type ("primary"|"dashed"|"text"|"link"|"default"),
// variant (Ant "solid"|"outlined"|"dashed"|"filled"|"text"|"link", shadcn
// "default"|"secondary"|"outline"|"ghost"|"link"|"destructive"), shape
// ("round"|"circle"), danger, ghost, block.
func (b *ButtonW) configDefaults() {
	c := b.c
	shad2aSize(c, "Button", &b.size)
	if t, ok := p2bStr(c, "Button", "type"); ok {
		b.variant = buttonVariantOf(t, b.variant)
	}
	if v, ok := p2bStr(c, "Button", "variant"); ok {
		if v == "destructive" {
			b.variant, b.danger = VariantPrimary, true
		} else {
			b.variant = buttonVariantOf(v, b.variant)
		}
	}
	if s, ok := p2bStr(c, "Button", "shape"); ok {
		b.round, b.circle = s == "round", s == "circle"
	}
	if v, ok := p2bBool(c, "Button", "danger"); ok {
		b.danger = v
	}
	if v, ok := p2bBool(c, "Button", "ghost"); ok {
		b.ghost = v
	}
	if v, ok := p2bBool(c, "Button", "block"); ok && v {
		b.Block()
	}
}

// buttonVariantOf maps an Ant type / variant or shadcn variant name.
func buttonVariantOf(s string, def ButtonVariant) ButtonVariant {
	switch strings.ToLower(s) {
	case "primary", "solid":
		return VariantPrimary
	case "default", "outlined", "outline":
		return VariantDefault
	case "dashed":
		return VariantDashed
	case "text", "ghost":
		return VariantText
	case "link":
		return VariantLink
	case "filled", "secondary":
		return VariantSecondary
	}
	return def
}

// switchDefaults: size ("small"), checkedChildren / unCheckedChildren.
func (w *SwitchW) configDefaults() {
	c := w.c
	if v, ok := c.ConfigProp("Switch", "size"); ok {
		if s, ok := shad2aParseSize(v); ok {
			w.small = s == Small
		}
	}
	if s, ok := p2bStr(c, "Switch", "checkedChildren"); ok {
		w.onText = s
	}
	if s, ok := p2bStr(c, "Switch", "unCheckedChildren"); ok {
		w.offText = s
	}
}

// radioGroupDefaults (component "Radio"): size, optionType ("button"),
// buttonStyle ("solid").
func (g *RadioGroupW) configDefaults() {
	c := g.c
	shad2aSize(c, "Radio", &g.size)
	if s, ok := p2bStr(c, "Radio", "optionType"); ok {
		g.buttons = s == "button"
	}
	if s, ok := p2bStr(c, "Radio", "buttonStyle"); ok && s == "solid" {
		g.buttons, g.solid = true, true
	}
}

// sizeDefaults adds "size" to the Input defaults, and "placeholder" for
// the components wrapping an Input (AutoComplete). The plain Input has no
// placeholder default: it would leak into the inputs other widgets embed.
func (w *InputW) sizeDefaults(comp string) {
	shad2aSize(w.c, comp, &w.size)
	if comp == "Input" {
		return
	}
	if s, ok := p2bStr(w.c, comp, "placeholder"); ok {
		w.placeholder = s
	}
}

// InputNumber: size, placeholder (plus controlsPosition, variant, wheel).
func (w *InputNumberW) sizeDefaults() {
	shad2aSize(w.c, "InputNumber", &w.size)
	if s, ok := p2bStr(w.c, "InputNumber", "placeholder"); ok {
		w.placeholder = s
	}
}

// TextArea (component "TextArea", falling back to "Input" keys):
// placeholder, rows, showCount, allowClear, maxLength.
func (w *TextAreaW) configDefaults() {
	c := w.c
	for _, comp := range []string{"Input", "TextArea"} {
		if s, ok := p2bStr(c, comp, "placeholder"); ok {
			w.placeholder = s
		}
		if v, ok := p2bBool(c, comp, "showCount"); ok {
			w.showCount = v
		}
		if v, ok := p2bBool(c, comp, "allowClear"); ok {
			w.allowClear = v
		}
		if v, ok := p2bNum(c, comp, "maxLength"); ok {
			w.maxLen = int(v)
		}
	}
	if v, ok := p2bNum(c, "TextArea", "rows"); ok {
		w.rows = max(1, int(v))
	}
}

// Search (component "Search", falling back to "Input" keys): size,
// placeholder, allowClear, enterButton (string label).
func (w *SearchW) configDefaults() {
	c := w.c
	for _, comp := range []string{"Input", "Search"} {
		shad2aSize(c, comp, &w.size)
		if s, ok := p2bStr(c, comp, "placeholder"); ok {
			w.placeholder = s
		}
		if v, ok := p2bBool(c, comp, "allowClear"); ok {
			w.allowClear = v
		}
	}
	if s, ok := p2bStr(c, "Search", "enterButton"); ok {
		w.enter = s
	}
}

// selectorDefaults applies size / placeholder / allowClear / maxTagCount of
// comp to a selector shell (InputTag; Select-family shells use
// p2bSelectDefaults).
func (b *selector) shadDefaults(comp string) {
	c := b.c
	shad2aSize(c, comp, &b.size)
	if s, ok := p2bStr(c, comp, "placeholder"); ok {
		b.placeholder = s
	}
	if v, ok := p2bBool(c, comp, "allowClear"); ok {
		b.allowClear = v
	}
	if v, ok := p2bNum(c, comp, "maxTagCount"); ok {
		b.maxCount = int(v)
	}
}

// OTP: size, mask (string), length is the constructor argument.
func (w *OTPW) configDefaults() {
	shad2aSize(w.c, "OTP", &w.size)
	if s, ok := p2bStr(w.c, "OTP", "mask"); ok {
		w.mask = s
	}
}

// Rate: count, allowHalf, allowClear.
func (w *RateW) configDefaults() {
	c := w.c
	if v, ok := p2bNum(c, "Rate", "count"); ok {
		w.count = max(1, int(v))
	}
	if v, ok := p2bBool(c, "Rate", "allowHalf"); ok {
		w.half = v
	}
	if v, ok := p2bBool(c, "Rate", "allowClear"); ok {
		w.allowClear = v
	}
}

// Segmented: size, block.
func (w *SegmentedW) configDefaults() {
	shad2aSize(w.c, "Segmented", &w.size)
	if v, ok := p2bBool(w.c, "Segmented", "block"); ok && v {
		w.Block()
	}
}

// ColorPicker: size, showText, allowClear, disabledAlpha, format
// ("hex"|"rgb"|"hsb"|"hsv").
func (w *ColorPickerW) configDefaults() {
	c := w.c
	shad2aSize(c, "ColorPicker", &w.size)
	if v, ok := p2bBool(c, "ColorPicker", "showText"); ok {
		w.showText = v
	}
	if v, ok := p2bBool(c, "ColorPicker", "allowClear"); ok {
		w.allowClear = v
	}
	if v, ok := p2bBool(c, "ColorPicker", "disabledAlpha"); ok {
		w.noAlpha = v
	}
	if !w.st.fmtInit { // the format is retained: default only on first build
		w.st.fmtInit = true
		if f, ok := p2bStr(c, "ColorPicker", "format"); ok && cpFormatOK(f) {
			w.st.format = f
		}
	}
}

// Mentions: size, placeholder, rows, prefix (string).
func (w *MentionsW) configDefaults() {
	c := w.c
	shad2aSize(c, "Mentions", &w.size)
	if s, ok := p2bStr(c, "Mentions", "placeholder"); ok {
		w.placeholder = s
	}
	if v, ok := p2bNum(c, "Mentions", "rows"); ok {
		w.rows = max(1, int(v))
	}
	if s, ok := p2bStr(c, "Mentions", "prefix"); ok && s != "" {
		w.prefixes = []string{s}
	}
}

// AutoComplete: size, placeholder, allowClear (on the inner Input).
func (w *AutoCompleteW) configDefaults() {
	in := w.in
	in.sizeDefaults("AutoComplete")
	if v, ok := p2bBool(w.c, "AutoComplete", "allowClear"); ok {
		in.allowClear = v
	}
}
