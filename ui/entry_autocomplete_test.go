package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestEntryAutoCompleteFilterKeyboardPick(t *testing.T) {
	val := ""
	var w *AutoCompleteW
	picked := ""
	d := driver(func(c *Context) {
		Column(c, func() {
			w = AutoComplete(c, &val, Opts("Wall Street", "Baker Street", "Downing Street", "Abbey Road")).Width(240)
			if v, ok := w.Selected(); ok {
				picked = v
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(geom.Pt(60, 36))
	d.Type("street")
	if !w.IsOpen() {
		t.Fatal("suggestions not shown")
	}
	if _, ok := d.Find("Abbey Road"); ok {
		t.Fatal("non-matching suggestion shown")
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if val != "Baker Street" || picked != "Baker Street" || w.IsOpen() {
		t.Fatalf("keyboard pick: %q %q open=%v", val, picked, w.IsOpen())
	}
	d.Key(input.KeyBackspace, 0)
	if !w.IsOpen() {
		t.Fatal("editing should reopen")
	}
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() {
		t.Fatal("escape did not close")
	}
	d.Type("x")
	d.Key(input.KeyBackspace, 0)
	d.ClickLabel("Baker Street")
	if val != "Baker Street" {
		t.Fatalf("click pick: %q", val)
	}
}

func TestEntryAutoCompleteSourceDisabled(t *testing.T) {
	val := ""
	var w, w2 *AutoCompleteW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = AutoComplete(c, &val, nil).Width(240).Source(func(q string) []Option {
				if q == "" {
					return nil
				}
				return Opts(q+"@qq.com", q+"@163.com")
			})
			w2 = AutoComplete(c, &val, Opts("a")).Width(240).Disabled(true)
		}, layout.Pad(20), layout.Gap(20))
	})
	d.Frame()
	d.Click(geom.Pt(60, 36))
	if w.IsOpen() {
		t.Fatal("empty source should stay closed")
	}
	d.Type("tom")
	if _, ok := d.Find("tom@163.com"); !ok {
		t.Fatal("dynamic suggestion missing")
	}
	d.Click(geom.Pt(60, 88))
	if w2.IsOpen() {
		t.Fatal("disabled opened")
	}
}
