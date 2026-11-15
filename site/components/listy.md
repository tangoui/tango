---
title: Listy 高性能列表
---
# Listy 高性能列表

面向长数据的列表组件，支持可变行高、虚拟滚动、分组吸顶、稳定行键、程序化滚动和加载更多。

## 何时使用

- 长列表的行高由内容决定，或同一列表中有长短不同的描述。
- 需要分组标题吸顶、按键定位或在滚动到底部时追加数据。
- 每行固定高度且只追求最小开销时用 [VirtualList 虚拟列表](/components/virtuallist)；需要列结构、排序筛选时用 [Table 表格](/components/table)。

## 在线体验

示例包含十万条分组数据、粘性分组标题、可变行高以及加载更多。

<GalleryPreview component="Listy" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var users = []string{"Ada", "Linus", "Grace"}

func page(c *ui.Context) {
    ui.Listy(c, len(users), func(i int) {
        ui.Text(c, fmt.Sprintf("%s 的资料", users[i]))
    }).Show()
}
```

`Listy(c, n, item)` 声明 `n` 个条目，`item(i)` 构建第 `i` 行，最后调用 `Show()`。没有 `Height` 时列表按普通列一次构建全部行；加上 `Height` 和 `Virtual()` 后只构建视口附近的行。

## 常见用法

以下片段沿用基本用法里的 `users`，省略了 `import`。

### 虚拟滚动与分组吸顶

`Group` 按首次出现的分组顺序生成标题，`Sticky` 把当前分组标题固定在视口顶部。虚拟滚动必须同时设置 `Height`。

```go
var groupJump bool

func groupedList(c *ui.Context) {
    if ui.Button(c, "跳到 G 组").Clicked() {
        groupJump = true
    }
    list := ui.Listy(c, 100000, func(i int) {
        ui.Text(c, fmt.Sprintf("用户 #%d", i+1))
    }).Group(
        func(i int) string { return string(rune('A' + i%26)) },
        func(key string, count int) { ui.Text(c, fmt.Sprintf("%s · %d 人", key, count)) },
    ).Sticky().Virtual().Height(360)
    if groupJump {
        list.ScrollToGroup("G", ui.AlignTop, 0)
        groupJump = false
    }
    list.Show()
}
```

### 可变行高与稳定行键

默认会在绘制后测量行高；`EstimatedItemHeight` 是还没有测量到的行所使用的估计值。数据会插入、删除或重排时，用 `RowKey` 返回稳定键，避免行内状态跟着索引错位。

```go
type notice struct {
    ID      string
    Title   string
    Details string
}

var notices = []notice{{"a", "短通知", ""}, {"b", "长通知", "这是一段较长的描述，用来演示 Listy 的可变行高。"}}

func variableRows(c *ui.Context) {
    ui.Listy(c, len(notices), func(i int) {
        ui.Text(c, notices[i].Title).Strong()
        if notices[i].Details != "" {
            ui.Paragraph(c, notices[i].Details)
        }
    }).RowKey(func(i int) any { return notices[i].ID }).
        EstimatedItemHeight(52).Virtual().Height(320).Show()
}
```

### 加载更多

`LoadMore(hasMore, loading)` 加入底部状态，滚动到距底部 100 像素以内时 `EndReached()` 返回 `true`。请求完成后增加 `n` 或更新数据，并让 `loading` 回到 `false`。

```go
var (
    itemCount   = 30
    listLoading bool
    listHasMore = true
)

func moreRows(c *ui.Context) {
    list := ui.Listy(c, itemCount, func(i int) {
        ui.Text(c, fmt.Sprintf("通知 %d", i+1))
    }).Virtual().Height(320).LoadMore(listHasMore, listLoading).Show()
    if list.EndReached() && !listLoading && listHasMore {
        listLoading = true
        itemCount += 30 // 实际应用中在请求完成后更新
        listLoading = false
        listHasMore = itemCount < 120
    }
}
```

### 点击与程序化滚动

`Clicked` 返回本次 `Show` 中点击的条目索引；`ScrollToKey` 需要先用 `RowKey` 建立稳定键，`ScrollTo` 则直接接受像素偏移。对分组列表还可以用 `ScrollToGroup`。

```go
var focusUser = "b"

func navigableList(c *ui.Context) {
    list := ui.Listy(c, len(users), func(i int) {
        ui.Text(c, users[i])
    }).RowKey(func(i int) any { return users[i] }).Height(240).Virtual()
    list.ScrollToKey(focusUser, ui.AlignAuto, 0).Show()
    if i, ok := list.Clicked(); ok {
        ui.Text(c, fmt.Sprintf("点击了第 %d 项", i+1)).Tertiary()
    }
}
```

### 数据原地变化

如果切片长度和行键数量不变，但分组或行内容在原地修改，用 `DataVersion` 递增版本号，让列表重新计算分组、偏移和缓存。

```go
var listVersion int

func refreshedList(c *ui.Context) {
    ui.Listy(c, len(users), func(i int) { ui.Text(c, users[i]) }).
        DataVersion(listVersion).Show()
}
```

## 主题与样式

Ant Design 风格默认有行分隔线和悬停背景；Element Plus 使用更紧凑的行视觉；shadcn 保留分隔结构并使用较重的标题字重。`RowHoverable(false)` 和 `Split(false)` 可在实例级关闭悬停和分隔线。

`ConfigProvider.Defaults("Listy", ...)` 支持 `"split"` 与 `"rowHoverable"`；实例上的 `Split`、`RowHoverable` 优先。列表外框、圆角和内边距仍由父布局或主题通用令牌控制。

## 注意事项

- **调用顺序**：`Group`、`Sticky`、`Virtual`、`Height`、`LoadMore` 等配置要在 `Show` 前调用；`Range`、`Clicked`、`EndReached` 在 `Show` 后读取。
- **状态放在哪**：数据、加载状态、版本号和跳转目标放在页面变量或 `ui.State` 中。插入、删除或排序时务必提供稳定 `RowKey`。
- **虚拟化条件**：`Virtual()` 需要 `Height`；没有固定视口时它无法计算可见范围。行高未固定时让组件测量，全部固定时用 `ItemHeight` 提高速度。
- **性能**：`Range` 的返回区间包含分组标题；`EstimatedItemHeight` 过于偏离真实高度会造成初始滚动位置在测量后修正。
- **无障碍**：列表及列表项使用列表语义；行内按钮、链接和输入框仍需可访问名称。分组标题会作为标题语义构建。
- **RTL**：行布局、分隔线和粘性标题遵循当前 RTL 方向。

## 示例代码

<!--@include: ./_gen/listy-examples.md-->

## API

<!--@include: ./_gen/listy-api.md-->

## 相关链接

- 源码：[ui/fill_listy.go](https://github.com/tangoui/tango/blob/main/ui/fill_listy.go)
- [ui Go API 参考](/reference/api/ui)
- [VirtualList 虚拟列表](/components/virtuallist)、[InfiniteScroll 无限滚动](/components/infinitescroll)、[List 列表](/components/list)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
