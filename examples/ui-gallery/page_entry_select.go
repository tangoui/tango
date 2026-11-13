package main

import (
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var (
	selAC1, selAC2, selAC3, selAC4 = "", "杭", "", "无效地址"
	selMT1                         = "你好 @"
	selMT2, selMT3                 = "请 @李雷 审阅一下", ""
	selAC5, selMT4                 = "西", "抄送 @韩"
	selTags1                       = []string{"Go", "Rust", "TypeScript"}
	selTags2                       = []string{"前端", "后端", "测试", "运维", "产品"}
	selTags3                       []string
	selMS1                         = []string{"bj", "sh"}
	selMS2                         = []string{"bj", "sh", "gz", "sz", "hz"}
	selMS3                         = []string{"bj"}
	selMS4                         []string
	selCas1                        = []string{"zj", "hz", "xh"}
	selCas2                        []string
	selCas3                        []string
	selCasM                        = [][]string{{"zj", "hz", "xh"}, {"zj", "hz", "gs"}, {"zj", "nb", "hs"}}
	selCasS                        []string
	selCasP                        = []string{"js", "nj", "gl"}
	selCasPM                       = [][]string{{"zj", "nb", "hs"}}
)

var selCities = []ui.Option{
	{Label: "北京", Value: "bj"}, {Label: "上海", Value: "sh"}, {Label: "广州", Value: "gz"},
	{Label: "深圳", Value: "sz"}, {Label: "杭州", Value: "hz"}, {Label: "成都", Value: "cd", Disabled: true},
	{Label: "南京", Value: "nj"}, {Label: "武汉", Value: "wh"},
}

var selUsers = []ui.Option{
	{Label: "李雷", Value: "李雷"}, {Label: "韩梅梅", Value: "韩梅梅"}, {Label: "Lucy", Value: "Lucy"}, {Label: "Lily", Value: "Lily"},
}

var selRegions = []ui.CascaderOption{
	{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{
		{Label: "杭州", Value: "hz", Children: []ui.CascaderOption{{Label: "西湖区", Value: "xh"}, {Label: "拱墅区", Value: "gs"}, {Label: "滨江区", Value: "bj"}}},
		{Label: "宁波", Value: "nb", Children: []ui.CascaderOption{{Label: "海曙区", Value: "hs"}, {Label: "江北区", Value: "jb"}}},
		{Label: "温州", Value: "wz", Disabled: true},
	}},
	{Label: "江苏", Value: "js", Children: []ui.CascaderOption{
		{Label: "南京", Value: "nj", Children: []ui.CascaderOption{{Label: "玄武区", Value: "xw"}, {Label: "鼓楼区", Value: "gl"}}},
		{Label: "苏州", Value: "su", Children: []ui.CascaderOption{{Label: "姑苏区", Value: "gsu"}}},
	}},
	{Label: "北京", Value: "bjs"},
}

func selDemo(c *ui.Context, label string, fn func()) {
	ui.Column(c, func() {
		ui.Text(c, label).Tertiary()
		fn()
	}, layout.Gap(8), layout.W(260))
}

func init() {
	entryPage(gallery.Page{Name: "AutoComplete", Title: "AutoComplete 自动完成", Order: 60, Height: 470,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				selDemo(c, "基本（输入以过滤）", func() {
					ui.AutoComplete(c, &selAC1, ui.Opts("Downing Street", "Wall Street", "Baker Street")).Placeholder("请输入地址").AllowClear()
				})
				selDemo(c, "带前缀 / 错误状态", func() {
					ui.AutoComplete(c, &selAC4, nil).Prefix(icon.Search).Status("error").A11yLabel("搜索")
				})
				selDemo(c, "禁用", func() {
					ui.AutoComplete(c, &selAC3, nil).Placeholder("禁用状态").Disabled(true)
				})
			})
			ui.Gap(c, 24)
			entryRow(c, func() {
				selDemo(c, "展开：高亮匹配", func() {
					ui.AutoComplete(c, &selAC2, ui.Opts("杭州西湖", "杭州滨江", "余杭未来科技城", "上海浦东")).Highlight().Open().A11yLabel("地点")
				})
				selDemo(c, "动态建议（邮箱后缀）", func() {
					ui.AutoComplete(c, &selAC3, nil).Placeholder("输入用户名").Source(func(q string) []ui.Option {
						if q == "" {
							return nil
						}
						return ui.Opts(q+"@qq.com", q+"@163.com", q+"@gmail.com")
					})
				})
				selDemo(c, "远程搜索（防抖 + 加载中）", func() {
					// Remote runs after the debounce; fetch in the background,
					// then pass the results and Loading(false).
					ui.AutoComplete(c, &selAC5, nil).Remote(func(string) {}).Loading(true).Open().A11yLabel("远程地点")
				})
			})
		}})
	entryPage(gallery.Page{Name: "Mentions", Title: "Mentions 提及", Order: 61, Height: 520,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				selDemo(c, "输入 @ 弹出候选", func() {
					ui.Mentions(c, &selMT1, selUsers).Open().A11yLabel("评论")
				})
				selDemo(c, "多行 / # 话题", func() {
					ui.Mentions(c, &selMT2, selUsers).Prefix("@", "#").Rows(3).A11yLabel("多行评论")
				})
				selDemo(c, "错误 / 禁用", func() {
					ui.Mentions(c, &selMT3, selUsers).Placeholder("请输入评论").Status("error")
					ui.Mentions(c, &selMT3, selUsers).Placeholder("禁用状态").Disabled(true)
				})
			})
			ui.Gap(c, 150)
			entryRow(c, func() {
				selDemo(c, "远程联想（Remote + 加载中）", func() {
					// Remote receives the trigger prefix and the typed query.
					ui.Mentions(c, &selMT4, nil).Remote(func(prefix, query string) {}).Loading(true).Open().A11yLabel("远程评论")
				})
			})
		}})
	entryPage(gallery.Page{Name: "InputTag", Title: "InputTag 标签输入", Order: 62,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				selDemo(c, "回车或逗号添加，可关闭", func() {
					ui.InputTag(c, &selTags1).Placeholder("请输入标签").AllowClear()
				})
				selDemo(c, "最多显示 2 个", func() {
					ui.InputTag(c, &selTags2).MaxTagCount(2).A11yLabel("标签")
				})
				selDemo(c, "空 / 错误 / 禁用", func() {
					ui.InputTag(c, &selTags3).Placeholder("最多 3 个标签").MaxTags(3)
					ui.InputTag(c, &selTags3).Placeholder("错误状态").Status("error")
					ui.InputTag(c, &selTags1).Disabled(true).A11yLabel("标签（禁用）")
				})
			})
			ui.Gap(c, 16)
			selDemo(c, "尺寸：小 / 大", func() {
				ui.InputTag(c, &selTags1).Size(ui.Small).A11yLabel("标签（小）")
				ui.InputTag(c, &selTags1).Size(ui.Large).A11yLabel("标签（大）")
			})
		}})
	entryPage(gallery.Page{Name: "MultiSelect", Title: "Select 多选（标签折叠）", Order: 63, Height: 560,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				selDemo(c, "多选（可清空）", func() {
					ui.MultiSelect(c, &selMS1, selCities).Placeholder("请选择城市").AllowClear()
				})
				selDemo(c, "maxTagCount = 2", func() {
					ui.MultiSelect(c, &selMS2, selCities).MaxTagCount(2).A11yLabel("城市")
				})
				selDemo(c, "错误 / 禁用", func() {
					ui.MultiSelect(c, &selMS4, selCities).Placeholder("请至少选择一项").Status("error")
					ui.MultiSelect(c, &selMS3, selCities).Disabled(true).A11yLabel("城市（禁用）")
				})
			})
			ui.Gap(c, 24)
			entryRow(c, func() {
				selDemo(c, "展开：可搜索，选中项右侧打勾", func() {
					ui.MultiSelect(c, &selMS3, selCities).Searchable().Open().A11yLabel("搜索城市")
				})
				selDemo(c, "小尺寸 / 大尺寸", func() {
					ui.MultiSelect(c, &selMS1, selCities).Size(ui.Small).A11yLabel("城市（小）")
					ui.MultiSelect(c, &selMS1, selCities).Size(ui.Large).A11yLabel("城市（大）")
				})
			})
		}})
	entryPage(gallery.Page{Name: "Cascader", Title: "Cascader 级联选择", Order: 64, Width: 1480, Height: 1560,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				selDemo(c, "基本（显示完整路径）", func() {
					ui.Cascader(c, &selCas1, selRegions).AllowClear().A11yLabel("地区")
				})
				selDemo(c, "可搜索", func() {
					ui.Cascader(c, &selCasS, selRegions).Placeholder("搜索地区").Searchable()
				})
				selDemo(c, "错误 / 禁用", func() {
					ui.Cascader(c, &selCas3, selRegions).Placeholder("请选择地区").Status("error")
					ui.Cascader(c, &selCas1, selRegions).Disabled(true).A11yLabel("地区（禁用）")
				})
			})
			ui.Gap(c, 24)
			entryRow(c, func() {
				selDemo(c, "展开：单选", func() {
					ui.Cascader(c, &selCas2, selRegions).Placeholder("请选择").Open().Expand("zj", "hz")
				})
				ui.Gap(c, 330)
				selDemo(c, "多选：复选框与半选", func() {
					ui.Cascader(c, &selCas3, selRegions).A11yLabel("多选地区").Multiple(&selCasM).MaxTagCount(2).Open().Expand("zj", "hz")
				})
			})
			ui.Gap(c, 200)
			selDemo(c, "搜索：输入“区”过滤完整路径", func() {
				ui.Cascader(c, &selCasS, selRegions).Placeholder("输入关键字搜索").Searchable()
			})
			ui.Gap(c, 220)
			entryRow(c, func() {
				selDemo(c, "CascaderPanel 内嵌面板（单选，可选任意级）", func() {
					ui.CascaderPanel(c, &selCasP, selRegions).ChangeOnSelect().A11yLabel("内嵌地区")
					ui.Text(c, "已选："+strings.Join(selCasP, " / ")).Secondary()
				})
				selDemo(c, "CascaderPanel 多选", func() {
					ui.CascaderPanel(c, nil, selRegions).Multiple(&selCasPM).A11yLabel("内嵌多选地区")
				})
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("输入关键字搜索")
			d.Type("区")
		}})
}
