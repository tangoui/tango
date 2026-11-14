package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/theme"
)

// defSp formats values separated by spaces.
func defSp(a ...any) string { return strings.TrimSuffix(fmt.Sprintln(a...), "\n") }

// TestDefaultsWiring builds each newly wired component inside
// ConfigProvider.Defaults: def reads the state the defaults produced and
// exp the state after an explicit modifier, which must win (exp is nil
// when the component has no modifier that undoes the default).
func TestDefaultsWiring(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		comp    string
		props   Props
		def     func(c *Context) any
		want    any
		exp     func(c *Context) any
		wantExp any
	}{
		{"Addon", "Addon", Props{"size": "large"},
			func(c *Context) any { return Addon(c, "x").size }, Large,
			func(c *Context) any { return Addon(c, "x").Size(Small).size }, Small},
		{"BackTop", "BackTop", Props{"visibilityHeight": 200, "duration": 100},
			func(c *Context) any { b := BackTop(c, 0); return defSp(b.visible, b.dur) }, "200 100ms",
			func(c *Context) any {
				b := BackTop(c, 0).VisibilityHeight(50).Duration(time.Second)
				return defSp(b.visible, b.dur)
			}, "50 1s"},
		{"Compact", "Compact", Props{"block": true},
			func(c *Context) any { return Compact(c).block }, true, nil, nil},
		{"Flex size", "Flex", Props{"gap": "large"},
			func(c *Context) any { return Flex(c).b.n.ln.Gap }, float32(24),
			func(c *Context) any { return Flex(c).Gap(Small).b.n.ln.Gap }, float32(8)},
		{"Flex px", "Flex", Props{"gap": 12},
			func(c *Context) any { return Flex(c).b.n.ln.Gap }, float32(12),
			func(c *Context) any { return Flex(c).GapPx(4).b.n.ln.Gap }, float32(4)},
		{"FloatButtonGroup", "FloatButtonGroup", Props{"shape": "square", "trigger": "hover", "type": "primary"},
			func(c *Context) any { g := FloatButtonGroup(c); return defSp(g.square, g.trigger, g.primary) }, "true hover true",
			func(c *Context) any { return FloatButtonGroup(c).Trigger("click").trigger }, "click"},
		{"SplitterPanel", "SplitterPanel", Props{"collapsible": true, "resizable": false},
			func(c *Context) any { p := SplitterPanel(c); return defSp(p.collapsible, p.fixed) }, "true true",
			func(c *Context) any { return SplitterPanel(c).Resizable(true).fixed }, false},
		{"Command", "Command", Props{"placeholder": "P", "emptyText": "E"},
			func(c *Context) any { w := Command(c, nil); return w.place + w.empty }, "PE",
			func(c *Context) any { w := Command(c, nil).Placeholder("Q").Empty("F"); return w.place + w.empty }, "QF"},
		{"CascaderPanel", "Cascader", Props{"expandTrigger": "hover", "changeOnSelect": true, "showCheckedStrategy": "child", "strategyValue": true},
			func(c *Context) any {
				w := CascaderPanel(c, nil, nil).cas
				return defSp(w.hoverExpand, w.changeOnSelect, w.strategy == ShowChild, w.stratVal)
			}, "true true true true",
			func(c *Context) any { return CascaderPanel(c, nil, nil).ShowCheckedStrategy(ShowAll).cas.strategy }, ShowAll},
		{"Checkbox", "Checkbox", Props{"size": "small"},
			func(c *Context) any { return Checkbox(c, "a", new(bool)).size }, Small,
			func(c *Context) any { return Checkbox(c, "a", new(bool)).Size(Large).size }, Large},
		{"CheckboxGroup", "Checkbox", Props{"size": Large},
			func(c *Context) any { return CheckboxGroup(c, new([]string), nil).size }, Large,
			func(c *Context) any { return CheckboxGroup(c, new([]string), nil).Size(Small).size }, Small},
		{"Radio", "Radio", Props{"size": "large"},
			func(c *Context) any { return Radio(c, "a", false).size }, Large,
			func(c *Context) any { return Radio(c, "a", false).Size(Small).size }, Small},
		{"ColorPickerPanel", "ColorPicker", Props{"allowClear": true, "disabledAlpha": true, "size": "small", "format": "hsb"},
			func(c *Context) any {
				cp := ColorPickerPanel(c, new(theme.Color)).cp
				return defSp(cp.allowClear, cp.noAlpha, cp.size == Small, cp.st.format)
			}, "true true true hsb",
			func(c *Context) any { return ColorPickerPanel(c, new(theme.Color)).Format("rgb").cp.st.format }, "rgb"},
		{"ColorPickerPanelGradient", "ColorPicker", Props{"allowClear": true},
			func(c *Context) any { return ColorPickerPanelGradient(c, new(theme.Gradient)).cp.allowClear }, true, nil, nil},
		{"DatePickerPanel", "DatePicker", Props{"picker": PickerMonth},
			func(c *Context) any { return DatePickerPanel(c, &day).picker }, PickerMonth,
			func(c *Context) any { return DatePickerPanel(c, &day).Picker(PickerYear).picker }, PickerYear},
		{"TimePanel", "TimePicker", Props{"format": "15:04", "use12Hours": true, "minuteStep": 5, "hideDisabledOptions": true},
			func(c *Context) any {
				w := TimePanel(c, &day)
				return defSp(w.format, w.clock.use12, w.steps[1], w.clock.hide)
			}, "15:04 true 5 true",
			func(c *Context) any {
				w := TimePanel(c, &day).Format("15:04:05").Steps(1, 2, 3)
				return defSp(w.format, w.steps)
			}, "15:04:05 [1 2 3]"},
		{"MultiDatePicker", "DatePicker", Props{"format": "2006/01/02", "placeholder": "P", "picker": PickerMonth, "allowClear": false, "size": Small, "width": 300},
			func(c *Context) any {
				w := MultiDatePicker(c, new([]time.Time))
				return defSp(w.format, w.formatSet, w.box.placeholder, w.phSet, w.picker == PickerMonth, w.box.allowClear, w.box.size == Small, w.box.n.ln.Width)
			}, "2006/01/02 true P true true false true 300",
			func(c *Context) any {
				w := MultiDatePicker(c, new([]time.Time)).Format("01-02").Placeholder("Q").AllowClear(true)
				return defSp(w.format, w.box.placeholder, w.box.allowClear)
			}, "01-02 Q true"},
		{"MarkdownEditor", "MarkdownEditor", Props{"mode": "edit", "placeholder": "P"},
			func(c *Context) any { w := MarkdownEditor(c, new(string)); return w.mode + w.placeholder }, "editP",
			func(c *Context) any {
				w := MarkdownEditor(c, new(string)).Mode("preview").Placeholder("Q")
				return w.mode + w.placeholder
			}, "previewQ"},
		{"AvatarGroup", "AvatarGroup", Props{"maxCount": 3},
			func(c *Context) any { return AvatarGroup(c).max }, 3,
			func(c *Context) any { return AvatarGroup(c).Max(5).max }, 5},
		{"BadgeRibbon", "BadgeRibbon", Props{"placement": "start"},
			func(c *Context) any {
				BadgeRibbon(c, "hot", "", "", func() {})
				return badgeRibbonPlacement(c, "")
			}, "start",
			func(c *Context) any { return badgeRibbonPlacement(c, "end") }, "end"},
		{"Countdown", "Countdown", Props{"format": "mm:ss"},
			func(c *Context) any { return Countdown(c, "", day).format }, "mm:ss",
			func(c *Context) any { return Countdown(c, "", day).Format("ss").format }, "ss"},
		{"StatisticTimer", "Countdown", Props{"format": "D HH"},
			func(c *Context) any { return StatisticTimer(c, "", TimerCountup, day).format }, "D HH",
			func(c *Context) any { return StatisticTimer(c, "", TimerCountup, day).Format("ss").format }, "ss"},
		{"InfiniteScroll", "InfiniteScroll", Props{"distance": 50},
			func(c *Context) any { return InfiniteScroll(c, func() {}).distance }, float32(50),
			func(c *Context) any { return InfiniteScroll(c, func() {}).Distance(10).distance }, float32(10)},
		{"Listy", "Listy", Props{"split": false, "rowHoverable": false},
			func(c *Context) any { w := Listy(c, 0, func(int) {}); return defSp(w.split, w.hoverable) }, "false false",
			func(c *Context) any {
				w := Listy(c, 0, func(int) {}).Split(true).RowHoverable(true)
				return defSp(w.split, w.hoverable)
			}, "true true"},
		{"Tour", "Tour", Props{"type": "primary", "mask": false, "arrow": false},
			func(c *Context) any { w := Tour(c, new(bool), new(int), nil); return defSp(w.primary, w.mask, w.arrow) }, "true false false", nil, nil},
		{"VirtualTree", "VirtualTree", Props{"checkStrictly": true, "expandOnClickNode": false, "checkOnClickNode": true},
			func(c *Context) any {
				w := VirtualTree(c, nil)
				return defSp(w.strict, w.expClick, w.expSet, w.chkClick)
			}, "true false true true",
			func(c *Context) any { return VirtualTree(c, nil).ExpandOnClick(true).expClick }, true},
		{"Loading", "Spin", Props{"size": "large", "tip": "Wait"},
			func(c *Context) any { m := Loading(c, true); return defSp(m.size == Large, m.sizeSet, m.tip) }, "true true Wait",
			func(c *Context) any { m := Loading(c, true).Size(Small).Tip("X"); return defSp(m.size == Small, m.tip) }, "true X"},
		{"Item", "Item", Props{"variant": "outline", "size": "sm"},
			func(c *Context) any { w := Item(c, "a"); return defSp(w.variant, w.size) }, "outline sm",
			func(c *Context) any { w := Item(c, "a").Muted().Size(ItemSizeDefault); return defSp(w.variant, w.size) }, "muted default"},
		{"Item typed", "Item", Props{"variant": ItemVariantMuted, "size": ItemSizeSmall},
			func(c *Context) any { w := Item(c, "a"); return defSp(w.variant, w.size) }, "muted sm", nil, nil},
		{"ItemGroup", "ItemGroup", Props{"split": true, "bordered": true},
			func(c *Context) any { g := ItemGroup(c); return defSp(g.sep, g.bordered) }, "true true", nil, nil},
		{"ButtonGroup", "ButtonGroup", Props{"size": "small", "type": "primary"},
			func(c *Context) any {
				g := ButtonGroup(c)
				return defSp(g.size == Small, g.sizeSet, g.variant == VariantPrimary, g.varSet)
			}, "true true true true",
			func(c *Context) any {
				g := ButtonGroup(c).Size(Large).Variant(VariantDashed)
				return defSp(g.size == Large, g.variant == VariantDashed)
			}, "true true"},
		{"ButtonGroup unknown type", "ButtonGroup", Props{"type": "nope", "size": "small"},
			func(c *Context) any { g := ButtonGroup(c); return defSp(g.sizeSet, g.varSet) }, "true false", nil, nil},
		{"Button autoInsertSpace", "Button", Props{"autoInsertSpace": true},
			func(c *Context) any { return Button(c, "确定").shownLabel() }, "确 定",
			func(c *Context) any { return Button(c, "确定").AutoInsertSpace(false).shownLabel() }, "确定"},
		// Already wired entries that are new since the last count.
		{"SwitchValue", "Switch", Props{"size": "small"},
			func(c *Context) any { v := "off"; return SwitchValue(c, &v, "on", "off").small }, true,
			func(c *Context) any { v := "off"; return SwitchValue(c, &v, "on", "off").Size(Middle).small }, false},
		{"ColorPickerGradient", "ColorPicker", Props{"showText": true},
			func(c *Context) any { return ColorPickerGradient(c, new(theme.Gradient)).showText }, true, nil, nil},
		{"ProFieldOf", "ProField", Props{"mode": "edit"},
			func(c *Context) any { var v any; return ProFieldOf(c, ProField{Label: "a"}, &v).mode }, ProFieldEdit,
			func(c *Context) any {
				var v any
				return ProFieldOf(c, ProField{Label: "a"}, &v).Mode(ProFieldRead).mode
			}, ProFieldRead},
		{"SchemaForm", "SchemaForm", Props{"layoutType": string(SchemaLayoutQueryFilter)},
			func(c *Context) any { return SchemaForm(c, nil).lt }, SchemaLayoutQueryFilter,
			func(c *Context) any { return SchemaForm(c, nil).LayoutType(SchemaLayoutForm).lt }, SchemaLayoutForm},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got, gotExp, base any
			d := NewDriver(func(c *Context) {
				base = tc.def(c) // no provider: the built-in default
				ConfigProvider(c).Defaults(tc.comp, tc.props).Body(func() {
					got = tc.def(c)
					if tc.exp != nil {
						gotExp = tc.exp(c)
					}
				})
			}, DriverOptions{Width: 800, Height: 600})
			d.Frame()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("default: got %v, want %v", got, tc.want)
			}
			if reflect.DeepEqual(base, tc.want) {
				t.Errorf("the built-in default already equals %v: the test proves nothing", tc.want)
			}
			if tc.exp != nil && !reflect.DeepEqual(gotExp, tc.wantExp) {
				t.Errorf("explicit: got %v, want %v", gotExp, tc.wantExp)
			}
		})
	}
}

// TestButtonAutoInsertSpaceRules: only two Han characters, no icon, not
// text / link; the accessible name keeps the label.
func TestButtonAutoInsertSpaceRules(t *testing.T) {
	var got []string
	var b, plain *ButtonW
	d := NewDriver(func(c *Context) {
		got = got[:0]
		b = Button(c, "提交").AutoInsertSpace(true)
		plain = Button(c, "提交")
		got = append(got,
			b.shownLabel(),
			Button(c, "确定吗").AutoInsertSpace(true).shownLabel(),
			Button(c, "OK").AutoInsertSpace(true).shownLabel(),
			Button(c, "删除").AutoInsertSpace(true).Icon(icon.Delete).shownLabel(),
			Button(c, "链接").AutoInsertSpace(true).Link().shownLabel(),
			Button(c, "文字").AutoInsertSpace(true).Text().shownLabel(),
			Button(c, "主要").AutoInsertSpace(true).Primary().shownLabel(),
			Button(c, "默认").shownLabel(),
		)
	}, DriverOptions{Width: 600, Height: 200})
	d.EnableAccessibility()
	d.Frame()
	want := []string{"提 交", "确定吗", "OK", "删除", "链接", "文字", "主 要", "默认"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labels %q, want %q", got, want)
	}
	if b.measure(0, 0).W <= plain.measure(0, 0).W {
		t.Error("the spaced label is not measured wider")
	}
	if b.n.label != "提交" {
		t.Errorf("node label %q, want the label as given", b.n.label)
	}
}

// TestMessageMaxCount: the "Message" maxCount default closes the oldest
// messages; without it every message stays.
func TestMessageMaxCount(t *testing.T) {
	for _, tc := range []struct {
		props Props
		want  []string
	}{
		{Props{"maxCount": 2}, []string{"b", "c"}},
		{Props{"maxCount": 0}, []string{"a", "b", "c"}},
		{nil, []string{"a", "b", "c"}},
	} {
		var got []string
		frame := 0
		d := NewDriver(func(c *Context) {
			frame++
			ConfigProvider(c).Defaults("Message", tc.props).Body(func() {
				if frame == 1 {
					Message(c, "info", "a")
					Message(c, "info", "b")
					Message(c, "info", "c")
				}
				got = got[:0]
				for _, m := range c.toasts().msgs {
					got = append(got, m.title)
				}
			})
		}, DriverOptions{Width: 600, Height: 400})
		d.Frame()
		d.Frame()
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: messages %q, want %q", tc.props, got, tc.want)
		}
	}
}

// TestFieldSetDefaults: FieldSet goes through ProFormFieldSet, so the
// "ProFormFieldSet" separator applies; the set's own separator wins.
func TestFieldSetDefaults(t *testing.T) {
	for _, tc := range []struct{ own, want, not string }{
		{"", "至", ""},
		{"~", "~", "至"},
	} {
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Defaults("ProFormFieldSet", Props{"separator": "至"}).Body(func() {
				f := Form(c)
				FieldSet(c, f, ProFieldSet{Key: "r", Label: "范围", Separator: tc.own,
					Parts: []ProField{{ValueType: "digit"}, {ValueType: "digit"}}}, ProValues{})
			})
		}, DriverOptions{Width: 600, Height: 300})
		d.Settle()
		if _, ok := d.FindText(tc.want); !ok {
			t.Errorf("own %q: separator %q not drawn", tc.own, tc.want)
		}
		if tc.not != "" {
			if _, ok := d.FindText(tc.not); ok {
				t.Errorf("own %q: default separator %q drawn", tc.own, tc.not)
			}
		}
	}
}

// TestTimeRangePanelsIgnoreTimePickerDefaults: TimeRangePicker's inner
// panels do not take the "TimePicker" keys TimePanel reads.
func TestTimeRangePanelsIgnoreTimePickerDefaults(t *testing.T) {
	var p *TimePanelW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("TimePicker", Props{"use12Hours": true}).Body(func() {
			p = timePanel(c, new(time.Time))
		})
	}, DriverOptions{Width: 300, Height: 300})
	d.Frame()
	if p.clock.use12 {
		t.Fatal("timePanel took the TimePicker defaults")
	}
}
