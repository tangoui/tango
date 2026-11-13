package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// fillThemes runs fn under Ant Design and Element Plus.
func fillThemes(t *testing.T, fn func(t *testing.T, th func() *theme.Theme)) {
	for _, tc := range []struct {
		name string
		th   func() *theme.Theme
	}{{"ant", func() *theme.Theme { return theme.AntDesign() }}, {"element", func() *theme.Theme { return theme.ElementPlus() }}} {
		t.Run(tc.name, func(t *testing.T) { fn(t, tc.th) })
	}
}

func fillDriver(th func() *theme.Theme, page func(*Context)) *Driver {
	d := NewDriver(page, DriverOptions{Width: 900, Height: 700, Theme: th()})
	d.Settle()
	return d
}

func fillOpts(n int) []Option {
	out := make([]Option, n)
	for i := range out {
		out[i] = Option{Label: fmt.Sprintf("Item %06d", i), Value: fmt.Sprint(i)}
	}
	return out
}

func TestVirtualSelectSingleFilterKeyboard(t *testing.T) {
	opts := fillOpts(100000)
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		var w *VirtualSelectW
		d := fillDriver(th, func(c *Context) {
			w = VirtualSelect(c, &val, opts).Placeholder("pick").Filterable().Width(240)
			w.Changed()
		})
		if err := d.ClickLabel("pick"); err != nil {
			t.Fatal(err)
		}
		if !w.IsOpen() {
			t.Fatal("dropdown should open on click")
		}
		// Only a window of rows is built.
		if _, ok := d.Find("Item 000000"); !ok {
			t.Fatal("first option not built")
		}
		if _, ok := d.Find("Item 000050"); ok {
			t.Fatal("rows far below the viewport must not be built")
		}
		// Keyboard: End jumps to the last option and the list follows.
		d.Key(input.KeyEnd, 0)
		d.Settle()
		if _, ok := d.Find("Item 099999"); !ok {
			t.Fatal("highlight did not scroll the last option into view")
		}
		d.Key(input.KeyUp, 0)
		d.Key(input.KeyEnter, 0)
		if val != "99998" || w.IsOpen() {
			t.Fatalf("Enter picked %q open=%v", val, w.IsOpen())
		}
		// Filtering.
		d.ClickLabel("pick")
		d.Type("12345")
		if _, ok := d.Find("Item 012345"); !ok {
			t.Fatal("filtered option missing")
		}
		if _, ok := d.Find("Item 000000"); ok {
			t.Fatal("non-matching option still shown")
		}
		d.ClickLabel("Item 012345")
		if val != "12345" {
			t.Fatalf("click picked %q", val)
		}
	})
}

func TestVirtualSelectMultipleLimitCreateGroups(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var vals []string
		created := ""
		groups := []VirtualOptionGroup{
			{Label: "Fruits", Options: Opts("Apple", "Banana")},
			{Label: "Veggies", Options: Opts("Carrot", "Daikon")},
		}
		d := fillDriver(th, func(c *Context) {
			w := VirtualSelect(c, nil, nil).Groups(groups).Multiple(&vals).MaxCount(2).AllowCreate().Placeholder("multi").Width(300)
			if v, ok := w.Created(); ok {
				created = v
			}
		})
		d.ClickLabel("multi")
		if _, ok := d.FindText("Fruits"); !ok {
			t.Fatal("group title missing")
		}
		d.ClickLabel("Apple")
		d.ClickLabel("Carrot")
		if len(vals) != 2 {
			t.Fatalf("vals = %v", vals)
		}
		d.ClickLabel("Banana") // over the limit: ignored
		if len(vals) != 2 {
			t.Fatalf("limit not enforced: %v", vals)
		}
		d.ClickLabel("Apple") // toggle off
		if len(vals) != 1 || vals[0] != "Carrot" {
			t.Fatalf("toggle off: %v", vals)
		}
		d.Type("Elder")
		d.Key(input.KeyEnter, 0)
		if created != "Elder" || len(vals) != 2 || vals[1] != "Elder" {
			t.Fatalf("create: created=%q vals=%v", created, vals)
		}
		// The created entry keeps its label as a tag.
		if _, ok := d.Find("Elder"); !ok {
			t.Fatal("created tag missing")
		}
	})
}

func TestVirtualSelectRemoteDebounce(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		var queries []string
		var opts []Option
		loading := false
		d := fillDriver(th, func(c *Context) {
			VirtualSelect(c, &val, opts).Placeholder("remote").Remote(func(q string) {
				queries = append(queries, q)
				opts = Opts(q + " result")
			}).Loading(loading).Width(240)
		})
		d.ClickLabel("remote")
		d.Type("a")
		d.Type("b")
		if len(queries) != 0 {
			t.Fatalf("remote called before the debounce: %v", queries)
		}
		d.Advance(350 * time.Millisecond)
		d.Settle()
		if len(queries) != 1 || queries[0] != "ab" {
			t.Fatalf("queries = %v", queries)
		}
		d.Settle()
		d.ClickLabel("ab result")
		if val != "ab result" {
			t.Fatalf("val = %q", val)
		}
	})
}

func TestVirtualSelectCustomRender(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		val := ""
		d := fillDriver(th, func(c *Context) {
			VirtualSelect(c, &val, Opts("One", "Two")).Placeholder("custom").OptionRender(func(o Option, s VirtualOptionState) {
				Text(c, "custom:"+o.Label)
			}).Width(240)
		})
		d.ClickLabel("custom")
		if _, ok := d.FindText("custom:Two"); !ok {
			t.Fatal("custom content missing")
		}
		d.ClickLabel("Two")
		if val != "Two" {
			t.Fatalf("val = %q", val)
		}
	})
}

// BenchmarkVirtualSelect100k measures a frame with the 100k-option
// dropdown open and the list scrolled.
func BenchmarkVirtualSelect100k(b *testing.B) {
	opts := fillOpts(100000)
	val := ""
	d := NewDriver(func(c *Context) {
		VirtualSelect(c, &val, opts).Placeholder("pick").Filterable().Width(240)
	}, DriverOptions{Width: 900, Height: 700})
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
