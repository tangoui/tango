package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// dockFixture is a dock over row(g1[a b c] g2[d]) in an 800×500 window;
// content(k) places a "content k" text and records the ID of a box built
// in the tab's content.
type dockFixture struct {
	d       *Driver
	l       *DockLayout
	w       *DockW
	ids     map[string]ID
	torn    []tearOff
	changes int
}

func newDockFixture(t *testing.T, th *theme.Theme) *dockFixture {
	f := &dockFixture{l: NewDockLayout("a", "b", "c", "d"), ids: map[string]ID{}}
	f.l.SplitTab("d", "g1", DockRight)
	f.l.Activate("a")
	f.d = NewDriver(func(c *Context) {
		f.w = Dock(c, f.l).
			Tab(func(k string) TabItem { return TabItem{Label: "tab " + k} }).
			Content(func(k string) {
				f.ids[k] = Div(c).ID()
				Text(c, "content "+k)
			}).
			OnTearOff(func(k string, p geom.Point) { f.torn = append(f.torn, tearOff{k, p}) }).
			Style(layout.H(460)).Show()
		if f.w.Changed() {
			f.changes++
		}
	}, DriverOptions{Width: 800, Height: 500, Theme: th})
	f.d.Frame()
	return f
}

func (f *dockFixture) find(t *testing.T, label string) geom.Rect {
	t.Helper()
	r, ok := f.d.Find(label)
	if !ok {
		r, ok = f.d.FindText(label)
	}
	if !ok {
		t.Fatalf("no %q", label)
	}
	return r
}

// body returns the content rectangle of the group whose active tab is key.
func (f *dockFixture) body(t *testing.T, key string) geom.Rect {
	t.Helper()
	for _, g := range f.w.st.geo {
		if gr := f.l.Group(g.group); gr != nil && gr.Active == key {
			return f.w.c.rects[g.body]
		}
	}
	t.Fatalf("no group showing %q", key)
	return geom.Rect{}
}

func TestDockDragSplitMergeInsert(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(theme.Dark())} {
		f := newDockFixture(t, th)
		// Drop tab b on the bottom part of group g2: g2 becomes a column.
		bd := f.body(t, "d")
		dragTo(f.d, f.find(t, "tab b").Center(), geom.Pt(bd.Center().X, bd.Bottom()-10), true)
		if got := dockShape(f.l.Root); got != "row(g1[a* c] column(g2[d*] g3[b*]))" {
			t.Fatalf("%v: split bottom: %s", th.Flavor, got)
		}
		if f.w.st.drag.on || f.changes == 0 || f.l.Focus != "g3" {
			t.Fatalf("%v: after drop: drag %v changes %d focus %q", th.Flavor, f.w.st.drag.on, f.changes, f.l.Focus)
		}
		// Drop tab c on the middle of the d group: merged at the end.
		bd = f.body(t, "d")
		dragTo(f.d, f.find(t, "tab c").Center(), bd.Center(), true)
		if got := dockShape(f.l.Root); got != "row(g1[a*] column(g2[d c*] g3[b*]))" {
			t.Fatalf("%v: merge: %s", th.Flavor, got)
		}
		// Drop tab a on the tab bar of g2, before d: g1 empties and goes.
		rd := f.find(t, "tab d")
		dragTo(f.d, f.find(t, "tab a").Center(), geom.Pt(rd.X+3, rd.Center().Y), true)
		if got := dockShape(f.l.Root); got != "column(g2[a* d c] g3[b*])" {
			t.Fatalf("%v: insert: %s", th.Flavor, got)
		}
		// Dropping a group's only tab on its own body does nothing.
		bb := f.body(t, "b")
		dragTo(f.d, f.find(t, "tab b").Center(), geom.Pt(bb.X+5, bb.Center().Y), true)
		if got := dockShape(f.l.Root); got != "column(g2[a* d c] g3[b*])" {
			t.Fatalf("%v: self drop: %s", th.Flavor, got)
		}
	}
}

func TestDockContentKeepsIdentity(t *testing.T) {
	f := newDockFixture(t, nil)
	before := f.ids["a"]
	bd := f.body(t, "d")
	dragTo(f.d, f.find(t, "tab a").Center(), geom.Pt(bd.Center().X, bd.Y+8), true)
	if got := dockShape(f.l.Root); got != "row(g1[b* c] column(g3[a*] g2[d*]))" {
		t.Fatalf("split top: %s", got)
	}
	if f.ids["a"] != before {
		t.Fatal("the content of a moved tab changed identity")
	}
}

func TestDockPreviewAndTearOff(t *testing.T) {
	f := newDockFixture(t, nil)
	bd := f.body(t, "d")
	// While dragging over the right part of g2 the target is that zone.
	dragTo(f.d, f.find(t, "tab c").Center(), geom.Pt(bd.Right()-10, bd.Center().Y), false)
	tg := f.w.st.drag.target
	if !f.w.st.drag.on || !tg.ok || tg.group != "g2" || tg.side != DockRight || tg.insert != -1 {
		t.Fatalf("preview: %+v", f.w.st.drag)
	}
	// Out of the window and released there: torn off.
	f.d.Move(geom.Pt(400, -30))
	if tg := f.w.st.drag.target; !tg.ok || !tg.tear {
		t.Fatalf("tear target: %+v", tg)
	}
	f.d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(400, -30)})
	f.d.Settle()
	if len(f.torn) != 1 || f.torn[0].key != "c" || f.torn[0].pos != geom.Pt(400, -30) {
		t.Fatalf("torn: %+v", f.torn)
	}
	if k, _, ok := f.w.TornOff(); ok || k != "" {
		// TornOff is per frame: gone after the settle frames.
		t.Fatalf("TornOff still reported: %q", k)
	}
	if got := dockShape(f.l.Root); got != "row(g1[a* b] g2[d*])" {
		t.Fatalf("after tear off: %s", got)
	}
}

func TestDockCloseMaximizeResize(t *testing.T) {
	f := newDockFixture(t, nil)
	loc := ZhCN
	// Close b with its x.
	if err := f.d.ClickLabel(loc.RemoveItem("tab b")); err != nil {
		t.Fatal(err)
	}
	if got := dockShape(f.l.Root); got != "row(g1[a* c] g2[d*])" {
		t.Fatalf("close: %s", got)
	}
	// Maximize g2 (its header button), then restore it.
	var maxBtn geom.Rect
	for _, h := range f.d.r.c.hits {
		if h.label == loc.Dock.Maximize && h.rect.X > 400 {
			maxBtn = h.rect
		}
	}
	f.d.Click(maxBtn.Center())
	if f.l.Maximized != "g2" {
		t.Fatalf("maximized %q", f.l.Maximized)
	}
	if _, ok := f.d.FindText("content a"); ok {
		t.Fatal("other groups still shown while maximized")
	}
	if err := f.d.ClickLabel(loc.Dock.Restore); err != nil {
		t.Fatal(err)
	}
	if f.l.Maximized != "" {
		t.Fatal("not restored")
	}
	// Split button: the active tab of g1 goes to a new group on its right.
	var split geom.Rect
	for _, h := range f.d.r.c.hits {
		if h.label == loc.Dock.Split && h.rect.X < 400 {
			split = h.rect
		}
	}
	f.d.Click(split.Center())
	if got := dockShape(f.l.Root); got != "row(g1[c*] g3[a*] g2[d*])" {
		t.Fatalf("split button: %s", got)
	}
	// Drag the first bar 100px to the right.
	f.l.Root.Sizes = []float64{0.5, 0.25, 0.25}
	f.d.Frame()
	f.d.Frame()
	var gut geom.Rect
	for _, h := range f.d.r.c.hits {
		if h.label == "dock-gutter-0" {
			gut = h.rect
		}
	}
	f.changes = 0
	dragTo(f.d, gut.Center(), gut.Center().Add(geom.Pt(100, 0)), true)
	total := 800 - 2*f.w.tk.GutterSize
	want := 0.5 + 100/float64(total)
	if s := f.l.Root.Sizes; s[0] < want-0.01 || s[0] > want+0.01 || s[0]+s[1] < 0.749 || s[0]+s[1] > 0.751 || s[2] != 0.25 {
		t.Fatalf("resize: %v want %v", s, want)
	}
	if f.changes == 0 {
		t.Fatal("resize not reported")
	}
	// The bar stops at MinPaneSize.
	gut.X += 100
	dragTo(f.d, gut.Center(), gut.Center().Add(geom.Pt(400, 0)), true)
	if px := f.l.Root.Sizes[1] * float64(total); px < float64(f.w.tk.MinPaneSize)-0.5 {
		t.Fatalf("pane squeezed to %vpx", px)
	}
}

func TestDockGutterKeyboardAndA11y(t *testing.T) {
	f := newDockFixture(t, nil)
	f.d.EnableAccessibility()
	f.d.Frame()
	n, ok := f.d.A11yFind(a11y.RoleSeparator, ZhCN.A11y.Splitter)
	if !ok || !n.HasRange || n.ValueNow != 50 {
		t.Fatalf("separator: %+v %v", n, ok)
	}
	var gut geom.Rect
	for _, h := range f.d.r.c.hits {
		if h.label == "dock-gutter-0" {
			gut = h.rect
		}
	}
	f.d.Click(gut.Center())
	f.d.Key(input.KeyRight, 0)
	if f.l.Root.Sizes[0] <= 0.5 {
		t.Fatalf("arrow: %v", f.l.Root.Sizes)
	}
	if rep := f.d.Audit(); rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 || rep.Count(AuditNotKeyboard) > 0 {
		for _, is := range rep.Issues {
			t.Errorf("audit: %+v", is)
		}
	}
}

func TestDockRTLZones(t *testing.T) {
	var w *DockW
	l := NewDockLayout("a", "b")
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			w = Dock(c, l).Style(layout.H(300)).Show()
		})
	}, DriverOptions{Width: 600, Height: 320})
	d.Frame()
	r := geom.Rect{X: 0, Y: 0, W: 400, H: 200}
	var left, right DockSide
	ConfigProvider(w.c).Direction(RTL).Body(func() {
		left, right = w.zone(r, geom.Pt(5, 100)), w.zone(r, geom.Pt(395, 100))
	})
	if left != DockRight || right != DockLeft {
		t.Fatalf("rtl zones: %v %v", left, right)
	}
	// Splitting off to the logical left puts the new group on the right.
	l.SplitTab("b", "g1", DockLeft)
	d.Frame()
	d.Frame()
	ra, _ := d.Find("a")
	rb, _ := d.Find("b")
	if rb.X < ra.X {
		t.Fatalf("rtl split: b at %v, a at %v", rb, ra)
	}
}
