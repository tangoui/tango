package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func TestAutoCompleteRemoteDebounceLoading(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		var queries []string
		var opts []Option
		loading := false
		picked := ""
		var w *AutoCompleteW
		d := fillDriver(th, func(c *Context) {
			w = AutoComplete(c, &val, opts).Placeholder("city").Width(240).Loading(loading).
				Remote(func(q string) { queries = append(queries, q); loading = true }).
				Debounce(200 * time.Millisecond)
			if v, ok := w.Selected(); ok {
				picked = v
			}
		})
		d.Click(d.r.c.rects[w.in.box.id].Center())
		d.Type("par")
		if len(queries) != 0 {
			t.Fatalf("remote called before the debounce: %v", queries)
		}
		if w.IsOpen() {
			t.Fatal("dropdown shown with nothing to show")
		}
		d.Advance(100 * time.Millisecond)
		d.Type("i") // restarts the debounce
		d.Advance(150 * time.Millisecond)
		d.Settle()
		if len(queries) != 0 {
			t.Fatalf("debounce not restarted: %v", queries)
		}
		d.Advance(100 * time.Millisecond)
		d.Settle()
		if len(queries) != 1 || queries[0] != "pari" {
			t.Fatalf("queries %v", queries)
		}
		d.Settle()
		if _, ok := d.FindText(ZhCN.Loading); !ok || !w.IsOpen() {
			t.Fatalf("loading row missing (open %v)", w.IsOpen())
		}
		d.Key(input.KeyEnter, 0) // nothing to pick behind the loading row
		if val != "pari" {
			t.Fatalf("enter while loading picked %q", val)
		}
		// Server results are shown unfiltered (no label contains "pari").
		opts, loading = Opts("Paris, FR", "Paris, TX", "Parikia"), false
		d.Settle()
		if _, ok := d.FindText(ZhCN.Loading); ok {
			t.Fatal("loading row still shown")
		}
		if _, ok := d.Find("Paris, TX"); !ok {
			t.Fatal("remote results not shown")
		}
		d.Key(input.KeyDown, 0)
		d.Key(input.KeyEnter, 0)
		if val != "Paris, TX" || picked != "Paris, TX" || w.IsOpen() {
			t.Fatalf("pick: val %q picked %q open %v", val, picked, w.IsOpen())
		}
		d.Advance(time.Second)
		d.Settle()
		if len(queries) != 1 {
			t.Fatalf("a pick must not search again: %v", queries)
		}
	})
}

func TestAutoCompleteSourceKeepsLocalBehaviour(t *testing.T) {
	val := ""
	var w *AutoCompleteW
	d := fillDriver(func() *theme.Theme { return theme.AntDesign() }, func(c *Context) {
		w = AutoComplete(c, &val, nil).Width(240).Source(func(q string) []Option {
			if q == "" {
				return nil
			}
			return Opts(q+"@a.com", q+"@b.com")
		})
	})
	d.Click(d.r.c.rects[w.in.box.id].Center())
	d.Type("me")
	if !w.IsOpen() {
		t.Fatal("source suggestions not shown")
	}
	if _, ok := d.Find("me@b.com"); !ok {
		t.Fatal("source option missing")
	}
	d.Key(input.KeyBackspace, 0)
	d.Key(input.KeyBackspace, 0)
	if w.IsOpen() {
		t.Fatal("empty source must close")
	}
}

func TestMentionsRemoteDebounceLoading(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		type call struct{ prefix, query string }
		var calls []call
		var opts []Option
		loading := false
		var w *MentionsW
		d := fillDriver(th, func(c *Context) {
			w = Mentions(c, &val, opts).Prefix("@", "#").Width(300).Loading(loading).
				Remote(func(p, q string) { calls = append(calls, call{p, q}); loading = true }).
				Debounce(150 * time.Millisecond)
		})
		d.Click(d.r.c.rects[w.box.id].Center())
		d.Type("hello ")
		d.Advance(time.Second)
		d.Settle()
		if len(calls) != 0 {
			t.Fatalf("remote called outside a mention: %v", calls)
		}
		d.Type("#go")
		if len(calls) != 0 {
			t.Fatal("remote called before the debounce")
		}
		if p, q, ok := w.Query(); !ok || p != "#" || q != "go" {
			t.Fatalf("Query() = %q %q %v", p, q, ok)
		}
		d.Advance(200 * time.Millisecond)
		d.Settle()
		if len(calls) != 1 || calls[0] != (call{"#", "go"}) {
			t.Fatalf("calls %v", calls)
		}
		d.Settle()
		if _, ok := d.FindText(ZhCN.Loading); !ok || !w.IsOpen() {
			t.Fatalf("loading row missing (open %v)", w.IsOpen())
		}
		// Results are shown as-is (not filtered by "go").
		opts, loading = Opts("golang", "gopher", "rust"), false
		d.Settle()
		if _, ok := d.Find("rust"); !ok {
			t.Fatal("remote results not shown unfiltered")
		}
		d.ClickLabel("gopher")
		if val != "hello #gopher " || w.IsOpen() {
			t.Fatalf("insert %q open %v", val, w.IsOpen())
		}
		if _, _, ok := w.Query(); ok {
			t.Fatal("Query() active after the insert")
		}
		// A pending search whose mention is gone by the deadline is dropped.
		d.Type("@x")
		d.Key(input.KeyBackspace, 0)
		d.Key(input.KeyBackspace, 0)
		d.Advance(time.Second)
		d.Settle()
		if len(calls) != 1 {
			t.Fatalf("stale search fired: %v", calls)
		}
	})
}

func TestSelectFilterMethod(t *testing.T) {
	byValue := func(q string, o Option) bool { return strings.HasPrefix(o.Value, q) }
	opts := []Option{{Label: "Beijing", Value: "bj"}, {Label: "Shanghai", Value: "sh"}, {Label: "Shenzhen", Value: "sz"}}
	for _, rich := range []bool{false, true} {
		name := "plain"
		if rich {
			name = "engine"
		}
		t.Run(name, func(t *testing.T) {
			fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
				val := ""
				var w *SelectW
				d := fillDriver(th, func(c *Context) {
					w = Select(c, &val, opts).Placeholder("city").Width(240).FilterMethod(byValue)
					if rich {
						w.Virtual(true)
					}
				})
				if !w.search {
					t.Fatal("FilterMethod must imply Searchable")
				}
				d.Click(d.r.c.rects[w.n.id].Center())
				d.Type("s")
				if _, ok := d.Find("Beijing"); ok {
					t.Fatal("custom filter ignored (Beijing shown)")
				}
				if _, ok := d.Find("Shenzhen"); !ok {
					t.Fatal("custom filter dropped a match")
				}
				d.Type("z")
				if _, ok := d.Find("Shanghai"); ok {
					t.Fatal("Shanghai shown for sz")
				}
				d.ClickLabel("Shenzhen")
				if val != "sz" {
					t.Fatalf("val %q", val)
				}
			})
		})
	}
}

func TestMultiSelectFilterMethod(t *testing.T) {
	byValue := func(q string, o Option) bool { return strings.HasPrefix(o.Value, q) }
	opts := []Option{{Label: "Beijing", Value: "bj"}, {Label: "Shanghai", Value: "sh"}, {Label: "Shenzhen", Value: "sz"}}
	for _, rich := range []bool{false, true} {
		fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
			var vals []string
			var w *MultiSelectW
			d := fillDriver(th, func(c *Context) {
				w = MultiSelect(c, &vals, opts).Placeholder("city").Width(300).FilterMethod(byValue)
				if rich {
					w.Virtual(true)
				}
			})
			d.Click(d.r.c.rects[w.box.n.id].Center())
			d.Type("b")
			if _, ok := d.Find("Shanghai"); ok {
				t.Fatalf("rich=%v: custom filter ignored", rich)
			}
			d.ClickLabel("Beijing")
			if len(vals) != 1 || vals[0] != "bj" {
				t.Fatalf("rich=%v: vals %v", rich, vals)
			}
		})
	}
}

func TestFilterOptions(t *testing.T) {
	opts := Opts("Apple", "banana", "Cherry")
	if got := filterOptions(opts, "", nil); len(got) != 3 {
		t.Fatalf("empty query: %v", got)
	}
	if got := filterOptions(opts, "AN", nil); len(got) != 1 || got[0].Label != "banana" {
		t.Fatalf("label match: %v", got)
	}
	exact := func(q string, o Option) bool { return o.Label == q }
	if got := filterOptions(opts, "Cherry", exact); len(got) != 1 {
		t.Fatalf("custom: %v", got)
	}
	if got := filterOptions(opts, "cherry", exact); len(got) != 0 {
		t.Fatalf("custom gets the raw query: %v", got)
	}
}
