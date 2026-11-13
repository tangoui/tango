// mobile-ui shows Tango's self-drawn UI (packages ui and ui/mobile) as a
// full-screen page through uihost.Open: no WebView. It runs unchanged on
// desktop (go run ./examples/mobile-ui, a phone-sized window) and on iOS
// (tan build -platform ios-simulator in this directory, or
// bash mobile/ios/build.sh examples/mobile-ui). See
// docs/specs/mobile-canvas.md §6.3.
//
// Three tabs: a list page (rows, switches, a tap counter), a form page
// (text fields of every keyboard kind stacked down to the bottom of the
// screen, to exercise the soft keyboard, input methods and keyboard
// avoidance) and a settings page that shows how the window is drawn: the
// text engine (text.Engine()), the renderer (uihost.Renderer), the window
// size, scale, safe area and keyboard inset, and the followed system
// settings, plus a stepper and a slider for a screen reader's adjust
// gestures. The app logs the same facts at start, every tap it sees and
// every value committed in the form, so a device run can be checked from
// the console.
package main

import (
	"fmt"
	"log"
	"runtime"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/mobile"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/uihost"
)

var (
	app    *tango.App
	runner *ui.Runner

	tab            = "list"
	wifi, bt, dark = true, false, false
	taps           int
	logged         bool
	qty            = 1.0  // settings page stepper
	volume         = 50.0 // settings page slider
)

func main() {
	app = tango.New(tango.Options{Name: "Tango Mobile UI", ID: "dev.tango.mobileui"})
	app.OnReady(func() {
		r, s, err := uihost.Open(app, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "Mobile UI", Width: 390, Height: 844},
		}, page, ui.RunnerOptions{})
		if err != nil {
			log.Fatalf("mobile-ui: uihost.Open: %v", err)
		}
		runner = r
		log.Printf("mobile-ui: surface %d open on %s/%s, text engine %q, renderer %q",
			s.ID(), runtime.GOOS, runtime.GOARCH, text.Engine(), uihost.Renderer(r))
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func renderer() string {
	if runner == nil {
		return "?"
	}
	return uihost.Renderer(runner)
}

func page(c *ui.Context) {
	if !logged && c.Size().W > 0 {
		logged = true
		in := c.SafeArea()
		log.Printf("mobile-ui: first frame %.0fx%.0f, safe area top %.0f bottom %.0f left %.0f right %.0f, dark %v",
			c.Size().W, c.Size().H, in.Top, in.Bottom, in.Left, in.Right, c.Theme.Dark)
	}
	mobile.Provider(c, mobile.Options{}, func() {
		ui.Column(c, func() {
			mobile.SafeArea(c, mobile.Top)
			// Landscape: keep content clear of the cutout and the side
			// navigation bar.
			ui.Row(c, func() {
				mobile.SafeArea(c, mobile.Left)
				ui.Column(c, func() {
					// One identity scope per tab, so each page keeps its own
					// scroll position instead of sharing the other's.
					ui.Key(c, tab, func() {
						switch tab {
						case "settings":
							settingsPage(c)
						case "form":
							formPage(c)
						default:
							listPage(c)
						}
					})
				}, layout.Grow(1))
				mobile.SafeArea(c, mobile.Right)
			}, layout.Grow(1))
			tb := mobile.TabBar(c, []mobile.TabItem{
				{Key: "list", Title: "列表", Icon: icon.Home},
				{Key: "form", Title: "表单", Icon: icon.Edit},
				{Key: "settings", Title: "设置", Icon: icon.Setting},
			}, &tab)
			if tb.Changed() {
				log.Printf("mobile-ui: tab %q", tab)
			}
		}, layout.Grow(1))
	})
}

func listPage(c *ui.Context) {
	mobile.NavBar(c, "Tango 自绘界面")
	mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
		mobile.List(c, "网络").Body(func() {
			if mobile.Cell(c, "无线局域网").Icon(icon.Global).Switch(&wifi).Changed() {
				log.Printf("mobile-ui: wifi %v", wifi)
			}
			if mobile.Cell(c, "蓝牙").Icon(icon.Link).Switch(&bt).Changed() {
				log.Printf("mobile-ui: bluetooth %v", bt)
			}
			mobile.Cell(c, "蜂窝网络").Icon(icon.Bell).Extra("已开启").Arrow()
		})
		mobile.List(c, "触摸").Body(func() {
			if mobile.Cell(c, "点我计数").Desc("每次点按加一").Extra(fmt.Sprint(taps)).Clickable().Clicked() {
				taps++
				log.Printf("mobile-ui: tap #%d", taps)
			}
			for i := 1; i <= 12; i++ {
				mobile.Cell(c, fmt.Sprintf("第 %d 行 · Row %d", i, i)).Desc("向上滑动以滚动列表").Arrow()
			}
		})
		ui.Column(c, func() {
			if mobile.Button(c, "按钮 Button").Primary().Block().Clicked() {
				taps++
				log.Printf("mobile-ui: button tap #%d", taps)
			}
		}, layout.PadVH(16, 16))
	})
}

func settingsPage(c *ui.Context) {
	mobile.NavBar(c, "设置")
	sz, in := c.Size(), c.SafeArea()
	mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
		mobile.List(c, "绘制").Body(func() {
			mobile.Cell(c, "文字引擎 text.Engine()").Extra(text.Engine())
			mobile.Cell(c, "渲染器 uihost.Renderer").Extra(renderer())
			mobile.Cell(c, "平台").Extra(runtime.GOOS + "/" + runtime.GOARCH)
			mobile.Cell(c, "窗口尺寸").Extra(fmt.Sprintf("%.0f × %.0f", sz.W, sz.H))
			mobile.Cell(c, "安全区 上/下").Extra(fmt.Sprintf("%.0f / %.0f", in.Top, in.Bottom))
			mobile.Cell(c, "安全区 左/右").Extra(fmt.Sprintf("%.0f / %.0f", in.Left, in.Right))
			mobile.Cell(c, "键盘 inset").Extra(fmt.Sprintf("%.0f", c.KeyboardInset().Bottom))
		})
		mobile.List(c, "跟随系统").Body(func() {
			dark = c.Theme.Dark
			mobile.Cell(c, "深色模式").Extra(onOff(dark))
			mobile.Cell(c, "减少动态效果").Extra(onOff(c.ReduceMotion()))
			mobile.Cell(c, "语言").Extra(i18n.Locale())
		})
		mobile.List(c, "文字").Body(func() {
			mobile.Cell(c, "中文排版：汉字、标点。").Desc("English · Ελληνικά · العربية · 😀")
		})
		// Adjustable controls: a screen reader's increment / decrement
		// (VoiceOver swipe up / down, TalkBack volume keys or swipes)
		// step them.
		mobile.Form(c).Header("调节 Adjust").Body(func() {
			mobile.FormItem(c, "数量").Body(func() {
				if mobile.Stepper(c, &qty).Min(0).Max(10).Changed() {
					log.Printf("mobile-ui: stepper %g", qty)
				}
			})
			mobile.FormItem(c, "音量").Body(func() {
				if ui.Slider(c, &volume, 0, 100).A11yLabel("音量").Changed() {
					log.Printf("mobile-ui: slider %g", volume)
				}
			})
		})
	})
}

// The form tab: one field per soft-keyboard kind, the last ones near the
// bottom of a phone screen, so focusing them must lift them above the
// keyboard.
var (
	form = struct {
		name, email, phone, age, amount, site, search, password, note string
	}{}
	editing  = map[ui.ID]string{} // focused field -> its value on focus
	kbLogged float32
)

func formPage(c *ui.Context) {
	mobile.NavBar(c, "表单 Form")
	if kb := c.KeyboardInset().Bottom; kb != kbLogged {
		kbLogged = kb
		log.Printf("mobile-ui: keyboard inset %.0f", kb)
	}
	mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
		mobile.Form(c).Header("账号 Account").Body(func() {
			field(c, "用户名", "name", &form.name, input.TextPlain, input.EnterNext, "请输入用户名")
			field(c, "邮箱", "email", &form.email, input.TextEmail, input.EnterNext, "name@example.com")
			field(c, "手机号", "phone", &form.phone, input.TextPhone, input.EnterNext, "138 0000 0000")
			field(c, "密码", "password", &form.password, input.TextPassword, input.EnterDone, "至少 8 位")
		})
		mobile.Form(c).Header("资料 Profile").Body(func() {
			field(c, "年龄", "age", &form.age, input.TextNumber, input.EnterNext, "数字")
			field(c, "金额", "amount", &form.amount, input.TextDecimal, input.EnterNext, "0.00")
			field(c, "网址", "url", &form.site, input.TextURL, input.EnterGo, "https://")
			field(c, "搜索", "search", &form.search, input.TextSearch, input.EnterSearch, "关键词")
		})
		mobile.Form(c).Header("备注 Note（多行）").Vertical().Body(func() {
			mobile.FormItem(c, "留言").Body(func() {
				ta := ui.TextArea(c, &form.note).Rows(3).Placeholder("多行文本，回车换行")
				ta.A11yLabel("留言")
				watch(c, "note", ta.ID(), &form.note, false)
			})
		})
		ui.Column(c, func() {
			if mobile.Button(c, "提交 Submit").Primary().Block().Clicked() {
				log.Printf("mobile-ui: form submit name=%q email=%q phone=%q age=%q amount=%q url=%q search=%q password=%d runes note=%q",
					form.name, form.email, form.phone, form.age, form.amount, form.site, form.search, len([]rune(form.password)), form.note)
			}
		}, layout.PadVH(16, 16))
	})
}

// field adds one labelled text field of kind k and logs its value when it
// is committed (Enter, or leaving the field after an edit).
func field(c *ui.Context, label, name string, val *string, k input.TextKind, enter input.EnterKey, ph string) {
	mobile.FormItem(c, label).Body(func() {
		f := mobile.Field(c, val).Type(k).EnterKey(enter).Placeholder(ph)
		if k != input.TextPassword {
			f.Input().AllowClear()
		}
		if f.Submitted() {
			log.Printf("mobile-ui: form %s submitted %s", name, shown(*val, k == input.TextPassword))
		}
		watch(c, name, f.Input().ID(), val, k == input.TextPassword)
	})
}

// watch logs the value of field id when focus leaves it after a change.
func watch(c *ui.Context, name string, id ui.ID, val *string, secret bool) {
	before, was := editing[id]
	switch f := c.Focused(); {
	case f == id && !was:
		editing[id] = *val
	case f != id && was:
		delete(editing, id)
		if *val != before {
			log.Printf("mobile-ui: form %s = %s", name, shown(*val, secret))
		}
	}
}

func shown(v string, secret bool) string {
	if secret {
		return fmt.Sprintf("(%d runes hidden)", len([]rune(v)))
	}
	return fmt.Sprintf("%q", v)
}

func onOff(b bool) string {
	if b {
		return "开"
	}
	return "关"
}
