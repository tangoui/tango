---
title: MessageBox 信息提示
---
# MessageBox 信息提示

用一个命令式对话框告知用户结果或状态，只提供一个“确定”按钮，不需要维护打开状态。

## 何时使用

- 操作完成后需要用户确认知晓，例如保存成功、导入完成或请求失败。
- 需要用状态图标区分信息、成功、警告和错误时使用 `MessageBox`。
- 需要用户做“确定 / 取消”选择时用 [Confirm 确认框](/components/confirm)；需要输入内容时用 [Prompt 输入框](/components/prompt)；短暂且无需确认的反馈用 [Message 全局提示](/components/message)。

## 在线体验

<GalleryPreview component="MessageBox" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    if ui.Button(c, "提交订单").Primary().Clicked() {
        ui.MessageBox(c, ui.ConfirmOptions{
            Title:   "订单已提交",
            Content: "我们会尽快为你发货。",
            Status:  "success",
        })
    }
}
```

调用必须放在按钮 `Clicked()` 等一次性事件分支中；对话框会在全局浮层中显示，点击确定后关闭。

## 常见用法

### 使用状态图标

`Status` 支持 `info`、`success`、`warning` 和 `error`，用于匹配结果的语义；未设置时默认为 `info`。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    if ui.Button(c, "检查连接").Clicked() {
        ui.MessageBox(c, ui.ConfirmOptions{
            Title:   "服务暂时不可用",
            Content: "请检查网络后重试。",
            Status:  "error",
            OkText:  "知道了",
        })
    }
}
```

### 居中、拖动和自定义图标

`Center` 让图标、标题、内容和按钮居中，`Draggable` 允许拖动标题，`Icon` 可以换成图标库中的图标。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func page(c *ui.Context) {
    if ui.Button(c, "显示收藏结果").Clicked() {
        ui.MessageBox(c, ui.ConfirmOptions{
            Title:     "收藏成功",
            Content:   "可以拖动标题移动这个对话框。",
            Status:    "success",
            Center:    true,
            Draggable: true,
            Icon:      icon.StarFilled,
        })
    }
}
```

### 在回调中更新页面状态

`OnOk` 在 UI 线程执行；保存返回的 `*ConfirmRef` 后还可以通过 `Action` 判断用户的关闭方式，或用 `IsOpen` 观察当前状态。

```go
import "github.com/tangoui/tango/ui"

var (
    messageRef *ui.ConfirmRef
    result     string
)

func page(c *ui.Context) {
    if ui.Button(c, "打开提示").Clicked() {
        messageRef = ui.MessageBox(c, ui.ConfirmOptions{
            Title: "同步完成",
            OnOk:  func(d *ui.ConfirmRef) { result = "已确认，动作：" + string(d.Action()) },
        })
    }
    if messageRef != nil && messageRef.IsOpen() {
        ui.Text(c, "提示正在显示").Secondary()
    }
    ui.Text(c, result).Secondary()
}
```

### 自定义宽度与遮罩关闭

`Width` 控制对话框宽度；`MaskClosable` 允许点击遮罩关闭，关闭会按取消路径处理。信息提示通常保留默认的单按钮行为。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    if ui.Button(c, "查看详细说明").Clicked() {
        ui.MessageBox(c, ui.ConfirmOptions{
            Title:        "导入完成",
            Content:      "已导入 128 条记录，点击遮罩或确定都可以关闭。",
            Status:       "info",
            Width:        520,
            MaskClosable: true,
            OnCancel:     func() { /* 记录用户关闭了提示 */ },
        })
    }
}
```

## 主题与样式

MessageBox 与 Confirm、Prompt、Modal 共用 `theme.Components.Modal` 令牌。Ant Design 风格默认在窗口顶部展示，状态图标位于内容左侧；Element Plus 和 shadcn 风格使用各自的对话框布局，后者默认居中。可以通过 `ConfigProvider.Defaults("Confirm", ui.Props{...})` 配置默认 `width`、`okText` 等值，选项对象中的显式值优先。

## 注意事项

- **只在事件中调用**：不要在页面函数每帧无条件调用 `MessageBox`，否则会不断创建新对话框。
- **单按钮语义**：MessageBox 只有确定按钮；需要用户在两个动作间选择时改用 `Confirm`。
- **回调线程**：`OnOk`、`OnCancel` 和 `OnClose` 在 UI 线程执行，可以直接更新页面状态。
- **关闭方式**：关闭按钮、Esc 和显式启用的遮罩关闭都走取消回调。`NoKeyboard: true` 禁止 Esc 关闭。启用 `DistinguishCancelAndClose` 时，这些关闭方式会调用 `OnClose`。
- **焦点与无障碍**：打开后焦点进入模态对话框并被限制在其中，关闭后恢复到打开它的控件。

## 示例代码

<!--@include: ./_gen/messagebox-examples.md-->

## API

<!--@include: ./_gen/messagebox-api.md-->

## 相关链接

- [源码 ui/more_confirm.go](https://github.com/tangoui/tango/blob/main/ui/more_confirm.go)
- [Go API 参考](/reference/api/ui)
- [Confirm 确认框](/components/confirm)、[Prompt 输入框](/components/prompt)、[Modal 对话框](/components/modal)
- [组件总览](/components/)
