package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

// shad2cConfirmBox is the shadcn AlertDialog body of Confirm / MessageBox
// / Prompt: p-6, a text-lg semibold title, a muted description (gap-2),
// the footer buttons right-aligned below (gap-4). AlertDialog has no
// status icon; MessageBox, and a Confirm given an explicit Status, show
// the outlined (lucide-style) glyph before the title.
func shad2cConfirmBox(c *Context, d *ConfirmRef, status string, buttons, input func()) {
	th := c.Theme
	showIcon := status != "none" && (d.kind == moreBoxAlert || d.o.Status != "")
	Column(c, func() {
		Column(c, func() {
			if d.o.Title != "" || showIcon {
				Row(c, func() {
					if showIcon {
						shad2cStatusNode(c, status, 20)
					}
					if d.o.Title != "" {
						Text(c, d.o.Title).Size(th.FontSizeLG + 2).Weight(text.Semibold).Color(th.TextHeading).Layout(layout.Grow(1))
					}
				}, layout.Gap(8), layout.Middle())
			}
			if d.o.Content != "" {
				Text(c, d.o.Content).Color(th.TextTertiary)
			}
			if d.kind == moreBoxPrompt {
				Column(c, input, layout.Gap(4), layout.Margin(geom.Insets{Top: 4}))
			}
		}, layout.Gap(8))
		Row(c, buttons, layout.End(), layout.Gap(8), layout.Margin(geom.Insets{Top: 16}))
	}, layout.Pad(24))
}
