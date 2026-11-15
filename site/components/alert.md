---
title: Alert 警告提示
---
# Alert 警告提示

以横幅形式展示需要用户注意的成功、信息、警告或错误状态。

## 何时使用

- 在页面内容中展示持续可见的状态、风险或操作结果。
- 需要用户主动关闭，或需要附加操作（如撤销、重试）时使用。
- 只需短暂反馈时用 [Message](/components/message)；需要确认操作时用 [Popconfirm](/components/catalog/popconfirm)。

## 在线体验

<GalleryPreview component="Alert" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func alertPage(c *ui.Context) {
    ui.Alert(c, "info", "有新版本可用").Show()
}
```

`status` 支持 `success`、`info`、`warning`、`error`。链式设置完成后调用 `Show()` 构建组件。

## 常见用法

### 辅助说明与关闭

```go
func alertDescription(c *ui.Context) {
    ui.Alert(c, "warning", "请检查配置").
        Description("保存前仍有两个必填项未完成。").Closable().Show()
}
```

`Closable()` 显示关闭按钮；`Closed()` 可在后续即时渲染中读取关闭动作并更新业务状态。

### 深色效果与居中

```go
func alertEffects(c *ui.Context) {
    ui.Alert(c, "success", "发布成功").Effect(ui.AlertEffectDark).Center().Show()
    ui.Alert(c, "error", "发布失败").NoIcon().Show()
}
```

`Effect(ui.AlertEffectDark)` 使用状态色实底；`Center()` 居中文本和图标；`NoIcon()` 隐藏状态图标。

### 附加操作

```go
func alertAction(c *ui.Context) {
    ui.Alert(c, "info", "草稿已保存").Action(func() {
        ui.Button(c, "撤销").Small()
    }).Show()
}
```

`Action` 的回调构建右侧操作区，适合放置小型按钮；不要在其中塞入复杂表单。

### Banner 模式

```go
func alertBanner(c *ui.Context) {
    ui.Alert(c, "warning", "系统将在今晚维护").Banner().Show()
}
```

`Banner()` 适合放在页面或窗口顶部的全宽提示。

## 主题与样式

Alert 的状态色、圆角、间距和图标由当前主题控制。可通过 `ConfigProvider.Defaults("Alert", ...)` 设置 `showIcon`、`closable`、`banner` 三个布尔默认值；单个实例的 `NoIcon`、`Closable` 和 `Banner` 会覆盖默认值。Ant Design 使用状态色浅底，Element Plus 支持 `EffectDark`，shadcn 使用带边框的 Alert 卡片。

## 注意事项

即时模式下，把是否显示、业务文案等状态放在页面函数外或 `ui.State` 中，避免每帧重置。关闭后应由业务状态决定是否再次构建。可关闭提示必须提供明确文案；键盘用户可以聚焦关闭按钮并用 Enter 或空格操作。

## 示例代码

<!--@include: ./_gen/alert-examples.md-->

## API

<!--@include: ./_gen/alert-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/feedback.go)
- [ui Go API](/reference/api/ui)
- [Message 消息提示](/components/message)
- [Notification 通知提醒](/components/notification)
