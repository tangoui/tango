---
title: Confirm 确认框
---
# Confirm 确认框

命令式的对话框：一次函数调用就弹出“确认 / 取消”对话框，结果通过回调返回，不需要维护打开状态。同一组 API 还提供只有一个按钮的信息框 `MessageBox` 和带输入框的 `Prompt`，对应 antd 的 `Modal.confirm` / `Modal.info` 等与 Element Plus 的 `ElMessageBox`。

## 何时使用

- 操作前的二次确认，例如删除、清空、提交审核、离开未保存的页面。
- 告知一个需要用户确认知晓的结果（`MessageBox`），例如“订单已提交”“网络请求失败”。
- 只需要输入一个值（`Prompt`），例如重命名、修改邮箱。
- 内容复杂、包含多个字段或自定义布局时，用 [Modal 对话框](/components/modal)；就地的轻量确认（按钮旁边弹出）用 [Popconfirm](/components/popconfirm)；不需要用户确认的反馈用 [Message](/components/message)。

## 在线体验

<GalleryPreview component="Confirm" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var result string

func page(c *ui.Context) {
	if ui.Button(c, "删除").Danger().Clicked() {
		ui.Confirm(c, ui.ConfirmOptions{
			Title:    "确定要删除这些条目吗？",
			Content:  "删除后无法恢复，请谨慎操作。",
			OnOk:     func(*ui.ConfirmRef) { result = "已删除" },
			OnCancel: func() { result = "已取消" },
		})
	}
	ui.Text(c, result).Secondary()
}
```

`ui.Confirm(c, opts)` 在调用时打开对话框（默认带警告图标、“取消”和“确定”两个按钮），之后由框架在全局浮层中渲染。`OnOk` 在点击确定时执行。默认情况下，取消按钮和 Esc 会触发 `OnCancel`。显示关闭按钮的主题也支持点按钮取消，启用 `MaskClosable` 后可点遮罩取消。两个回调都运行在 UI 线程。`OnOk` 返回后对话框自动关闭，除非它开启了加载状态。

## 常见用法

### 危险操作与按钮文字

`Danger: true` 让确定按钮变红，`Status: "error"` 换成错误图标；`OkText` / `CancelText` 修改按钮文字（默认取语言包的“确定” / “取消”）；`MaskClosable: true` 允许点遮罩关闭（按取消处理）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var trash = []string{"a.txt", "b.txt"}

func page(c *ui.Context) {
	if ui.Button(c, "清空回收站").Danger().Clicked() {
		ui.Confirm(c, ui.ConfirmOptions{
			Title: "清空回收站", Content: "此操作将永久删除全部文件。",
			Status: "error", Danger: true, OkText: "清空", MaskClosable: true,
			OnOk: func(*ui.ConfirmRef) { trash = nil },
		})
	}
}
```

### 异步关闭

在 `OnOk` 里调用 `d.Loading(true)`：确定按钮显示加载状态且不可再点，`OnOk` 返回后对话框保持打开；后台任务完成后调用 `d.Close()` 关闭。`Loading` 和 `Close` 都可以在任意 goroutine 调用，它们会自动请求重绘。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	submitted = make(chan string, 1)
	status    string
)

func page(c *ui.Context) {
	if ui.Button(c, "提交审核").Primary().Clicked() {
		ui.Confirm(c, ui.ConfirmOptions{
			Title: "提交审核", Content: "提交后两秒内完成。", Status: "info",
			OnOk: func(d *ui.ConfirmRef) {
				d.Loading(true)
				go func() {
					time.Sleep(2 * time.Second) // 模拟网络请求
					submitted <- "已提交"
					d.Close() // 不会再调用任何回调
				}()
			},
		})
	}
	select {
	case s := <-submitted:
		status = s
	default:
	}
	ui.Text(c, status).Secondary()
}
```

后台 goroutine 不要直接修改界面状态，把结果通过 channel 交回页面函数处理（`d.Close()` 触发的那一帧会读到它）。失败时可以调用 `d.Loading(false)` 让用户重试，而不关闭对话框。

### 区分“取消”和“关闭”

默认情况下关闭按钮、Esc、点遮罩都按“取消”处理。设置 `DistinguishCancelAndClose: true` 后，这三种方式改为执行 `OnClose`，只有点“取消”按钮才执行 `OnCancel`，适合“保存 / 放弃修改 / 留在此页”这类三选一。`ref.Action()` 返回用户关闭对话框的方式（`ui.ConfirmActionConfirm`、`ui.ConfirmActionCancel`、`ui.ConfirmActionClose`）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var leave string

func page(c *ui.Context) {
	if ui.Button(c, "离开页面").Clicked() {
		ui.Confirm(c, ui.ConfirmOptions{
			Title: "未保存的修改", Content: "离开前是否保存？",
			OkText: "保存", CancelText: "放弃修改",
			DistinguishCancelAndClose: true,
			OnOk:     func(*ui.ConfirmRef) { leave = "已保存" },
			OnCancel: func() { leave = "放弃修改" },
			OnClose:  func() { leave = "留在此页" },
		})
	}
	ui.Text(c, leave).Secondary()
}
```

### 信息提示 MessageBox

`ui.MessageBox` 只有一个确定按钮，`Status` 决定图标（`info` 默认、`success`、`warning`、`error`），对应 antd 的 `Modal.info / success / warning / error` 和 Element Plus 的 `ElMessageBox.alert`。`Center` 让图标、标题、内容和按钮居中，`Draggable` 允许拖动标题，`Icon` 替换状态图标。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

func page(c *ui.Context) {
	if ui.Button(c, "提交订单").Primary().Clicked() {
		ui.MessageBox(c, ui.ConfirmOptions{Title: "订单已提交成功", Content: "我们会尽快为你发货。", Status: "success"})
	}
	if ui.Button(c, "收藏").Clicked() {
		ui.MessageBox(c, ui.ConfirmOptions{Title: "收藏成功", Content: "可以拖动标题移动对话框。",
			Status: "success", Center: true, Draggable: true, Icon: icon.StarFilled})
	}
}
```

MessageBox 的确定按钮在 Ant Design 风格下默认文字为“知道了”。

### 输入框 Prompt

`ui.Prompt` 在内容下方放一个输入框：`InputValue` 是初始值，`InputPlaceholder` 是占位文字，`Validate` 返回错误信息（返回 `""` 表示通过），校验不通过时不能确定。在 `OnOk` 里用 `d.Value()` 读取输入；输入框里按回车等同于点确定。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var email = "old@example.com"

func page(c *ui.Context) {
	if ui.Button(c, "修改邮箱").Clicked() {
		ui.Prompt(c, ui.ConfirmOptions{
			Title: "修改邮箱", Content: "请输入新的邮箱地址",
			InputValue: email, InputPlaceholder: "name@example.com",
			Validate: func(v string) string {
				if !strings.Contains(v, "@") {
					return "邮箱格式不正确"
				}
				return ""
			},
			OnOk: func(d *ui.ConfirmRef) { email = d.Value() },
		})
	}
	ui.Text(c, "当前邮箱："+email).Secondary()
}
```

MessageBox 和 Prompt 各自也有生成的示例页：[MessageBox 信息提示](/components/messagebox)、[Prompt 输入框](/components/prompt)。

## 主题与样式

Confirm / MessageBox / Prompt 与 [Modal](/components/modal#主题与样式) 共用 `theme.Components.Modal` 令牌（背景、圆角、内边距、遮罩色、阴影等）。三种风格的差异：Ant Design 距窗口顶部 100、左侧状态图标 + 标题 + 内容、按钮在末端；Element Plus 按 `ElMessageBox` 布局并垂直居中；shadcn 按 AlertDialog 布局并垂直居中。

`ConfigProvider.Defaults("Confirm", ui.Props{...})` 支持 `width`、`okText`、`cancelText`；未设置的 `okText` / `cancelText` 会再读 `Defaults("Modal", ...)` 的同名键。`ConfirmOptions` 里写明的值优先。

## 注意事项

- **只在事件里调用**：`Confirm` / `MessageBox` / `Prompt` 每调用一次就打开一个新对话框，放在 `if ...Clicked()` 等只执行一次的分支里，不要每帧调用。
- **不需要保存状态**：对话框的状态由框架保存，返回的 `*ui.ConfirmRef` 只在需要时保留（例如要从外部 `Close()`、查询 `IsOpen()` 或 `Action()`）。
- **回调与线程**：`OnOk` / `OnCancel` / `OnClose` 在 UI 线程执行，可以直接改页面状态；`d.Loading` 和 `d.Close()` 可以在任意 goroutine 调用，其余状态改动请交回 UI 线程。
- **焦点**。对话框打开后焦点移入对话框并被限制在其中，Prompt 的焦点在输入框里。关闭后焦点回到打开前的控件。Esc 默认取消，设置 `DistinguishCancelAndClose` 时计为关闭，设置 `NoKeyboard` 时保留对话框。
- **主题与方向**：对话框使用调用处作用域的主题、语言和方向（例如在 RTL 的 `ConfigProvider` 里调用就按 RTL 布局）。

## 示例代码

<!--@include: ./_gen/confirm-examples.md-->

## API

<!--@include: ./_gen/confirm-api.md-->

## 相关链接

- [源码 ui/more_confirm.go](https://github.com/tangoui/tango/blob/main/ui/more_confirm.go)
- [Go API 参考](/reference/api/ui)
- [Modal 对话框](/components/modal)、[Popconfirm](/components/popconfirm)、[MessageBox](/components/messagebox)、[Prompt](/components/prompt)
- [组件总览](/components/)
