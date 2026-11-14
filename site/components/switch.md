---
title: Switch 开关
---
# Switch 开关

表示立即生效的开/关状态，例如启用通知。状态由 bool 变量持有。

## 在线体验

点击开关，或聚焦画布后按空格，查看画布下方的状态反馈。 工具栏支持 Ant Design、Element Plus、shadcn 以及深浅色切换；重置会恢复初始状态。

<ComponentPreview component="switch" title="Switch 开关" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var enabled bool

func page(c *ui.Context) {
    if ui.Switch(c, &enabled).Changed() {
        // enabled 已经更新，可在这里处理业务逻辑。
    }
}
```

## 常用参数与方法

| 参数 / 方法 | 说明 |
| --- | --- |
| `val *bool` | 应用持有的开关值 |
| `Texts(on, off string)` | 设置开关内的状态文字 |
| `Small()` | 紧凑尺寸 |
| `Loading(bool)` | 加载状态 |
| `Disabled(bool)` | 禁用开关 |
| `Changed() bool` | 当前帧是否发生状态切换 |

## 使用说明

开关适合修改设置；需要用户确认提交的多项选中状态，通常使用 Checkbox。

## 更多场景

官方 gallery 里这个组件的全部示例（基础、进阶与组合场景），可切换场景、主题和深浅色。

<GalleryPreview component="Switch" />

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/choice.go)
- [组件截图目录](/components/screenshots)
- [组件总览](/components/)
- [ui Go API](/reference/api/ui)
