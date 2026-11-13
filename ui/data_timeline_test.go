package ui

import "testing"

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
