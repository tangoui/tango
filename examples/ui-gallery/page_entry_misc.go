package main

import (
	"image"
	"image/color"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var miscTree = []ui.TreeSelectNode{
	{Label: "浙江省", Value: "zj", Children: []ui.TreeSelectNode{
		{Label: "杭州市", Value: "hz", Children: []ui.TreeSelectNode{
			{Label: "西湖区", Value: "xh"}, {Label: "余杭区", Value: "yh"}, {Label: "滨江区", Value: "bj", Disabled: true},
		}},
		{Label: "宁波市", Value: "nb"},
	}},
	{Label: "江苏省", Value: "js", Children: []ui.TreeSelectNode{
		{Label: "南京市", Value: "nj"}, {Label: "苏州市", Value: "sz"},
	}},
	{Label: "北京市", Value: "bjs"},
}

var miscState = struct {
	ts1, ts2, ts3, ts4 string
	tsm, tsc, tsc2     []string
	target, target2    []string
	files, cards       []ui.UploadFile
	drag               []ui.UploadFile
	all, all2          []string
	q                  string
}{
	ts1: "xh", ts3: "nb",
	tsm:    []string{"hz", "nj"},
	tsc:    []string{"xh", "yh", "nb", "sz"},
	tsc2:   []string{"xh"},
	target: []string{"k3", "k6"},
	all:    []string{"苹果", "香蕉"},
	all2:   []string{"苹果", "香蕉", "橙子"},
}

func miscThumb(c1, c2 color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			t := float64(x+y) / 190
			l := func(a, b uint8) uint8 { return uint8(float64(a)*(1-t) + float64(b)*t) }
			img.SetNRGBA(x, y, color.NRGBA{l(c1.R, c2.R), l(c1.G, c2.G), l(c1.B, c2.B), 255})
		}
	}
	return img
}

func init() {
	miscState.files = []ui.UploadFile{
		{Name: "需求说明书.docx", Size: 23000, Status: "done"},
		{Name: "设计稿-v2.fig", Size: 1200000, Status: "uploading", Percent: 46},
		{Name: "合同扫描件.pdf", Status: "error"},
	}
	miscState.cards = []ui.UploadFile{
		{Name: "image.png", Status: "done", Thumb: miscThumb(color.NRGBA{22, 119, 255, 255}, color.NRGBA{114, 46, 209, 255})},
		{Name: "photo.jpg", Status: "done", Thumb: miscThumb(color.NRGBA{82, 196, 26, 255}, color.NRGBA{250, 173, 20, 255})},
		{Name: "上传中.png", Status: "uploading", Percent: 50},
		{Name: "error.png", Status: "error"},
	}

	entryPage(gallery.Page{Name: "TreeSelect", Title: "TreeSelect 树选择", Order: 70, Height: 820, Render: func(c *ui.Context) {
		s := &miscState
		entryRow(c, func() {
			entryCol(c, func() {
				ui.Text(c, "多选").Tertiary()
				ui.TreeSelect(c, &s.ts2, miscTree).Multiple(&s.tsm).Width(300).Placeholder("请选择城市")
				ui.Text(c, "可勾选 + 折叠标签").Tertiary()
				ui.TreeSelect(c, &s.ts2, miscTree).Multiple(&s.tsc).Checkable().MaxTagCount(2).Width(300)
				ui.Text(c, "可搜索 / 小尺寸").Tertiary()
				ui.TreeSelect(c, &s.ts4, miscTree).Placeholder("请选择").Searchable().Width(300)
				ui.TreeSelect(c, &s.ts4, miscTree).Size(ui.Small).Placeholder("小尺寸").Width(300)
			})
			entryCol(c, func() {
				ui.Text(c, "校验 / 禁用").Tertiary()
				ui.TreeSelect(c, &s.ts3, miscTree).Status("error").Width(200)
				ui.TreeSelect(c, &s.ts3, miscTree).Status("warning").Width(200)
				ui.TreeSelect(c, &s.ts3, miscTree).Disabled(true).Width(200)
				ui.TreeSelect(c, &s.ts2, miscTree).Multiple(&s.tsm).Disabled(true).Width(200)
			})
		})
		ui.Gap(c, 16)
		entryRow(c, func() {
			entryCol(c, func() {
				ui.Text(c, "单选（展开）").Tertiary()
				ui.TreeSelect(c, &s.ts1, miscTree).Width(220).AllowClear().Open()
			})
			entryCol(c, func() {
				ui.Text(c, "可勾选（半选联动，展开）").Tertiary()
				ui.TreeSelect(c, &s.ts2, miscTree).Multiple(&s.tsc2).Checkable().DefaultExpandAll().Width(300).Open()
			})
		})
	}})

	items := []ui.TransferItem{}
	for i, t := range []string{"内容一", "内容二", "内容三", "内容四", "内容五", "内容六", "内容七", "内容八"} {
		items = append(items, ui.TransferItem{Key: "k" + string(rune('1'+i)), Title: t, Disabled: i == 3})
	}
	entryPage(gallery.Page{Name: "Transfer", Title: "Transfer 穿梭框", Order: 71, Render: func(c *ui.Context) {
		s := &miscState
		entryCol(c, func() {
			ui.Text(c, "基本用法").Tertiary()
			ui.Transfer(c, items, &s.target).Titles("候选", "已选").DefaultChecked("k1", "k2", "k6")
			ui.Text(c, "带搜索、空状态").Tertiary()
			ui.Transfer(c, items[:3], &s.target2).Searchable().ListSize(220, 260)
			ui.Text(c, "禁用 / 校验失败").Tertiary()
			ui.Row(c, func() {
				ui.Transfer(c, items[:2], &s.target2).Disabled(true).ListSize(160, 150)
				ui.Transfer(c, items[:2], &s.target2).Status("error").ListSize(160, 150)
			}, layout.Gap(24))
		})
	}})

	entryPage(gallery.Page{Name: "Upload", Title: "Upload 上传", Order: 72, Render: func(c *ui.Context) {
		s := &miscState
		entryRow(c, func() {
			entryCol(c, func() {
				ui.Text(c, "默认（文本列表）").Tertiary()
				ui.Upload(c, &s.files).Tip("单个文件不超过 10MB").Width(360)
			})
			entryCol(c, func() {
				ui.Text(c, "禁用").Tertiary()
				ui.Upload(c, &s.drag).Disabled(true)
			})
		})
		ui.Gap(c, 16)
		ui.Text(c, "照片墙").Tertiary()
		ui.Gap(c, 8)
		ui.Upload(c, &s.cards).PictureCard()
		ui.Gap(c, 16)
		ui.Text(c, "拖拽上传").Tertiary()
		ui.Gap(c, 8)
		ui.Upload(c, &s.drag).Dragger().Width(520)
	}})

	entryPage(gallery.Page{Name: "CheckAll", Title: "Checkbox 全选 / 半选", Order: 73, Render: func(c *ui.Context) {
		s := &miscState
		opts := ui.Opts("苹果", "香蕉", "橙子", "葡萄")
		entryCol(c, func() {
			ui.Text(c, "部分选中（半选）").Tertiary()
			ui.CheckAll(c, &s.all, opts)
			ui.Divider(c)
			ui.Text(c, "全部可选项已选中（含禁用项）").Tertiary()
			ui.CheckAll(c, &s.all2, []ui.Option{{Label: "苹果", Value: "苹果"}, {Label: "香蕉", Value: "香蕉"}, {Label: "橙子", Value: "橙子"}, {Label: "葡萄（禁用）", Value: "葡萄", Disabled: true}})
			ui.Divider(c)
			ui.Text(c, "按钮样式单选（已有 RadioGroup.Buttons / Solid）").Tertiary()
			v := "上海"
			ui.RadioGroup(c, &v, ui.Opts("北京", "上海", "广州")).Buttons()
		})
	}})
}
