// Package ui is split: stable symbols are listed in api/stable/ui.txt, the
// rest is Experimental.
package ui

// Context carries configuration.
type Context struct {
	props map[string]map[string]any
}

// ConfigProp returns a raw default.
func (c *Context) ConfigProp(component, key string) (any, bool) {
	v, ok := c.props[component][key]
	return v, ok
}

// Prop returns a typed default.
func Prop[T any](c *Context, component, key string, def T) T {
	if v, ok := c.ConfigProp(component, key); ok {
		if t, ok := v.(T); ok {
			return t
		}
	}
	return def
}

// Size is a control size.
type Size int

const cardName = "Card"

// Button reads its defaults.
func Button(c *Context) {
	_ = Prop(c, "Button", "size", Size(0))
	_ = Prop(c, "Button", "ghost", false)
	_ = Prop[string](c, cardName, "title", "")
	comp := "Dynamic"
	_ = Prop(c, comp, "skipped", 1)
	c.ConfigProp("Card", "extra")
}

// Option is a functional option over an unexported type; users cannot name
// options, so it is not a cross-tier reference.
type Option func(*options)

type options struct{ dark bool }
