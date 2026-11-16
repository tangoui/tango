package ui

// Spec alignment batch 1, group C (selectors): Select, VirtualSelect,
// MultiSelect, AutoComplete, Mentions, InputTag, Cascader, TreeSelect and
// Transfer against antd 6.7 / Element Plus 2.14.8 / shadcn v4
// (docs/specs/align/1-select.md).

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// TestSelectUnknownValue: a value that no option has is shown as is
// (rc-select Select.js: mergedLabel = label ?? val), for Select, its
// engine path, MultiSelect tags and VirtualSelect.
func TestSelectUnknownValue(t *testing.T) {
	val, rich, vs := "ghost", "ghost", "ghost"
	vals := []string{"a", "ghost"}
	var w, wr *SelectW
	var ms *MultiSelectW
	var vw *VirtualSelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a", "b")).Width(200)
		wr = Select(c, &rich, Opts("a", "b")).Virtual(true).Width(200)
		ms = MultiSelect(c, &vals, Opts("a", "b")).Width(300)
		vw = VirtualSelect(c, &vs, Opts("a", "b")).Width(200)
	}, DriverOptions{Width: 900, Height: 600})
	d.EnableAccessibility()
	d.Settle()
	if got := w.label(); got != "ghost" {
		t.Fatalf("Select label = %q, want the raw value", got)
	}
	if got := wr.label(); got != "ghost" {
		t.Fatalf("Select (engine) label = %q", got)
	}
	tagged := false
	for _, k := range ms.box.n.kids {
		if k != ms.box.ed && k.label == "ghost" {
			tagged = true
		}
	}
	if !tagged {
		t.Fatal("MultiSelect tag for the unknown value missing")
	}
	if vw.box.single != "ghost" {
		t.Fatalf("VirtualSelect shows %q", vw.box.single)
	}
	// Picking a real option replaces it without panicking.
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Key(input.KeyEnter, 0)
	if val != "a" {
		t.Fatalf("val = %q after Enter", val)
	}
}

// TestSelectA11yUnknownValue: the combobox's value is the raw value.
func TestSelectA11yUnknownValue(t *testing.T) {
	val := "ghost"
	d := NewDriver(func(c *Context) {
		Select(c, &val, Opts("a", "b")).A11yLabel("City").Width(200)
	}, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	n, ok := d.A11yFind(a11y.RoleCombobox, "City")
	if !ok || n.Value != "ghost" {
		t.Fatalf("combobox %+v", n)
	}
}

// TestSelectLabelRender: LabelRender formats the selected value, also for
// a value outside the options.
func TestSelectLabelRender(t *testing.T) {
	val := "b"
	var w *SelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, []Option{{Label: "Beta", Value: "b"}}).Width(200).
			LabelRender(func(o Option) string { return o.Value + ":" + o.Label })
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if got := w.label(); got != "b:Beta" {
		t.Fatalf("label = %q", got)
	}
	val = "zz"
	d.Settle()
	if got := w.label(); got != "zz:zz" {
		t.Fatalf("unknown label = %q", got)
	}
}

// TestSelectPlacement: Placement(TopStart) and the "placement" default
// (Ant names) open the dropdown above the trigger; topRight aligns the
// inline end edges.
func TestSelectPlacement(t *testing.T) {
	val := ""
	var vals []string
	var w *SelectW
	var ms *MultiSelectW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Select", Props{"placement": "topRight"}).Body(func() {
			Column(c, func() {
				Gap(c, 300)
				Row(c, func() { // away from the window edge (overlays keep 4px off it)
					Gap(c, 100)
					w = Select(c, &val, Opts("a", "b", "c")).Width(200).Open()
				})
				ms = MultiSelect(c, &vals, Opts("a", "b")).Width(260).Placement(TopStart).Open()
			})
		})
	}, DriverOptions{Width: 600, Height: 700})
	d.Settle()
	if w.place != TopEnd {
		t.Fatalf("default placement = %v", w.place)
	}
	tr, pr := d.r.c.rects[w.n.id], d.r.c.rects[w.st.panel]
	if pr.H == 0 || pr.Bottom() > tr.Y+0.5 {
		t.Fatalf("Select panel %v not above trigger %v", pr, tr)
	}
	if d := pr.Right() - tr.Right(); d > 0.5 || d < -0.5 {
		t.Fatalf("topRight: panel right %v, trigger right %v", pr.Right(), tr.Right())
	}
	mr, mp := d.r.c.rects[ms.box.n.id], d.r.c.rects[ms.st.pop.panel]
	if mp.H == 0 || mp.Bottom() > mr.Y+0.5 {
		t.Fatalf("MultiSelect panel %v not above trigger %v", mp, mr)
	}
}

// TestSelectDefaultsVariant: ConfigProvider.Defaults "variant" reaches
// every select (string or FieldVariant).
func TestSelectDefaultsVariant(t *testing.T) {
	val, ts := "", ""
	var vals, cas []string
	var w *SelectW
	var ms *MultiSelectW
	var vw *VirtualSelectW
	var cw *CascaderW
	var tw *TreeSelectW
	var ac *AutoCompleteW
	var mt *MentionsW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Select", Props{"variant": "filled"}).
			Defaults("Cascader", Props{"variant": FieldUnderlined}).
			Defaults("TreeSelect", Props{"variant": "borderless"}).
			Defaults("AutoComplete", Props{"variant": "filled"}).
			Defaults("Mentions", Props{"variant": "underlined"}).Body(func() {
			w = Select(c, &val, Opts("a"))
			ms = MultiSelect(c, &vals, Opts("a"))
			vw = VirtualSelect(c, &val, Opts("a"))
			cw = Cascader(c, &cas, casTestOpts)
			tw = TreeSelect(c, &ts, tsTestTree)
			ac = AutoComplete(c, &val, Opts("a"))
			mt = Mentions(c, &val, Opts("a"))
		})
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	if w.variant != FieldFilled || ms.box.variant != FieldFilled || vw.box.variant != FieldFilled {
		t.Fatalf("Select family variants %q %q %q", w.variant, ms.box.variant, vw.box.variant)
	}
	if cw.box.variant != FieldUnderlined || tw.box.variant != FieldBorderless {
		t.Fatalf("Cascader %q TreeSelect %q", cw.box.variant, tw.box.variant)
	}
	if ac.in.variant != "filled" || mt.variant != FieldUnderlined {
		t.Fatalf("AutoComplete %q Mentions %q", ac.in.variant, mt.variant)
	}
}

// TestSelectNotFoundContent: the empty dropdown shows the custom text in
// Select, MultiSelect, Cascader search and TreeSelect.
func TestSelectNotFoundContent(t *testing.T) {
	val, ts := "", ""
	var vals, cas []string
	var w *SelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a", "b")).Searchable().NotFoundContent("nothing-s").Width(200)
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Type("zzz")
	mustFindText(t, d, "nothing-s")

	var ms *MultiSelectW
	d = NewDriver(func(c *Context) {
		ms = MultiSelect(c, &vals, nil).NotFoundContent("nothing-m").Width(200).Open()
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	mustFindText(t, d, "nothing-m")
	_ = ms

	var cw *CascaderW
	d = NewDriver(func(c *Context) {
		cw = Cascader(c, &cas, casTestOpts).Searchable().NotFoundContent("nothing-c").Width(200)
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	d.Click(d.r.c.rects[cw.box.n.id].Center())
	d.Type("qqqq")
	mustFindText(t, d, "nothing-c")

	d = NewDriver(func(c *Context) {
		TreeSelect(c, &ts, nil).NotFoundContent("nothing-t").Width(200).Open()
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	mustFindText(t, d, "nothing-t")
}

// TestSelectOpenChanged: OpenChanged fires once per open and close.
func TestSelectOpenChanged(t *testing.T) {
	val := ""
	var vals []string
	n, mn := 0, 0
	var w *SelectW
	var ms *MultiSelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a", "b")).Width(200)
		if w.OpenChanged() {
			n++
		}
		ms = MultiSelect(c, &vals, Opts("a", "b")).Width(200)
		if ms.OpenChanged() {
			mn++
		}
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	if !w.IsOpen() || n != 1 {
		t.Fatalf("open: IsOpen %v changes %d", w.IsOpen(), n)
	}
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() || n != 2 {
		t.Fatalf("close: IsOpen %v changes %d", w.IsOpen(), n)
	}
	d.Click(d.r.c.rects[ms.box.n.id].Center())
	if !ms.IsOpen() || mn != 1 {
		t.Fatalf("MultiSelect open: %v changes %d", ms.IsOpen(), mn)
	}
}

// TestSelectDisabledOpenChanged: a disabled select neither opens nor
// reports OpenChanged.
func TestSelectDisabledOpenChanged(t *testing.T) {
	val := ""
	n := 0
	var w *SelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a", "b")).Width(200).Placement(TopStart).Disabled(true)
		if w.OpenChanged() {
			n++
		}
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Key(input.KeyDown, 0)
	if w.IsOpen() || n != 0 {
		t.Fatalf("disabled select opened (%v) or changed %d", w.IsOpen(), n)
	}
}

// hasFillColor reports a FillPath op of color col in l.
func hasFillColor(l *paint.List, col theme.Color) bool {
	for _, op := range l.Ops {
		if f, ok := op.(paint.FillPath); ok && f.Color == col {
			return true
		}
	}
	return false
}

// TestSelectLoadingSuffix: Loading puts the spinner (Ant LoadingOutlined,
// useIcons.js) in place of the arrow.
func TestSelectLoadingSuffix(t *testing.T) {
	val := ""
	var vals []string
	var w, plain *SelectW
	var ms *MultiSelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a")).Loading(true).Width(200)
		plain = Select(c, &val, Opts("a")).Width(200)
		ms = MultiSelect(c, &vals, Opts("a")).Loading(true).Width(200)
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	col := w.th.Components.Select.LoadingIconColor
	var l, lp, lm paint.List
	w.paint(&l, d.r.c.rects[w.n.id])
	plain.paint(&lp, d.r.c.rects[plain.n.id])
	ms.box.paint(&lm, d.r.c.rects[ms.box.n.id])
	if !hasFillColor(&l, col) || hasFillColor(&lp, col) || !hasFillColor(&lm, col) {
		t.Fatalf("spinner: loading %v plain %v multi %v", hasFillColor(&l, col), hasFillColor(&lp, col), hasFillColor(&lm, col))
	}
}

// TestMultiSelectMaxTagPlaceholder: the collapsed tag reads "+ N ..." by
// default (rc-select MultipleContent.js) and MaxTagPlaceholder's text
// otherwise; MaxTagTextLength cuts labels to n runes + "...".
func TestMultiSelectMaxTagPlaceholder(t *testing.T) {
	vals := []string{"a", "b", "c", "d"}
	var omitted []string
	d := NewDriver(func(c *Context) {
		MultiSelect(c, &vals, Opts("a", "b", "c", "d")).MaxTagCount(2).Width(300)
		MultiSelect(c, &vals, Opts("a", "b", "c", "d")).MaxTagCount(1).Width(300).
			MaxTagPlaceholder(func(o []string) string { omitted = o; return "and " + itoaEnt(len(o)) + " more" })
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	mustFindText(t, d, "+ 2 ...")
	mustFindText(t, d, "and 3 more")
	if strings.Join(omitted, ",") != "b,c,d" {
		t.Fatalf("omitted = %v", omitted)
	}
	if got := selTagText("abcdefgh", 3); got != "abc..." {
		t.Fatalf("selTagText = %q", got)
	}
	if got := selTagText("ab", 3); got != "ab" {
		t.Fatalf("short label cut: %q", got)
	}
	long := []string{"abcdefghijklmnop"}
	var cut, full *MultiSelectW
	d = NewDriver(func(c *Context) {
		full = MultiSelect(c, &long, nil).Width(400)
		cut = MultiSelect(c, &long, nil).MaxTagTextLength(3).Width(400)
	}, DriverOptions{Width: 600, Height: 500})
	d.Settle()
	wf := d.r.c.rects[full.box.n.kids[0].id].W
	wc := d.r.c.rects[cut.box.n.kids[0].id].W
	if !(wc < wf) {
		t.Fatalf("cut tag %v not narrower than full %v", wc, wf)
	}
	if cut.box.n.kids[0].label != "abcdefghijklmnop" {
		t.Fatalf("tag is named %q, want the full label", cut.box.n.kids[0].label)
	}
}

// TestInputTagTriggerSpace: with Trigger("Space") the space key adds the
// tag (EP use-input-tag.mjs handleKeydown) and Enter does nothing.
func TestInputTagTriggerSpace(t *testing.T) {
	var tags []string
	var w *InputTagW
	d := NewDriver(func(c *Context) {
		w = InputTag(c, &tags).Trigger("Space").Width(300)
	}, DriverOptions{Width: 600, Height: 300})
	d.Settle()
	d.Click(d.r.c.rects[w.box.n.id].Center())
	d.Type("go")
	d.Key(input.KeyEnter, 0)
	if len(tags) != 0 {
		t.Fatalf("Enter added %v with the Space trigger", tags)
	}
	d.Key(input.KeySpace, 0)
	d.Type(" ") // the platform's text event for the same key
	if strings.Join(tags, ",") != "go" || w.box.st.query != "" {
		t.Fatalf("tags %v query %q", tags, w.box.st.query)
	}
	// Default trigger: Enter.
	var t2 []string
	var w2 *InputTagW
	d = NewDriver(func(c *Context) { w2 = InputTag(c, &t2).Width(300) }, DriverOptions{Width: 600, Height: 300})
	d.Settle()
	d.Click(d.r.c.rects[w2.box.n.id].Center())
	d.Type("a b")
	d.Key(input.KeyEnter, 0)
	if strings.Join(t2, ",") != "a b" {
		t.Fatalf("Enter trigger: %v", t2)
	}
}

// TestInputTagTriggerDisabled: a disabled tag input ignores the trigger.
func TestInputTagTriggerDisabled(t *testing.T) {
	var tags []string
	var w *InputTagW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("InputTag", Props{"trigger": "Space"}).Body(func() {
			w = InputTag(c, &tags).Width(300).Disabled(true)
		})
	}, DriverOptions{Width: 600, Height: 300})
	d.Settle()
	if w.trigger != "Space" {
		t.Fatalf("default trigger %q", w.trigger)
	}
	d.Click(d.r.c.rects[w.box.n.id].Center())
	d.Type("go")
	d.Key(input.KeySpace, 0)
	if len(tags) != 0 {
		t.Fatalf("disabled input added %v", tags)
	}
}

var b1cTransferItems = []TransferItem{{Key: "a", Title: "Alpha"}, {Key: "b", Title: "Bravo"}, {Key: "c", Title: "Charlie"}}

// b1cMove checks the left items in keys (one by one) and moves them.
func b1cMove(t *testing.T, d *Driver, titles ...string) {
	t.Helper()
	for _, s := range titles {
		if err := d.ClickLabel(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.ClickLabel("to-right"); err != nil {
		t.Fatal(err)
	}
}

// TestTransferTargetOrder: moved keys go first by default (antd
// transfer/index.js: newMoveKeys.concat(targetKeys)) and the right list
// follows the target slice (hooks/useData.js); "push" appends,
// "original" keeps the item order (EP use-move.mjs).
func TestTransferTargetOrder(t *testing.T) {
	for _, tc := range []struct{ order, want string }{
		{"", "a,c"}, {"unshift", "a,c"}, {"push", "c,a"}, {"original", "a,c"},
	} {
		t.Run(tc.order, func(t *testing.T) {
			var target []string
			d := NewDriver(func(c *Context) {
				w := Transfer(c, b1cTransferItems, &target).Actions("to-right", "to-left")
				if tc.order != "" {
					w.TargetOrder(tc.order)
				}
			}, DriverOptions{Width: 800, Height: 400})
			d.Settle()
			b1cMove(t, d, "Charlie") // moved first: push keeps it first,
			b1cMove(t, d, "Alpha")   // unshift and original put Alpha first
			if got := strings.Join(target, ","); got != tc.want {
				t.Fatalf("target = %s, want %s", got, tc.want)
			}
			// The right list shows the target order.
			ra, rc := mustFind(t, d, "Alpha"), mustFind(t, d, "Charlie")
			if (tc.want == "c,a") != (rc.Y < ra.Y) { // the right list follows the slice
				t.Fatalf("right list order: Alpha %v Charlie %v", ra.Y, rc.Y)
			}
		})
	}
}

// TestTransferUnknownTargetKey: target keys without an item are kept and
// not shown (antd rightDataSource.filter(Boolean)).
func TestTransferUnknownTargetKey(t *testing.T) {
	target := []string{"ghost", "b"}
	var w *TransferW
	d := NewDriver(func(c *Context) {
		w = Transfer(c, b1cTransferItems, &target)
	}, DriverOptions{Width: 800, Height: 400})
	d.Settle()
	if got := w.list(1, false); len(got) != 1 || got[0].Key != "b" {
		t.Fatalf("right list %v", got)
	}
	if len(target) != 2 {
		t.Fatalf("target changed: %v", target)
	}
}

// TestTransferShowSelectAll: ShowSelectAll(false) drops the header
// checkboxes; Disabled keeps the move buttons inert.
func TestTransferShowSelectAll(t *testing.T) {
	var target []string
	d := NewDriver(func(c *Context) {
		Transfer(c, b1cTransferItems, &target).ShowSelectAll(false)
	}, DriverOptions{Width: 800, Height: 400})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleCheckbox, d.r.c.Locale().SelectAll); ok {
		t.Fatal("select-all checkbox shown")
	}
	d = NewDriver(func(c *Context) {
		Transfer(c, b1cTransferItems, &target)
	}, DriverOptions{Width: 800, Height: 400})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleCheckbox, d.r.c.Locale().SelectAll); !ok {
		t.Fatal("select-all checkbox missing by default")
	}
}

// TestTransferDisabledTargetOrder: a disabled transfer moves nothing.
func TestTransferDisabledTargetOrder(t *testing.T) {
	var target []string
	d := NewDriver(func(c *Context) {
		Transfer(c, b1cTransferItems, &target).TargetOrder("push").Actions("to-right", "to-left").Disabled(true)
	}, DriverOptions{Width: 800, Height: 400})
	d.Settle()
	d.ClickLabel("Alpha")
	d.ClickLabel("to-right")
	if len(target) != 0 {
		t.Fatalf("disabled transfer moved %v", target)
	}
}

// TestCascaderSeparator: Separator joins the path labels (EP separator).
func TestCascaderSeparator(t *testing.T) {
	var val []string
	var w *CascaderW
	d := NewDriver(func(c *Context) {
		w = Cascader(c, &val, casTestOpts).Separator(" > ").Width(240)
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
	if len(casTestOpts) == 0 || len(casTestOpts[0].Children) == 0 {
		t.Skip("fixture changed")
	}
	o := casTestOpts[0]
	val = []string{o.Value, o.Children[0].Value}
	d.Settle()
	if got, want := w.displayText(), o.Label+" > "+o.Children[0].Label; got != want {
		t.Fatalf("display %q, want %q", got, want)
	}
}

// TestMentionsSplitPlacement: Split is inserted after a pick; Placement
// (TopStart) opens the suggestions above the frame.
func TestMentionsSplitPlacement(t *testing.T) {
	val := ""
	var w *MentionsW
	d := NewDriver(func(c *Context) {
		Gap(c, 300)
		w = Mentions(c, &val, Opts("alice", "bob")).Split(",").Placement(TopStart).Width(300)
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	d.Click(d.r.c.rects[w.ed.id].Center())
	d.Type("@al")
	d.Settle()
	if !w.IsOpen() {
		t.Fatal("suggestions closed")
	}
	pr, br := d.r.c.rects[w.st.pop.panel], d.r.c.rects[w.box.id]
	if pr.H == 0 || pr.Bottom() > br.Y+0.5 {
		t.Fatalf("panel %v not above the frame %v", pr, br)
	}
	d.Key(input.KeyEnter, 0)
	if val != "@alice," {
		t.Fatalf("val = %q", val)
	}
}

// TestAutoCompleteNotFoundPlacement: NotFoundContent keeps an empty
// dropdown open with its text; Placement opens it above.
func TestAutoCompleteNotFoundPlacement(t *testing.T) {
	val := ""
	var w *AutoCompleteW
	d := NewDriver(func(c *Context) {
		Gap(c, 300)
		w = AutoComplete(c, &val, Opts("apple", "banana")).NotFoundContent("no-match").Placement(TopStart).Width(240)
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	d.Click(d.r.c.rects[w.in.box.id].Center())
	d.Type("zzz")
	if !w.IsOpen() {
		t.Fatal("dropdown closed")
	}
	mustFindText(t, d, "no-match")
	pr, br := d.r.c.rects[w.st.pop.panel], d.r.c.rects[w.in.box.id]
	if pr.Bottom() > br.Y+0.5 {
		t.Fatalf("panel %v not above %v", pr, br)
	}
}

// TestSelectPlacementThemes: placement and the empty text work in every
// design system (no Flavor branch).
func TestSelectPlacementThemes(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(theme.Dark())} {
		val := ""
		var w *SelectW
		d := NewDriver(func(c *Context) {
			Gap(c, 300)
			w = Select(c, &val, nil).Placement(TopEnd).NotFoundContent("empty-x").Width(200).Open()
		}, DriverOptions{Width: 600, Height: 600, Theme: th})
		d.Settle()
		mustFindText(t, d, "empty-x")
		if pr, tr := d.r.c.rects[w.st.panel], d.r.c.rects[w.n.id]; pr.Bottom() > tr.Y+0.5 {
			t.Fatalf("%v: panel %v trigger %v", th.Flavor, pr, tr)
		}
	}
}

// TestSelectKeyboardSkipsDisabled: Up / Down skip disabled options
// (rc-select OptionList.js getEnabledActiveIndex).
func TestSelectKeyboardSkipsDisabled(t *testing.T) {
	val := "a"
	opts := []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b", Disabled: true}, {Label: "C", Value: "c"}}
	var w *SelectW
	d := NewDriver(func(c *Context) { w = Select(c, &val, opts).Width(200) }, DriverOptions{Width: 400, Height: 400})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if val != "c" {
		t.Fatalf("Down landed on %q, want c (b is disabled)", val)
	}
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Key(input.KeyUp, 0)
	d.Key(input.KeyEnter, 0)
	if val != "a" {
		t.Fatalf("Up landed on %q, want a", val)
	}
}

// TestSelectEmptyValueOption: an option whose value is "" shows its label
// (antd shows the placeholder only for no value, not for a "" option).
func TestSelectEmptyValueOption(t *testing.T) {
	val := ""
	var w, bare *SelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, []Option{{Label: "All", Value: ""}, {Label: "A", Value: "a"}}).Placeholder("pick").A11yLabel("Filter").Width(200)
		bare = Select(c, &val, Opts("a")).Placeholder("pick").Width(200)
	}, DriverOptions{Width: 500, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	if got := w.label(); got != "All" {
		t.Fatalf("label = %q, want All", got)
	}
	if n, ok := d.A11yFind(a11y.RoleCombobox, "Filter"); !ok || n.Value != "All" {
		t.Fatalf("combobox %+v", n)
	}
	if got := bare.label(); got != "" {
		t.Fatalf("no value without a \"\" option shows %q, want the placeholder", got)
	}
}

// b1cAbove fails unless the overlay panel sits above the trigger.
func b1cAbove(t *testing.T, d *Driver, name string, panel, trigger ID) {
	t.Helper()
	pr, tr := d.r.c.rects[panel], d.r.c.rects[trigger]
	if panel == 0 || pr.H == 0 || pr.Bottom() > tr.Y+0.5 {
		t.Fatalf("%s: panel %v not above trigger %v", name, pr, tr)
	}
}

// TestVirtualSelectPlacementEmptyLoading: VirtualSelect opens above with
// Placement, shows NotFoundContent for an empty list and the loading
// spinner in place of the arrow.
func TestVirtualSelectPlacementEmptyLoading(t *testing.T) {
	val := ""
	var vw, lw *VirtualSelectW
	d := NewDriver(func(c *Context) {
		Gap(c, 300)
		vw = VirtualSelect(c, &val, nil).Placement(TopStart).NotFoundContent("vs-empty").Width(200).Open()
		lw = VirtualSelect(c, &val, Opts("a")).Loading(true).Width(200)
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	b1cAbove(t, d, "VirtualSelect", vw.st.pop.panel, vw.box.n.id)
	mustFindText(t, d, "vs-empty")
	var l paint.List
	lw.box.paint(&l, d.r.c.rects[lw.box.n.id])
	if !hasFillColor(&l, lw.box.th.Components.Select.LoadingIconColor) {
		t.Fatal("VirtualSelect loading spinner missing")
	}
}

// TestSelectEnginePathHost: when the VirtualSelect engine drives Select's
// dropdown (Virtual, Groups, Remote, Loading, long lists) Placement and
// NotFoundContent still apply (fillVSState.host) and OpenChanged fires.
func TestSelectEnginePathHost(t *testing.T) {
	val := ""
	n := 0
	var w *SelectW
	d := NewDriver(func(c *Context) {
		Gap(c, 300)
		w = Select(c, &val, nil).Virtual(true).Placement(TopStart).NotFoundContent("eng-empty").Width(200)
		if w.OpenChanged() {
			n++
		}
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	if !w.IsOpen() || w.eng == nil || n != 1 {
		t.Fatalf("open: %v engine %v changes %d", w.IsOpen(), w.eng != nil, n)
	}
	b1cAbove(t, d, "Select engine", w.st.panel, w.n.id)
	mustFindText(t, d, "eng-empty")
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() || n != 2 {
		t.Fatalf("close: %v changes %d", w.IsOpen(), n)
	}
}

// TestSelectDisabledWhileOpen: disabling an open select closes it and
// reports OpenChanged once.
func TestSelectDisabledWhileOpen(t *testing.T) {
	val := ""
	dis := false
	n := 0
	var w *SelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a", "b")).Width(200).Disabled(dis)
		if w.OpenChanged() {
			n++
		}
	}, DriverOptions{Width: 500, Height: 400})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	if !w.IsOpen() || n != 1 {
		t.Fatalf("open %v changes %d", w.IsOpen(), n)
	}
	dis = true
	d.Inject(input.KeyEvent{}) // any event: rebuild
	d.Settle()
	if w.IsOpen() || n != 2 {
		t.Fatalf("disabled: open %v changes %d", w.IsOpen(), n)
	}
}

// TestSelectLoadingKeepsSuffixIcon: a custom suffix icon wins over the
// loading spinner (antd select/useIcons.js: suffix before loading).
func TestSelectLoadingKeepsSuffixIcon(t *testing.T) {
	val := ""
	var vals []string
	var w *SelectW
	var ms *MultiSelectW
	d := NewDriver(func(c *Context) {
		w = Select(c, &val, Opts("a")).Loading(true).SuffixIcon(icon.Search).Width(200)
		ms = MultiSelect(c, &vals, Opts("a")).Loading(true).SuffixIcon(icon.Search).Width(200)
	}, DriverOptions{Width: 500, Height: 400})
	d.Settle()
	col := w.th.Components.Select.LoadingIconColor
	var l, lm paint.List
	w.paint(&l, d.r.c.rects[w.n.id])
	ms.box.paint(&lm, d.r.c.rects[ms.box.n.id])
	if hasFillColor(&l, col) || hasFillColor(&lm, col) {
		t.Fatal("spinner replaced the custom suffix icon")
	}
}

// TestCascaderTreeSelectPlacementTags: Cascader and TreeSelect open above
// with Placement(TopStart) and fold tags with MaxTagPlaceholder.
func TestCascaderTreeSelectPlacementTags(t *testing.T) {
	var cas []string
	casM := [][]string{{"zj", "hz", "xh"}, {"zj", "nb"}, {"js", "nj"}}
	ts := ""
	tsM := []string{"xh", "nb", "bjs"}
	var cw *CascaderW
	var tw *TreeSelectW
	var cOmit, tOmit []string
	d := NewDriver(func(c *Context) {
		Gap(c, 300)
		Row(c, func() {
			cw = Cascader(c, &cas, casTestOpts).Placement(TopStart).Width(200).Open()
			Gap(c, 260)
			tw = TreeSelect(c, &ts, tsTestTree).Placement(TopStart).Width(200).Open()
		})
		Cascader(c, &cas, casTestOpts).Multiple(&casM).MaxTagCount(1).Width(300).
			MaxTagPlaceholder(func(o []string) string { cOmit = o; return "cas-more" })
		TreeSelect(c, &ts, tsTestTree).Multiple(&tsM).MaxTagCount(1).Width(300).
			MaxTagPlaceholder(func(o []string) string { tOmit = o; return "ts-more" })
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	b1cAbove(t, d, "Cascader", cw.st.pop.panel, cw.box.n.id)
	b1cAbove(t, d, "TreeSelect", tw.st.pop.panel, tw.box.n.id)
	mustFindText(t, d, "cas-more")
	mustFindText(t, d, "ts-more")
	if len(cOmit) != 2 || len(tOmit) != 2 {
		t.Fatalf("omitted: cascader %v, tree select %v", cOmit, tOmit)
	}
}

// TestTransferDuplicateTargetKey: a key repeated in the target slice
// shows once on the right.
func TestTransferDuplicateTargetKey(t *testing.T) {
	target := []string{"b", "b", "a"}
	var w *TransferW
	d := NewDriver(func(c *Context) { w = Transfer(c, b1cTransferItems, &target) }, DriverOptions{Width: 800, Height: 400})
	d.Settle()
	if got := w.list(1, false); len(got) != 2 || got[0].Key != "b" || got[1].Key != "a" {
		t.Fatalf("right list %v", got)
	}
}

// TestCascaderEmptySeparator: Separator("") joins the labels directly.
func TestCascaderEmptySeparator(t *testing.T) {
	val := []string{"zj", "nb"}
	var w *CascaderW
	d := NewDriver(func(c *Context) { w = Cascader(c, &val, casTestOpts).Separator("").Width(240) }, DriverOptions{Width: 500, Height: 300})
	d.Settle()
	if got := w.displayText(); got != "浙江宁波" { // i18n:ok test fixture labels
		t.Fatalf("display %q", got)
	}
}

// TestSelectPlacementElementNames: Defaults "placement" also takes the
// Element names; Mentions takes "top" and Placement values.
func TestSelectPlacementElementNames(t *testing.T) {
	val := ""
	var w *SelectW
	var m1, m2 *MentionsW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Select", Props{"placement": "top-end"}).
			Defaults("Mentions", Props{"placement": "top"}).Body(func() {
			w = Select(c, &val, Opts("a")).Width(200)
			m1 = Mentions(c, &val, Opts("a"))
		})
		ConfigProvider(c).Defaults("Mentions", Props{"placement": TopStart}).Body(func() {
			m2 = Mentions(c, &val, Opts("a"))
		})
	}, DriverOptions{Width: 500, Height: 400})
	d.Settle()
	if w.place != TopEnd || m1.place != TopStart || m2.place != TopStart {
		t.Fatalf("placements %v %v %v", w.place, m1.place, m2.place)
	}
	// The omitted labels handed to MaxTagPlaceholder are a copy.
	labels := []string{"a", "b", "c"}
	_ = NewDriver(func(c *Context) {
		selectorTags(c, labels, selTagOpts{max: 1, rest: func(o []string) string { o[0] = "x"; return "r" }}, false, false, Middle, false)
	}, DriverOptions{Width: 300, Height: 200}).Settle()
	if labels[1] != "b" {
		t.Fatal("MaxTagPlaceholder callback modified the selector's labels")
	}
}
