package ui

import (
	"sync"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// File drag feedback: hosts report files dragged over a window
// (Runner.FileDragOver / FileDragLeave, from platform.CanvasDropTarget in
// uihost) and dropped (Runner.DropFiles); pages read it through DropZone
// or Context.FileDrag.
//
// The state lives beside the Runner (keyed by its Context), so the
// Runner and Context types need no new fields.

// fileDragState is the drag / drop state of one window. UI thread only;
// the map holding it is shared by all windows.
type fileDragState struct {
	over bool
	pos  geom.Point

	pass   uint64 // build pass the zones below were collected in
	target ID     // zone under the drag, resolved at the end of the last pass
	next   ID     // candidate of the current pass (the last zone built wins)
	spoken ID     // zone whose "release to drop" was announced

	drops    []fileDrop // dropped, not yet resolved to a zone
	dropNext ID
	claim    fileDrop // resolved drop waiting for its zone's Dropped
	claimID  ID
	claimAt  uint64
}

type fileDrop struct {
	pos   geom.Point
	paths []string
}

var fileDrags sync.Map // *Context -> *fileDragState

func fileDragOf(c *Context, create bool) *fileDragState {
	if v, ok := fileDrags.Load(c); ok {
		return v.(*fileDragState)
	}
	if !create {
		return nil
	}
	v, _ := fileDrags.LoadOrStore(c, &fileDragState{})
	return v.(*fileDragState)
}

// FileDragOver reports files dragged over the window at p (window logical
// pixels), when the drag enters and as it moves. Hosts call it on the UI
// thread (uihost does from platform.CanvasDropTarget).
func (r *Runner) FileDragOver(p geom.Point) {
	st := fileDragOf(r.c, true)
	if st.over && st.pos == p {
		return
	}
	st.over, st.pos = true, p
	r.Invalidate()
}

// FileDragLeave reports that the file drag left the window, was cancelled
// or was dropped (hosts call it before DropFiles).
func (r *Runner) FileDragLeave() {
	st := fileDragOf(r.c, false)
	if st == nil || !st.over {
		return
	}
	st.over, st.target, st.next, st.spoken = false, 0, 0, 0
	st.forget(r.c)
	r.Invalidate()
}

// forget drops an idle state (no drag, no pending or unread drop), so
// closed windows leave nothing behind.
func (st *fileDragState) forget(c *Context) {
	if !st.over && len(st.drops) == 0 && st.claimID == 0 {
		fileDrags.Delete(c)
	}
}

// FileDrag returns the position of a file drag over the window.
func (r *Runner) FileDrag() (geom.Point, bool) { return r.c.FileDrag() }

// DropFiles delivers files dropped at p (window logical pixels) to the
// DropZone under it: its Dropped returns them during the next frame.
// Files dropped outside every zone are discarded. UI thread only.
func (r *Runner) DropFiles(p geom.Point, paths []string) {
	if len(paths) == 0 {
		return
	}
	st := fileDragOf(r.c, true)
	st.drops = append(st.drops, fileDrop{pos: p, paths: append([]string(nil), paths...)})
	r.Invalidate()
}

// FileDrag reports a file drag over the window and its position (window
// logical pixels). It is read-only: use DropZone for hit testing and
// highlighting.
func (c *Context) FileDrag() (geom.Point, bool) {
	st := fileDragOf(c, false)
	if st == nil || !st.over {
		return geom.Point{}, false
	}
	return st.pos, true
}

// FileDragOver simulates files dragged over the window at p and runs a
// frame (tests).
func (d *Driver) FileDragOver(p geom.Point) {
	d.r.FileDragOver(p)
	d.Settle()
}

// FileDragLeave simulates the file drag leaving the window.
func (d *Driver) FileDragLeave() {
	d.r.FileDragLeave()
	d.Settle()
}

// DropFiles simulates dropping files at p (after FileDragLeave, as hosts
// do) and runs a frame.
func (d *Driver) DropFiles(p geom.Point, paths ...string) {
	d.r.FileDragLeave()
	d.r.DropFiles(p, paths)
	d.Frame()
}

// DropZoneW is an area that reacts to files dragged over the window
// (Ant Design Upload.Dragger's hover state, as a standalone wrapper):
//
//	z := ui.DropZone(c, func(over bool) {
//		ui.Upload(c, &files).Dragger().DragOver(over)
//	}).Highlight(false)
//	if paths := z.Dropped(); paths != nil { ... }
//
// Nested zones: the innermost (last built) zone under the drag wins.
type DropZoneW struct {
	_ [0]func()

	c         *Context
	box       *Box
	st        *fileDragState
	over      bool
	highlight bool
}

// DropZone builds body in a zone; body receives whether files are dragged
// over the zone (from the previous build pass, so it is stable within a
// frame).
func DropZone(c *Context, body func(over bool)) *DropZoneW {
	th := c.Theme
	z := &DropZoneW{c: c, highlight: true}
	z.box = c.box("dropzone", layout.Column, nil)
	id := z.box.n.id
	z.box.n.setRole(a11y.RoleGroup).setName(c.Locale().DropZone.Name)
	if st := fileDragOf(c, false); st != nil {
		z.st = st
		z.collect(id)
		z.over = st.over && st.target == id
		if z.over && st.spoken != id {
			st.spoken = id
			c.Announce(c.Locale().DropZone.Release, a11y.LivePolite)
		}
	}
	// The highlight fades through Anim: the Runner damages the zone's rect
	// whenever the value changes, so no Damage call is needed here.
	k := float32(0)
	if z.over {
		k = 1
	}
	a := c.Anim(id, "file-drag", k, th.DurationFast)
	// Tint below the content, dashed outline above it.
	z.box.Paint(func(l *paint.List, r geom.Rect) {
		if z.highlight && a > 0 {
			l.Fill(r, geom.Rad(th.RadiusLG), theme.Fade(th.Primary.Bg, float64(a)))
		}
	})
	z.box.PaintOver(func(l *paint.List, r geom.Rect) {
		if z.highlight && a > 0 {
			l.StrokeDashed(r, geom.Rad(th.RadiusLG), 1, theme.Fade(th.Primary.Base, float64(a)), []float32{4, 3})
		}
	})
	if body != nil {
		over := z.over
		z.box.Body(func() { body(over) })
	}
	return z
}

// collect records the zone as a candidate when the drag (or a pending
// drop) is over its last rect, and resolves the winners once per pass.
func (z *DropZoneW) collect(id ID) {
	c, st := z.c, z.st
	if st.pass != c.pass {
		st.pass, st.next, st.dropNext = c.pass, 0, 0
		c.deferred = append(c.deferred, func() { st.resolve(c) })
	}
	r, ok := c.rects[id]
	if !ok {
		return
	}
	if st.over && r.Contains(st.pos) {
		st.next = id
	}
	if len(st.drops) > 0 && r.Contains(st.drops[0].pos) {
		st.dropNext = id
	}
}

// resolve runs after a build pass that had zones: the topmost candidate
// becomes the target; a pending drop goes to its zone.
func (st *fileDragState) resolve(c *Context) {
	if !st.over {
		st.next = 0
	}
	if st.next != st.target {
		st.target = st.next
		c.invalid = true
	}
	if st.claimID != 0 && c.pass > st.claimAt+3 {
		st.claimID, st.claim = 0, fileDrop{} // nobody read it
	}
	if len(st.drops) > 0 {
		d := st.drops[0]
		st.drops = st.drops[1:]
		if st.dropNext != 0 {
			st.claim, st.claimID, st.claimAt = d, st.dropNext, c.pass
			c.invalid = true
		}
		if len(st.drops) > 0 {
			c.invalid = true
		}
	}
	st.forget(c)
}

// Highlight toggles the built-in highlight (a tinted, dashed overlay;
// default on). Turn it off when the content shows its own (Upload's
// Dragger with DragOver).
func (z *DropZoneW) Highlight(on bool) *DropZoneW { z.highlight = on; return z }

// A11yLabel names the zone for screen readers ("" keeps the default).
func (z *DropZoneW) A11yLabel(s string) *DropZoneW {
	if s != "" {
		z.box.n.setName(s)
	}
	return z
}

// Layout applies layout options to the zone.
func (z *DropZoneW) Layout(o ...layout.Option) *DropZoneW { z.box.Style(o...); return z }

// Over reports files dragged over the zone.
func (z *DropZoneW) Over() bool { return z.over }

// Dropped returns the paths of files dropped on the zone (Runner.DropFiles)
// once, or nil.
func (z *DropZoneW) Dropped() []string {
	st := z.st
	if st == nil || st.claimID == 0 || st.claimID != z.box.n.id {
		return nil
	}
	p := st.claim.paths
	st.claimID, st.claim = 0, fileDrop{}
	return p
}

// ID returns the zone's widget ID.
func (z *DropZoneW) ID() ID { return z.box.n.id }
