package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

type keyboardWin struct {
	damageWin
	kb    geom.Insets
	polls int
}

func (w *keyboardWin) KeyboardInset() geom.Insets { w.polls++; return w.kb }

// wholeFrame reports whether the last frame redraws the whole window: no
// hints at all, or a damage rect covering it.
func wholeFrame(r *Runner, size geom.Size) bool {
	d, s := hints(r)
	if len(d)+len(s) == 0 {
		return true
	}
	for _, x := range d {
		if x.X <= 0 && x.Y <= 0 && x.Right() >= size.W && x.Bottom() >= size.H {
			return true
		}
	}
	return false
}

func TestKeyboardInset(t *testing.T) {
	var got geom.Insets
	page := func(c *Context) {
		got = c.KeyboardInset()
		Div(c, layout.Size(100, 40))
		c.Damage(smallDamage)
	}
	w := &keyboardWin{damageWin: *newWin()}
	g := newDamageRig(t, page, w, RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.frame(t)
	if got != (geom.Insets{}) {
		t.Fatalf("hidden keyboard %v", got)
	}
	if wholeFrame(g.r, w.size) {
		t.Fatal("baseline frame is not partial")
	}
	polls := w.polls

	// The keyboard comes up: the page sees it and the frame is whole.
	w.kb = geom.Insets{Bottom: 336}
	g.frame(t)
	if got.Bottom != 336 {
		t.Fatalf("keyboard up: %v", got)
	}
	if !wholeFrame(g.r, w.size) {
		t.Fatal("change did not redraw the whole frame")
	}
	if w.polls != polls+1 {
		t.Fatalf("polled %d times in one frame", w.polls-polls)
	}
	g.frame(t)
	if wholeFrame(g.r, w.size) {
		t.Fatal("unchanged inset still redraws everything")
	}

	w.kb = geom.Insets{}
	g.frame(t)
	if got != (geom.Insets{}) || !wholeFrame(g.r, w.size) {
		t.Fatalf("keyboard hidden: %v whole=%v", got, wholeFrame(g.r, w.size))
	}

	// The host override wins over the window.
	g.r.SetKeyboardInset(geom.Insets{Bottom: 200})
	w.kb = geom.Insets{Bottom: 50}
	g.frame(t)
	if got.Bottom != 200 {
		t.Fatalf("override %v", got)
	}
	g.r.ClearKeyboardInset()
	g.frame(t)
	if got.Bottom != 50 {
		t.Fatalf("cleared override %v", got)
	}
}

func TestKeyboardInsetPlainWindow(t *testing.T) {
	var got geom.Insets
	page := func(c *Context) { got = c.KeyboardInset() }
	g := newDamageRig(t, page, newWin(), RunnerOptions{NoAnimations: true})
	g.frame(t)
	if got != (geom.Insets{}) {
		t.Fatalf("desktop window %v", got)
	}

	var dg geom.Insets
	d := NewDriver(func(c *Context) { dg = c.KeyboardInset() }, DriverOptions{})
	d.SetKeyboardInset(geom.Insets{Bottom: 300})
	d.Frame()
	if dg.Bottom != 300 {
		t.Fatalf("driver keyboard inset %v", dg)
	}
}

func TestKeyboardCover(t *testing.T) {
	var c *Context
	d := NewDriver(func(cc *Context) { c = cc }, DriverOptions{Width: 400, Height: 800})
	d.Frame()
	r := geom.Rect{Y: 300, W: 400, H: 400} // ends at 700
	if v := c.KeyboardCover(r); v != 0 {
		t.Fatalf("hidden keyboard covers %v", v)
	}
	d.SetKeyboardInset(geom.Insets{Bottom: 300}) // top at 500
	d.Frame()
	if v := c.KeyboardCover(r); v != 200 {
		t.Fatalf("cover %v, want 200", v)
	}
	if v := c.KeyboardCover(geom.Rect{Y: 600, W: 10, H: 50}); v != 50 {
		t.Fatalf("fully covered rect %v, want 50", v)
	}
	if v := c.KeyboardCover(geom.Rect{Y: 0, W: 10, H: 50}); v != 0 {
		t.Fatalf("rect above the keyboard %v", v)
	}
}

// formPage is a scroll container with rows rows of 60px and a text input
// after each of the rows listed in fields.
type formPage struct {
	rows   int
	fields []int
	vals   []string
	ids    []ID
	scroll ID
	sb     *ScrollBox
}

func (p *formPage) build(c *Context) {
	if p.vals == nil {
		p.vals = make([]string, len(p.fields))
		p.ids = make([]ID, len(p.fields))
	}
	Column(c, func() {
		Div(c, layout.H(50)) // a navigation bar
		p.sb = Scroll(c, func() {
			k := 0
			for i := 0; i < p.rows; i++ {
				Div(c, layout.H(60))
				if k < len(p.fields) && p.fields[k] == i {
					p.ids[k] = Input(c, &p.vals[k]).Placeholder(fmt.Sprintf("field %d", k)).ID()
					k++
				}
			}
		}, layout.Grow(1))
		p.scroll = p.sb.n.id
	}, layout.Grow(1))
}

func focusAt(t *testing.T, d *Driver, id ID) {
	t.Helper()
	r, ok := d.r.c.rects[id]
	if !ok {
		t.Fatalf("widget %x not painted", id)
	}
	d.Click(r.Center())
	d.Frame()
	if d.r.c.focus != id {
		t.Fatalf("click did not focus the field")
	}
}

// The keyboard comes up over the focused field at the bottom of a scroll
// container: the container scrolls (past its end, by the covered part)
// until the caret is above the keyboard.
func TestKeyboardAvoidScrollsField(t *testing.T) {
	p := &formPage{rows: 12, fields: []int{11}}
	d := NewDriver(p.build, DriverOptions{Width: 390, Height: 844})
	d.Frame()
	d.Frame()
	focusAt(t, d, p.ids[0])
	if _, y := p.sb.Offset(); y != 0 {
		t.Fatalf("scrolled before the keyboard: %v", y)
	}
	on, caret := d.TextInputActive()
	if !on || caret.Bottom() < 844-336 {
		t.Fatalf("setup: caret %v should start under the keyboard", caret)
	}
	d.SetKeyboardInset(geom.Insets{Bottom: 336})
	d.Frame() // the inset grows: a reveal is queued
	if !d.NeedsFrame() {
		t.Fatal("no frame requested for the reveal")
	}
	d.Frame() // the reveal is applied
	d.Frame()
	_, caret = d.TextInputActive()
	limit := float32(844 - 336 - keyboardMargin)
	if caret.Bottom() > limit+0.5 {
		t.Fatalf("caret %v still under the keyboard (limit %v)", caret, limit)
	}
	fr := d.r.c.rects[p.ids[0]]
	if fr.Bottom() > limit+0.5 || fr.Bottom() < limit-1 {
		t.Fatalf("field %v not placed right above the keyboard (limit %v)", fr, limit)
	}
	_, y := p.sb.Offset()
	if y <= 0 {
		t.Fatal("container did not scroll")
	}

	// The keyboard hides: the extra range goes away again.
	d.SetKeyboardInset(geom.Insets{})
	d.Frame()
	d.Frame()
	s := p.sb.s
	if s.kbPad != 0 || s.off.Y > s.maxY() {
		t.Fatalf("offset %v beyond range %v after the keyboard hid", s.off.Y, s.maxY())
	}
}

// Focus moving to another field while the keyboard is up brings that one
// above the keyboard; a field already visible is left alone.
func TestKeyboardAvoidFocusChange(t *testing.T) {
	p := &formPage{rows: 12, fields: []int{1, 11}}
	d := NewDriver(p.build, DriverOptions{Width: 390, Height: 844})
	d.Frame()
	d.Frame()
	focusAt(t, d, p.ids[0])
	d.SetKeyboardInset(geom.Insets{Bottom: 336})
	d.Frame()
	d.Frame()
	d.Frame()
	if _, y := p.sb.Offset(); y != 0 {
		t.Fatalf("visible field scrolled the container to %v", y)
	}
	// A hardware Tab moves to the field under the keyboard.
	d.Key(input.KeyTab, 0)
	d.Frame()
	if d.r.c.focus != p.ids[1] {
		t.Fatal("Tab did not reach the second field")
	}
	d.Frame()
	d.Frame()
	_, caret := d.TextInputActive()
	if caret.Bottom() > 844-336-keyboardMargin+0.5 {
		t.Fatalf("caret %v under the keyboard after focus moved", caret)
	}
}

func TestScrollIntoViewInset(t *testing.T) {
	p := &formPage{rows: 20, fields: []int{11}}
	var ok bool
	ask := false
	in := geom.Insets{Bottom: 400} // visible: window y < 444
	d := NewDriver(func(c *Context) {
		p.build(c)
		if ask {
			ok = c.ScrollIntoViewInset(p.scroll, p.ids[0], in)
			ask = false
		}
	}, DriverOptions{Width: 390, Height: 844})
	d.Frame()
	d.Frame()
	if fr := d.r.c.rects[p.ids[0]]; fr.Bottom() <= 444 {
		t.Fatalf("setup: field %v already visible", fr)
	}
	ask = true
	d.Frame()
	d.Frame()
	if !ok {
		t.Fatal("ScrollIntoViewInset found nothing")
	}
	fr := d.r.c.rects[p.ids[0]]
	if fr.Bottom() > 444.5 || fr.Bottom() < 443 {
		t.Fatalf("field %v, want its bottom at 444", fr)
	}
	if d.r.c.ScrollIntoViewInset(p.scroll, 12345, in) {
		t.Fatal("unknown widget reported success")
	}
}
