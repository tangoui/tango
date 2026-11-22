package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// CodeHeaderStyle styles the header shared by CodeBlock and Markdown fences.
type CodeHeaderStyle struct {
	Padding  geom.Insets // logical pixels around the header contents
	Divider  theme.Color // 1px separator, omitted when transparent
	Mono     bool        // use monospace for the title or language
	TextCopy bool        // show the localized Copy label instead of an icon
}

func codeHeader(c *Context, title string, copyable, copied bool, style CodeHeaderStyle, copyNow func()) {
	th, loc := c.Theme, c.Locale()
	pad := style.Padding
	pad.Bottom += b2f(style.Divider.A > 0)
	hd := Row(c, func() {
		label := Text(c, title).Size(12).LineHeight(20).Color(th.TextSecondary).Ellipsis().Layout(layout.Grow(1))
		if style.Mono {
			label.st.Mono, label.st.FontFamily = true, ""
		}
		if !copyable {
			return
		}
		if !style.TextCopy {
			if IconButton(c, codeCopyIcon(copied), loc.Markdown.CopyCode).Size(22, 14).Clicked() {
				copyNow()
			}
			return
		}
		name := loc.Copy
		if copied {
			name = loc.Markdown.Copied
		}
		font := c.TextStyle(12, text.Regular)
		button := Row(c, nil, layout.W(max(text.Width(loc.Copy, font), text.Width(loc.Markdown.Copied, font))+16), layout.H(22), layout.Center())
		button.Semantics(a11y.RoleButton, loc.Markdown.CopyCode).Radius(4)
		ia := c.interact(button.n, true, input.CursorPointer)
		fg := th.TextSecondary
		if ia.Hovered || ia.FocusVisible {
			button.Bg(th.FillSecondary)
			fg = th.Text
		}
		button.Body(func() { Text(c, name).Size(12).LineHeight(20).NoWrap().Color(fg) })
		if ia.Clicked {
			copyNow()
		}
	}, layout.Middle(), layout.Gap(8), layout.Padding(pad))
	if style.Divider.A > 0 {
		hd.PaintOver(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, style.Divider)
		})
	}
}

func codeCopyIcon(copied bool) *icon.Icon {
	if copied {
		return icon.Check
	}
	return icon.Copy
}
