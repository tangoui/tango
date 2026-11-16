---
title: HotkeyInput 快捷键录制
---
# HotkeyInput 快捷键录制

录制键盘快捷键的输入框：聚焦后按下组合键，就把它记成一个加速键字符串（如 `CmdOrCtrl+Shift+P`），并按当前平台显示成键帽。配套的 `ShortcutKeys`、`ShortcutLabel` 把同样的字符串按平台格式化，用来在菜单、提示里展示。

## 何时使用

- 设置页里让用户自定义快捷键：命令面板、保存、运行……值是一个 `string`，格式与 `uihost.Shortcut`、`sys/hotkey`、菜单加速键相同，可以直接存进配置再注册回去。
- 需要在界面里把加速键显示成平台习惯的样子（macOS 的 `⌘⇧K`，Windows / Linux 的 `Ctrl+Shift+K`）：用 `ShortcutKeys` 取键帽列表，或用 `ShortcutLabel` 取整串文本。
- 只是展示一个固定的快捷键，不需要录制，用 [Kbd 键帽](/components/kbd)；要输入普通文本，用 [Input 输入框](/components/input)。

## 在线体验

<GalleryPreview component="HotkeyInput" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var palette = "CmdOrCtrl+Shift+P"

func page(c *ui.Context) {
	ui.HotkeyInput(c, &palette)
}
```

`HotkeyInput` 绑定一个 `*string`。点击输入框，或用 Tab 聚焦后按 Enter / 空格，开始录制：按住的修饰键会实时显示成键帽，再按下主键就完成录制，结果写回你的变量。平台的“快捷键修饰键”（macOS 的 Cmd、其他平台的 Ctrl）记作 `CmdOrCtrl`，所以同一个值可以跨平台使用；`Literal()` 则按实际按下的键记为 `Cmd` / `Ctrl`。输入框右侧有清除按钮，聚焦时单独按 Backspace 或 Delete 也能清空。

主键必须搭配 Ctrl、Alt 或 Cmd 之一（只加 Shift 不算），F1–F12 例外；不满足时按键被忽略，继续等待。Esc 取消录制并保留原值；Tab 不会被录制，仍然用来移动焦点。

## 常见用法

以下片段沿用基本用法里的 `palette` 变量，省略了 `ui`、`ui/layout` 等常用 `import`；用到额外的包时会写出。

### 放进表单

在 `Form` 里给输入框配标签，标签同时是它的无障碍名称。

```go
var (
	saveKey = "CmdOrCtrl+S"
	runKey  = "F5"
	empty   string
)

func settings(c *ui.Context) {
	f := ui.Form(c).LabelWidth(96)
	f.Body(func() {
		f.Item("命令面板").Body(func() { ui.HotkeyInput(c, &palette) })
		f.Item("保存").Body(func() { ui.HotkeyInput(c, &saveKey) })
		f.Item("运行").Body(func() { ui.HotkeyInput(c, &runKey) })
		f.Item("未设置").Body(func() { ui.HotkeyInput(c, &empty) })
	})
}
```

### 冲突提示

`Conflict` 的回调拿到刚录制（或已有）的加速键，返回非空文字表示“已被占用或保留”。文字显示在输入框下方，输入框变成警告状态；**值仍然保留**，是否保存由应用决定。

```go
func owner(accel, self string) string {
	for _, b := range []struct{ accel, name string }{
		{palette, "命令面板"}, {saveKey, "保存"}, {runKey, "运行"},
	} {
		if b.accel == accel && b.name != self {
			return "已被“" + b.name + "”使用"
		}
	}
	return ""
}

func withConflict(c *ui.Context) {
	ui.HotkeyInput(c, &runKey).Conflict(func(a string) string { return owner(a, "运行") })
}
```

### 状态、尺寸与占位文字

`Status("error")` / `Status("warning")` 设置校验状态，`Size` 选择控件尺寸，`Width` 设置宽度，`Disabled(true)` 禁止录制和清除，`Placeholder` 替换空值时的提示文字（默认取本地化的“点击录制快捷键”）。

```go
func variants(c *ui.Context) {
	ui.Row(c, func() {
		ui.HotkeyInput(c, &empty).Placeholder("未绑定").Width(220)
		ui.HotkeyInput(c, &saveKey).Size(ui.Small)
		ui.HotkeyInput(c, &runKey).Disabled(true)
	}, layout.Gap(16))
}
```

### 允许单键，或保留左右修饰键

`AllowBare(true)` 允许不带修饰键的主键（字母、数字、Enter），适合游戏键位、终端内的单键命令。`Literal()` 把平台修饰键按实际键名记成 `Cmd` / `Ctrl`，而不是 `CmdOrCtrl`。

```go
var fireKey = "Space"

func bare(c *ui.Context) {
	ui.HotkeyInput(c, &fireKey).AllowBare(true)
}
```

### 响应变化

`Changed()` 在本帧录制或清除了值时返回 `true`；`Recording()` 报告是否正在等待按键。

```go
func react(c *ui.Context, save func(string)) {
	hk := ui.HotkeyInput(c, &saveKey)
	if hk.Changed() {
		save(saveKey)
	}
	if hk.Recording() {
		ui.Text(c, "请按下新的组合键").Tertiary()
	}
}
```

### 把结果注册成窗口快捷键

录制的值就是 `uihost.Shortcut` 的参数。每次变化时先移除旧的绑定，再注册新的；`Shortcut` 对无法识别的加速键返回错误，需要处理。

```go
import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/uihost"
)

type binding struct {
	accel  string
	fn     func()
	remove func()
}

// unbind 移除已注册的窗口快捷键。
func (b *binding) unbind() {
	if b.remove != nil {
		b.remove()
		b.remove = nil
	}
}

// rebind 按当前的 accel 重新注册窗口快捷键。
func (b *binding) rebind(s *tango.Surface) error {
	b.unbind()
	if b.accel == "" {
		return nil
	}
	remove, err := uihost.Shortcut(s, b.accel, b.fn)
	if err != nil {
		return err
	}
	b.remove = remove
	return nil
}
```

录制期间要暂停这些绑定，见下文“注意事项”里的 `syncShortcuts`。

### 按平台显示加速键

`ShortcutKeys` 返回键帽列表，交给 `Kbd` 逐个绘制；`ShortcutLabel` 返回一整串文本，适合 `MenuItem.Shortcut` 这类纯文本位置。两者对无法解析的加速键分别返回 `nil` 和空串。

```go
func hints(c *ui.Context) {
	ui.Row(c, func() {
		ui.Text(c, "命令面板")
		for _, k := range ui.ShortcutKeys(palette) {
			ui.Kbd(c, k)
		}
	}, layout.Gap(4), layout.Middle())

	items := []ui.MenuItem{
		{Key: "save", Label: "保存", Shortcut: ui.ShortcutLabel(saveKey)},
		{Key: "run", Label: "运行", Shortcut: ui.ShortcutLabel(runKey)},
	}
	_ = items
}
```

`CmdOrCtrl+Shift+K` 在 macOS 上显示为 `⇧⌘K`（修饰键按 Apple 的 `⌃⌥⇧⌘` 顺序），在 Windows 上是 `Ctrl+Shift+K`；Windows 的 Meta 键叫 `Win`，Linux 叫 `Super`。方向键显示为箭头，macOS 上 Enter、Backspace、Delete、Esc、Tab 等也使用 Apple 的符号。

## 主题与样式

组件令牌是 `theme.Components.HotkeyInput`（`HotkeyInputToken`）：`MinWidth`（默认 180）、`PaddingInline`、`KeyGap`、`PlaceholderColor`、`RecordingColor`（录制中的提示色，Ant Design / Element 为主色，shadcn 为正文色）、`ClearIconSize`、`ClearColor`、`MessageFontSize`、`MessageColor`，可通过 `ui.ConfigProvider(c).Components(...)` 局部覆盖。输入框的边框、悬停、聚焦与状态色沿用各主题的输入框样式；键帽就是 `Kbd`。

## 注意事项

### 录制与应用快捷键

录制读取的是按键事件，不是文本输入。需要清楚下面几条：

- **窗口快捷键先于输入框**：用 `uihost.Shortcut` 注册的窗口快捷键，在按键到达界面之前就按加速键匹配并**吞掉**按键。`HotkeyInput` 不是文本输入框（不会向宿主申请文本输入），所以 `uihost` 对文本框的豁免（只放行 F1–F12 和部分 Ctrl 组合）不适用：录制时如果按下的组合已被某个窗口快捷键占用，触发的是那个快捷键，输入框收不到按键。
- **菜单加速键更靠前**：菜单项的加速键由平台先处理（macOS 的 key equivalent、Windows 的 TranslateAccelerator、GTK 的 accel group），同样到不了输入框，也无法被录制。用 `Conflict` 把这些保留组合报给用户，不要指望录制出来。
- **录制期间暂停窗口快捷键**：要让用户能重新录制已绑定的组合，在 `Recording()` 为 `true` 时先移除相关窗口快捷键，录制结束（`Recording()` 回到 `false`）后再注册回去。下面沿用上文的 `binding`：
- **系统与输入法保留的组合**：切换输入法、Cmd+Space、Alt+F4、Cmd+Q 这类由系统或输入法在应用之前处理的组合键，应用收不到，也就录不到。
- **输入法**：输入框不会开启输入法组合；`uihost` 在输入法正在组合时不触发快捷键，这与录制无关。录制时只看按下的键和修饰键，不看输入法产生的文字。
- **Esc 的归属**：录制中按 Esc 只取消录制并保留原值，这次 Esc 不会再传给外层，所以放在 Modal、Drawer 里时它们不会被顺带关闭；不在录制时的 Esc 照常交给外层。Popover 和下拉面板在构建内容前就检查 Esc，因此放在它们里面时 Esc 会直接关闭面板。

```go
var (
	bindings        []*binding
	shortcutsPaused bool
)

// syncShortcuts 在录制开始时移除所有窗口快捷键，录制结束后恢复。
func syncShortcuts(s *tango.Surface, recording bool) {
	if recording == shortcutsPaused {
		return
	}
	shortcutsPaused = recording
	for _, b := range bindings {
		if recording {
			b.unbind()
		} else {
			_ = b.rebind(s)
		}
	}
}
```

页面里在建好所有 `HotkeyInput` 之后，用其中任一个的 `Recording()` 调用 `syncShortcuts(surface, hk.Recording())`（`surface` 是 `uihost.Open` 返回的窗口）。

### 状态与无障碍

- 值保存在你传入的变量里；录制状态（是否在录制、已按住的修饰键）由控件按焦点控件的 ID 自动保存，所以在循环里创建多个 `HotkeyInput` 时请用 `ui.Key` 区分。
- 无障碍角色是只读文本框，值是 `ShortcutLabel` 的结果；名称来自 `A11yLabel` 或所在的 `FormItem` 标签，两者都没有时用占位文字。冲突文字会作为描述朗读。
- 键帽始终从左到右排列，即使在 RTL 下也是如此。

## 示例代码

<!--@include: ./_gen/hotkeyinput-examples.md-->

## API

<!--@include: ./_gen/hotkeyinput-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/hotkey_input.go)
- [Go API 参考](/reference/api/ui)
- [Kbd 键帽](/components/kbd)、[Form 表单](/components/form)、[Menu 导航菜单](/components/menu)
