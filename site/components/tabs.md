---
title: Tabs 标签页
---
# Tabs 标签页

在同一工作区切换内容面板，例如概览、事件、设置。用稳定的 Key 标识标签，用 Label 显示标题。

## 在线体验

点击不同标签查看面板变化；聚焦后可以用方向键切换。 工具栏支持 Ant Design、Element Plus、shadcn 以及深浅色切换；重置会恢复初始状态。

<ComponentPreview component="tabs" title="Tabs 标签页" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var active = "overview"

func page(c *ui.Context) {
    ui.Tabs(c, &active, []ui.TabItem{
        {Key: "overview", Label: "Overview"},
        {Key: "events", Label: "Events"},
        {Key: "settings", Label: "Settings"},
    }).Show()
    switch active {
    case "events":
        ui.Text(c, "Calls, events and streams")
    default:
        ui.Text(c, "Build your interface with Tango")
    }
}
```

## 常用参数与方法

| 参数 / 方法 | 说明 |
| --- | --- |
| `active *string` | 当前标签的 Key |
| `items []ui.TabItem` | 标签项列表 |
| `Card()` | 卡片式标签 |
| `Placement(ui.Placement)` | 标签栏的位置 |
| `Closable()` | 允许关闭标签 |
| `Show()` | 完成标签栏构建 |
| `Changed() bool` | 读取当前帧是否切换标签 |

## 使用说明

本文示例根据 active 渲染内容。也可以使用 Panel 回调组织面板；可关闭标签的应用需要维护自己的标签列表。

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav.go)
- [完整组件截图目录](/guide/components)
- [GUI 组件在线体验总览](/components/)
- [ui Go API](/reference/api/ui)
