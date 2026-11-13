package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/geom"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// fillTreeData builds 10 × 100 × 100 leaves (101,010 nodes).
func fillTreeData() []TreeNode {
	out := make([]TreeNode, 10)
	for a := range out {
		ka := fmt.Sprint(a)
		out[a] = TreeNode{Key: ka, Title: "Zone " + ka, Children: make([]TreeNode, 100)}
		for b := range out[a].Children {
			kb := fmt.Sprintf("%s-%d", ka, b)
			n := TreeNode{Key: kb, Title: "Shop " + kb, Children: make([]TreeNode, 100)}
			for c := range n.Children {
				kc := fmt.Sprintf("%s-%d", kb, c)
				n.Children[c] = TreeNode{Key: kc, Title: "Dev " + kc, Disabled: c == 99}
			}
			out[a].Children[b] = n
		}
	}
	return out
}

func TestVirtualTreeExpandCheckKeyboard(t *testing.T) {
	data := fillTreeData()
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var exp, chk []string
		cur := ""
		var w *VirtualTreeW
		d := fillDriver(th, func(c *Context) {
			w = VirtualTree(c, data).Expanded(&exp).Checkable(&chk).Current(&cur).ExpandOnClick(false).Height(300).Show()
		})
		if w.VisibleCount() != 10 {
			t.Fatalf("visible = %d", w.VisibleCount())
		}
		r := mustFind(t, d, "switch:0")
		d.Click(r.Center())
		if w.VisibleCount() != 110 || len(exp) != 1 {
			t.Fatalf("after expand visible=%d exp=%v", w.VisibleCount(), exp)
		}
		d.Click(mustFind(t, d, "switch:0-0").Center())
		if _, ok := d.Find("Dev 0-0-0"); !ok {
			t.Fatal("child row missing")
		}
		if f, e := w.Range(); e-f > 30 {
			t.Fatalf("built %d rows", e-f)
		}
		// Check a leaf: the parents turn half-checked.
		d.Click(mustFind(t, d, "check:0-0-1").Center())
		if len(chk) != 1 || chk[0] != "0-0-1" {
			t.Fatalf("chk = %v", chk)
		}
		half := w.HalfChecked()
		if len(half) != 2 {
			t.Fatalf("half = %v", half)
		}
		// Check the shop: every checkable child (99) plus the shop.
		d.Click(mustFind(t, d, "check:0-0").Center())
		if len(chk) != 100 {
			t.Fatalf("checked %d keys", len(chk))
		}
		// Uncheck a child: the shop drops to half-checked.
		d.Click(mustFind(t, d, "check:0-0-2").Center())
		if contains(chk, "0-0") || len(chk) != 98 {
			t.Fatalf("after uncheck %d keys, has parent %v", len(chk), contains(chk, "0-0"))
		}
		// Keyboard: click a row, then move down and select with Enter.
		d.ClickLabel("Zone 0")
		if cur != "0" {
			t.Fatalf("cur = %q", cur)
		}
		d.Key(input.KeyDown, 0)
		d.Key(input.KeyEnter, 0)
		if cur != "0-0" {
			t.Fatalf("keyboard cur = %q", cur)
		}
		d.Key(input.KeyLeft, 0) // collapse 0-0
		if contains(exp, "0-0") {
			t.Fatalf("Left did not collapse: %v", exp)
		}
		d.Key(input.KeyEnd, 0)
		d.Key(input.KeyEnter, 0)
		if cur != "9" {
			t.Fatalf("End+Enter cur = %q", cur)
		}
	})
}

func TestVirtualTreeStrictFilterLazy(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		data := fillTreeData()
		var chk []string
		var exp []string
		q := ""
		var w *VirtualTreeW
		d := fillDriver(th, func(c *Context) {
			w = VirtualTree(c, data).Expanded(&exp).Checkable(&chk).CheckStrictly().Filter(q).Height(300).Show()
		})
		q = "dev 3-42-7"
		d.Settle()
		// Matches: Dev 3-42-7 and Dev 3-42-70..79 plus their two ancestors.
		if w.VisibleCount() != 13 {
			t.Fatalf("filtered rows = %d", w.VisibleCount())
		}
		d.Click(mustFind(t, d, "check:3-42").Center())
		if len(chk) != 1 || chk[0] != "3-42" {
			t.Fatalf("strict check = %v", chk)
		}
		q = ""
		d.Settle()

		// Lazy loading.
		lazy := []TreeNode{{Key: "a", Title: "A"}, {Key: "b", Title: "B", IsLeaf: true}}
		calls := 0
		var lexp []string
		var lw *VirtualTreeW
		d2 := fillDriver(th, func(c *Context) {
			lw = VirtualTree(c, lazy).Expanded(&lexp).LoadData(func(key string) bool {
				calls++
				if calls < 3 {
					return true
				}
				lazy[0].Children = []TreeNode{{Key: "a1", Title: "A1", IsLeaf: true}}
				return false
			}).Height(200).Show()
		})
		if _, ok := d2.Find("switch:b"); ok {
			t.Fatal("leaf must have no switcher")
		}
		d2.Click(mustFind(t, d2, "switch:a").Center())
		d2.Settle()
		if lw.VisibleCount() != 3 {
			t.Fatalf("lazy rows = %d (calls %d)", lw.VisibleCount(), calls)
		}
		if _, ok := d2.Find("A1"); !ok {
			t.Fatal("loaded child missing")
		}
	})
}

// BenchmarkVirtualTree100k measures a frame of a fully expanded 101k-node
// tree while scrolling.
func BenchmarkVirtualTree100k(b *testing.B) {
	data := fillTreeData()
	var exp []string
	treeWalk(data, func(n *TreeNode, _ []string) {
		if len(n.Children) > 0 {
			exp = append(exp, n.Key)
		}
	})
	var chk []string
	d := NewDriver(func(c *Context) {
		VirtualTree(c, data).Expanded(&exp).Checkable(&chk).Height(500).Show()
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	r := fillMustFindB(b, d, "Zone 0")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(60)
		if i%200 >= 100 {
			dy = -60
		}
		d.Inject(input.Scroll{Pos: r.Center(), DY: dy})
		d.Frame()
	}
}

func fillMustFindB(b *testing.B, d *Driver, label string) geom.Rect {
	r, ok := d.Find(label)
	if !ok {
		b.Fatalf("no %q", label)
	}
	return r
}
