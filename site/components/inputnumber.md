---
title: InputNumber 数字输入框
---
# InputNumber 数字输入框

通过键盘输入或步进按钮输入一个在范围内的数字，支持精度、步长、格式化显示和高精度小数。

## 何时使用

- 需要输入数值（数量、金额、比例），并希望限制范围、步长或小数位数时。
- 只在一个连续区间里粗略取值、对精确数字不敏感时，用 [Slider 滑动输入条](/components/slider) 更直观。
- 输入的是编号、电话等“由数字组成的文本”时，用 [Input 输入框](/components/input)，不要用数字框。

## 在线体验

<GalleryPreview component="InputNumber" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"fmt"

	"github.com/tangoui/tango/ui"
)

var count = 3.0

func page(c *ui.Context) {
	if ui.InputNumber(c, &count).Min(1).Max(10).A11yLabel("数量").Changed() {
		fmt.Println("数量变为", count)
	}
}
```

值绑定到 `*float64`。输入框获得焦点时可直接输入，也可以点击步进按钮或按方向键 ↑ / ↓ 增减；离开输入框时超出 `Min` / `Max` 的值会被修正。`Changed()` 报告本帧值是否改变。

## 常见用法

### 步长与精度

`Step` 设置每次增减的量，`Precision` 固定显示和保存的小数位数。`Wheel()` 允许在获得焦点时用鼠标滚轮步进（对应 antd `changeOnWheel`）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var ratio = 1.5

func page(c *ui.Context) {
	ui.InputNumber(c, &ratio).Min(0).Max(5).Step(0.5).Precision(1).Wheel().Width(120).A11yLabel("比例")
}
```

### 格式化显示

`Formatter` 把数值转换成展示文本（千分位、货币符号），`Parser` 把展示文本还原成纯数字，两者需要成对使用。只想在数字前加一个固定符号时，用 `PrefixText("¥")` 或 `Prefix(icon)` 更简单。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var price = 1280.0

func withComma(s string) string {
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if hasFrac {
		return "$ " + b.String() + "." + frac
	}
	return "$ " + b.String()
}

func page(c *ui.Context) {
	ui.InputNumber(c, &price).
		Formatter(withComma).
		Parser(func(s string) string { return strings.NewReplacer("$", "", ",", "", " ", "").Replace(s) }).
		Width(180).A11yLabel("金额")
	ui.InputNumber(c, &price).PrefixText("¥").Width(140).A11yLabel("价格")
}
```

### 可为空的值

默认情况下清空文本再离开会恢复原值。需要区分“没填”和“填了 0”时，用 `Empty(&empty)` 绑定一个布尔值：清空时 `empty` 为 `true`，输入框显示 `Placeholder`。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	discount      float64
	discountEmpty = true
)

func page(c *ui.Context) {
	ui.InputNumber(c, &discount).Empty(&discountEmpty).Placeholder("不打折").Min(0).Max(100)
	if !discountEmpty {
		ui.Text(c, "已设置折扣")
	}
}
```

### 高精度小数

`float64` 无法精确表示 `0.1 + 0.2` 或 20 位金额。`StringMode(&s)` 把值绑定成十进制字符串，步进和范围都按十进制计算（对应 antd `stringMode`）；此时第二个参数可以传 `nil`。

```go
import (
	"github.com/tangoui/tango/ui"
)

var exact = "0.1"

func page(c *ui.Context) {
	ui.InputNumber(c, nil).StringMode(&exact).Step(0.000000000000000001).Width(260).A11yLabel("高精度")
	ui.Text(c, "当前值："+exact)
}
```

### 按钮位置、尺寸与状态

`ControlsPosition("sides")` 把 − / + 放在两侧（Element Plus 的默认样式），`"right"` 为右侧上下堆叠（Ant Design 的样式）。`Size(ui.Small)` / `Size(ui.Large)` 设置尺寸，`Variant` 设置外观（`"outlined"`、`"filled"`、`"borderless"`、`"underlined"`），`Status("error" | "warning")` 设置校验状态。

```go
import (
	"github.com/tangoui/tango/ui"
)

var qty = 2.0

func page(c *ui.Context) {
	ui.InputNumber(c, &qty).ControlsPosition("sides").Min(1).Max(10).A11yLabel("数量")
	ui.InputNumber(c, &qty).Size(ui.Small).Variant("filled").A11yLabel("数量（小）")
	ui.InputNumber(c, &qty).Status("warning").A11yLabel("数量（警告）")
	ui.InputNumber(c, &qty).Disabled(true).A11yLabel("数量（禁用）")
}
```

## 主题与样式

- 步进按钮的默认位置随设计体系变化：Element Plus 下为两侧（`sides`），Ant Design 与 shadcn 下为右侧堆叠（`right`）。
- 组件令牌：`theme.Components.InputNumber`（`InputNumberToken`）控制默认宽度 `ControlWidth`、步进按钮宽度与颜色（`HandleWidth`、`HandleColor`、`HandleHoverBg`、`HandleBorderColor`…）以及两侧按钮（`SideHandleBg`、`SideHandleColor`）；边框、文字、占位符沿用 Input 令牌。
- `ConfigProvider.Defaults("InputNumber", ui.Props{...})` 支持 `controlsPosition`、`variant`、`wheel`。

## 注意事项

- **状态**：数值就是你绑定的变量；正在编辑的文本由组件内部按调用位置保存。在列表里渲染多个数字框时用 `ui.Key` 包住每一行，避免增删行后编辑状态错位。
- **表单校验**：放进 [Form](/components/form) 时用 `ui.RulesOf(f.Item("年龄"), &age, ui.RangeValue(18.0, 60.0, "..."))` 绑定数值规则；在出错的表单项里，数字框自动显示错误状态。
- **无障碍**：在表单项里自动使用表单项标签作为名称；单独使用时设置 `A11yLabel`。键盘 ↑ / ↓ 按步长增减。
- **格式化**：`Formatter` 与 `Parser` 必须互为逆运算，否则编辑后的文本无法还原成数字。

## 示例代码

<!--@include: ./_gen/inputnumber-examples.md-->

## API

<!--@include: ./_gen/inputnumber-api.md-->

## 相关链接

- [源码 ui/input.go](https://github.com/tangoui/tango/blob/main/ui/input.go)
- [ui Go API](/reference/api/ui)
- [Input 输入框](/components/input)、[Slider 滑动输入条](/components/slider)、[Form 表单](/components/form)
