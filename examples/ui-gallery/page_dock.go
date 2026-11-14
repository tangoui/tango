package main

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Dock: an IDE workbench of tab groups in nested splits, and Tabs with
// drag sorting.

// dockTabs describes the demo's tab keys.
var dockTabs = map[string]ui.TabItem{
	"main.go":   {Label: "main.go", Icon: icon.Code},
	"dock.go":   {Label: "dock.go", Icon: icon.Code},
	"README.md": {Label: "README.md", Icon: icon.FileText},
	"terminal":  {Label: "终端", Icon: icon.Terminal, NoClose: true},
	"output":    {Label: "输出", Icon: icon.Monitor},
	"outline":   {Label: "大纲", Icon: icon.Appstore},
}

// dockDemoLayout is the starting layout: an editor group beside a column
// of tool groups.
func dockDemoLayout() *ui.DockLayout {
	l := ui.NewDockLayout("main.go", "dock.go", "README.md", "terminal", "output", "outline")
	l.SplitTab("terminal", "g1", ui.DockRight)
	l.MoveTab("output", "g2", -1)
	l.SplitTab("outline", "g2", ui.DockBottom)
	l.Activate("terminal")
	l.Activate("main.go")
	l.Root.Sizes = []float64{0.62, 0.38}
	return l
}

var (
	dockLayout     = dockDemoLayout()
	dockDropLayout = dockDemoLayout()
	dockSaved      string
)

// dockContent fakes the content of a tab.
func dockContent(c *ui.Context, key string) {
	ui.Column(c, func() {
		ui.Text(c, key).Strong()
		var lines []string
		switch {
		case strings.HasSuffix(key, ".go"):
			lines = []string{"package main", "", "func main() {", "    ui.Dock(c, layout).Show()", "}"}
		case key == "terminal":
			lines = []string{"$ go test ./ui", "ok   github.com/tangoui/tango/ui"}
		case key == "outline":
			lines = []string{"Dock", "DockLayout", "DockNode"}
		default:
			lines = []string{"拖动标签页到其它组的边缘拆分，拖到中间合并。"}
		}
		for _, s := range lines {
			ui.Text(c, s).Secondary()
		}
	}, layout.Pad(12), layout.Gap(4))
}

func dockDemo(c *ui.Context, l *ui.DockLayout) *ui.DockW {
	return ui.Dock(c, l).
		Tab(func(key string) ui.TabItem { return dockTabs[key] }).
		Content(func(key string) { dockContent(c, key) }).
		Style(layout.H(380)).
		Show()
}

func dockPage(c *ui.Context) {
	gallery.Demo(c, "停靠布局", "拖动标签页到另一组的标签栏、内容区边缘或中间；拖动分割条调整大小；右上角按钮向右拆分、最大化。布局可序列化为 JSON 保存。", func() {
		if dockDemo(c, dockLayout).Changed() {
			// Save the layout; ui.ParseDockLayout restores it.
			if b, err := json.Marshal(dockLayout); err == nil {
				dockSaved = string(b)
			}
		}
		if dockSaved != "" {
			ui.Text(c, "已保存 "+strconv.Itoa(len(dockSaved))+" 字节布局 JSON").Tertiary()
		}
	})
}

// dockDropPage shows the drop preview: a tab dragged onto the right part
// of another group.
func dockDropPage(c *ui.Context) {
	gallery.Demo(c, "拖放预览", "把 README.md 拖到右上组内容区的右侧：松开后在其右侧拆出新组。", func() {
		dockDemo(c, dockDropLayout)
	})
}

var (
	tabsDragItems = []ui.TabItem{{Key: "a", Label: "首页"}, {Key: "b", Label: "订单"}, {Key: "c", Label: "客户"}, {Key: "d", Label: "报表"}}
	tabsDragKey   = "a"
	tabsTorn      string
)

func tabsDragPage(c *ui.Context) {
	gallery.Demo(c, "拖拽排序与撕出", "Draggable 拖动标签页排序（Shift+Ctrl/⌘+方向键同样可移动）；OnTearOff 拖到窗口外松开时报告标签与位置，由应用创建新窗口。", func() {
		tb := ui.Tabs(c, &tabsDragKey, tabsDragItems).Closable().Draggable().
			OnTearOff(func(key string, _ geom.Point) { tabsTorn = key }).Show()
		if it := tb.Reordered(); it != nil {
			tabsDragItems = it
		}
		if key, _, ok := tb.TornOff(); ok {
			tabsDragItems = slices.DeleteFunc(tabsDragItems, func(it ui.TabItem) bool { return it.Key == key })
		}
		if k := tb.Closed(); k != "" {
			tabsDragItems = slices.DeleteFunc(tabsDragItems, func(it ui.TabItem) bool { return it.Key == k })
		}
		if tabsTorn != "" {
			ui.Text(c, "已撕出："+tabsTorn).Tertiary()
		}
	})
}

// pressDrag presses at from and moves to to without releasing (the
// screenshot shows the drag in progress).
func pressDrag(d *ui.Driver, from, to geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: from})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from})
	d.Frame()
	for i := 1; i <= 4; i++ {
		f := float32(i) / 4
		d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)})
		d.Frame()
	}
}

func init() {
	gallery.Register(gallery.Page{Name: "Dock", Title: "Dock 停靠布局", Group: gallery.Layout, Order: 60, Render: dockPage})
	gallery.Register(gallery.Page{Name: "DockDrop", Title: "Dock 拖放预览", Group: gallery.Layout, Order: 61, Parent: "Dock", Render: dockDropPage,
		Setup: dockDropSetup, Interact: dockDropSetup})
	gallery.Register(gallery.Page{Name: "TabsDraggable", Title: "Tabs 拖拽排序", Group: gallery.Navigation, Order: 3, Parent: "Tabs", Render: tabsDragPage,
		Setup: tabsDragSetup, Interact: tabsDragSetup})
}

func dockDropSetup(d *ui.Driver) {
	src, ok1 := d.Find("README.md")
	dst, ok2 := d.FindText("ok   github.com/tangoui/tango/ui")
	if !ok1 || !ok2 {
		return
	}
	pressDrag(d, src.Center(), geom.Pt(dst.X+dst.W-40, dst.Y+30))
}

func tabsDragSetup(d *ui.Driver) {
	a, ok1 := d.Find("首页")
	b, ok2 := d.Find("客户")
	if ok1 && ok2 {
		pressDrag(d, a.Center(), geom.Pt(b.Right()-4, b.Center().Y+6))
	}
}
