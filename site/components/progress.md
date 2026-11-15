---
title: Progress 进度条
---
# Progress 进度条

展示任务完成比例，支持线型、环形、仪表盘、分段和不确定进度。

## 何时使用

- 上传、导入、处理等有明确完成比例的长任务。
- 需要在卡片中展示评分、容量或完成度时使用环形进度。
- 无法估计进度时使用 `Indeterminate()`；只表示正在处理时用 [Spin](/components/spin)。

## 在线体验

<GalleryPreview component="Progress" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func progressPage(c *ui.Context) {
    ui.Progress(c, 68)
}
```

百分比范围是 0 到 100，默认显示线型进度和信息文字。

## 常见用法

### 状态与文字

```go
func progressStatus(c *ui.Context) {
    ui.Progress(c, 100).Status("success")
    ui.Progress(c, 72).Status("active")
    ui.Progress(c, 40).Status("exception").Format(func(p float64) string { return "失败" })
}
```

`Status` 支持 `active`、`success`、`exception`、`warning`；`Format` 自定义信息文字。

### 环形与仪表盘

```go
func progressCircle(c *ui.Context) {
    ui.Progress(c, 75).Circle(120)
    ui.Progress(c, 60).Dashboard(120).GapDegree(90)
}
```

`Circle` 为完整圆环，`Dashboard` 为带缺口的仪表盘，参数是直径。

### 分段、渐变和条纹

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

func progressStyles(c *ui.Context) {
    ui.Progress(c, 80).Steps(5).Color(theme.Hex("#1677ff"))
    ui.Progress(c, 60).Gradient(theme.Hex("#108ee9"), theme.Hex("#87d068"))
    ui.Progress(c, 50).Striped(true).TextInside()
}
```

`Steps` 分割为固定段数，`Gradient` 设置渐变，`Striped(true)` 显示流动条纹，`TextInside` 将文字放入条内。

### 不确定进度与无障碍

```go
func progressA11y(c *ui.Context) {
    ui.Progress(c, 0).Indeterminate().A11yLabel("正在同步数据")
    ui.Progress(c, 45).HideInfo().A11yLabel("文件上传进度")
}
```

无法估计完成比例时不要伪造百分比。`A11yLabel` 为进度指示器设置可访问名称，`HideInfo` 隐藏视觉文字但不替代无障碍名称。

## 主题与样式

Progress 的状态色、轨道色、线宽和动画由主题控制。使用 `Color`、`TrailColor`、`StrokeWidth` 做局部调整；统一品牌色应配置主题令牌。shadcn 线型默认可不显示文字，调用 `ShowInfo()` 可显式显示；不同主题对圆角、条纹和状态色有对应适配。

## 注意事项

进度值应来自任务状态并限制在 0–100；大任务更新频率过高会增加重绘，可按合适间隔更新。成功、失败和不确定状态应同时用文字或图标表达，不要只依赖颜色。RTL 下线型方向由组件处理，不要手工反转百分比。

## 示例代码

<!--@include: ./_gen/progress-examples.md-->

## API

<!--@include: ./_gen/progress-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_progress.go)
- [ui Go API](/reference/api/ui)
- [Spin 加载中](/components/spin)
