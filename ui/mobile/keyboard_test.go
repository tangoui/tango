package mobile

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// The soft keyboard comes up over a field at the end of a ScrollView: the
// view scrolls (past its end by the covered height) until the field is
// above the keyboard, and returns into range when the keyboard hides.
func TestScrollViewKeyboardAvoidance(t *testing.T) {
	var c *ui.Context
	var sv *ScrollViewW
	val := ""
	var field ui.ID
	d := phone(func(cc *ui.Context) {
		c = cc
		ui.Column(c, func() {
			NavBar(c, "Form")
			sv = ScrollView(c).Layout(layout.Grow(1)).Body(func() {
				Form(c).Body(func() {
					for i := 0; i < 12; i++ {
						ui.Key(c, i, func() { Cell(c, fmt.Sprint("row ", i)) })
					}
					FormItem(c, "Note").Body(func() {
						field = Field(c, &val).Type(input.TextEmail).Input().ID()
					})
				})
			})
		}, layout.Grow(1))
	}, false)
	d.Frame()
	d.Frame()
	fr, ok := c.RectOf(field)
	if !ok || fr.Bottom() > 844 || fr.Bottom() < 844-336 {
		t.Fatalf("setup: field %v should be visible but under the keyboard", fr)
	}
	d.Click(fr.Center())
	d.Frame()
	if c.Focused() != field {
		t.Fatal("tap did not focus the field")
	}
	if st, ok := d.TextInputState(); !ok || st.Hints.Kind != input.TextEmail {
		t.Fatalf("field hints %+v (%v)", st.Hints, ok)
	}
	pos := sv.ScrollOffset()
	d.SetKeyboardInset(geom.Insets{Bottom: 336})
	d.Frame()
	d.Frame()
	d.Frame()
	limit := float32(844 - 336 - 8)
	_, caret := d.TextInputActive()
	fr, _ = c.RectOf(field)
	if caret.Bottom() > limit+0.5 || fr.Bottom() > limit+0.5 {
		t.Fatalf("field %v caret %v still under the keyboard (limit %v)", fr, caret, limit)
	}
	if sv.ScrollOffset() <= pos {
		t.Fatal("the view did not scroll")
	}

	d.SetKeyboardInset(geom.Insets{})
	d.Settle()
	st := sv.st
	if sv.ScrollOffset() > max(0, st.contentH-st.viewH)+0.5 {
		t.Fatalf("offset %v left past the end %v", sv.ScrollOffset(), st.contentH-st.viewH)
	}
}

// Field.Type sets the keyboard hints; a password type masks the text.
func TestFieldType(t *testing.T) {
	for _, k := range []input.TextKind{input.TextNumber, input.TextPhone, input.TextPassword} {
		val := "12"
		var id ui.ID
		var c *ui.Context
		d := phone(func(cc *ui.Context) {
			c = cc
			Form(c).Body(func() {
				FormItem(c, "F").Body(func() { id = Field(c, &val).Type(k).Input().ID() })
			})
		}, false)
		d.Frame()
		c.SetFocus(id)
		d.Frame()
		d.Frame()
		st, ok := d.TextInputState()
		if !ok || st.Hints.Kind != k {
			t.Errorf("%v: hints %+v", k, st.Hints)
			continue
		}
		if k == input.TextPassword && (st.Text != "••" || st.Hints.Autocorrect()) {
			t.Errorf("password state %+v", st)
		}
	}
}

// Bottom sheets keep their content above the keyboard: plain popups add the
// keyboard's height under the content, ActionSheet and Picker use the
// larger of the safe area and the keyboard.
func TestBottomSheetsAboveKeyboard(t *testing.T) {
	open := true
	var c *ui.Context
	var body ui.ID
	d := phone(func(cc *ui.Context) {
		c = cc
		Popup(c, &open).Body(func() { body = ui.Div(c, layout.H(200)).ID() })
	}, false)
	d.Settle()
	r, _ := c.RectOf(body)
	if r.Bottom() < 843.5 {
		t.Fatalf("popup content %v not at the bottom", r)
	}
	d.SetKeyboardInset(geom.Insets{Bottom: 300})
	d.Settle()
	r, _ = c.RectOf(body)
	if r.Bottom() > 544.5 || r.Bottom() < 543.5 {
		t.Fatalf("popup content %v, want its bottom at the keyboard (544)", r)
	}

	as := true
	var cancel geom.Rect
	d2 := phone(func(cc *ui.Context) {
		c = cc
		ActionSheet(c, &as, []Action{{Text: "Copy"}}).Cancel("Cancel").Show()
	}, false)
	d2.SetSafeArea(geom.Insets{Bottom: 34})
	d2.Settle()
	cancel, _ = d2.FindText("Cancel")
	safeGap := 844 - cancel.Bottom()
	d2.SetKeyboardInset(geom.Insets{Bottom: 300})
	d2.Settle()
	cancel, _ = d2.FindText("Cancel")
	if gap := 844 - cancel.Bottom(); gap < 300 || gap > safeGap+300-34+0.5 {
		t.Fatalf("cancel row gap %v (safe-area gap %v)", gap, safeGap)
	}
}
