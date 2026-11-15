---
title: Result 结果
---
# Result 结果

用于反馈操作结果，并集中展示状态图标、标题、说明和后续操作。

## 何时使用

- 提交、支付、授权等流程完成后需要一个完整结果页时使用。
- 需要提供“返回列表”“再试一次”等后续操作时使用。
- 只有一句短反馈时用 [Alert](/components/alert) 或 [Message](/components/message)；错误字段需要定位时用 [Form](/components/form)。

## 在线体验

<GalleryPreview component="Result" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func resultPage(c *ui.Context) {
    ui.Result(c, "success", "提交成功").
        SubTitle("订单已创建，稍后可以在列表中查看").Show()
}
```

`status` 可以是 `success`、`error`、`info`、`warning`，也可以使用 `403`、`404`、`500` 等错误页状态。`Show()` 构建没有底部内容的结果页。

## 常见用法

### 操作按钮

```go
func resultActions(c *ui.Context) {
    ui.Result(c, "error", "提交失败").SubTitle("请检查网络后重试").Extra(func() {
        ui.Button(c, "重新提交").Primary()
        ui.Button(c, "返回编辑")
    }).Show()
}
```

`Extra` 构建操作区，回调中可放多个按钮。

### 结果页内容

```go
func resultBody(c *ui.Context) {
    ui.Result(c, "info", "正在处理").SubTitle("预计需要几秒钟").Extra(func() {
        ui.Button(c, "返回列表")
    }).Body(func() {
        ui.Text(c, "任务编号：T-1024")
    })
}
```

`Body` 在操作区下方构建补充内容；它返回布局盒，可继续组合布局。

### 自定义图标

```go
func resultIcon(c *ui.Context) {
    ui.Result(c, "success", "导入完成").Icon(func() {
        ui.Text(c, "OK")
    }).Show()
}
```

`Icon` 用于替换默认状态图标，适合放置自定义插画或图标内容。

### 错误状态

```go
func resultNotFound(c *ui.Context) {
    ui.Result(c, "404", "页面不存在").SubTitle("请确认地址是否正确").Extra(func() {
        ui.Button(c, "返回首页").Primary()
    }).Show()
}
```

错误页的状态字符串由组件解释为对应插画；保持标题和操作文案清晰，避免只显示错误码。

## 主题与样式

Result 的状态图标、标题层级、间距和内容面板由主题决定。Ant Design 的 `Body` 作为灰色内容区域，Element Plus 将内容放在底部插槽，三套主题均保留同一 Go API。需要统一调整时应通过主题令牌完成，而不是在每个页面加固定颜色。

## 注意事项

结果页通常是流程状态的一部分，成功或失败状态应来自页面状态而不是每帧递增。按钮回调中的异步结果应保存到页面级状态，再在下一帧重新构建。自定义图标和标题仍应提供足够的文本信息，避免只依赖颜色或图形传达结果。

## 示例代码

<!--@include: ./_gen/result-examples.md-->

## API

<!--@include: ./_gen/result-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_result.go)
- [ui Go API](/reference/api/ui)
- [Alert 警告提示](/components/alert)
