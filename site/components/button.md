---
title: Button 按钮
---
# Button 按钮

触发一个明确动作，例如保存表单、提交查询、打开对话框。Primary 用于主要操作，Danger 表示具有破坏性的动作。

## 在线体验

点击预览按钮，观察计数变化；Disabled 按钮不会改变状态。 工具栏支持 Ant Design、Element Plus、shadcn 以及深浅色切换；重置会恢复初始状态。

<ComponentPreview component="button" title="Button 按钮" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "fmt"
)

var clicks int

func page(c *ui.Context) {
    if ui.Button(c, "Click me").Primary().Clicked() {
        clicks++
    }
    ui.Text(c, fmt.Sprintf("Clicks: %d", clicks))
}
```

## 常用参数与方法

| 参数 / 方法 | 说明 |
| --- | --- |
| `Primary()` | 主要操作的强调样式 |
| `Danger()` | 危险操作样式，可与 Primary 组合 |
| `Disabled(bool)` | 禁用交互 |
| `Loading(bool)` | 显示加载状态并阻止重复点击 |
| `Small() / Large()` | 设置控件尺寸 |
| `Clicked() bool` | 读取当前帧是否发生点击 |

## 使用说明

使用 Tab 聚焦后，Enter 或空格可以触发按钮。异步操作开始后可使用 Loading(true) 阻止重复提交。

## 更多场景

官方 gallery 里这个组件的全部示例（基础、进阶与组合场景），可切换场景、主题和深浅色。

<GalleryPreview component="Button" />

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/button.go)
- [组件截图目录](/components/screenshots)
- [组件总览](/components/)
- [ui Go API](/reference/api/ui)
