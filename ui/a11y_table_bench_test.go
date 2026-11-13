package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// benchVirtualTableA11y is benchVirtualTable with the semantics tree built
// every frame (DriverOptions.Accessibility), to measure the overhead of the
// table's annotations and of the tree itself.
func benchVirtualTableA11y(scale float32) *Driver {
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
	}, DriverOptions{Width: 1440, Height: 900, Scale: scale, Accessibility: true})
	d.Settle()
	return d
}

// BenchmarkTableVirtualScrollA11y is BenchmarkTableVirtualScroll with
// accessibility on.
func BenchmarkTableVirtualScrollA11y(b *testing.B) {
	d := benchVirtualTableA11y(2)
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

// BenchmarkTableVirtualScrollNoRasterA11y is
// BenchmarkTableVirtualScrollNoRaster with accessibility on: the frame
// also builds, diffs and reports the semantics tree (as Runner.Frame does).
func BenchmarkTableVirtualScrollNoRasterA11y(b *testing.B) {
	d := benchVirtualTableA11y(2)
	p := d.r.c.rects[d.r.c.root.kids[0].id].Center()
	r := d.r
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Inject(input.Scroll{Pos: p, DY: 120})
		r.c.now = d.Now()
		host, on := r.accessibilityOn()
		r.c.a11yOn = on
		r.c.announce = r.c.announce[:0]
		r.processEvents()
		r.c.invalid = false
		r.build()
		r.clearFrameInput()
		r.layoutAndPaint()
		r.afterFrame()
		if on {
			r.reportA11y(host)
		}
	}
}
