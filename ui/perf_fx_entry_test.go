package ui

import (
	"strconv"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func perfEntryBool(c *Context, key string) *bool {
	return State[bool](c, hashID("perf.entry."+key), nil)
}

func perfEntryString(c *Context, key, initial string) *string {
	return State[string](c, hashID("perf.entry."+key), func() string { return initial })
}

func perfEntryDate(c *Context, key string, initial time.Time) *time.Time {
	return State[time.Time](c, hashID("perf.entry."+key), func() time.Time { return initial })
}

func perfEntryClick(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Click(r.Center())
	} else {
		r.D.Key(input.KeyEscape, 0)
		r.D.Click(perfOutside)
	}
}

func perfEntryText(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Click(r.Center())
		r.D.Type("a")
	} else {
		r.D.Key(input.KeyBackspace, 0)
		r.D.Click(perfOutside)
	}
}

func perfEntryStateChange(r *perfRun, i int) {
	v := perfEntryBool(r.D.r.c, "changed")
	*v = i%2 == 0
	r.D.r.Invalidate()
	r.D.Frame()
}

func perfEntryMarkdownChange(r *perfRun, i int) {
	perfEntryStateChange(r, i)
	if i%2 == 1 {
		r.D.Settle()
	}
}

func perfEntryMarkdownBuild(c *Context, changed bool) {
	value := State[string](c, hashID("perf.entry.markdown"), func() string { return "# Rules\n\nEdit me" })
	MarkdownEditor(c, value).Mode("edit").Height(280).Layout(layout.W(1000))
	if changed {
		Text(c, "编辑状态 Updated")
	} else {
		Text(c, "编辑状态 Ready")
	}
}

func perfEntryOptions() []Option {
	return Opts("alpha", "beta", "gamma")
}

func perfEntryTags(n int) []string {
	values := make([]string, n)
	for i := range values {
		values[i] = "tag-" + strconv.Itoa(i+1)
	}
	return values
}

func perfEntryLargeOptions(n int) []Option {
	opts := make([]Option, n)
	for i := range opts {
		s := "option-" + strconv.Itoa(i+1)
		opts[i] = Option{Label: s, Value: s}
	}
	return opts
}

func init() {
	registerPerf(
		perfFixture{Name: "InputNumber", Build: func(c *Context) {
			v := 42.0
			if *perfEntryBool(c, "changed") {
				v = 43
			}
			InputNumber(c, &v).Width(220)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "InputTag", Build: func(c *Context) {
			values := perfEntryTags(3)
			InputTag(c, &values).Width(360)
		}, Interact: perfEntryText,
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					values := perfEntryTags(n)
					InputTag(c, &values).Width(520)
				}
			}},
		perfFixture{Name: "TextArea", Build: func(c *Context) {
			TextArea(c, perfEntryString(c, "textarea", "多行输入 TextArea")).ShowCount().Layout(layout.Size(420, 100))
		}, Interact: perfEntryText},
		perfFixture{Name: "Search", Build: func(c *Context) { Search(c, perfEntryString(c, "search", "搜索规则")) }, Interact: perfEntryText},
		perfFixture{Name: "OTP", Build: func(c *Context) {
			v := "1234"
			if *perfEntryBool(c, "changed") {
				v = "5678"
			}
			OTP(c, &v, 6)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "AutoComplete", Build: func(c *Context) {
			AutoComplete(c, perfEntryString(c, "autocomplete", ""), perfEntryOptions()).Width(360)
		}, Interact: perfEntryText,
			Large: func(n int) func(*Context) {
				opts := perfEntryLargeOptions(n)
				return func(c *Context) { AutoComplete(c, perfEntryString(c, "autocomplete.large", ""), opts).Width(360) }
			}},
		perfFixture{Name: "Mentions", Build: func(c *Context) { Mentions(c, perfEntryString(c, "mentions", "@Tango"), perfEntryOptions()).Width(360) }, Interact: perfEntryText,
			Large: func(n int) func(*Context) {
				opts := perfEntryLargeOptions(n)
				return func(c *Context) { Mentions(c, perfEntryString(c, "mentions.large", "@"), opts).Width(360) }
			}},
		perfFixture{Name: "Checkbox", Covers: []string{"CheckboxGroup", "CheckAll"}, Build: func(c *Context) {
			v := *perfEntryBool(c, "changed")
			Checkbox(c, "同意条款", &v)
			selected := State[[]string](c, hashID("perf.entry.checkbox.selected"), func() []string { return []string{"alpha"} })
			CheckboxGroup(c, selected, perfEntryOptions()).Buttons()
			CheckAll(c, selected, perfEntryOptions())
		}, Interact: perfEntryStateChange,
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					opts := perfEntryLargeOptions(n)
					selected := []string{}
					CheckboxGroup(c, &selected, opts)
					CheckAll(c, &selected, opts)
				}
			}},
		perfFixture{Name: "Radio", Covers: []string{"RadioGroup"}, Build: func(c *Context) {
			changed := *perfEntryBool(c, "changed")
			Radio(c, "备选方案", !changed)
			value := "alpha"
			if changed {
				value = "beta"
			}
			RadioGroup(c, perfEntryString(c, "radio", value), perfEntryOptions()).Buttons()
		}, Interact: perfEntryStateChange,
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					opts := perfEntryLargeOptions(n)
					value := opts[0].Value
					RadioGroup(c, &value, opts)
				}
			}},
		perfFixture{Name: "Switch", Covers: []string{"SwitchValue"}, Build: func(c *Context) {
			changed := *perfEntryBool(c, "changed")
			Switch(c, &changed).Texts("开", "关")
			v := "off"
			if changed {
				v = "on"
			}
			SwitchValue(c, &v, "on", "off").Texts("ON", "OFF")
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "Toggle", Covers: []string{"ToggleGroup"}, Build: func(c *Context) {
			changed := *perfEntryBool(c, "changed")
			Toggle(c, "重点", &changed)
			selected := []string{"alpha"}
			ToggleGroup(c, &selected, perfEntryOptions())
		}, Interact: perfEntryStateChange,
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					opts := perfEntryLargeOptions(n)
					selected := []string{opts[0].Value}
					ToggleGroup(c, &selected, opts)
				}
			}},
		perfFixture{Name: "Slider", Covers: []string{"SliderRange"}, Build: func(c *Context) {
			changed := *perfEntryBool(c, "changed")
			v := 30.0
			if changed {
				v = 60
			}
			Slider(c, &v, 0, 100).Marks(map[float64]string{0: "0", 100: "100"})
			lo, hi := 20.0, 80.0
			if changed {
				lo, hi = 40, 90
			}
			SliderRange(c, &lo, &hi, 0, 100)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "ColorPicker", Covers: []string{"ColorPickerGradient"}, Build: func(c *Context) {
			col := State[theme.Color](c, hashID("perf.entry.color"), func() theme.Color { return theme.Hex("#1677ff") })
			ColorPicker(c, col).ShowText()
			grad := State[theme.Gradient](c, hashID("perf.entry.gradient"), func() theme.Gradient {
				return theme.Gradient{Angle: 90, Stops: []theme.GradientStop{{Offset: 0, Color: theme.Hex("#1677ff")}, {Offset: 1, Color: theme.Hex("#722ed1")}}}
			})
			ColorPickerGradient(c, grad).ShowText()
		}, Interact: perfEntryClick},
		perfFixture{Name: "ColorPickerPanel", Covers: []string{"ColorPickerPanelGradient"}, Build: func(c *Context) {
			changed := *perfEntryBool(c, "changed")
			col := theme.Hex("#13c2c2")
			ColorPickerPanel(c, &col).Presets(theme.Hex("#1677ff"), theme.Hex("#52c41a")).Disabled(changed)
			grad := theme.Gradient{Angle: 45, Stops: []theme.GradientStop{{Offset: 0, Color: theme.Hex("#fa8c16")}, {Offset: 1, Color: theme.Hex("#f5222d")}}}
			ColorPickerPanelGradient(c, &grad).NoAlpha().Disabled(changed)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "DatePicker", Covers: []string{"DatePickerPanel"}, Build: func(c *Context) {
			d := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
			if *perfEntryBool(c, "changed") {
				d = d.AddDate(0, 0, 1)
			}
			DatePicker(c, &d)
			DatePickerPanel(c, &d)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "RangePicker", Covers: []string{"MultiDatePicker", "DateTimePicker"}, Build: func(c *Context) {
			changed := *perfEntryBool(c, "changed")
			start, end := t0(), t1()
			if changed {
				start, end = start.AddDate(0, 0, 1), end.AddDate(0, 0, 1)
			}
			r := State[[2]time.Time](c, hashID("perf.entry.range"), func() [2]time.Time { return [2]time.Time{start, end} })
			*r = [2]time.Time{start, end}
			RangePicker(c, r)
			multi := State[[]time.Time](c, hashID("perf.entry.multi"), func() []time.Time { return nil })
			MultiDatePicker(c, multi)
			d := perfEntryDate(c, "datetime", start)
			*d = start
			DateTimePicker(c, d)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "TimePicker", Covers: []string{"TimePanel", "TimeRangePicker", "TimeSelect"}, Build: func(c *Context) {
			t := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
			if *perfEntryBool(c, "changed") {
				t = time.Date(2026, 11, 16, 17, 45, 0, 0, time.UTC)
			}
			TimePicker(c, &t)
			TimePanel(c, &t)
			r := [2]time.Time{t0(), t1()}
			TimeRangePicker(c, &r)
			ts := "09:30"
			TimeSelect(c, &ts)
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "Upload", Covers: []string{"DropZone"}, Build: func(c *Context) {
			files := []UploadFile{{Name: "rules.json", Size: 128, Status: "done"}}
			changed := *perfEntryBool(c, "changed")
			Upload(c, &files).Dragger().DragOver(changed).Width(520)
			DropZone(c, func(over bool) { Upload(c, &files).Dragger().DragOver(over || changed).Width(520) }).Layout(layout.Size(520, 120))
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "Form", Covers: []string{"FieldSet"}, Build: func(c *Context) {
			f := Form(c).Layout(layout.W(620))
			vals := ProValues{"budget": []any{float64(1), float64(2)}}
			FieldSet(c, f, ProFieldSet{Key: "budget", Label: "预算", Parts: []ProField{{ValueType: ValueDigit}, {ValueType: ValueDigit}}, Separator: "~"}, vals)
			value := "规则"
			if *perfEntryBool(c, "changed") {
				value = "更新后的规则"
			}
			f.Item("规则名称").Required().Body(func() { Input(c, &value) })
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "ItemGroup", Covers: []string{"Item"}, Build: func(c *Context) {
			ItemGroup(c).Bordered().Body(func() {
				Item(c, "安全设置").Description("双重验证已开启").Clickable()
				Item(c, "通知设置").Description("邮件与站内信").Clickable()
			})
		}, Interact: perfEntryClick},
		perfFixture{Name: "Addon", Build: func(c *Context) {
			label := "tango"
			if *perfEntryBool(c, "changed") {
				label = "updated"
			}
			Row(c, func() { Addon(c, "https://"); Input(c, &label); Addon(c, ".com") })
		}, Interact: perfEntryStateChange},
		perfFixture{Name: "HotkeyInput", Build: func(c *Context) { HotkeyInput(c, perfEntryString(c, "hotkey", "Ctrl+K")).Width(260) }, Interact: perfEntryClick},
		perfFixture{Name: "MarkdownEditor", Build: func(c *Context) {
			perfEntryMarkdownBuild(c, *perfEntryBool(c, "changed"))
		}, Interact: perfEntryMarkdownChange},
	)
}

func t0() time.Time { return time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC) }
func t1() time.Time { return time.Date(2026, 11, 16, 17, 0, 0, 0, time.UTC) }
