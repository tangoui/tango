package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestMarkdownVirtualWheel(t *testing.T) {
	var doc strings.Builder
	for i := range 80 {
		fmt.Fprintf(&doc, "## Section %d\n\n%s\n\n", i+1, strings.Repeat("Paragraph text wraps in the preview. ", 7+i%5))
	}
	for _, step := range []float32{180, 1e6} {
		t.Run(fmt.Sprint(step), func(t *testing.T) {
			var m *MarkdownW
			d := NewDriver(func(c *Context) {
				m = Markdown(c, doc.String(), layout.Fill(), layout.Pad(12)).Virtual()
			}, DriverOptions{Width: 420, Height: 300, Accessibility: true})
			d.Settle()
			limit := 160
			if step > 180 {
				limit = 1
			}
			for i := 0; i < limit; i++ {
				d.Scroll(geom.Pt(200, 150), step)
				s := m.b.n.scroll
				if s.painted.Y <= 0 {
					t.Fatal("wheel offset was lost on the next frame")
				}
				if s.painted.Y >= s.maxY() {
					break
				}
			}
			s := m.b.n.scroll
			last, found := d.A11yFind(a11y.RoleHeading, "Section 80")
			end := m.b.n.kids[len(m.b.n.kids)-1]
			if !found || last.Bounds.Y < 0 || last.Bounds.Y+last.Bounds.H > 300 || s.painted.Y != s.maxY() {
				t.Fatalf("document end: heading=%+v offset=%.1f max=%.1f", last, s.painted.Y, s.maxY())
			}
			if r, ok := m.c.RectOf(end.id); !ok || r.Bottom() < 287 || r.Bottom() > 289 {
				t.Fatalf("last block ends at %.1f, want 288 with bottom padding", r.Bottom())
			}
		})
	}
}

func TestMarkdownVirtualBoundsBuild(t *testing.T) {
	var calls int
	d := NewDriver(func(c *Context) {
		m := Markdown(c, strings.Repeat("![image](test.png)\n\n", 600), layout.Fill()).
			Images(func(string) image.Image { calls++; return nil })
		asVirtualMarkdown(m)
	}, DriverOptions{Width: 420, Height: 300})
	d.Frame()
	d.Frame()
	if calls > 60 {
		t.Fatalf("image resolver called %d times for a 300px viewport", calls)
	}
}

// Use the same workload with the pre-viewport test binary.
func asVirtualMarkdown(m *MarkdownW) bool {
	if v, ok := any(m).(interface{ Virtual() *MarkdownW }); ok {
		v.Virtual()
		return true
	}
	return false
}

func BenchmarkMarkdownPreview(b *testing.B) {
	var doc strings.Builder
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&doc, "## Note section %d\n\nA **long note** with [a link](https://example.org) and more text to wrap across the panel width.\n\n- [ ] Task %d\n- Supporting information\n\n", i, i)
	}
	base := doc.String()
	for _, mode := range []string{"Scroll", "SplitCursor", "SplitEdit"} {
		b.Run(mode, func(b *testing.B) {
			src, cursor := base, 0
			d := NewDriver(func(c *Context) {
				Row(c, func() {
					if mode != "Scroll" {
						Column(c, func() { Text(c, fmt.Sprintf("Editor cursor %d", cursor)) }, layout.Grow(1), layout.MinW(0))
					}
					opts := []layout.Option{layout.Grow(1), layout.MinW(0), layout.MinH(0), layout.Pad(12)}
					if _, ok := any((*MarkdownW)(nil)).(interface{ Virtual() *MarkdownW }); ok {
						asVirtualMarkdown(Markdown(c, src, opts...))
					} else {
						Scroll(c, func() { Markdown(c, src) }, opts...)
					}
				}, layout.Fill())
			}, DriverOptions{Width: 960, Height: 600})
			d.Frame()
			d.Frame()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				switch mode {
				case "Scroll":
					d.Inject(input.Scroll{Pos: geom.Pt(750, 300), DY: 120})
				case "SplitCursor":
					cursor++
				case "SplitEdit":
					cursor++
					src = fmt.Sprintf("# Edit %d\n\n%s", cursor, base)
				}
				d.Frame()
			}
		})
	}
}
