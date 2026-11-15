---
title: Slider 滑块
---
# Slider 滑块

在指定数值范围内拖动选择一个值，适合音量、缩放、亮度或阈值设置。

## 在线体验

拖动滑块，或聚焦画布后按左右方向键，观察当前值。 工具栏支持 Ant Design、Element Plus、shadcn 以及深浅色切换；重置会恢复初始状态。

<ComponentPreview component="slider" title="Slider 滑块" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "fmt"
)

var level float64 = 35

func page(c *ui.Context) {
    ui.Slider(c, &level, 0, 100).Step(1)
    ui.Text(c, fmt.Sprintf("Value: %.0f", level))
}
```

## 常用参数与方法

| 参数 / 方法 | 说明 |
| --- | --- |
| `val *float64` | 当前数值，由应用持有 |
| `min, max float64` | 允许的数值区间 |
| `Step(float64)` | 步长 |
| `Marks(map[float64]string)` | 刻度标签 |
| `WithInput()` | 组合数值输入框 |
| `Disabled(bool)` | 禁用滑块 |
| `Changed() bool` | 当前帧是否发生数值变化 |

## 使用说明

范围选择使用 ui.SliderRange(c, &lo, &hi, min, max)。键盘方向键可细调数值，精确输入可使用 WithInput。

## 更多场景

官方 gallery 里这个组件的全部示例（基础、进阶与组合场景），可切换场景、主题和深浅色。

<GalleryPreview component="Slider" />

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/p2b_slider.go)
- [组件截图目录](/components/screenshots)
- [组件总览](/components/)
- [ui Go API](/reference/api/ui)

## 全部示例代码

<!--@include: ./_gen/slider-examples.md-->

## 完整 API

<!--@include: ./_gen/slider-api.md-->
