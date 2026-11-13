package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

type p3Project struct {
	key, name, desc, owner, status string
	stars                          int
	progress                       float64
}

func p3Projects() []p3Project {
	names := []string{"Tango UI", "Duet 框架", "MinCode", "云中继", "名片插件", "抓包代理", "断点调试"}
	descs := []string{"即时模式自绘 GUI，对标 Ant Design 与 Element Plus", "自研 Go Web 框架，约定优于配置", "桌面端 AI 编程助手",
		"跨网络的会话中继服务", "上架市场的名片插件", "HTTP/HTTPS 抓包代理", "多语言断点调试器"}
	sts := []string{"online", "running", "online", "error", "online", "running", "online"}
	var out []p3Project
	for i, n := range names {
		out = append(out, p3Project{key: fmt.Sprint("p", i), name: n, desc: descs[i], owner: []string{"曲丽丽", "张三", "李四"}[i%3],
			status: sts[i], stars: 120 + i*37, progress: float64(30 + i*10)})
	}
	return out
}

func p3Metas() ui.ProListMetas[p3Project] {
	return ui.ProListMetas[p3Project]{
		Title:       func(r p3Project) string { return r.name },
		Description: func(r p3Project) string { return r.desc },
		Avatar: func(c *ui.Context, r p3Project) {
			ui.Avatar(c, string([]rune(r.name)[:1])).Size(40)
		},
		SubTitle: func(c *ui.Context, r p3Project) {
			e := map[string]string{"online": "success", "running": "processing", "error": "error"}[r.status]
			ui.Tag(c, map[string]string{"online": "已上线", "running": "开发中", "error": "异常"}[r.status]).Color(e)
		},
		Content: func(c *ui.Context, r p3Project) {
			ui.Column(c, func() {
				ui.Text(c, "负责人 "+r.owner).Tertiary()
				ui.Progress(c, r.progress).StrokeWidth(6)
			})
		},
		Actions: func(c *ui.Context, r p3Project) {
			ui.Link(c, "编辑")
			ui.Link(c, "查看")
		},
	}
}

func init() {
	pro3Page("ProList", "ProList 高级列表", gallery.Display, 61, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		rows := p3Projects()
		var sel = []string{"p1"}
		var sel2 = []string{"p2"}
		return func(c *ui.Context) {
				tbDemo(c, "列表模式", "metas 描述标题、副标题、描述、头像、内容和操作；工具栏、可选择、可展开、分页。", func() {
					ui.ProList(c, rows, p3Metas()).HeaderTitle("项目列表").RowKey(func(r p3Project) string { return r.key }).
						ToolBar(func() { ui.Button(c, "新建项目").Primary().Icon(icon.Plus) }).
						Selection(&sel).Pagination(4).
						Expandable(func(c *ui.Context, r p3Project) {
							ui.Text(c, fmt.Sprintf("%s · 星标 %d · 负责人 %s", r.desc, r.stars, r.owner)).Tertiary()
						}).Show()
				})
				tbDemo(c, "卡片网格", "Grid(3)：每项渲染为卡片，点击卡片切换选中。", func() {
					ui.ProList(c, rows[:6], ui.ProListMetas[p3Project]{
						Title:       func(r p3Project) string { return r.name },
						Description: func(r p3Project) string { return r.desc },
						Avatar:      func(c *ui.Context, r p3Project) { ui.Avatar(c, string([]rune(r.name)[:1])) },
						Actions:     func(c *ui.Context, r p3Project) { ui.Link(c, "详情") },
						Extra:       func(c *ui.Context, r p3Project) { ui.Text(c, fmt.Sprintf("★ %d", r.stars)).Tertiary() },
					}).Grid(3).RowKey(func(r p3Project) string { return r.key }).Selection(&sel2).Show()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("展开")
			}
	})
}
