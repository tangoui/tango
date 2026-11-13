package main

import (
	"fmt"
	"math"
	"path/filepath"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var p2eRegions = []ui.ProEnum{
	{Value: "zj", Label: "浙江", Children: []ui.ProEnum{{Value: "hz", Label: "杭州", Children: []ui.ProEnum{{Value: "xh", Label: "西湖区"}, {Value: "bj", Label: "滨江区"}}}, {Value: "nb", Label: "宁波"}}},
	{Value: "js", Label: "江苏", Children: []ui.ProEnum{{Value: "nj", Label: "南京"}, {Value: "sz", Label: "苏州"}}},
}

func p2eFields() []ui.ProField {
	return []ui.ProField{
		{Key: "area", Label: "所在地区", ValueType: ui.ValueCascader, ValueEnum: p2eRegions, Span: 12},
		{Key: "dept", Label: "所属城市", ValueType: ui.ValueTreeSelect, ValueEnum: p2eRegions, Span: 12},
		{Key: "at", Label: "提醒时间", ValueType: ui.ValueTime, Span: 12},
		{Key: "span", Label: "营业时段", ValueType: ui.ValueTimeRange, Span: 12},
		{Key: "color", Label: "主题色", ValueType: ui.ValueColor, Span: 12},
		{Key: "price", Label: "价格区间", ValueType: ui.ValueDigitRange, Span: 12},
		{Key: "files", Label: "附件", ValueType: ui.ValueUpload, Dragger: true},
	}
}

func p2eFieldValues() ui.ProValues {
	at := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	return ui.ProValues{
		"area":  []string{"zj", "hz", "xh"},
		"dept":  "sz",
		"at":    at,
		"span":  [2]time.Time{at, at.Add(9 * time.Hour)},
		"color": "#1677FF",
		"price": [2]float64{100, math.NaN()},
		"files": []ui.UploadFile{{Name: "方案.pdf", Status: "done"}},
	}
}

func init() {
	p2ePage("ProFieldTypes", "ProField 值类型（级联、树选择、上传、时间、颜色、数字范围）", gallery.Form, "ProForm", 35, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		vals := p2eFieldValues()
		view := p2eFieldValues()
		return func(c *ui.Context) {
			tbDemo(c, "编辑模式", "ProForm 按 valueType 选用 Cascader、TreeSelect、TimePicker、ColorPicker、InputNumber 区间与 Upload（拖拽区）。", func() {
				fields := p2eFields()
				fields[6].Pick = func(done func([]ui.UploadFile)) { done([]ui.UploadFile{{Name: "截图.png", Status: "done"}}) }
				ui.ProForm(c, fields).Columns(2).Values(&vals).NoReset().Show()
			})
			tbDemo(c, "只读模式", "ProDescriptions 用同一组 valueType 渲染只读值；空值显示为“-”。", func() {
				var items []ui.ProDescItem
				for _, fd := range p2eFields() {
					items = append(items, ui.ProDescItem{Key: fd.Key, Label: fd.Label, ValueType: fd.ValueType, ValueEnum: fd.ValueEnum})
				}
				ui.ProDescriptions(c, items).Column(2).Bordered().DataSource(&view).Show()
			})
			tbDemo(c, "金额的货币符号随 Locale", "valueType money 默认取当前语言的货币符号（zh-CN ¥、en-US $、zh-TW NT$、ja ¥），ProColumn.Currency 可单独指定。", func() {
				amt := ui.ProValues{"amount": 12800.5}
				ui.Row(c, func() {
					for _, l := range []*ui.Locale{ui.ZhCN, ui.EnUS, ui.ZhTW, ui.Ja} {
						ui.Key(c, l.Code, func() {
							ui.Column(c, func() {
								ui.ConfigProvider(c).Locale(l).Body(func() {
									ui.ProDescriptions(c, []ui.ProDescItem{{Key: "amount", Label: l.Code, ValueType: ui.ValueMoney}}).Column(1).Bordered().DataSource(&amt).Show()
								})
							}, layout.Grow(1), layout.MinW(0))
						})
					}
				}, layout.Gap(16))
			})
		}, nil
	})

	p2ePage("TableRowClass", "Table 行样式与选择菜单", gallery.Display, "Table", 120, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(8)
		var sel []int
		page, size := 1, 5
		return func(c *ui.Context) {
				tbDemo(c, "RowClass · Selections", "RowClass 按行返回样式（状态底色、自定义背景、文字色、加粗），对应 Element 的 row-class-name；表头勾选框旁的下拉提供全选所有、选择当页、反选当页、清空。", func() {
					cols := []ui.TableColumn{
						{Key: "name", Title: "姓名", Width: 160, Text: func(i int) string { return data[i].name }},
						{Key: "age", Title: "年龄", Width: 100, Text: func(i int) string { return fmt.Sprint(data[i].age) }},
						{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
					}
					ui.Table(c, cols, len(data)).Selection(&sel).Selections().Pagination(&page, &size).
						RowClass(func(i int) ui.RowStyle {
							switch {
							case i == 1:
								return ui.RowStyle{Status: "warning"}
							case i == 3:
								return ui.RowStyle{Status: "success"}
							case data[i].age > 40:
								return ui.RowStyle{Text: c.Theme.Error.Base, Bold: true}
							}
							return ui.RowStyle{}
						}).Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("选择选项")
			}
	})

	p2ePage("LightFilterCollapse", "LightFilter 折叠更多筛选", gallery.Form, "LightFilter", 36, 960, 560, func() (func(*ui.Context), func(*ui.Driver)) {
		vals := ui.ProValues{"status": "running", "amount": 300.0}
		return func(c *ui.Context) {
				tbDemo(c, "Collapse(2)", "超过 2 个字段时，其余字段收进“更多筛选”，在一个浮层里一起编辑、确认或清除；按钮上显示已设置的个数。", func() {
					ui.LightFilter(c, p3FilterFields()).Values(&vals).Collapse(2).Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("更多筛选")
			}
	})

	p2ePage("ProListRequest", "ProList 远程分页", gallery.Display, "ProList", 62, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		base := p3Projects()
		all := make([]p3Project, 23)
		for i := range all {
			all[i] = base[i%len(base)]
			all[i].key = fmt.Sprint("r", i)
			all[i].name = fmt.Sprintf("%s #%02d", all[i].name, i+1)
		}
		return func(c *ui.Context) {
				tbDemo(c, "Request(page, size)", "ProList 在后台按页请求数据并显示加载态；翻页触发新的请求，失败时显示错误与重试。", func() {
					ui.ProList(c, nil, p3Metas()).HeaderTitle("项目列表").RowKey(func(r p3Project) string { return r.key }).Pagination(5).
						Request(func(page, size int) ([]p3Project, int, error) {
							a := (page - 1) * size
							return all[a:min(len(all), a+size)], len(all), nil
						}).Show()
				})
			}, func(d *ui.Driver) {
				for i := 0; i < 50; i++ {
					d.Settle()
					time.Sleep(2 * time.Millisecond)
				}
			}
	})

	p2ePage("DropZone", "DropZone 拖入文件高亮", gallery.Form, "Upload", 51, 960, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		var files []ui.UploadFile
		var last []string
		return func(c *ui.Context) {
				tbDemo(c, "悬停高亮 · 落下", "宿主（uihost）把系统拖放的悬停位置交给 Runner；DropZone 判断文件是否悬停在自身上并高亮，Upload 拖拽区在内部套用它。", func() {
					ui.Row(c, func() {
						z := ui.DropZone(c, func(over bool) {
							ui.Column(c, func() {
								msg := "把文件拖到这里"
								if over {
									msg = "松开即可添加"
								}
								ui.Text(c, msg)
								for _, p := range last {
									ui.Text(c, filepath.Base(p)).Tertiary()
								}
							}, layout.Pad(24), layout.MinH(120), layout.Center())
						}).A11yLabel("附件区").Layout(layout.Grow(1))
						if p := z.Dropped(); p != nil {
							last = p
						}
						ui.Column(c, func() {
							ui.Upload(c, &files).Dragger()
						}, layout.Grow(1))
					}, layout.Gap(24))
				})
			}, func(d *ui.Driver) {
				if r, ok := d.FindText("把文件拖到这里"); ok {
					d.FileDragOver(r.Center())
				}
			}
	})
}
