---
title: Radio 单选框
---
# Radio 单选框

在一组互斥的选项中选择一个。包含单个单选框 `Radio` 和单选框组 `RadioGroup`，组还支持按钮样式。

## 何时使用

- 选项不多（一般不超过 5 个）、需要把所有选项平铺展示以便比较时。
- 只能选一个值；需要多选时用 [Checkbox 多选框](/components/checkbox)。
- 选项较多或空间有限时用 [Select 选择器](/components/select)；作为视图切换（列表 / 看板）时用 [Segmented 分段控制器](/components/segmented)。

## 在线体验

<GalleryPreview component="Radio" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var plan = "A"

func page(c *ui.Context) {
	ui.RadioGroup(c, &plan, []ui.Option{
		{Label: "选项 A", Value: "A"},
		{Label: "选项 B", Value: "B"},
		{Label: "选项 C", Value: "C"},
		{Label: "禁用", Value: "D", Disabled: true},
	}).A11yLabel("方案")
	ui.Text(c, "当前："+plan)
}
```

`RadioGroup` 把选中项的 `Value` 绑定到 `*string`；`Changed()` 报告本帧是否切换了选项。`ui.Opts("北京", "上海")` 可以快速生成值与标签相同的选项。

## 常见用法

### 单个单选框

`Radio(c, label, checked)` 只负责显示和报告点击：传入当前是否选中，点击后由你更新状态。适合选项需要自定义排版（例如每项下方带说明）的场景。

```go
import (
	"github.com/tangoui/tango/ui"
)

var tier = "free"

func page(c *ui.Context) {
	for _, p := range []struct{ v, label, desc string }{
		{"free", "免费版", "适合个人试用"},
		{"pro", "专业版", "适合小团队"},
	} {
		if ui.Radio(c, p.label, tier == p.v).Clicked() {
			tier = p.v
		}
		ui.Text(c, p.desc).Secondary()
	}
}
```

### 按钮样式

`Buttons()` 把单选框组渲染成相连的按钮；`Solid()` 让选中按钮用主色填充（隐含 `Buttons()`）。`Size` 对按钮和普通单选框都生效。

```go
import (
	"github.com/tangoui/tango/ui"
)

var city = "上海"

func page(c *ui.Context) {
	ui.RadioGroup(c, &city, ui.Opts("北京", "上海", "广州", "深圳")).Buttons().A11yLabel("城市")
	ui.RadioGroup(c, &city, ui.Opts("北京", "上海", "广州")).Solid().Size(ui.Small).A11yLabel("城市（实心）")
}
```

### 边框与禁用

`Border()` 给每个单选框加边框（Element Plus `el-radio border`，对按钮样式无效）；`Disabled(true)` 禁用整组，`Option.Disabled` 禁用单项。

```go
import (
	"github.com/tangoui/tango/ui"
)

var size = "M"

func page(c *ui.Context) {
	ui.RadioGroup(c, &size, ui.Opts("S", "M", "L")).Border().A11yLabel("尺码")
	ui.RadioGroup(c, &size, ui.Opts("S", "M", "L")).Buttons().Disabled(true).A11yLabel("尺码（禁用）")
}
```

## 主题与样式

- Ant Design / shadcn 下默认为圆点加文字；按钮样式对应 antd `optionType="button"`，`Solid` 对应 `buttonStyle="solid"`；`Border` 对应 Element Plus 的带边框单选框。
- 组件令牌：`theme.Components.Radio`（`RadioToken`）。
- `ConfigProvider.Defaults("Radio", ui.Props{...})` 支持 `size`、`optionType`（`"button"`）、`buttonStyle`（`"solid"`），可以把一个范围内的单选框组统一切换成按钮样式。

## 注意事项

- **状态**：`RadioGroup` 的值就是你绑定的字符串；单个 `Radio` 不保存任何状态，选中与否完全由你传入的 `checked` 决定。
- **表单校验**：放进 [Form](/components/form) 时用 `f.Item("类型").Rules(&kind, ui.Required("请选择类型"))`；字段联动时可用 `RulesFunc` 让表单通过 `f.Value(name)` 读取当前选项。
- **无障碍**：单选框组在表单项里默认以表单项标签命名，单独使用时用 `A11yLabel` 命名整组；Tab 聚焦后可用键盘激活选项。
- **RTL**：在 RTL 范围内圆点位于文字右侧，按钮组的顺序也随之镜像。

## 示例代码

<!--@include: ./_gen/radio-examples.md-->

## API

<!--@include: ./_gen/radio-api.md-->

## 相关链接

- [源码 ui/choice.go](https://github.com/tangoui/tango/blob/main/ui/choice.go)
- [ui Go API](/reference/api/ui)
- [Checkbox 多选框](/components/checkbox)、[Select 选择器](/components/select)、[Segmented 分段控制器](/components/segmented)、[Form 表单](/components/form)
