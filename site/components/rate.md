---
title: Rate 评分
---
# Rate 评分

用星级或自定义字符收集和展示评分。

## 何时使用

- 商品评价、满意度反馈和质量评级。
- 只读评分可禁用交互；数值精度要求高时使用输入框或滑块。

## 在线体验

<GalleryPreview component="Rate" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var score float64 = 3

func page(c *ui.Context) {
    ui.Rate(c, &score).A11yLabel("服务评分")
}
```

## 常见用法

### 半星与清除

```go
func half(c *ui.Context, value *float64) {
    ui.Rate(c, value).AllowHalf().AllowClear(true)
}
```

### 文案和分数

```go
func labels(c *ui.Context, value *float64) {
    ui.Rate(c, value).Texts("很差", "较差", "一般", "满意", "惊喜").ShowScore()
}
```

### 自定义字符和只读

```go
func readOnly(c *ui.Context, value *float64) {
    ui.Rate(c, value).Count(10).Character("♥").Disabled(true)
}
```

## 主题与样式

评分令牌控制星形尺寸、间距、选中色与未选中色；`Size` 和 `Color` 可对单个组件覆盖。

## 注意事项

`Changed` 在用户修改本帧返回 true，应用应在这里保存分值。半星模式下值以 0.5 为步长；提供可读标签，确保屏幕阅读器能报告当前评分。

## 示例代码

<!--@include: ./_gen/rate-examples.md-->

## API

<!--@include: ./_gen/rate-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/entry_rate.go)
- [Slider 滑块](/components/slider)
