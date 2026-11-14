package main

import (
	"strconv"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/icon/antd"
	"github.com/tangoui/tango/ui/icon/element"
	"github.com/tangoui/tango/ui/icon/filetype"
	"github.com/tangoui/tango/ui/icon/lucide"
	"github.com/tangoui/tango/ui/layout"
)

// The icon browser state.
var icons = struct {
	set, query string
	page       int
}{set: "lucide", page: 1}

const iconsPerPage = 180

func iconCell(c *ui.Context, ic *icon.Icon, label string) {
	ui.Tooltip(c, label, func() {
		ui.Column(c, func() { ui.Icon(c, ic).Size(20) }, layout.Size(40, 40), layout.Center())
	})
}

func renderIconPage(c *ui.Context) {
	gallery.Demo(c, "内置图标", "ui/icon 自带的原创图标（24 网格）。组件画内置图标时，主题会换成各自设计体系的图标："+
		"Ant Design 主题用 @ant-design/icons，Element 主题用 Element Plus 图标，shadcn 主题用 Lucide。", func() {
		ui.Row(c, func() {
			for _, n := range icon.Names() {
				if strings.Contains(n, ":") {
					continue
				}
				iconCell(c, icon.ByName(n), n)
			}
		}, layout.WrapItems(), layout.Gap(4))
	})
	gallery.Demo(c, "状态图标", "实心状态图标的符号是镂空的，透出背景色。", func() {
		ui.Row(c, func() {
			ui.Icon(c, icon.CheckFilled).Size(28).Color(c.Theme.Success.Base)
			ui.Icon(c, icon.InfoFilled).Size(28).Color(c.Theme.Primary.Base)
			ui.Icon(c, icon.WarningFilled).Size(28).Color(c.Theme.Warning.Base)
			ui.Icon(c, icon.CloseFilled).Size(28).Color(c.Theme.Error.Base)
			ui.Icon(c, antd.CheckCircleTwoTone).Size(28).Color(c.Theme.Primary.Base)
			ui.Icon(c, antd.HeartTwoTone).Size(28).Color(c.Theme.Error.Base)
			ui.Icon(c, icon.Loading).Size(28).Spin()
		}, layout.Gap(12), layout.Middle())
	})
	gallery.Demo(c, "文件类型图标", "filetype 按文件名解析类别，图标随设计体系切换。", func() {
		set := filetype.ForFlavor(c.Theme.Flavor)
		ui.Row(c, func() {
			for _, n := range []string{"src/", "main.go", "App.vue", "index.ts", "style.css", "data.json", "README.md",
				"logo.png", "intro.mp4", "song.mp3", "app.db", "dist.zip", "Dockerfile", ".gitignore", "report.pdf", "notes"} {
				dir := strings.HasSuffix(n, "/")
				ui.Row(c, func() {
					ui.Icon(c, set.Icon(strings.TrimSuffix(n, "/"), dir, false)).Size(16)
					ui.Text(c, n)
				}, layout.Gap(6), layout.Middle(), layout.W(120))
			}
		}, layout.WrapItems(), layout.Gap(8))
	})
	gallery.Demo(c, "图标库", "Element Plus 293 个、Ant Design 848 个（线框 / 实底 / 双色）、Lucide 2052 个，全部可用；"+
		"程序只链接实际引用到的图标。悬停查看名称，名称可用于 icon.ByName（先调用该库的 Register）。", func() {
		var all []*icon.Icon
		switch icons.set {
		case "element":
			all = element.All()
		case "antd":
			all = antd.All()
		default:
			all = lucide.All()
		}
		q := strings.ToLower(strings.TrimSpace(icons.query))
		var hits []*icon.Icon
		for _, ic := range all {
			if q == "" || strings.Contains(ic.Name, q) {
				hits = append(hits, ic)
			}
		}
		ui.Row(c, func() {
			if ui.Segmented(c, &icons.set, []ui.Option{
				{Label: "Lucide (shadcn)", Value: "lucide"}, {Label: "Ant Design", Value: "antd"}, {Label: "Element Plus", Value: "element"},
			}).Changed() {
				icons.page = 1
			}
			ui.Input(c, &icons.query).Placeholder("搜索图标名").Prefix(icon.Search).AllowClear().Layout(layout.W(220))
			ui.Text(c, strconv.Itoa(len(hits))+" 个").Secondary()
		}, layout.Gap(12), layout.Middle())
		pages := max(1, (len(hits)+iconsPerPage-1)/iconsPerPage)
		icons.page = min(max(icons.page, 1), pages)
		lo := (icons.page - 1) * iconsPerPage
		hi := min(lo+iconsPerPage, len(hits))
		ui.Row(c, func() {
			for _, ic := range hits[lo:hi] {
				iconCell(c, ic, ic.Name)
			}
		}, layout.WrapItems(), layout.Gap(4))
		if pages > 1 {
			ui.Pagination(c, &icons.page, len(hits), iconsPerPage).Show()
		}
	})
}
