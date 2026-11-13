package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestEntryTransferCheckMoveAndButtons(t *testing.T) {
	items := []TransferItem{{Key: "a", Title: "甲"}, {Key: "b", Title: "乙"}, {Key: "c", Title: "丙", Disabled: true}, {Key: "d", Title: "丁"}}
	var target []string
	moved := 0
	var w *TransferW
	d := NewDriver(func(c *Context) {
		w = Transfer(c, items, &target).Operations("右移", "左移")
		if w.Changed() {
			moved++
		}
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	// Buttons are disabled while nothing is checked.
	d.ClickLabel("右移")
	if len(target) != 0 || moved != 0 {
		t.Fatal("disabled move button acted")
	}
	d.ClickLabel("甲")
	d.ClickLabel("丙") // disabled item cannot be checked
	if got := w.Checked(0); len(got) != 1 || got[0] != "a" {
		t.Fatalf("checked = %v", got)
	}
	d.ClickLabel("右移")
	if len(target) != 1 || target[0] != "a" || moved != 1 {
		t.Fatalf("target = %v moved=%d", target, moved)
	}
	if len(w.Checked(0)) != 0 {
		t.Fatal("check survived the move")
	}
	// Header check-all of the left list checks every enabled item.
	r := mustFind(t, d, "乙")
	d.Click(geom.Pt(r.X+12+8, r.Y-24))
	if got := w.Checked(0); len(got) != 2 {
		t.Fatalf("check all = %v", got)
	}
	d.Click(geom.Pt(r.X+12+8, r.Y-24))
	if got := w.Checked(0); len(got) != 0 {
		t.Fatalf("uncheck all = %v", got)
	}
	// Move back.
	d.ClickLabel("甲")
	d.ClickLabel("左移")
	if len(target) != 0 {
		t.Fatalf("target = %v", target)
	}
}

func TestEntryTransferSearchAndDisabled(t *testing.T) {
	items := []TransferItem{{Key: "a", Title: "苹果"}, {Key: "b", Title: "香蕉"}}
	var target []string
	var w *TransferW
	dis := false
	d := NewDriver(func(c *Context) {
		w = Transfer(c, items, &target).Searchable().Disabled(dis)
	}, DriverOptions{Width: 800, Height: 500})
	d.Frame()
	w.st.query[0] = "香"
	d.Frame()
	if _, ok := d.Find("苹果"); ok {
		t.Fatal("filter did not hide item")
	}
	if got := w.list(0, true); len(got) != 1 || got[0].Key != "b" {
		t.Fatalf("filtered = %v", got)
	}
	dis = true
	d.Frame()
	d.ClickLabel("香蕉")
	if len(w.Checked(0)) != 0 {
		t.Fatal("disabled transfer checked an item")
	}
}
