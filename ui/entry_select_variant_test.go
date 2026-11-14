package ui

// Variant (AutoComplete, Mentions, Select, MultiSelect, Cascader,
// TreeSelect), Select / MultiSelect Prefix and SuffixIcon,
// AutoComplete.TriggerOnFocus, Mentions AllowClear / Whole, Cascader
// FilterMethod / DisplayRender and TreeSelect CheckStrictly /
// FilterTreeNode.

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// frameLook classifies the frame a paint function draws into r: whether it
// strokes a border, fills the full rect with fill, or draws only a bottom
// line.
type frameLook struct {
	stroke, bottomLine, filled bool
}

func lookOf(paintFn func(*paint.List, geom.Rect), r geom.Rect, fill theme.Color) frameLook {
	var l paint.List
	paintFn(&l, r)
	var lk frameLook
	for _, op := range l.Ops {
		switch o := op.(type) {
		case paint.StrokeRect:
			if o.Rect == r {
				lk.stroke = true
			}
		case paint.FillRect:
			if o.Rect.H == 1 && o.Rect.Y == r.Bottom()-1 && o.Rect.W == r.W {
				lk.bottomLine = true
			}
			if o.Rect == r && o.Color == fill {
				lk.filled = true
			}
		}
	}
	return lk
}

// TestFieldVariantLooks: every selector paints the four variants through
// the shared frame (outlined border, filled tint, no frame, bottom line).
func TestFieldVariantLooks(t *testing.T) {
	type painter struct {
		name  string
		build func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID)
	}
	s, ms, ac, mt := "a", []string{"a"}, "", ""
	var cas []string
	ts := ""
	ps := []painter{
		{"Select", func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID) {
			w := Select(c, &s, Opts("a", "b")).Variant(v)
			return w.paint, w.n.id
		}},
		{"MultiSelect", func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID) {
			w := MultiSelect(c, &ms, Opts("a", "b")).Variant(v)
			return w.box.paint, w.box.n.id
		}},
		{"Cascader", func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID) {
			w := Cascader(c, &cas, casTestOpts).Variant(v)
			return w.box.paint, w.box.n.id
		}},
		{"TreeSelect", func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID) {
			w := TreeSelect(c, &ts, tsTestTree).Variant(v)
			return w.box.paint, w.box.n.id
		}},
		{"AutoComplete", func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID) {
			w := AutoComplete(c, &ac, Opts("x")).Variant(v)
			return w.in.paintBox, w.in.box.id
		}},
		{"Mentions", func(c *Context, v FieldVariant) (func(*paint.List, geom.Rect), ID) {
			w := Mentions(c, &mt, Opts("x")).Variant(v)
			return w.paintBox, w.box.id
		}},
	}
	th := theme.AntDesign()
	for _, p := range ps {
		for _, v := range []FieldVariant{"", FieldOutlined, FieldFilled, FieldBorderless, FieldUnderlined} {
			var fn func(*paint.List, geom.Rect)
			var id ID
			d := NewDriver(func(c *Context) {
				Column(c, func() { fn, id = p.build(c, v) }, layout.Pad(20), layout.W(300))
			}, DriverOptions{Width: 400, Height: 200, Theme: th})
			d.Frame()
			r := d.r.c.rects[id]
			lk := lookOf(fn, r, th.FillTertiary)
			var ok bool
			switch v {
			case "", FieldOutlined:
				ok = lk.stroke && !lk.bottomLine
			case FieldFilled:
				ok = lk.filled && !lk.stroke
			case FieldBorderless:
				ok = !lk.stroke && !lk.filled && !lk.bottomLine
			case FieldUnderlined:
				ok = lk.bottomLine && !lk.stroke
			}
			if !ok {
				t.Errorf("%s variant %q: look %+v", p.name, v, lk)
			}
		}
	}
}

// TestSelectPrefixSuffixIcon: the prefix sits before the value inside the
// frame, SuffixIcon replaces the arrow (also while searching), and the
// MultiSelect shell gets the same options.
func TestSelectPrefixSuffixIcon(t *testing.T) {
	v := "b"
	var w *SelectW
	var m *MultiSelectW
	vals := []string{"a"}
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			w = Select(c, &v, Opts("a", "b")).Prefix(icon.User).PrefixText("Owner").SuffixIcon(icon.Search).Searchable().Width(240)
			m = MultiSelect(c, &vals, Opts("a", "b")).PrefixText("Tags").SuffixIcon(icon.Search).Width(240)
		}, layout.Pad(20), layout.Gap(12))
	}, DriverOptions{Width: 400, Height: 300})
	d.Frame()
	d.Frame()
	pr := d.r.c.rects[hashID(w.n.id, "prefix")]
	vr := d.r.c.rects[w.edID]
	if pr.W <= 0 || pr.Right() > vr.X {
		t.Fatalf("prefix %v must precede the value %v", pr, vr)
	}
	// The prefix is a decoration: the combobox keeps its value.
	if w.label() != "b" {
		t.Fatalf("label %q", w.label())
	}
	if m.box.prefixN == nil || d.r.c.rects[m.box.prefixN.id].W <= 0 {
		t.Fatal("multiselect prefix missing")
	}
	if m.box.chipCount() != 1 {
		t.Fatalf("chips = %d (the prefix is not a tag)", m.box.chipCount())
	}
	if m.box.arrowIcon != icon.Search {
		t.Fatal("multiselect suffix icon")
	}
	// Selecting still works with the prefix in front.
	d.Click(d.r.c.rects[w.n.id].Center())
	d.ClickLabel("a")
	if v != "a" {
		t.Fatalf("pick with prefix: %q", v)
	}
}

// TestAutoCompleteTriggerOnFocus: focusing (Tab or click) opens the
// suggestions with nothing typed; without the option it stays closed.
func TestAutoCompleteTriggerOnFocus(t *testing.T) {
	for _, on := range []bool{false, true} {
		val := ""
		var w *AutoCompleteW
		d := driver(func(c *Context) {
			Column(c, func() {
				w = AutoComplete(c, &val, Opts("apple", "banana")).Placeholder("fruit").Width(240)
				if on {
					w.TriggerOnFocus()
				}
			}, layout.Pad(20))
		})
		d.Frame()
		d.Key(input.KeyTab, 0)
		if w.IsOpen() != on {
			t.Fatalf("on=%v: open after Tab = %v", on, w.IsOpen())
		}
		if on {
			if _, ok := d.Find("banana"); !ok {
				t.Fatal("all options expected")
			}
			d.Key(input.KeyEscape, 0)
			if w.IsOpen() {
				t.Fatal("escape")
			}
			d.Frame()
			if w.IsOpen() {
				t.Fatal("must not reopen while focus stays")
			}
		}
	}
	// Remote: the callback runs at once on focus.
	val := ""
	calls := []string{}
	d := driver(func(c *Context) {
		AutoComplete(c, &val, nil).TriggerOnFocus().Remote(func(q string) { calls = append(calls, q) })
	})
	d.Frame()
	d.Key(input.KeyTab, 0)
	if len(calls) != 1 || calls[0] != "" {
		t.Fatalf("remote on focus: %q", calls)
	}
}

// TestMentionsAllowClear: the clear button shows with text and empties it.
func TestMentionsAllowClear(t *testing.T) {
	val := "hi @Lucy"
	var w *MentionsW
	cleared := false
	d := driver(func(c *Context) {
		Column(c, func() {
			w = Mentions(c, &val, Opts("Lucy")).AllowClear().Width(300)
			cleared = cleared || w.Cleared()
		}, layout.Pad(20))
	})
	d.Frame()
	r, ok := d.Find("clear")
	if !ok {
		t.Fatal("clear button missing")
	}
	if br := d.r.c.rects[w.box.id]; r.Right() > br.Right() || r.X < br.Right()-40 {
		t.Fatalf("clear %v not at the end of %v", r, br)
	}
	d.Click(r.Center())
	if val != "" || !cleared {
		t.Fatalf("clear: %q cleared=%v", val, cleared)
	}
	if _, ok := d.Find("clear"); ok {
		t.Fatal("clear button shown on empty input")
	}
}

// TestMentionsWhole: Backspace after a mention deletes it as a unit (with
// the space a pick inserted); other text deletes one rune; WholeCheck
// restricts the mentions.
func TestMentionsWhole(t *testing.T) {
	val := ""
	var w *MentionsW
	d := driver(func(c *Context) {
		Column(c, func() { w = Mentions(c, &val, Opts("Lucy", "Lily")).Whole().Width(300) }, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.ed.id].Center())
	d.Type("hi @lu")
	d.Key(input.KeyEnter, 0)
	if val != "hi @Lucy " {
		t.Fatalf("insert: %q", val)
	}
	d.Key(input.KeyBackspace, 0)
	if val != "hi " || w.st.edit.cur != 3 {
		t.Fatalf("whole delete: %q cur=%d", val, w.st.edit.cur)
	}
	d.Key(input.KeyBackspace, 0)
	if val != "hi" {
		t.Fatalf("plain backspace: %q", val)
	}
	d.Type(" @Bob")
	d.Key(input.KeyEscape, 0)
	d.Key(input.KeyBackspace, 0)
	if val != "hi " {
		t.Fatalf("mention at caret: %q", val)
	}

	// WholeCheck: only known names go as a unit.
	val2 := "@Lucy @Bob"
	var w2 *MentionsW
	d2 := driver(func(c *Context) {
		Column(c, func() {
			w2 = Mentions(c, &val2, Opts("Lucy")).WholeCheck(func(p, v string) bool { return p == "@" && v == "Lucy" }).Width(300)
		}, layout.Pad(20))
	})
	d2.Frame()
	d2.Click(d2.r.c.rects[w2.ed.id].Center())
	w2.st.edit.cur, w2.st.edit.anc = len([]rune(val2)), len([]rune(val2))
	d2.Key(input.KeyBackspace, 0)
	if val2 != "@Lucy @Bo" {
		t.Fatalf("unknown mention must delete one rune: %q", val2)
	}
	w2.st.edit.cur, w2.st.edit.anc = 5, 5
	d2.Key(input.KeyBackspace, 0)
	if val2 != " @Bo" {
		t.Fatalf("known mention: %q", val2)
	}
}

// TestCascaderFilterMethodDisplayRender: the custom filter decides the
// search hits from the option path; DisplayRender sets the trigger text.
func TestCascaderFilterMethodDisplayRender(t *testing.T) {
	val := []string{"zj", "hz", "xh"}
	var w *CascaderW
	var gotPath []string
	d := driver(func(c *Context) {
		w = Cascader(c, &val, casTestOpts).Placeholder("pick").Width(300).
			FilterMethod(func(q string, p []CascaderOption) bool {
				// Match by the leaf's value prefix.
				return strings.HasPrefix(p[len(p)-1].Value, q)
			}).
			DisplayRender(func(ls []string, p []CascaderOption) string {
				gotPath = gotPath[:0]
				for _, o := range p {
					gotPath = append(gotPath, o.Value)
				}
				return ls[len(ls)-1]
			})
	})
	d.Frame()
	if w.box.single != "西湖" || casKey(gotPath) != casKey(val) {
		t.Fatalf("display %q path %v", w.box.single, gotPath)
	}
	if !w.box.search {
		t.Fatal("FilterMethod implies Searchable")
	}
	d.Click(d.r.c.rects[w.box.n.id].Center())
	d.Type("n")
	hits := w.searchResults("n")
	if len(hits) != 2 || hits[0].labels[1] != "宁波" || hits[1].labels[1] != "南京" {
		t.Fatalf("hits %+v", hits)
	}
	if _, ok := d.Find("浙江 / 杭州 / 西湖"); ok {
		t.Fatal("label match must not apply")
	}
	d.ClickLabel("江苏 / 南京")
	if casKey(val) != casKey([]string{"js", "nj"}) || w.box.single != "南京" {
		t.Fatalf("pick %v display %q", val, w.box.single)
	}
	// Without DisplayRender the full path shows.
	v2 := []string{"js", "nj"}
	var w2 *CascaderW
	d2 := driver(func(c *Context) { w2 = Cascader(c, &v2, casTestOpts) })
	d2.Frame()
	if w2.box.single != "江苏 / 南京" {
		t.Fatalf("default display %q", w2.box.single)
	}
}

// TestTreeSelectCheckStrictly: parents and children are checked on their
// own; tags and Backspace follow the checked nodes.
func TestTreeSelectCheckStrictly(t *testing.T) {
	v := ""
	var vals []string
	d, w := tsDriver(func(c *Context) *TreeSelectW {
		return TreeSelect(c, &v, tsTestTree).Multiple(&vals).CheckStrictly().ShowCheckedStrategy(ShowParent).DefaultExpandAll().Width(400)
	})
	d.Click(d.r.c.rects[(*w).n.id].Center())
	d.ClickLabel("杭州市")
	if len(vals) != 1 || vals[0] != "hz" {
		t.Fatalf("strict parent check: %v", vals)
	}
	set := (*w).valueSet()
	if st := (*w).checkState(tsFind(tsTestTree, "xh"), set); st != tsOff {
		t.Fatalf("child linked: %d", st)
	}
	if st := (*w).checkState(tsFind(tsTestTree, "zj"), set); st != tsOff {
		t.Fatalf("ancestor half-checked: %d", st)
	}
	d.ClickLabel("西湖区")
	if len(vals) != 2 || vals[1] != "xh" {
		t.Fatalf("strict child check: %v", vals)
	}
	if got := (*w).Checked(); len(got) != 2 || got[0] != "hz" || got[1] != "xh" {
		t.Fatalf("tags follow the checked nodes: %v", got)
	}
	d.ClickLabel("杭州市")
	if len(vals) != 1 || vals[0] != "xh" {
		t.Fatalf("strict uncheck: %v", vals)
	}
	d.r.c.SetFocus((*w).box.ed.id)
	d.Frame()
	d.Key(input.KeyBackspace, 0)
	if len(vals) != 0 {
		t.Fatalf("backspace: %v", vals)
	}
}

// TestTreeSelectFilterTreeNode: the custom match picks nodes (shown with
// their ancestors) by any field.
func TestTreeSelectFilterTreeNode(t *testing.T) {
	v := ""
	var gotQ string
	d, w := tsDriver(func(c *Context) *TreeSelectW {
		return TreeSelect(c, &v, tsTestTree).FilterTreeNode(func(q string, n TreeSelectNode) bool {
			gotQ = q
			return n.Value == q // match by value
		})
	})
	if !(*w).box.search {
		t.Fatal("FilterTreeNode implies Searchable")
	}
	d.Click(d.r.c.rects[(*w).n.id].Center())
	d.Type("nb")
	rows := (*w).rows()
	if len(rows) != 2 || rows[0].nd.Value != "zj" || rows[1].nd.Value != "nb" || !rows[1].match || rows[0].match {
		t.Fatalf("rows %+v", rows)
	}
	if gotQ != "nb" {
		t.Fatalf("query %q", gotQ)
	}
	if _, ok := d.Find("北京市"); ok {
		t.Fatal("label match must not apply")
	}
}
