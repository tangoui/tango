package main

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// Select / MultiSelect: option groups, remote search, creating entries,
// custom option rendering, selection limits and virtual dropdowns.

var (
	p2bSelGroup    = "hz"
	p2bSelRender   = []string{"go"}
	p2bSelLimit    = []string{"bj", "sh"}
	p2bSelRemote   = ""
	p2bSelCreate   = []string{"Go"}
	p2bSelVirtual  = "50"
	p2bSelVirtualM []string
	p2bSelLoading  = true
	p2bSelFilter   = ""
)

var p2bSelCityGroups = []ui.Option{
	{Label: "北京", Value: "bj", Group: "华北"}, {Label: "天津", Value: "tj", Group: "华北"},
	{Label: "上海", Value: "sh", Group: "华东"}, {Label: "杭州", Value: "hz", Group: "华东"},
	{Label: "南京", Value: "nj", Group: "华东"}, {Label: "广州", Value: "gz", Group: "华南"},
	{Label: "深圳", Value: "sz", Group: "华南", Disabled: true},
}

var p2bSelLangs = []ui.Option{
	{Label: "Go", Value: "go"}, {Label: "Rust", Value: "rust"}, {Label: "TypeScript", Value: "ts"},
	{Label: "Python", Value: "py"}, {Label: "Kotlin", Value: "kt"},
}

var p2bSelLangDesc = map[string]string{"go": "Google", "rust": "Mozilla", "ts": "Microsoft", "py": "PSF", "kt": "JetBrains"}

var p2bSelMany = func() []ui.Option {
	out := make([]ui.Option, 100000)
	for i := range out {
		out[i] = ui.Option{Label: fmt.Sprintf("选项 %06d", i), Value: fmt.Sprint(i)}
	}
	return out
}()

func p2bSelDemo(c *ui.Context, label string, w float32, fn func()) {
	ui.Column(c, func() {
		ui.Text(c, label).Tertiary()
		fn()
	}, layout.Gap(8), layout.W(w))
}

func init() {
	entryPage(gallery.Page{Name: "SelectAdvanced", Parent: "Select", Title: "Select 分组 / 远程搜索 / 创建条目 / 自定义选项 / 虚拟列表", Order: 62, Width: 1180, Height: 940,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				p2bSelDemo(c, "选项分组（Option.Group）", 240, func() {
					ui.Select(c, &p2bSelGroup, p2bSelCityGroups).Open().A11yLabel("城市")
				})
				p2bSelDemo(c, "自定义选项渲染", 260, func() {
					ui.MultiSelect(c, &p2bSelRender, p2bSelLangs).A11yLabel("语言").OptionRender(func(o ui.Option, s ui.OptionState) {
						ui.Text(c, o.Label)
						ui.Spacer(c)
						ui.Text(c, p2bSelLangDesc[o.Value]).Tertiary().Size(12)
					}).Open()
				})
				p2bSelDemo(c, "最多选择 2 项（MaxCount）", 260, func() {
					ui.MultiSelect(c, &p2bSelLimit, selCities).MaxCount(2).A11yLabel("城市（最多 2 项）").Open()
				})
				p2bSelDemo(c, "10 万选项（虚拟列表）", 260, func() {
					ui.Select(c, &p2bSelVirtual, p2bSelMany).Searchable().A11yLabel("大量选项").Open()
				})
			})
			ui.Gap(c, 330)
			entryRow(c, func() {
				p2bSelDemo(c, "远程搜索（防抖 + 加载中）", 240, func() {
					ui.Select(c, &p2bSelRemote, nil).Placeholder("输入用户名搜索").Remote(func(q string) {}).Loading(p2bSelLoading).Open()
				})
				p2bSelDemo(c, "创建新条目（AllowCreate）", 260, func() {
					ui.MultiSelect(c, &p2bSelCreate, ui.Opts("Go", "Rust", "Zig")).AllowCreate().Placeholder("输入后回车创建").A11yLabel("标签")
				})
				p2bSelDemo(c, "多选 + 10 万选项", 260, func() {
					ui.MultiSelect(c, &p2bSelVirtualM, p2bSelMany).Placeholder("请选择").MaxTagCount(2).A11yLabel("大量选项（多选）")
				})
				p2bSelDemo(c, "ConfigProvider 默认可清除 / 可搜索", 260, func() {
					ui.ConfigProvider(c).Defaults("Select", ui.Props{"allowClear": true, "showSearch": true}).Body(func() {
						ui.Select(c, &p2bSelGroup, p2bSelCityGroups).A11yLabel("城市（全局默认）")
					})
				})
			})
			ui.Gap(c, 90) // below the open loading dropdown of the remote demo
			entryRow(c, func() {
				p2bSelDemo(c, "自定义过滤（FilterMethod）", 260, func() {
					ui.Select(c, &p2bSelFilter, selCities).Placeholder("输入 bj、sh…").A11yLabel("城市（自定义过滤）").
						FilterMethod(func(q string, o ui.Option) bool { return strings.HasPrefix(o.Value, strings.ToLower(q)) })
				})
			})
		}})
}
