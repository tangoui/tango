package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// RTL behaviour of the navigation group (Menu, Dropdown, Tabs, Breadcrumb,
// Pagination, Steps, Splitter, Layout / Sider, FloatButton).

// g3Menu is a menu with icons, a nested submenu, a group and a disabled item.
func g3Menu() []MenuItem {
	return []MenuItem{
		{Key: "home", Label: "Home", Icon: icon.Home},
		{Key: "sub", Label: "Settings", Icon: icon.Setting, Children: []MenuItem{
			{Key: "a", Label: "Profile"}, {Key: "b", Label: "Security"},
			{Key: "deep", Label: "Deeper", Children: []MenuItem{{Key: "d1", Label: "Leaf"}}},
		}},
		{Key: "grp", Label: "Group", Group: true, Children: []MenuItem{{Key: "g1", Label: "G one"}}},
		{Key: "x", Label: "Disabled", Disabled: true},
	}
}

// g3Dir wraps page in a ConfigProvider with direction rtl.
func g3Dir(rtl bool, page func(c *Context)) func(c *Context) {
	return func(c *Context) {
		d := LTR
		if rtl {
			d = RTL
		}
		ConfigProvider(c).Direction(d).Body(func() { page(c) })
	}
}

// g3Texts returns the origins of the glyph runs drawn inside r.
func g3Texts(d *Driver, r geom.Rect) []geom.Point {
	var out []geom.Point
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.Text); ok && r.Contains(t.Origin) {
			out = append(out, t.Origin)
		}
	}
	return out
}

// g3IconAt returns the translation of the first icon drawn inside r (icons
// are drawn as a translated path).
func g3IconAt(d *Driver, r geom.Rect) (geom.Point, bool) {
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.PushTransform); ok {
			p := geom.Pt(t.M.E, t.M.F)
			if r.Contains(p) {
				return p, true
			}
		}
	}
	return geom.Point{}, false
}

// g3HasPath reports whether the frame draws path p (icon renditions are
// cached, so the same icon at the same size and mirroring shares a path).
func g3HasPath(d *Driver, p *paint.Path) bool {
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillPath); ok && f.Path == p {
			return true
		}
	}
	return false
}

// g3IconPath returns the path icon.DrawWith emits for ic in r under the
// default (Ant Design) theme's icon style.
func g3IconPath(ic *icon.Icon, r geom.Rect, rtl bool) *paint.Path {
	l := paint.List{IconStyle: theme.AntDesign().Icons}
	icon.DrawWith(&l, ic, r, theme.Hex("#000000"), icon.DrawOpts{RTL: rtl})
	for _, op := range l.Ops {
		if f, ok := op.(paint.FillPath); ok {
			return f.Path
		}
	}
	return nil
}

func TestG3MenuInlineRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		sel := "home"
		d := NewDriver(g3Dir(rtl, func(c *Context) {
			Menu(c, g3Menu(), &sel).DefaultOpen("sub").Width(240).Show()
		}), DriverOptions{Width: 400, Height: 500, Theme: theme.AntDesign()})
		d.Settle()
		r, ok := d.Find("Settings")
		if !ok {
			t.Fatal("Settings item missing")
		}
		texts := g3Texts(d, r)
		if len(texts) != 1 {
			t.Fatalf("rtl=%v: %d texts in the item", rtl, len(texts))
		}
		// The label starts after indent + icon from the start edge.
		mid := r.X + r.W/2
		if rtl != (texts[0].X > mid) {
			t.Errorf("rtl=%v: label at %v in %+v", rtl, texts[0], r)
		}
		// Icon at the start, chevron at the end.
		start, _ := g3IconAt(d, geom.Rect{X: r.X, Y: r.Y, W: 60, H: r.H})
		end, _ := g3IconAt(d, geom.Rect{X: r.Right() - 60, Y: r.Y, W: 60, H: r.H})
		if start == (geom.Point{}) || end == (geom.Point{}) {
			t.Errorf("rtl=%v: icons start=%v end=%v", rtl, start, end)
		}
		// The nested items are indented from the start edge.
		pr, _ := d.Find("Profile")
		pt := g3Texts(d, pr)
		if len(pt) != 1 || rtl != (pt[0].X > mid) {
			t.Errorf("rtl=%v: nested label %v vs %v", rtl, pt, texts)
		}
	}
}

func TestG3MenuKeysRTL(t *testing.T) {
	sel := "home"
	d := NewDriver(g3Dir(true, func(c *Context) {
		Menu(c, g3Menu(), &sel).Width(240).Show()
	}), DriverOptions{Width: 400, Height: 500})
	d.Settle()
	d.ClickLabel("Settings") // opens it and focuses it
	if _, ok := d.FindText("Profile"); !ok {
		t.Fatal("click should open the submenu")
	}
	// RTL: Right closes (towards the start), Left opens.
	d.Key(input.KeyRight, 0)
	d.Settle()
	if _, ok := d.FindText("Profile"); ok {
		t.Fatal("Right should close the submenu in RTL")
	}
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if _, ok := d.FindText("Profile"); !ok {
		t.Fatal("Left should open the submenu in RTL")
	}
}

func TestG3MenuPopupRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		sel := "home"
		var id ID
		d := NewDriver(g3Dir(rtl, func(c *Context) {
			Row(c, func() {
				id = Menu(c, g3Menu(), &sel).Collapsed(true).Show().id
			}, layout.Pad(300))
		}), DriverOptions{Width: 900, Height: 600})
		d.Settle()
		mr := d.r.c.rects[id]
		r, ok := d.Find("Settings")
		if !ok {
			t.Fatal("Settings item missing")
		}
		d.Move(r.Center())
		d.Advance(500 * time.Millisecond)
		d.Settle()
		pr, ok := d.FindText("Profile")
		if !ok {
			t.Fatalf("rtl=%v: pop-out submenu not shown", rtl)
		}
		if rtl && pr.Right() > mr.X || !rtl && pr.X < mr.Right() {
			t.Errorf("rtl=%v: submenu %+v next to menu %+v", rtl, pr, mr)
		}
		// The submenu chevron of "Deeper" points to the end.
		dr, _ := d.FindText("Deeper")
		want := g3IconPath(icon.Right, geom.Rect{W: 10, H: 10}, rtl)
		if !g3HasPath(d, want) {
			t.Errorf("rtl=%v: Deeper chevron not drawn mirrored=%v (%+v)", rtl, rtl, dr)
		}
	}
}

func TestG3TabsRTL(t *testing.T) {
	act := "t2"
	items := []TabItem{{Key: "t1", Label: "First"}, {Key: "t2", Label: "Second"}, {Key: "t3", Label: "Third"}}
	d := NewDriver(g3Dir(true, func(c *Context) {
		Column(c, func() { Tabs(c, &act, items).Show() }, layout.W(600))
	}), DriverOptions{Width: 700, Height: 300, Theme: theme.AntDesign()})
	d.Settle()
	r1, _ := d.Find("First")
	r2, _ := d.Find("Second")
	if r1.X <= r2.X {
		t.Fatalf("RTL tab order: First %+v Second %+v", r1, r2)
	}
	// The ink bar sits under the active tab.
	ink := geom.Rect{}
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.H == 2 && f.Color == theme.AntDesign().Components.Tabs.InkBarColor {
			ink = f.Rect
		}
	}
	if ink.X != r2.X || ink.W != r2.W {
		t.Fatalf("ink %+v under %+v", ink, r2)
	}
	// Arrow keys follow the visual order: Left goes to the next tab.
	d.Click(r2.Center())
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if act != "t3" {
		t.Fatalf("Left in RTL: active %q want t3", act)
	}
}

func TestG3TabsVerticalRTL(t *testing.T) {
	act := "t1"
	items := []TabItem{{Key: "t1", Label: "First"}, {Key: "t2", Label: "Second"}}
	var panel *Box
	d := NewDriver(g3Dir(true, func(c *Context) {
		Column(c, func() {
			panel = Tabs(c, &act, items).Placement(Left).Show().Panel(func() { Text(c, "panel") })
		}, layout.W(600))
	}), DriverOptions{Width: 700, Height: 300, Theme: theme.AntDesign()})
	d.Settle()
	r1, _ := d.Find("First")
	pr := d.r.c.rects[panel.ID()]
	if r1.X < pr.Right() {
		t.Fatalf("a Left bar sits at the start (right) in RTL: tab %+v panel %+v", r1, pr)
	}
	// The vertical ink bar is on the edge facing the panel (left).
	ok := false
	for _, op := range d.r.List().Ops {
		if f, isFill := op.(paint.FillRect); isFill && f.Rect.W == 2 && f.Rect.X <= r1.X+1 && f.Rect.Y >= r1.Y-1 {
			ok = true
		}
	}
	if !ok {
		t.Fatal("vertical ink bar not on the panel side")
	}
}

func TestG3BreadcrumbRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		d := NewDriver(g3Dir(rtl, func(c *Context) {
			BreadcrumbItems(c, []BreadcrumbItem{{Label: "Home"}, {Label: "List"}}).SeparatorIcon(icon.Right).Show()
		}), DriverOptions{Width: 400, Height: 100})
		d.Settle()
		// The chevron mirrors with the direction.
		if !g3HasPath(d, g3IconPath(icon.Right, geom.Rect{W: 14, H: 14}, rtl)) {
			t.Errorf("rtl=%v: separator chevron not drawn mirrored=%v", rtl, rtl)
		}
		h, _ := d.Find("Home")
		l, _ := d.Find("List")
		if rtl != (h.X > l.X) {
			t.Errorf("rtl=%v: Home %+v List %+v", rtl, h, l)
		}
	}
}

func TestG3PaginationRTL(t *testing.T) {
	page := 3
	d := NewDriver(g3Dir(true, func(c *Context) {
		Pagination(c, &page, 100, 10).Show()
	}), DriverOptions{Width: 700, Height: 100, Theme: theme.AntDesign()})
	d.Settle()
	loc := ZhCN
	prev, ok1 := d.Find(loc.PrevPage)
	next, ok2 := d.Find(loc.NextPage)
	if !ok1 || !ok2 || prev.X <= next.X {
		t.Fatalf("RTL pager: prev %+v next %+v", prev, next)
	}
	if !g3HasPath(d, g3IconPath(icon.Left, geom.Rect{W: 12, H: 12}, true)) || !g3HasPath(d, g3IconPath(icon.Right, geom.Rect{W: 12, H: 12}, true)) {
		t.Fatal("pager arrows should be mirrored")
	}
	d.Click(prev.Center())
	if page != 2 {
		t.Fatalf("prev: page %d", page)
	}
}

func TestG3StepsRTL(t *testing.T) {
	items := []StepItem{{Title: "One"}, {Title: "Two"}, {Title: "Three"}}
	d := NewDriver(g3Dir(true, func(c *Context) {
		Column(c, func() { Steps(c, items).Current(1).Show() }, layout.W(600))
	}), DriverOptions{Width: 700, Height: 200, Theme: theme.AntDesign()})
	d.Settle()
	one, _ := d.FindText("One")
	two, _ := d.FindText("Two")
	if one.X <= two.X {
		t.Fatalf("RTL step order: %+v %+v", one, two)
	}
	// The numbered circle is at the start (right) of its step and the
	// title to its left.
	circle := geom.Rect{}
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.W == 32 && f.Rect.H == 32 && one.Contains(f.Rect.Center()) {
			circle = f.Rect
		}
	}
	if circle.W == 0 || circle.Right() != one.Right() {
		t.Fatalf("circle %+v in step %+v", circle, one)
	}
	txt := g3Texts(d, one)
	if len(txt) == 0 || txt[0].X >= circle.X {
		t.Fatalf("title %v should be left of the circle %+v", txt, circle)
	}
	// The tail runs from the title towards the next step (to the left).
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.H == 1 && one.Contains(geom.Pt(f.Rect.Right()-1, f.Rect.Y)) {
			if f.Rect.X > txt[0].X {
				t.Fatalf("tail %+v right of the title %v", f.Rect, txt[0])
			}
			return
		}
	}
	t.Fatal("tail line missing")
}

func TestG3SplitterRTL(t *testing.T) {
	var sp *SplitterW
	d := NewDriver(g3Dir(true, func(c *Context) {
		sp = Splitter(c).Style(layout.W(406), layout.H(100))
		sp.Body(func() {
			SplitterPanel(c).Body(func() { Text(c, "A") })
			SplitterPanel(c).Body(func() { Text(c, "B") })
		})
	}), DriverOptions{Width: 500, Height: 200})
	d.Settle()
	a, _ := d.FindText("A")
	b, _ := d.FindText("B")
	if a.X <= b.X {
		t.Fatalf("RTL panes: A %+v B %+v", a, b)
	}
	bar, ok := d.Find("splitter-bar-1")
	if !ok {
		t.Fatal("bar missing")
	}
	// Dragging the bar left grows the start (right) pane.
	d.Drag(bar.Center(), geom.Pt(bar.Center().X-100, bar.Center().Y))
	if f := sp.Sizes()[0]; f < 0.7 || f > 0.8 {
		t.Fatalf("after drag left: start pane %v want 0.75", f)
	}
	// Left grows it too, Right shrinks it.
	bar, _ = d.Find("splitter-bar-1")
	d.Click(bar.Center())
	before := sp.Sizes()[0]
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if sp.Sizes()[0] <= before {
		t.Fatalf("Left in RTL: %v -> %v", before, sp.Sizes()[0])
	}
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyRight, 0)
	d.Settle()
	if sp.Sizes()[0] >= before {
		t.Fatalf("Right in RTL: %v -> %v", before, sp.Sizes()[0])
	}
}

func TestG3SiderRTL(t *testing.T) {
	coll := false
	var sid, cid ID
	d := NewDriver(g3Dir(true, func(c *Context) {
		Layout(c, func() {
			sid = CollapsibleSider(c, 160, &coll).Light().Body(func() {}).ID()
			cid = Content(c, func() {}).ID()
		}, layout.W(500), layout.H(300))
	}), DriverOptions{Width: 600, Height: 400, Theme: theme.AntDesign()})
	d.Settle()
	sr, cr := d.r.c.rects[sid], d.r.c.rects[cid]
	if sr.X < cr.Right() {
		t.Fatalf("sider %+v should be right of content %+v", sr, cr)
	}
	// The split line is on the sider's left (end) edge.
	found := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.W == 1 && f.Rect.H == sr.H && f.Rect.X == sr.X {
			found = true
		}
	}
	if !found {
		t.Fatal("split line not on the end edge")
	}
	// The trigger arrow (Left: collapse) is mirrored.
	if !g3HasPath(d, g3IconPath(icon.Left, geom.Rect{W: 14, H: 14}, true)) {
		t.Fatal("trigger arrow not mirrored")
	}
}

func TestG3FloatButtonRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		d := NewDriver(g3Dir(rtl, func(c *Context) {
			FloatButton(c).Icon(icon.Setting)
		}), DriverOptions{Width: 600, Height: 400})
		d.Settle()
		r, ok := d.Find("float")
		if !ok {
			t.Fatal("float button missing")
		}
		if rtl && r.X != 24 || !rtl && r.Right() != 600-24 {
			t.Errorf("rtl=%v: float button %+v", rtl, r)
		}
	}
}

func TestG3DropdownSubmenuRTL(t *testing.T) {
	open := true
	items := []MenuItem{{Key: "a", Label: "Alpha"}, {Key: "s", Label: "More", Children: []MenuItem{{Key: "x", Label: "Inner"}}}}
	d := NewDriver(g3Dir(true, func(c *Context) {
		Row(c, func() {
			Dropdown(c, items).Trigger(TriggerClick).Open(&open).Body(func() { Button(c, "Open") })
		}, layout.Pad(300))
	}), DriverOptions{Width: 900, Height: 600})
	d.Settle()
	mr, ok := d.Find("More")
	if !ok {
		t.Fatal("dropdown not open")
	}
	d.Click(mr.Center())
	ir, ok := d.FindText("Inner")
	if !ok {
		t.Fatal("submenu not open")
	}
	if ir.Right() > mr.X {
		t.Fatalf("RTL submenu %+v should open to the left of %+v", ir, mr)
	}
}
