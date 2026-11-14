---
title: Dropdown 下拉菜单
---
# Dropdown 下拉菜单

向下弹出的命令列表：把一组操作收进一个触发元素（链接、按钮、区域），悬停、点击或右键时展开，选择一项后执行对应命令。

## 何时使用

- 页面上的操作太多，把次要操作收进“更多”菜单；或用户头像、设置入口下的一组命令。
- 一个主操作加若干相关的次要操作，用按钮下拉（`Button` 形式，即 antd 的 `Dropdown.Button`、Element Plus 的 split-button）。
- 区域的右键菜单（`TriggerContextMenu`，或任意位置的 `ui.ContextMenuAt`）。
- 从一组选项里**选值**并显示在控件上，用 [Select](/components/select)；导航菜单用 [Menu](/components/menu)；弹出的是说明或表单而不是命令时，用 [Popover 气泡卡片](/components/popover)。

## 在线体验

<GalleryPreview component="Dropdown" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	items = []ui.MenuItem{
		{Key: "edit", Label: "编辑"},
		{Key: "copy", Label: "复制"},
		{Divider: true},
		{Key: "delete", Label: "删除", Danger: true},
	}
	last string
)

func page(c *ui.Context) {
	if k := ui.Dropdown(c, items).Body(func() { ui.Link(c, "悬停展开 ▾") }).Clicked(); k != "" {
		last = k
	}
	ui.Text(c, "最近选择："+last).Secondary()
}
```

`ui.Dropdown(c, items)` 先链式设置选项，再用 `Body(fn)` 构建触发元素；`Clicked()` 返回本帧选中项的 `Key`（没有选择时为 `""`），选中后菜单自动关闭。默认悬停展开，菜单出现在触发元素下方、与起始边对齐。

## 常见用法

### 菜单项：图标、禁用、危险、分隔线、分组与快捷键

菜单内容是 `[]ui.MenuItem`：`Icon` 显示图标，`Disabled` 禁用，`Danger` 标红，`Divider: true` 是分隔线；`Group: true` 把 `Children` 显示为带标题的一组（标题不可点击，键盘导航跳过），`Shortcut` 在右侧显示快捷键文字（只是展示，按键本身由应用处理）。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

var fileMenu = []ui.MenuItem{
	{Key: "g1", Label: "文件", Group: true, Children: []ui.MenuItem{
		{Key: "new", Label: "新建", Icon: icon.Plus, Shortcut: "Ctrl+N"},
		{Key: "download", Label: "下载", Icon: icon.Download},
	}},
	{Divider: true},
	{Key: "settings", Label: "设置", Icon: icon.Setting, Disabled: true},
	{Key: "logout", Label: "退出登录", Icon: icon.Logout, Danger: true},
}

func page(c *ui.Context) {
	ui.Dropdown(c, fileMenu).Trigger(ui.TriggerClick).Body(func() {
		ui.Button(c, "文件").IconRight(icon.Down)
	})
}
```

### 触发方式

`Trigger` 取 `ui.TriggerHover`（默认）、`ui.TriggerClick`（点击切换，推荐用于按钮）或 `ui.TriggerContextMenu`（在触发区域右键或长按 500ms，菜单在指针处打开）。`Arrow()` 画一个指向触发元素的箭头，`Selected(key)` 高亮当前项。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var (
	sortItems = []ui.MenuItem{{Key: "name", Label: "按名称"}, {Key: "time", Label: "按时间"}, {Key: "size", Label: "按大小"}}
	sortBy    = "time"
)

func page(c *ui.Context) {
	if k := ui.Dropdown(c, sortItems).Trigger(ui.TriggerClick).Arrow().Selected(sortBy).
		Body(func() { ui.Button(c, "排序") }).Clicked(); k != "" {
		sortBy = k
	}
	if k := ui.Dropdown(c, sortItems).Trigger(ui.TriggerContextMenu).Body(func() {
		ui.Column(c, func() { ui.Text(c, "在此区域右键").Tertiary() }, layout.Size(220, 80), layout.Center())
	}).Clicked(); k != "" {
		sortBy = k
	}
}
```

### 多级子菜单

菜单项带 `Children` 时成为子菜单，悬停或按 → 向行内末端（LTR 为右侧）展开，可以多层嵌套；`Clicked()` 只返回叶子项的 key。需要知道完整路径时用 `ui.MenuPath(items, key)`。

```go
import (
	"github.com/tangoui/tango/ui"
)

var shareMenu = []ui.MenuItem{
	{Key: "mail", Label: "邮件"},
	{Key: "social", Label: "社交平台", Children: []ui.MenuItem{
		{Key: "wx", Label: "微信"},
		{Key: "wb", Label: "微博"},
	}},
}

var shared string

func page(c *ui.Context) {
	if k := ui.Dropdown(c, shareMenu).Body(func() { ui.Button(c, "分享到") }).Clicked(); k != "" {
		for _, it := range ui.MenuPath(shareMenu, k) {
			shared += it.Label + " / "
		}
	}
}
```

### 按钮下拉

`Button(label)` 构建左右两段的按钮：左侧是主操作（`ButtonClicked()`），右侧展开菜单（`Clicked()`）。`Primary()`、`Danger()`、`Size(...)`、`Disabled(...)` 设置按钮样式，`MenuIcon` 替换右侧图标。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	saveMenu = []ui.MenuItem{{Key: "draft", Label: "存为草稿"}, {Key: "template", Label: "存为模板"}}
	saved    string
)

func page(c *ui.Context) {
	dd := ui.Dropdown(c, saveMenu).Primary().Button("保存")
	if dd.ButtonClicked() {
		saved = "已保存"
	}
	if k := dd.Clicked(); k != "" {
		saved = "已" + k
	}
}
```

### 受控打开与长菜单

`Open(&v)` 绑定打开状态，`OpenChanged()` 报告本帧打开或关闭，`IsOpen()` 返回当前状态；`MinWidth` 设置菜单最小宽度，`MaxHeight` 限制高度（超出滚动，键盘移动时自动滚到当前项）；`Placement` 取 `ui.BottomStart`（默认）、`ui.Bottom`、`ui.BottomEnd`、`ui.TopStart`、`ui.Top`、`ui.TopEnd`。

```go
import (
	"fmt"

	"github.com/tangoui/tango/ui"
)

var (
	menuOpen bool
	cities   []ui.MenuItem
)

func init() {
	for i := 1; i <= 40; i++ {
		cities = append(cities, ui.MenuItem{Key: fmt.Sprint(i), Label: fmt.Sprintf("城市 %d", i)})
	}
}

func page(c *ui.Context) {
	ui.Dropdown(c, cities).Trigger(ui.TriggerClick).Open(&menuOpen).
		Placement(ui.BottomEnd).MinWidth(160).MaxHeight(240).
		Body(func() { ui.Button(c, "选择城市") })
	if menuOpen {
		ui.Text(c, "菜单已打开").Secondary()
	}
}
```

### 任意位置的右键菜单

右键位置不在固定触发元素上时（例如画布、表格单元格），用 `c.ContextMenuFor(id)` 检测右键，再调用 `ui.ContextMenuAt` 在指针处打开菜单；选中的 key 通过回调返回。只在检测到右键的那一帧调用一次。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var (
	ctxItems = []ui.MenuItem{{Key: "copy", Label: "复制", Shortcut: "Ctrl+C"}, {Key: "paste", Label: "粘贴", Shortcut: "Ctrl+V"}}
	action   string
)

func page(c *ui.Context) {
	area := ui.Column(c, func() { ui.Text(c, "右键这里").Tertiary() }, layout.Size(300, 120), layout.Center())
	if p, ok := c.ContextMenuFor(area.ID()); ok {
		ui.ContextMenuAt(c, area.ID(), p, ctxItems, func(key string) { action = key })
	}
	ui.Text(c, "操作："+action).Secondary()
}
```

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 面板 | 无边框、大圆角、阴影 | 细边框、普通圆角 | 细边框、普通圆角 |
| 菜单项 | 高 32，悬停浅灰底，选中项主色浅底 | 左右 16 内边距，悬停主色浅底 + 主色文字，无选中底色 | 左右 8 内边距，16px 弱化色图标 |
| 危险项 | 悬停红底白字 | 只有红字 | 悬停浅红底 |
| 分组标题 | 弱化色 | 12px 弱化色 | 中等字重正文色 |

组件令牌在 `theme.Components.Dropdown`（`theme.DropdownToken`，与 Menu 的弹出子菜单共用）：面板 `PopupBg`、`PopupBorderColor`、`PopupRadius`、`PopupShadow`、`PaddingBlock`、`PaddingInline`；菜单项 `ItemHeight`、`ItemPaddingInline`、`ItemBorderRadius`、`ItemColor`、`ItemHoverBg`、`ItemHoverColor`、`ItemSelectedBg`、`ItemSelectedColor`、`ItemIconColor`、`IconSize`；危险项 `DangerColor`、`DangerHoverBg`、`DangerHoverColor`；以及 `SubmenuArrowColor`、`GroupTitle*`、`DividerColor`、`Shortcut*`。

`ConfigProvider.Defaults("Dropdown", ui.Props{...})` 支持：`trigger`（`ui.Trigger` 或字符串 `"hover"` / `"click"` / `"contextMenu"`）、`placement`、`arrow`、`disabled`。

## 注意事项

- **状态**：菜单的打开状态、展开的子菜单由组件按触发区域保存；`items` 可以是包级变量（推荐，避免每帧重新分配）。循环里生成多个下拉菜单时用 `ui.Key(c, key, fn)` 包住每一项。
- **读取结果**：`Clicked()` 只在选中的那一帧返回 key，用 `if k := ...; k != ""` 的写法处理，不要把它当作“当前值”。
- **键盘**：触发器聚焦后 Enter / 空格 / ↓ 打开菜单；↑ / ↓ / Home / End 移动，→ 进入子菜单，← 返回上级，Enter 选择，Esc 关闭最深一层；输入字母或数字跳到以它开头的项。菜单关闭后焦点回到触发器。
- **右键与长按**：`TriggerContextMenu` 在触摸设备上用长按 500ms 打开；键盘用户在触发区域聚焦时按菜单键或 Shift+F10 打开。
- **RTL**：起始 / 末端对齐和子菜单展开方向随方向镜像。

## 示例代码

<!--@include: ./_gen/dropdown-examples.md-->

## API

<!--@include: ./_gen/dropdown-api.md-->

## 相关链接

- [源码 ui/nav_dropdown.go](https://github.com/tangoui/tango/blob/main/ui/nav_dropdown.go)
- [Go API 参考](/reference/api/ui)
- [Menu](/components/menu)、[Select](/components/select)、[Popover 气泡卡片](/components/popover)
- [组件总览](/components/)
