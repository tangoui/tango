package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

var p2bGrouped = []Option{
	{Label: "Apple", Value: "apple", Group: "Fruits"},
	{Label: "Carrot", Value: "carrot", Group: "Veggies"},
	{Label: "Banana", Value: "banana", Group: "Fruits"},
	{Label: "Daikon", Value: "daikon", Group: "Veggies", Disabled: true},
}

func TestP2BSelectOptionGroups(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		var w *SelectW
		d := fillDriver(th, func(c *Context) {
			w = Select(c, &val, p2bGrouped).Placeholder("food").Width(240)
		})
		d.EnableAccessibility()
		d.Click(d.r.c.rects[w.n.id].Center())
		d.Settle()
		if _, ok := d.FindText("Fruits"); !ok {
			t.Fatal("group title missing")
		}
		// Grouped by first appearance: Apple, Banana, then Veggies.
		a, _ := d.Find("Apple")
		b, _ := d.Find("Banana")
		cr, _ := d.Find("Carrot")
		if !(a.Y < b.Y && b.Y < cr.Y) {
			t.Fatalf("group order: apple %v banana %v carrot %v", a, b, cr)
		}
		// Semantics: listbox > group(name) > option with in-group positions.
		g, ok := d.A11yFind(a11y.RoleGroup, "Veggies")
		if !ok {
			t.Fatal("veggies group not exposed")
		}
		opt, ok := d.A11yFind(a11y.RoleOption, "Carrot")
		if !ok || opt.Parent != g.ID || opt.PosInSet != 1 || opt.SetSize != 2 {
			t.Fatalf("carrot option %+v group %v", opt, g.ID)
		}
		// Keyboard skips group titles and disabled options.
		d.Key(input.KeyDown, 0) // Banana
		d.Key(input.KeyDown, 0) // Carrot
		d.Key(input.KeyEnter, 0)
		if val != "carrot" {
			t.Fatalf("val = %q", val)
		}
		if w.label() != "Carrot" {
			t.Fatalf("label %q", w.label())
		}
	})
}

func TestP2BMultiSelectGroupsLimitRender(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var vals []string
		groups := []OptionGroup{{Label: "Hot", Options: Opts("Go", "Rust")}, {Label: "Old", Options: Opts("Perl", "Tcl")}}
		d := fillDriver(th, func(c *Context) {
			MultiSelect(c, &vals, nil).Groups(groups).MaxCount(2).Placeholder("langs").Width(320).
				OptionRender(func(o Option, s OptionState) {
					Text(c, "★ "+o.Label)
				})
		})
		d.ClickLabel("langs")
		if _, ok := d.FindText("★ Rust"); !ok {
			t.Fatal("custom option content missing")
		}
		if _, ok := d.FindText("Old"); !ok {
			t.Fatal("group title missing")
		}
		d.ClickLabel("Go")
		d.ClickLabel("Perl")
		d.ClickLabel("Tcl") // over the limit
		if len(vals) != 2 || vals[0] != "Go" || vals[1] != "Perl" {
			t.Fatalf("vals = %v", vals)
		}
		d.ClickLabel("Go") // deselect frees a slot
		d.ClickLabel("Tcl")
		if len(vals) != 2 || vals[1] != "Tcl" {
			t.Fatalf("after swap vals = %v", vals)
		}
	})
}

func TestP2BSelectRemoteLoadingCreate(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		var queries []string
		var opts []Option
		loading := false
		created := ""
		var w *SelectW
		d := fillDriver(th, func(c *Context) {
			w = Select(c, &val, opts).Placeholder("user").Width(240).Loading(loading).
				Remote(func(q string) { queries = append(queries, q); loading = true }).Debounce(200 * time.Millisecond)
			if v, ok := w.Created(); ok {
				created = v
			}
		})
		d.Click(d.r.c.rects[w.n.id].Center())
		d.Type("li")
		if len(queries) != 0 {
			t.Fatal("remote called before the debounce")
		}
		d.Advance(250 * time.Millisecond)
		d.Settle()
		if len(queries) != 1 || queries[0] != "li" {
			t.Fatalf("queries %v", queries)
		}
		d.Settle()
		if _, ok := d.FindText(ZhCN.Loading); !ok {
			t.Fatal("loading row missing")
		}
		opts, loading = Opts("lily", "lisa"), false
		d.Settle()
		d.ClickLabel("lisa")
		if val != "lisa" || w.IsOpen() {
			t.Fatalf("val %q open %v", val, w.IsOpen())
		}
		_ = created
	})
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		created := ""
		var w *SelectW
		d := fillDriver(th, func(c *Context) {
			w = Select(c, &val, Opts("red", "green")).AllowCreate().Placeholder("color").Width(240)
			if v, ok := w.Created(); ok {
				created = v
			}
		})
		d.Click(d.r.c.rects[w.n.id].Center())
		d.Type("teal")
		d.Key(input.KeyEnter, 0)
		if val != "teal" || created != "teal" {
			t.Fatalf("create: val %q created %q", val, created)
		}
		if w.label() != "teal" {
			t.Fatalf("created label %q", w.label())
		}
	})
}

func TestP2BMultiSelectCreateRemote(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var vals []string
		var got []string
		var opts []Option
		d := fillDriver(th, func(c *Context) {
			MultiSelect(c, &vals, opts).Placeholder("tags").AllowCreate().Width(320).
				Remote(func(q string) { got = append(got, q); opts = Opts(q + "-1") })
		})
		d.ClickLabel("tags")
		d.Type("ab")
		d.Advance(350 * time.Millisecond)
		d.Settle()
		if len(got) != 1 || got[0] != "ab" {
			t.Fatalf("remote %v", got)
		}
		d.Settle()
		if _, ok := d.Find("ab-1"); !ok {
			t.Fatal("remote result missing")
		}
		d.Key(input.KeyEnter, 0) // the create row comes first
		if len(vals) != 1 || vals[0] != "ab" {
			t.Fatalf("vals %v", vals)
		}
	})
}

func TestP2BSelectVirtual100k(t *testing.T) {
	opts := fillOpts(100000)
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := "500"
		var w *SelectW
		d := fillDriver(th, func(c *Context) {
			w = Select(c, &val, opts).Searchable().Width(240)
		})
		d.Click(d.r.c.rects[w.n.id].Center())
		d.Settle()
		if _, ok := d.Find("Item 000500"); !ok {
			t.Fatal("selected option not revealed")
		}
		if _, ok := d.Find("Item 000000"); ok {
			t.Fatal("rows far above the window are built")
		}
		d.Key(input.KeyEnd, 0)
		d.Settle()
		if _, ok := d.Find("Item 099999"); !ok {
			t.Fatal("End did not reach the last option")
		}
		d.Key(input.KeyPageUp, 0)
		d.Key(input.KeyEnter, 0)
		if val == "500" || val == "99999" || w.IsOpen() {
			t.Fatalf("PageUp+Enter picked %q open=%v", val, w.IsOpen())
		}
		var vals []string
		d2 := fillDriver(th, func(c *Context) {
			MultiSelect(c, &vals, opts).Placeholder("many").Width(300)
		})
		d2.ClickLabel("many")
		if _, ok := d2.Find("Item 000040"); ok {
			t.Fatal("multi: rows below the window are built")
		}
		d2.ClickLabel("Item 000002")
		if len(vals) != 1 || vals[0] != "2" {
			t.Fatalf("multi vals %v", vals)
		}
	})
}

func TestP2BSelectConfigDefaults(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := "a"
		var w *SelectW
		var m *MultiSelectW
		vals := []string{"a"}
		fillDriver(th, func(c *Context) {
			ConfigProvider(c).Defaults("Select", Props{"allowClear": true, "showSearch": true, "placeholder": "pick", "maxTagCount": 1}).Body(func() {
				w = Select(c, &val, Opts("a", "b"))
				m = MultiSelect(c, &vals, Opts("a", "b"))
			})
		})
		if !w.allowClear || !w.search || w.placeholder != "pick" {
			t.Fatalf("select defaults: clear %v search %v ph %q", w.allowClear, w.search, w.placeholder)
		}
		if !m.box.allowClear || !m.box.search || m.box.maxCount != 1 {
			t.Fatal("multiselect defaults not applied")
		}
	})
}

// BenchmarkSelect100k measures a frame of Select's virtual dropdown over
// 100k options while it scrolls.
func BenchmarkSelect100k(b *testing.B) {
	opts := fillOpts(100000)
	val := ""
	var w *SelectW
	d := NewDriver(func(c *Context) { w = Select(c, &val, opts).Width(240) }, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Settle()
	r, _ := d.Find("Item 000000")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(40)
		if i%200 >= 100 {
			dy = -40
		}
		d.Inject(input.Scroll{Pos: r.Center(), DY: dy})
		d.Frame()
	}
}

// BenchmarkMultiSelect100k is BenchmarkSelect100k for MultiSelect.
func BenchmarkMultiSelect100k(b *testing.B) {
	opts := fillOpts(100000)
	var vals []string
	d := NewDriver(func(c *Context) { MultiSelect(c, &vals, opts).Placeholder("pick").Width(240) }, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	d.ClickLabel("pick")
	r, _ := d.Find("Item 000000")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(40)
		if i%200 >= 100 {
			dy = -40
		}
		d.Inject(input.Scroll{Pos: r.Center(), DY: dy})
		d.Frame()
	}
}
