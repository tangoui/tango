package ui

import "github.com/tangoui/tango/ui/geom"

// Soft-keyboard inset and keyboard avoidance (docs/specs/mobile-canvas.md
// §2.4). The keyboard is not part of the safe area: it comes and goes
// while a text field has focus, and only views that must stay visible
// above it (forms, chat inputs, bottom sheets) move out of its way.
//
// The Runner reads the inset at the start of every frame, like the safe
// area; a change makes the frame a full redraw. When the inset grows, or
// focus moves while the keyboard is up, and the caret of the focused text
// field ends below the keyboard's top edge (minus a small margin), the
// Runner asks the field's nearest scroll container to bring it into view:
// a touch scroll view built on its own scroller claims the request during
// the next build (PendingKeyboardReveal), otherwise the innermost ui
// scroll container around the field is scrolled after that build
// (ScrollIntoViewInset). While the keyboard covers part of a ui scroll
// container, the container may scroll that much further, so content at
// its end can be lifted above the keyboard.

// KeyboardInsetWindow is implemented by windows that know the part of
// their content area covered by the soft keyboard (usually only Bottom;
// zero while it is hidden). It is polled every frame like SafeAreaWindow,
// so it must return a cached value.
type KeyboardInsetWindow interface {
	KeyboardInset() geom.Insets
}

// keyboardMargin is the gap kept between the caret and the keyboard.
const keyboardMargin = 8

// kbState is the keyboard inset in effect, the host override and the
// pending avoidance request.
type kbState struct {
	cur      geom.Insets
	override geom.Insets
	set      bool // override in use
	seen     bool // cur was computed by a frame

	// Avoidance (Runner.checkKeyboard / applyKeyboardReveal).
	lastBottom float32 // inset bottom when avoidance last looked
	lastFocus  ID      // focus when avoidance last looked
	reveal     KeyboardReveal
	pending    bool // reveal waits for the next build
	claimed    bool // a scroll view handled reveal this frame
}

// KeyboardReveal is a request to scroll the focused text field above the
// soft keyboard. Rectangles are window logical pixels from the last
// painted frame.
type KeyboardReveal struct {
	Field ID        // the focused widget
	Rect  geom.Rect // its rectangle
	Caret geom.Rect // its caret (Context.WantTextInput)
	// Limit is the window y the revealed part must end above: the top of
	// the keyboard minus a margin.
	Limit float32
}

// Target returns what to bring into view in a viewport whose visible part
// starts at window y top: the whole field (with its caret) when it fits
// between top and Limit, else only the caret line.
func (k KeyboardReveal) Target(top float32) geom.Rect {
	if k.Rect.Empty() {
		return k.Caret
	}
	if u := k.Rect.Union(k.Caret); u.H <= k.Limit-top {
		return u
	}
	return k.Caret
}

// SetKeyboardInset sets the window's soft-keyboard inset (logical pixels),
// taking precedence over KeyboardInsetWindow, and requests a frame. Hosts
// that are told about keyboard changes call it instead of implementing
// KeyboardInsetWindow. UI thread.
func (r *Runner) SetKeyboardInset(in geom.Insets) {
	r.c.kb.override, r.c.kb.set = in, true
	r.Invalidate()
}

// ClearKeyboardInset removes the SetKeyboardInset override. UI thread.
func (r *Runner) ClearKeyboardInset() {
	r.c.kb.set = false
	r.Invalidate()
}

// KeyboardInset returns the part of the window covered by the soft
// keyboard, in logical pixels: usually only Bottom, zero while the
// keyboard is hidden and on desktops. Views that must stay visible pad or
// lift themselves by it (with SafeArea, take the larger bottom of the
// two). A change redraws the whole frame.
func (c *Context) KeyboardInset() geom.Insets { return c.kb.cur }

// KeyboardCover returns how much of the bottom of r (window logical
// pixels) the soft keyboard hides; 0 while it is hidden.
func (c *Context) KeyboardCover(r geom.Rect) float32 {
	kb := c.kb.cur.Bottom
	if kb <= 0 {
		return 0
	}
	return max(0, min(r.H, r.Bottom()-(c.size.H-kb)))
}

// keyboardPad is the extra scroll range of a container at r while the
// keyboard covers it: the covered height plus the margin kept above the
// keyboard.
func (c *Context) keyboardPad(r geom.Rect) float32 {
	if v := c.KeyboardCover(r); v > 0 {
		return v + keyboardMargin
	}
	return 0
}

// PendingKeyboardReveal returns the keyboard avoidance request of this
// frame, if any. A scroll container that moves its content itself (not a
// ui scroll container) checks that Field lies in its content, scrolls so
// that Target ends above Limit and calls ClaimKeyboardReveal. Unclaimed
// requests go to the innermost ui scroll container around the field after
// the build.
func (c *Context) PendingKeyboardReveal() (KeyboardReveal, bool) {
	k := &c.kb
	if !k.pending || k.reveal.Field != c.focus {
		return KeyboardReveal{}, false
	}
	return k.reveal, true
}

// ClaimKeyboardReveal marks the pending keyboard avoidance request as
// handled (see PendingKeyboardReveal). The frame is redrawn completely.
func (c *Context) ClaimKeyboardReveal() {
	if c.kb.pending {
		c.kb.pending, c.kb.claimed = false, true
	}
}

// updateKeyboardInset reads the inset for this frame; a change makes the
// frame a full redraw.
func (r *Runner) updateKeyboardInset() {
	k := &r.c.kb
	in := k.override
	if !k.set {
		in = geom.Insets{}
		if w, ok := r.win.(KeyboardInsetWindow); ok {
			in = w.KeyboardInset()
		}
	}
	if k.seen && in != k.cur {
		r.dmg.started = false // layout may move everywhere: redraw it all
	}
	k.cur, k.seen = in, true
	k.claimed = false
}

// checkKeyboard runs after a frame: when the keyboard grew or focus moved
// while it is up and the focused field's caret is under it, it queues a
// reveal for the next frame.
func (r *Runner) checkKeyboard() {
	c := r.c
	k := &c.kb
	kb := k.cur.Bottom
	grew := kb > k.lastBottom
	moved := c.focus != k.lastFocus
	k.lastBottom, k.lastFocus = kb, c.focus
	if kb <= 0 || !c.wantText || c.focus == 0 || !(grew || moved) {
		return
	}
	limit := c.size.H - kb - keyboardMargin
	caret := c.textCaret
	if caret.Bottom() <= limit {
		return
	}
	k.reveal = KeyboardReveal{Field: c.focus, Rect: c.rects[c.focus], Caret: caret, Limit: limit}
	k.pending = true
	r.win.Invalidate()
}

// applyKeyboardReveal hands a reveal no scroll view claimed during the
// build to the innermost ui scroll container around the field. It runs
// between build and paint, on the last frame's geometry.
func (r *Runner) applyKeyboardReveal() {
	c := r.c
	k := &c.kb
	if k.claimed {
		r.dmg.started = false
	}
	if !k.pending {
		return
	}
	k.pending = false
	if k.reveal.Field != c.focus {
		return
	}
	sid, ok := c.scrollAncestor(k.reveal.Field)
	if !ok {
		return
	}
	in := k.cur
	in.Bottom += keyboardMargin
	vr := c.rects[sid]
	top := max(vr.Y, in.Top)
	if changed, _ := c.revealInset(sid, k.reveal.Target(top), in); changed {
		r.dmg.started = false
	}
}

// scrollAncestor returns the innermost ui scroll container of the last
// frame whose content holds widget id's hit.
func (c *Context) scrollAncestor(id ID) (ID, bool) {
	hi := -1
	for i := len(c.hits) - 1; i >= 0; i-- {
		if c.hits[i].id == id {
			hi = i
			break
		}
	}
	if hi < 0 {
		return 0, false
	}
	best, from := ID(0), -1
	for _, t := range c.prevScroll {
		if hi > t.from && hi < t.to && t.from > from {
			best, from = t.id, t.from
		}
	}
	return best, from >= 0
}

// SetKeyboardInset sets the keyboard inset the page sees
// (Runner.SetKeyboardInset); the next frame picks it up.
func (d *Driver) SetKeyboardInset(in geom.Insets) { d.r.SetKeyboardInset(in) }
