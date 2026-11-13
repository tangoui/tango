---
title: Input 输入框
---
# Input 输入框

编辑单行文本。文本由应用中的 string 变量持有，Input 接收指针并在编辑时更新它。

## 在线体验

点击输入框并输入文字，试试 Backspace、方向键、全选和清空按钮。 工具栏支持 Ant Design、Element Plus、shadcn 以及深浅色切换；重置会恢复初始状态。

<ComponentPreview component="input" title="Input 输入框" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var name string

func page(c *ui.Context) {
    ui.Input(c, &name).
        Placeholder("Type here...").
        AllowClear()
    ui.Text(c, "Value: " + name)
}
```

## 常用参数与方法

| 参数 / 方法 | 说明 |
| --- | --- |
| `Placeholder(string)` | 空值时的占位提示 |
| `AllowClear()` | 显示清空按钮 |
| `MaxLength(int)` | 限制输入长度 |
| `ReadOnly()` | 允许查看但不可编辑 |
| `Disabled(bool)` | 禁用输入框 |
| `Changed() bool` | 读取当前帧是否发生内容修改 |

## 使用说明

值必须保存在页面函数外，或通过 ui.State 持有，避免每帧重置。网页输入通过浏览器输入法桥接，支持组合输入；预览已内置中文演示字形，更多字符可以导入本地字体。

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/input.go)
- [完整组件截图目录](/guide/components)
- [GUI 组件在线体验总览](/components/)
- [ui Go API](/reference/api/ui)
