package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// shad2cStatusIcon returns the outlined (lucide-like) status glyph shadcn
// uses in Alert and Sonner: circle-check, info, triangle-alert,
// circle-alert. The glyphs are the outlined ones of ui/icon.
func shad2cStatusIcon(status string) *icon.Icon {
	switch status {
	case "success":
		return icon.CheckCircle
	case "warning":
		return icon.Warning
	case "error", "danger":
		return icon.Exclamation
	}
	return icon.Info
}

// shad2cStatusColor is the glyph color of a shadcn alert: the default
// variant draws info in the foreground color and keeps the status hue for
// success / warning; the destructive (error) variant is red.
func shad2cStatusColor(th *theme.Theme, status string) theme.Color {
	switch status {
	case "success", "warning", "error", "danger":
		return th.ColorOf(status).Base
	}
	return th.Text
}

// shad2cStatusNode adds an outlined status glyph of size px.
func shad2cStatusNode(c *Context, status string, size float32) *node {
	return shad2cStatusNodeColor(c, status, size, shad2cStatusColor(c.Theme, status))
}

// shad2cStatusNodeColor is shad2cStatusNode drawn in col.
func shad2cStatusNodeColor(c *Context, status string, size float32, col theme.Color) *node {
	n := c.add("status-icon", status)
	n.ln.Width, n.ln.Height = size, size
	n.aHidden = true
	ic := shad2cStatusIcon(status)
	n.paint = func(l *paint.List, r geom.Rect) { c.paintIcon(l, ic, r, col, 0) }
	return n
}
