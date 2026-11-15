---
title: Spin 加载中
---
# Spin 加载中

用旋转指示器表示内容正在加载或操作尚未完成。

## 何时使用

- 短时间内无法立即完成的操作，需要保留当前页面上下文时。
- 局部区域加载时用 `Wrap` 覆盖已有内容；全局复杂加载可考虑 Loading。
- 已知内容结构时用 [Skeleton](/components/skeleton)；有明确百分比时用 [Progress](/components/progress)。

## 在线体验

<GalleryPreview component="Spin" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func spinPage(c *ui.Context) {
    ui.Spin(c).Tip("加载中")
}
```

默认显示中等尺寸指示器。`Tip` 在指示器下方显示辅助文案。

## 常见用法

### 尺寸和延迟

```go
import "time"

func spinOptions(c *ui.Context) {
    ui.Spin(c).Size(ui.Small)
    ui.Spin(c).Size(ui.Large)
    ui.Spin(c).Delay(500 * time.Millisecond)
}
```

尺寸支持 `ui.Small`、`ui.Middle`、`ui.Large`。`Delay` 避免极短请求造成闪烁。

### 包裹局部内容

```go
var sectionLoading = true

func spinNested(c *ui.Context) {
    ui.Spin(c).Spinning(sectionLoading).Tip("正在刷新").Wrap(func() {
        ui.Text(c, "这里是待刷新的内容")
    })
}
```

`Wrap` 在加载时让内容变暗并将指示器居中；把 `sectionLoading` 放在页面状态中，异步结束后再更新。

### 百分比进度

```go
func spinPercent(c *ui.Context) {
    ui.Spin(c).Percent(45).Tip("上传中")
}
```

`Percent` 用进度环替代旋转指示器，范围为 0–100；任务无法估计比例时不要使用它。

### 自定义尺寸与停止

```go
var loading = false

func spinControl(c *ui.Context) {
    ui.Spin(c).IndicatorSize(28).Spinning(loading)
}
```

`IndicatorSize` 设置指示器像素尺寸，`Spinning(false)` 隐藏加载状态。

## 主题与样式

Spin 的颜色、动画和默认尺寸由主题控制。与内容叠加时，组件自动处理遮罩和层级；不要通过固定颜色实现主题适配。Percent 模式复用进度环的主题语义。

## 注意事项

加载开始和结束必须由真实业务状态驱动。短请求使用 `Delay`，长请求应提供可读提示，必要时增加取消操作。包裹区域应保留稳定布局，避免加载期间内容消失造成跳动；键盘焦点不应因为局部 Spin 被意外移走。

## 示例代码

<!--@include: ./_gen/spin-examples.md-->

## API

<!--@include: ./_gen/spin-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/feedback.go)
- [ui Go API](/reference/api/ui)
- [Skeleton 骨架屏](/components/skeleton)
