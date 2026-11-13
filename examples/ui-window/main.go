// Command ui-window opens Tango's self-drawn UI in a real window.
//
//	go run ./examples/ui-window
//
// Everything on screen is drawn by package ui and shown through uihost
// (no WebView). Try: typing (including an IME such as Pinyin), Tab focus,
// the theme switch (by default the window follows the system's light/dark
// appearance), sorting the table, opening the dialog, resizing the
// window.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

type person struct {
	name, city string
	age        int
	score      float64
}

var (
	app    *tango.App
	runner *ui.Runner

	themeName = "跟随系统" // uihost follows the system's light/dark by default
	darkMode  bool
	tab       = "form"

	name, email, city, bio string
	agree, notify          bool    = false, true
	level                  float64 = 40
	stars                  float64 = 4
	clicks                 int
	dialogOpen             bool

	people = []person{
		{"张三", "北京", 28, 92.5}, {"李四", "上海", 34, 78}, {"王五", "杭州", 23, 88},
		{"赵六", "深圳", 41, 65.5}, {"孙七", "成都", 30, 99}, {"周八", "南京", 26, 71},
	}
	selected []int
)

// themeFor builds the chosen design system, light or dark.
func themeFor(n string, dark bool) *theme.Theme {
	switch n {
	case "Element Plus":
		if dark {
			return theme.ElementPlus(theme.Dark())
		}
		return theme.ElementPlus()
	case "shadcn":
		if dark {
			return theme.Shadcn(theme.Dark())
		}
		return theme.Shadcn()
	}
	if dark {
		return theme.AntDesign(theme.Dark())
	}
	return theme.AntDesign()
}

// applyTheme switches the window to the chosen theme after this frame:
// following the system (Ant, light or dark as the system is), or a fixed
// design system with the dark switch.
func applyTheme() {
	name, dark := themeName, darkMode
	app.Post(func() {
		if name == "跟随系统" {
			uihost.FollowSystemTheme(runner, true)
			return
		}
		uihost.FollowSystemTheme(runner, false) // a fixed theme from now on
		runner.SetTheme(themeFor(name, dark))
	})
}

func page(c *ui.Context) {
	ui.Scroll(c, func() {
		ui.Column(c, func() {
			ui.Row(c, func() {
				ui.Title(c, "Tango 自绘 UI", 3)
				ui.Spacer(c)
				if ui.Segmented(c, &themeName, ui.Opts("跟随系统", "Ant Design", "Element Plus", "shadcn")).Changed() {
					applyTheme()
				}
				ui.Text(c, "暗色").Tertiary()
				// While following the system the switch shows the system's
				// choice; turning it picks a fixed Ant theme.
				darkMode = c.Theme.Dark
				if ui.Switch(c, &darkMode).A11yLabel("暗色").Changed() {
					if themeName == "跟随系统" {
						themeName = "Ant Design"
					}
					applyTheme()
				}
			}, layout.Middle(), layout.Gap(12))

			ui.Row(c, func() {
				stat(c, "按钮点击", float64(clicks))
				stat(c, "已选人数", float64(len(selected)))
				stat(c, "评分", stars)
			}, layout.Gap(16))

			ui.Tabs(c, &tab, []ui.TabItem{{Key: "form", Label: "表单"}, {Key: "table", Label: "表格"}, {Key: "misc", Label: "其他"}}).Show()
			switch tab {
			case "form":
				formTab(c)
			case "table":
				tableTab(c)
			default:
				miscTab(c)
			}
		}, layout.Gap(16), layout.Pad(24))
	}, layout.Fill()) // Scroll needs a bounded height: fill the window
}

func stat(c *ui.Context, title string, v float64) {
	ui.Card(c, "").Layout(layout.Grow(1)).Body(func() {
		ui.Statistic(c, title, v, 0)
	})
}

func formTab(c *ui.Context) {
	f := ui.Form(c).LabelWidth(90)
	f.Body(func() {
		f.Item("姓名").Rules(&name, ui.Required("请输入姓名")).Body(func() {
			ui.Input(c, &name).Placeholder("可以试试拼音输入法").AllowClear()
		})
		f.Item("邮箱").Rules(&email, ui.Email("邮箱格式不正确")).Body(func() {
			ui.Input(c, &email).Placeholder("name@example.com")
		})
		f.Item("城市").Body(func() {
			ui.Select(c, &city, ui.Opts("北京", "上海", "杭州", "深圳", "成都")).Placeholder("请选择").Searchable().AllowClear()
		})
		f.Item("简介").Body(func() { ui.TextArea(c, &bio).Rows(3).ShowCount().MaxLength(200) })
		f.Item("等级").Body(func() { ui.Slider(c, &level, 0, 100) })
		f.Item("通知").Body(func() { ui.Switch(c, &notify) })
		f.Item("").NoLabel().Body(func() { ui.Checkbox(c, "我已阅读并同意协议", &agree) })
		f.Item("").NoLabel().Body(func() {
			ui.Row(c, func() {
				if ui.Button(c, "提交").Primary().Disabled(!agree).Clicked() {
					clicks++
					if f.Validate() {
						ui.Message(c, "success", "提交成功："+name)
					}
				}
				if ui.Button(c, "重置").Clicked() {
					clicks++
					name, email, city, bio = "", "", "", ""
					f.ResetValidation()
				}
			}, layout.Gap(8))
		})
	})
}

func tableTab(c *ui.Context) {
	ui.Table(c, []ui.TableColumn{
		{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return people[i].name }},
		{Key: "city", Title: "城市", Text: func(i int) string { return people[i].city },
			Filters: ui.Opts("北京", "上海", "杭州", "深圳", "成都", "南京"), OnFilter: func(i int, v string) bool { return people[i].city == v }},
		{Key: "age", Title: "年龄", Width: 100, Align: ui.AlignRight, Sortable: true,
			Compare: func(a, b int) int { return people[a].age - people[b].age }, Text: func(i int) string { return strconv.Itoa(people[i].age) }},
		{Key: "score", Title: "分数", Width: 220, Sortable: true,
			Compare: func(a, b int) int { return int(people[a].score - people[b].score) },
			Render:  func(i int) { ui.Progress(c, people[i].score) }},
	}, len(people)).Selection(&selected).Show()
}

func miscTab(c *ui.Context) {
	ui.Column(c, func() {
		ui.Alert(c, "info", "这一页的所有内容都是 Go 画出来的，没有用 WebView。")
		ui.Row(c, func() {
			ui.Text(c, "评分：")
			ui.Rate(c, &stars)
		}, layout.Middle(), layout.Gap(8))
		ui.Row(c, func() {
			for _, t := range []string{"success", "processing", "warning", "error"} {
				ui.Tag(c, t).Color(t)
			}
		}, layout.Gap(8))
		ui.Row(c, func() {
			if ui.Button(c, "打开对话框").Clicked() {
				dialogOpen = true
			}
			if ui.Button(c, "通知").Clicked() {
				ui.Notification(c, "info", "你好").Description(fmt.Sprintf("已经点击了 %d 次按钮", clicks))
			}
		}, layout.Gap(8))
	}, layout.Gap(16))

	m := ui.Modal(c, &dialogOpen, "对话框").Body(func() {
		ui.Text(c, "这是一个自绘的模态对话框，按 Esc 或点遮罩可以关闭。")
	})
	if m.Ok() {
		ui.Message(c, "success", "点了确定")
	}
}

func main() {
	renderer := flag.String("renderer", "", `force the renderer: "cpu" or "gpu" (sets TANGO_RENDERER); prints the one in use`)
	flag.Parse()
	if *renderer != "" {
		os.Setenv("TANGO_RENDERER", *renderer)
	}
	quit := true // a single-window demo: closing the window quits on macOS too
	app = tango.New(tango.Options{Name: "Tango UI", ID: "dev.tango.uiwindow", QuitOnLastClose: &quit})
	app.OnReady(func() {
		r, _, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "Tango 自绘 UI", Width: 1000, Height: 760, MinWidth: 640, MinHeight: 480,
			Resizable: true, Centered: true,
		}}, page, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		runner = r
		if *renderer != "" {
			log.Printf("ui-window: renderer: %s", uihost.Renderer(r))
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
