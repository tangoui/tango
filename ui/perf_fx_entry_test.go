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
	r.D.Settle()
}

func perfEntryTagEdit(r *perfRun, i int) {
	p := r.At(0.9, 0.5)
	r.D.Click(p)
	if i%2 == 0 {
		r.D.Type("new")
		r.D.Key(input.KeyEnter, 0)
	} else {
		r.D.Key(input.KeyBackspace, 0)
	}
	r.D.Click(perfOutside)
}

func perfEntryNumber(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Click(r.Center())
		r.D.Key(input.KeyEnd, 0)
		r.D.Type("1")
	} else {
		r.D.Key(input.KeyBackspace, 0)
		r.D.Click(perfOutside)
	}
}

func perfEntryOTP(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Click(r.At(0.1, 0.5))
		r.D.Type("5")
	} else {
		r.D.Key(input.KeyBackspace, 0)
		r.D.Click(perfOutside)
	}
}

func perfEntryHotkey(r *perfRun, i int) {
	r.D.Click(r.Center())
	if i%2 == 0 {
		r.D.Key(input.KeyP, input.ModCtrl)
	} else {
		r.D.Key(input.KeyK, input.ModCtrl)
	}
	r.D.Click(perfOutside)
}

func perfEntrySliderDrag(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Drag(r.At(0.1, 0.5), r.At(0.6, 0.5))
	} else {
		r.D.Click(r.Center())
		r.D.Key(input.KeyHome, 0)
		r.D.Click(perfOutside)
	}
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

func perfEntryRangeDrag(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Click(r.Center())
		r.D.Key(input.KeyEnd, 0)
	} else {
		r.D.Key(input.KeyHome, 0)
		r.D.Click(perfOutside)
	}
	r.D.Settle()
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

func perfEntryFieldSet(r *perfRun, i int) {
	if box, ok := r.D.FindText("1"); ok && i%2 == 0 {
		r.D.Click(box.Center())
	} else if i%2 == 0 {
		r.D.Click(r.Center())
	} else {
		r.D.Key(input.KeyEscape, 0)
		r.D.Click(perfOutside)
	}
	r.D.Settle()
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
		r.D.Settle()
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

func TestPerfHotkeyCapture(t *testing.T) {
	value := "Ctrl+K"
	d := newPerfDriver(func(c *Context) { HotkeyInput(c, &value) }, false)
	d.Settle()
	d.Click(perfTarget(d).Center())
	d.Key(input.KeyP, input.ModCtrl)
	if value == "Ctrl+K" {
		t.Fatal("hotkey capture did not update its bound value")
	}
	d.Click(perfTarget(d).Center())
	d.Key(input.KeyK, input.ModCtrl)
	if value != "Ctrl+K" {
		t.Fatalf("hotkey capture did not restore value: %q", value)
	}
}
func TestPerfEntrySliderDrag(t *testing.T) {
	value := 30.0
	d := newPerfDriver(func(c *Context) { Slider(c, &value, 0, 100) }, false)
	d.Settle()
	r := &perfRun{D: d}
	r.Target = perfTarget(d)
	d.Drag(r.At(0.3, 0.5), r.At(0.6, 0.5))
	if value == 30 {
		t.Fatal("slider drag did not change its bound value")
	}
}

func TestPerfEntryRetainedInputTag(t *testing.T) {
	fx := perfRegistry["InputTag"]
	d := newPerfDriver(perfPage(fx.Build), false)
	d.Settle()
	d.Click(perfTarget(d).Center())
	d.Type("new")
	d.Key(input.KeyEnter, 0)
	values := State[[]string](d.r.c, hashID("perf.entry.tags"), nil)
	if !contains(*values, "new") {
		t.Fatalf("fixture InputTag event produced %v", *values)
	}
	d.Frame()
	if !contains(*values, "new") {
		t.Fatalf("fixture InputTag value was overwritten on rebuild: %v", *values)
	}
	d.Click(perfTarget(d).Center())
	d.Key(input.KeyBackspace, 0)
	if contains(*values, "new") {
		t.Fatalf("fixture InputTag undo did not remove retained value: %v", *values)
	}
}

func TestPerfEntryDateTimeOwnPopup(t *testing.T) {
	value := t0()
	d := newPerfDriver(func(c *Context) { DateTimePicker(c, &value).Placeholder("选择日期时间") }, false)
	d.Settle()
	d.Click(perfTarget(d).Center())
	if _, ok := d.Find("确定"); !ok {
		t.Fatal("DateTimePicker did not open its own confirmation popup")
	}
	d.Key(input.KeyEscape, 0)
	if _, ok := d.Find("确定"); ok {
		t.Fatal("DateTimePicker popup did not close with Escape")
	}
}
func TestPerfEntryPopupOpenClose(t *testing.T) {
	tests := []struct {
		name, label, expected string
		build                 func(*Context) func() bool
	}{
		{"DatePicker", "选择日期", "2026-01", func(c *Context) func() bool {
			w := DatePicker(c, State[time.Time](c, hashID("popup.date"), func() time.Time { return time.Time{} })).Placeholder("选择日期")
			return w.IsOpen
		}},
		{"RangePicker", "开始日期", "2026-11-16", func(c *Context) func() bool {
			w := RangePicker(c, State[[2]time.Time](c, hashID("popup.range"), func() [2]time.Time { return [2]time.Time{t0(), t1()} })).Placeholder("开始日期", "结束日期")
			return w.IsOpen
		}},
		{"MultiDatePicker", "选择多个日期", "2026-01", func(c *Context) func() bool {
			w := MultiDatePicker(c, State[[]time.Time](c, hashID("popup.multi"), func() []time.Time { return nil })).Placeholder("选择多个日期")
			return w.IsOpen
		}},
		{"DateTimePicker", "选择日期时间", "确定", func(c *Context) func() bool {
			w := DateTimePicker(c, State[time.Time](c, hashID("popup.datetime"), func() time.Time { return t0() })).Placeholder("选择日期时间")
			return w.IsOpen
		}},
		{"TimePicker", "选择时间", "h00", func(c *Context) func() bool {
			w := TimePicker(c, State[time.Time](c, hashID("popup.time"), func() time.Time { return time.Time{} })).Placeholder("选择时间")
			return w.IsOpen
		}},
		{"TimeRangePicker", "开始时刻", "h09", func(c *Context) func() bool {
			w := TimeRangePicker(c, State[[2]time.Time](c, hashID("popup.timerange"), func() [2]time.Time { return [2]time.Time{t0(), t1()} })).Placeholder("开始时刻", "结束时刻")
			return w.IsOpen
		}},
		{"TimeSelect", "选择时刻", "09:30", func(c *Context) func() bool {
			w := TimeSelect(c, State[string](c, hashID("popup.timeselect"), func() string { return "09:30" })).Placeholder("选择时刻")
			return w.IsOpen
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var isOpen func() bool
			d := newPerfDriver(func(c *Context) { isOpen = tc.build(c) }, false)
			d.Settle()
			if box, ok := d.FindText(tc.label); ok {
				d.Click(box.Center())
			} else {
				d.Click(perfTarget(d).Center())
			}
			if !isOpen() {
				t.Fatal("popup did not open")
			}
			if _, ok := d.Find(tc.expected); !ok {
				if _, textOK := d.FindText(tc.expected); !textOK {
					t.Fatalf("popup did not render %q", tc.expected)
				}
			}
			d.Key(input.KeyEscape, 0)
			d.Settle()
			if isOpen() {
				t.Fatalf("popup remained open after close")
			}
		})
	}
}
func TestPerfLargeEntryLists(t *testing.T) {
	for _, n := range []int{1000, 100000} {
		for _, name := range []string{"AutoComplete", "Mentions"} {
			fx := perfRegistry[name]
			d := newPerfDriver(perfPage(fx.Large(n)), false)
			d.Settle()
			r := &perfRun{D: d}
			perfStep(r, fx.LargeInteract, 0)
			candidate := "option-" + strconv.Itoa(n)
			if _, ok := d.Find(candidate); !ok {
				if _, textOK := d.FindText(candidate); !textOK {
					t.Fatalf("%s Large%d did not render its candidate list", name, n)
				}
			}
		}
	}
}

func init() {
	registerPerf(
		perfFixture{Name: "InputNumber", Build: func(c *Context) {
			v := State[float64](c, hashID("perf.entry.number"), func() float64 { return 42 })
			InputNumber(c, v).A11yLabel("数量").Width(220)
		}, Interact: perfEntryNumber},
		perfFixture{Name: "InputTag", Build: func(c *Context) {
			values := State[[]string](c, hashID("perf.entry.tags"), func() []string { return []string{"one", "two", "three"} })
			InputTag(c, values).Width(360)
		}, Interact: perfEntryTagEdit,
			Large: func(n int) func(*Context) {
				return func(c *Context) {
					values := State[[]string](c, hashID("perf.entry.tags.large"), func() []string { return perfEntryTags(n) })
					InputTag(c, values).Width(520)
				}
			}},
		perfFixture{Name: "TextArea", Build: func(c *Context) {
			TextArea(c, perfEntryString(c, "textarea", "多行输入 TextArea")).ShowCount().Layout(layout.Size(420, 100))
		}, Interact: perfEntryText},
		perfFixture{Name: "Search", Build: func(c *Context) { Search(c, perfEntryString(c, "search", "搜索规则")) }, Interact: perfEntryText},
		perfFixture{Name: "OTP", Build: func(c *Context) {
			v := State[string](c, hashID("perf.entry.otp"), func() string { return "" })
			OTP(c, v, 6).A11yLabel("验证码")
		}, Interact: perfEntryOTP},
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
		perfFixture{Name: "Checkbox", Build: func(c *Context) {
			v := State[bool](c, hashID("perf.entry.checkbox"), func() bool { return false })
			Checkbox(c, "同意条款", v)
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "同意条款")
			r.D.Click(perfOutside)
		}},
		perfFixture{Name: "CheckboxGroup", Build: func(c *Context) {
			selected := State[[]string](c, hashID("perf.entry.checkbox.group"), func() []string { return []string{"alpha"} })
			CheckboxGroup(c, selected, perfEntryOptions()).Buttons()
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "beta")
			} else {
				perfEntryClickLabel(r, "beta")
			}
			r.D.Click(perfOutside)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				opts := perfEntryLargeOptions(n)
				selected := State[[]string](c, hashID("perf.entry.checkbox.group.large"), func() []string { return nil })
				CheckboxGroup(c, selected, opts)
			}
		}},
		perfFixture{Name: "CheckAll", Build: func(c *Context) {
			selected := State[[]string](c, hashID("perf.entry.checkall"), func() []string { return nil })
			CheckAll(c, selected, perfEntryOptions())
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "全选")
			r.D.Click(perfOutside)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				opts := perfEntryLargeOptions(n)
				selected := State[[]string](c, hashID("perf.entry.checkall.large"), func() []string { return nil })
				CheckAll(c, selected, opts)
			}
		}}, perfFixture{Name: "Radio", Build: func(c *Context) {
			selected := State[int](c, hashID("perf.entry.radio.single"), func() int { return 0 })
			a := Radio(c, "备选方案", *selected == 0)
			b := Radio(c, "其他方案", *selected == 1)
			if a.Clicked() {
				*selected = 1
			}
			if b.Clicked() {
				*selected = 0
			}
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "备选方案")
			} else {
				perfEntryClickLabel(r, "其他方案")
			}
			r.D.Click(perfOutside)
			r.D.Settle()
		}},
		perfFixture{Name: "RadioGroup", Build: func(c *Context) {
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
				value := State[string](c, hashID("perf.entry.radio.large"), func() string { return opts[0].Value })
				RadioGroup(c, value, opts)
			}
		}},
		perfFixture{Name: "Switch", Build: func(c *Context) {
			v := State[bool](c, hashID("perf.entry.switch"), func() bool { return false })
			Switch(c, v).OuterTexts("启用", "停用")
		}, Interact: func(r *perfRun, _ int) { r.D.Click(r.Center()); r.D.Click(perfOutside) }},
		perfFixture{Name: "SwitchValue", Build: func(c *Context) {
			value := State[string](c, hashID("perf.entry.switch.value"), func() string { return "off" })
			SwitchValue(c, value, "on", "off").OuterTexts("ON", "OFF")
		}, Interact: func(r *perfRun, _ int) { r.D.Click(r.Center()); r.D.Click(perfOutside) }},
		perfFixture{Name: "Toggle", Build: func(c *Context) {
			v := State[bool](c, hashID("perf.entry.toggle"), func() bool { return false })
			Toggle(c, "重点", v)
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "重点")
			r.D.Click(perfOutside)
		}},
		perfFixture{Name: "ToggleGroup", Build: func(c *Context) {
			selected := State[[]string](c, hashID("perf.entry.toggle.group"), func() []string { return []string{"alpha"} })
			ToggleGroup(c, selected, perfEntryOptions())
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "beta")
			r.D.Click(perfOutside)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				opts := perfEntryLargeOptions(n)
				selected := State[[]string](c, hashID("perf.entry.toggle.group.large"), func() []string { return []string{opts[0].Value} })
				ToggleGroup(c, selected, opts)
			}
		}},
		perfFixture{Name: "Slider", Build: func(c *Context) {
			v := State[float64](c, hashID("perf.entry.slider"), func() float64 { return 0 })
			Slider(c, v, 0, 100).Marks(map[float64]string{0: "0", 100: "100"})
		}, Interact: perfEntrySliderDrag},
		perfFixture{Name: "SliderRange", Build: func(c *Context) {
			lo := State[float64](c, hashID("perf.entry.slider.range.lo"), func() float64 { return 0 })
			hi := State[float64](c, hashID("perf.entry.slider.range.hi"), func() float64 { return 0 })
			SliderRange(c, lo, hi, 0, 100)
		}, Interact: perfEntryRangeDrag},
		perfFixture{Name: "ColorPicker", Build: func(c *Context) {
			col := State[theme.Color](c, hashID("perf.entry.color"), func() theme.Color { return theme.Hex("#1677ff") })
			ColorPicker(c, col).ShowText()
		}, Interact: perfEntryColorOpenClose},
		perfFixture{Name: "ColorPickerGradient", Build: func(c *Context) {
			grad := State[theme.Gradient](c, hashID("perf.entry.gradient"), func() theme.Gradient {
				return theme.Gradient{Angle: 90, Stops: []theme.GradientStop{{Offset: 0, Color: theme.Hex("#1677ff")}, {Offset: 1, Color: theme.Hex("#722ed1")}}}
			})
			ColorPickerGradient(c, grad).ShowText()
		}, Interact: perfEntryColorOpenClose},
		perfFixture{Name: "ColorPickerPanel", Build: func(c *Context) {
			col := State[theme.Color](c, hashID("perf.entry.panel.color"), func() theme.Color { return theme.Hex("#13c2c2") })
			ColorPickerPanel(c, col).Presets(theme.Hex("#1677ff"), theme.Hex("#13c2c2"))
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "#1677FF")
			} else {
				perfEntryClickLabel(r, "#13C2C2")
			}
			r.D.Click(perfOutside)
		}},
		perfFixture{Name: "ColorPickerPanelGradient", Build: func(c *Context) {
			grad := State[theme.Gradient](c, hashID("perf.entry.panel.gradient"), func() theme.Gradient {
				return theme.Gradient{Angle: 45, Stops: []theme.GradientStop{{Offset: 0, Color: theme.Hex("#fa8c16")}, {Offset: 1, Color: theme.Hex("#f5222d")}}}
			})
			ColorPickerPanelGradient(c, grad).Presets(theme.Hex("#1677ff"), theme.Hex("#fa8c16"))
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "#1677FF")
			} else {
				perfEntryClickLabel(r, "#FA8C16")
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
		}, Interact: perfEntryHover}, perfFixture{Name: "RangePicker", Build: func(c *Context) {
			r := State[[2]time.Time](c, hashID("perf.entry.range"), func() [2]time.Time { return [2]time.Time{t0(), t1()} })
			RangePicker(c, r).Placeholder("开始日期", "结束日期")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "开始日期") }},
		perfFixture{Name: "MultiDatePicker", Build: func(c *Context) {
			values := State[[]time.Time](c, hashID("perf.entry.multi"), func() []time.Time { return nil })
			MultiDatePicker(c, values).Placeholder("选择多个日期")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "选择多个日期") }},
		perfFixture{Name: "DateTimePicker", Build: func(c *Context) {
			d := State[time.Time](c, hashID("perf.entry.datetime"), func() time.Time { return t0() })
			DateTimePicker(c, d).Placeholder("选择日期时间")
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "选择日期时间") }},
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
		}, Interact: func(r *perfRun, i int) { perfEntryOpenClose(r, i, "选择时刻") }}, perfFixture{Name: "Upload", Build: func(c *Context) {
			files := State[[]UploadFile](c, hashID("perf.entry.files"), func() []UploadFile { return []UploadFile{{Name: "rules.json", Size: 128, Status: "done"}} })
			picked := State[bool](c, hashID("perf.entry.upload.picked"), func() bool { return false })
			Upload(c, files).Dragger().OnPick(func() { *picked = !*picked }).Width(520)
			Text(c, map[bool]string{false: "等待上传", true: "已触发上传"}[*picked])
		}, Interact: func(r *perfRun, _ int) { perfEntryClickLabel(r, "dragger"); r.D.Click(perfOutside); r.D.Settle() }},
		perfFixture{Name: "DropZone", Build: func(c *Context) {
			DropZone(c, func(over bool) { Text(c, map[bool]string{false: "拖入文件", true: "释放文件"}[over]) }).Layout(layout.Size(520, 120))
		}, Interact: perfEntryDrag},
		perfFixture{Name: "Form", Build: func(c *Context) {
			f := Form(c).Layout(layout.W(620))
			value := State[string](c, hashID("perf.entry.form.value"), func() string { return "规则" })
			f.Item("规则名称").Required().Body(func() { Input(c, value).A11yLabel("规则名称") })
		}, Interact: func(r *perfRun, i int) { perfEntryFocusAt(r, i, 0.5, 0.5) }},
		perfFixture{Name: "FieldSet", Build: func(c *Context) {
			f := Form(c).Layout(layout.W(620))
			vals := State[ProValues](c, hashID("perf.entry.form.values"), func() ProValues { return ProValues{"budget": []any{float64(1), float64(2)}} })
			FieldSet(c, f, ProFieldSet{Key: "budget", Label: "预算", Parts: []ProField{{ValueType: ValueDigit}, {ValueType: ValueDigit}}, Separator: "~"}, *vals)
			clicked := State[bool](c, hashID("perf.entry.fieldset.clicked"), func() bool { return false })
			button := Button(c, "字段集操作")
			if button.Clicked() {
				*clicked = !*clicked
			}
			Text(c, map[bool]string{false: "字段集就绪", true: "字段集已操作"}[*clicked])
		}, Interact: func(r *perfRun, _ int) {
			perfEntryClickLabel(r, "字段集操作")
			r.D.Click(perfOutside)
			r.D.Settle()
		}},
		perfFixture{Name: "Item", Build: func(c *Context) {
			selected := State[bool](c, hashID("perf.entry.item.selected"), func() bool { return false })
			item := Item(c, "安全设置").Description(map[bool]string{false: "双重验证已开启", true: "已选择"}[*selected]).Clickable()
			if item.Clicked() {
				*selected = !*selected
			}
		}, Interact: func(r *perfRun, _ int) { perfEntryClickLabel(r, "安全设置"); r.D.Click(perfOutside); r.D.Settle() }},
		perfFixture{Name: "ItemGroup", Build: func(c *Context) {
			selected := State[string](c, hashID("perf.entry.itemgroup.selected"), func() string { return "safe" })
			ItemGroup(c).Bordered().Body(func() {
				safe := Item(c, "安全设置").Description(map[bool]string{false: "双重验证已开启", true: "已选择"}[*selected == "safe"]).Clickable()
				notice := Item(c, "通知设置").Description(map[bool]string{false: "邮件与站内信", true: "已选择"}[*selected == "notice"]).Clickable()
				if safe.Clicked() {
					*selected = "safe"
				}
				if notice.Clicked() {
					*selected = "notice"
				}
			})
		}, Interact: func(r *perfRun, i int) {
			if i%2 == 0 {
				perfEntryClickLabel(r, "通知设置")
			} else {
				perfEntryClickLabel(r, "安全设置")
			}
			r.D.Click(perfOutside)
			r.D.Settle()
		}},
		perfFixture{Name: "Addon", Build: func(c *Context) {
			label := State[string](c, hashID("perf.entry.addon"), func() string { return "tango" })
			a := Addon(c, "https://")
			a.Clicked()
			Row(c, func() { Input(c, label); Addon(c, ".com") })
		}, Interact: func(r *perfRun, i int) { perfEntryHoverText(r, i, "https://") }},
		perfFixture{Name: "HotkeyInput", Build: func(c *Context) {
			v := State[string](c, hashID("perf.entry.hotkey"), func() string { return "Ctrl+K" })
			HotkeyInput(c, v).Width(260)
		}, Interact: perfEntryHotkey},
		perfFixture{Name: "MarkdownEditor", Build: func(c *Context) {
			perfEntryMarkdownBuild(c)
		}, Interact: perfEntryText},
	)
}

func t0() time.Time { return time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC) }
func t1() time.Time { return time.Date(2026, 11, 16, 17, 0, 0, 0, time.UTC) }
