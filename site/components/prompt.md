---
title: Prompt 输入框
---
# Prompt 输入框

用一个命令式对话框请求用户输入单个文本值，并通过校验后回调返回结果。

## 何时使用

- 重命名文件、修改邮箱、输入标签等只需要一个文本值的场景。
- 需要输入前置值、占位提示和同步校验时使用 `Prompt`。
- 需要多个字段或复杂布局时用 [Modal 对话框](/components/modal)；只需要用户确认结果时用 [MessageBox 信息提示](/components/messagebox)；需要“确定 / 取消”而不输入时用 [Confirm 确认框](/components/confirm)。

## 在线体验

<GalleryPreview component="Prompt" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var nickname string

func page(c *ui.Context) {
    if ui.Button(c, "修改昵称").Clicked() {
        ui.Prompt(c, ui.ConfirmOptions{
            Title:       "修改昵称",
            Content:     "请输入新的昵称",
            InputValue:  nickname,
            OnOk:        func(d *ui.ConfirmRef) { nickname = d.Value() },
            OnCancel:    func() { nickname = "未修改" },
        })
    }
    ui.Text(c, "当前昵称："+nickname).Secondary()
}
```

输入框按 Enter 等同于点击确定；`OnOk` 中通过 `ConfirmRef.Value()` 读取提交值。

## 常见用法

### 占位文字与初始值

`InputValue` 设置打开时的值，`InputPlaceholder` 在值为空时显示提示；可以用 `OkText` 和 `CancelText` 让按钮更贴合业务语义。

```go
import "github.com/tangoui/tango/ui"

var branch = "main"

func page(c *ui.Context) {
    if ui.Button(c, "切换分支").Clicked() {
        ui.Prompt(c, ui.ConfirmOptions{
            Title:            "切换分支",
            Content:          "输入要检出的分支名。",
            InputValue:       branch,
            InputPlaceholder: "例如 feature/login",
            OkText:           "切换",
            CancelText:       "保持当前",
            OnOk:             func(d *ui.ConfirmRef) { branch = d.Value() },
        })
    }
}
```

### 输入校验

`Validate` 返回非空字符串时显示错误信息并阻止确定；返回空字符串表示通过。校验函数应保持同步且只依赖传入值。

```go
import (
    "strings"

    "github.com/tangoui/tango/ui"
)

var email string

func page(c *ui.Context) {
    if ui.Button(c, "修改邮箱").Clicked() {
        ui.Prompt(c, ui.ConfirmOptions{
            Title:       "修改邮箱",
            Content:     "请输入新的邮箱地址。",
            InputValue:  email,
            InputPlaceholder: "name@example.com",
            Validate: func(v string) string {
                v = strings.TrimSpace(v)
                if !strings.Contains(v, "@") {
                    return "邮箱格式不正确"
                }
                return ""
            },
            OnOk: func(d *ui.ConfirmRef) { email = strings.TrimSpace(d.Value()) },
        })
    }
}
```

### 确认后更新页面结果

`OnOk` 和 `OnCancel` 都在 UI 线程执行，适合在回调里更新页面状态；如果需要保留对话框引用，可以读取 `Action()` 或 `IsOpen()`。

```go
import "github.com/tangoui/tango/ui"

var (
    promptResult string
    promptRef    *ui.ConfirmRef
)

func page(c *ui.Context) {
    if ui.Button(c, "输入项目名").Clicked() {
        promptRef = ui.Prompt(c, ui.ConfirmOptions{
            Title: "项目名",
            OnOk: func(d *ui.ConfirmRef) {
                promptResult = "项目：" + d.Value()
            },
            OnCancel: func() { promptResult = "已取消" },
        })
    }
    if promptRef != nil && promptRef.IsOpen() {
        ui.Text(c, "等待输入…").Secondary()
    }
    ui.Text(c, promptResult).Secondary()
}
```

### 异步提交并保持对话框

在 `OnOk` 中调用 `Loading(true)` 可以让确定按钮进入加载状态并保持对话框；后台任务完成后调用 `Close`。跨 goroutine 只调用这两个线程安全方法，不直接改页面状态。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
    if ui.Button(c, "提交名称").Clicked() {
        ui.Prompt(c, ui.ConfirmOptions{
            Title: "提交名称",
            OnOk: func(d *ui.ConfirmRef) {
                d.Loading(true)
                go func() {
                    time.Sleep(500 * time.Millisecond) // 模拟请求
                    d.Close()
                }()
            },
        })
    }
}
```

## 主题与样式

Prompt 与 Confirm、MessageBox、Modal 共用 `theme.Components.Modal` 令牌。Ant Design 风格通常在窗口顶部显示，Element Plus 和 shadcn 风格默认居中。`ConfigProvider.Defaults("Confirm", ui.Props{...})` 可设置默认宽度和按钮文字，`ConfirmOptions` 中的值优先。

## 注意事项

- **只在事件中调用**：Prompt 是命令式 API，应放在 `Clicked()` 等一次性分支中，不要每帧调用。
- **输入状态**：输入值由 Prompt 内部保存；不要在每帧重新用旧值覆盖 `InputValue`。需要提交结果时在 `OnOk` 里读取 `d.Value()`。
- **校验时机**：点击确定或按 Enter 时执行 `Validate`；校验失败时对话框保持打开，用户修改后会重新校验。
- **异步回调**：`Loading` 和 `Close` 可从任意 goroutine 调用；其他页面状态更新应通过 channel 等方式交回 UI 线程。
- **焦点与键盘**：打开后焦点进入输入框，Enter 提交，Esc 默认按取消处理，关闭后焦点回到打开前控件。

## 示例代码

<!--@include: ./_gen/prompt-examples.md-->

## API

<!--@include: ./_gen/prompt-api.md-->

## 相关链接

- [源码 ui/more_confirm.go](https://github.com/tangoui/tango/blob/main/ui/more_confirm.go)
- [Go API 参考](/reference/api/ui)
- [Confirm 确认框](/components/confirm)、[MessageBox 信息提示](/components/messagebox)、[Modal 对话框](/components/modal)
- [组件总览](/components/)
