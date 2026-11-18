package ui

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// TestV4SelIndexMatchesContains: selIndex.has equals contains on the live
// slice after any sequence of the mirrored append / remove / replace,
// on both paths (scan and set).
func TestV4SelIndexMatchesContains(t *testing.T) {
	defer func(m int) { selIndexMin = m }(selIndexMin)
	rng := rand.New(rand.NewSource(4))
	vals := func() string { return "v" + strconv.Itoa(rng.Intn(24)) }
	for _, min := range []int{0, 8, 1 << 30} {
		selIndexMin = min
		for trial := 0; trial < 200; trial++ {
			var sel []string
			for i := rng.Intn(20); i > 0; i-- {
				sel = append(sel, vals()) // duplicates included
			}
			in := newSelIndex(&sel)
			for op := 0; op < 40; op++ {
				v := vals()
				switch rng.Intn(3) {
				case 0:
					sel = append(sel, v)
					in.add(v)
				case 1:
					sel = remove(sel, v)
					in.del(v)
				default:
					sel = []string{v}
					in.reset(v)
				}
				for i := 0; i < 24; i++ {
					w := "v" + strconv.Itoa(i)
					if in.has(w) != contains(sel, w) {
						t.Fatalf("min %d trial %d op %d: has(%s) = %v, contains = %v (sel %v)", min, trial, op, w, in.has(w), contains(sel, w), sel)
					}
				}
			}
		}
	}
}

// TestV4SelIndexGroupsIdentical runs the same clicks on CheckboxGroup
// (plain, Border, Buttons, Min / Max), CheckAll and ToggleGroup (multiple
// and single) once on the old per-option scan (selIndexMin huge) and once
// on the set (selIndexMin 0): the selection (values and order), Changed,
// the pixels and the accessibility tree are identical after every step.
// The options include a duplicate value and the selection unknown values.
func TestV4SelIndexGroupsIdentical(t *testing.T) {
	defer func(m int) { selIndexMin = m }(selIndexMin)
	opts := Opts("o0", "o1", "o2", "o3", "o4", "o5", "o6", "o7", "o8", "o9", "o10", "o11")
	opts = append(opts, Option{Label: "dupA", Value: "dup"}, Option{Label: "dupB", Value: "dup"})
	opts[3].Disabled = true
	start := func() []string {
		return []string{"o1", "x-unknown", "o3", "o5", "o6", "o7", "o8", "o9", "dup", "o10", "y-unknown"}
	}
	clicks := []string{"o2", "o5", "dupA", "dupB", "o3", "o0", "o11", "o1", "全选", "全选", "o4", "dupB", "o6"}
	kinds := []struct {
		name string
		show func(c *Context, sel *[]string) bool
	}{
		{"CheckboxGroup", func(c *Context, sel *[]string) bool { return CheckboxGroup(c, sel, opts).Changed() }},
		{"Border", func(c *Context, sel *[]string) bool { return CheckboxGroup(c, sel, opts).Border().Changed() }},
		{"Buttons", func(c *Context, sel *[]string) bool { return CheckboxGroup(c, sel, opts).Buttons().Changed() }},
		{"MinMax", func(c *Context, sel *[]string) bool { return CheckboxGroup(c, sel, opts).Min(9).Max(11).Changed() }},
		{"CheckAll", func(c *Context, sel *[]string) bool { return CheckAll(c, sel, opts).Changed() }},
		{"ToggleGroup", func(c *Context, sel *[]string) bool { return ToggleGroup(c, sel, opts).Changed() }},
		{"ToggleSingle", func(c *Context, sel *[]string) bool { return ToggleGroup(c, sel, opts).Single().Changed() }},
	}
	type step struct {
		sel     string
		changed bool
		img     []byte
		tree    string
	}
	run := func(min int, show func(*Context, *[]string) bool) []step {
		selIndexMin = min
		sel := start()
		changed := false
		d := NewDriver(func(c *Context) {
			if show(c, &sel) {
				changed = true
			}
		}, DriverOptions{Width: 900, Height: 400})
		d.EnableAccessibility()
		var out []step
		snap := func() {
			img := d.Frame()
			out = append(out, step{fmt.Sprint(sel), changed, bytes.Clone(img.Pix), d.AccessibilityTree().Dump()})
			changed = false
		}
		snap()
		for _, l := range clicks {
			r, ok := d.Find(l)
			if !ok {
				continue // a label the kind lacks (全选)
			}
			// The click's own frame (the change happens mid-build), then
			// the settled one.
			p := r.Center()
			d.Inject(input.Pointer{Kind: input.Move, Pos: p})
			d.Inject(input.Pointer{Kind: input.Press, Pos: p})
			d.Inject(input.Pointer{Kind: input.Release, Pos: p})
			snap()
			d.Settle()
			snap()
		}
		return out
	}
	for _, k := range kinds {
		old, set := run(1<<30, k.show), run(0, k.show)
		moved := false
		for i := range old {
			a, b := old[i], set[i]
			if a.sel != b.sel || a.changed != b.changed {
				t.Fatalf("%s step %d: selection %s changed %v (scan) vs %s %v (set)", k.name, i, a.sel, a.changed, b.sel, b.changed)
			}
			if !bytes.Equal(a.img, b.img) {
				t.Fatalf("%s step %d: pixels differ", k.name, i)
			}
			if a.tree != b.tree {
				t.Fatalf("%s step %d: a11y tree differs:\n%s\nvs\n%s", k.name, i, a.tree, b.tree)
			}
			moved = moved || a.changed
		}
		if !moved || old[0].sel == old[len(old)-1].sel {
			t.Fatalf("%s: the clicks changed nothing (%s → %s): the test would pass vacuously", k.name, old[0].sel, old[len(old)-1].sel)
		}
	}
}

// TestV4ConversationsScrollToKeyStaleHeaders (V2 re-review m1): in the
// non-virtual Height form a group disappearing above the window (all of
// Yesterday moved to today, the slice sorted in place: same length, same
// backing array) must not offset a far ScrollToKey target by a header.
func TestV4ConversationsScrollToKeyStaleHeaders(t *testing.T) {
	stale := newV2Conv(1000, false)
	stale.scrollTo("500") // builds the row table with the Yesterday header
	now := stale.d.r.c.Now()
	for i := 3; i < 7; i++ {
		stale.xs[i].Time = now
	}
	sortConvs := func(xs []Conversation) {
		slices.SortStableFunc(xs, func(a, b Conversation) int { return b.Time.Compare(a.Time) })
	}
	sortConvs(stale.xs)
	stale.d.Settle()
	stale.scrollTo("900")
	fresh := newV2Conv(1000, false)
	for i := 3; i < 7; i++ {
		fresh.xs[i].Time = fresh.d.r.c.Now()
	}
	sortConvs(fresh.xs)
	fresh.d.Settle()
	fresh.scrollTo("900")
	if v2Label(stale.d, stale.d.r.c.Locale().Chat.Yesterday) != nil {
		t.Fatal("the Yesterday group is still there: the edit did nothing")
	}
	a, b := v2Label(stale.d, v2Conv(900)), v2Label(fresh.d, v2Conv(900))
	if a == nil || b == nil {
		t.Fatalf("conversation 900 not built: %v %v", a, b)
	}
	if !v1Near(a.abs, b.abs) {
		t.Fatalf("conversation 900 after ScrollToKey at %v (cached table), want %v (fresh)", a.abs, b.abs)
	}
}

// TestV4TabsMoreLazyIdentical: the more menu built lazily (only while it
// shows) and eagerly (every frame, as before) give the same pixels,
// accessibility tree and active tab after every step: closed, opened by
// click, keyboard moves inside, an item chosen, and opened by hover, in
// every flavor and placement.
func TestV4TabsMoreLazyIdentical(t *testing.T) {
	defer func(e bool) { tabsMoreEager = e }(tabsMoreEager)
	for name, mk := range shad2cThemes() {
		for _, pl := range []Placement{Top, Left} {
			type step struct {
				act  string
				img  []byte
				tree string
			}
			run := func(eager bool) []step {
				tabsMoreEager = eager
				act := "k3"
				var items []TabItem
				for i := 1; i <= 16; i++ {
					items = append(items, TabItem{Key: fmt.Sprint("k", i), Label: fmt.Sprint("Tab number ", i), Disabled: i == 13})
				}
				d := NewDriver(func(c *Context) {
					Column(c, func() {
						Tabs(c, &act, items).Placement(pl).Height(220).Show().Panel(func() { Text(c, "panel "+act) })
					}, layout.W(420), layout.Pad(16))
				}, DriverOptions{Width: 600, Height: 500, Theme: mk(), Animations: true})
				d.EnableAccessibility()
				var out []step
				snap := func() {
					img := d.Settle()
					out = append(out, step{act, bytes.Clone(img.Pix), d.AccessibilityTree().Dump()})
				}
				settleFrames(d, 4)
				snap()
				more, ok := d.Find(ZhCN.Tabs.More)
				if !ok {
					t.Fatalf("%s %v: no more button", name, pl)
				}
				d.Click(more.Center()) // open
				snap()
				d.Key(input.KeyDown, 0)
				snap()
				d.Key(input.KeyDown, 0)
				snap()
				d.Key(input.KeyEnter, 0) // choose
				settleFrames(d, 2)
				snap()
				d.Move(more.Center()) // hover open
				d.Advance(400 * time.Millisecond)
				snap()
				if r, ok := d.Find("Tab number 16"); ok {
					d.Click(r.Center())
					settleFrames(d, 2)
				}
				snap()
				d.Key(input.KeyEscape, 0)
				d.Move(perfOutside)
				snap()
				return out
			}
			lazy, eager := run(false), run(true)
			for i := range eager {
				a, b := eager[i], lazy[i]
				if a.act != b.act {
					t.Fatalf("%s %v step %d: active %q (eager) vs %q (lazy)", name, pl, i, a.act, b.act)
				}
				if !bytes.Equal(a.img, b.img) {
					t.Fatalf("%s %v step %d: pixels differ", name, pl, i)
				}
				if a.tree != b.tree {
					t.Fatalf("%s %v step %d: a11y tree differs:\n%s\nvs\n%s", name, pl, i, a.tree, b.tree)
				}
			}
			if eager[0].act == eager[len(eager)-1].act {
				t.Fatalf("%s %v: the steps never changed the active tab (%q)", name, pl, eager[0].act)
			}
		}
	}
}
