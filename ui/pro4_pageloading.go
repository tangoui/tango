package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// PageLoadingW is ProLayout's centred, large Spin wrapper.
type PageLoadingW struct {
	_           [0]func()
	c           *Context
	spinning    bool
	size        ControlSize
	delay       time.Duration
	description string
}

// PageLoading starts a page loading indicator; chain options, then Show.
// Like Pro v3 PageLoading, it adds 100px top padding and centres a large
// Spin. The legacy react-loadable isLoading/pastDelay/error props do not
// control visibility; Spinning is the Spin visibility input.
// ConfigProvider.Defaults("PageLoading", ...): "spinning", "size",
// "delay" (time.Duration), "description".
func PageLoading(c *Context) *PageLoadingW {
	return &PageLoadingW{c: c, spinning: Prop(c, "PageLoading", "spinning", true),
		size:        Prop(c, "PageLoading", "size", Large),
		delay:       Prop(c, "PageLoading", "delay", time.Duration(0)),
		description: Prop(c, "PageLoading", "description", "")}
}

// Spinning controls the loading indicator, not the surrounding page.
func (p *PageLoadingW) Spinning(on bool) *PageLoadingW { p.spinning = on; return p }

// Size selects the Spin size (default large).
func (p *PageLoadingW) Size(size ControlSize) *PageLoadingW { p.size = size; return p }

// Delay delays showing the indicator; stopping always hides immediately.
func (p *PageLoadingW) Delay(d time.Duration) *PageLoadingW { p.delay = d; return p }

// Description sets the loading indicator's text and accessible name.
func (p *PageLoadingW) Description(s string) *PageLoadingW { p.description = s; return p }

// Show builds the page loading wrapper using the shared Spin implementation.
func (p *PageLoadingW) Show() *PageLoadingW {
	// Pro es/layout/components/PageLoading/index.js paddingBlockStart: 100.
	Column(p.c, func() {
		Row(p.c, func() {
			Spin(p.c).Size(p.size).Spinning(p.spinning).Delay(p.delay).Description(p.description)
		}, layout.Center())
	}, layout.Padding(geom.Insets{Top: 100}))
	return p
}
