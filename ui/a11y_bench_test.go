package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

// a11yBenchPage is a form-like page with ~300 controls.
func a11yBenchPage(vals []bool, texts []string) func(c *Context) {
	return func(c *Context) {
		Column(c, func() {
			for i := range vals {
				Key(c, i, func() {
					Row(c, func() {
						Checkbox(c, "Option "+strconv.Itoa(i), &vals[i])
						Input(c, &texts[i]).Placeholder("Value")
						Button(c, "Apply")
					})
				})
			}
		})
	}
}

// BenchmarkFrameA11y compares a frame of a 300-control page with the
// semantics tree off and on (build + diff + host notification).
func BenchmarkFrameA11y(b *testing.B) {
	for _, on := range []bool{false, true} {
		name := "off"
		if on {
			name = "on"
		}
		b.Run(name, func(b *testing.B) {
			vals, texts := make([]bool, 100), make([]string, 100)
			d := NewDriver(a11yBenchPage(vals, texts), DriverOptions{Width: 800, Height: 4000, Accessibility: on})
			d.Settle()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d.Inject(input.Pointer{Kind: input.Move, Pos: d.r.c.pointer})
				vals[i%len(vals)] = !vals[i%len(vals)] // one node changes per frame
				r := d.r
				r.c.now = d.Now()
				r.c.a11yOn = on
				r.processEvents()
				r.build()
				r.layoutAndPaint()
				r.afterFrame()
				if on {
					r.reportA11y(d.w)
				}
			}
		})
	}
}
