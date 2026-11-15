---
title: Collapse 折叠面板
---
# Collapse 折叠面板

把相关内容收进可展开、可收起的面板，并用调用方状态保存当前展开项。

## 何时使用

- FAQ、设置分组、详情摘要等需要按需展开的内容。
- 多个面板可同时打开时使用普通模式；需要一次只看一组内容时使用 `Accordion()`。
- 页面级导航或滚动目录不要用 Collapse，应使用 [Anchor 锚点](/components/anchor)；互斥的视图切换使用 [Tabs 标签页](/components/tabs)。
- 侧栏需要随窗口断点自动折叠时，使用本页的 `CollapsibleSider`，而不是把整个导航塞进 Collapse。

## 在线体验

<GalleryPreview component="Collapse" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var openPanels = []string{"intro"}

func page(c *ui.Context) {
    ui.Collapse(c, &openPanels, []ui.CollapseItem{
        {Key: "intro", Label: "简介", Body: func() { ui.Text(c, "这里是简介内容") }},
        {Key: "details", Label: "详情", Body: func() { ui.Text(c, "这里是详情内容") }},
    }).Show()
}
```

`active` 是展开面板 key 的切片指针。即时模式每帧都会根据它重绘，状态应放在页面状态或包级变量中，而不是在 `page` 函数里每次重新初始化。

## 常见用法

### 手风琴

`Accordion()` 限制最多一个面板展开，适合互斥的说明或步骤。

```go
import "github.com/tangoui/tango/ui"

var accordionOpen = []string{"step-1"}

func page(c *ui.Context) {
    ui.Collapse(c, &accordionOpen, []ui.CollapseItem{
        {Key: "step-1", Label: "第一步", Body: func() { ui.Text(c, "准备数据") }},
        {Key: "step-2", Label: "第二步", Body: func() { ui.Text(c, "提交数据") }},
    }).Accordion().Show()
}
```

### 禁用面板与额外操作

`Disabled` 禁止打开面板，`Extra` 在标题行尾渲染操作或状态；额外节点应避免触发与标题相冲突的动作。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var open = []string{"settings"}

func page(c *ui.Context) {
    ui.Collapse(c, &open, []ui.CollapseItem{
        {Key: "settings", Label: "设置", Extra: func() { ui.Icon(c, icon.Setting).Size(14) }, Body: func() { ui.Text(c, "可编辑设置") }},
        {Key: "locked", Label: "锁定内容", Disabled: true, Body: func() { ui.Text(c, "不可用") }},
    }).Show()
}
```

### 无边框与尺寸

`Borderless()`、`Ghost()` 可用于卡片内部或页面背景；`Size(ui.Small)` 用于紧凑区域，尺寸只影响标题区域的内边距。

```go
import "github.com/tangoui/tango/ui"

var smallOpen = []string{"one"}

func page(c *ui.Context) {
    ui.Collapse(c, &smallOpen, []ui.CollapseItem{
        {Key: "one", Label: "紧凑面板", Body: func() { ui.Text(c, "内容") }},
    }).Borderless().Size(ui.Small).Show()
}
```

### 侧栏折叠

`CollapsibleSider` 将折叠状态绑定到 `*bool`，带底部触发条；`Breakpoint` 在窄窗口自动收起，菜单应复用同一个状态。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var (
    siderCollapsed bool
    selected       = "home"
    menuItems      = []ui.MenuItem{{Key: "home", Label: "首页"}, {Key: "settings", Label: "设置"}}
)

func page(c *ui.Context) {
    ui.Layout(c, func() {
        ui.CollapsibleSider(c, 220, &siderCollapsed).Breakpoint(ui.LG).Body(func() {
            ui.Menu(c, menuItems, &selected).Collapsed(siderCollapsed).Show()
        })
        ui.Content(c, func() { ui.Text(c, "当前：" + selected) }, layout.Pad(24))
    })
}
```

## 主题与样式

Collapse 的边框、标题背景、标题内边距和箭头颜色来自主题组件令牌，三套主题会分别采用 Ant Design 的分组边框、Element Plus 的简洁标题和 shadcn 的卡片式分隔。需要统一默认间距时优先通过主题配置覆盖，不要在每个 Body 中重复画边框。

## 注意事项

- `active` 的 key 必须稳定且唯一；`IsOpen(key)` 可读取当前状态，`Changed()` 可检测本帧是否发生切换。
- 面板内容仍会参与当前帧构建；很重的内容应拆分为更小的页面区域，避免在大量面板中重复创建大数据视图。
- 标题和额外操作应提供清晰文本；面板标题可用 Tab 聚焦并用 Enter 或空格切换。自定义图标按钮要设置 `A11yLabel`。
- RTL 作用域下箭头位置和方向会自动镜像；不要用固定的左右坐标覆盖它。

## 示例代码

<!--@include: ./_gen/collapse-examples.md-->

## API

<!--@include: ./_gen/collapse-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_collapse.go)
- [ui Go API](/reference/api/ui)
- [Layout 布局](/components/layout)
- [Anchor 锚点](/components/anchor)
