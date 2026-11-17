package ui

// Batch 3 / V1: windowed dropdown option lists (AutoComplete, Mentions
// through msOptionList), Command and Cascader / CascaderPanel columns,
// with the Virtual(on bool) escape hatch.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// v1Nodes returns the nodes of kind in the page and its overlays.
func v1Nodes(d *Driver, kind string) []*node {
	c := d.r.c
	var out []*node
	var walk func(n *node)
	walk = func(n *node) {
		if n == nil {
			return
		}
		if n.kind == kind {
			out = append(out, n)
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	for _, o := range c.overlays {
		walk(o.root)
	}
	return out
}

func v1Count(d *Driver, kind string) int { return len(v1Nodes(d, kind)) }

// v1Built returns the node of kind labelled label (nil: not built).
func v1Built(d *Driver, kind, label string) *node {
	for _, n := range v1Nodes(d, kind) {
		if n.label == label || n.idLabel == label {
			return n
		}
	}
	return nil
}

// v1Scrolls returns the scroll states of the overlays (or the page when
// there is none).
func v1Scrolls(d *Driver) []*scrollState {
	c := d.r.c
	var out []*scrollState
	var walk func(n *node)
	walk = func(n *node) {
		if n.scroll != nil {
			out = append(out, n.scroll)
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	for _, o := range c.overlays {
		walk(o.root)
	}
	return out
}

// v1ScrollAll moves every vertically scrollable container to offset y
// (clamped by the next frame).
func v1ScrollAll(d *Driver, y float64) {
	d.Frame() // a pending first-open reveal runs before the offset is set
	for _, s := range v1Scrolls(d) {
		if max(s.extent.Y, s.content.Y) > float64(s.view.H)+1 {
			s.off.Y = y
		}
	}
	d.Frame()
	d.Settle()
}

func v1Opts(n int) []Option {
	out := make([]Option, n)
	for i := range out {
		s := "option-" + strconv.Itoa(i+1)
		out[i] = Option{Label: s, Value: s}
	}
	return out
}

func v1Center(d *Driver, id ID) geom.Point {
	r := d.r.c.rects[id]
	return geom.Pt(r.X+r.W/2, r.Y+r.H/2)
}

// v1Window is the most rows a windowed 8-row option list builds: the
// viewport, the overscan on both sides, one partly visible row and the
// keyboard row.
const v1Window = 8 + 2*fillWinOverscan + 2

// ---------------------------------------------------------------------------
// AutoComplete

func v1AutoComplete(n int, virt *bool) (*Driver, **AutoCompleteW) {
	opts := v1Opts(n)
	val := ""
	var w *AutoCompleteW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = AutoComplete(c, &val, opts).Width(240)
			if virt != nil {
				w.Virtual(*virt)
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(v1Center(d, w.in.ed.id))
	if !w.IsOpen() {
		panic("autocomplete did not open")
	}
	return d, &w
}

func TestV1AutoCompleteWindow(t *testing.T) {
	d, _ := v1AutoComplete(1000, nil)
	if got := v1Count(d, "ms-option"); got == 0 || got > v1Window {
		t.Fatalf("1000 options built %d rows, want 1..%d", got, v1Window)
	}
	r, ok := d.Find("option-1")
	if !ok {
		t.Fatal("first option not built")
	}
	if v1Built(d, "ms-option", "option-1000") != nil {
		t.Fatal("last option built while out of view")
	}
	// Scrolling moves the window (option-1 stays as the keyboard row).
	v1ScrollAll(d, float64(r.H)*40)
	if v1Built(d, "ms-option", "option-2") != nil {
		t.Fatal("option 2 still built after scrolling 40 rows down")
	}
	if v1Built(d, "ms-option", "option-41") == nil {
		t.Fatal("option 41 not built after scrolling 40 rows down")
	}
	if got := v1Count(d, "ms-option"); got > v1Window {
		t.Fatalf("scrolled list built %d rows", got)
	}
}

func TestV1AutoCompleteThreshold(t *testing.T) {
	off, on := false, true
	for _, tc := range []struct {
		n    int
		virt *bool
		all  bool
	}{
		{100, nil, true},
		{101, nil, false},
		{101, &off, true},
		{1000, &off, true},
		{50, &on, false},
	} {
		d, _ := v1AutoComplete(tc.n, tc.virt)
		got := v1Count(d, "ms-option")
		if tc.all && got != tc.n || !tc.all && got > v1Window {
			t.Errorf("n=%d virt=%v: %d rows built (all=%v)", tc.n, tc.virt, got, tc.all)
		}
	}
}

func TestV1AutoCompleteKeyboardAcrossWindow(t *testing.T) {
	d, w := v1AutoComplete(1000, nil)
	d.EnableAccessibility()
	d.Settle()
	d.Key(input.KeyUp, 0) // wraps to the last option
	d.Settle()
	last := v1Built(d, "ms-option", "option-1000")
	if last == nil {
		t.Fatal("keyboard row not built")
	}
	if lr := d.r.c.rects[(*w).listID]; !lr.Contains(geom.Pt(last.abs.X+1, last.abs.Y+last.abs.H/2)) {
		t.Fatalf("keyboard row %v not scrolled into the list %v", last.abs, lr)
	}
	cb := d.AccessibilityTree().Node(a11y.NodeID((*w).in.ed.id))
	if cb == nil || cb.ActiveDescendant != a11y.NodeID(last.id) {
		t.Fatalf("ActiveDescendant %v, want the last option %d", cb, last.id)
	}
	opt := d.AccessibilityTree().Node(a11y.NodeID(last.id))
	if opt == nil || opt.PosInSet != 1000 || opt.SetSize != 1000 {
		t.Fatalf("last option a11y %v, want 1000/1000", opt)
	}
	// Down wraps back to the first: the window follows.
	d.Key(input.KeyDown, 0)
	d.Settle()
	if v1Built(d, "ms-option", "option-1") == nil || v1Built(d, "ms-option", "option-1000") != nil {
		t.Fatal("window did not follow the keyboard back to the top")
	}
}

func TestV1AutoCompleteKeepRow(t *testing.T) {
	d, w := v1AutoComplete(1000, nil)
	d.EnableAccessibility()
	d.Key(input.KeyDown, 0) // active = option-2
	d.Settle()
	v1ScrollAll(d, 32*500) // far away from the keyboard row, pointer on the input
	if v1Built(d, "ms-option", "option-500") == nil && v1Built(d, "ms-option", "option-501") == nil {
		t.Fatal("window did not move")
	}
	keep := v1Built(d, "ms-option", "option-2")
	if keep == nil {
		t.Fatal("keyboard row not built while out of view")
	}
	cb := d.AccessibilityTree().Node(a11y.NodeID((*w).in.ed.id))
	if cb == nil || cb.ActiveDescendant != a11y.NodeID(keep.id) {
		t.Fatalf("ActiveDescendant %v, want the kept row %d", cb, keep.id)
	}
	if got := v1Count(d, "ms-option"); got > v1Window {
		t.Fatalf("%d rows built", got)
	}
}

// TestV1AutoCompleteWidth: the widest label is far out of view; the
// windowed dropdown is exactly as wide and high as the full one.
func TestV1AutoCompleteWidth(t *testing.T) {
	size := func(virt bool) geom.Rect {
		opts := v1Opts(150)
		opts[140].Label = strings.Repeat("long option label ", 2)
		val := ""
		var w *AutoCompleteW
		d := driver(func(c *Context) {
			Column(c, func() { w = AutoComplete(c, &val, opts).Width(160).Virtual(virt) }, layout.Pad(20))
		})
		d.Frame()
		d.Click(v1Center(d, w.in.ed.id))
		d.Settle()
		if got := v1Count(d, "ms-option"); virt == (got == 150) {
			t.Fatalf("virt=%v built %d rows", virt, got)
		}
		return d.r.c.rects[w.listID]
	}
	full, win := size(false), size(true)
	if full.W <= 160 || full != win {
		t.Fatalf("windowed list %v, full list %v", win, full)
	}
}

// ---------------------------------------------------------------------------
// Mentions

func v1Mentions(n int, virt *bool) (*Driver, **MentionsW) {
	opts := v1Opts(n)
	val := "@"
	var w *MentionsW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = Mentions(c, &val, opts).Width(240).Open()
			if virt != nil {
				w.Virtual(*virt)
			}
		}, layout.Pad(20))
	})
	d.Settle()
	return d, &w
}

func TestV1MentionsWindow(t *testing.T) {
	off := false
	d, _ := v1Mentions(1000, nil)
	if got := v1Count(d, "ms-option"); got == 0 || got > v1Window {
		t.Fatalf("1000 suggestions built %d rows", got)
	}
	if _, ok := d.Find("option-1"); !ok {
		t.Fatal("first suggestion not built")
	}
	d, _ = v1Mentions(100, nil)
	if got := v1Count(d, "ms-option"); got != 100 {
		t.Fatalf("100 suggestions built %d rows", got)
	}
	d, _ = v1Mentions(101, &off)
	if got := v1Count(d, "ms-option"); got != 101 {
		t.Fatalf("Virtual(false): %d rows of 101", got)
	}
}

// ---------------------------------------------------------------------------
// Command

func v1CommandItems(n int, groups ...string) []CommandItem {
	out := make([]CommandItem, n)
	for i := range out {
		s := strconv.Itoa(i + 1)
		g := ""
		if len(groups) > 0 {
			g = groups[i*len(groups)/n]
		}
		out[i] = CommandItem{Value: "cmd-" + s, Label: "command " + s, Group: g}
	}
	return out
}

func v1Command(items []CommandItem, virt *bool) (*Driver, **CommandW) {
	var w *CommandW
	d := driver(func(c *Context) {
		Row(c, func() {
			w = Command(c, items)
			if virt != nil {
				w.Virtual(*virt)
			}
		}, layout.Pad(10))
	})
	d.Settle()
	return d, &w
}

func TestV1CommandWindow(t *testing.T) {
	d, w := v1Command(v1CommandItems(1000, "group"), nil)
	d.EnableAccessibility()
	d.Settle()
	got := v1Count(d, "commanditem")
	if got == 0 || got > 20 {
		t.Fatalf("1000 items built %d rows", got)
	}
	if v1Built(d, "commanditem", "command 1") == nil || v1Built(d, "commanditem", "command 1000") != nil {
		t.Fatal("window does not start at the top")
	}
	g, ok := d.A11yFind(a11y.RoleGroup, "group")
	if !ok || g.SetSize != 1000 {
		t.Fatalf("group %v, want SetSize 1000", g)
	}
	// End moves to the last item: the list follows.
	d.Click(v1Center(d, (*w).in.ed.id))
	d.Key(input.KeyEnd, 0)
	d.Settle()
	last := v1Built(d, "commanditem", "command 1000")
	if last == nil || v1Built(d, "commanditem", "command 1") != nil {
		t.Fatal("window did not follow End")
	}
	lr := d.r.c.rects[(*w).list]
	if last.abs.Bottom() > lr.Bottom()+0.5 || last.abs.Y < lr.Y-0.5 {
		t.Fatalf("active row %v not scrolled into the list %v", last.abs, lr)
	}
	cb := d.AccessibilityTree().Node(a11y.NodeID((*w).in.ed.id))
	if cb == nil || cb.ActiveDescendant != a11y.NodeID(last.id) {
		t.Fatalf("ActiveDescendant %v, want %d", cb, last.id)
	}
	opt := d.AccessibilityTree().Node(a11y.NodeID(last.id))
	if opt == nil || opt.PosInSet != 1000 || opt.SetSize != 1000 {
		t.Fatalf("last item a11y %v", opt)
	}
	// Scrolled away from the active item, it is still built.
	v1ScrollAll(d, 0)
	if v1Built(d, "commanditem", "command 1") == nil {
		t.Fatal("window did not move back to the top")
	}
	if keep := v1Built(d, "commanditem", "command 1000"); keep == nil {
		t.Fatal("active item not built while out of view")
	}
}

func TestV1CommandThreshold(t *testing.T) {
	off, on := false, true
	for _, tc := range []struct {
		n    int
		virt *bool
		all  bool
	}{
		{100, nil, true},
		{101, nil, false},
		{101, &off, true},
		{60, &on, false},
	} {
		d, _ := v1Command(v1CommandItems(tc.n), tc.virt)
		got := v1Count(d, "commanditem")
		if tc.all && got != tc.n || !tc.all && got >= tc.n {
			t.Errorf("n=%d virt=%v: %d items built (all=%v)", tc.n, tc.virt, got, tc.all)
		}
	}
}

// TestV1CommandGroups: two titled groups and a separator; the windowed
// list has the full list's size and its rows line up with the full list's
// at the same scroll offset.
func TestV1CommandGroups(t *testing.T) {
	items := v1CommandItems(240, "alpha", "beta")
	items[200].Label = strings.Repeat("long command label ", 2)
	off, on := false, true
	geo := func(virt *bool, y float64) (geom.Rect, map[string]geom.Rect) {
		d, w := v1Command(items, virt)
		v1ScrollAll(d, y)
		rows := map[string]geom.Rect{}
		for _, n := range v1Nodes(d, "commanditem") {
			rows[n.label] = n.abs
		}
		return d.r.c.rects[(*w).list], rows
	}
	for _, y := range []float64{0, 60 * 32, 3000, 1e6} {
		fullBox, fullRows := geo(&off, y)
		winBox, winRows := geo(&on, y)
		if fullBox.W != winBox.W {
			t.Fatalf("y=%v: list width %v windowed, %v full", y, winBox.W, fullBox.W)
		}
		if len(winRows) >= len(fullRows) {
			t.Fatalf("y=%v: windowed list built %d rows of %d", y, len(winRows), len(fullRows))
		}
		for label, r := range winRows {
			if fr := fullRows[label]; !v1Near(fr, r) {
				t.Fatalf("y=%v: %s at %v windowed, %v full", y, label, r, fr)
			}
		}
	}
}

// v1Near compares rects to within float32 rounding of long sums.
func v1Near(a, b geom.Rect) bool {
	near := func(x, y float32) bool { return x-y < 0.01 && y-x < 0.01 }
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

// ---------------------------------------------------------------------------
// Cascader / CascaderPanel

func v1CasOpts(n int) []CascaderOption {
	out := make([]CascaderOption, n)
	for i := range out {
		out[i] = CascaderOption{Label: "item " + strconv.Itoa(i), Value: "n" + strconv.Itoa(i)}
	}
	return out
}

func v1Panel(opts []CascaderOption, val []string, virt *bool) (*Driver, **CascaderPanelW) {
	var w *CascaderPanelW
	d := driver(func(c *Context) {
		Row(c, func() {
			v := State(c, hashID("v1.cas"), func() []string { return val })
			w = CascaderPanel(c, v, opts)
			if virt != nil {
				w.Virtual(*virt)
			}
		}, layout.Pad(10))
	})
	d.Settle()
	return d, &w
}

func TestV1CascaderWindow(t *testing.T) {
	opts := v1CasOpts(1000)
	var w *CascaderW
	val := []string(nil)
	d := driver(func(c *Context) {
		Column(c, func() { w = Cascader(c, &val, opts).Width(240).Open() }, layout.Pad(10))
	})
	d.Settle()
	if got := v1Count(d, "cas-option"); got == 0 || got > 20 {
		t.Fatalf("1000 options built %d rows", got)
	}
	if v1Built(d, "cas-option", "item 0") == nil || v1Built(d, "cas-option", "item 999") != nil {
		t.Fatal("window does not start at the top")
	}
	if ext := perfCollExtent(d, false); ext < 1000*float64(w.tk.OptionHeight) {
		t.Fatalf("scroll extent %v shorter than 1000 rows", ext)
	}
}

func TestV1CascaderThreshold(t *testing.T) {
	off, on := false, true
	for _, tc := range []struct {
		n    int
		virt *bool
		all  bool
	}{
		{100, nil, true},
		{101, nil, false},
		{101, &off, true},
		{60, &on, false},
	} {
		d, _ := v1Panel(v1CasOpts(tc.n), nil, tc.virt)
		got := v1Count(d, "cas-option")
		if tc.all && got != tc.n || !tc.all && got >= tc.n {
			t.Errorf("n=%d virt=%v: %d rows built (all=%v)", tc.n, tc.virt, got, tc.all)
		}
	}
}

// TestV1CascaderWidth: a long label out of view widens the column exactly
// as in the full column, and a child column follows.
func TestV1CascaderWidth(t *testing.T) {
	opts := v1CasOpts(400)
	opts[350].Label = strings.Repeat("long cascader label ", 2)
	opts[0].Children = v1CasOpts(5)
	off, on := false, true
	fd, fw := v1Panel(opts, []string{"n0", "n1"}, &off)
	wd, ww := v1Panel(opts, []string{"n0", "n1"}, &on)
	fr, wr := fd.r.c.rects[(*fw).ID()], wd.r.c.rects[(*ww).ID()]
	if fr != wr {
		t.Fatalf("windowed panel %v, full panel %v", wr, fr)
	}
	if v1Count(wd, "cas-option") >= v1Count(fd, "cas-option") {
		t.Fatal("windowed panel built every row")
	}
}

func TestV1CascaderKeepAndKeyboard(t *testing.T) {
	d, w := v1Panel(v1CasOpts(1000), []string{"n500"}, nil)
	d.EnableAccessibility()
	d.Settle()
	row := v1Built(d, "cas-option", "item 500")
	if row == nil {
		t.Fatal("expanded row not built (and revealed) on open")
	}
	opt := d.AccessibilityTree().Node(a11y.NodeID(row.id))
	if opt == nil || opt.PosInSet != 501 || opt.SetSize != 1000 {
		t.Fatalf("row a11y %v, want 501/1000", opt)
	}
	// A click on the row focuses the panel (and keeps the path).
	d.Click(geom.Pt(row.abs.X+row.abs.W/2, row.abs.Y+row.abs.H/2))
	d.Move(geom.Pt(590, 390)) // off the column: no hover
	v1ScrollAll(d, 0)
	keep := v1Built(d, "cas-option", "item 500")
	if keep == nil || v1Built(d, "cas-option", "item 0") == nil {
		t.Fatal("expanded row not kept while out of view")
	}
	panel := d.AccessibilityTree().Node(a11y.NodeID((*w).ID()))
	if panel == nil || panel.ActiveDescendant != a11y.NodeID(keep.id) {
		t.Fatalf("panel ActiveDescendant %v, want %d", panel, keep.id)
	}
	// The arrow keys move the expanded row across the window: the column
	// follows.
	for range 30 {
		d.Key(input.KeyDown, 0)
	}
	d.Settle()
	moved := v1Built(d, "cas-option", "item 530")
	if moved == nil {
		t.Fatal("keyboard row not built")
	}
	pr := d.r.c.rects[(*w).ID()]
	if moved.abs.Y < pr.Y || moved.abs.Bottom() > pr.Bottom() {
		t.Fatalf("keyboard row %v not in view %v", moved.abs, pr)
	}
}

// ---------------------------------------------------------------------------
// Fix round 1: live fields, cache gating, pointer on a scrolled window

// v1ACPage is an AutoComplete over opts that records picks.
func v1ACPage(opts []Option, val *string, picked *string) (func(*Context), **AutoCompleteW) {
	var w *AutoCompleteW
	return func(c *Context) {
		Column(c, func() {
			w = AutoComplete(c, val, opts).Width(240)
			if v, ok := w.Selected(); ok {
				*picked = v
			}
		}, layout.Pad(20))
	}, &w
}

// TestV1AutoCompleteLiveFields: above the threshold the matches are
// cached, but an option changed in place shows its new label, cannot be
// picked once disabled, and a pick inserts its current value.
func TestV1AutoCompleteLiveFields(t *testing.T) {
	opts := v1Opts(500)
	val, picked := "", ""
	page, w := v1ACPage(opts, &val, &picked)
	d := driver(page)
	d.Frame()
	d.Click(v1Center(d, (*w).in.ed.id))
	d.Type("o") // all 500 match: virtual
	if got := v1Count(d, "ms-option"); got > v1Window {
		t.Fatalf("%d rows built", got)
	}
	opts[0].Label = "renamed option"
	opts[1].Disabled = true
	opts[2].Value = "fresh-value"
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "renamed option") == nil {
		t.Fatal("in-place label change not shown")
	}
	dis := v1Built(d, "ms-option", "option-2")
	if dis == nil {
		t.Fatal("option-2 not built")
	}
	d.Click(geom.Pt(dis.abs.X+dis.abs.W/2, dis.abs.Y+dis.abs.H/2))
	if picked != "" || val != "o" {
		t.Fatalf("disabled option picked: %q / %q", picked, val)
	}
	d.Click(v1Center(d, (*w).in.ed.id))
	d.Key(input.KeyDown, 0) // skips the disabled option-2
	d.Key(input.KeyEnter, 0)
	if val != "fresh-value" || picked != "fresh-value" {
		t.Fatalf("pick inserted %q (picked %q), want the current value", val, picked)
	}
}

// TestV1AutoCompleteGate: a query with at most 100 matches is a full list,
// matched again every frame, so an option renamed in place joins it.
func TestV1AutoCompleteGate(t *testing.T) {
	opts := v1Opts(500)
	val, picked := "", ""
	page, w := v1ACPage(opts, &val, &picked)
	d := driver(page)
	d.Frame()
	d.Click(v1Center(d, (*w).in.ed.id))
	d.Type("option-49") // option-49, option-490..499
	if got := v1Count(d, "ms-option"); got != 11 {
		t.Fatalf("%d rows built, want the 11 matches", got)
	}
	opts[0].Label = "option-49 renamed"
	d.Frame()
	d.Settle()
	if got := v1Count(d, "ms-option"); got != 12 || v1Built(d, "ms-option", "option-49 renamed") == nil {
		t.Fatalf("renamed option did not join the short list (%d rows)", got)
	}
}

func TestV1MentionsLiveFields(t *testing.T) {
	opts := v1Opts(500)
	val := "@o"
	var w *MentionsW
	d := driver(func(c *Context) {
		Column(c, func() { w = Mentions(c, &val, opts).Width(240).Open() }, layout.Pad(20))
	})
	d.Settle()
	if got := v1Count(d, "ms-option"); got == 0 || got > v1Window {
		t.Fatalf("%d rows built", got)
	}
	opts[0].Label = "renamed option"
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "renamed option") == nil {
		t.Fatal("in-place label change not shown")
	}
	_ = w
}

// TestV1AutoCompleteScrolledClick: a click on a row of a scrolled window
// picks that row.
func TestV1AutoCompleteScrolledClick(t *testing.T) {
	opts := v1Opts(1000)
	val, picked := "", ""
	page, w := v1ACPage(opts, &val, &picked)
	d := driver(page)
	d.Frame()
	d.Click(v1Center(d, (*w).in.ed.id))
	first, ok := d.Find("option-1")
	if !ok {
		t.Fatal("list not open")
	}
	v1ScrollAll(d, float64(first.H)*40)
	row := v1Built(d, "ms-option", "option-41")
	if row == nil {
		t.Fatal("option-41 not built after scrolling 40 rows")
	}
	if lr := d.r.c.rects[(*w).listID]; !lr.Contains(geom.Pt(row.abs.X+1, row.abs.Y+row.abs.H/2)) {
		t.Fatalf("option-41 %v not in the list %v", row.abs, lr)
	}
	d.Click(geom.Pt(row.abs.X+row.abs.W/2, row.abs.Y+row.abs.H/2))
	if picked != "option-41" || val != "option-41" {
		t.Fatalf("picked %q / %q, want option-41", picked, val)
	}
}

func TestV1MentionsKeyboardAcrossWindow(t *testing.T) {
	opts := v1Opts(1000)
	val := "@"
	var w *MentionsW
	d := driver(func(c *Context) {
		Column(c, func() { w = Mentions(c, &val, opts).Width(240) }, layout.Pad(20))
	})
	d.EnableAccessibility()
	d.Settle()
	er := d.r.c.rects[w.ed.id]
	d.Click(geom.Pt(er.Right()-4, er.Y+er.H/2)) // caret after "@"
	d.Settle()
	if !w.IsOpen() {
		t.Fatal("suggestions not open")
	}
	d.Key(input.KeyUp, 0) // wraps to the last
	d.Settle()
	last := v1Built(d, "ms-option", "option-1000")
	if last == nil || v1Built(d, "ms-option", "option-1") != nil {
		t.Fatal("window did not follow the keyboard to the end")
	}
	tb := d.AccessibilityTree().Node(a11y.NodeID(w.ed.id))
	if tb == nil || tb.ActiveDescendant != a11y.NodeID(last.id) {
		t.Fatalf("ActiveDescendant %v, want %d", tb, last.id)
	}
}

// TestV1CommandKeepGroup: the active item built out of view stays inside
// a node of its group.
func TestV1CommandKeepGroup(t *testing.T) {
	d, w := v1Command(v1CommandItems(1000, "group"), nil)
	d.EnableAccessibility()
	d.Settle()
	d.Click(v1Center(d, (*w).in.ed.id))
	d.Key(input.KeyEnd, 0)
	d.Settle()
	v1ScrollAll(d, 0)
	keep := v1Built(d, "commanditem", "command 1000")
	if keep == nil {
		t.Fatal("active item not built while out of view")
	}
	g := d.AccessibilityTree().Ancestor(a11y.NodeID(keep.id), a11y.RoleGroup)
	if g == nil || g.Name != "group" {
		t.Fatalf("kept item's group %v, want \"group\"", g)
	}
}

// TestV1MentionsEmptyQueryLive: with an empty query ("@") every option is
// shown straight from the slice: an in-place label, Disabled or Value
// change shows (and picks) at once.
func TestV1MentionsEmptyQueryLive(t *testing.T) {
	opts := v1Opts(500)
	val := "@"
	var w *MentionsW
	d := driver(func(c *Context) {
		Column(c, func() { w = Mentions(c, &val, opts).Width(240) }, layout.Pad(20))
	})
	d.Settle()
	er := d.r.c.rects[w.ed.id]
	d.Click(geom.Pt(er.Right()-4, er.Y+er.H/2)) // caret after "@"
	d.Settle()
	if !w.IsOpen() {
		t.Fatal("suggestions not open")
	}
	opts[0].Label = "renamed option"
	opts[1].Disabled = true
	opts[2].Value = "fresh"
	d.Frame()
	d.Settle()
	if v1Built(d, "ms-option", "renamed option") == nil {
		t.Fatal("in-place label change not shown")
	}
	dis := v1Built(d, "ms-option", "option-2")
	if dis == nil || !dis.disabled {
		t.Fatal("in-place Disabled not shown")
	}
	d.Key(input.KeyDown, 0) // skips the disabled option-2
	d.Key(input.KeyEnter, 0)
	if !strings.HasPrefix(val, "@fresh") {
		t.Fatalf("pick inserted %q, want the current value", val)
	}
}
