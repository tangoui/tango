---
title: ProDescriptions 高级描述列表
---
# ProDescriptions 高级描述列表

按字段定义展示详情数据，并支持远程加载和单项行内编辑。

## 何时使用

- 规则、订单、用户等详情页的结构化字段展示。
- 需要按 `ValueType` 格式化金额、日期、状态或进度时使用。
- 静态少量字段可用 [Descriptions 描述列表](/components/descriptions)。

## 在线体验

<GalleryPreview component="ProDescriptions" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var detail = ui.ProValues{"name": "TradeCode 99", "amount": 12800.5}

func page(c *ui.Context) {
    items := []ui.ProDescItem{{Key: "name", Label: "名称"}, {Key: "amount", Label: "金额", ValueType: ui.ValueMoney}}
    ui.ProDescriptions(c, items).Title("规则详情").Column(2).Bordered().DataSource(&detail).Show()
}
```

## 常见用法

### 远程数据

```go
func remote(c *ui.Context, items []ui.ProDescItem) {
    ui.ProDescriptions(c, items).Request(func() (map[string]any, error) {
        return map[string]any{"name": "TradeCode 99"}, nil
    }).Show()
}
```

### 行内编辑

```go
func editable(c *ui.Context, items []ui.ProDescItem) {
    ui.ProDescriptions(c, items).Editable(func(key string, v any, all ui.ProValues, done func(error)) {
        done(nil)
    }).Show()
}
```

### 布局和尺寸

```go
func vertical(c *ui.Context, items []ui.ProDescItem) {
    ui.ProDescriptions(c, items).Column(3).Size(ui.Middle).Vertical().Show()
}
```

## 主题与样式

ProDescriptions 复用描述列表、ProField 和 Pro 值类型的令牌；状态、标签和金额与 ProTable 显示一致。

## 注意事项

`DataSource` 绑定外部值；`Request` 的加载状态和错误由组件保存。编辑回调需要调用 `done`，失败时传入错误以保持编辑状态。字段 `Key` 必须稳定。

## 示例代码

<!--@include: ./_gen/prodescriptions-examples.md-->

## API

<!--@include: ./_gen/prodescriptions-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/pro3_descriptions.go)
- [ProTable 高级表格](/components/protable)
