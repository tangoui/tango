---
title: Steps 步骤条
---
# Steps 步骤条

引导用户按流程完成任务的导航条，显示一共有几步、当前在哪一步、哪些步骤已完成或出错。

## 何时使用

- 任务较复杂或有先后顺序，需要拆成多个步骤完成（注册、下单、发布审批）。
- 展示一个流程的当前进度，例如订单状态、部署阶段。
- 多步骤的表单可以直接用 StepsForm，它把 Steps 和每一步的表单校验组合好了；按时间排列的事件记录用 Timeline 时间轴；单纯的百分比进度用 Progress 进度条。

## 在线体验

下面是官方 gallery 中 Steps 的全部场景：基础与小尺寸、图标与错误状态、可点击、点状、标签竖排、竖直方向、内联 / 面板 / 自定义点，以及导航型与进度。

<GalleryPreview component="Steps" />

## 基本用法

步骤数据是 `[]ui.StepItem`，`Current(i)` 设置当前步骤（从 0 开始），最后调用 `Show()`。当前步骤之前的步骤显示为已完成，之后的为等待。下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var steps = []ui.StepItem{
    {Title: "填写信息", Description: "姓名与地址"},
    {Title: "确认订单", SubTitle: "剩余 00:00:08", Description: "核对商品"},
    {Title: "完成支付"},
}

var current = 0

func page(c *ui.Context) {
    ui.Steps(c, steps).Current(current).Show()
    ui.Row(c, func() {
        if current > 0 && ui.Button(c, "上一步").Clicked() {
            current--
        }
        if current < len(steps)-1 && ui.Button(c, "下一步").Primary().Clicked() {
            current++
        }
    }, layout.Gap(8))
}
```

`StepItem` 的 `Title` 是标题，`SubTitle` 显示在标题右侧，`Description` 显示在标题下方。

## 常见用法

### 状态与图标

`Status` 设置当前步骤的状态：`"wait"`、`"process"`（默认）、`"finish"`、`"error"`，常用来标出“审核未通过”。单个步骤也可以用 `StepItem.Status` 单独覆盖。`StepItem.Icon` 用图标替换序号圆圈。`FinishStatus("success")` 让已完成的步骤显示为成功色（Element Plus 的 `finish-status`）。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func page(c *ui.Context) {
    ui.Steps(c, []ui.StepItem{
        {Title: "登录", Icon: icon.User},
        {Title: "验证", Icon: icon.Lock},
        {Title: "支付", Icon: icon.Shop},
        {Title: "完成", Icon: icon.Smile},
    }).Current(2).Show()

    ui.Steps(c, []ui.StepItem{
        {Title: "提交"},
        {Title: "审核", Description: "审核未通过"},
        {Title: "完成"},
    }).Current(1).Status("error").Show()

    ui.Steps(c, []ui.StepItem{{Title: "提交"}, {Title: "审核"}, {Title: "完成"}}).
        Current(2).FinishStatus("success").Show()
}
```

### 可点击切换

`Clickable(&cur)` 绑定当前步骤并允许点击任意步骤切换，`Changed()` 报告本帧是否切换过。不允许跳到的步骤设置 `StepItem.Disabled`。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var cur = 0

var items = []ui.StepItem{
    {Title: "基本信息"},
    {Title: "配置规则"},
    {Title: "发布", Disabled: true},
}

func page(c *ui.Context) {
    if ui.Steps(c, items).Clickable(&cur).Show().Changed() {
        fmt.Println("切换到第", cur+1, "步")
    }
    switch cur {
    case 0:
        ui.Text(c, "基本信息表单")
    case 1:
        ui.Text(c, "规则配置表单")
    }
}
```

### 方向、尺寸与标签位置

- `Vertical()`：竖直排列，适合放在页面侧边或步骤描述较长时。
- `Small()` 或 `Size(ui.Small)`：小尺寸。
- `LabelVertical()`：标题放在图标下方。
- `Dot()`：点状步骤条，标题在点的下方。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var three = []ui.StepItem{
    {Title: "已完成", Description: "这是一段描述"},
    {Title: "进行中", Description: "这是一段描述"},
    {Title: "待处理", Description: "这是一段描述"},
}

func page(c *ui.Context) {
    ui.Steps(c, three).Current(1).Small().Show()
    ui.Steps(c, three).Current(1).Dot().Show()
    ui.Steps(c, three).Current(2).LabelVertical().Show()
    ui.Row(c, func() {
        ui.Steps(c, three).Vertical().Current(1).Show()
        ui.Steps(c, three).Vertical().Dot().Current(1).Show()
    }, layout.Gap(48))
}
```

### 导航型、内联与面板

`Type` 选择步骤条类型（Ant 的 `type`）：

| 类型 | 写法 | 外观 |
| --- | --- | --- |
| 默认 | `Type("default")` | 序号圆圈 + 连接线 |
| 导航型 | `Type("navigation")` | 像标签页一样带箭头分隔，当前步骤有下划线；Element Plus 风格下即 simple 简洁步骤条（也接受 `"simple"`） |
| 内联 | `Inline()` 或 `Type("inline")` | 紧凑的小点 + 标题，按内容宽度，适合放进列表行和卡片；不画描述 |
| 面板 | `Panel()` 或 `Type("panel")` | 每步一个箭头形面板，当前步骤高亮 |

```go
import "github.com/tangoui/tango/ui"

var (
    navCur    = 0
    inlineCur = 1
)

var order = []ui.StepItem{
    {Title: "填写信息", Description: "姓名与地址"},
    {Title: "确认订单", Description: "核对商品"},
    {Title: "完成支付", Description: "选择支付方式"},
}

func page(c *ui.Context) {
    ui.Steps(c, order).Type("navigation").Clickable(&navCur).Show()
    ui.Steps(c, order).Inline().Clickable(&inlineCur).Show()
    ui.Steps(c, order).Panel().Current(1).Show()
}
```

### 进度与自定义点

`Percent(p)` 在当前步骤图标外圈画出 0–100 的进度环（Ant 的 `percent`），适合“上传中 60%”这类单步耗时较长的场景。`DotRender` 使用点状样式并由回调绘制每个点：回调拿到 `ui.StepDot`（序号、状态、标题、点的矩形和默认颜色），可先调用 `d.Default()` 画默认点再叠加装饰。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/paint"
)

var items = []ui.StepItem{{Title: "上传"}, {Title: "校验", Description: "已完成 60%"}, {Title: "发布"}}

func page(c *ui.Context) {
    ui.Steps(c, items).Current(1).Percent(60).Show()

    ui.Steps(c, items).Current(1).DotRender(func(l *paint.List, d ui.StepDot) {
        d.Default()
        if d.Status == "process" { // 当前点外加一圈
            p := d.Rect.Center()
            l.StrokePath(paint.NewPath().Circle(p.X, p.Y, 8), paint.StrokeStyle{Width: 1.5}, d.Color)
        }
    }).Show()
}
```

## 主题与样式

三套风格的颜色、尺寸都来自各自的 Steps 令牌；可点击步骤的悬停反馈也按各自规范区分：

- Ant Design：悬停时标题和描述变色，等待中的步骤图标也一起变色。
- Element Plus：只有等待中的步骤响应悬停；导航型呈现为 el-steps 的 simple 简洁步骤条，`FinishStatus` 对应 `finish-status`。
- shadcn：悬停时只有标题变色（等待中的步骤图标也变色）。
- 组件令牌：`theme.Components.Steps`（`theme.StepsToken`），包括图标尺寸（`IconSize`、`IconSizeSM`、导航型的 `NavIconSize`）、点尺寸（`DotSize`、`DotCurrentSize`）、标题与描述字号、连接线粗细 `TailWidth`，以及各状态的图标、标题、描述颜色（`ProcessIconBg`、`ProcessTitleColor`……）。
- `ConfigProvider.Defaults("Steps", ...)` 键：`"type"`（`"default"` / `"navigation"`）、`"size"`（`ui.ControlSize` 或 `"small"`）、`"labelPlacement"`（`"vertical"`）、`"direction"`（`"vertical"`）、`"progressDot"`（bool）。

## 注意事项

- **状态放在哪**：当前步骤是调用方的 `int`（`Current` 传值、`Clickable` 传指针），放在包级变量或页面状态结构里；Steps 本身不保存“当前是第几步”。
- **从 0 开始**：`Current` 和 `Clickable` 的序号都是 0 基；`Current(len(items))` 表示全部完成。
- **只读还是可点**：只展示进度时用 `Current`；用 `Clickable` 时用户可以跳到任意未禁用步骤，表单类流程应先校验当前步骤再允许前进，或者把后面的步骤设为 `Disabled`。
- **无障碍**：可点击的步骤可用 Tab 聚焦、回车或空格切换；内联类型不绘制描述，但描述仍作为无障碍说明提供给读屏软件。
- **RTL**：在从右到左的作用域里，步骤顺序、连接线与导航型箭头自动镜像。

## 示例代码

<!--@include: ./_gen/steps-examples.md-->

## API

<!--@include: ./_gen/steps-api.md-->

## 相关链接

- 源码：[ui/nav_steps.go](https://github.com/tangoui/tango/blob/main/ui/nav_steps.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Menu 导航菜单](/components/menu)、[Breadcrumb 面包屑](/components/breadcrumb)、[Tabs 标签页](/components/tabs)
