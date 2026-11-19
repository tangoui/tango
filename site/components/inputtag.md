---
title: InputTag 标签输入
---
# InputTag 标签输入

把用户输入的文本转换为可删除的标签，默认按 Enter 或键入半角、全角逗号后提交。

## 何时使用

- 用户可以输入多个自由格式标签，例如关键词、收件人或技术栈时。
- 需要从固定候选项多选时，用 [MultiSelect 多选](/components/multiselect)。
- 需要一个带搜索按钮的关键字输入时，用 [Input.Search 搜索框](/components/search)。

## 在线体验

<GalleryPreview component="InputTag" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var tags = []string{"Go", "Rust"}

func page(c *ui.Context) {
    ui.InputTag(c, &tags).Placeholder("输入标签并按 Enter").AllowClear().A11yLabel("技术栈")
}
```

`InputTag` 绑定 `*[]string`。输入后按 Enter 或默认逗号分隔符即可追加，标签上的关闭按钮会删除对应值。

## 常见用法

### 限制数量与折叠显示

`MaxTags` 限制实际可以加入的标签数，`MaxTagCount` 只折叠显示，二者可以同时使用。

```go
import "github.com/tangoui/tango/ui"

var topics = []string{"Go", "GUI", "Testing"}

func page(c *ui.Context) {
    ui.InputTag(c, &topics).MaxTags(5).MaxTagCount(2).
        Placeholder("最多 5 个标签").A11yLabel("主题")
}
```

### 自定义分隔符

`Separators` 接收任意多个字符串，空字符串会被忽略。无参调用 `Separators()` 会停用字符分隔符，保留提交键。提交键默认为 Enter，`Trigger("Space")` 可改为空格键。

```go
import "github.com/tangoui/tango/ui"

var paths []string

func page(c *ui.Context) {
    ui.InputTag(c, &paths).Separators(";", " ").
        Placeholder("用分号或空格结束标签").A11yLabel("路径标签")
}
```

### 失焦保存

默认情况下，输入框里尚未按 Enter 的文本仍是编辑中的查询，不会写入绑定切片；`SaveOnBlur` 会在编辑器失去焦点时把非空文本转换为标签。

```go
import "github.com/tangoui/tango/ui"

var recipients []string

func page(c *ui.Context) {
    ui.InputTag(c, &recipients).SaveOnBlur().
        Placeholder("输入收件人，点击其他地方保存").A11yLabel("收件人")
}
```

### 拖动排序与事件

`Draggable` 允许拖动标签重新排序。`Changed()` 报告添加、删除或拖放，`Added()` 返回当前帧新加入的标签。

```go
import "github.com/tangoui/tango/ui"

var (
    ordered []string
    lastAdded string
)

func page(c *ui.Context) {
    w := ui.InputTag(c, &ordered).Draggable().AllowClear().A11yLabel("可排序标签")
    if w.Changed() {
        lastAdded = w.Added()
    }
    ui.Text(c, "本帧添加："+lastAdded)
}
```

### 尺寸、状态与禁用

`Size`、`Status`、`Width` 和 `Disabled` 与其他输入型控件一致；`A11yLabel` 为独立使用的编辑器提供可访问名称。

```go
import "github.com/tangoui/tango/ui"

var statusTags []string

func page(c *ui.Context) {
    ui.InputTag(c, &statusTags).Size(ui.Small).Width(240).A11yLabel("小尺寸标签")
    ui.InputTag(c, &statusTags).Status("error").A11yLabel("错误标签")
    ui.InputTag(c, &statusTags).Disabled(true).A11yLabel("禁用标签")
}
```

## 主题与样式

- InputTag 使用选择器外壳和标签令牌，主题来源是 `theme.Components.Select`（`SelectToken`），其中 `MultipleItemBg`、`MultipleItemColor` 和标签高度控制标签外观。
- `ConfigProvider.Defaults("InputTag", ui.Props{...})` 支持 `size`、`placeholder`、`allowClear`、`maxTagCount` 和 `trigger`（`"Enter"` / `"Space"`）等默认值。显式方法优先。
- Ant Design、Element Plus 和 shadcn 的边框、焦点环、标签颜色会随当前主题切换，不需要在页面里判断主题。

## 注意事项

- **状态**：标签切片放在页面函数外或 `ui.State` 中；组件会复制切片后写回，应用不要在同一帧用旧切片覆盖结果。
- **重复值**：空白标签和已经存在的标签会被忽略；`Added()` 返回空字符串不代表删除事件。
- **键盘**：Enter 默认提交当前输入，`Trigger("Space")` 下改用空格键。输入为空时 Backspace 删除最后一个标签。
- **无障碍**：`A11yLabel` 命名的是文本编辑器；标签删除按钮会以各自的标签值暴露给辅助技术。
- **输入法**：分隔符会处理粘贴文本；需要允许带空格的标签时，不要把空格加入 `Separators`。

## 示例代码

<!--@include: ./_gen/inputtag-examples.md-->

## API

<!--@include: ./_gen/inputtag-api.md-->

## 相关链接

- [源码 ui/entry_inputtag.go](https://github.com/tangoui/tango/blob/main/ui/entry_inputtag.go)、[ui/selector.go](https://github.com/tangoui/tango/blob/main/ui/selector.go)
- [ui Go API](/reference/api/ui#InputTag)
- [MultiSelect 多选](/components/multiselect)、[Input 输入框](/components/input)、[Form 表单](/components/form)
