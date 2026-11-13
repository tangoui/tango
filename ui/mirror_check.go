package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tangoui/tango/ui/geom"
)

// Mirror checker (docs/specs/rtl-and-component-tokens.md §3.8): renders a
// page once left to right and once right to left at the same size and
// reports nodes whose RTL rectangle is not the mirror image of their LTR
// one. It finds layout that should flip but does not (absolute offsets
// computed in physical pixels, hand-placed children) and layout that flips
// but should not.

// MirrorViolation is a node whose RTL position is not the mirror of its
// LTR position inside its parent.
type MirrorViolation struct {
	ID    ID
	Label string    // the node's internal label (to locate it)
	Under string    // label of the nearest labelled ancestor ("" if none)
	LTR   geom.Rect // absolute rectangle in the LTR frame
	RTL   geom.Rect // absolute rectangle in the RTL frame
	Want  geom.Rect // where the RTL rectangle was expected
}

func (v MirrorViolation) String() string {
	under := ""
	if v.Under != "" {
		under = fmt.Sprintf(" under %q", v.Under)
	}
	return fmt.Sprintf("%q%s ltr=(%.0f,%.0f %.0fx%.0f) rtl=(%.0f,%.0f %.0fx%.0f) want x=%.0f y=%.0f %.0fx%.0f",
		v.Label, under, v.LTR.X, v.LTR.Y, v.LTR.W, v.LTR.H, v.RTL.X, v.RTL.Y, v.RTL.W, v.RTL.H, v.Want.X, v.Want.Y, v.Want.W, v.Want.H)
}

// MirrorReport is the result of Driver.MirrorCheck.
type MirrorReport struct {
	Width      float32 // frame width both passes ran at
	Checked    int     // nodes compared
	Skipped    int     // nodes present in both frames but exempt (NoMirror, LTR parent)
	Violations []MirrorViolation
	// Icons lists directional icons drawn unmirrored in the RTL frame
	// (icon check); IconsSkipped counts those exempt inside a NoMirror
	// node or subtree.
	Icons        []IconViolation
	IconsSkipped int
}

// OK reports whether no violation (layout or icon) was found.
func (r MirrorReport) OK() bool { return len(r.Violations) == 0 && len(r.Icons) == 0 }

// String summarizes the report, one violation per line.
func (r MirrorReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d nodes checked, %d skipped, %d violations, %d unmirrored icons\n", r.Checked, r.Skipped, len(r.Violations), len(r.Icons))
	for _, v := range r.Violations {
		b.WriteString("  ")
		b.WriteString(v.String())
		b.WriteByte('\n')
	}
	for _, v := range r.Icons {
		b.WriteString("  ")
		b.WriteString(v.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// mirrorTol is the position tolerance in logical px (rounding of
// fractional layouts).
const mirrorTol = 1

// MirrorCheck renders page left to right and right to left (wrapped in
// ConfigProvider(c).Direction(...)) at d's size, scale and theme, settles
// each frame (after the optional setup, e.g. opening a popup) and compares
// every node drawn in both frames.
//
// Each node is checked relative to its parent: with p the parent's
// rectangle, the RTL node must sit at x' = p'.X + p.W - (x - p.X) - w, with
// the same y offset and size (±1px). Checking against the parent instead
// of the window (|X_rtl − (W − X_ltr − W_n)|) reports only the node at
// fault, not its whole subtree; for a chain of correctly mirrored parents
// the two are the same. Nodes without a parent drawn in both frames
// (overlay roots) are checked against the window.
//
// Skipped: nodes with layout.Physical() (NoMirror) and children of boxes
// that lay out left to right in the RTL frame (an LTR scope such as a code
// editor or chart, or the window root outside the page's provider).
//
// Icon check: every icon drawn in the RTL frame is recorded (see
// paint.List.IconNote); a directional icon drawn unmirrored while its node
// was painted in a right-to-left scope is reported in Icons, located at
// the deepest node containing it. Icons in LTR scopes are not reported;
// icons inside a NoMirror node or subtree are counted in IconsSkipped.
func (d *Driver) MirrorCheck(page func(*Context), setup ...func(*Driver)) MirrorReport {
	size, scale, th := d.w.size, d.w.scale, d.r.c.Theme
	var rec *iconRecorder
	pass := func(dir Direction) *Driver {
		p := func(c *Context) { ConfigProvider(c).Direction(dir).Body(func() { page(c) }) }
		dd := NewDriver(p, DriverOptions{Width: size.W, Height: size.H, Scale: scale, Theme: th})
		if dir == RTL {
			rec, _ = dd.recordIcons() // the driver is discarded with its recorder
		}
		dd.Settle()
		for _, f := range setup {
			f(dd)
			dd.Settle()
		}
		return dd
	}
	ltr, rtl := pass(LTR), pass(RTL)
	rep := mirrorCompare(ltr.r.c, rtl.r.c, size.W)
	rep.Icons, rep.IconsSkipped = rtl.r.c.iconViolations(rec)
	return rep
}

// mirrorNode is one drawn node of a frame.
type mirrorNode struct {
	n      *node
	parent *node // nearest ancestor (nil for roots)
}

func mirrorCollect(c *Context) map[ID]mirrorNode {
	out := map[ID]mirrorNode{}
	var walk func(n, parent *node)
	walk = func(n, parent *node) {
		if n.drawn == c.pass {
			if _, dup := out[n.id]; !dup {
				out[n.id] = mirrorNode{n: n, parent: parent}
			}
		}
		for _, k := range n.kids {
			walk(k, n)
		}
	}
	walk(c.root, nil)
	for _, o := range c.overlays {
		walk(o.root, nil)
	}
	return out
}

func mirrorCompare(lc, rc *Context, winW float32) MirrorReport {
	rep := MirrorReport{Width: winW}
	ln, rn := mirrorCollect(lc), mirrorCollect(rc)
	ids := make([]ID, 0, len(rn))
	for id := range rn {
		if _, ok := ln[id]; ok {
			ids = append(ids, id)
		}
	}
	// Report in paint order of the RTL frame is not available after the
	// fact; sort by position for stable output.
	sort.Slice(ids, func(i, j int) bool {
		a, b := ln[ids[i]].n.abs, ln[ids[j]].n.abs
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.X != b.X {
			return a.X < b.X
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		l, r := ln[id], rn[id]
		if r.n.ln.NoMirror || (r.parent != nil && !r.parent.ln.RTL) {
			rep.Skipped++
			continue
		}
		// Box the node is mirrored in: its parent when the parent is drawn
		// in both frames with the same identity, else the window.
		lbox, rbox := geom.Rect{W: winW}, geom.Rect{W: winW}
		if l.parent != nil && r.parent != nil && l.parent.id == r.parent.id {
			if _, ok := ln[l.parent.id]; ok {
				if _, ok := rn[r.parent.id]; ok {
					lbox, rbox = l.parent.abs, r.parent.abs
				}
			}
		}
		la, ra := l.n.abs, r.n.abs
		want := geom.Rect{
			X: rbox.X + lbox.W - (la.X - lbox.X) - la.W,
			Y: rbox.Y + (la.Y - lbox.Y),
			W: la.W, H: la.H,
		}
		rep.Checked++
		if abs32(ra.X-want.X) > mirrorTol || abs32(ra.Y-want.Y) > mirrorTol ||
			abs32(ra.W-want.W) > mirrorTol || abs32(ra.H-want.H) > mirrorTol {
			under := ""
			for a := r.n.parent; a != nil && under == ""; a = a.parent {
				under = a.label
			}
			rep.Violations = append(rep.Violations, MirrorViolation{ID: id, Label: r.n.label, Under: under, LTR: la, RTL: ra, Want: want})
		}
	}
	return rep
}
