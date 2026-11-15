---
title: Checkbox 多选框
---
# Checkbox 多选框

在一组可选项中进行多项选择，或单独用于两种状态之间的切换（例如“同意协议”）。包含单个多选框 `Checkbox`、多选框组 `CheckboxGroup` 和带“全选”的 `CheckAll`。

## 何时使用

- 在一组选项中选择多个值，例如兴趣、标签、权限。
- 单独使用表示勾选或不勾选，通常需要用户再点击“提交”才生效。
- 需要切换后立即生效的开关（例如“开启通知”），用 [Switch 开关](/components/switch)。
- 只能选一个值时用 [Radio 单选框](/components/radio)；选项很多时用多选模式的 [Select 选择器](/components/select)。

## 在线体验

<GalleryPreview component="Checkbox" />

<GalleryPreview component="CheckAll" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var agree bool

func page(c *ui.Context) {
	ui.Checkbox(c, "我已阅读并同意用户协议", &agree)
	ui.Button(c, "注册").Primary().Disabled(!agree)
}
```

`Checkbox` 绑定一个 `*bool`，点击标签或方框都会切换；`Changed()` 报告本帧是否发生切换。

## 常见用法

### 多选框组

`CheckboxGroup` 把选中项的 `Value` 绑定到 `*[]string`。选项用 `[]ui.Option` 描述（`Label` 显示、`Value` 存储、`Disabled` 单项禁用），`ui.Opts` 可以从字符串快速生成值与标签相同的选项。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var langs = []string{"go"}

func page(c *ui.Context) {
	opts := []ui.Option{
		{Label: "Go", Value: "go"},
		{Label: "Rust", Value: "rust"},
		{Label: "TypeScript", Value: "ts"},
		{Label: "Swift（暂不支持）", Value: "swift", Disabled: true},
	}
	if ui.CheckboxGroup(c, &langs, opts).A11yLabel("语言").Changed() {
		ui.Message(c, "info", "已选："+strings.Join(langs, ", "))
	}
}
```

### 全选

`CheckAll` 在多选框组上方加一个“全选”多选框：部分选中时显示半选状态，点击时全选或全不选；被禁用的选项保持原状态。

```go
import (
	"github.com/tangoui/tango/ui"
)

var fruits = []string{"苹果"}

func page(c *ui.Context) {
	ui.CheckAll(c, &fruits, []ui.Option{
		{Label: "苹果", Value: "苹果"},
		{Label: "香蕉", Value: "香蕉"},
		{Label: "橙子", Value: "橙子"},
		{Label: "葡萄（禁用）", Value: "葡萄", Disabled: true},
	})
}
```

需要自己摆放“全选”时，用单个 `Checkbox` 加 `Indeterminate` 自行实现：

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	all     = ui.Opts("北京", "上海", "广州")
	checked []string
)

func page(c *ui.Context) {
	full := len(checked) == len(all)
	if ui.Checkbox(c, "全部城市", &full).Indeterminate(len(checked) > 0 && !full).Changed() {
		checked = checked[:0]
		if full {
			for _, o := range all {
				checked = append(checked, o.Value)
			}
		}
	}
	ui.CheckboxGroup(c, &checked, all).A11yLabel("城市")
}
```

### 选择数量限制

`Min(n)` 保证至少选中 n 项（只剩 n 项时无法再取消），`Max(n)` 最多选 n 项（达到上限后其余选项变为禁用）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var picks = []string{"Go"}

func page(c *ui.Context) {
	ui.CheckboxGroup(c, &picks, ui.Opts("Go", "Rust", "Zig", "C")).Min(1).Max(2).A11yLabel("语言（1–2 项）")
}
```

### 边框与按钮样式

`Border()` 给多选框加边框（Element Plus `el-checkbox border`），`Buttons()` 把选项组渲染成相连的切换按钮（`el-checkbox-button`）。`Size` 对两种样式都生效。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	bordered bool
	cities   = []string{"上海"}
)

func page(c *ui.Context) {
	ui.Checkbox(c, "带边框", &bordered).Border()
	ui.CheckboxGroup(c, &cities, ui.Opts("北京", "上海", "广州", "深圳")).Buttons().A11yLabel("城市")
	ui.CheckboxGroup(c, &cities, ui.Opts("北京", "上海", "广州")).Border().Size(ui.Small).A11yLabel("城市（小号）")
}
```

## 主题与样式

- Ant Design 与 shadcn 下默认是方框加文字；`Border()`、`Buttons()` 对应 Element Plus 的变体，在任何风格下都可使用，配色跟随当前主题。
- 组件令牌：`theme.Components.Checkbox`（`CheckboxToken`）。
- `ConfigProvider.Defaults("Checkbox", ui.Props{"size": ...})` 统一设置 `Checkbox` 与 `CheckboxGroup` 的尺寸。

## 注意事项

- **状态**：勾选状态就是你绑定的 `bool` / `[]string`，组件不另存副本；在代码中修改它们，下一帧界面就会更新。
- **表单校验**：放进 [Form](/components/form) 时，单个多选框用 `ui.RulesOf(item, &agree, ui.MustBeTrue("..."))`，多选框组用 `ui.RulesOf` 配合 `ui.RequiredItems[string]`、`MinItems`、`MaxItems` 规则；在出错的表单项里方框自动显示错误色，也可以用 `Status("error")` 手动设置。
- **无障碍**：Tab 聚焦，空格切换。没有可见标签的多选框（例如表格行选择）用 `A11yLabel` 设置名称；多选框组用 `A11yLabel` 命名整组（在表单项里默认使用表单项标签）。
- **大量选项**：一屏放不下的选项请改用多选 Select 或 [Transfer 穿梭框](/components/transfer)。

## 示例代码

<!--@include: ./_gen/checkbox-examples.md-->

## API

<!--@include: ./_gen/checkbox-api.md-->

`CheckAll` 的签名见 [Go API 参考](/reference/api/ui#CheckAll)：`func CheckAll(c *Context, selected *[]string, opts []Option) *CheckAllW`，返回值的 `Changed()` 报告选择变化。

## 相关链接

- [源码 ui/choice.go](https://github.com/tangoui/tango/blob/main/ui/choice.go)
- [ui Go API](/reference/api/ui)
- [Radio 单选框](/components/radio)、[Switch 开关](/components/switch)、[Select 选择器](/components/select)、[Form 表单](/components/form)
