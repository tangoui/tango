---
title: Mobile Tabs 移动端标签页
---
# Mobile Tabs 移动端标签页

提供带下划线的移动端标签栏，可选择只显示标题，或绑定可滑动的内容面板。

## 何时使用

- 同一页面有少量平级内容分区时使用 `Tabs`。
- 需要底部主导航且带图标时使用 `TabBar`，不要用 Tabs 代替它。
- 标签超过一行或内容不是平级分区时，应改用导航列表或筛选器。
- 需要左右手势切换内容时，在 Tabs 上调用 `Swipeable` 并提供 `Body`。

## 在线体验

真实 gallery 展示顶部 Tabs、多个标题和底部 TabBar 组合，可切换选中项。

<GalleryPreview component="mobile.Tabs" />

## 基本用法

active 指向当前下标；`Header` 只构建标签栏，不构建内容面板。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/mobile"
)

var active int

func page(c *ui.Context) {
    mobile.Tabs(c, []string{"推荐", "关注", "热门"}, &active).Header()
    ui.Text(c, "当前："+[]string{"推荐", "关注", "热门"}[active])
}
```

## 常见用法

### 标签栏和面板

`Body` 的参数是当前面板下标；按下标构建内容即可。

```go
func panes(c *ui.Context) {
    mobile.Tabs(c, []string{"概览", "明细"}, &active).Body(func(i int) {
        if i == 0 {
            ui.Text(c, "概览内容")
            return
        }
        ui.Text(c, "明细内容")
    })
}
```

### 手势切换

`Swipeable` 允许在面板区域横向滑动切换，仍由 active 指针保存选择。

```go
func swipeTabs(c *ui.Context) {
    mobile.Tabs(c, []string{"图片", "评论", "详情"}, &active).
        Swipeable().
        Body(func(i int) { ui.Text(c, "面板 "+string(rune('A'+i))) })
}
```

### 监听切换

`Changed` 只在本帧选中项变化时为真，适合更新筛选条件或触发数据加载。

```go
func changedTabs(c *ui.Context) {
    tabs := mobile.Tabs(c, []string{"全部", "待处理"}, &active).Header()
    if tabs.Changed() {
        // 根据 active 加载对应数据
    }
}
```

### 底部 TabBar

主导航使用 `TabItem` 的稳定 Key 和标题，可选图标与 Badge。

```go
import "github.com/tangoui/tango/ui/icon"

var tab = "home"

func bottomNav(c *ui.Context) {
    mobile.TabBar(c, []mobile.TabItem{
        {Key: "home", Title: "首页", Icon: icon.Home},
        {Key: "msg", Title: "消息", Icon: icon.Message, Badge: "5"},
    }, &tab)
}
```

## 主题与样式

Tabs 的下划线、激活色和间距读取移动主题；shadcn 风格使用更明显的分段激活状态。底部 TabBar 的图标和 Badge 也跟随 Provider 的令牌。不要用颜色字符串硬编码激活态，必要时读取 `mobile.ThemeOf(c)`。

## 注意事项

active 指针必须稳定且范围有效；动态增删标签时同步修正下标。`Changed` 是逐帧事件，不要把它当作持久选择。标签标题应简短并可被屏幕阅读器理解，面板内容要保留足够点击和滑动空间。RTL 下标签顺序和滑动方向由 ui 方向作用域处理。

## 示例代码

<!--@include: ./_gen/mobile-tabs-examples.md-->

## API

<!--@include: ./_gen/mobile-tabs-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/tabs.go)
- [NavBar 导航栏](/components/mobile-navbar)
- [Button 按钮](/components/mobile-button)
- [ui mobile Go API](/reference/api/ui-mobile)
