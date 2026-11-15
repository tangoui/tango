---
title: Checkbox 全选 / 半选
---
# Checkbox 全选 / 半选

在多选框组上方提供全选和半选状态，并把选择结果绑定到一个字符串切片。

## 何时使用

- 需要对一组多选项提供“全选 / 全不选”操作时。
- 选项较少、需要同时看到每个选项时；选项很多时改用 [MultiSelect 多选](/components/multiselect)。
- 只需要一个布尔开关时，用 [Checkbox 多选框](/components/checkbox) 或 [Switch 开关](/components/switch)。

## 在线体验

<GalleryPreview component="CheckAll" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var fruits = []string{"苹果"}

func page(c *ui.Context) {
    ui.CheckAll(c, &fruits, []ui.Option{
        {Label: "苹果", Value: "苹果"},
        {Label: "香蕉", Value: "香蕉"},
        {Label: "橙子", Value: "橙子"},
    })
}
```

`CheckAll` 会在组上方绘制全选框，部分选中时自动显示半选状态。`selected` 保存的是选项的 `Value`，而不是显示标签。

## 常见用法

### 监听选择变化

返回值的 `Changed()` 只报告当前帧的选择变化；最终选择内容始终读取绑定的 `[]string`。

```go
import (
    "strings"

    "github.com/tangoui/tango/ui"
)

var selectedFiles []string

func page(c *ui.Context) {
    w := ui.CheckAll(c, &selectedFiles, ui.Opts("报告.pdf", "数据.csv", "图片.png"))
    if w.Changed() {
        ui.Text(c, "已选："+strings.Join(selectedFiles, "、"))
    }
}
```

### 禁用部分选项

`Option.Disabled` 的选项不会参与全选和半选计算，也不会被全选操作改变；如果它原本已在 `selected` 中，会继续保留。

```go
import "github.com/tangoui/tango/ui"

var permissions = []string{"read", "audit"}

func page(c *ui.Context) {
    ui.CheckAll(c, &permissions, []ui.Option{
        {Label: "读取", Value: "read"},
        {Label: "写入", Value: "write"},
        {Label: "审计（只读）", Value: "audit", Disabled: true},
    })
}
```

### 使用独立选项定义

选项可以使用不同的 `Label` 和 `Value`，也可以从字符串快速生成相同的标签和值。把选项切片放在页面函数外可避免每帧重新分配。

```go
import "github.com/tangoui/tango/ui"

var (
    selectedLanguages []string
    languageOptions = []ui.Option{
        {Label: "Go 语言", Value: "go"},
        {Label: "Rust 语言", Value: "rust"},
        {Label: "Swift（暂不可用）", Value: "swift", Disabled: true},
    }
)

func page(c *ui.Context) {
    ui.CheckAll(c, &selectedLanguages, languageOptions)
}
```

### 放进表单

`CheckAll` 的返回对象提供 `Changed()`，表单校验应针对绑定的 `[]string`；需要至少选择一项时，在对应 `FormItem` 上配置规则，而不是只检查全选框。

```go
import "github.com/tangoui/tango/ui"

var choices []string

func page(c *ui.Context) {
    f := ui.Form(c)
    f.Body(func() {
        f.Item("可见范围").Body(func() {
            ui.CheckAll(c, &choices, ui.Opts("团队", "组织", "公开"))
        })
    })
}
```

## 主题与样式

- 全选框和组内复选框都使用 `theme.Components.Checkbox`（`CheckboxToken`）；半选状态由 Checkbox 的 `Indeterminate` 视觉令牌呈现。
- `ConfigProvider.Defaults("Checkbox", ui.Props{"size": ...})` 可统一调整组内复选框尺寸；具体标签布局仍由 `CheckAll` 自己管理。
- 三套风格都保留相同的全选、半选和禁用语义，颜色和焦点环随主题令牌变化。

## 注意事项

- **状态**：`selected` 是唯一的选择来源；全选框本身没有单独的 `bool` 变量。把切片放在页面函数外或 `ui.State` 中。
- **禁用项**：全选只操作未禁用选项，未知值和禁用项会保留；不要在业务代码中用“选项总数”简单覆盖切片。
- **无障碍**：Tab 聚焦，空格切换；给没有清晰上下文的 `CheckAll` 放进带标签的 `FormItem`，或通过外围表单标签说明选择范围。
- **数量**：选项数量很大时，逐项复选框会构建大量节点，应改用多选选择器或 Transfer。
- **事件**：`Changed()` 是本帧事件，批量保存时读取并复制 `selected`，不要保存返回对象本身。

## 示例代码

<!--@include: ./_gen/checkall-examples.md-->

## API

<!--@include: ./_gen/checkall-api.md-->

## 相关链接

- [源码 ui/entry_checkall.go](https://github.com/tangoui/tango/blob/main/ui/entry_checkall.go)、[ui/choice.go](https://github.com/tangoui/tango/blob/main/ui/choice.go)
- [ui Go API](/reference/api/ui#CheckAll)
- [Checkbox 多选框](/components/checkbox)、[MultiSelect 多选](/components/multiselect)、[Form 表单](/components/form)
