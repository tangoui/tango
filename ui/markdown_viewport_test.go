package ui

import (
	"fmt"
	"image"
	"math"
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
			}, DriverOptions{Width: 420, Height: 300, Scale: 1, Accessibility: true})
			d.Settle()
			// The height index sums float64 block heights, while flex layout
			// measures the built window in float32. They can round differently
			// even when the final block sits at the bottom pixel.
			atEnd := func(s *scrollState) bool {
				return math.Round(s.painted.Y) == math.Round(s.maxY())
			}
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
				if atEnd(s) {
					break
				}
			}
			s := m.b.n.scroll
			last, found := d.A11yFind(a11y.RoleHeading, "Section 80")
			end := m.b.n.kids[len(m.b.n.kids)-1]
			r, laidOut := m.c.RectOf(end.id)
			if !found || last.Bounds.Y < 0 || last.Bounds.Y+last.Bounds.H > 300 || !atEnd(s) {
				t.Fatalf("document end: heading=%+v found=%t offset=%g max=%g last block=%+v laidOut=%t", last, found, s.painted.Y, s.maxY(), r, laidOut)
			}
			if !laidOut || r.Bottom() < 287 || r.Bottom() > 289 {
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

func TestMarkdownHeadingBlocks(t *testing.T) {
	src := "# **Hello**, `World`!\n\n> # 中文\n\n- ## Hello World\n\nHello World-1\n-------------\n\n```go\n# Hidden\n```"
	var got map[string]int
	d := NewDriver(func(c *Context) {
		m := Markdown(c, src)
		got = make(map[string]int)
		for _, id := range []string{"hello-world", "#%E4%B8%AD%E6%96%87", "hello-world-1", "hello-world-1-1", "hidden", "edited"} {
			if block, ok := m.HeadingBlock(id); ok {
				got[id] = block
			}
		}
	}, DriverOptions{Width: 420, Height: 300})
	d.Frame()
	if len(got) != 4 || got["hello-world"] != 0 || got["#%E4%B8%AD%E6%96%87"] != 1 || got["hello-world-1"] != 2 || got["hello-world-1-1"] != 3 {
		t.Fatalf("heading blocks: %v", got)
	}
	src = "# Edited"
	d.Frame()
	if block, ok := got["edited"]; !ok || block != 0 || len(got) != 1 {
		t.Fatalf("stale heading blocks after edit: %v", got)
	}
}

func TestMarkdownVirtualScrollToHeading(t *testing.T) {
	var doc strings.Builder
	doc.WriteString("[Go to section](#section-70)\n\n")
	for i := range 80 {
		fmt.Fprintf(&doc, "## Section %d\n\n%s\n\n", i+1, strings.Repeat("Paragraph text wraps in the preview. ", 12))
	}
	src := doc.String()
	var request, clicked bool
	d := NewDriver(func(c *Context) {
		m := Markdown(c, src, layout.Fill(), layout.Pad(12)).Virtual()
		if request {
			request = false
			if m.ScrollToBlock(-1) || m.ScrollToBlock(161) || m.ScrollToHeading("#missing") || !m.ScrollToBlock(0) {
				t.Error("unexpected block or heading lookup result")
			}
		}
		if link := m.ClickedLink(); link != "" {
			clicked = m.ScrollToHeading(link)
		}
	}, DriverOptions{Width: 420, Height: 300, Accessibility: true})
	d.Settle()
	d.Frame() // RichText link nodes use the previously painted link bounds.
	link, ok := d.A11yFind(a11y.RoleLink, "Go to section")
	if !ok {
		t.Fatal("link is missing")
	}
	d.Click(geom.Pt(link.Bounds.X+link.Bounds.W/2, link.Bounds.Y+link.Bounds.H/2))
	target, ok := d.A11yFind(a11y.RoleHeading, "Section 70")
	if !clicked || !ok || target.Bounds.Y < 0 || target.Bounds.Y > 30 {
		t.Fatalf("fragment click=%v, heading=%+v", clicked, target)
	}
	request = true
	d.Settle()
	if p, ok := d.A11yFind(a11y.RoleParagraph, "Go to section"); !ok || p.Bounds.Y < 0 || p.Bounds.Y > 30 {
		t.Fatal("scrolling to block 0 did not bring the link back into view")
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
