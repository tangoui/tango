package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestDatePickerKeyboardAndDisabled(t *testing.T) {
	v := dtJan(15)
	changed := 0
	var w *DatePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DatePicker(c, &v).DisabledDate(func(d time.Time) bool { return d.Day() == 16 })
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.n.id].Center())
	if !w.IsOpen() {
		t.Fatal("click should open the panel")
	}
	// A disabled day does not pick.
	d.Click(mustFind(t, d, "2026-01-16").Center())
	if !v.Equal(dtJan(15)) || !w.IsOpen() || changed != 0 {
		t.Fatalf("disabled day picked: %v open=%v", v, w.IsOpen())
	}
	// Keyboard: the cursor starts at the value; Down = +7 days.
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if !v.Equal(dtJan(23)) || changed != 1 || w.IsOpen() {
		t.Fatalf("keyboard pick = %v changed=%d open=%v", v, changed, w.IsOpen())
	}
	// Enter reopens; PageDown moves the panel to the next month.
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyPageDown, 0)
	d.Settle()
	if _, ok := d.Find("2026-02-28"); !ok || !w.IsOpen() {
		t.Fatal("PageDown should show February")
	}
	d.Click(mustFind(t, d, "2026-02-10").Center())
	if !v.Equal(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)) || w.IsOpen() {
		t.Fatalf("click pick = %v", v)
	}
}

func TestDateUtilHM(t *testing.T) {
	if dtParseHM(" 9:05 ") != 545 || dtParseHM("x") != -1 || dtParseHM("9:75") != -1 {
		t.Fatal("dtParseHM")
	}
	if dtFormatHM(545) != "09:05" || dtFormatHM(23*60+59) != "23:59" {
		t.Fatal("dtFormatHM")
	}
	if names := dtWeekNames(ZhCN, time.Monday); names[0] != "一" || names[6] != "日" {
		t.Fatalf("week names %v", names)
	}
	if g := dtGridStart(dtJan(1), time.Monday); !g.Equal(time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("grid start %v", g)
	}
}

func TestDisplayLazyBuilders(t *testing.T) {
	var badge *BadgeW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			badge = Badge(c).Status("success", "成功")
			Badge(c).Count(5).Body(func() { Button(c, "inbox") })
			Empty(c).Simple().Description("没有订单")
			Descriptions(c, []DescItem{{Label: "姓名", Value: "张三"}}).Title("信息").Bordered()
			Space(c).Split(func() { VDivider(c) }).Body(func() { Text(c, "x"); Text(c, "y") })
		})
	}, DriverOptions{})
	d.Settle()
	var labels []string
	var walk func(n *node)
	walk = func(n *node) {
		labels = append(labels, n.label)
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	for _, s := range []string{"成功", "inbox", "没有订单", "张三", "信息"} {
		if !contains(labels, s) {
			t.Errorf("%q missing", s)
		}
	}
	if r := d.r.c.rects[badge.n.id]; r.W < 30 {
		t.Errorf("status badge should measure its label: %v", r)
	}
}
