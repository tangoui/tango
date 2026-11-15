---
title: Form 表单
---
# Form 表单

高性能表单容器：统一管理一组带标签的表单项，负责布局、数据校验（同步与异步）、字段联动、动态增减项，以及提交与重置。

## 何时使用

- 需要收集、校验并提交一组数据，例如注册、创建或编辑一条记录。
- 多个输入控件需要统一的标签宽度、对齐方式、尺寸或禁用状态。
- 字段之间存在依赖：显示/隐藏、选项随上级刷新、跨字段校验（确认密码）。
- 表单字段完全由配置描述（值类型、枚举、栅格、只读切换、异步初始值）时，改用 [ProForm 高级表单](/components/proform)，它在 Form 之上内置了提交按钮和数据管理。

## 在线体验

<GalleryPreview component="Form" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var user, email string

func page(c *ui.Context) {
	f := ui.Form(c).LabelWidth(90)
	f.Body(func() {
		f.Item("用户名").Rules(&user, ui.Required("请输入用户名"), ui.MinLen(3, "至少 3 个字符")).Body(func() {
			ui.Input(c, &user).Placeholder("请输入用户名")
		})
		f.Item("邮箱").Rules(&email, ui.Required("请输入邮箱"), ui.Email("邮箱格式不正确")).Body(func() {
			ui.Input(c, &email).Placeholder("name@example.com")
		})
		f.Item("").NoLabel().Body(func() {
			ui.Row(c, func() {
				if ui.Button(c, "提交").Primary().Clicked() {
					f.Submit()
				}
				if ui.Button(c, "重置").Clicked() {
					f.ResetFields()
				}
			}, layout.Gap(8))
		})
	})
	if f.Finished() {
		ui.Message(c, "success", "提交成功："+user)
	}
}
```

`ui.Form(c)` 创建表单，`f.Item(label)` 创建一个带标签的表单项，`Body` 里放真正的输入控件。`Rules(&值, 规则...)` 把校验规则绑定到这个值上：值变化后自动校验，错误信息显示在控件下方。表单本身不保存业务数据——数据就是你声明的变量，表单只保存校验状态。`Submit()` 校验全部表单项，全部通过后 `Finished()` 返回一次 `true`。

## 常见用法

### 表单项与布局

默认是水平布局（标签在左）。`Vertical()` 让标签位于控件上方，`Inline()` 把所有项排成一行（常用于查询栏）。`LabelWidth`、`LabelAlign`、`Colon` 控制标签；`Size` 统一设置表单内所有控件的尺寸（控件自己的 `Size` 优先）。

表单项上常用的修饰：`Tooltip` 在标签后显示问号提示，`Extra` 在控件下方显示说明文字，`Help` 显示提示（有错误时被错误信息替代），`NoLabel` 用于按钮行（与上方控件对齐但不显示标签）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var keyword, status string

func page(c *ui.Context) {
	f := ui.Form(c).Inline().Size(ui.Small)
	f.Body(func() {
		f.Item("关键字").Tooltip("按名称或编号模糊匹配").Body(func() {
			ui.Input(c, &keyword).Width(160)
		})
		f.Item("状态").Body(func() {
			ui.Select(c, &status, ui.Opts("全部", "进行中", "已完成")).Width(120)
		})
		f.Item("").NoLabel().Body(func() { ui.Button(c, "查询").Primary() })
	})

	v := ui.Form(c).Vertical().RequiredMark(false)
	v.Body(func() {
		v.Item("描述").Extra("不超过 200 字，提交后可在详情页修改").Body(func() {
			ui.TextArea(c, &keyword).Rows(3)
		})
	})
}
```

### 校验规则

字符串值用 `Rules(&s, ...)`，内置规则：`Required`、`MinLen`、`MaxLen`、`Pattern`（正则）、`Email`、`Match`（与另一个值相同）、`Check`（自定义函数，返回错误信息或空串）。任何规则都可以加 `.Warning()` 变成警告：显示警告色但不阻止提交（对标 antd 的 `warningOnly`）。

非字符串值用泛型函数 `ui.RulesOf(item, &v, ...)`，规则有 `RequiredValue`、`MinValue`、`MaxValue`、`RangeValue`、`RequiredItems`、`MinItems`、`MaxItems`、`MustBeTrue`、`CheckValue`。带 `Required` 类规则的项会自动显示必填星号。

校验时机默认是值变化时（`ValidateOnChange`），可用 `ValidateTrigger` 在表单或单个表单项上改为失焦（`ValidateOnBlur`）或仅提交时（`ValidateOnSubmit`）。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var (
	nick, phone, mail string
	age               float64
	tags              []string
	agree             bool
)

func page(c *ui.Context) {
	f := ui.Form(c).LabelWidth(100)
	f.Body(func() {
		f.Item("昵称").Rules(&nick, ui.MinLen(6, "建议至少 6 个字符").Warning()).Body(func() {
			ui.Input(c, &nick)
		})
		f.Item("手机").Rules(&phone, ui.Required("请输入手机号"), ui.Pattern(`^1\d{10}$`, "手机号格式不正确")).Body(func() {
			ui.Input(c, &phone)
		})
		f.Item("邮箱").ValidateTrigger(ui.ValidateOnBlur).Rules(&mail, ui.Email("邮箱格式不正确")).Body(func() {
			ui.Input(c, &mail)
		})
		ui.RulesOf(f.Item("年龄"), &age, ui.RangeValue(18.0, 60.0, "年龄需在 18–60 之间")).Body(func() {
			ui.InputNumber(c, &age)
		})
		ui.RulesOf(f.Item("兴趣"), &tags, ui.RequiredItems[string]("至少选择一项")).Body(func() {
			ui.CheckboxGroup(c, &tags, ui.Opts("阅读", "运动", "音乐"))
		})
		ui.RulesOf(f.Item("协议").NoLabel(), &agree, ui.MustBeTrue("请先同意用户协议")).Body(func() {
			ui.Checkbox(c, "我已阅读并同意用户协议", &agree)
		})
		f.Item("").NoLabel().Body(func() {
			ui.Row(c, func() {
				if ui.Button(c, "校验").Primary().Clicked() {
					f.Validate()
				}
				if ui.Button(c, "只校验手机").Clicked() {
					f.ValidateFields("手机")
				}
				if ui.Button(c, "清除校验").Clicked() {
					f.ClearValidate()
				}
			}, layout.Gap(8))
		})
	})
}
```

`Validate()` 立即校验本帧已构建的全部表单项并返回是否通过；`ValidateFields(names...)` 只校验指定项（按 `Name`，未设置时按标签）；`ClearValidate(names...)` 只清除校验结果，不改值。

### 异步校验

`ui.AsyncCheck` 适合需要请求服务器的校验（用户名是否被占用）。函数拿到值后发起检查，在任意 goroutine 中调用 `done(错误信息或 "")`；它在同步规则都通过后才运行，同一个值只查一次，进行中表单项显示“校验中”状态。`Submit()` 会等待异步规则完成，期间 `Submitting()` 为 `true`，可以直接用来给提交按钮加 `Loading`。

```go
import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui"
)

var account string

func page(c *ui.Context) {
	f := ui.Form(c).LabelWidth(90)
	f.Body(func() {
		f.Item("用户名").Rules(&account, ui.Required("请输入用户名"), ui.AsyncCheck(func(v string, done func(string)) {
			go func() {
				time.Sleep(300 * time.Millisecond) // 模拟请求
				if strings.EqualFold(v, "admin") {
					done("用户名已被占用")
					return
				}
				done("")
			}()
		})).Body(func() { ui.Input(c, &account) })
		f.Item("").NoLabel().Body(func() {
			if ui.Button(c, "注册").Primary().Loading(f.Submitting()).Clicked() {
				f.Submit()
			}
		})
	})
	if f.Finished() {
		ui.Message(c, "success", "注册成功")
	}
}
```

### 字段联动

即时模式下联动就是普通的 Go 条件：每帧根据当前值决定 `Hidden`、`Disabled` 和选项。需要按“字段名”读取别的表单项时，给它 `Name`，并用 `f.Value(name)` 读取上一次看到的值（字符串形式）；非字符串值用 `RulesFunc` 或 `RulesOf` 绑定后同样能读取。

`Dependencies(names...)` 声明依赖：被依赖字段的值变化时，本项自动重新校验，并执行 `OnDepsChange`（例如省份变化时清空城市）。跨字段校验（确认密码）用 `ui.Match` 配合 `Dependencies`。隐藏的项不构建、也不参与校验。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	kind             = "person"
	company          string
	province, city   string
	pwd, confirm     string
	citiesOfProvince = map[string][]ui.Option{
		"浙江": ui.Opts("杭州", "宁波"),
		"江苏": ui.Opts("南京", "苏州"),
	}
)

func page(c *ui.Context) {
	f := ui.Form(c).LabelWidth(100).ScrollToFirstError()
	f.Body(func() {
		f.Item("主体类型").Name("kind").RulesFunc(func() string { return kind }).Body(func() {
			ui.RadioGroup(c, &kind, []ui.Option{{Label: "个人", Value: "person"}, {Label: "企业", Value: "company"}}).Buttons()
		})
		f.Item("企业名称").Hidden(f.Value("kind") != "company").
			Rules(&company, ui.Required("请输入企业名称")).Body(func() { ui.Input(c, &company) })
		f.Item("省份").Name("province").RulesFunc(func() string { return province }).Body(func() {
			ui.Select(c, &province, ui.Opts("浙江", "江苏"))
		})
		f.Item("城市").Dependencies("province").OnDepsChange(func() { city = "" }).
			Disabled(province == "").Rules(&city, ui.Required("请选择城市")).Body(func() {
			ui.Select(c, &city, citiesOfProvince[province])
		})
		f.Item("密码").Name("pwd").Rules(&pwd, ui.Required("请输入密码"), ui.MinLen(8, "至少 8 位")).Body(func() {
			ui.Input(c, &pwd).Password()
		})
		f.Item("确认密码").Dependencies("pwd").Rules(&confirm, ui.Match(&pwd, "两次输入的密码不一致")).Body(func() {
			ui.Input(c, &confirm).Password()
		})
	})
}
```

### 动态增减表单项 FormList

`ui.FormList(f, 标签, &切片)` 把一组表单项绑定到切片（对标 antd `Form.List`）：每个元素是一行，自带添加、删除、上移、下移按钮。`Min`/`Max` 限制行数，`Creator` 决定新行的初始值，`Rule` 校验列表本身（例如至少两行）。每行里的表单项必须用 `row.Name(field)` 生成唯一名字，否则同标签的多行会共用校验状态。

```go
import (
	"github.com/tangoui/tango/ui"
)

type contact struct{ Name, Phone string }

var (
	project  string
	contacts = []contact{{Name: "曲丽丽", Phone: "13800000000"}}
)

func page(c *ui.Context) {
	f := ui.Form(c).LabelWidth(90)
	f.Body(func() {
		f.Item("项目名称").Rules(&project, ui.Required("请输入项目名称")).Body(func() {
			ui.Input(c, &project)
		})
		ui.FormList(f, "联系人", &contacts).Min(1).Max(5).AddText("添加联系人").
			Creator(func() contact { return contact{} }).
			Rule(func(n int) string {
				if n < 2 {
					return "至少需要 2 位联系人"
				}
				return ""
			}).
			Body(func(row *ui.FormListRow[contact]) {
				v := row.Value
				f.Item("姓名").Name(row.Name("name")).Rules(&v.Name, ui.Required("请输入姓名")).Body(func() {
					ui.Input(c, &v.Name).Width(140)
				})
				f.Item("手机").Name(row.Name("phone")).Rules(&v.Phone, ui.Pattern(`^1\d{10}$`, "手机号格式不正确")).Body(func() {
					ui.Input(c, &v.Phone).Width(160)
				})
			})
		f.Item("").NoLabel().Body(func() {
			if ui.Button(c, "提交").Primary().Clicked() {
				f.Submit()
			}
		})
	})
}
```

需要自定义行操作时用 `NoActions()` 隐藏默认按钮，再在行内调用 `row.Remove()`、`row.MoveUp()`、`row.MoveDown()`；`Changed()` 报告本帧是否增删或移动了行。也可以用 `ConfigProvider.Defaults("ProFormList", ...)` 统一设置 `min`、`max`、`addText`。

### 提交与重置

| 方法 | 作用 |
| --- | --- |
| `Submit()` | 校验全部表单项（等待异步规则），通过后让 `Finished()` 返回一次 `true` |
| `Finished()` | 本次提交是否完成；读一次后复位，适合在 `Body` 之后判断 |
| `Submitting()` / `Validating()` | 提交是否在等待异步校验 / 是否有异步规则在运行 |
| `Validate()` | 同步校验本帧已构建的表单项，立即返回结果（不等待异步规则） |
| `ResetFields(names...)` | 把用 `Rules`、`RulesOf` 绑定的值恢复为表单项首次构建时的值，并清除校验；`RulesFunc` 绑定的项改为调用它的 `OnReset` |
| `ResetValidation()` | 只清除错误和“已触碰”标记，不改值 |
| `ScrollToFirstError()` | 校验失败时把第一个错误项滚动到可见区域 |

`Errors()` 返回当前每个表单项的错误信息，可用于在表单外汇总展示。表单级 `Disabled(true)` 会禁用其中所有控件（例如提交期间锁定表单）。

## 主题与样式

- 三套风格的差异主要在标签与消息（切换风格无需改代码）：
  - Ant Design：表单项间距 24px，标签用正文色，帮助与 Extra 用三级文字色。
  - Element Plus：标签用次要文字色、与控件间距 12px，表单项间距 18px，错误与帮助文字 12px。
  - shadcn：标签字重 500，出错时标签也变为错误色；垂直布局的标签更紧凑（行高 14px，下方 8px），Extra 与控件间距 8px。
- 组件令牌：`theme.Components.Form`（`FormToken`），包含 `LabelColor`、`LabelFontSize`、`LabelRequiredMarkColor`、`LabelTooltipColor`、`ItemMarginBottom`、`InlineItemMarginBottom`、`ErrorColor`、`WarningColor`、`HelpColor`、`VerticalLabelHeight` 等，可通过 `ui.ConfigProvider(c).Components(...)` 局部覆盖。
- `ConfigProvider.Defaults("Form", ui.Props{...})` 支持的键：`layout`（`"horizontal"`、`"vertical"`、`"inline"`）、`labelWidth`、`labelAlign`（`"left"`、`"right"`）、`colon`、`requiredMark`、`disabled`、`validateTrigger`（`"onChange"`、`"onBlur"`、`"onSubmit"`）、`size`。

## 注意事项

- **状态放在哪**：表单值是你自己的变量（包级、闭包或结构体字段），表单只在内部保存校验状态（错误、触碰标记、重置用的初始值），按表单项的 `Name`（默认标签）索引。同一表单里标签重复的项（例如 FormList 的各行）必须设置不同的 `Name`。
- **Key 的作用**：在循环里渲染多个表单、或表单前面的兄弟组件会动态增删时，用 `ui.Key(c, id, func() { ... })` 包住表单，避免校验状态串到别的表单上。
- **Validate 的时机**：`Validate()` 只检查“本帧已构建”的表单项；放在最后一项里的按钮或 `Body` 之后调用即可覆盖全部表单项。`Submit()` 没有这个限制，它会在 `Body` 结束时统一校验。
- **初始值与重置**：`ResetFields` 恢复的是表单项**第一次构建时**的值；若要从服务器加载初始值，先赋值再首次渲染表单，或在加载完成后自行赋值并调用 `ResetValidation()`。
- **异步回调**：`AsyncCheck` 的 `done` 可在任意 goroutine 调用；不要在回调里直接修改界面绑定的变量以外的共享状态而不加锁。
- **无障碍**：表单项把标签、必填标记和校验信息传给其中第一个控件，作为它的无障碍名称与描述；没有可见标签的控件（`NoLabel`）请给控件本身设置 `A11yLabel`。
- **RTL**：在 `ConfigProvider.Direction` 为 RTL 的范围内，标签移到控件右侧，`LabelAlign` 的左/右按逻辑方向（起始/结束）解析。

## 示例代码

<!--@include: ./_gen/form-examples.md-->

## API

<!--@include: ./_gen/form-api.md-->

`FormList`、`RulesOf` 与类型化规则（`RequiredValue`、`RangeValue`、`MustBeTrue` 等）的签名见 [Go API 参考](/reference/api/ui#FormList)。

## 相关链接

- [源码 ui/form.go](https://github.com/tangoui/tango/blob/main/ui/form.go)、[ui/p2d_form.go](https://github.com/tangoui/tango/blob/main/ui/p2d_form.go)、[ui/form_pro_list.go](https://github.com/tangoui/tango/blob/main/ui/form_pro_list.go)
- [ui Go API](/reference/api/ui)
- [ProForm 高级表单](/components/proform)
- [Input 输入框](/components/input)、[Select 选择器](/components/select)、[InputNumber 数字输入框](/components/inputnumber)、[Checkbox 多选框](/components/checkbox)、[Radio 单选框](/components/radio)
- [Go GUI 入门](/guide/ui)
