package ui

import (
	"strconv"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func perfEntryString(c *Context, key, initial string) *string {
	return State[string](c, hashID("perf.entry."+key), func() string { return initial })
}

func perfEntryDate(c *Context, key string, initial time.Time) *time.Time {
	return State[time.Time](c, hashID("perf.entry."+key), func() time.Time { return initial })
}

func perfEntryClickLabel(r *perfRun, label string) {
	if err := r.D.ClickLabel(label); err == nil {
		return
	}
	if box, ok := r.D.FindText(label); ok {
		r.D.Click(box.Center())
		return
	}
	panic("perf: entry target not found: " + label)
}

func perfEntryOpenClose(r *perfRun, i int, label string) {
	if i%2 == 0 {
		if box, ok := r.D.FindText(label); ok {
			r.D.Click(box.Center())
		} else {
			r.D.Click(r.Target.Center())
		}
		return
	}
	r.D.Key(input.KeyEscape, 0)
	r.D.Click(perfOutside)
	r.D.r.c.focus = 0
	r.D.r.Invalidate()
	r.D.Settle()
}
func perfEntryLargeOpen(r *perfRun, i int) {
	if i == 0 {
		r.D.Click(r.Center())
		r.D.Type("o")
		return
	}
	perfScroll(r, i)
}
func perfEntryDrag(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.FileDragOver(r.Center())
	} else {
		r.D.FileDragLeave()
	}
}

func perfEntryColorOpenClose(r *perfRun, i int) {
	if i%2 == 1 {
		r.D.Key(input.KeyEscape, 0)
		r.D.Click(perfOutside)
		return
	}
	r.D.Click(r.Target.Center())
}

func perfEntryFocusAt(r *perfRun, i int, fx, fy float32) {
	if i%2 == 0 {
		r.D.Click(r.At(fx, fy))
	} else {
		r.D.Click(perfOutside)
	}
}
func perfEntryHoverText(r *perfRun, i int, label string) {
	if i%2 == 1 {
		r.D.Move(perfOutside)
		return
	}
	box, ok := r.D.FindText(label)
	if !ok {
		panic("perf: hover target not found: " + label)
	}
	r.D.Move(box.Center())
}

func perfEntryHover(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Move(r.Center())
	} else {
		r.D.Move(perfOutside)
	}
}

func perfEntryTextLabel(r *perfRun, i int, label string) {
	box, ok := r.D.FindText(label)
	if !ok {
		panic("perf: text target not found: " + label)
	}
	if i%2 == 0 {
		r.D.Click(box.Center())
		r.D.Type("a")
	} else {
		r.D.Key(input.KeyBackspace, 0)
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

func perfEntryMarkdownBuild(c *Context) {
	value := State[string](c, hashID("perf.entry.markdown"), func() string { return "# Rules\n\nEdit me" })
	MarkdownEditor(c, value).Mode("edit").Height(280).Layout(layout.W(1000))
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

func TestPerfEntryInteractionEvents(t *testing.T) {
	var checked bool
	d := newPerfDriver(func(c *Context) { Checkbox(c, "事件复选框", &checked) }, false)
	d.Settle()
	if err := d.ClickLabel("事件复选框"); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("checkbox interaction did not update its bound value")
	}

	var query string
	d2 := newPerfDriver(func(c *Context) { AutoComplete(c, &query, Opts("alpha", "beta")).Placeholder("搜索候选") }, false)
	d2.Settle()
	d2.Click(perfTarget(d2).Center())
	d2.Type("a")
	if _, ok := d2.Find("alpha"); !ok {
		t.Fatal("autocomplete interaction did not render candidates")
	}
}

func TestPerfLargeEntryLists(t *testing.T) {
	for _, name := range []string{"AutoComplete", "Mentions"} {
		fx := perfRegistry[name]
		d := newPerfDriver(perfPage(fx.Large(1000)), false)
		d.Settle()
		r := &perfRun{D: d}
		perfStep(r, fx.LargeInteract, 0)
		if _, ok := d.Find("option-1000"); !ok {
			if _, textOK := d.FindText("option-1000"); !textOK {
				t.Fatalf("%s Large1000 did not render its candidate list", name)
			}
		}
	}
}

func init() {
	registerPerf(
		perfFixture{Name: "InputNumber", Build: func(c *Context) {
			v := State[float64](c, hashID("perf.entry.number"), func() float64 { return 42 })
			InputNumber(c, v).A11yLabel("数量").Width(220)
		}, Interact: perfEntryHover},
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
			v := State[string](c, hashID("perf.entry.otp"), func() string { return "1234" })
			OTP(c, v, 6).A11yLabel("验证码")
		}, Interact: func(r *perfRun, i int) { perfEntryFocusAt(r, i, 0.1, 0.5) }},
		perfFixture{Name: "AutoComplete", Build: func(c *Context) {
			AutoComplete(c, perfEntryString(c, "autocomplete", ""), perfEntryOptions()).Width(360)
		}, Interact: perfEntryText,
			Large: func(n int) func(*Context) {
				opts := perfEntryLargeOptions(n)
				return func(c *Context) {
					AutoComplete(c, perfEntryString(c, "autocomplete.large", ""), opts).Width(360)
				}
			}, LargeInteract: perfEntryLargeOpen},
		perfFixture{Name: "Mentions", Build: func(c *Context) { Mentions(c, perfEntryString(c, "mentions", "@Tango"), perfEntryOptions()).Width(360) }, Interact: perfEntryText,
			Large: func(n int) func(*Context) {
				opts := perfEntryLargeOptions(n)
				return func(c *Context) {
					Mentions(c, perfEntryString(c, "mentions.large", "@"), opts).Width(360)
				}
			}, LargeInteract: perfEntryLargeOpen},
		perfFixture{Name: "Checkbox", Covers: []string{"CheckboxGroup", "CheckAll"}, Build: func(c *Context) {
			v := State[bool](c, hashID("perf.entry.checkbox"), func() bool { return false })
			Checkbox(c, "同意条款", v)
			selected := State[[]string](c, hashID("perf.entry.checkbox.selected"), func() []string { return []string{"alpha"} })
			CheckboxGroup(c, selected, perfEntryOptions()).Buttons()
			CheckAll(c, selected, perfEntryOptions())
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "同意条款")
			r.D.Click(perfOutside)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				opts := perfEntryLargeOptions(n)
				selected := []string{}
				CheckboxGroup(c, &selected, opts)
				CheckAll(c, &selected, opts)
			}
		}}, perfFixture{Name: "Radio", Covers: []string{"RadioGroup"}, Build: func(c *Context) {
			Radio(c, "备选方案", true)
			value := State[string](c, hashID("perf.entry.radio"), func() string { return "alpha" })
			RadioGroup(c, value, perfEntryOptions()).Buttons()
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "beta")
			} else {
				perfEntryClickLabel(r, "alpha")
			}
			r.D.Click(perfOutside)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				opts := perfEntryLargeOptions(n)
				value := opts[0].Value
				RadioGroup(c, &value, opts)
			}
		}},
		perfFixture{Name: "Switch", Covers: []string{"SwitchValue"}, Build: func(c *Context) {
			v := State[bool](c, hashID("perf.entry.switch"), func() bool { return false })
			Switch(c, v).OuterTexts("启用", "停用")
			value := State[string](c, hashID("perf.entry.switch.value"), func() string { return "off" })
			SwitchValue(c, value, "on", "off").OuterTexts("ON", "OFF")
		}, Interact: func(r *perfRun, _ int) {
			r.D.Click(r.At(0.5, 0.25))
			r.D.Click(perfOutside)
		}},
		perfFixture{Name: "Toggle", Covers: []string{"ToggleGroup"}, Build: func(c *Context) {
			v := State[bool](c, hashID("perf.entry.toggle"), func() bool { return false })
			Toggle(c, "重点", v)
			selected := State[[]string](c, hashID("perf.entry.toggle.selected"), func() []string { return []string{"alpha"} })
			ToggleGroup(c, selected, perfEntryOptions())
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "重点")
			r.D.Click(perfOutside)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				opts := perfEntryLargeOptions(n)
				selected := []string{opts[0].Value}
				ToggleGroup(c, &selected, opts)
			}
		}},
		perfFixture{Name: "Slider", Covers: []string{"SliderRange"}, Build: func(c *Context) {
			v := State[float64](c, hashID("perf.entry.slider"), func() float64 { return 30 })
			Slider(c, v, 0, 100).Marks(map[float64]string{0: "0", 100: "100"})
			lo := State[float64](c, hashID("perf.entry.slider.lo"), func() float64 { return 20 })
			hi := State[float64](c, hashID("perf.entry.slider.hi"), func() float64 { return 80 })
			SliderRange(c, lo, hi, 0, 100)
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				r.D.Move(r.Center())
			} else {
				r.D.Move(perfOutside)
			}
		}},
		perfFixture{Name: "ColorPicker", Covers: []string{"ColorPickerGradient"}, Build: func(c *Context) {
			col := State[theme.Color](c, hashID("perf.entry.color"), func() theme.Color { return theme.Hex("#1677ff") })
			ColorPicker(c, col).ShowText()
			grad := State[theme.Gradient](c, hashID("perf.entry.gradient"), func() theme.Gradient {
				return theme.Gradient{Angle: 90, Stops: []theme.GradientStop{{Offset: 0, Color: theme.Hex("#1677ff")}, {Offset: 1, Color: theme.Hex("#722ed1")}}}
			})
			ColorPickerGradient(c, grad).ShowText()
		}, Interact: perfEntryColorOpenClose},
		perfFixture{Name: "ColorPickerPanel", Covers: []string{"ColorPickerPanelGradient"}, Build: func(c *Context) {
			col := State[theme.Color](c, hashID("perf.entry.panel.color"), func() theme.Color { return theme.Hex("#13c2c2") })
			ColorPickerPanel(c, col).Presets(theme.Hex("#1677ff"), theme.Hex("#13c2c2"))
			grad := State[theme.Gradient](c, hashID("perf.entry.panel.gradient"), func() theme.Gradient {
				return theme.Gradient{Angle: 45, Stops: []theme.GradientStop{{Offset: 0, Color: theme.Hex("#fa8c16")}, {Offset: 1, Color: theme.Hex("#f5222d")}}}
			})
			ColorPickerPanelGradient(c, grad).NoAlpha()
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "#1677FF")
			} else {
				perfEntryClickLabel(r, "#13C2C2")
			}
			r.D.Click(perfOutside)
		}},
		perfFixture{Name: "DatePicker", Build: func(c *Context) {
			d := State[time.Time](c, hashID("perf.entry.date"), func() time.Time { return time.Time{} })
			DatePicker(c, d).Placeholder("选择日期")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "选择日期") }},
		perfFixture{Name: "DatePickerPanel", Build: func(c *Context) {
			d := State[time.Time](c, hashID("perf.entry.date.panel"), func() time.Time { return t0() })
			DatePickerPanel(c, d)
		}, Interact: perfEntryHover}, perfFixture{Name: "RangePicker", Covers: []string{"MultiDatePicker", "DateTimePicker"}, Build: func(c *Context) {
			r := State[[2]time.Time](c, hashID("perf.entry.range"), func() [2]time.Time { return [2]time.Time{t0(), t1()} })
			RangePicker(c, r).Placeholder("开始日期", "结束日期")
			multi := State[[]time.Time](c, hashID("perf.entry.multi"), func() []time.Time { return nil })
			MultiDatePicker(c, multi)
			d := State[time.Time](c, hashID("perf.entry.datetime"), func() time.Time { return t0() })
			DateTimePicker(c, d).Placeholder("选择日期时间")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "开始日期") }},
		perfFixture{Name: "TimePicker", Build: func(c *Context) {
			t := State[time.Time](c, hashID("perf.entry.time"), func() time.Time { return time.Time{} })
			TimePicker(c, t).Placeholder("选择时间")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "选择时间") }},
		perfFixture{Name: "TimePanel", Build: func(c *Context) {
			t := State[time.Time](c, hashID("perf.entry.time.panel"), func() time.Time { return t0() })
			TimePanel(c, t)
		}, Interact: perfEntryHover},
		perfFixture{Name: "TimeRangePicker", Build: func(c *Context) {
			r := State[[2]time.Time](c, hashID("perf.entry.timerange"), func() [2]time.Time { return [2]time.Time{t0(), t1()} })
			TimeRangePicker(c, r).Placeholder("开始时刻", "结束时刻")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "开始时刻") }},
		perfFixture{Name: "TimeSelect", Build: func(c *Context) {
			ts := State[string](c, hashID("perf.entry.timeselect"), func() string { return "09:30" })
			TimeSelect(c, ts).Placeholder("选择时刻")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "选择时刻") }}, perfFixture{Name: "Upload", Covers: []string{"DropZone"}, Build: func(c *Context) {
			files := State[[]UploadFile](c, hashID("perf.entry.files"), func() []UploadFile { return []UploadFile{{Name: "rules.json", Size: 128, Status: "done"}} })
			Upload(c, files).Dragger().Width(520)
			DropZone(c, func(over bool) { Upload(c, files).Dragger().DragOver(over).Width(520) }).Layout(layout.Size(520, 120))
		}, Interact: perfEntryDrag},
		perfFixture{Name: "Form", Covers: []string{"FieldSet"}, Build: func(c *Context) {
			f := Form(c).Layout(layout.W(620))
			vals := State[ProValues](c, hashID("perf.entry.form.values"), func() ProValues { return ProValues{"budget": []any{float64(1), float64(2)}} })
			FieldSet(c, f, ProFieldSet{Key: "budget", Label: "预算", Parts: []ProField{{ValueType: ValueDigit}, {ValueType: ValueDigit}}, Separator: "~"}, *vals)
			value := State[string](c, hashID("perf.entry.form.value"), func() string { return "规则" })
			f.Item("规则名称").Required().Body(func() { Input(c, value).A11yLabel("规则名称") })
		}, Interact: func(r *perfRun, i int) { perfEntryFocusAt(r, i, 0.5, 0.85) }},
		perfFixture{Name: "ItemGroup", Covers: []string{"Item"}, Build: func(c *Context) {
			ItemGroup(c).Bordered().Body(func() {
				Item(c, "安全设置").Description("双重验证已开启").Clickable()
				Item(c, "通知设置").Description("邮件与站内信").Clickable()
			})
		}, Interact: perfEntryHover},
		perfFixture{Name: "Addon", Build: func(c *Context) {
			label := State[string](c, hashID("perf.entry.addon"), func() string { return "tango" })
			a := Addon(c, "https://")
			a.Clicked()
			Row(c, func() { Input(c, label); Addon(c, ".com") })
		}, Interact: func(r *perfRun, i int) { perfEntryHoverText(r, i, "https://") }},
		perfFixture{Name: "HotkeyInput", Build: func(c *Context) {
			v := State[string](c, hashID("perf.entry.hotkey"), func() string { return "CmdOrCtrl+K" })
			HotkeyInput(c, v).Width(260)
		}, Interact: perfEntryHover},
		perfFixture{Name: "MarkdownEditor", Build: func(c *Context) {
			perfEntryMarkdownBuild(c)
		}, Interact: perfEntryText},
	)
}

func t0() time.Time { return time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC) }
func t1() time.Time { return time.Date(2026, 11, 16, 17, 0, 0, 0, time.UTC) }
