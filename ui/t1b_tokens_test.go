package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Component tokens of the date / time pickers, Calendar, ColorPicker,
// Upload, Rate, InputNumber and Form (round 2, group T1b).

var t1bMark = theme.Hex("#12ab34")

// t1bCount counts fill, stroke, path and text ops painted in col.
func t1bCount(d *Driver, col theme.Color) int {
	n := 0
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == col {
				n++
			}
		case paint.StrokeRect:
			if o.Color == col {
				n++
			}
		case paint.FillPath:
			if o.Color == col {
				n++
			}
		case paint.Text:
			if o.Color == col {
				n++
			}
		}
	}
	return n
}

type t1bCase struct {
	name string
	cs   theme.Components
	def  func(th *theme.Theme) theme.Color // the derived value of the overridden field
	fn   func(c *Context)                  // builds one instance
}

func t1bCases() []t1bCase {
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	return []t1bCase{
		{"DatePickerPanel", theme.Components{DatePicker: theme.DatePickerToken{CellActiveBg: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.DatePicker.CellActiveBg },
			func(c *Context) { v := day; DatePickerPanel(c, &v) }},
		{"DatePicker popup", theme.Components{DatePicker: theme.DatePickerToken{PanelBg: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.DatePicker.PanelBg },
			func(c *Context) { v := day; DatePicker(c, &v).Open() }},
		{"TimePanel", theme.Components{DatePicker: theme.DatePickerToken{TimeCellActiveColor: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.DatePicker.TimeCellActiveColor },
			func(c *Context) { v := day.Add(10 * time.Hour); TimePanel(c, &v) }},
		{"Calendar", theme.Components{Calendar: theme.CalendarToken{DayActiveBg: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.Calendar.DayActiveBg },
			func(c *Context) { v := day; Calendar(c, &v).Card().Layout(layout.W(300)) }},
		{"ColorPicker", theme.Components{ColorPicker: theme.ColorPickerToken{TriggerSwatchBorder: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.ColorPicker.TriggerSwatchBorder },
			func(c *Context) { v := theme.Hex("#1677ff"); ColorPicker(c, &v) }},
		{"Upload", theme.Components{Upload: theme.UploadToken{PictureCardBg: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.Upload.PictureCardBg },
			func(c *Context) { f := []UploadFile{{Name: "a.png", Status: "done"}}; Upload(c, &f).PictureCard() }},
		{"Rate", theme.Components{Rate: theme.RateToken{StarColor: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.Rate.StarColor },
			func(c *Context) { v := 3.0; Rate(c, &v) }},
		{"InputNumber", theme.Components{InputNumber: theme.InputNumberToken{SideHandleBg: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.InputNumber.SideHandleBg },
			func(c *Context) { v := 1.0; InputNumber(c, &v).ControlsPosition("sides") }},
		{"Form", theme.Components{Form: theme.FormToken{LabelRequiredMarkColor: t1bMark}},
			func(th *theme.Theme) theme.Color { return th.Components.Form.LabelRequiredMarkColor },
			func(c *Context) {
				var s string
				f := Form(c)
				f.Body(func() { f.Item("Name").Required().Body(func() { Input(c, &s) }) })
			}},
	}
}

// TestT1bTokenOverridesAreScoped overrides one token per component in a
// nested ConfigProvider: the instance inside paints it, the instance
// outside keeps the derived value, and nothing paints it without the
// provider.
func TestT1bTokenOverridesAreScoped(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		for _, tc := range t1bCases() {
			t.Run(th.Flavor.String()+"/"+tc.name, func(t *testing.T) {
				d := NewDriver(func(c *Context) {
					Row(c, func() {
						ConfigProvider(c).Components(tc.cs).Body(func() { tc.fn(c) })
						tc.fn(c)
					}, layout.Gap(40), layout.Pad(20))
				}, DriverOptions{Width: 1600, Height: 900, Theme: th})
				d.Settle()
				d.Settle()
				if n := t1bCount(d, t1bMark); n == 0 {
					t.Errorf("override not painted inside the scope")
				}
				if n := t1bCount(d, tc.def(th)); n == 0 {
					t.Errorf("default %v not painted outside the scope", tc.def(th))
				}
				plain := NewDriver(func(c *Context) {
					Row(c, func() { tc.fn(c) }, layout.Pad(20))
				}, DriverOptions{Width: 1600, Height: 900, Theme: th})
				plain.Settle()
				plain.Settle()
				if n := t1bCount(plain, t1bMark); n != 0 {
					t.Errorf("mark painted %d times without the provider", n)
				}
			})
		}
	}
}

// TestT1bDerivedDefaults: the derived tokens equal the flat values the
// components painted before (spot checks; the full pixel comparison ran
// during the migration).
func TestT1bDerivedDefaults(t *testing.T) {
	for _, th := range []*theme.Theme{
		theme.AntDesign(), theme.AntDesign(theme.Dark()), theme.ElementPlus(), theme.ElementPlus(theme.Dark()),
		theme.Shadcn(), theme.Shadcn(theme.Dark()),
	} {
		k := th.Components
		el := th.Flavor == theme.Element
		shad := th.Flavor == theme.StyleShadcn
		check := func(name string, got, want any) {
			if got != want {
				t.Errorf("%v dark=%v %s = %v, want %v", th.Flavor, th.Dark, name, got, want)
			}
		}
		check("Rate.StarColor", k.Rate.StarColor, th.RateStar)
		check("Rate.StarBg", k.Rate.StarBg, th.RateStarEmpty)
		check("DatePicker.CellActiveBg", k.DatePicker.CellActiveBg, th.Primary.Base)
		check("DatePicker.PanelBg", k.DatePicker.PanelBg, th.BgElevated)
		check("Calendar.ItemActiveBg", k.Calendar.ItemActiveBg, th.Primary.Bg)
		check("ColorPicker.ClearColor", k.ColorPicker.ClearColor, th.Error.Base)
		check("Upload.ProgressColor", k.Upload.ProgressColor, th.Primary.Base)
		check("InputNumber.HandleBorderColor", k.InputNumber.HandleBorderColor, th.Border)
		check("Form.ErrorColor", k.Form.ErrorColor, th.Error.Base)
		check("Form.LabelHeight", k.Form.LabelHeight, th.ControlHeight)
		w, h := float32(36), float32(32)
		if el {
			w, h = 40, 36
		} else if shad {
			w, h = 32, 32
		}
		check("DatePicker.CellWidth", k.DatePicker.CellWidth, w)
		check("DatePicker.CellHeight", k.DatePicker.CellHeight, h)
		size := float32(102)
		if el {
			size = 148
		}
		check("Upload.PictureCardSize", k.Upload.PictureCardSize, size)
	}
}

// TestT1bAliasRecolorsPanel: a DatePicker Alias (brand color) reaches the
// selected day of the panel built in its scope.
func TestT1bAliasRecolorsPanel(t *testing.T) {
	th := theme.AntDesign()
	brand := theme.Hex("#00b96b")
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	cs := theme.Components{DatePicker: theme.DatePickerToken{Alias: theme.Alias{ColorPrimary: brand}}}
	want := th.Override(cs).For(theme.CompDatePicker).Primary.Base
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Components(cs).Body(func() {
			v := day
			DatePickerPanel(c, &v)
		})
	}, DriverOptions{Width: 600, Height: 600, Theme: th})
	d.Settle()
	if n := t1bCount(d, want); n == 0 {
		t.Errorf("selected day not painted in the Alias primary %v", want)
	}
	if n := t1bCount(d, th.Primary.Base); n != 0 {
		t.Errorf("selected day still painted in the global primary (%d ops)", n)
	}
}
