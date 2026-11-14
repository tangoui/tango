package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// HotkeyInput: records a keyboard shortcut.

var hotkeyDemo struct {
	palette, save, run, empty, taken string
}

func hotkeyReset() {
	hotkeyDemo.palette, hotkeyDemo.save, hotkeyDemo.run = "CmdOrCtrl+Shift+P", "CmdOrCtrl+S", "F5"
	hotkeyDemo.empty, hotkeyDemo.taken = "", "CmdOrCtrl+S"
}

// hotkeyOwners maps the demo's bound shortcuts to their commands.
func hotkeyOwner(accel, self string) string {
	for _, b := range []struct{ accel, name string }{
		{hotkeyDemo.palette, "命令面板"}, {hotkeyDemo.save, "保存"}, {hotkeyDemo.run, "运行"},
	} {
		if b.accel == accel && b.name != self {
			return "已被“" + b.name + "”使用"
		}
	}
	return ""
}

func hotkeyPage(c *ui.Context) {
	gallery.Demo(c, "快捷键录制", "聚焦（点击，或 Tab 后按 Enter）开始录制：按住的修饰键实时显示，按下主键完成；Esc 取消录制并保留原值，再按一次 Esc 才交给外层浮层；Backspace 或清除按钮清空。主键需要 Ctrl / Alt / ⌘ 之一（F1–F12 除外）。", func() {
		f := ui.Form(c).LabelWidth(96)
		f.Body(func() {
			f.Item("命令面板").Body(func() { ui.HotkeyInput(c, &hotkeyDemo.palette) })
			f.Item("保存").Body(func() { ui.HotkeyInput(c, &hotkeyDemo.save) })
			f.Item("运行").Body(func() { ui.HotkeyInput(c, &hotkeyDemo.run) })
			f.Item("未设置").Body(func() { ui.HotkeyInput(c, &hotkeyDemo.empty) })
		})
	})
	gallery.Demo(c, "冲突提示", "Conflict 回调返回提示时显示在输入框下方，输入框变为警告状态；值仍然保留，由应用决定是否保存。", func() {
		ui.Row(c, func() {
			ui.HotkeyInput(c, &hotkeyDemo.taken).Conflict(func(a string) string { return hotkeyOwner(a, "格式化") })
			ui.HotkeyInput(c, &hotkeyDemo.empty).Size(ui.Small).Disabled(true)
		}, layout.Gap(16), layout.Margin(geom.Insets{Bottom: 8}))
	})
}

func init() {
	hotkeyReset()
	gallery.Register(gallery.Page{Name: "HotkeyInput", Title: "HotkeyInput 快捷键录制", Group: gallery.Form, Order: 80, Render: hotkeyPage,
		Setup: func(d *ui.Driver) {
			hotkeyReset()
			d.Frame()
			// Start recording the empty field and hold Ctrl+Shift.
			if r, ok := d.FindText("点击录制快捷键"); ok {
				d.Click(r.Center())
				d.Inject(input.KeyEvent{Key: input.KeyCtrlLeft, Down: true, Mods: input.ModCtrl})
				d.Inject(input.KeyEvent{Key: input.KeyShiftLeft, Down: true, Mods: input.ModCtrl | input.ModShift})
				d.Settle()
			}
		}})
}
