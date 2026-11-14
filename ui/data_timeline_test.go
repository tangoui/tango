package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestTimelineHorizontal(t *testing.T) {
	items := []TimelineItem{{Content: "first", Label: "L1"}, {Content: "second", Color: "success"}, {Content: "third"}}
	for _, mode := range []string{"left", "right", "alternate"} {
		rtl := false
		d := driver(func(c *Context) {
			dir := LTR
			if rtl {
				dir = RTL
			}
			ConfigProvider(c).Direction(dir).Body(func() {
				Timeline(c, items).Horizontal().Mode(mode).Pending("waiting").Show()
			})
		})
		d.Frame()
		f, s, p := mustFindText(t, d, "first"), mustFindText(t, d, "second"), mustFindText(t, d, "waiting")
		if !(f.X < s.X && s.X < p.X) {
			t.Fatalf("%s: not left to right: %v %v %v", mode, f, s, p)
		}
		l1 := mustFindText(t, d, "L1")
		switch mode {
		case "left":
			if !(l1.Y < f.Y) || f.Y != s.Y {
				t.Fatalf("left: content under axis, label above: %v %v %v", l1, f, s)
			}
		case "right":
			if !(l1.Y > f.Y) || f.Y != s.Y {
				t.Fatalf("right: content above axis, label under: %v %v %v", l1, f, s)
			}
		case "alternate":
			if !(s.Y < f.Y) {
				t.Fatalf("alternate: second above first: %v %v", s, f)
			}
		}
		rtl = true
		d.Frame()
		d.Frame()
		if f, p := mustFindText(t, d, "first"), mustFindText(t, d, "waiting"); f.X < p.X {
			t.Fatalf("%s rtl: %v %v", mode, f, p)
		}
	}
}

func mustFindText(t *testing.T, d *Driver, s string) geom.Rect {
	t.Helper()
	r, ok := d.FindText(s)
	if !ok {
		t.Fatalf("text %q not found", s)
	}
	return r
}

func TestTimelineOrderAndModes(t *testing.T) {
	items := []TimelineItem{{Content: "first"}, {Content: "second", Color: "success"}, {Content: "third", Label: "L"}}
	for _, mode := range []string{"left", "right", "alternate"} {
		d := driver(func(c *Context) {
			Timeline(c, items).Mode(mode).Reverse().Pending("waiting").Show()
		})
		d.Frame()
		// Reverse puts the pending item first and the last item next.
		ctx := d.Runner().Context()
		var ys []float32
		for _, s := range []string{"waiting", "third", "second", "first"} {
			found := false
			var walk func(n *node)
			walk = func(n *node) {
				if n.label == s && !found {
					found = true
					ys = append(ys, n.abs.Y)
				}
				for _, k := range n.kids {
					walk(k)
				}
			}
			walk(ctx.root)
			if !found {
				t.Fatalf("%s: %q not built", mode, s)
			}
		}
		for i := 1; i < len(ys); i++ {
			if ys[i] <= ys[i-1] {
				t.Fatalf("%s: order wrong %v", mode, ys)
			}
		}
	}
}
