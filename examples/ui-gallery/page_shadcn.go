package main

import (
	"fmt"
	"github.com/tangoui/tango/ui/a11y"
	"math"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Pages of the components that come from shadcn/ui (Command, Toggle,
// ToggleGroup, Kbd) and the shadcn dashboard example. They render in every
// theme; run with -theme shadcn for the shadcn screenshots.

var (
	cmdPicked   string
	toggleBold  = true
	toggleItal  bool
	toggleUnder bool
	toggleMulti = []string{"bold"}
	toggleAlign = []string{"left"}
	toggleOut   = []string{"b"}
)

var cmdItems = []ui.CommandItem{
	{Value: "calendar", Label: "日历", Group: "建议", Icon: icon.Calendar},
	{Value: "emoji", Label: "搜索表情", Group: "建议", Icon: icon.Smile},
	{Value: "calc", Label: "计算器", Group: "建议", Icon: icon.Appstore, Disabled: true},
	{Value: "profile", Label: "个人资料", Group: "设置", Icon: icon.User, Shortcut: "⌘P"},
	{Value: "billing", Label: "账单", Group: "设置", Icon: icon.File, Shortcut: "⌘B"},
	{Value: "settings", Label: "设置", Group: "设置", Icon: icon.Setting, Shortcut: "⌘S"},
}

func init() {
	gallery.Register(gallery.Page{Name: "Command", Title: "Command 命令面板", Group: gallery.Navigation, Order: 30, Height: 520,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "命令面板", "输入关键字过滤，↑↓ 移动，Enter 或点击选择；分组、快捷键提示与禁用项。", func() {
				ui.Row(c, func() {
					w := ui.Command(c, cmdItems).Shadow().Layout(layout.W(420))
					if it, ok := w.Selected(); ok {
						cmdPicked = it.Label
					}
					ui.Column(c, func() {
						ui.Text(c, "已选择："+cmdPicked).Tertiary()
						ui.Row(c, func() {
							ui.Text(c, "打开面板")
							ui.Kbd(c, "⌘")
							ui.Kbd(c, "K")
						}, layout.Gap(4), layout.Middle())
					}, layout.Gap(8))
				}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("搜索表情")
			d.Settle()
			d.Key(input.KeyDown, 0)
			d.Settle()
		},
	})
	gallery.Register(gallery.Page{Name: "Toggle", Title: "Toggle 切换按钮 / ToggleGroup / Kbd", Group: gallery.General, Order: 5,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "Toggle", "按下保持开启；outline 变体带边框。", func() {
				ui.Row(c, func() {
					ui.Toggle(c, "加粗", &toggleBold).IconOnly(icon.Edit)
					ui.Toggle(c, "斜体", &toggleItal).Icon(icon.Edit)
					ui.Toggle(c, "下划线", &toggleUnder).Outline()
					off := false
					ui.Toggle(c, "禁用", &off).Disabled(true)
				}, layout.Gap(8), layout.Middle())
			})
			gallery.Demo(c, "ToggleGroup", "多选、单选与 outline 拼接样式。", func() {
				ui.Column(c, func() {
					ui.ToggleGroup(c, &toggleMulti, []ui.Option{{Label: "加粗", Value: "bold"}, {Label: "斜体", Value: "italic"}, {Label: "下划线", Value: "under"}})
					ui.ToggleGroup(c, &toggleAlign, []ui.Option{{Label: "左对齐", Value: "left"}, {Label: "居中", Value: "center"}, {Label: "右对齐", Value: "right"}}).Single().Outline()
					ui.ToggleGroup(c, &toggleOut, ui.Opts("a", "b", "c", "d")).Outline().Size(ui.Small).Icons(map[string]*icon.Icon{"a": icon.Home, "b": icon.Star, "c": icon.Bell, "d": icon.Setting})
				}, layout.Gap(12))
			})
			gallery.Demo(c, "Kbd", "键盘按键与快捷键。", func() {
				ui.Row(c, func() {
					ui.Kbd(c, "⌘")
					ui.Kbd(c, "Shift")
					ui.Kbd(c, "Ctrl + B")
					ui.Text(c, "保存：")
					ui.Kbd(c, "⌘ S")
				}, layout.Gap(8), layout.Middle())
			})
		},
	})
	gallery.Register(gallery.Page{Name: "ShadcnDashboard", Title: "shadcn 仪表盘", Group: gallery.Display, Order: 230, Width: 1440, Height: 1110, Bare: true,
		Render: shadcnDashboard,
		Setup:  func(d *ui.Driver) { hoverAt(d, &sdVisitorsView, 0.62, 0.5) },
	})
}

var (
	sdNav          = "dashboard"
	sdRange        = []string{"90d"}
	sdVisitorsView chart.View
)

var sdVisitors = func() []chart.Row {
	var out []chart.Row
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 91; i++ {
		d := start.AddDate(0, 0, i)
		x := float64(i)
		desk := 220 + 120*math.Sin(x/6) + 60*math.Sin(x/2.3) + x*1.6
		mob := 150 + 80*math.Sin(x/5+1) + 40*math.Cos(x/1.7) + x*1.1
		out = append(out, chart.Row{"date": d, "type": "桌面端", "visitors": math.Round(desk)}, chart.Row{"date": d, "type": "移动端", "visitors": math.Round(mob)})
	}
	return out
}()

var sdMonthly = func() []chart.Row {
	vals := []float64{186, 305, 237, 73, 209, 214, 268, 190, 312, 254, 280, 330}
	var out []chart.Row
	for i, v := range vals {
		out = append(out, chart.Row{"month": fmt.Sprintf("%d月", i+1), "sales": v * 12})
	}
	return out
}()

type sdTx struct{ name, email, amount string }

var sdTxs = []sdTx{
	{"Olivia Martin", "olivia.martin@email.com", "+¥1,999.00"},
	{"Jackson Lee", "jackson.lee@email.com", "+¥39.00"},
	{"Isabella Nguyen", "isabella.nguyen@email.com", "+¥299.00"},
	{"William Kim", "will@email.com", "+¥99.00"},
	{"Sofia Davis", "sofia.davis@email.com", "+¥39.00"},
}

// shadcnDashboard follows the layout of the shadcn/ui dashboard example:
// a sidebar, a header, four statistic cards, an area chart and a row with
// a bar chart and the recent transactions.
func shadcnDashboard(c *ui.Context) {
	th := c.Theme
	ui.Row(c, func() {
		sdSidebar(c)
		ui.Column(c, func() {
			// Header.
			ui.Row(c, func() {
				ui.Icon(c, icon.MenuFold).Size(16).Color(th.TextTertiary)
				ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 1, H: 16} }, func(l *paint.List, r geom.Rect) {
					l.Fill(r, geom.Radii{}, th.Split)
				})
				ui.Text(c, "仪表盘").Weight(text.Medium)
				ui.Spacer(c)
				ui.Button(c, "GitHub").Text().Small()
				ui.Button(c, "新建").Primary().Small().Icon(icon.Plus)
			}, layout.H(56), layout.PadVH(0, 24), layout.Gap(12), layout.Middle()).
				Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
				})
			ui.Column(c, func() {
				ui.Row(c, func() {
					sdStat(c, "总收入", "¥1,250.00", "+12.5%", true, "本月趋势向上", "近 6 个月访客")
					sdStat(c, "新客户", "1,234", "-20%", false, "本期下降 20%", "获客需要关注")
					sdStat(c, "活跃账户", "45,678", "+12.5%", true, "用户留存良好", "互动超出目标")
					sdStat(c, "增长率", "4.5%", "+4.5%", true, "稳定提升", "符合增长预期")
				}, layout.Gap(16))
				ui.Card(c, "访客总数").Extra(func() {
					ui.ToggleGroup(c, &sdRange, []ui.Option{{Label: "近 3 个月", Value: "90d"}, {Label: "近 30 天", Value: "30d"}, {Label: "近 7 天", Value: "7d"}}).Single().Outline().Size(ui.Small)
				}).Body(func() {
					ui.Text(c, "近 3 个月访客合计").Tertiary().Layout(layout.Margin(geom.Insets{Bottom: 8}))
					sdVisitorsView = chart.Area(c, sdVisitors).X("date").Y("visitors").Series("type").Smooth().Stack().Height(250).Show()
				})
				ui.Row(c, func() {
					ui.Column(c, func() {
						ui.Card(c, "概览").Body(func() {
							chart.Column(c, sdMonthly).X("month").Y("sales").Legend(chart.LegendNone).Height(300).Show()
						})
					}, layout.Grow(4), layout.Basis(0))
					ui.Column(c, func() {
						ui.Card(c, "最近交易").Body(func() {
							ui.Text(c, "本月共 265 笔交易").Tertiary().Layout(layout.Margin(geom.Insets{Bottom: 16}))
							ui.Column(c, func() {
								for _, tx := range sdTxs {
									ui.Row(c, func() {
										ui.Avatar(c, initials(tx.name))
										ui.Column(c, func() {
											ui.Text(c, tx.name).Weight(text.Medium)
											ui.Text(c, tx.email).Tertiary()
										}, layout.Grow(1), layout.Gap(2))
										ui.Text(c, tx.amount).Weight(text.Medium)
									}, layout.Gap(16), layout.Middle())
								}
							}, layout.Gap(22))
						})
					}, layout.Grow(3), layout.Basis(0))
				}, layout.Gap(16), layout.AlignItems(layout.AlignStretch))
			}, layout.Pad(24), layout.Gap(16))
		}, layout.Grow(1), layout.MinW(0))
	}, layout.Grow(1), layout.AlignItems(layout.AlignStretch))
}

func initials(name string) string {
	out := ""
	for i, r := range name {
		if i == 0 || (i > 0 && name[i-1] == ' ') {
			out += string(r)
		}
	}
	return out
}

func sdSidebar(c *ui.Context) {
	th := c.Theme
	ui.Div(c, layout.W(256), layout.Pad(8), layout.Gap(2)).Bg(th.SiderLightBg).
		Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(r, geom.Radii{}, th.SiderLightBg)
			l.Fill(geom.Rect{X: r.Right() - 1, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, th.Split)
		}).
		Body(func() {
			ui.Row(c, func() {
				ui.Div(c, layout.W(32), layout.H(32), layout.Center()).Bg(th.Primary.Base).Radius(th.RadiusLG).Body(func() {
					ui.Icon(c, icon.Appstore).Size(16).Color(th.TextOnPrimary)
				})
				ui.Column(c, func() {
					ui.Text(c, "Tango Inc.").Weight(text.Semibold)
					ui.Text(c, "企业版").Size(th.FontSizeSM).Tertiary()
				})
			}, layout.Gap(8), layout.Pad(8), layout.Middle())
			sdGroup(c, "平台")
			for _, it := range []struct {
				key, label string
				ic         *icon.Icon
			}{
				{"dashboard", "仪表盘", icon.Dashboard}, {"lifecycle", "生命周期", icon.Reload}, {"analytics", "分析", icon.BarChart},
				{"projects", "项目", icon.Folder}, {"team", "团队", icon.Team},
			} {
				sdNavItem(c, it.key, it.label, it.ic)
			}
			sdGroup(c, "文档")
			sdNavItem(c, "library", "数据库", icon.Inbox)
			sdNavItem(c, "reports", "报表", icon.FileText)
			sdNavItem(c, "assistant", "写作助手", icon.Edit)
			ui.Spacer(c)
			sdNavItem(c, "settings", "设置", icon.Setting)
			ui.Row(c, func() {
				ui.Avatar(c, "曲")
				ui.Column(c, func() {
					ui.Text(c, "曲丽丽").Weight(text.Medium)
					ui.Text(c, "m@example.com").Size(th.FontSizeSM).Tertiary()
				})
			}, layout.Gap(8), layout.Pad(8), layout.Middle())
		})
}

func sdGroup(c *ui.Context, title string) {
	ui.Text(c, title).Size(c.Theme.FontSizeSM).Weight(text.Medium).Tertiary().Layout(layout.PadVH(0, 8), layout.H(32), layout.Margin(geom.Insets{Top: 8}))
}

func sdNavItem(c *ui.Context, key, label string, ic *icon.Icon) {
	th := c.Theme
	ui.Key(c, key, func() {
		b := ui.Div(c, layout.H(32), layout.PadVH(0, 8))
		ia := b.Focusable(input.CursorPointer)
		if ia.Clicked {
			sdNav = key
		}
		on := sdNav == key
		b.Semantics(a11y.RoleLink, label)
		if on {
			b.A11yState(a11y.Current)
		}
		// The sidebar is the light one (SiderLightBg). shadcn's sider tokens
		// describe it (SiderBg == SiderLightBg); Ant Design's and Element's
		// describe their dark sider, so use the light surface's colors there.
		fg, fgOn, bg := th.SiderText, th.SiderTextActive, th.SiderItemHoverBg
		if th.SiderBg != th.SiderLightBg {
			fg, fgOn, bg = th.TextSecondary, th.Text, th.FillSecondary
		}
		if !on && !ia.Hovered {
			bg.A = 0
		}
		b.Bg(bg).Radius(th.Radius).Body(func() {
			ui.Row(c, func() {
				ui.Icon(c, ic).Size(16).Color(fg)
				t := ui.Text(c, label).Color(fg)
				if on {
					t.Weight(text.Medium).Color(fgOn)
				}
			}, layout.Gap(8), layout.Middle(), layout.Grow(1))
		})
	})
}

func sdStat(c *ui.Context, title, value, delta string, up bool, foot, sub string) {
	th := c.Theme
	ui.Column(c, func() {
		ui.Card(c, "").Body(func() {
			ui.Row(c, func() {
				ui.Text(c, title).Tertiary()
				ui.Spacer(c)
				ic := icon.Rise
				if !up {
					ic = icon.Fall
				}
				ui.Row(c, func() {
					ui.Icon(c, ic).Size(12).Color(th.Text)
					ui.Text(c, delta).Size(th.FontSizeSM).Weight(text.Medium)
				}, layout.Gap(4), layout.PadVH(2, 8), layout.Middle()).
					Paint(func(l *paint.List, r geom.Rect) { l.Stroke(r, geom.Rad(th.Radius), 1, th.Split) })
			}, layout.Middle())
			ui.Text(c, value).Size(30).Weight(text.Semibold).Layout(layout.Margin(geom.Insets{Top: 4, Bottom: 16}))
			ui.Text(c, foot).Weight(text.Medium)
			ui.Text(c, sub).Tertiary()
		})
	}, layout.Grow(1), layout.Basis(0))
}
