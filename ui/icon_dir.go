package ui

import (
	"fmt"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// paintIcon draws ic into the square r rotated by angle radians, mirroring
// directional icons (icon.Icon.IsDirectional) when the node being painted
// lays out right to left. Call it from paint callbacks instead of
// icon.Draw for icons that point along the reading direction; it is
// identical to icon.Draw in LTR.
func (c *Context) paintIcon(l *paint.List, ic *icon.Icon, r geom.Rect, col theme.Color, angle float64) {
	icon.DrawWith(l, ic, r, col, icon.DrawOpts{Angle: angle, RTL: c.IsRTL()})
}

// Icon recording for the RTL icon check (docs/specs/rtl-and-component-tokens.md
// §3.8): Driver.MirrorCheck and Driver.Audit install an iconRecorder on the
// runner's display list (paint.List.IconNote) for the frames they inspect.
// The recorder is per list, so drivers in parallel tests stay independent,
// and nothing is recorded in normal runs (IconNote is nil).

// iconNote is one icon draw of a frame.
type iconNote struct {
	name          string
	directional   bool
	mirrored      bool
	rtl           bool      // the scope being painted lays out right to left
	r             geom.Rect // in paint space (the space of node.abs)
	pass          uint64    // build pass the draw belongs to
	unmirroredDir bool      // directional, unmirrored, in an RTL scope
}

// iconRecorder collects the icon draws of the last painted frame.
type iconRecorder struct {
	c     *Context
	notes []iconNote
}

func (rec *iconRecorder) note(name string, directional, mirrored bool, r geom.Rect) {
	c := rec.c
	if len(rec.notes) > 0 && rec.notes[0].pass != c.pass {
		rec.notes = rec.notes[:0] // a new frame: keep only the latest one
	}
	rtl := c.IsRTL()
	rec.notes = append(rec.notes, iconNote{name: name, directional: directional, mirrored: mirrored, rtl: rtl,
		r: r, pass: c.pass, unmirroredDir: directional && !mirrored && rtl})
}

// recordIcons installs a fresh icon recorder on d's display list and
// returns it with a function that removes it again. Run at least one
// frame (Frame, Settle) before reading the notes.
func (d *Driver) recordIcons() (*iconRecorder, func()) {
	rec := &iconRecorder{c: d.r.c}
	l := &d.r.list
	prev := l.IconNote
	l.IconNote = rec.note
	return rec, func() { l.IconNote = prev }
}

// IconViolation is a directional icon (icon.Icon.IsDirectional) drawn
// without mirroring while its node was painted in a right-to-left scope:
// the widget used icon.Draw instead of the direction-aware paint helper.
type IconViolation struct {
	Icon  string    // icon name
	Label string    // label of the deepest labelled node containing the icon
	Under string    // label of the nearest labelled ancestor of that node
	Rect  geom.Rect // where the icon was drawn
}

func (v IconViolation) String() string {
	under := ""
	if v.Under != "" {
		under = fmt.Sprintf(" under %q", v.Under)
	}
	return fmt.Sprintf("icon %q unmirrored in %q%s at (%.0f,%.0f %.0fx%.0f)", v.Icon, v.Label, under, v.Rect.X, v.Rect.Y, v.Rect.W, v.Rect.H)
}

// iconViolations turns the unmirrored directional icons of rec into
// violations, located at the deepest node drawn in the last frame that
// contains each icon. Icons inside a NoMirror (layout.Physical) node or
// subtree are skipped and counted. Icons drawn in left-to-right scopes
// (an LTR ConfigProvider, a code editor) are never reported: rtl is the
// scope of the node being painted at draw time.
func (c *Context) iconViolations(rec *iconRecorder) (out []IconViolation, skipped int) {
	seen := map[iconNote]bool{}
	for _, nt := range rec.notes {
		if !nt.unmirroredDir || nt.pass != c.pass || seen[nt] {
			continue
		}
		seen[nt] = true
		n := c.iconNode(nt.r)
		if n != nil && underNoMirror(n) {
			skipped++
			continue
		}
		v := IconViolation{Icon: nt.name, Rect: nt.r}
		for a := n; a != nil; a = a.parent {
			if a.label == "" {
				continue
			}
			if v.Label == "" {
				v.Label = a.label
			} else {
				v.Under = a.label
				break
			}
		}
		out = append(out, v)
	}
	return out, skipped
}

// underNoMirror reports whether n or an ancestor keeps physical layout.
func underNoMirror(n *node) bool {
	for ; n != nil; n = n.parent {
		if n.ln.NoMirror {
			return true
		}
	}
	return false
}

// iconNode returns the deepest node drawn in the last frame whose
// rectangle contains the centre of r, searching the topmost overlay
// first.
func (c *Context) iconNode(r geom.Rect) *node {
	p := geom.Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
	var deepest func(n *node) *node
	deepest = func(n *node) *node {
		// Children are searched even outside the parent's rectangle
		// (absolutely positioned content may overflow it).
		for i := len(n.kids) - 1; i >= 0; i-- {
			if d := deepest(n.kids[i]); d != nil {
				return d
			}
		}
		if n.drawn == c.pass && n.abs.Contains(p) {
			return n
		}
		return nil
	}
	for i := len(c.overlays) - 1; i >= 0; i-- {
		if n := deepest(c.overlays[i].root); n != nil {
			return n
		}
	}
	return deepest(c.root)
}
