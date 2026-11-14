---
title: Popover 气泡卡片
---
# Popover 气泡卡片

附着在触发元素旁边的浮层卡片，可以有标题和任意内容（文字、按钮、表单控件），由悬停、点击或聚焦打开。

## 何时使用

- 在不离开页面的情况下展示一块补充信息或少量操作，例如用户名片、字段解释、快捷设置、筛选条件。
- 与 [Tooltip](/components/tooltip) 相比，Popover 可以有标题、多行内容和可交互的控件；只是一句说明时用 Tooltip。
- 需要用户确认一个操作时用 [Popconfirm](/components/catalog/popconfirm)；选择一组菜单命令时用 [Dropdown 下拉菜单](/components/dropdown)；内容多到需要滚动或分步时用 [Modal](/components/modal) 或 [Drawer](/components/drawer)。

## 在线体验

<GalleryPreview component="Popover" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.Popover(c).Title("标题").Content(func() {
		ui.Text(c, "气泡卡片内容")
		ui.Text(c, "更多信息")
	}).Body(func() {
		ui.Button(c, "悬停打开")
	})
}
```

`ui.Popover(c)` 先链式设置标题、内容和选项，最后调用 `Body(fn)`：`fn` 构建触发元素，卡片打开时同时构建浮层。默认悬停打开、出现在上方。只有一段文字时用 `Text(s)` 代替 `Content`。

## 常见用法

### 触发方式

`Trigger` 取 `"hover"`（默认；键盘焦点进入触发元素时也会打开，鼠标可以移进卡片里操作）、`"click"`（点击切换，点外部或按 Esc 关闭）、`"focus"`（聚焦时打开）或 `"manual"`（只由 `Open` 绑定的变量控制）。卡片里有按钮、输入框等可交互内容时，推荐 `"click"`。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Row(c, func() {
		ui.Popover(c).Title("悬停").Text("鼠标移入打开").Body(func() { ui.Button(c, "Hover") })
		ui.Popover(c).Title("点击").Text("点外部或按 Esc 关闭").Trigger("click").Body(func() { ui.Button(c, "Click") })
		ui.Popover(c).Title("聚焦").Text("Tab 聚焦打开").Trigger("focus").Body(func() { ui.Button(c, "Focus") })
	}, layout.Gap(16))
}
```

### 位置

`Placement` 支持 12 个方向：`top`、`topLeft`、`topRight`、`bottom`、`bottomLeft`、`bottomRight`、`left`、`leftTop`、`leftBottom`、`right`、`rightTop`、`rightBottom`，空间不够时翻转到对侧。`left` / `right` 是逻辑方向，RTL 下镜像。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.Popover(c).Title("下方左对齐").Text("从触发元素左下方弹出").Placement("bottomLeft").Body(func() {
		ui.Button(c, "bottomLeft")
	})
}
```

### 受控打开：在卡片里关闭

`Open(&v)` 把打开状态绑定到你的变量上，可以在代码里打开或关闭，例如卡片里的“确定”按钮操作完成后关闭。`OpenChanged()` 报告本帧用户触发的状态变化，`IsOpen()` 返回当前是否打开；这两个方法在 `PopoverW` 上，`Body` 返回的是触发区域的 `*ui.Box`，所以先把 `PopoverW` 存在变量里。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

var (
	renameOpen bool
	name       = "未命名项目"
	draft      string
)

func page(c *ui.Context) {
	p := ui.Popover(c).Title("重命名").Trigger("click").Placement("bottomLeft").Open(&renameOpen).Width(260).
		Content(func() {
			ui.Input(c, &draft).Placeholder("新名称")
			ui.Row(c, func() {
				if ui.Button(c, "取消").Small().Clicked() {
					renameOpen = false
				}
				if ui.Button(c, "确定").Small().Primary().Clicked() && draft != "" {
					name, renameOpen = draft, false
				}
			}, layout.Gap(8), layout.End(), layout.Margin(geom.Insets{Top: 8}))
		})
	p.Body(func() { ui.Button(c, name) })
	if p.OpenChanged() && renameOpen {
		draft = name // 每次打开时从当前名称开始编辑
	}
}
```

### 自定义卡片：宽度、内边距与箭头

`Width(px)` 固定卡片宽度；`NoPadding()` 去掉内边距，适合放列表、菜单或自己画的卡片；`NoArrow()` 去掉箭头。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Popover(c).NoPadding().NoArrow().Width(240).Content(func() {
		ui.Column(c, func() {
			ui.Text(c, "张三").Strong()
			ui.Text(c, "前端工程师 · 杭州").Secondary()
		}, layout.Pad(16))
	}).Body(func() {
		ui.Link(c, "@张三")
	})
}
```

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 箭头 | 有 | 有 | 默认无（shadcn/ui 的 Popover 没有箭头） |
| 标题 | 14px 加粗 | 16px 标题色、常规字重 | 中等字重（500） |
| 内边距 / 圆角 | 12 / 大圆角 | 12 / 普通圆角 | 16 / 普通圆角 |

组件令牌在 `theme.Components.Popover`（`theme.PopoverToken`，同时作用于 Popconfirm）：`Bg`、`TitleColor`、`Color`、`TitleFontSize`、`TitleFontWeight`、`TitleMarginBottom`、`TitleMinWidth`、`InnerPadding`、`Radius`、`BorderColor`、`Shadow`。

`ConfigProvider.Defaults("Popover", ui.Props{...})` 支持：`placement`、`trigger`、`arrow`（布尔）。

## 注意事项

- **状态放在哪**：不绑定 `Open` 时，打开状态由组件按触发区域的 ID 保存；在循环里生成多个 Popover 时用 `ui.Key(c, key, fn)` 包住每一项。卡片里输入框绑定的变量要放在包级或结构体里。
- **触发元素**：触发区域里第一个可聚焦的控件（例如按钮）就是触发器，读屏软件会知道它带有弹出层及其展开状态；触发区域里没有可聚焦控件时，组件让整个区域可聚焦，键盘用户也能打开。
- **键盘**：click 触发的卡片可以用 Enter / 空格打开，Esc 或点击外部关闭；hover 触发的卡片在键盘焦点进入时打开，Esc 关闭。
- **不要嵌太多**：Popover 里再放 Popover 或复杂表单会让关闭逻辑难以预期，内容较多时改用 Modal 或 Drawer。

## 示例代码

<!--@include: ./_gen/popover-examples.md-->

## API

<!--@include: ./_gen/popover-api.md-->

## 相关链接

- [源码 ui/data_popover.go](https://github.com/tangoui/tango/blob/main/ui/data_popover.go)
- [Go API 参考](/reference/api/ui)
- [Tooltip 文字提示](/components/tooltip)、[Popconfirm](/components/catalog/popconfirm)、[Dropdown 下拉菜单](/components/dropdown)
- [组件总览](/components/)
