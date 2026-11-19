---
title: Card 卡片
---
# Card 卡片

把一组相关的信息放进一个带标题的容器：可以有封面、右上角操作、底部操作栏、页脚和页签。

## 何时使用

- 页面上需要把内容分块展示，例如仪表盘的指标区、详情页的分组、设置页的分区。
- 以卡片栅格展示同类对象（商品、项目、成员），配合 [List 列表](/components/list) 的 `Grid` 使用。
- 需要可折叠、分栏、内嵌统计数字等更复杂的布局，用 `ProCard`；只想加一层背景和圆角、不需要标题等结构时，直接用布局容器的 `Bg` / `Radius` 即可。

## 在线体验

示例包含默认、小尺寸、可悬浮、无边框卡片，封面 + 元信息 + 操作栏，加载中，带页脚、内部卡片，以及带页签的卡片。

<GalleryPreview component="Card" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var showMore bool

func page(c *ui.Context) {
    ui.Card(c, "默认卡片").
        Extra(func() {
            if ui.Link(c, "更多").Clicked() {
                showMore = true
            }
        }).
        Layout(layout.W(320)).
        Body(func() {
            ui.Text(c, "卡片内容")
            ui.Text(c, "卡片内容")
        })
}
```

`ui.Card(c, title)` 开始一张卡片。标题、`Extra` 和 `Tabs` 都为空时省略头部。先链式设置 `Extra`、`Cover`、`Actions` 等选项，最后调用 `Body(fn)` 构建整张卡片，`fn` 里放卡片内容。宽度等布局用 `Layout(layout.W(...))`、`Layout(layout.Grow(1))` 设置。

## 常见用法

以下片段省略了 `import`。

### 尺寸、边框与阴影

`Small()` 使用紧凑的头部和内边距。`Borderless()` 去掉边框，Ant Design 的无边框卡片默认仍有浅阴影。`Shadow(mode)` 接收 `"always"`、`"hover"` 或 `"never"`。`Hoverable()` 让卡片悬停时浮起，并且可以用 `Clicked()` 读取点击。

```go
func cardVariants(c *ui.Context) {
    ui.Row(c, func() {
        ui.Card(c, "小尺寸").Small().Layout(layout.Grow(1)).Body(func() { ui.Text(c, "Small") })
        ui.Card(c, "无边框").Borderless().Shadow("always").Layout(layout.Grow(1)).Body(func() {
            ui.Text(c, "Borderless + Shadow")
        })
        card := ui.Card(c, "可点击").Hoverable().Layout(layout.Grow(1)).Body(func() {
            ui.Text(c, "鼠标移入出现阴影")
        })
        if card.Clicked() {
            showMore = true
        }
    }, layout.Gap(16))
}
```

### 封面、元信息与操作栏

`Cover(fn)` 在头部上方通栏放置封面（通常是图片）；`CardMeta(c, avatar, title, desc)` 是「头像 + 标题 + 描述」块；`Actions(fns...)` 在底部加等宽的操作栏，每个函数构建一格，`CardAction(c, icon, label)` 是带悬停效果的图标操作，`Clicked()` 读取点击。

```go
var likes int

func coverCard(c *ui.Context, cover func()) {
    ui.Card(c, "").Layout(layout.W(300)).
        Cover(cover).
        Actions(
            func() {
                if ui.CardAction(c, icon.Like, "点赞").Clicked() {
                    likes++
                }
            },
            func() { ui.CardAction(c, icon.Edit, "编辑") },
            func() { ui.CardAction(c, icon.More, "更多") },
        ).
        Body(func() {
            ui.CardMeta(c, func() { ui.Avatar(c, "T") }, "Card title", "This is the description")
        })
}
```

### 加载中

`Loading(true)` 用骨架屏代替卡片内容，数据到达后改回 false。

```go
var cardLoading = true

func loadingCard(c *ui.Context) {
    ui.Card(c, "加载中").Loading(cardLoading).Layout(layout.W(300)).Body(func() {
        ui.Text(c, "数据已加载")
    })
}
```

### 内部卡片与页脚

`Inner()` 使用 Ant Design 的内部卡片样式（灰色头部带），适合嵌在另一张卡片里分组；`Footer(fn)` 在内容下方加 Element Plus 风格的页脚；`Padding(geom.Insets)` 覆盖内容区内边距。

```go
func nestedCard(c *ui.Context) {
    ui.Card(c, "外层卡片").Footer(func() { ui.Text(c, "最后更新：今天").Tertiary() }).Body(func() {
        ui.Card(c, "内部卡片").Inner().Small().Extra(func() { ui.Link(c, "More") }).Body(func() {
            ui.Text(c, "Inner card content")
        })
    })
}
```

### 带页签的卡片

`Tabs(&active, items)` 在标题下方加页签栏（对应 antd 的 `tabList` / `activeTabKey`），切换时改写 `active`；`Body` 的内容就是当前页签的面板，按 `active` 构建即可。`TabBarExtra(fn)` 在页签栏末尾放内容，`TabChanged()` 报告本帧切换。没有标题时页签栏就是卡片头部。

```go
var cardTab = "article"

func tabCard(c *ui.Context) {
    tabs := []ui.TabItem{{Key: "article", Label: "文章"}, {Key: "app", Label: "应用"}, {Key: "project", Label: "项目"}}
    ui.Card(c, "带页签的卡片").Tabs(&cardTab, tabs).TabBarExtra(func() { ui.Link(c, "全部") }).Body(func() {
        switch cardTab {
        case "app":
            ui.Text(c, "应用列表内容")
        case "project":
            ui.Text(c, "项目列表内容")
        default:
            ui.Text(c, "文章列表内容")
        }
    })
}
```

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 头部有固定高度与分隔线，标题加粗；默认无阴影，`Hoverable` 悬停浮起。 |
| Element Plus | 对照 `el-card`：标题常规字重，头部与内容内边距为 18/20；默认始终显示阴影（`shadow="always"`）。 |
| shadcn | 对照 shadcn `Card`：头部 p-6（小尺寸 p-4）、无分隔线，内容区紧接头部（`pt-0`）；默认带阴影。 |

组件令牌在 `theme.Components.Card`（`theme.CardToken`）：`Bg`、`HeaderBg`、`InnerHeaderBg`、`HeaderColor`、`HeaderFontSize`、`HeaderFontWeight`、`HeaderHeight`、`HeaderPaddingBlock` / `HeaderPaddingInline`、`BodyPadding`、`ActionsBg`、`BorderColor`、`Radius`、`Shadow`、`HoverShadow`（`SM` 后缀对应小尺寸）。

`ConfigProvider.Defaults("Card", ...)` 支持的键：`"size"`（`"small"` 或 `ui.Small`）、`"bordered"`（bool）、`"variant"`（`"outlined"` / `"borderless"`）、`"hoverable"`（bool）、`"shadow"`（`"always"` / `"hover"` / `"never"`）。

```go
func cardDefaults(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("Card", ui.Props{"size": "small", "shadow": "hover"}).Body(func() {
        ui.Card(c, "全局小尺寸").Body(func() { ui.Text(c, "内容") })
    })
}
```

## 注意事项

- **调用顺序**：`Body` 会立即构建卡片，所有选项都要在它之前设置；`Clicked()`、`TabChanged()` 在它之后读取。即使没有内容也要调用 `Body(nil)`，否则卡片不会出现。
- **状态放在哪**：页签的 `active`、加载标志等放在包级变量或页面闭包里。
- **无障碍**：`Hoverable` 的卡片是可点击的，也会成为一个 Tab 停靠点，聚焦时显示焦点环；`CardAction` 的 `label` 作为图标按钮的无障碍名称，请写清楚动作。
- **RTL**：`Extra` 在 RTL 布局中位于头部左侧，操作栏与元信息的排列同样镜像。

## 示例代码

<!--@include: ./_gen/card-examples.md-->

## API

<!--@include: ./_gen/card-api.md-->

## 相关链接

- 源码：[ui/data_card.go](https://github.com/tangoui/tango/blob/main/ui/data_card.go)
- [ui Go API 参考](/reference/api/ui)
- [List 列表](/components/list)、[Descriptions 描述列表](/components/descriptions)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
