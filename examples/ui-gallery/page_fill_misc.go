package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/mobile"
)

// ---------------------------------------------------------------------------
// IconButton, Radio, TimePanel

var fillMiscDemo struct {
	likes   int
	radioA  bool
	plan    string
	at, at2 time.Time
}

func fillMiscReset() {
	d := &fillMiscDemo
	d.likes, d.radioA, d.plan = 3, true, "pro"
	d.at = time.Date(2026, 11, 12, 9, 30, 15, 0, time.UTC)
	d.at2 = time.Date(2026, 11, 12, 18, 45, 0, 0, time.UTC)
}

func init() {
	fillMiscReset()
	morePage(gallery.Page{Name: "IconButton", Title: "IconButton 图标按钮", Group: gallery.General, Order: 2,
		Render: func(c *ui.Context) {
			d := &fillMiscDemo
			gallery.Demo(c, "只有图标的按钮", "label 用于测试定位与无障碍名称；悬停出现浅色背景，键盘聚焦显示焦点环。", func() {
				moreRow(c, func() {
					for _, it := range []struct {
						ic    *icon.Icon
						label string
					}{{icon.Search, "搜索"}, {icon.Edit, "编辑"}, {icon.Delete, "删除"}, {icon.Copy, "复制"}, {icon.Setting, "设置"}, {icon.More, "更多"}} {
						ui.IconButton(c, it.ic, it.label)
					}
				})
				ui.Gap(c, 16)
				moreRow(c, func() {
					ui.IconButton(c, icon.Plus, "大号").Size(32, 20)
					ui.IconButton(c, icon.Close, "无背景").NoHoverBg()
					ui.IconButton(c, icon.Star, "收藏").Color(c.Theme.Error.Base)
					if ui.IconButton(c, icon.Like, "加一").Clicked() {
						d.likes++
					}
					ui.Text(c, fmt.Sprintf("已点赞 %d 次", d.likes)).Tertiary()
				})
			})
		},
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("编辑"); ok {
				d.Move(r.Center())
			}
			d.Key(input.KeyTab, 0)
		},
	}, fillMiscReset)

	morePage(gallery.Page{Name: "Radio", Title: "Radio 单个单选框", Group: gallery.Form, Order: 5,
		Render: func(c *ui.Context) {
			d := &fillMiscDemo
			gallery.Demo(c, "单个 Radio", "Radio 只报告 Clicked，由调用方决定选中值；多个 Radio 自行组成一组。", func() {
				moreRow(c, func() {
					if ui.Radio(c, "同意协议", d.radioA).Clicked() {
						d.radioA = !d.radioA
					}
					ui.Radio(c, "禁用未选", false).Disabled(true)
					ui.Radio(c, "禁用已选", true).Disabled(true)
				})
				ui.Gap(c, 12)
				moreRow(c, func() {
					for _, p := range []struct{ label, v string }{{"免费版", "free"}, {"专业版", "pro"}, {"企业版", "ent"}} {
						if ui.Radio(c, p.label, d.plan == p.v).Clicked() {
							d.plan = p.v
						}
					}
				})
			})
		},
		Setup: func(d *ui.Driver) { d.ClickLabel("企业版") },
	}, fillMiscReset)

	morePage(gallery.Page{Name: "TimePanel", Title: "TimePanel 时间面板", Group: gallery.Form, Order: 52,
		Render: func(c *ui.Context) {
			d := &fillMiscDemo
			gallery.Demo(c, "内嵌的时间滚动列", "TimePicker 的下拉面板单独使用；可聚焦，上下键改值、左右键切列。", func() {
				ui.Row(c, func() {
					ui.Column(c, func() {
						ui.TimePanel(c, &d.at)
						ui.Text(c, d.at.Format("15:04:05")).Tertiary()
					}, layout.Gap(8))
					ui.Column(c, func() {
						ui.TimePanel(c, &d.at2).Format("15:04").Steps(1, 15, 0).Height(200)
						ui.Text(c, d.at2.Format("15:04")+"（分钟步长 15）").Tertiary()
					}, layout.Gap(8))
				}, layout.Gap(32))
			})
		},
	}, fillMiscReset)
}

// ---------------------------------------------------------------------------
// mobile: Area / Within / Touchable, ShowToast / ClearToast, SafeAreaInsets

var fillMobileDemo struct {
	taps, longs int
	dx          float32
	last        string
}

func fillMobileReset() {
	fillMobileDemo.taps, fillMobileDemo.longs, fillMobileDemo.dx, fillMobileDemo.last = 0, 0, 0, ""
}

func fillMobileBody(c *ui.Context) {
	d := &fillMobileDemo
	th := mobile.ThemeOf(c)
	in := mobile.SafeAreaInsets(c)
	section := func(title string, fn func()) {
		ui.Text(c, title).Color(th.TextSecondary).Size(th.FontSmall).Layout(layout.Margin(geom.Insets{Top: 16, Bottom: 8, Left: th.PadH}))
		ui.Column(c, fn, layout.PadVH(12, th.PadH), layout.Gap(10)).Bg(th.Box)
	}
	ui.Column(c, func() {
		section("SafeAreaInsets", func() {
			ui.Text(c, fmt.Sprintf("上 %.0f · 下 %.0f · 左 %.0f · 右 %.0f（桌面窗口模拟 iPhone 安全区）", in.Top, in.Bottom, in.Left, in.Right)).Size(th.FontMain)
		})
		section("Area + Within + Touchable", func() {
			area := ui.Div(c, layout.H(120)).Radius(th.Radius).Clip()
			g := mobile.Area(c, area, mobile.AxisHorizontal, nil)
			if g.Pan.Active {
				d.dx = g.Pan.Delta.X
			}
			if g.Pan.Ended {
				d.dx = 0
				if g.Swiped {
					d.last = "滑动：" + g.Swipe.Dir.String()
				}
			}
			area.Bg(th.PrimaryLight)
			area.Body(func() {
				mobile.Within(c, area.ID(), func() {
					ui.Row(c, func() {
						for i, label := range []string{"点按", "长按"} {
							i, label := i, label
							b := ui.Div(c, layout.Size(96, 44), layout.Center()).Radius(th.ButtonRadius).Bg(th.Primary)
							pr := mobile.Touchable(c, b)
							b.Body(func() { ui.Text(c, label).Color(th.OnPrimary) })
							if pr.Pressed {
								b.Opacity(0.7)
							}
							if pr.Tap && i == 0 {
								d.taps++
								d.last = "点按"
							}
							if pr.LongPress && i == 1 {
								d.longs++
								d.last = "长按"
							}
						}
					}, layout.Gap(12), layout.Center(), layout.Grow(1))
				})
			})
			area.Transform(geom.Translate(d.dx*0.3, 0))
			ui.Text(c, fmt.Sprintf("点按 %d 次 · 长按 %d 次 · 最近：%s", d.taps, d.longs, d.last)).Size(th.FontSmall).Color(th.TextWeak)
		})
		section("ShowToast / ClearToast", func() {
			ui.Row(c, func() {
				if mobile.Button(c, "显示 Toast").Primary().Clicked() {
					mobile.ShowToast(c, "已保存", mobile.ToastOptions{Icon: "success", Duration: 3 * time.Second})
				}
				if mobile.Button(c, "清除 Toast").Clicked() {
					mobile.ClearToast(c)
				}
			}, layout.Gap(12))
		})
		ui.Spacer(c)
		mobile.SafeArea(c, mobile.Bottom)
	}, layout.Grow(1))
}

func init() {
	fillMobileReset()
	gallery.Register(gallery.Page{
		Name: "MobileGesture", Title: "Mobile · 手势、Toast 与安全区", Group: gallery.Display, Order: 960, Width: phoneW + 48,
		Render: func(c *ui.Context) {
			ui.Div(c, layout.Size(phoneW, phoneH)).Clip().Radius(24).Border(1, c.Theme.BorderSecondary).Body(func() {
				mobile.Provider(c, mobile.Options{SafeArea: &phoneInsets}, func() {
					th := mobile.ThemeOf(c)
					ui.Column(c, func() {
						statusBar(c, th, phoneInsets.Top)
						mobile.NavBar(c, "手势与 Toast")
						fillMobileBody(c)
					}, layout.Grow(1)).Bg(th.PageBg)
				})
			})
		},
		Setup: func(d *ui.Driver) {
			fillMobileReset()
			d.Settle()
			if r, ok := d.FindText("点按"); ok {
				d.Click(r.Center())
			}
			// mobile.Button has no test label: click below the section title.
			if r, ok := d.FindText("ShowToast / ClearToast"); ok {
				d.Click(geom.Pt(r.X+40, r.Bottom()+8+12+20))
			}
			d.Settle()
		},
	})
}
