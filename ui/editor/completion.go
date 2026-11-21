package editor

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// OnDocumentChange observes each document change made by this view while
// it processes input, including undo and redo. The caret and selections
// have already been updated. Call Changed during build to process input
// there. To observe edits made outside this view's input, use Model.Subscribe.
func (w *ViewW) OnDocumentChange(fn func(ModelChange)) *ViewW {
	w.onChange = fn
	return w
}

// CompletionAnchor returns the primary caret's rectangle in UI logical
// coordinates, using the current caret and the last laid-out viewport.
// It processes pending input like Changed. The result is unavailable
// before layout or when the caret is outside the visible text area.
func (w *ViewW) CompletionAnchor() (geom.Rect, bool) {
	w.finish()
	if !w.hasGeom() || w.vs.buf != w.st.Model.Buffer() {
		return geom.Rect{}, false
	}
	st := w.st
	snap := st.Snapshot()
	off := clampOff(snap, st.Primary().Head)
	line := snap.LineOf(off)
	vl := st.lines()
	if vl.hidden(line) {
		return geom.Rect{}, false
	}
	row := 0
	if vl.on() {
		row = st.breaks(line).rowOf(off-snap.LineStart(line), st.caretUp())
	}
	v := w.displayViewport()
	y := w.vs.text.Y + float32(vl.viewOf(line)+row-v.TopLine)*v.LineHeight - v.OffsetY
	if y >= w.vs.text.Bottom() || y+v.LineHeight <= max(w.vs.text.Y, w.vs.stickyRect.Bottom()) {
		return geom.Rect{}, false
	}
	x, ok := w.contentX(snap, off)
	if !ok {
		return geom.Rect{}, false
	}
	x += w.vs.text.X - v.ScrollX
	r := geom.R(x, y, 2, v.LineHeight).Intersect(w.vs.text)
	return r, !r.Empty()
}

type completionKeys struct {
	count    int
	selected *int
	accept   func(int)
	dismiss  func()
	version  uint64
}

// CompletionKeys gives an open completion list keyboard control while
// this writable view has focus. Unmodified Up and Down wrap across count
// candidates, Home and End select the ends, Enter and Tab confirm, and
// Escape dismisses. Use -1 for no selection. Nil accept or dismiss leaves
// the corresponding keys to the editor. Pass zero count to turn it off.
// Call before Changed or CompletionAnchor; the application draws the list.
func (w *ViewW) CompletionKeys(count int, selected *int, accept func(int), dismiss func()) *ViewW {
	w.completion = completionKeys{count, selected, accept, dismiss, w.st.Snapshot().Version()}
	if count > 0 && selected != nil && !w.opts.ReadOnly && w.active() {
		if dismiss != nil && w.vs.comp == "" {
			w.c.ClaimEscape(w.box.ID())
		}
		if accept != nil && *selected >= 0 && *selected < count {
			w.c.CaptureTab(w.box.ID())
		}
	}
	return w
}

func (w *ViewW) completionKey(k input.KeyEvent) bool {
	q := &w.completion
	if !k.Down || k.Mods != 0 || k.Code == input.KeyBack || q.count <= 0 || q.selected == nil ||
		w.opts.ReadOnly || !w.active() || q.version != w.st.Snapshot().Version() {
		return false
	}
	i := *q.selected
	valid := 0 <= i && i < q.count
	switch k.Key.Base() {
	case input.KeyDown:
		if !valid {
			i = -1
		}
		*q.selected = (i + 1) % q.count
	case input.KeyUp:
		if !valid {
			i = 0
		}
		*q.selected = (i + q.count - 1) % q.count
	case input.KeyHome:
		*q.selected = 0
	case input.KeyEnd:
		*q.selected = q.count - 1
	case input.KeyEnter, input.KeyTab:
		if q.accept == nil || !valid {
			return false
		}
		q.count = 0
		q.accept(i)
	case input.KeyEscape:
		if q.dismiss == nil {
			return false
		}
		q.count = 0
		q.dismiss()
	default:
		return false
	}
	w.keymap().ClearPending()
	w.c.Invalidate()
	return true
}
