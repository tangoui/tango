package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// RTL tests of Tree, VirtualTree, TreeSelect, Cascader and Transfer.

// g5Dir wraps body in a provider of the given direction.
func g5Dir(c *Context, rtl bool, body func()) {
	dir := LTR
	if rtl {
		dir = RTL
	}
	ConfigProvider(c).Direction(dir).Body(body)
}

// g5TextOp returns the first glyph run whose baseline lies inside r.
func g5TextOp(d *Driver, r geom.Rect) (paint.Text, bool) {
	for _, op := range d.r.List().Ops {
		if g, ok := op.(paint.Text); ok && g.Origin.Y >= r.Y && g.Origin.Y <= r.Bottom() && g.Origin.X >= r.X-1 && g.Origin.X <= r.Right() {
			return g, true
		}
	}
	return paint.Text{}, false
}

func TestG5TreeRTLKeys(t *testing.T) {
	nodes := treeTestNodes()
	var expanded []string
	var w *TreeW
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() { w = Tree(c, nodes).Expanded(&expanded).Show() })
	}, DriverOptions{})
	d.Settle()
	key := func(k input.Key) { d.Key(k, 0); d.Settle() }
	key(input.KeyTab)
	key(input.KeyDown)
	key(input.KeyLeft) // RTL: Left expands
	if !contains(expanded, "p") {
		t.Fatalf("Left should expand in RTL: %v", expanded)
	}
	key(input.KeyLeft) // enters the first child
	if w.st.cursor != "a" {
		t.Fatalf("Left on an open node should enter its first child, cursor=%q", w.st.cursor)
	}
	key(input.KeyRight) // a is collapsed: back to parent
	if w.st.cursor != "p" {
		t.Fatalf("Right should go to the parent in RTL, cursor=%q", w.st.cursor)
	}
	key(input.KeyRight) // collapses p
	if contains(expanded, "p") {
		t.Fatalf("Right should collapse in RTL: %v", expanded)
	}
}

// TestG5TreeRTLLayout: indentation grows from the right, the switcher sits
// at the start (right) of the title and the title text is start-aligned.
func TestG5TreeRTLLayout(t *testing.T) {
	nodes := treeTestNodes()
	expanded := []string{"p"}
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() { Tree(c, nodes).Expanded(&expanded).Show() })
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	sp := mustFind(t, d, "switch:p")
	sa := mustFind(t, d, "switch:a")
	if !(sa.Right() < sp.X+1) || sp.Right() < 390 {
		t.Fatalf("child switcher %v should be left of parent switcher %v at the right edge", sa, sp)
	}
	title := mustFind(t, d, "parent")
	if title.Right() > sp.X+0.5 {
		t.Fatalf("title %v should be left of its switcher %v", title, sp)
	}
	g, ok := g5TextOp(d, title)
	if !ok {
		t.Fatal("title glyphs not found")
	}
	w := text.Width("parent", d.r.c.style(d.r.c.Theme.FontSize, text.Regular))
	if !near(g.Origin.X+w, title.Right()-4) {
		t.Fatalf("title text ends at %v, want %v (start-aligned in RTL)", g.Origin.X+w, title.Right()-4)
	}
}

// TestG5TreeRTLConnectors: showLine connectors mirror with the rows.
func TestG5TreeRTLConnectors(t *testing.T) {
	nodes := treeTestNodes()
	expanded := []string{"p"}
	var lines [2][]float32
	for i, rtl := range []bool{false, true} {
		d := NewDriver(func(c *Context) {
			g5Dir(c, rtl, func() { Tree(c, nodes).Expanded(&expanded).ShowLine().Show() })
		}, DriverOptions{Width: 400, Height: 300})
		d.Settle()
		for _, op := range d.r.List().Ops {
			if f, ok := op.(paint.FillRect); ok && f.Rect.W == 1 && f.Rect.H > 4 {
				lines[i] = append(lines[i], f.Rect.X)
			}
		}
	}
	if len(lines[0]) == 0 || len(lines[0]) != len(lines[1]) {
		t.Fatalf("connector counts differ: ltr %v rtl %v", lines[0], lines[1])
	}
	for i := range lines[0] {
		if !near(lines[1][i], 400-lines[0][i]-1) {
			t.Fatalf("connector %d: rtl x %v, want mirror of %v", i, lines[1][i], lines[0][i])
		}
	}
}

// TestG5TreeSwitcherMirrors: the collapsed caret points along the reading
// direction (drawn mirrored in RTL) in every flavor.
func TestG5TreeSwitcherMirrors(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		for _, rtl := range []bool{false, true} {
			var l paint.List
			NewDriver(func(c *Context) {
				g5Dir(c, rtl, func() {
					Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 24, H: 24} }, func(_ *paint.List, r geom.Rect) {
						(&TreeW{c: c}).paintSwitcher(&l, r, true, false, 0)
					})
				})
			}, DriverOptions{Width: 100, Height: 100, Theme: th}).Settle()
			mirrored := false
			for _, op := range l.Ops {
				if m, ok := op.(paint.PushTransform); ok && m.M.A == -1 {
					mirrored = true
				}
			}
			if mirrored != rtl {
				t.Errorf("%v rtl=%v: caret mirrored=%v", th.Flavor, rtl, mirrored)
			}
		}
	}
}

func TestG5VirtualTreeRTLKeys(t *testing.T) {
	data := fillTreeData()
	var exp []string
	cur := ""
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() {
			VirtualTree(c, data).Expanded(&exp).Current(&cur).ExpandOnClick(false).Height(300).Show()
		})
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	d.ClickLabel("Zone 0")
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if !contains(exp, "0") {
		t.Fatalf("Left should expand in RTL: %v", exp)
	}
	d.Key(input.KeyRight, 0)
	d.Settle()
	if contains(exp, "0") {
		t.Fatalf("Right should collapse in RTL: %v", exp)
	}
	// The row's switcher is at its right (start) edge.
	sw := mustFind(t, d, "switch:0")
	row := mustFind(t, d, "Zone 0")
	if sw.Right() < row.Right()-1 {
		t.Fatalf("switcher %v not at the start of row %v", sw, row)
	}
}

func TestG5TreeSelectRTL(t *testing.T) {
	v := ""
	var w *TreeSelectW
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() { w = TreeSelect(c, &v, tsTestTree).Width(240) })
	}, DriverOptions{Width: 600, Height: 500})
	d.Frame()
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Key(input.KeyEscape, 0)
	d.Key(input.KeyDown, 0) // open, active = 浙江省
	d.Key(input.KeyLeft, 0) // RTL: expand
	if _, ok := d.Find("杭州市"); !ok {
		t.Fatal("Left did not expand in RTL")
	}
	// Child rows indent from the right: the child label starts further left.
	p, ok1 := g5TextOp(d, mustFind(t, d, "浙江省"))
	ch, ok2 := g5TextOp(d, mustFind(t, d, "杭州市"))
	if !ok1 || !ok2 {
		t.Fatal("row glyphs not found")
	}
	row := mustFind(t, d, "浙江省")
	st := d.r.c.style(d.r.c.Theme.FontSize, text.Regular)
	pe, ce := p.Origin.X+text.Width("浙江省", st), ch.Origin.X+text.Width("杭州市", st)
	if !(ce < pe-10) || pe < row.X+row.W/2 {
		t.Fatalf("label ends: parent %v child %v in row %v", pe, ce, row)
	}
	// A press at the right edge hits the switcher (collapses, no pick).
	d.Click(geom.Pt(row.Right()-10, row.Y+row.H/2))
	if _, ok := d.Find("杭州市"); ok || v != "" {
		t.Fatalf("switcher press at the start edge should collapse (v=%q)", v)
	}
	d.Key(input.KeyLeft, 0)  // expand again
	d.Key(input.KeyLeft, 0)  // to 杭州市
	d.Key(input.KeyRight, 0) // back to the parent
	d.Key(input.KeyRight, 0) // collapse
	if _, ok := d.Find("杭州市"); ok {
		t.Fatal("Right did not collapse in RTL")
	}
}

func TestG5CascaderRTL(t *testing.T) {
	var val []string
	var w *CascaderW
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() { w = Cascader(c, &val, casTestOpts).Placeholder("地区").Width(200) })
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.ClickLabel("地区")
	d.Key(input.KeyDown, 0) // 浙江
	d.Key(input.KeyLeft, 0) // RTL: next column
	if w.st.kcol != 1 || casKey(w.st.path) != casKey([]string{"zj", "hz"}) {
		t.Fatalf("Left in RTL: %v col %d", w.st.path, w.st.kcol)
	}
	// The second column opens to the left of the first.
	c0, c1 := mustFind(t, d, "浙江"), mustFind(t, d, "杭州")
	if !(c1.Right() <= c0.X+1) {
		t.Fatalf("column 1 %v should be left of column 0 %v", c1, c0)
	}
	// Labels are start (right) aligned.
	g, ok := g5TextOp(d, c0)
	if !ok || g.Origin.X < c0.X+c0.W/2 {
		t.Fatalf("label not right-aligned in %v: %v", c0, g.Origin)
	}
	d.Key(input.KeyRight, 0) // back
	if w.st.kcol != 0 {
		t.Fatalf("Right in RTL should go back, col %d", w.st.kcol)
	}
}

func TestG5TransferRTL(t *testing.T) {
	items := []TransferItem{{Key: "a", Title: "alpha"}, {Key: "b", Title: "beta"}}
	target := []string{"b"}
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() { Transfer(c, items, &target) })
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	src, dst := mustFind(t, d, "alpha"), mustFind(t, d, "beta")
	if !(src.X > dst.Right()) {
		t.Fatalf("source item %v should be right of target item %v", src, dst)
	}
	g, ok := g5TextOp(d, src)
	if !ok {
		t.Fatal("item glyphs not found")
	}
	bs := d.r.c.boxSize()
	w := text.Width("alpha", d.r.c.style(d.r.c.Theme.FontSize, text.Regular))
	if want := src.Right() - 12 - bs - 8; !near(g.Origin.X+w, want) {
		t.Fatalf("item text ends at %v, want %v (after the checkbox at the right)", g.Origin.X+w, want)
	}
}

// TestG5ShadItemRTL: the shadcn tree / cascader item mirrors its chevron,
// checkbox and text.
func TestG5ShadItemRTL(t *testing.T) {
	var val []string
	var multi [][]string
	d := NewDriver(func(c *Context) {
		g5Dir(c, true, func() { Cascader(c, &val, casTestOpts).Multiple(&multi).Placeholder("地区").Width(200) })
	}, DriverOptions{Width: 600, Height: 400, Theme: theme.Shadcn()})
	d.Frame()
	d.ClickLabel("地区")
	r := mustFind(t, d, "浙江")
	g, ok := g5TextOp(d, r)
	if !ok {
		t.Fatal("item glyphs not found")
	}
	st := d.r.c.style(d.r.c.Theme.FontSize, text.Regular)
	if want := r.Right() - 8 - d.r.c.boxSize() - 8; !near(g.Origin.X+text.Width("浙江", st), want) {
		t.Fatalf("shadcn item text ends at %v, want %v", g.Origin.X+text.Width("浙江", st), want)
	}
}

// TestG5MarkedTextRTL: a search match in an RTL row keeps its place in the
// shaped line and gets the mark color.
func TestG5MarkedTextRTL(t *testing.T) {
	var l paint.List
	r := geom.Rect{X: 0, Y: 0, W: 200, H: 24}
	st := text.Style{Size: 14}
	red := theme.Hex("#ff0000")
	treeMarkedText(&l, r, 4, "abcdef", 2, 4, st, st, 0, treeMid, theme.Hex("#000000"), red, nil)
	var n, marked int
	var right float32
	for _, op := range l.Ops {
		g := op.(paint.Text)
		n += len(g.Glyphs)
		if g.Color == red {
			marked += len(g.Glyphs)
		}
		right = max(right, g.Origin.X)
	}
	full := text.Width("abcdef", st)
	if n != 6 || marked != 2 {
		t.Fatalf("glyphs %d marked %d", n, marked)
	}
	if !near(right+full, 196) {
		t.Fatalf("line ends at %v, want 196", right+full)
	}
}
