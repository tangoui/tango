package ui

import (
	"strconv"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// benchVirtualTable is a 100 000 × 50 table in a 1440×900 window.
func benchVirtualTable(scale float32) *Driver {
	cols := []TableColumn{
		{Key: "id", Title: "编号", Width: 100, Fixed: FixedLeft, Text: func(i int) string { return "#" + strconv.Itoa(i+1) }},
		{Key: "name", Title: "姓名", Width: 100, Fixed: FixedLeft, Text: func(i int) string { return "用户" + strconv.Itoa(i%997) }},
	}
	for k := 0; k < 47; k++ {
		k := k
		cols = append(cols, TableColumn{Key: "c" + strconv.Itoa(k), Title: "指标" + strconv.Itoa(k), Width: 120, Align: AlignRight,
			Text: func(i int) string { return strconv.Itoa((i*31 + k*17) % 100000) }})
	}
	cols = append(cols, TableColumn{Key: "op", Title: "操作", Width: 90, Fixed: FixedRight, Text: func(i int) string { return "查看" }})
	var sel []int
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Table(c, cols, 100000).Selection(&sel).ScrollY(760).Virtual().Show()
		}, layout.Pad(24))
	}, DriverOptions{Width: 1440, Height: 900, Scale: scale})
	d.Settle()
	return d
}

// BenchmarkTableVirtualScroll measures one frame (build + layout + paint
// list + CPU rasterization) while wheel-scrolling a 100 000 × 50 table at
// 2x scale.
func BenchmarkTableVirtualScroll(b *testing.B) {
	d := benchVirtualTable(2)
	p := d.r.c.rects[d.r.c.root.kids[0].id].Center()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(120)
		if (i/400)%2 == 1 {
			dy = -120
		}
		d.Inject(input.Scroll{Pos: p, DY: dy})
		d.Frame()
	}
}

// BenchmarkTableVirtualScrollNoRaster measures build + layout + display
// list only (what a GPU host would pay on the CPU).
func BenchmarkTableVirtualScrollNoRaster(b *testing.B) {
	d := benchVirtualTable(2)
	p := d.r.c.rects[d.r.c.root.kids[0].id].Center()
	r := d.r
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Inject(input.Scroll{Pos: p, DY: 120})
		r.c.now = d.Now()
		r.processEvents()
		r.c.invalid = false
		r.build()
		r.clearFrameInput()
		r.layoutAndPaint()
		r.afterFrame()
	}
}

// TestTableVirtualFrameTime logs per-frame timings (go test -run FrameTime -v).
func TestTableVirtualFrameTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	d := benchVirtualTable(2)
	p := d.r.c.rects[d.r.c.root.kids[0].id].Center()
	const n = 120
	var worst, sum time.Duration
	for i := 0; i < n; i++ {
		d.Inject(input.Scroll{Pos: p, DY: 37 * 3})
		t0 := time.Now()
		d.Frame()
		dt := time.Since(t0)
		sum += dt
		worst = max(worst, dt)
	}
	// A jump (scrollbar drag to the middle) shapes a whole new screen.
	d.Inject(input.Scroll{Pos: p, DY: 2e6})
	t0 := time.Now()
	d.Frame()
	jump := time.Since(t0)
	t.Logf("100000x50 virtual table, 1440x900 @2x: avg %v, worst %v per scroll frame; jump frame %v", sum/n, worst, jump)
}

// benchVirtualWrapTable is benchVirtualTable with a wrapping column, so rows
// have variable (measured) heights.
func benchVirtualWrapTable(scale float32) *Driver {
	notes := []string{"短说明", "这一行的说明比较长，需要在单元格里换行显示，行高随内容增长。", "中等长度的说明文字。"}
	cols := []TableColumn{
		{Key: "id", Title: "编号", Width: 100, Fixed: FixedLeft, Text: func(i int) string { return "#" + strconv.Itoa(i+1) }},
		{Key: "note", Title: "说明", Width: 220, Wrap: true, Text: func(i int) string { return notes[i%len(notes)] }},
	}
	for k := 0; k < 46; k++ {
		k := k
		cols = append(cols, TableColumn{Key: "c" + strconv.Itoa(k), Title: "指标" + strconv.Itoa(k), Width: 120, Align: AlignRight,
			Text: func(i int) string { return strconv.Itoa((i*31 + k*17) % 100000) }})
	}
	cols = append(cols, TableColumn{Key: "op", Title: "操作", Width: 90, Fixed: FixedRight, Text: func(i int) string { return "查看" }})
	var sel []int
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Table(c, cols, 100000).Selection(&sel).ScrollY(760).Virtual().Show()
		}, layout.Pad(24))
	}, DriverOptions{Width: 1440, Height: 900, Scale: scale})
	d.Settle()
	return d
}

// BenchmarkTableVirtualWrapScrollNoRaster is the variable-height variant of
// BenchmarkTableVirtualScrollNoRaster (new rows are measured every frame).
func BenchmarkTableVirtualWrapScrollNoRaster(b *testing.B) {
	d := benchVirtualWrapTable(2)
	p := d.r.c.rects[d.r.c.root.kids[0].id].Center()
	r := d.r
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Inject(input.Scroll{Pos: p, DY: 120})
		r.c.now = d.Now()
		r.processEvents()
		r.c.invalid = false
		r.build()
		r.clearFrameInput()
		r.layoutAndPaint()
		r.afterFrame()
	}
}

// BenchmarkTableVirtualWrapScroll measures full frames of the
// variable-height table.
func BenchmarkTableVirtualWrapScroll(b *testing.B) {
	d := benchVirtualWrapTable(2)
	p := d.r.c.rects[d.r.c.root.kids[0].id].Center()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(120)
		if (i/400)%2 == 1 {
			dy = -120
		}
		d.Inject(input.Scroll{Pos: p, DY: dy})
		d.Frame()
	}
}
