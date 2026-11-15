---
title: Mobile IndexBar 索引栏
---
# Mobile IndexBar 索引栏

为按字母或分类分组的长列表提供侧边索引，支持点按或拖动跳转并固定当前分组标题。

## 何时使用

- 通讯录、城市选择和按首字母分组的长列表适合使用 `IndexBar`。
- 分组数量少或需要搜索时，优先使用普通 List 加 SearchBar。
- 不要把 IndexBar 用于分页导航或平级内容切换，分别使用 Tabs 或 TabBar。
- 每个索引应对应一个可展示的分组，空分组应在业务层过滤。

## 在线体验

真实 gallery 展示城市分组、右侧索引和可点击 Cell，可拖动索引快速定位。

<GalleryPreview component="mobile.IndexBar" />

## 基本用法

`indexes` 的顺序决定分组顺序；Body 回调收到当前分组的下标。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/ui/mobile"
)

var groups = []string{"A", "B", "C"}

func page(c *ui.Context) {
    mobile.IndexBar(c, groups).Layout(layout.Grow(1)).Body(func(i int) {
        ui.Text(c, groups[i])
        mobile.Cell(c, "分组 "+groups[i]).Clickable()
    })
}
```

## 常见用法

### 城市分组

用 map 保存分组内容，Body 中用索引下标取值；动态行用 `ui.Key` 保持身份。

```go
var cities = map[string][]string{
    "A": {"安庆", "鞍山", "安阳"},
    "B": {"北京", "保定"},
}

func cityList(c *ui.Context) {
    mobile.IndexBar(c, groups).Layout(layout.Grow(1)).Body(func(i int) {
        for _, name := range cities[groups[i]] {
            name := name
            ui.Key(c, name, func() { mobile.Cell(c, name).Clickable() })
        }
    })
}
```

### 点击行

IndexBar 负责索引跳转，Cell 负责行交互；两者的事件可以在同一帧处理。

```go
func clickableGroups(c *ui.Context) {
    mobile.IndexBar(c, []string{"A", "B"}).Body(func(i int) {
        cell := mobile.Cell(c, "条目").Clickable()
        if cell.Clicked() {
            ui.Text(c, "选择了第 "+fmt.Sprint(i)+" 组")
        }
    })
}
```

### 配合安全区和浮动操作

底部安全区避免最后一组被系统手势条遮挡；FloatingBubble 可放置辅助操作。

```go
import "github.com/tangoui/tango/ui/icon"

func safeIndex(c *ui.Context) {
    mobile.IndexBar(c, []string{"A", "B"}).Layout(layout.Grow(1)).Body(func(i int) {
        mobile.Cell(c, "分组 "+groups[i])
    })
    mobile.SafeArea(c, mobile.Bottom)
    mobile.FloatingBubble(c, icon.Message, mobile.AxisBoth, true).A11yLabel("客服")
}
```

### 自定义布局

`Layout` 可给索引栏分配剩余高度或固定高度；索引本身不替代外层页面布局。

```go
func fixedIndex(c *ui.Context) {
    mobile.IndexBar(c, []string{"A", "B", "C"}).Layout(layout.H(420)).Body(func(i int) {
        mobile.Cell(c, fmt.Sprintf("第 %d 组", i+1))
    })
}
```

## 注意事项

索引切片和分组数据应保持稳定顺序，回调下标只在当前构建中有效。长列表内容放在稳定的页面状态中，动态行使用 `ui.Key`。底部安全区由宿主或 Provider 提供时不要重复计算；为浮动按钮设置 `A11yLabel`。RTL 会把索引移到另一侧，业务不要写死右侧坐标。

## 示例代码

<!--@include: ./_gen/mobile-indexbar-examples.md-->

## API

<!--@include: ./_gen/mobile-indexbar-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/indexbar.go)
- [主题与安全区源码](https://github.com/tangoui/tango/blob/main/ui/mobile/theme.go)
- [List 列表](/components/mobile-list)
- [ui mobile Go API](/reference/api/ui-mobile)
