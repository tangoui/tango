package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func gridCell(c *ui.Context, label string, dark bool) {
	th := c.Theme
	bg := th.Primary.Base
	if !dark {
		bg = th.Primary.Shades[4]
	}
	ui.Column(c, func() { ui.Text(c, label).Color(theme.Hex("#ffffff")).NoWrap() }, layout.H(40), layout.Center()).Bg(bg).Radius(th.RadiusSM)
}

func gridPage(c *ui.Context) {
	navDemo(c, "24 栅格", "", func() {
		ui.Column(c, func() {
			for _, spans := range [][]int{{24}, {12, 12}, {8, 8, 8}, {6, 6, 6, 6}} {
				ui.GridRow(c).Body(func() {
					for i, s := range spans {
						i, s := i, s
						ui.Col(c, s).Body(func() { gridCell(c, fmt.Sprintf("col-%d", s), i%2 == 0) })
					}
				})
			}
		}, layout.Gap(8))
	})
	navDemo(c, "间距 gutter 与偏移 offset", "gutter 16/8；第二行 offset 8。", func() {
		ui.GridRow(c).Gutter(16, 8).Body(func() {
			for i := 0; i < 8; i++ {
				i := i
				ui.Col(c, 6).Body(func() { gridCell(c, "col-6", i%2 == 0) })
			}
		})
		ui.Gap(c, 16)
		ui.GridRow(c).Gutter(16, 0).Body(func() {
			ui.Col(c, 8).Body(func() { gridCell(c, "col-8", true) })
			ui.Col(c, 8).Offset(8).Body(func() { gridCell(c, "col-8 offset-8", false) })
		})
	})
	navDemo(c, "排版 justify 与响应式", fmt.Sprintf("当前断点：%s（窗口宽度 %.0f）。最后一行 xs=24 / md=12 / lg=6。", ui.ScreenBreakpoint(c), c.Size().W), func() {
		for _, j := range []layout.Justify{layout.JustifyCenter, layout.JustifyEnd, layout.JustifySpaceBetween, layout.JustifySpaceAround} {
			ui.GridRow(c).Justify(j).Body(func() {
				for i := 0; i < 4; i++ {
					i := i
					ui.Col(c, 4).Body(func() { gridCell(c, "col-4", i%2 == 0) })
				}
			})
			ui.Gap(c, 8)
		}
		ui.GridRow(c).Gutter(8, 8).Body(func() {
			for i := 0; i < 4; i++ {
				i := i
				ui.Col(c, 24).MD(12).LG(6).Body(func() { gridCell(c, "响应式", i%2 == 0) })
			}
		})
	})
}

var splitState = struct{}{}

func flexPage(c *ui.Context) {
	th := c.Theme
	navDemo(c, "Flex 弹性布局", "gap small/middle/large、justify、竖排、换行。", func() {
		ui.Flex(c).Vertical().Gap(ui.Middle).Body(func() {
			ui.Flex(c).Gap(ui.Small).Body(func() {
				for _, s := range []string{"主按钮", "次按钮", "虚线按钮"} {
					ui.Button(c, s)
				}
			})
			ui.Flex(c).Justify(layout.JustifySpaceBetween).Body(func() {
				for i := 0; i < 4; i++ {
					ui.Column(c, func() {}, layout.Size(120, 40)).Bg(th.Primary.Shades[3+i%2]).Radius(th.RadiusSM)
				}
			}).Style(layout.Pad(8)).Border(1, th.Primary.Base).Radius(th.Radius)
			ui.Flex(c).Wrap().Gap(ui.Small).Body(func() {
				for i := 0; i < 14; i++ {
					ui.Button(c, fmt.Sprintf("按钮 %d", i+1))
				}
			})
		})
	})
}

func splitterPage(c *ui.Context) {
	th := c.Theme
	pane := func(s string) func() {
		return func() {
			ui.Column(c, func() { ui.Text(c, s).Tertiary() }, layout.Grow(1), layout.Center())
		}
	}
	navDemo(c, "Splitter 分隔面板", "拖动分隔条调整大小；悬停分隔条显示折叠箭头。", func() {
		ui.Splitter(c).Style(layout.H(200)).Body(func() {
			ui.SplitterPanel(c).DefaultPercent(0.3).Min(80).Collapsible().Body(pane("左侧面板"))
			ui.SplitterPanel(c).Body(pane("中间面板"))
			ui.SplitterPanel(c).DefaultPercent(0.25).Collapsible().Body(pane("右侧面板"))
		}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
	})
	navDemo(c, "单箭头折叠", "IDE 侧栏样式：悬停分隔条出现一个箭头，带提示，折叠后箭头反向。", func() {
		ui.Splitter(c).Style(layout.H(200)).Body(func() {
			ui.SplitterPanel(c).DefaultPercent(0.3).Min(120).CollapseToggle("折叠工程侧栏", "展开工程侧栏").Body(pane("工程侧栏"))
			ui.SplitterPanel(c).Body(pane("编辑区"))
		}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
	})
	navDemo(c, "竖向与嵌套", "", func() {
		ui.Splitter(c).Vertical().Style(layout.H(260)).Body(func() {
			ui.SplitterPanel(c).DefaultPercent(0.4).Body(pane("上"))
			ui.SplitterPanel(c).Body(func() {
				ui.Splitter(c).Style(layout.Grow(1)).Body(func() {
					ui.SplitterPanel(c).Body(pane("左下"))
					ui.SplitterPanel(c).Resizable(false).Body(pane("右下（不可拖动）"))
				})
			})
		}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
	})
	navDemo(c, "延迟提交（Lazy）与拖拽事件", "拖动时只显示预览线，松手才提交比例；DragStarted / DragEnded 单独上报。", func() {
		sp := ui.Splitter(c).Lazy().Style(layout.H(160))
		sp.Body(func() {
			ui.SplitterPanel(c).DefaultPercent(0.4).Min(80).Body(pane("预览线"))
			ui.SplitterPanel(c).Min(80).Body(pane("松手提交"))
		}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
		if _, ok := sp.DragStarted(); ok {
			splitterLazyLog = "拖动中…"
		}
		if _, ok := sp.DragEnded(); ok {
			splitterLazyLog = fmt.Sprintf("已提交：%.0f%% / %.0f%%", sp.Sizes()[0]*100, sp.Sizes()[1]*100)
		}
		ui.Text(c, splitterLazyLog).Secondary()
	})
}

var splitterLazyLog = "拖动分隔条试试"

func init() {
	navPage(gallery.Page{Name: "Grid", Title: "Grid 栅格", Group: gallery.Layout, Order: 10, Render: gridPage}, nil)
	navPage(gallery.Page{Name: "Flex", Title: "Flex 弹性布局", Group: gallery.Layout, Order: 11, Render: flexPage}, nil)
	navPage(gallery.Page{Name: "Splitter", Title: "Splitter 分隔面板", Group: gallery.Layout, Order: 12, Render: splitterPage,
		Setup: func(d *ui.Driver) {
			// Drag the second bar 80px to the left, then hover it.
			if r, ok := d.Find("splitter-bar-2"); ok {
				p := r.Center()
				d.Inject(pointerPress(p))
				d.Frame()
				p.X -= 80
				d.Move(p)
				d.Inject(pointerRelease(p))
				d.Frame()
				d.Move(geom.Pt(p.X, p.Y+60))
				d.Settle()
			}
		}}, nil)
	_ = geom.Pt
}
