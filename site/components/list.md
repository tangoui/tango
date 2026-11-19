---
title: List 列表
---
# List 列表

通用的列表容器：逐项渲染文字、头像、描述、操作等内容，支持页头页脚、分页、加载更多和栅格布局。

## 何时使用

- 展示一组结构相近的条目，例如消息、文章、成员、文件，每项可带头像、标题、描述和操作。
- 需要以卡片栅格展示同类内容时（`Grid`）。
- 数据有统一的列结构、需要排序筛选时，用 [Table 表格](/components/table)；上万条以上的长列表用 `VirtualList` 虚拟列表；需要按 Pro 规范从数据自动生成标题、描述、操作的列表，用 `ProList`。

Ant Design 6.7 已将 List 标为弃用，TanGo 保留此组件及其分页、栅格和虚拟滚动接口。

## 在线体验

示例包含头像元信息 + 操作 + 分页、加载更多，以及带边框和页头页脚的列表。

<GalleryPreview component="List" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

type article struct {
    Title, Desc, Author string
}

var articles = []article{
    {"TanGo 自绘 UI 入门", "用纯 Go 画界面，不需要 WebView。", "Ann"},
    {"即时模式与状态", "状态放在你的 Go 变量里，组件每帧读取。", "Bob"},
    {"主题与令牌", "Ant Design、Element Plus、shadcn 三套风格。", "Cara"},
}

func page(c *ui.Context) {
    ui.List(c, len(articles)).Item(func(i int) {
        a := articles[i]
        ui.ListItemMeta(c, func() { ui.Avatar(c, a.Author[:1]) }, a.Title, a.Desc)
        ui.ListActions(c, func() {
            ui.Link(c, "编辑")
            ui.Link(c, "更多")
        })
    })
}
```

`ui.List(c, n)` 声明 n 个条目，先链式设置页头、分页等选项，最后调用 `Item(fn)`：它构建整个列表，并对每个可见条目调用一次 `fn(i)`。条目内容完全由你决定；`ListItemMeta(c, avatar, title, desc)` 是常用的「头像 + 标题 + 描述」块，会占满一行的剩余空间（`avatar` 可以为 nil）；`ListActions` 把操作放在行尾并用竖线分隔。

## 常见用法

以下片段沿用基本用法里的 `articles` 数据，省略了 `import`。

### 页头、页脚与边框

`Header(fn)` / `Footer(fn)` 添加页头页脚，`Bordered()` 加外边框，`Split(false)` 去掉条目之间的分隔线；`Size(ui.Small)` / `Size(ui.Large)` 调整条目内边距。这些选项都要在 `Item` 之前设置。

```go
func borderedList(c *ui.Context) {
    ui.List(c, len(articles)).Bordered().Size(ui.Small).
        Header(func() { ui.Text(c, "最新文章") }).
        Footer(func() { ui.Text(c, "共 3 篇").Tertiary() }).
        Item(func(i int) {
            ui.Text(c, articles[i].Title)
        })
}
```

### 分页

`Pagination(&page, pageSize)` 从第 1 页计数，仅在数据超过一页时显示分页器。`Item` 只构建当前页，回调里的 `i` 仍是全部数据中的下标。`PageChanged()` 报告翻页。`PaginationPosition` 可设为 `"bottom"`（默认）、`"top"` 或 `"both"`，`PaginationAlign` 可设为 `"start"`、`"center"` 或 `"end"`（默认）。

```go
var listPage = 1

func pagedList(c *ui.Context) {
    l := ui.List(c, len(articles)).Pagination(&listPage, 2).Item(func(i int) {
        ui.ListItemMeta(c, nil, articles[i].Title, articles[i].Desc)
    })
    if l.PageChanged() {
        // 服务端分页时在这里请求新的一页
    }
}
```

### 加载更多

`LoadMore(label, busy)` 在有条目时于列表底部居中显示「加载更多」按钮，`busy` 为 true 时按钮显示加载状态，用 `LoadMoreClicked()` 读取点击。`Loading(true)` 在条目区显示加载遮罩，常用于首屏加载。

```go
var (
    shown   = 1
    loading bool
)

func loadMoreList(c *ui.Context) {
    l := ui.List(c, shown).LoadMore("加载更多", loading).Item(func(i int) {
        ui.ListItemMeta(c, nil, articles[i].Title, articles[i].Desc)
    })
    if l.LoadMoreClicked() && shown < len(articles) {
        shown++ // 真实场景：在 goroutine 里请求下一批，返回后再追加
    }
}
```

### 栅格列表

`Grid(columns, gap)` 把条目排成栅格，常和 `Card` 搭配；`GridAt(bp, columns)` 按窗口断点设置列数（对应 antd 按断点设置的 `grid` 对象，如 xs: 1、md: 3），只用 `GridAt` 时默认间距为 16。

```go
func gridList(c *ui.Context) {
    ui.List(c, len(articles)).Grid(1, 16).GridAt(ui.MD, 3).Item(func(i int) {
        ui.Card(c, articles[i].Title).Small().Body(func() {
            ui.Text(c, articles[i].Desc)
        })
    })
}
```

### 长列表：虚拟滚动

`Height(px)` 给条目区一个固定高度，超出部分在区内滚动（页头、页脚、加载更多和分页器留在区外）；再加 `Virtual()`，只构建视口内的条目（上下各多建几项），上万条数据也只付出一屏的代价。没有 `Height` 时 `Virtual()` 不生效，列表照旧逐项构建。

```go
func longList(c *ui.Context, items []article) {
    ui.List(c, len(items)).Height(480).Virtual().
        Item(func(i int) {
            ui.ListItemMeta(c, nil, items[i].Title, items[i].Desc)
        })
}
```

- 条目高度在构建后实测并按 `RowKey(func(i int) any)` 缓存，默认键为下标。自定义键必须可比较，适合用稳定且唯一的 ID。栅格行使用首项的键。条目控件仍需用 `ui.Key` 管理自身状态。
- 未测到的条目按 `EstimatedItemHeight(px)` 估算，默认值为一行文字加条目内边距（Ant Design 中号为 46）。行高一致时可用 `ItemHeight(px)` 固定条目高度并跳过实测。
- 在 `Item` 之前调用 `ScrollToKey(key, ui.AlignTop, offset)`，可在有 `Height` 的列表中滚到当前页内的条目。对齐支持 `AlignTop`、`AlignBottom`、`AlignAuto`，`offset` 以像素计。`Item` 之后调用 `Range()` 读取半开区间 `[first, end)`，虚拟模式下是构建窗口的下标范围，单独保留的焦点项可能在这个区间之外。
- 与 `Pagination` 同用时只在当前页内窗口化；与 `Grid` 同用时按“行”窗口化，一行的高度取该行最高的条目。
- 持有键盘焦点的条目在滚出视口的那一帧仍会构建。焦点控件被裁剪后会失去焦点。离开构建窗口的条目随后从无障碍树、`FindText` 和 Tab 顺序中移除，其内部保留状态会在一段时间后被回收。

### 空状态

条目数为 0 时显示空状态，`EmptyText` 自定义描述文字（默认随语言包显示「暂无数据」）。

```go
func emptyList(c *ui.Context) {
    ui.List(c, 0).EmptyText("还没有文章").Item(func(i int) {})
}
```

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 条目间细分隔线；`Bordered` 外框为大圆角；标题为常规字重。 |
| Element Plus | 外框颜色更浅、圆角更小；标题加粗（500）。 |
| shadcn | 结构同 Ant Design，标题为 `font-medium`（500）。 |

组件令牌在 `theme.Components.List`（`theme.ListToken`）：`HeaderBg`、`FooterBg`、`SplitColor`、`BorderColor`、`Radius`，三档尺寸的 `ItemPaddingBlock` / `ItemPaddingInline`（`SM`、`LG` 后缀），以及 `ListItemMeta` / `ListActions` 用到的 `AvatarMarginInlineEnd`、`TitleColor`、`TitleFontWeight`、`DescriptionColor`、`ActionsMarginInlineStart`、`ActionsSplitColor`。

`ConfigProvider.Defaults("List", ...)` 支持 `size`、`bordered`、`split` 和 `itemLayout`。普通条目的 `ItemLayout("vertical")` 将 `ListActions` 放到主体下方。`ItemExtra(func(i int))` 在行尾添加内容，纵向布局时会为它留出独立一列。

```go
func listDefaults(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("List", ui.Props{"bordered": true, "size": "small"}).Body(func() {
        ui.List(c, len(articles)).Item(func(i int) { ui.Text(c, articles[i].Title) })
    })
}
```

## 注意事项

- **调用顺序**：`Item` 会立即构建列表，所以 `Header`、`Footer`、`Pagination`、`LoadMore`、`Grid` 等选项要在它之前设置；`Layout`、`LoadMoreClicked`、`PageChanged` 在它之后调用。
- **状态放在哪**：页码、已加载条数等由应用持有。条目里有输入框、折叠面板等自带状态的组件，并且列表会插入或删除条目时，在 `Item` 回调里用 `ui.Key(c, 稳定的键, fn)` 包住条目内容，状态才会跟着数据走。
- **性能**：默认形态每帧构建当前页的全部条目，适合几百条以内的数据；更长的列表用分页，或 `Height(px).Virtual()`（见上文“长列表：虚拟滚动”）、`VirtualList`。
- **RTL**：在 RTL 布局中头像、操作区和分隔线位置自动镜像。

## 示例代码

<!--@include: ./_gen/list-examples.md-->

## API

<!--@include: ./_gen/list-api.md-->

## 相关链接

- 源码：[ui/data_list.go](https://github.com/tangoui/tango/blob/main/ui/data_list.go)
- [ui Go API 参考](/reference/api/ui)
- [Card 卡片](/components/card)、[Pagination 分页](/components/pagination)、[Table 表格](/components/table)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
