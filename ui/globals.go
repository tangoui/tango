package ui

// renderGlobals draws app-wide layers (messages, notifications) after the
// page function. Filled in by feedback components.
var globalLayers []func(c *Context)

func renderGlobals(c *Context) {
	for _, f := range globalLayers {
		f(c)
	}
}
