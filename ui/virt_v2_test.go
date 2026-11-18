package ui

// Batch 3 / V2: the opt-in virtual form of List, ProList and
// Conversations (Height + Virtual).

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// v2Label returns the node labelled exactly s in the page (nil: not built).
func v2Label(d *Driver, s string) *node {
	var walk func(n *node) *node
	walk = func(n *node) *node {
		if n.label == s {
			return n
		}
		for _, k := range n.kids {
			if f := walk(k); f != nil {
				return f
			}
		}
		return nil
	}
	return walk(d.r.c.root)
}

func v2Item(i int) string { return "item-" + strconv.Itoa(i) }

// v2List is a page with a List of n two-line items; cfg configures it
// before Item on every frame. built counts the items built by the last
// frame.
type v2List struct {
	d     *Driver
	w     *ListW
	built int
}

func newV2List(n int, cfg func(w *ListW), item func(c *Context, i int)) *v2List {
	t := &v2List{}
	if item == nil {
		item = func(c *Context, i int) { ListItemMeta(c, nil, v2Item(i), "desc") }
	}
	t.d = driver(func(c *Context) {
		t.built = 0
		Column(c, func() {
			w := List(c, n)
			if cfg != nil {
				cfg(w)
			}
			t.w = w.Item(func(i int) {
				t.built++
				item(c, i)
			})
		}, layout.Pad(10), layout.W(500))
	})
	t.d.Settle()
	return t
}

func (t *v2List) viewRect() (y, h float32) {
	r := t.d.r.c.rects[t.w.view]
	return r.Y, r.H
}

func TestV2ListWindow(t *testing.T) {
	l := newV2List(1000, func(w *ListW) { w.Height(300).Virtual() }, nil)
	if l.built == 0 || l.built > 20 {
		t.Fatalf("1000 items built %d, want a window", l.built)
	}
	a, b := l.w.Range()
	if a != 0 || b != l.built {
		t.Fatalf("Range %d..%d, built %d", a, b, l.built)
	}
	if v2Label(l.d, v2Item(0)) == nil || v2Label(l.d, v2Item(999)) != nil {
		t.Fatal("window does not start at the top")
	}
	v1ScrollAll(l.d, 72*400)
	a, b = l.w.Range()
	if a < 300 || b > 700 || v2Label(l.d, v2Item(0)) != nil || v2Label(l.d, v2Item(a)) == nil {
		t.Fatalf("after scrolling: Range %d..%d", a, b)
	}
	if l.built > 20 {
		t.Fatalf("scrolled window built %d items", l.built)
	}
	// Without Height, Virtual is ignored.
	l = newV2List(50, func(w *ListW) { w.Virtual() }, nil)
	if a, b := l.w.Range(); l.built != 50 || a != 0 || b != 50 {
		t.Fatalf("Virtual without Height built %d of 50 (Range %d..%d)", l.built, a, b)
	}
}

// TestV2ListPositions: the windowed list places its items where the full
// list in the same viewport does, at the top and after ScrollToKey.
func TestV2ListPositions(t *testing.T) {
	to := -1
	cfg := func(virt bool) func(w *ListW) {
		return func(w *ListW) {
			w.Height(300)
			if virt {
				w.Virtual()
			}
			if to >= 0 {
				w.ScrollToKey(to, AlignTop, 0)
			}
		}
	}
	full := newV2List(1000, cfg(false), nil)
	virt := newV2List(1000, cfg(true), nil)
	for i := 0; i < 3; i++ {
		f, v := v2Label(full.d, v2Item(i)), v2Label(virt.d, v2Item(i))
		if f == nil || v == nil || !v1Near(f.abs, v.abs) {
			t.Fatalf("item %d: full %v, virtual %v", i, f, v)
		}
	}
	for _, k := range []int{500, 37, 998} {
		to = k
		full.d.Frame()
		virt.d.Frame()
		to = -1
		full.d.Settle()
		virt.d.Settle()
		f, v := v2Label(full.d, v2Item(k)), v2Label(virt.d, v2Item(k))
		if f == nil || v == nil {
			t.Fatalf("item %d not built: full %v virtual %v", k, f, v)
		}
		if !v1Near(f.abs, v.abs) {
			t.Fatalf("item %d at %v (full) vs %v (virtual)", k, f.abs, v.abs)
		}
		if a, b := virt.w.Range(); k < a || k >= b || virt.built > 20 {
			t.Fatalf("ScrollToKey(%d): Range %d..%d, %d built", k, a, b, virt.built)
		}
	}
	// The last item ends at the bottom of the viewport in both.
	vy, vh := virt.viewRect()
	if r := v2Label(virt.d, v2Item(998)).abs; r.Y < vy || r.Y > vy+vh {
		t.Fatalf("item 998 %v outside the viewport %v+%v", r, vy, vh)
	}
}

func TestV2ListRowKeyAndItemHeight(t *testing.T) {
	to := ""
	l := newV2List(1000, func(w *ListW) {
		w.Height(300).Virtual().ItemHeight(50).RowKey(func(i int) any { return "k" + strconv.Itoa(i) })
		if to != "" {
			w.ScrollToKey(to, AlignTop, 0)
		}
	}, nil)
	if l.built != 300/50+fillWinOverscan {
		t.Fatalf("fixed 50px items in 300px built %d", l.built)
	}
	to = "k700"
	l.d.Frame()
	to = ""
	l.d.Settle()
	a, b := l.w.Range()
	if a != 700-fillWinOverscan || b != 700+300/50+fillWinOverscan {
		t.Fatalf("Range %d..%d after ScrollToKey(k700)", a, b)
	}
	vy, _ := l.viewRect()
	// The row (padding included) is 50px; its first line of text sits
	// inside it from the viewport top.
	if r := v2Label(l.d, v2Item(700)).abs; r.Y < vy || r.Y > vy+50 {
		t.Fatalf("item 700 text %v not in the first row from %v", r, vy)
	}
}

// TestV2ListKeepFocused: the item holding the focus is still built in the
// frame the window moves away from it. Focus then leaves it once it is
// painted out of view, exactly as in the full list (a focusable scrolled
// out of a Scroll leaves the focus order).
func TestV2ListKeepFocused(t *testing.T) {
	var lost [2]bool
	for k, virt := range []bool{true, false} {
		l := newV2List(1000, func(w *ListW) {
			w.Height(300)
			if virt {
				w.Virtual()
			}
		}, func(c *Context, i int) {
			Row(c, func() {
				Text(c, v2Item(i))
				Button(c, "btn-"+strconv.Itoa(i))
			})
		})
		r, ok := l.d.Find("btn-2")
		if !ok {
			t.Fatal("button 2 not built")
		}
		l.d.Click(r.Center())
		l.d.Settle()
		focus := l.d.r.c.focus
		if focus == 0 {
			t.Fatal("button 2 not focused")
		}
		for _, s := range v1Scrolls(l.d) {
			s.off.Y = 5000
		}
		l.d.Frame()
		if virt {
			if a, _ := l.w.Range(); a < 50 {
				t.Fatalf("window did not move (first %d)", a)
			}
			if v2Label(l.d, v2Item(2)) == nil {
				t.Fatal("the focused item is not built in the frame the window left it")
			}
			if l.built > 21 {
				t.Fatalf("%d items built", l.built)
			}
		}
		l.d.Settle()
		lost[k] = l.d.r.c.focus != focus
	}
	if lost[0] != lost[1] {
		t.Fatalf("focus lost after scrolling away: virtual %v, full %v", lost[0], lost[1])
	}
}

func TestV2ListPageAndGrid(t *testing.T) {
	page := 3
	l := newV2List(1000, func(w *ListW) { w.Pagination(&page, 200).Height(300).Virtual() }, nil)
	if a, b := l.w.Range(); a != 400 || b > 420 || l.built > 20 {
		t.Fatalf("page 3 of 200: Range %d..%d, %d built", a, b, l.built)
	}
	if _, ok := l.d.FindText("5"); !ok {
		t.Fatal("pager not shown")
	}
	v1ScrollAll(l.d, 1e9)
	if a, b := l.w.Range(); b != 600 || a < 580 {
		t.Fatalf("bottom of page 3: Range %d..%d", a, b)
	}
	g := newV2List(1000, func(w *ListW) { w.Grid(4, 16).Height(300).Virtual() }, nil)
	a, b := g.w.Range()
	if a != 0 || b%4 != 0 || g.built != b || g.built > 4*12 {
		t.Fatalf("grid: Range %d..%d, %d built", a, b, g.built)
	}
	v1ScrollAll(g.d, 1e9)
	if a, b := g.w.Range(); b != 1000 || a%4 != 0 || g.built != b-a {
		t.Fatalf("grid bottom: Range %d..%d, %d built", a, b, g.built)
	}
}

func TestV2ListA11y(t *testing.T) {
	l := newV2List(1000, func(w *ListW) { w.Height(300).Virtual() }, nil)
	l.d.EnableAccessibility()
	v1ScrollAll(l.d, 72*100)
	tree := l.d.AccessibilityTree()
	list := tree.Node(a11y.NodeID(l.w.view))
	if list == nil || list.Role != a11y.RoleList || list.RowCount != 1000 {
		t.Fatalf("list node %+v, want a list of 1000 rows", list)
	}
	a, b := l.w.Range()
	items := l.d.A11yFindAll(a11y.RoleListItem, "")
	if len(items) != b-a {
		t.Fatalf("%d list items in the tree, Range %d..%d", len(items), a, b)
	}
	for _, it := range items {
		if it.SetSize != 1000 || it.PosInSet < a+1 || it.PosInSet > b {
			t.Fatalf("item %d/%d outside Range %d..%d", it.PosInSet, it.SetSize, a, b)
		}
	}
}

// ---------------------------------------------------------------------------
// ProList

func TestV2ProListWindow(t *testing.T) {
	rows := make([]int, 1000)
	for i := range rows {
		rows[i] = i
	}
	var sel []string
	page := false
	var w *ProListW[int]
	d := driver(func(c *Context) {
		Column(c, func() {
			w = ProList[int](c, rows, ProListMetas[int]{Title: func(i int) string { return v2Item(i) }}).
				RowKey(func(i int) string { return "r" + strconv.Itoa(i) }).Selection(&sel).Height(300).Virtual()
			if page {
				w.Pagination(100)
			}
			w.Show()
		}, layout.W(500))
	})
	d.Settle()
	a, b := w.list.Range()
	if a != 0 || b > 20 || v2Label(d, v2Item(0)) == nil || v2Label(d, v2Item(b)) != nil {
		t.Fatalf("Range %d..%d", a, b)
	}
	v1ScrollAll(d, 1e9)
	if a, b := w.list.Range(); b != 1000 || a < 980 || v2Label(d, v2Item(999)) == nil {
		t.Fatalf("bottom: Range %d..%d", a, b)
	}
	page = true
	v1ScrollAll(d, 0) // the offset is kept across a page change
	if a, b := w.list.Range(); a != 0 || b > 20 {
		t.Fatalf("page 1 of 100: Range %d..%d", a, b)
	}
	v1ScrollAll(d, 1e9)
	if a, b := w.list.Range(); b != 100 || a < 80 {
		t.Fatalf("bottom of page 1: Range %d..%d", a, b)
	}
}

// ---------------------------------------------------------------------------
// Conversations

func v2Conv(i int) string { return "conv-" + strconv.Itoa(i) }

// v2Convs is n conversations: the first 3 today, 4 yesterday, the rest
// earlier (three group headers).
func v2Convs(n int, now time.Time) []Conversation {
	xs := make([]Conversation, n)
	for i := range xs {
		t := now.AddDate(0, 0, -60)
		switch {
		case i < 3:
			t = now
		case i < 7:
			t = now.AddDate(0, 0, -1)
		}
		xs[i] = Conversation{Key: strconv.Itoa(i), Title: v2Conv(i), Time: t}
	}
	return xs
}

type v2ConvPage struct {
	d      *Driver
	w      *ConversationsW
	active string
	to     string
	xs     []Conversation
}

func newV2Conv(n int, virt bool) *v2ConvPage {
	p := &v2ConvPage{}
	p.d = driver(func(c *Context) {
		if p.xs == nil {
			p.xs = v2Convs(n, c.Now())
		}
		Column(c, func() {
			p.w = Conversations(c, p.xs, &p.active).Height(300)
			if virt {
				p.w.Virtual()
			}
			if p.to != "" {
				p.w.ScrollToKey(p.to, AlignTop, 0)
			}
		}, layout.Pad(10), layout.W(400))
	})
	p.d.Settle()
	return p
}

// built counts the conversation rows built.
func (p *v2ConvPage) built() int {
	n := 0
	var walk func(nd *node)
	walk = func(nd *node) {
		if nd.kind == "row" && strings.HasPrefix(nd.label, "conv-") {
			n++
		}
		for _, k := range nd.kids {
			walk(k)
		}
	}
	walk(p.d.r.c.root)
	return n
}

func (p *v2ConvPage) scrollTo(key string) {
	p.to = key
	p.d.Frame()
	p.to = ""
	p.d.Settle()
}

func TestV2ConversationsWindow(t *testing.T) {
	p := newV2Conv(1000, true)
	if n := p.built(); n == 0 || n > 20 {
		t.Fatalf("1000 conversations built %d rows", n)
	}
	if v2Label(p.d, v2Conv(999)) != nil {
		t.Fatal("last conversation built while out of view")
	}
	p.scrollTo("700")
	// Row 0 stays built: it is the Tab stop (no selection).
	if v2Label(p.d, v2Conv(700)) == nil || v2Label(p.d, v2Conv(1)) != nil || p.built() > 21 {
		t.Fatalf("after ScrollToKey(700): %d rows built", p.built())
	}
}

// TestV2ConversationsPositions: rows and group headers sit where the full
// list in the same viewport puts them.
func TestV2ConversationsPositions(t *testing.T) {
	full, virt := newV2Conv(1000, false), newV2Conv(1000, true)
	check := func(labels ...string) {
		t.Helper()
		for _, s := range labels {
			f, v := v2Label(full.d, s), v2Label(virt.d, s)
			if f == nil || v == nil || !v1Near(f.abs, v.abs) {
				t.Fatalf("%q: full %v, virtual %v", s, f, v)
			}
		}
	}
	loc := full.d.r.c.Locale().Chat
	check(v2Conv(0), v2Conv(2), loc.Today, loc.Yesterday, v2Conv(3), loc.Earlier, v2Conv(7))
	for _, k := range []int{500, 6, 999} {
		full.scrollTo(strconv.Itoa(k))
		virt.scrollTo(strconv.Itoa(k))
		check(v2Conv(k))
	}
}

func TestV2ConversationsKeepActive(t *testing.T) {
	p := newV2Conv(1000, true)
	p.active = "500"
	p.d.Settle()
	if v2Label(p.d, v2Conv(500)) == nil {
		t.Fatal("selected conversation not built while out of view")
	}
	if n := p.built(); n > 21 {
		t.Fatalf("%d rows built", n)
	}
	// The selected row is the Tab stop: keyboard users land on it.
	if nd := v2Label(p.d, v2Conv(500)); nd.noTab {
		t.Fatal("selected row is not the Tab stop")
	}
}

func TestV2ConversationsKeyboard(t *testing.T) {
	p := newV2Conv(1000, true)
	p.d.EnableAccessibility()
	r := v2Label(p.d, v2Conv(0)).abs
	p.d.Click(r.Center()) // selects and focuses row 0
	p.d.Settle()
	p.d.Key(input.KeyUp, 0) // wraps to the last row, far out of the window
	p.d.Settle()
	last := v2Label(p.d, v2Conv(999))
	if last == nil || p.d.r.c.focus != last.id {
		t.Fatalf("Up from the first row: focus %v, last row %v", p.d.r.c.focus, last)
	}
	vr := p.d.r.c.rects[p.w.b.n.id]
	if !vr.Contains(last.abs.Center()) {
		t.Fatalf("focused last row %v not scrolled into the list %v", last.abs, vr)
	}
	opt := p.d.AccessibilityTree().Node(a11y.NodeID(last.id))
	if opt == nil || opt.PosInSet != 1000 || opt.SetSize != 1000 {
		t.Fatalf("last row a11y %+v, want 1000/1000", opt)
	}
	p.d.Key(input.KeyDown, 0) // wraps back to the first
	p.d.Settle()
	if first := v2Label(p.d, v2Conv(0)); first == nil || p.d.r.c.focus != first.id {
		t.Fatal("Down from the last row did not wrap to the first")
	}
	p.d.Key(input.KeyEnd, 0)
	p.d.Settle()
	if nd := v2Label(p.d, v2Conv(999)); nd == nil || p.d.r.c.focus != nd.id {
		t.Fatal("End did not focus the last row")
	}
	p.d.Key(input.KeyHome, 0)
	p.d.Settle()
	if nd := v2Label(p.d, v2Conv(0)); nd == nil || p.d.r.c.focus != nd.id {
		t.Fatal("Home did not focus the first row")
	}
}

func TestV2ConversationsRenameKept(t *testing.T) {
	p := newV2Conv(1000, true)
	r := v2Label(p.d, v2Conv(2)).abs
	p.d.Click(r.Center())
	p.d.Settle()
	p.d.Key(input.KeyF2, 0)
	p.d.Settle()
	if p.w.st.renaming != "2" {
		t.Fatalf("renaming %q after F2", p.w.st.renaming)
	}
	for _, s := range v1Scrolls(p.d) {
		s.off.Y = 20000
	}
	p.d.Frame()
	ren := p.w.st.lay.rowOf(2)
	if ren < 0 || len(p.w.st.lay.ids.ids) == 0 {
		t.Fatal("no row table")
	}
	found := false
	var walk func(nd *node)
	walk = func(nd *node) {
		if nd.kind == "input" || strings.Contains(nd.kind, "input") {
			found = true
		}
		for _, k := range nd.kids {
			walk(k)
		}
	}
	walk(p.d.r.c.root)
	if !found {
		t.Fatal("rename input not built in the frame the window left it")
	}
}

// TestV2ConversationsDayChange: the time groups move at midnight.
func TestV2ConversationsDayChange(t *testing.T) {
	p := newV2Conv(1000, true)
	loc := p.d.r.c.Locale().Chat
	if v2Label(p.d, loc.Today) == nil {
		t.Fatal("no Today header")
	}
	p.d.Advance(24 * time.Hour)
	p.d.Settle()
	if v2Label(p.d, loc.Today) != nil || v2Label(p.d, loc.Yesterday) == nil {
		t.Fatal("groups did not follow the day change")
	}
	full := newV2Conv(1000, false)
	full.d.Advance(24 * time.Hour)
	full.d.Settle()
	for _, s := range []string{loc.Yesterday, loc.Last7Days, v2Conv(3), v2Conv(7)} {
		f, v := v2Label(full.d, s), v2Label(p.d, s)
		if f == nil || v == nil || !v1Near(f.abs, v.abs) {
			t.Fatalf("%q after a day: full %v, virtual %v", s, f, v)
		}
	}
}
