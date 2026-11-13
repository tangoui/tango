package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// pro2Themes are the design languages every pro2 component is checked in.
func pro2Themes() map[string]func() *theme.Theme {
	return map[string]func() *theme.Theme{
		"ant":     func() *theme.Theme { return theme.AntDesign() },
		"element": func() *theme.Theme { return theme.ElementPlus() },
	}
}

func pro2Driver(th *theme.Theme, w, h float32, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Grow(1), layout.Pad(10)) },
		DriverOptions{Width: w, Height: h, Theme: th})
	d.Settle()
	return d
}

// pro2Leaf returns the rect of the leaf (text) node labelled label; card
// containers carry their title as label too, so FindText may return them.
func pro2Leaf(d *Driver, label string) (geom.Rect, bool) {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if found != nil {
			return
		}
		if n.label == label && len(n.kids) == 0 {
			found = n
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	for i := len(d.r.c.overlays) - 1; i >= 0 && found == nil; i-- {
		walk(d.r.c.overlays[i].root)
	}
	if found == nil {
		walk(d.r.c.root)
	}
	if found == nil {
		return geom.Rect{}, false
	}
	return found.abs, !found.abs.Empty()
}

func pro2MustFind(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	r, ok := pro2Leaf(d, label)
	if !ok {
		t.Fatalf("%q not rendered", label)
	}
	return r
}

func pro2ClickText(t *testing.T, d *Driver, label string) {
	t.Helper()
	r := pro2MustFind(t, d, label)
	d.Click(r.Center())
	d.Settle()
}

// ---------------------------------------------------------------------------
// PageContainer / FooterToolbar

func TestPro2PageContainer(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			tab, submits, backs, crumb := "a", 0, 0, -1
			var pc *PageContainerW
			d := pro2Driver(mk(), 900, 600, func(c *Context) {
				pc = PageContainer(c, "PageTitle").SubTitle("SubTitle").Breadcrumb("Home", "List", "PageTitle").Back().
					Extra(func() { Button(c, "ExtraAction") }).
					Content(func() { Text(c, "HeaderContent") }).
					ExtraContent(func() { Text(c, "RightContent") }).
					Tabs(&tab, []TabItem{{Key: "a", Label: "TabA"}, {Key: "b", Label: "TabB"}}).
					FooterExtra(func() { Text(c, "FooterInfo") }).
					Footer(func() {
						if Button(c, "Submit").Primary().Clicked() {
							submits++
						}
					}).
					Fill().
					Body(func() { Text(c, "Body-"+tab) })
				if pc.BackClicked() {
					backs++
				}
				if i := pc.CrumbClicked(); i >= 0 {
					crumb = i
				}
			})
			for _, s := range []string{"PageTitle", "SubTitle", "ExtraAction", "HeaderContent", "RightContent", "TabA", "Body-a", "FooterInfo", "Submit"} {
				pro2MustFind(t, d, s)
			}
			// Extra content sits right of the content.
			if pro2MustFind(t, d, "RightContent").X <= pro2MustFind(t, d, "HeaderContent").X {
				t.Fatal("extra content not at the right")
			}
			// Tabs switch the body.
			pro2ClickText(t, d, "TabB")
			if tab != "b" {
				t.Fatalf("tab = %q", tab)
			}
			pro2MustFind(t, d, "Body-b")
			// Footer toolbar: at the bottom of the filled container, full width.
			sub := pro2MustFind(t, d, "Submit")
			if sub.Bottom() < 600-80 {
				t.Fatalf("footer not at the bottom: %v", sub)
			}
			if info := pro2MustFind(t, d, "FooterInfo"); info.X > sub.X {
				t.Fatal("footer extra not left of the actions")
			}
			pro2ClickText(t, d, "Submit")
			if submits != 1 {
				t.Fatalf("submits = %d", submits)
			}
			// Breadcrumb.
			pro2ClickText(t, d, "Home")
			if crumb != 0 {
				t.Fatalf("crumb = %d", crumb)
			}
		})
	}
}

func TestPro2PageContainerBackAndLoading(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			backs := 0
			loading := true
			d := pro2Driver(mk(), 800, 500, func(c *Context) {
				pc := PageContainer(c, "Detail").Back().Loading(loading).Body(func() { Text(c, "Loaded") })
				if pc.BackClicked() {
					backs++
				}
			})
			if _, ok := pro2Leaf(d, "Loaded"); ok {
				t.Fatal("body shown while loading")
			}
			if err := d.ClickLabel(d.r.c.Locale().Back); err != nil {
				t.Fatal(err)
			}
			d.Settle()
			if backs == 0 {
				t.Fatal("back not clicked")
			}
			loading = false
			d.Settle()
			pro2MustFind(t, d, "Loaded")
		})
	}
}

func TestPro2FooterToolbar(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			clicked := false
			var ft *FooterToolbarW
			d := pro2Driver(mk(), 700, 400, func(c *Context) {
				Row(c, func() {
					Column(c, nil, layout.W(150)) // a sider
					Column(c, func() {
						Scroll(c, func() { Text(c, "Content") }, layout.Grow(1))
						ft = FooterToolbar(c).Extra(func() { Text(c, "LeftInfo") })
						ft.Body(func() {
							if Button(c, "Done").Primary().Clicked() {
								clicked = true
							}
						})
					}, layout.Grow(1))
				}, layout.Grow(1))
			})
			bar, ok := d.r.c.RectOf(ft.ID())
			if !ok {
				t.Fatal("no bar")
			}
			// Width follows the content area (window 700 - pad 20 - sider 150).
			if bar.X < 150 || bar.W < 500 || bar.Bottom() < 380 {
				t.Fatalf("bar rect %v", bar)
			}
			if pro2MustFind(t, d, "Done").Right() < bar.Right()-40 {
				t.Fatal("actions not right-aligned")
			}
			pro2ClickText(t, d, "Done")
			if !clicked {
				t.Fatal("action not clicked")
			}
		})
	}
}

func TestPro2FooterToolbarSticky(t *testing.T) {
	d := pro2Driver(theme.AntDesign(), 600, 300, func(c *Context) {
		Scroll(c, func() {
			Column(c, nil, layout.H(1000))
			FooterToolbar(c).Sticky().Body(func() { Button(c, "Pinned") })
		}, layout.Grow(1))
	})
	d.Settle()
	r := pro2MustFind(t, d, "Pinned")
	if r.Y > 300 || r.Bottom() < 240 {
		t.Fatalf("sticky bar not pinned to the viewport bottom: %v", r)
	}
}

// ---------------------------------------------------------------------------
// ProCard

func TestPro2ProCardCollapseAndExtra(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			extra := 0
			var folded bool
			toggles := 0
			d := pro2Driver(mk(), 700, 500, func(c *Context) {
				w := ProCard(c, "Folding").SubTitle("Sub").Tooltip("Tip").Bordered().Collapsed(&folded).
					Extra(func() {
						if Button(c, "Act").Clicked() {
							extra++
						}
					}).
					Body(func() { Text(c, "Inner body") })
				if w.Toggled() {
					toggles++
				}
				ProCard(c, "Uncontrolled").DefaultCollapsed().Body(func() { Text(c, "Hidden body") })
			})
			pro2MustFind(t, d, "Inner body")
			if _, ok := pro2Leaf(d, "Hidden body"); ok {
				t.Fatal("DefaultCollapsed card shows its body")
			}
			// Clicking an action in the header does not fold.
			pro2ClickText(t, d, "Act")
			if extra != 1 || folded {
				t.Fatalf("extra=%d folded=%v", extra, folded)
			}
			pro2ClickText(t, d, "Folding")
			if !folded || toggles != 1 {
				t.Fatalf("not folded: %v %d", folded, toggles)
			}
			if _, ok := pro2Leaf(d, "Inner body"); ok {
				t.Fatal("folded card shows its body")
			}
			// Keyboard: the header is focusable; Enter toggles.
			pro2ClickText(t, d, "Uncontrolled")
			pro2MustFind(t, d, "Hidden body")
			d.Key(input.KeyEnter, 0)
			d.Settle()
			if _, ok := pro2Leaf(d, "Hidden body"); ok {
				t.Fatal("Enter did not fold")
			}
		})
	}
}

func TestPro2ProCardSplitAndSpan(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			var left, right, top, bottom, a, b *ProCardW
			d := pro2Driver(mk(), 900, 600, func(c *Context) {
				ProCard(c, "Split").Split(SplitVertical).Bordered().Body(func() {
					left = ProCard(c, "Left").ColSpan(8).Body(func() { Text(c, "L") })
					right = ProCard(c, "Right").Split(SplitHorizontal).Body(func() {
						top = ProCard(c, "Top").Body(func() { Text(c, "T") })
						bottom = ProCard(c, "Bottom").Body(func() { Text(c, "B") })
					})
				})
				ProCard(c, "").Ghost().Gutter(10).Body(func() {
					a = ProCard(c, "A").ColWidth(200).Body(nil)
					b = ProCard(c, "B2").Body(nil)
				})
			})
			rect := func(w *ProCardW) geom.Rect {
				r, ok := d.r.c.RectOf(w.ID())
				if !ok {
					t.Fatalf("card %q missing", w.title)
				}
				return r
			}
			l, r := rect(left), rect(right)
			if r.X < l.Right()-1 || l.Y != r.Y {
				t.Fatalf("vertical split not side by side: %v %v", l, r)
			}
			if ratio := l.W / (l.W + r.W); ratio < 0.3 || ratio > 0.37 {
				t.Fatalf("colSpan 8 ratio %.2f", ratio)
			}
			tp, bt := rect(top), rect(bottom)
			if bt.Y < tp.Bottom()-1 || tp.X != bt.X {
				t.Fatalf("horizontal split not stacked: %v %v", tp, bt)
			}
			ra, rb := rect(a), rect(b)
			if ra.W != 200 || rb.X-ra.Right() != 10 {
				t.Fatalf("ColWidth/Gutter: %v %v", ra, rb)
			}
		})
	}
}

func TestPro2ProCardTabsAndLoading(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			tab := "x"
			loading := true
			changed := 0
			d := pro2Driver(mk(), 700, 500, func(c *Context) {
				w := ProCard(c, "Tabbed").Tabs(&tab, []ProCardTab{
					{Key: "x", Label: "TabX", Body: func() { Text(c, "PanelX") }},
					{Key: "y", Label: "TabY", Body: func() { Text(c, "PanelY") }},
				}).TabsCard().Body(nil)
				if w.TabChanged() {
					changed++
				}
				ProCard(c, "Busy").Loading(loading).Body(func() { Text(c, "Ready") })
			})
			pro2MustFind(t, d, "PanelX")
			pro2ClickText(t, d, "TabY")
			if tab != "y" || changed != 1 {
				t.Fatalf("tab=%q changed=%d", tab, changed)
			}
			pro2MustFind(t, d, "PanelY")
			if _, ok := pro2Leaf(d, "Ready"); ok {
				t.Fatal("loading card shows its body")
			}
			loading = false
			d.Settle()
			pro2MustFind(t, d, "Ready")
		})
	}
}

// ---------------------------------------------------------------------------
// StatisticCard

func TestPro2StatisticCard(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			chartBuilt := 0
			d := pro2Driver(mk(), 900, 500, func(c *Context) {
				StatisticCardGroup(c).Body(func() {
					StatisticCard(c, "Sales").Value(126560, 0).Prefix("$").Tooltip("tip").Status("success").Trend("up").
						Desc("Week", "12%", "up").Desc("Day", "11%", "down").
						Chart(func() {
							chartBuilt++
							Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 100, H: 40} }, nil).Semantics(a11y.RoleFigure, "MiniChart")
							Text(c, "ChartSlot")
						}).
						Footer(func() { Text(c, "FooterRow") }).Show()
					StatisticCard(c, "Visits").ValueText("8,846").ChartPlace(ChartRight).
						Chart(func() { Text(c, "RightChart") }).Show()
				})
			})
			for _, s := range []string{"Sales", "126,560", "$", "Week", "12%", "Day", "11%", "ChartSlot", "FooterRow", "Visits", "8,846"} {
				pro2MustFind(t, d, s)
			}
			if chartBuilt == 0 {
				t.Fatal("chart slot not built")
			}
			// Trend marks are built (one main + two descriptions).
			marks := 0
			var walk func(n *node)
			walk = func(n *node) {
				if n.label == "up" || n.label == "down" {
					marks++
				}
				for _, k := range n.kids {
					walk(k)
				}
			}
			walk(d.r.c.root)
			if marks != 3 {
				t.Fatalf("trend marks = %d", marks)
			}
			// Chart placed right of the value.
			if pro2MustFind(t, d, "RightChart").X <= pro2MustFind(t, d, "8,846").Right() {
				t.Fatal("right chart not right of the statistic")
			}
			// Group: second card right of the first.
			if pro2MustFind(t, d, "Visits").X <= pro2MustFind(t, d, "Sales").X {
				t.Fatal("group not horizontal")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CheckCard

func TestPro2CheckCard(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			on, off := false, true
			changes := 0
			var cc *CheckCardW
			d := pro2Driver(mk(), 900, 400, func(c *Context) {
				Row(c, func() {
					cc = CheckCard(c, "Plain", &on).Description("desc").Avatar("P").Show()
					if cc.Changed() {
						changes++
					}
					CheckCard(c, "Locked", &off).Disabled(true).Show()
					CheckCard(c, "SmallOne", new(bool)).Small().Show()
					CheckCard(c, "LargeOne", new(bool)).Large().Show()
				}, layout.Gap(8), layout.WrapItems())
			})
			if err := d.ClickLabel("Plain"); err != nil {
				t.Fatal(err)
			}
			d.Settle()
			if !on || changes != 1 {
				t.Fatalf("on=%v changes=%d", on, changes)
			}
			// Keyboard: focused card toggles with Space.
			d.Key(input.KeySpace, 0)
			d.Settle()
			if on {
				t.Fatal("Space did not toggle")
			}
			// Disabled does not toggle.
			pro2ClickText(t, d, "Locked")
			if !off {
				t.Fatal("disabled card toggled")
			}
			// Sizes.
			small, _ := d.Find("SmallOne")
			large, _ := d.Find("LargeOne")
			plain, _ := d.Find("Plain")
			if small.W != 212 || plain.W != 320 || large.W != 440 {
				t.Fatalf("widths %v %v %v", small.W, plain.W, large.W)
			}
		})
	}
}

func TestPro2CheckCardGroup(t *testing.T) {
	opts := []CheckCardOption{{Value: "a", Title: "Alpha"}, {Value: "b", Title: "Beta"}, {Value: "c", Title: "Gamma", Disabled: true}}
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			single := "a"
			var multi []string
			changed := 0
			d := pro2Driver(mk(), 1100, 500, func(c *Context) {
				if CheckCardGroup(c, opts).Value(&single).Show().Changed() {
					changed++
				}
				Key(c, "multi", func() {
					CheckCardGroup(c, []CheckCardOption{{Value: "x", Title: "Xray"}, {Value: "y", Title: "Yank"}}).Values(&multi).Show()
				})
			})
			if err := d.ClickLabel("Beta"); err != nil {
				t.Fatal(err)
			}
			d.Settle()
			if single != "b" || changed != 1 {
				t.Fatalf("single=%q changed=%d", single, changed)
			}
			// Clicking the checked card again clears the choice.
			d.ClickLabel("Beta")
			d.Settle()
			if single != "" {
				t.Fatalf("single=%q, want cleared", single)
			}
			d.ClickLabel("Gamma")
			d.Settle()
			if single != "" {
				t.Fatal("disabled option selected")
			}
			// Multiple: values follow option order.
			d.ClickLabel("Yank")
			d.Settle()
			d.ClickLabel("Xray")
			d.Settle()
			if len(multi) != 2 || multi[0] != "x" || multi[1] != "y" {
				t.Fatalf("multi = %v", multi)
			}
			d.ClickLabel("Xray")
			d.Settle()
			if len(multi) != 1 || multi[0] != "y" {
				t.Fatalf("multi = %v", multi)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ProSkeleton

func TestPro2ProSkeleton(t *testing.T) {
	for name, mk := range pro2Themes() {
		t.Run(name, func(t *testing.T) {
			for _, kind := range []string{SkeletonList, SkeletonResult, SkeletonDescriptions} {
				var b *Box
				d := pro2Driver(mk(), 900, 900, func(c *Context) {
					b = ProSkeleton(c, kind).Active(false).List(3).Show()
				})
				r := b.Rect()
				if r.H < 200 || r.W < 800 {
					t.Fatalf("%s: skeleton rect %v", kind, r)
				}
				blocks := 0
				var walk func(n *node)
				walk = func(n *node) {
					if n.label == "" && n.ln.Measure != nil && n.paint != nil && len(n.kids) == 0 {
						blocks++
					}
					for _, k := range n.kids {
						walk(k)
					}
				}
				walk(b.n)
				if blocks < 6 {
					t.Fatalf("%s: only %d blocks", kind, blocks)
				}
				_ = d
			}
			// Toggles drop parts.
			var empty *Box
			pro2Driver(mk(), 900, 900, func(c *Context) {
				empty = ProSkeleton(c, "").PageHeader(false).Statistic(0).List(0).Show()
			})
			if h := empty.Rect().H; h > 1 {
				t.Fatalf("empty skeleton height %v", h)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Accessibility

func TestPro2Semantics(t *testing.T) {
	on := true
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ProCard(c, "Region").Collapsible().Body(func() { Text(c, "x") })
			CheckCard(c, "Choice", &on).Show()
			CheckCardGroup(c, []CheckCardOption{{Value: "r", Title: "RadioCard"}}).Value(new(string)).Show()
			FooterToolbar(c).Body(func() { Button(c, "Ok") })
			ProSkeleton(c, SkeletonResult).Show()
			StatisticCard(c, "Stat").Value(1, 0).Trend("down").Show()
		})
	}, DriverOptions{Width: 900, Height: 1400, Accessibility: true})
	d.Settle()
	if n, ok := d.A11yFind(a11y.RoleCheckbox, "Choice"); !ok || n.States&a11y.Checked == 0 {
		t.Fatalf("CheckCard checkbox: %v %v", n, ok)
	}
	if _, ok := d.A11yFind(a11y.RoleRadio, "RadioCard"); !ok {
		t.Fatal("group card not a radio")
	}
	if _, ok := d.A11yFind(a11y.RoleRadioGroup, ""); !ok {
		t.Fatal("no radio group")
	}
	if n, ok := d.A11yFind(a11y.RoleButton, "Region"); !ok || n.States&a11y.Expanded == 0 {
		t.Fatalf("collapsible header: %v %v", n, ok)
	}
	if _, ok := d.A11yFind(a11y.RoleToolbar, ZhCN.PageContainer.FooterToolbar); !ok {
		t.Fatal("no footer toolbar")
	}
	if n, ok := d.A11yFind(a11y.RoleStatus, ""); !ok || n.States&a11y.Busy == 0 {
		t.Fatal("skeleton not a busy status")
	}
	if _, ok := d.A11yFind(a11y.RoleImage, ZhCN.StatisticCard.Down); !ok {
		t.Fatal("trend mark not named")
	}
}
