package ui

// Slots retain the historical init order, regardless of first use. Keeping
// renderer references on the owning Context lets the linker discard features
// an application never calls and avoids process-global registration races.
type globalLayer uint8

const (
	globalConfirm globalLayer = iota
	globalContextMenu
	globalMessage
	globalNotification
	globalLayerCount
)

func (c *Context) useGlobalLayer(kind globalLayer, render func(*Context)) {
	c.globalLayers[kind] = render
}

// renderGlobals draws app-wide layers after the page function. An opening
// API installs its own renderer before this pass, so it appears this frame
// and stays installed for later close/animation frames.
func renderGlobals(c *Context) {
	for i := range c.globalLayers {
		// Read each slot when reached: an earlier layer may open a later
		// one (e.g. a dialog callback opens a notification) in this pass.
		if render := c.globalLayers[i]; render != nil {
			render(c)
		}
	}
}
