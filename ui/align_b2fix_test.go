package ui

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Fixes from the batch-2 review (.superpowers/sdd/2026-11-14-spec-align-b3/
// b2-review.md).

// b2fixDashes returns the bounds of the dotted axis strokes (tail-colored
// paths; the solid axis is a FillRect).
func b2fixDashes(d *Driver, tail theme.Color) []geom.Rect {
	var out []geom.Rect
	for _, o := range d.r.List().Ops {
		if f, ok := o.(paint.FillPath); ok && f.Color == tail {
			out = append(out, f.Path.Bounds())
		}
	}
	return out
}

// TestTimelinePendingTailDashed: rc-steps gives a rail the next step's
// status, or its own under railFollowPrevStatus (antd Timeline reverse), so
// the axis entering a busy item is dotted, or the one leaving it when
// reversed (antd 6.7 Timeline.js:104, rc-steps Step.js:147).
func TestTimelinePendingTailDashed(t *testing.T) {
	cases := []struct {
		name    string
		items   []TimelineItem
		pending string
		reverse bool
		from    string // the dotted part runs from this text's item...
		to      string // ...to this one's (in drawing order)
	}{
		{"pending", []TimelineItem{{Content: "a"}, {Content: "b"}}, "p", false, "b", "p"},
		{"pending reverse", []TimelineItem{{Content: "a"}, {Content: "b"}}, "p", true, "p", "b"},
		{"loading", []TimelineItem{{Content: "a"}, {Content: "b", Loading: true}, {Content: "c"}}, "", false, "a", "b"},
		{"loading reverse", []TimelineItem{{Content: "a"}, {Content: "b", Loading: true}, {Content: "c"}}, "", true, "b", "a"},
	}
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.Shadcn()} {
		tail := th.Components.Timeline.TailColor
		for _, tc := range cases {
			for _, horiz := range []bool{false, true} {
				d := NewDriver(func(c *Context) {
					w := Timeline(c, tc.items).Pending(tc.pending)
					if tc.reverse {
						w.Reverse()
					}
					if horiz {
						w.Horizontal()
					}
					w.Show()
				}, DriverOptions{Theme: th, Width: 600, Height: 400})
				d.Frame()
				from, to := mustFindText(t, d, tc.from), mustFindText(t, d, tc.to)
				ds := b2fixDashes(d, tail)
				if len(ds) == 0 {
					t.Fatalf("%v %s horiz=%v: no dotted axis", th.Flavor, tc.name, horiz)
				}
				// Exactly the busy rail is dotted: vertically its two halves
				// (drawn by the items at both ends), horizontally one tail;
				// every other segment is solid (a FillRect).
				want := 2
				if horiz {
					want = 1
				}
				if len(ds) != want {
					t.Errorf("%v %s horiz=%v: %d dotted pieces %v, want %d", th.Flavor, tc.name, horiz, len(ds), ds, want)
				}
				u := ds[0]
				for _, r := range ds[1:] {
					u = u.Union(r)
				}
				// The dotted span runs from the "from" dot to the "to" dot;
				// each dot sits on its text's first line (horizontally: over
				// the text's centre).
				lo, hi := u.Y, u.Bottom()
				aLo, aHi, bLo, bHi := from.Y-12, from.Bottom(), to.Y, to.Bottom()+12
				if horiz {
					lo, hi = min(u.X, u.Right()), max(u.X, u.Right())
					fc, tc := from.Center().X, to.Center().X
					aLo, aHi, bLo, bHi = min(fc, tc)-8, min(fc, tc)+8, max(fc, tc)-8, max(fc, tc)+8
				}
				if lo < aLo || lo > aHi || hi < bLo || hi > bHi {
					t.Errorf("%v %s horiz=%v: dotted span %v not %q..%q (%v %v)", th.Flavor, tc.name, horiz, u, tc.from, tc.to, from, to)
				}
			}
		}
	}
	// Element draws no dotted axis (BusyTailDashed false).
	th := theme.ElementPlus()
	d := NewDriver(func(c *Context) {
		Timeline(c, []TimelineItem{{Content: "a"}, {Content: "b"}}).Pending("p").Show()
	}, DriverOptions{Theme: th})
	d.Frame()
	for _, o := range d.r.List().Ops {
		if f, ok := o.(paint.FillPath); ok && f.Color == th.Components.Timeline.TailColor {
			if b := f.Path.Bounds(); b.H > b.W*3 {
				t.Fatalf("Element: dotted axis %v", b)
			}
		}
	}
}

// TestAlignB2FixTagPlainColors: Effect("plain") keeps the tint's label and
// border (Element el-tag--plain: the type color with its light-5 border, as
// before c04a878); a tag without a color reads TagToken.PlainColor, so the
// shadcn default stays readable without a contrast heuristic.
func TestAlignB2FixTagPlainColors(t *testing.T) {
	plain := func(th *theme.Theme, col string) (bg, border, fg theme.Color) {
		var tg *TagW
		NewDriver(func(c *Context) { tg = Tag(c, "plain").Effect("plain").Color(col) }, DriverOptions{Theme: th}).Frame()
		return tg.colors()
	}
	el := theme.ElementPlus()
	for _, col := range []string{"success", "warning", "error", "danger", "info", "primary"} {
		p := el.ColorOf(col)
		if bg, border, fg := plain(el, col); fg != p.Base || border != p.Border || bg != el.BgContainer {
			t.Errorf("Element plain %s: bg %v border %v fg %v, want %v %v %v", col, bg, border, fg, el.BgContainer, p.Border, p.Base)
		}
	}
	if _, border, fg := plain(el, ""); fg != el.Primary.Base || border != el.Primary.Border {
		t.Errorf("Element plain default: border %v fg %v", border, fg)
	}
	ant := theme.AntDesign()
	if _, _, fg := plain(ant, "success"); fg != ant.Success.Base {
		t.Errorf("Ant plain success label %v, want %v", fg, ant.Success.Base)
	}
	if _, border, fg := plain(ant, ""); fg != ant.Text || border != ant.Border {
		t.Errorf("Ant plain default: border %v fg %v", border, fg)
	}
	for _, th := range []*theme.Theme{theme.Shadcn(), theme.Shadcn(theme.Dark())} {
		bg, border, fg := plain(th, "")
		if fg != th.Text || border != th.Border {
			t.Errorf("shadcn dark=%v plain default: border %v fg %v", th.Dark, border, fg)
		}
		if r := theme.ContrastRatio(fg, theme.Over(bg, th.BgContainer)); r < 4.5 {
			t.Errorf("shadcn dark=%v plain default contrast %.2f", th.Dark, r)
		}
	}
}

// TestAlignB2FixProgressShowInfoToken: whether a line bar shows its
// percentage by default comes from ProgressToken.ShowInfo, not the flavor.
func TestAlignB2FixProgressShowInfoToken(t *testing.T) {
	shown := func(th *theme.Theme, f func(*ProgressW)) bool {
		var p *ProgressW
		d := NewDriver(func(c *Context) {
			Column(c, func() { p = Progress(c, 40); f(p) }, layout.W(300))
		}, DriverOptions{Theme: th})
		d.Settle()
		return p.infoWidth() > 0
	}
	none := func(*ProgressW) {}
	if !shown(theme.AntDesign(), none) || shown(theme.Shadcn(), none) {
		t.Fatal("theme defaults: Ant shows, shadcn hides the line percentage")
	}
	ant := theme.AntDesign()
	ant.Components.Progress.ShowInfo = false
	if shown(ant, none) || !shown(ant, func(p *ProgressW) { p.ShowInfo() }) {
		t.Fatal("ShowInfo token false: hidden unless ShowInfo()")
	}
	sh := theme.Shadcn()
	sh.Components.Progress.ShowInfo = true
	if !shown(sh, none) || shown(sh, func(p *ProgressW) { p.HideInfo() }) {
		t.Fatal("ShowInfo token true: shown unless HideInfo()")
	}
}

// TestAlignB2FixShiftAnchor: after a Shift range, the anchor is the
// clicked row (antd useSelection.js updatePrevSelectedIndex), not the
// range's far end.
func TestAlignB2FixShiftAnchor(t *testing.T) {
	var sel []int
	d := tblDriver(func(c *Context) {
		Table(c, e2Cols([]int{1, 2, 3, 4, 5, 6}), 6).Selection(&sel).Show()
	})
	shiftClick := func(key string) {
		r := mustFind(t, d, "row:"+key)
		p := geom.Pt(r.X+16, r.Y+r.H/2)
		d.Inject(input.Pointer{Kind: input.Move, Pos: p})
		d.Inject(input.Pointer{Kind: input.Press, Pos: p, Mods: input.ModShift})
		d.Inject(input.Pointer{Kind: input.Release, Pos: p, Mods: input.ModShift})
		d.Settle()
	}
	clickRowCheck(t, d, "5")
	shiftClick("2")
	if fmt.Sprint(sortedInts(sel)) != "[2 3 4 5]" {
		t.Fatalf("range 2..5: %v", sel)
	}
	shiftClick("4") // anchor 2: 2..4 are all checked, so they uncheck
	if fmt.Sprint(sortedInts(sel)) != "[5]" {
		t.Fatalf("range from anchor 2 to 4 should uncheck 2..4: %v", sel)
	}
	// Row 4 was checked before that click: the anchor is cleared, so the
	// next Shift+click checks a single row, not a range.
	shiftClick("0")
	if fmt.Sprint(sortedInts(sel)) != "[0 5]" {
		t.Fatalf("after a range uncheck there is no anchor: %v, want [0 5]", sel)
	}
}

func sortedInts(s []int) []int {
	out := append([]int(nil), s...)
	sort.Ints(out)
	return out
}

// TestTreeDisabledDrag: a Draggable tree that is disabled (Disabled or
// ComponentDisabled) reports no drop; the same tree enabled does.
func TestTreeDisabledDrag(t *testing.T) {
	for _, mode := range []string{"enabled", "disabled", "provider"} {
		dropped := false
		d := NewDriver(func(c *Context) {
			build := func() {
				tw := Tree(c, []TreeNode{{Key: "a", Title: "Aaa"}, {Key: "b", Title: "Bbb"}}).Draggable().
					Disabled(mode == "disabled").Show()
				if _, _, _, ok := tw.Dropped(); ok {
					dropped = true
				}
			}
			if mode == "provider" {
				ConfigProvider(c).ComponentDisabled(true).Body(build)
			} else {
				build()
			}
		}, DriverOptions{Width: 400, Height: 300})
		d.Settle()
		d.Drag(b2fCenter(t, d, "Aaa"), b2fCenter(t, d, "Bbb"))
		d.Settle()
		if dropped != (mode == "enabled") {
			t.Fatalf("%s: dropped %v", mode, dropped)
		}
	}
}

// TestAlignB2FixTagInverse: an Ant "<preset>-inverse" color is the preset
// solid (antd useColor.js), unless a Variant says otherwise.
func TestAlignB2FixTagInverse(t *testing.T) {
	var inv, solid, filled, plainBlue, ctx *TagW
	NewDriver(func(c *Context) {
		inv = Tag(c, "a").Color("blue-inverse")
		solid = Tag(c, "b").Color("blue").Variant("solid")
		filled = Tag(c, "c").Color("blue-inverse").Variant("filled")
		plainBlue = Tag(c, "d").Color("blue").Variant("filled")
		// antd useColor: the variant prop beats inverse, inverse beats the
		// context (Defaults) variant.
		ConfigProvider(c).Defaults("Tag", Props{"variant": "outlined"}).Body(func() {
			ctx = Tag(c, "e").Color("blue-inverse")
		})
	}, DriverOptions{Theme: theme.AntDesign()}).Frame()
	if v := ctx.effVariant(); v != "solid" {
		t.Errorf("blue-inverse under Defaults variant outlined: %q, want solid", v)
	}
	ib, _, ifg := inv.colors()
	sb, _, sfg := solid.colors()
	if ib != sb || ifg != sfg {
		t.Errorf("blue-inverse %v/%v, want the blue solid %v/%v", ib, ifg, sb, sfg)
	}
	fb, _, _ := filled.colors()
	pb, _, _ := plainBlue.colors()
	if fb != pb {
		t.Errorf("blue-inverse filled bg %v, want blue filled %v", fb, pb)
	}
}

// TestTabsDragBeforeLeave: activating the dragged tab goes through
// BeforeLeave like every other switch; a veto keeps the active tab (the
// reorder still happens).
func TestTabsDragBeforeLeave(t *testing.T) {
	items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
	active := "a"
	var asked [2]string
	d := NewDriver(func(c *Context) {
		tb := Tabs(c, &active, items).Draggable().BeforeLeave(func(n, o string) bool { asked = [2]string{n, o}; return false })
		if it := tb.Show().Reordered(); it != nil {
			items = it
		}
	}, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	ra, _ := d.Find("Alpha")
	rb, _ := d.Find("Beta")
	dragTo(d, rb.Center(), geom.Pt(ra.X+4, ra.Center().Y), true)
	if got := tabKeys(items); fmt.Sprint(got) != "[b a c]" {
		t.Fatalf("reorder: %v", got)
	}
	if active != "a" || asked != [2]string{"b", "a"} {
		t.Fatalf("active %q asked %v: BeforeLeave bypassed", active, asked)
	}
}

// TestCarouselOutOfRangeNoAnim: an out-of-range Current is clamped before
// the first frame, which then shows the clamped slide at once (no slide
// animation from slide 0).
func TestCarouselOutOfRangeNoAnim(t *testing.T) {
	pos := func(start int) geom.Rect {
		cur := start
		d := NewDriver(func(c *Context) {
			Carousel(c, 3, func(i int) { Text(c, fmt.Sprintf("s%d", i)) }).Current(&cur).Show()
		}, DriverOptions{Width: 400, Height: 300})
		d.Frame()
		r, _ := d.FindText("s2")
		return r
	}
	if got, want := pos(99), pos(2); got != want {
		t.Fatalf("first frame with Current 99: slide 2 at %v, want %v", got, want)
	}
}

// TestCalendarSelectToday: Element ControllerType("select") has the small
// "Today" button after the year / month selects (select-controller.vue),
// which selects today.
func TestCalendarSelectToday(t *testing.T) {
	val := time.Date(2020, 3, 5, 0, 0, 0, 0, time.UTC)
	d := NewDriver(func(c *Context) {
		Calendar(c, &val).ControllerType("select")
	}, DriverOptions{Width: 900, Height: 900, Theme: theme.ElementPlus()})
	d.Frame()
	if err := d.ClickLabel(ZhCN.Today); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	now := d.Now()
	if val.Year() != now.Year() || val.YearDay() != now.YearDay() {
		t.Fatalf("Today picked %v, want %v", val, now)
	}
}

// TestListTopPagerLastRule: with pagination anywhere (antd list/index.js:91
// isSomethingAfterLastItem), the last item keeps its rule, the same as with
// the pager below.
func TestListTopPagerLastRule(t *testing.T) {
	rules := func(pos string) int {
		page := 1
		d := NewDriver(func(c *Context) {
			List(c, 6).Pagination(&page, 3).PaginationPosition(pos).Item(func(i int) { Text(c, fmt.Sprintf("item %d", i)) })
		}, DriverOptions{Width: 500, Height: 500, Theme: theme.AntDesign()})
		d.Frame()
		col := theme.AntDesign().Components.List.SplitColor
		n := 0
		for _, o := range d.r.List().Ops {
			if f, ok := o.(paint.FillRect); ok && f.Color == col && f.Rect.H == 1 && f.Rect.W > 100 {
				n++
			}
		}
		return n
	}
	if top, bottom := rules("top"), rules("bottom"); top != 3 || bottom != 3 {
		t.Fatalf("item rules: top pager %d, bottom pager %d, want 3 each", top, bottom)
	}
}

// TestTreeDefaultExpandAllNoChange: defaultExpandAll is the initial state;
// rc-tree fires no onExpand, so ExpandChanged stays false.
func TestTreeDefaultExpandAllNoChange(t *testing.T) {
	var exp []string
	changed := false
	d := NewDriver(func(c *Context) {
		if Tree(c, b2fTree()).Expanded(&exp).DefaultExpandAll().Show().ExpandChanged() {
			changed = true
		}
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if len(exp) == 0 || changed {
		t.Fatalf("expanded %v changed %v", exp, changed)
	}
}

// TestTableTitleFooterBorders: in a bordered table the Title bar has no
// bottom border and the Footer no top border (antd bordered.js
// borderBottom / borderTop 0): each strokes a box one pixel taller with that
// edge clipped off, and nothing paints over the table frame's edge.
func TestTableTitleFooterBorders(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := NewDriver(func(c *Context) {
			Table(c, e2Cols([]int{1, 2}), 2).Bordered().
				Title(func() { Text(c, "TT") }).Footer(func() { Text(c, "FF") }).Show()
		}, DriverOptions{Width: 600, Height: 400, Theme: th})
		d.Settle()
		tt, ff := mustFindText(t, d, "TT"), mustFindText(t, d, "FF")
		ops := d.r.List().Ops
		var title, footer bool
		for i := 1; i < len(ops); i++ {
			cl, ok1 := ops[i-1].(paint.PushClip)
			sr, ok2 := ops[i].(paint.StrokeRect)
			if !ok1 || !ok2 || sr.Rect.H != cl.Rect.H+1 {
				continue
			}
			switch {
			case cl.Rect.Contains(tt.Center()):
				title = sr.Rect.Y == cl.Rect.Y // bottom edge clipped
			case cl.Rect.Contains(ff.Center()):
				footer = sr.Rect.Y == cl.Rect.Y-1 // top edge clipped
			}
		}
		if !title || !footer {
			t.Errorf("%v: title three-sided %v, footer three-sided %v", th.Flavor, title, footer)
		}
		bg := th.Components.Table.FooterAreaBg
		for _, o := range ops {
			if f, ok := o.(paint.FillRect); ok && f.Rect.H == 1 && bg.A > 0 && f.Color == bg {
				t.Errorf("%v: 1px FooterAreaBg strip %v over the frame edge", th.Flavor, f.Rect)
			}
		}
	}
}

// TestTableFooterRadius: the Ant footer's bottom corners take the table
// radius, bordered or not (antd radius.js &-footer 0 0 tableRadius
// tableRadius).
func TestTableFooterRadius(t *testing.T) {
	th := theme.AntDesign()
	tk := th.Components.Table
	for _, bordered := range []bool{false, true} {
		d := NewDriver(func(c *Context) {
			tb := Table(c, e2Cols([]int{1}), 1).Footer(func() { Text(c, "FF") })
			if bordered {
				tb.Bordered()
			}
			tb.Show()
		}, DriverOptions{Width: 600, Height: 400, Theme: th})
		d.Settle()
		var got *paint.FillRect
		for _, o := range d.r.List().Ops {
			if f, ok := o.(paint.FillRect); ok && f.Color == tk.FooterAreaBg && f.Rect.H > 10 {
				got = &f
			}
		}
		if got == nil || got.Radii.BL != tk.HeaderBorderRadius || got.Radii.BR != tk.HeaderBorderRadius || got.Radii.TL != 0 {
			t.Fatalf("bordered=%v: footer fill %+v, want bottom radius %v", bordered, got, tk.HeaderBorderRadius)
		}
	}
}

// TestTableSorterTooltipToken: the header sort tooltip follows
// TableToken.SorterTooltip (Element false), not the flavor.
func TestTableSorterTooltipToken(t *testing.T) {
	for _, on := range []bool{true, false} {
		th := theme.AntDesign()
		th.Components.Table.SorterTooltip = on
		cols := []TableColumn{{Key: "a", Title: "A", Width: 120, Sortable: true, Text: func(int) string { return "x" }}}
		d := NewDriver(func(c *Context) { Table(c, cols, 1).Show() }, DriverOptions{Width: 600, Height: 400, Theme: th})
		d.Settle()
		d.Move(g6Rect(t, d, "A").Center())
		for i := 0; i < 4; i++ {
			d.Settle()
		}
		if _, got := d.FindText(d.r.c.Locale().SortAsc); got != on {
			t.Fatalf("token %v: tooltip shown %v", on, got)
		}
	}
}
