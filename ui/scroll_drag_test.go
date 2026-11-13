package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestScrollbarDragAndTrackClick(t *testing.T) {
	clicked := -1
	var sb *ScrollBox
	d := driver(func(c *Context) {
		sb = Scroll(c, func() {
			for i := 0; i < 50; i++ {
				Key(c, i, func() {
					if Button(c, fmt.Sprintf("行 %d", i)).Block().Clicked() {
						clicked = i
					}
				})
			}
		}, layout.H(300), layout.W(400))
	})
	d.Frame()
	d.Frame()
	r := d.r.c.rects[sb.ID()]
	x := r.Right() - 3
	maxOff := sb.s.maxY()
	d.Drag(geom.Pt(x, r.Y+10), geom.Pt(x, r.Bottom()+40))
	if _, y := sb.Offset(); y != maxOff {
		t.Fatalf("drag to end: %v want %v", y, maxOff)
	}
	if clicked != -1 {
		t.Fatalf("scrollbar press clicked row %d", clicked)
	}
	// Track click above the thumb jumps it to the pointer.
	d.Click(geom.Pt(x, r.Y+r.H/3))
	if _, off := sb.Offset(); off <= 0 || off >= maxOff {
		t.Fatalf("track click offset %v", off)
	}
	if clicked != -1 {
		t.Fatal("track click reached a row")
	}
	// Rows still take clicks away from the strip.
	d.Click(geom.Pt(r.X+50, r.Y+20))
	if clicked < 0 {
		t.Fatal("row click lost")
	}
	if _, ok := d.FindText("行 0"); !ok && clicked < 0 {
		t.Fatal("FindText")
	}
}

func TestDriverFindTextRichText(t *testing.T) {
	d := driver(func(c *Context) { Column(c, func() { Text(c, "纯文本") }, layout.Pad(10)) })
	d.Frame()
	r, ok := d.FindText("纯文本")
	if !ok || r.X != 10 || r.W <= 0 {
		t.Fatalf("FindText %v %v", r, ok)
	}
	if _, ok := d.Find("纯文本"); ok {
		t.Fatal("Find should only see interactive widgets")
	}
}
