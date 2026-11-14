---
title: Pagination 分页
---
# Pagination 分页

把长列表分成多页，每次只加载和显示一页，并提供翻页、跳页和切换每页条数的入口。

## 何时使用

- 数据量较大、一次加载或显示会很慢时，按页加载。
- 需要让用户快速定位到某一页，或调整每页显示条数时。
- 表格和列表一般不需要单独放分页器：[Table](/components/table) 的 `Pagination` / `ServerPagination`、[List](/components/list) 的 `Pagination` 已内置分页；[ProTable](/components/protable) 也自带分页。
- 数据需要连续浏览（消息流、动态）时，用列表的「加载更多」或 `InfiniteScroll` 无限滚动代替分页。

## 在线体验

切换场景可以看到基础分页、总数与每页条数、小尺寸、Element 风格的自定义布局，以及跳页、简洁模式、背景色、禁用和单页隐藏。

<GalleryPreview component="Pagination" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var current = 1

func page(c *ui.Context) {
    ui.Pagination(c, &current, 500, 10).Show()
    ui.Text(c, fmt.Sprintf("当前第 %d 页", current))
}
```

`ui.Pagination(c, &page, total, pageSize)` 绑定当前页（从 1 开始），`total` 是数据总条数，`pageSize` 是每页条数，链式设置后调用 `Show()`。用户点击页码时分页器直接改写 `*page`，同一帧 `Changed()` 返回 true，可以在这里触发加载。

## 常见用法

以下片段省略了 `import`。

### 总数与每页条数

`ShowTotal()` 显示「共 N 条」；`SizeChanger(&size)` 显示每页条数选择器（10 / 20 / 50 / 100），绑定的变量同时作为 `pageSize` 传入。

```go
var listPage, listSize = 1, 20

func sizedPager(c *ui.Context) {
    p := ui.Pagination(c, &listPage, 85, listSize).ShowTotal().SizeChanger(&listSize).Show()
    if p.Changed() {
        // 按 listPage / listSize 重新加载数据
    }
}
```

### 跳页

`ShowQuickJumper()` 增加「跳至 [ ] 页」输入框，回车或离开输入框后跳转。

```go
var jumpPage = 1

func jumperPager(c *ui.Context) {
    ui.Pagination(c, &jumpPage, 500, 10).ShowTotal().ShowQuickJumper().Show()
}
```

### 简洁模式与小尺寸

`Simple()` 只保留上一页、可编辑的当前页、「/ 总页数」和下一页；再加 `ReadOnly()` 时当前页显示为文字，是最窄的分页器，适合放在列表或穿梭框底部。`Small()`（等价于 `Size(ui.Small)`）使用小尺寸。

```go
var simplePage = 2

func compactPagers(c *ui.Context) {
    ui.Pagination(c, &simplePage, 50, 10).Simple().Show()
    ui.Pagination(c, &simplePage, 50, 10).Simple().ReadOnly().Small().Show()
    ui.Pagination(c, &simplePage, 50, 10).Small().Show()
}
```

### 背景色、禁用与单页隐藏

`Background(true)` 使用带底色的页码按钮（Element Plus 的 `background`，Element 主题下默认开启，`Background(false)` 关闭）；`Disabled(true)` 禁用全部交互；`HideOnSinglePage()` 在只有一页时不显示分页器。

```go
var bgPage = 1

func styledPagers(c *ui.Context, loading bool) {
    ui.Pagination(c, &bgPage, 100, 10).Background(true).Disabled(loading).Show()
    ui.Pagination(c, &bgPage, 5, 10).HideOnSinglePage().Show() // 只有 1 页，不显示
}
```

### 自定义布局与页码数量

`Layout(parts...)` 按给定顺序排列分页器的各部分（对应 Element Plus 的 `layout`），只显示列出的部分：`PaginationTotal`、`PaginationPrev`、`PaginationPager`、`PaginationNext`、`PaginationSizes`（需要同时调用 `SizeChanger`）、`PaginationJumper`，以及把后面部分推到行尾的 `PaginationSpacer`。`PagerCount(n)` 设置最多显示多少个页码按钮（5–21 的奇数）。

```go
var layoutPage, layoutSize = 1, 10

func customLayout(c *ui.Context) {
    ui.Pagination(c, &layoutPage, 400, layoutSize).
        SizeChanger(&layoutSize).
        PagerCount(5).
        Layout(ui.PaginationPrev, ui.PaginationPager, ui.PaginationNext,
            ui.PaginationSpacer, ui.PaginationSizes, ui.PaginationTotal).
        Show()
}
```

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 当前页为描边按钮，其他页码无底色；`Background` 默认关闭。 |
| Element Plus | 默认 `Background(true)`，页码为填充方块，当前页主色填充。 |
| shadcn | 页码是幽灵按钮，当前页为描边按钮；上一页 / 下一页按钮在图标旁带文字（小尺寸和简洁模式只显示图标）。 |

组件令牌在 `theme.Components.Pagination`（`theme.PaginationToken`）：`ItemSize` / `ItemSizeSM`（按钮尺寸）、`ItemBg`、`ItemColor`、`ItemHoverBg`、`ItemActiveBg`、`ItemActiveColor`、`ItemActiveBorderColor`，以及背景模式的 `ItemFilledBg`、`ItemFilledColor`、`ItemFilledActiveBg`、`ItemFilledActiveColor`。

`ConfigProvider.Defaults("Pagination", ...)` 支持的键：`"showQuickJumper"`、`"showTotal"`、`"simple"`、`"hideOnSinglePage"`、`"background"`、`"disabled"`（均为 bool），`"size"`（`ui.ControlSize` 或 `"small"` / `"default"`）。

```go
func pagerDefaults(c *ui.Context) {
    ui.ConfigProvider(c).
        Defaults("Pagination", ui.Props{"showTotal": true, "size": "small"}).
        Body(func() {
            ui.Pagination(c, &current, 200, 10).Show()
        })
}
```

## 注意事项

- **状态放在哪**：当前页和每页条数由应用持有（包级变量或页面闭包），分页器只读写绑定的指针。`total` 变小后当前页超出范围时，分页器会把 `*page` 收回到最后一页。
- **触发加载**：在 `Changed()` 为 true 的那一帧发起数据请求；请求放在 goroutine 中，结果写回 UI 线程后重绘，见 [后台数据与线程](/guide/ui#后台数据与线程)。
- **文案与语言**：「共 N 条」「条/页」「跳至」等文字随 `ConfigProvider.Locale` 切换。
- **RTL**：在 RTL 布局中分页器整体镜像，上一页 / 下一页的箭头方向随之翻转。

## 示例代码

<!--@include: ./_gen/pagination-examples.md-->

## API

<!--@include: ./_gen/pagination-api.md-->

## 相关链接

- 源码：[ui/nav.go](https://github.com/tangoui/tango/blob/main/ui/nav.go)
- [ui Go API 参考](/reference/api/ui)
- [Table 表格](/components/table)、[List 列表](/components/list)、[ProTable 高级表格](/components/protable)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
