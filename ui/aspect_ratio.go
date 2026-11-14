package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// AspectRatioW is a container that keeps a fixed width-to-height ratio
// (shadcn/ui AspectRatio, CSS aspect-ratio): it takes the width its
// parent gives it and is ratio times less tall; the content fills it.
type AspectRatioW struct {
	_     [0]func()
	c     *Context
	box   *Box
	inner *Box
	ratio float32
	maxW  float32 // MaxWidth (0: none)
}

// AspectRatio adds a ratio box (width / height, e.g. 16.0/9) and builds
// fn inside it, stretched over the whole box:
//
//	ui.AspectRatio(c, 16.0/9, func() { ui.Image(c, src).Fill() }).Radius(8)
//
// In a column it spans the column's width; where nothing gives it a width
// (a row, a stack) it is zero wide unless MaxWidth or layout options set
// one. A ratio <= 0 is taken as 1.
func AspectRatio(c *Context, ratio float32, fn func(), opts ...layout.Option) *AspectRatioW {
	if ratio <= 0 || ratio != ratio {
		ratio = 1
	}
	w := &AspectRatioW{c: c, ratio: ratio}
	w.box = c.box("aspect-ratio", layout.Column, opts)
	w.box.Body(func() {
		// The sizer is the only in-flow child: its height follows the
		// width it is given, so the box gets ratio's height.
		s := c.add("aspect-sizer", "")
		s.aHidden = true
		s.ln.Measure = func(maxW, _ float32) geom.Size {
			if w.maxW > 0 {
				maxW = min(maxW, w.maxW)
			}
			if maxW <= 0 || maxW >= geom.Inf {
				return geom.Size{}
			}
			return geom.Size{W: maxW, H: maxW / w.ratio}
		}
		w.inner = StackBox(c, fn, layout.Abs(0, 0, 0, 0))
	})
	return w
}

// Radius rounds the corners and clips the content to them.
func (w *AspectRatioW) Radius(r float32) *AspectRatioW {
	w.box.Radius(r).Clip()
	w.inner.Radius(r)
	return w
}

// MaxWidth limits the width (and with it the height).
func (w *AspectRatioW) MaxWidth(px float32) *AspectRatioW {
	w.maxW = px
	w.box.Style(layout.MaxW(px))
	return w
}

// A11yLabel exposes the box as a named group (a figure caption).
func (w *AspectRatioW) A11yLabel(s string) *AspectRatioW {
	w.box.Semantics(a11y.RoleGroup, s)
	return w
}

// Ratio returns the width-to-height ratio in effect.
func (w *AspectRatioW) Ratio() float32 { return w.ratio }

// ID returns the box ID.
func (w *AspectRatioW) ID() ID { return w.box.n.id }

// Rect returns the box rectangle of the last frame.
func (w *AspectRatioW) Rect() geom.Rect { return w.box.Rect() }
