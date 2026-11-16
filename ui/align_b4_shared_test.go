package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
	"testing"
	"time"
)

func TestProCardBoxShadowHoverKeepsBase(t *testing.T) {
	var card *ProCardW
	th := theme.AntDesign()
	d := NewDriver(func(c *Context) {
		card = ProCard(c, "Card").BoxShadow(true).Hoverable().Body(func() { Text(c, "Body") })
	}, DriverOptions{Width: 600, Height: 400, Theme: th, Animations: true})
	d.Frame()
	d.Move(geom.Point{X: 590, Y: 390})
	d.Advance(time.Second)
	d.Frame()
	check := func() {
		t.Helper()
		if len(card.box.shadows) != len(th.ShadowTertiary) {
			t.Fatal("base shadow missing")
		}
		for i, s := range card.box.shadows {
			if s.Color.A != th.ShadowTertiary[i].Color.A {
				t.Fatalf("shadow flashed: alpha %v want %v", s.Color.A, th.ShadowTertiary[i].Color.A)
			}
		}
	}
	check()
	d.Move(card.box.Rect().Center())
	d.Frame()
	d.Advance(time.Millisecond * 16)
	d.Frame()
	check()
	d.Move(geom.Point{X: 590, Y: 390})
	d.Frame()
	d.Advance(time.Millisecond * 16)
	d.Frame()
	check()
}

func TestTableFilterResetDisabledIgnoresOrder(t *testing.T) {
	if !tblSameFilterSet([]string{"b", "a"}, []string{"a", "b"}) || tblSameFilterSet(nil, []string{"a"}) {
		t.Fatal("reset-disabled comparison must be an unordered set comparison")
	}
}

// A fixed FooterToolbar's empty area must not pass clicks to content
// underneath; only its full-screen positioning layer is transparent.
func TestFooterToolbarFixedBlocksClickThrough(t *testing.T) {
	under := 0
	var bar *FooterToolbarW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Spacer(c)
			if Button(c, "under").Layout(layout.W(600), layout.H(60)).Clicked() {
				under++
			}
		}, layout.H(400))
		bar = FooterToolbar(c).Body(func() { Button(c, "ok") })
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	r := bar.box.Rect()
	if r.Bottom() != 400 {
		t.Fatalf("bar bottom %v, want 400", r.Bottom())
	}
	d.Click(geom.Point{X: r.X + 20, Y: r.Y + r.H/2})
	d.Settle()
	if under != 0 {
		t.Fatal("click on the toolbar's empty area reached the content underneath")
	}
}
