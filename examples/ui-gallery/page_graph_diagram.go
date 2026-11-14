package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/graph"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// graphPage registers a ui/graph demo; `-theme all` renders it in every
// design system and its dark variant.
func graphPage(p gallery.Page) {
	p.Group = gallery.Display
	if p.Width == 0 {
		p.Width = 1100
	}
	gallery.Register(p)
}

// flowGraph is a small service topology.
func flowGraph() *graph.Graph {
	n := func(id, label, detail string, ic *icon.Icon, status string) graph.Node {
		return graph.Node{ID: id, Label: label, Detail: detail, Icon: ic, Status: status}
	}
	g := &graph.Graph{Nodes: []graph.Node{
		n("web", "Web 前端", "vue-app", icon.Monitor, ""),
		n("mobile", "移动端", "iOS / Android", icon.Appstore, ""),
		n("gw", "API 网关", "nginx · 3 实例", icon.Global, "processing"),
		n("auth", "认证服务", "OAuth2", icon.Lock, ""),
		n("order", "订单服务", "go · 8 实例", icon.Shop, "success"),
		n("pay", "支付服务", "降级中", icon.Key, "warning"),
		{ID: "check", Label: "库存充足？", Shape: graph.ShapeDiamond},
		n("stock", "库存服务", "", icon.Inbox, ""),
		n("mq", "消息队列", "kafka", icon.Send, ""),
		n("notify", "通知服务", "", icon.Bell, ""),
		n("db", "主数据库", "PostgreSQL", icon.Database, ""),
		n("cache", "缓存", "Redis", icon.Cpu, "error"),
	}}
	e := func(from, to, label string) graph.Edge { return graph.Edge{From: from, To: to, Label: label} }
	g.Edges = []graph.Edge{
		e("web", "gw", "HTTPS"), e("mobile", "gw", "HTTPS"),
		e("gw", "auth", ""), e("gw", "order", ""),
		e("order", "check", ""), e("check", "stock", "是"), e("check", "notify", "否"),
		e("order", "pay", "下单"), e("pay", "mq", ""), e("mq", "notify", ""),
		e("order", "db", ""), e("stock", "db", ""), e("auth", "cache", ""),
		{From: "order", To: "cache", Dashed: true, Arrow: graph.ArrowNone},
	}
	return g
}

// pipelineGraph is a CI pipeline drawn with orthogonal edges.
func pipelineGraph() *graph.Graph {
	ids := []struct{ id, label, status string }{
		{"src", "拉取代码", "success"}, {"lint", "静态检查", "success"}, {"unit", "单元测试", "success"},
		{"build", "构建镜像", "processing"}, {"e2e", "端到端测试", ""}, {"stage", "预发布", ""}, {"prod", "生产发布", ""},
	}
	g := &graph.Graph{}
	for _, it := range ids {
		g.Nodes = append(g.Nodes, graph.Node{ID: it.id, Label: it.label, Status: it.status, Shape: graph.ShapePill})
	}
	for _, e := range [][2]string{{"src", "lint"}, {"src", "unit"}, {"lint", "build"}, {"unit", "build"}, {"build", "e2e"}, {"e2e", "stage"}, {"stage", "prod"}, {"unit", "stage"}} {
		g.Edges = append(g.Edges, graph.Edge{From: e[0], To: e[1]})
	}
	return g
}

var (
	diagFlow     = flowGraph()
	diagPipeline = pipelineGraph()
	diagSel      []string
	diagView     *graph.DiagramView
)

func init() {
	graphPage(gallery.Page{Name: "Diagram", Title: "Diagram 关系图", Order: 300,
		Render: func(c *ui.Context) {
			ui.Card(c, "服务拓扑（分层自动布局 · 贝塞尔连线 · 小地图 · 网格吸附）").Layout(layout.Margin(geom.Insets{Bottom: 16})).Body(func() {
				diagView = graph.Diagram(c, diagFlow).Height(460).Grid(16).Snap().MiniMap().
					AutoLayout(graph.LayoutOptions{Direction: graph.LeftRight, RankSep: 72, NodeSep: 24}).
					Selection(&diagSel).Show()
			})
			ui.Card(c, "流水线（自上而下 · 正交折线 · 框选拖动）").Body(func() {
				graph.Diagram(c, diagPipeline).Height(420).Edges(graph.EdgeOrthogonal).MarqueeDrag().
					AutoLayout(graph.LayoutOptions{Direction: graph.TopDown, RankSep: 48}).Show()
			})
		},
		Setup: func(d *ui.Driver) {
			if diagView == nil {
				return
			}
			if r, ok := diagView.ScreenRect("order"); ok {
				d.Click(r.Center())
			}
			if r, ok := diagView.ScreenRect("pay"); ok {
				d.Move(r.Center())
			}
		},
	})
}
