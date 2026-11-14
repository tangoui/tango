---
title: Drawer 抽屉
---
# Drawer 抽屉

从窗口边缘滑出的浮层面板，用于在不离开当前页面的情况下查看详情、填写较长的表单或做一组设置。

## 何时使用

- 需要一个比对话框更大的区域，并且希望用户还能看到页面的上下文，例如记录详情、编辑表单、筛选面板、设置面板。
- 抽屉里还需要进入下一层（例如详情里再看子记录）时，可以嵌套抽屉，子抽屉打开会把父抽屉推开。
- 只是一句确认或一个短表单，用 [Modal 对话框](/components/modal) 或 [Confirm 确认框](/components/confirm)；需要常驻页面、可拖动分隔的侧栏，用布局组件而不是抽屉。

## 在线体验

<GalleryPreview component="Drawer" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var open bool

func page(c *ui.Context) {
	if ui.Button(c, "打开抽屉").Primary().Clicked() {
		open = true
	}
	ui.Drawer(c, &open, "基础抽屉").Body(func() {
		ui.Text(c, "抽屉内容……")
		ui.Text(c, "支持任意组件。")
	})
}
```

`ui.Drawer(c, &open, title)` 在 `open` 为 `true` 时从右侧（RTL 下从左侧）滑出。右上角关闭按钮、点击遮罩和 Esc 会把 `open` 置为 `false`，本帧 `Closed()` 返回 `true`；其他关闭方式（例如页脚的“完成”按钮）由你把 `open` 设为 `false`。

## 常见用法

### 方向与尺寸

`Placement` 选择滑出的边：`ui.DrawerEnd`（默认，LTR 为右侧）、`ui.DrawerStart`（LTR 为左侧）、`ui.DrawerTop`、`ui.DrawerBottom`；`ui.DrawerRight` / `ui.DrawerLeft` 是不随 RTL 镜像的物理方向。`Size(px)` 设置宽度（左右方向）或高度（上下方向），默认 Ant Design 378、Element Plus 窗口的 30%；`Large()` 是 736 的预设。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var left, top bool

func page(c *ui.Context) {
	ui.Row(c, func() {
		if ui.Button(c, "左侧").Clicked() {
			left = true
		}
		if ui.Button(c, "顶部").Clicked() {
			top = true
		}
	}, layout.Gap(8))
	ui.Drawer(c, &left, "左侧抽屉").Placement(ui.DrawerStart).Body(func() { ui.Text(c, "从左侧滑出") })
	ui.Drawer(c, &top, "顶部抽屉").Placement(ui.DrawerTop).Size(200).Body(func() { ui.Text(c, "从顶部滑出") })
}
```

### 标题栏操作与页脚

`Extra(fn)` 在标题栏末端放置操作按钮，`Footer(fn)` 在底部放置固定的页脚（内容区单独滚动）。页脚按钮的逻辑由你处理，例如校验通过后关闭抽屉。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	editOpen bool
	title    string
)

func page(c *ui.Context) {
	if ui.Button(c, "编辑文章").Clicked() {
		editOpen = true
	}
	ui.Drawer(c, &editOpen, "编辑文章").Size(480).
		Extra(func() {
			ui.Button(c, "预览").Small()
		}).
		Footer(func() {
			if ui.Button(c, "取消").Clicked() {
				editOpen = false
			}
			if ui.Button(c, "保存").Primary().Clicked() && title != "" {
				editOpen = false
			}
		}).
		Body(func() {
			ui.Input(c, &title).Placeholder("文章标题")
		})
}
```

### 多层嵌套

在一个抽屉的 `Body` 里再打开 `Drawer`，子抽屉滑出时父抽屉向外推开，层次清楚；关闭子抽屉后父抽屉回到原位。Esc 只关闭最上层的抽屉。

```go
import (
	"github.com/tangoui/tango/ui"
)

var parent, child bool

func page(c *ui.Context) {
	if ui.Button(c, "订单详情").Primary().Clicked() {
		parent = true
	}
	ui.Drawer(c, &parent, "订单详情").Size(520).Body(func() {
		ui.Text(c, "订单号 20261113")
		if ui.Button(c, "查看物流").Clicked() {
			child = true
		}
		ui.Drawer(c, &child, "物流信息").Size(360).Body(func() {
			ui.Text(c, "已签收")
		})
	})
}
```

### 拖动边缘调整大小

`Resizable(true)` 让面向页面的那条边可以拖动；这条边也是可聚焦的分隔条，方向键每次 10px（Shift 50px），Home / End 到上下限。`SizeLimits(min, max)` 限定范围，`BindSize(&px)` 绑定当前尺寸，便于持久化用户的选择；`Resized()` 报告本帧用户改变了尺寸。

```go
import (
	"fmt"

	"github.com/tangoui/tango/ui"
)

var (
	open  bool
	width float32 = 420
)

func page(c *ui.Context) {
	if ui.Button(c, "可调整宽度的抽屉").Clicked() {
		open = true
	}
	ui.Text(c, fmt.Sprintf("当前宽度 %.0fpx", width)).Secondary()
	ui.Drawer(c, &open, "可调整宽度").Resizable(true).SizeLimits(280, 720).BindSize(&width).
		Body(func() { ui.Text(c, "拖动左侧边缘调整宽度。") })
}
```

### 遮罩与关闭方式

`Mask(false)` 不画遮罩，`MaskClosable(false)` 点击遮罩不关闭，`Closable(false)` 去掉关闭按钮，`NoHeader()` 去掉整个标题栏（这时需要在内容里自己提供关闭入口）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var filterOpen bool

func page(c *ui.Context) {
	if ui.Button(c, "筛选").Clicked() {
		filterOpen = true
	}
	ui.Drawer(c, &filterOpen, "筛选").MaskClosable(false).Body(func() {
		if ui.Button(c, "应用并关闭").Primary().Clicked() {
			filterOpen = false
		}
	})
}
```

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 默认尺寸 | 378 | 窗口的 30% | 378 |
| 标题 | 16px 加粗，标题栏下有分隔线 | 次要文字色、常规字重，无分隔线 | 18px 半粗（600），无分隔线（对应 Sheet） |
| 页脚 | 上方有分隔线 | 无分隔线 | 无分隔线，四周 24 留白 |
| 边框 | 无 | 无 | 面向页面的一侧有细边框 |

组件令牌在 `theme.Components.Drawer`（`theme.DrawerToken`）：`Bg`、`TitleColor`、`TitleFontSize`、`TitleFontWeight`、`HeaderPaddingTop` / `HeaderPaddingBottom` / `HeaderPaddingInline`、`HeaderBorderColor`、`Padding`、`BodyPaddingTop`、`FooterPaddingTop` / `FooterPaddingBlock` / `FooterPaddingInline`、`FooterBorderColor`、`CloseIconColor`、`BorderColor`、`MaskBg`、`Shadow`，以及 Resizable 相关的 `ResizeHandleSize`、`ResizeLineColor`、`ResizeLineSize`。

`ConfigProvider.Defaults("Drawer", ui.Props{...})` 支持的键：`placement`（`ui.DrawerPlacement` 或字符串 `"left"` / `"top"` / `"bottom"`，其中 `"left"` 是逻辑起始边）、`size`（数字或 `"large"`）、`width`、`closable`、`mask`、`maskClosable`。

## 注意事项

- **状态放在哪**：`open` 和抽屉里的表单数据放在包级变量或结构体字段里；`Resizable` 选出的尺寸在抽屉存在期间由组件保存，需要跨会话保留时用 `BindSize`。
- **标题即身份**：抽屉的内部状态（动画、尺寸）由“作用域 + 标题”标识，打开期间不要改标题；同一作用域内不要有两个同名抽屉，必要时用 `ui.Key` 包住。
- **焦点**：打开后焦点移入抽屉并被限制在抽屉内，关闭后回到打开前的控件；Esc 关闭最上层抽屉。
- **RTL**：`DrawerEnd` / `DrawerStart` 随方向镜像，需要固定物理边时用 `DrawerRight` / `DrawerLeft`。

## 示例代码

<!--@include: ./_gen/drawer-examples.md-->

## API

<!--@include: ./_gen/drawer-api.md-->

## 相关链接

- [源码 ui/feedback.go](https://github.com/tangoui/tango/blob/main/ui/feedback.go)
- [Go API 参考](/reference/api/ui)
- [Modal 对话框](/components/modal)、[DrawerForm（见 ModalForm）](/components/catalog/modalform)
- [组件总览](/components/)
