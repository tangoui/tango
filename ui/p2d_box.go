package ui

import "github.com/tangoui/tango/ui/geom"

// Label gives a box (Custom, Div, Row …) a text label: Driver.Find,
// FindText and ClickLabel locate it by that text, and it is the box's
// accessible name unless Semantics set another. The widget ID is not
// affected (it was computed when the box was created).
func (b *Box) Label(s string) *Box {
	b.n.label = s
	return b
}

// PointerPos returns the last pointer position in the window and whether
// the pointer is inside it (hosts placing drops that carry no point).
func (r *Runner) PointerPos() (geom.Point, bool) { return r.c.Pointer() }
