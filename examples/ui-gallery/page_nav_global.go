package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var cfgState = struct {
	page  int
	open  bool
	group bool
}{page: 2}

func configPage(c *ui.Context) {
	th := c.Theme
	sample := func() {
		loc := c.Locale()
		ui.Column(c, func() {
			ui.Row(c, func() {
				ui.Button(c, loc.OK).Primary()
				ui.Button(c, loc.Cancel)
				ui.Dropdown(c, ddItems[:3]).Button(loc.More)
			}, layout.Gap(8), layout.Middle())
			ui.Steps(c, []ui.StepItem{{Title: loc.Success}, {Title: loc.Loading}, {Title: loc.Confirm}}).Current(1).Small().Show()
			ui.Text(c, fmt.Sprintf("%s · %s · %s · %s", loc.NoData, loc.Total(85), loc.PageSizeLabel(10), loc.BackTop)).Secondary()
			ui.Spin(c).Tip(loc.Loading)
		}, layout.Gap(12))
	}
	navDemo(c, "全局尺寸 small / middle / large", "ConfigProvider.Size 影响内部未显式指定尺寸的组件。", func() {
		ui.Row(c, func() {
			for _, s := range []ui.ControlSize{ui.Small, ui.Middle, ui.Large} {
				s := s
				ui.ConfigProvider(c).Size(s).Body(func() {
					ui.Dropdown(c, ddItems[:3]).Button(map[ui.ControlSize]string{ui.Small: "small", ui.Middle: "middle", ui.Large: "large"}[s])
				})
			}
		}, layout.Gap(16), layout.Middle())
	})
	navDemo(c, "国际化 Locale：zh-CN / en-US", "同一段界面分别包在两个 ConfigProvider 中；注册新语言用 ui.RegisterLocale。", func() {
		navCols(c, func() {
			ui.Text(c, "zh-CN").Strong()
			ui.Gap(c, 8)
			ui.ConfigProvider(c).Locale(ui.ZhCN).Body(sample)
		}, func() {
			ui.Text(c, "en-US").Strong()
			ui.Gap(c, 8)
			ui.ConfigProvider(c).LocaleCode("en-US").Body(sample)
		})
	})
	navDemo(c, "组件默认属性与禁用", "Defaults(\"Space\", Props{\"size\": Large}) 设置默认间距；ComponentDisabled 禁用内部所有组件。", func() {
		ui.ConfigProvider(c).Defaults("Space", ui.Props{"size": ui.Large}).ComponentDisabled(true).Body(func() {
			ui.Space(c).Body(func() {
				ui.Button(c, "禁用按钮")
				ui.Button(c, "禁用主按钮").Primary()
				ui.Link(c, "禁用链接")
			})
		})
	})
	_ = th
}

func floatPage(c *ui.Context) {
	navDemo(c, "FloatButton 悬浮按钮", "右下角依次为：按钮组（点击展开菜单）、方形按钮组、带描述与徽标的按钮、主色圆形按钮。", func() {
		ui.Text(c, "悬浮按钮固定在窗口角落，不随页面滚动。").Tertiary()
	})
	ui.FloatButton(c).Icon(icon.Question).Primary().Tooltip("帮助").Right(24).Bottom(40)
	ui.FloatButton(c).Icon(icon.Message).Description("客服").Badge(5).Right(88).Bottom(40)
	ui.FloatButtonGroup(c).Square().Position(152, 40).Body(func() {
		ui.FloatButton(c).Icon(icon.Question).A11yLabel("帮助")
		ui.FloatButton(c).Icon(icon.Message).Badge(-1).A11yLabel("消息")
		ui.FloatButton(c).Icon(icon.Setting).A11yLabel("设置")
	})
	ui.FloatButtonGroup(c).Trigger("click").Open(&cfgState.group).Primary().Position(216, 40).Body(func() {
		ui.FloatButton(c).Icon(icon.Message).A11yLabel("消息")
		ui.FloatButton(c).Icon(icon.Edit).A11yLabel("编辑")
	})
	ui.FloatButtonGroup(c).Position(280, 40).Body(func() {
		ui.FloatButton(c).Icon(icon.Bell).Badge(12).A11yLabel("通知")
		ui.FloatButton(c).Icon(icon.Star).A11yLabel("收藏")
	})
}

func scrollPage(c *ui.Context) {
	th := c.Theme
	navDemo(c, "Scroll 滚动容器", "滚动条悬停显示、悬停滑块变粗，可拖拽滑块，点击轨道跳转；Always 常显。", func() {
		navCols(c, func() {
			ui.Scroll(c, func() {
				for i := 1; i <= 20; i++ {
					ui.Column(c, func() { ui.Text(c, fmt.Sprintf("%d", i)).Color(th.Primary.Base) }, layout.H(40), layout.Center(), layout.Margin(navPad(8))).
						Bg(th.Primary.Bg).Radius(th.Radius)
				}
			}, layout.H(220)).Always()
		}, func() {
			ui.ScrollH(c, func() {
				ui.Row(c, func() {
					for i := 1; i <= 30; i++ {
						ui.Column(c, func() { ui.Text(c, fmt.Sprintf("%d", i)).Color(th.Warning.Base) }, layout.Size(80, 60), layout.Center()).
							Bg(th.Warning.Bg).Radius(th.Radius)
					}
				}, layout.Gap(8), layout.Margin(navPad(14)))
			}).Always()
		})
	})
	navDemo(c, "百万行", "VirtualList 一百万行，偏移量为 float64，滚到末尾行位置仍精确。", func() {
		vl := ui.VirtualList(c, 1000000, 32, func(i int) {
			ui.Row(c, func() { ui.Text(c, fmt.Sprintf("第 %d 行", i+1)) }, layout.Grow(1), layout.Middle(), layout.PadVH(0, 12))
		}).Height(192)
		if !scrollDemo.jumped {
			scrollDemo.jumped = true
			vl.ScrollToIndex(999990)
		}
	})
}

var scrollDemo struct{ jumped bool }

func init() {
	navPage(gallery.Page{Name: "ConfigProvider", Title: "ConfigProvider 全局配置 / Locale 国际化", Group: gallery.General, Order: 90, Render: configPage}, nil)
	navPage(gallery.Page{Name: "FloatButton", Title: "FloatButton 悬浮按钮", Group: gallery.General, Order: 91, Height: 420, Render: floatPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("float"); ok {
				_ = r
			}
		}}, func() { cfgState.group = true })
	navPage(gallery.Page{Name: "Scroll", Title: "Scroll 滚动容器", Group: gallery.Layout, Order: 14, Render: scrollPage,
		Setup: func(d *ui.Driver) {
			// Hover the vertical thumb so the wider hover state shows.
			if r, ok := d.Find("1"); ok {
				d.Move(geom.Pt(r.Right()+12, r.Y+10))
			}
		}}, nil)
}
