package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestNewIDCompatibility(t *testing.T) {
	c := newContext(theme.AntDesign())
	for _, scope := range []ID{0, 1, 1<<63 + 17} {
		c.scope = scope
		clear(c.counters)
		for _, pair := range [][2]string{{"text", "hello"}, {"text", "hello"}, {"div", ""}, {"text", "中文\x00العربية"}} {
			base := hashID(scope, pair[0], pair[1])
			occurrence := c.counters[base]
			want := hashID(base, occurrence)
			if got := c.newID(pair[0], pair[1]); got != want {
				t.Fatalf("ID = %x, want %x", got, want)
			}
		}
	}
}

func TestNewIDNoAllocations(t *testing.T) {
	c := newContext(theme.AntDesign())
	c.scope = 1<<63 + 17
	// Warm the map and exercise counts beyond the runtime's small-int box cache.
	base := hashID(c.scope, "text", "a label")
	c.counters[base] = 1024
	if got, want := c.newID("text", "a label"), hashID(base, 1024); got != want {
		t.Fatalf("large occurrence ID = %x, want %x", got, want)
	}
	if allocs := testing.AllocsPerRun(100, func() { c.newID("text", "a label") }); allocs != 0 {
		t.Fatalf("newID allocated %g times", allocs)
	}
}

func BenchmarkContextNewID(b *testing.B) {
	c := newContext(theme.AntDesign())
	c.scope = 12345
	c.newID("text", "a label")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.newID("text", "a label")
	}
}

// Keep the changing 600-cell workload identical to the external scene, but
// exercise only build/layout/paint-list generation: no native window or GPU.
func BenchmarkContextGrid600(b *testing.B) {
	frame := 0
	r := NewRunner(func(c *Context) {
		Column(c, func() {
			for row := 0; row < 30; row++ {
				Row(c, func() {
					for col := 0; col < 20; col++ {
						v := float64(((row*20+col)*7919+frame*31)%100000) / 100
						bg := theme.Hex("#c6e9c8")
						if v < 333 {
							bg = theme.Hex("#f6c9c4")
						} else if v < 666 {
							bg = theme.Hex("#f4e8b8")
						}
						Div(c, layout.Size(40, 20), layout.Center()).Bg(bg).Body(func() {
							Text(c, fmt.Sprintf("%.2f", v)).Size(11).Color(theme.Hex("#101010")).NoWrap()
						})
					}
				})
			}
		}, layout.Fill())
	}, RunnerOptions{Theme: theme.AntDesign()})
	r.c.size = geom.Size{W: 800, H: 600}
	r.build()
	r.layoutAndPaint()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame++
		r.build()
		r.layoutAndPaint()
		r.afterFrame()
	}
}
