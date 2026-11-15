---
title: ProForm 高级表单
---
# ProForm 高级表单

在 [Form](/components/form) 之上按字段定义生成表单（对标 Pro Components v3 的 ProForm）：用一组 `ui.ProField` 描述字段的值类型、枚举、栅格、校验与联动，ProForm 负责渲染控件、管理数据、异步加载初始值，并提供带加载状态的提交与重置按钮。

## 何时使用

- 字段较多、以“配置”方式描述更清晰的增删改表单，例如后台管理中的新建 / 编辑页面。
- 需要异步加载初始值、异步提交（按钮转圈、失败提示）、只读与编辑切换，而不想手写这些流程时。
- 字段定义需要与 [ProTable](/components/protable)、[ProDescriptions](/components/prodescriptions) 共用一套值类型时。
- 需要逐个控件精细排版、或字段很少时，直接用 [Form 表单](/components/form) 更灵活；同一份字段放进弹窗、抽屉、分步或查询栏时，用 ModalForm、DrawerForm、StepsForm、QueryFilter（见下文“其他表单外壳”）。

## 在线体验

<GalleryPreview component="ProForm" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var owners = []ui.ProEnum{{Value: "qll", Label: "曲丽丽"}, {Value: "zs", Label: "张三"}}

var fields = []ui.ProField{
	{Key: "name", Label: "规则名称", Required: true, Placeholder: "请输入规则名称"},
	{Key: "owner", Label: "负责人", ValueType: ui.ValueSelect, ValueEnum: owners, Required: true},
	{Key: "amount", Label: "预算金额", ValueType: ui.ValueMoney, Min: 0, Max: 1e7},
	{Key: "deadline", Label: "截止日期", ValueType: ui.ValueDate},
	{Key: "notify", Label: "上线通知", ValueType: ui.ValueSwitch},
}

func save(v ui.ProValues) error {
	time.Sleep(500 * time.Millisecond) // 模拟请求
	return nil
}

func page(c *ui.Context) {
	pf := ui.ProForm(c, fields).Columns(2).
		OnFinish(func(v ui.ProValues, done func(error)) {
			go func() { done(save(v)) }()
		}).
		Show()
	if pf.Finished() {
		ui.Message(c, "success", "保存成功")
	}
}
```

`ui.ProForm(c, fields)` 配置表单，链式设置后以 `Show()` 渲染。点击“提交”会校验全部字段（包括异步规则），通过后调用 `OnFinish`，参数是全部值 `ui.ProValues`（`map[string]any`）；在任意 goroutine 中调用 `done(err)` 结束提交，期间提交按钮显示加载状态。`done(nil)` 后 `Finished()` 返回一次 `true`；返回错误时表单底部显示错误提示，`Err()` 可读取该错误。

## 常见用法

### 字段与值类型

`ValueType` 决定字段使用的控件和值的 Go 类型，与 ProTable 的列共用一套：

| 值类型 | 控件 | 值类型（`ProValues` 中） |
| --- | --- | --- |
| `ValueText`（默认）、`ValuePassword`、`ValueTextArea` | 输入框 / 密码框 / 多行文本 | `string` |
| `ValueSelect`、`ValueRadio`、`ValueRadioButton`、`ValueSegmented` | 选择器 / 单选 / 按钮单选 / 分段控制器 | `string` |
| `ValueMultiSelect`、`ValueCheckbox` | 多选选择器 / 多选框组 | `[]string` |
| `ValueDigit`、`ValueMoney`、`ValuePercent`、`ValueRate`、`ValueSlider` | 数字框 / 金额 / 百分比 / 评分 / 滑块 | `float64` |
| `ValueDate`、`ValueDateTime`、`ValueDateWeek`、`ValueDateMonth`、`ValueDateQuarter`、`ValueDateYear`、`ValueTime` | 各粒度日期 / 时间选择 | `time.Time` |
| `ValueDateRange`、`ValueDateTimeRange`、`ValueTimeRange` | 范围选择 | `[2]time.Time` |
| `ValueSwitch` | 开关 | `bool` |
| `ValueCascader`、`ValueTreeSelect` | 级联选择 / 树选择（选项用 `ProEnum.Children`） | `[]string` / `string` |
| `ValueUpload` | 上传（`ProField.Pick` 打开文件选择，`Dragger` 显示拖拽区） | `[]ui.UploadFile` |
| `ValueImage`、`ValueAvatar` | 单图上传卡（头像也可以是名字文本） | `image.Image` 或 `ui.UploadFile`（见 Go API） |
| `ValueColor`、`ValueDigitRange`、`ValueCode`、`ValueJSONCode`、`ValueCaptcha`、`ValueFormList` | 颜色 / 数字范围 / 代码 / JSON / 验证码 / 动态列表 | 见 Go API |

枚举类字段的选项写在 `ValueEnum []ui.ProEnum`（`Value`、`Label`）。`Initial` 设置初始值（重置时恢复到它），`Placeholder`、`Help`（控件下方的提示）、`Width`（固定控件宽度）、`Min` / `Max` / `Precision`（数字字段）进一步配置控件。读取值时用 `ProValues` 的类型化方法：`String`、`Strings`、`Float`、`Bool`、`Time`、`Range`、`Files`、`DigitRange`。

### 栅格布局与分组

`Columns(n)` 把字段排成 n 列等宽栅格（列间距由 `Gutter` 设置，默认 24）；单个字段用 `Span`（以 24 为满宽）覆盖自己的宽度。默认标签在控件上方，`Horizontal()` 改为标签在左，配合 `LabelWidth`。`Group` 相同的连续字段会被自动包进带标题的 [ProFormGroup](/components/catalog/proformgroup)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var fields = []ui.ProField{
	{Key: "name", Label: "姓名", Group: "基本信息", Required: true},
	{Key: "phone", Label: "手机号", Group: "基本信息"},
	{Key: "addr", Label: "详细地址", Group: "联系方式", Span: 24},
	{Key: "remark", Label: "备注", Group: "联系方式", ValueType: ui.ValueTextArea, Span: 24},
}

func page(c *ui.Context) {
	ui.ProForm(c, fields).Columns(2).Horizontal().LabelWidth(90).Show()
}
```

### 校验与字段联动

- 校验：`Required` 添加必填规则（使用语言包中的提示）；`Rules` 接受 Form 的字符串规则（`ui.Pattern`、`ui.MinLen`…）检查值的文本形式；`Validate(v, all)` 拿到字段的类型化值和全部值，适合跨字段校验；`AsyncValidate` 在前面的规则都通过后运行（例如查重）。
- 联动：`Hide(all)` 返回 `true` 时隐藏字段（也不校验）；`Disable(all)` 控制禁用；`Options(all)` 动态提供选项；`Dependencies` 列出依赖的字段，依赖变化时重新校验本字段、刷新选项，并丢弃新选项中已不存在的值。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	provinces = []ui.ProEnum{{Value: "zj", Label: "浙江"}, {Value: "js", Label: "江苏"}}
	cities    = map[string][]ui.ProEnum{
		"zj": {{Value: "hz", Label: "杭州"}, {Value: "nb", Label: "宁波"}},
		"js": {{Value: "nj", Label: "南京"}},
	}
)

var fields = []ui.ProField{
	{Key: "name", Label: "规则名称", Required: true, Rules: []ui.Rule{ui.MaxLen(20, "不超过 20 个字符")},
		AsyncValidate: func(v any, done func(string)) {
			go func() {
				if v == "TradeCode 99" {
					done("规则名称已存在")
					return
				}
				done("")
			}()
		}},
	{Key: "province", Label: "省份", ValueType: ui.ValueSelect, ValueEnum: provinces},
	{Key: "city", Label: "城市", ValueType: ui.ValueSelect, Dependencies: []string{"province"},
		Options: func(all ui.ProValues) []ui.ProEnum { return cities[all.String("province")] },
		Disable: func(all ui.ProValues) bool { return all.String("province") == "" }},
	{Key: "amount", Label: "预算金额", ValueType: ui.ValueMoney},
	{Key: "budgetMax", Label: "预算上限", ValueType: ui.ValueDigit, Dependencies: []string{"amount"},
		Validate: func(v any, all ui.ProValues) string {
			if _, ok := v.(float64); ok && all.Float("budgetMax") < all.Float("amount") {
				return "预算上限不能小于预算金额"
			}
			return ""
		}},
	{Key: "closed", Label: "已关闭", ValueType: ui.ValueSwitch},
	{Key: "reason", Label: "关闭原因", ValueType: ui.ValueTextArea, Required: true,
		Hide: func(all ui.ProValues) bool { return !all.Bool("closed") }},
}

func page(c *ui.Context) {
	ui.ProForm(c, fields).Columns(2).Show()
}
```

### 初始值与数据绑定

`Request` 异步加载初始值：在任意 goroutine 中调用 `done(值, err)`，加载期间表单显示加载遮罩、`Loading()` 为 `true`；加载结果同时成为“重置”恢复的初始值。`Values(&vals)` 把表单数据绑定到你自己的 `ui.ProValues` 变量，便于在表单外读取或修改；不绑定时表单自己保存数据。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	fields = []ui.ProField{
		{Key: "name", Label: "规则名称", Required: true},
		{Key: "deadline", Label: "截止日期", ValueType: ui.ValueDate},
		{Key: "tags", Label: "标签", ValueType: ui.ValueCheckbox,
			ValueEnum: []ui.ProEnum{{Value: "core", Label: "核心"}, {Value: "beta", Label: "实验"}}},
	}
	vals ui.ProValues
)

func page(c *ui.Context) {
	ui.ProForm(c, fields).Values(&vals).
		Request(func(done func(ui.ProValues, error)) {
			go func() {
				done(ui.ProValues{
					"name":     "TradeCode 99",
					"deadline": time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
					"tags":     []string{"core"},
				}, nil)
			}()
		}).
		Show()
	ui.Text(c, "当前名称："+vals.String("name"))
}
```

### 自定义提交按钮

`SubmitText`、`ResetText` 修改按钮文字，`NoReset()` 隐藏重置按钮。`Submitter(func(s *ui.ProSubmitter))` 完全替换按钮区，通过 `s.Submit()`、`s.Reset()`、`s.Loading()` 操作表单。`NoSubmitter()` 隐藏按钮区，改由外部调用 `pf.Submit()` / `pf.Reset()`（例如放在页面底部工具栏或弹窗页脚）。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var fields = []ui.ProField{{Key: "title", Label: "标题", Required: true}}

func page(c *ui.Context) {
	ui.ProForm(c, fields).Submitter(func(s *ui.ProSubmitter) {
		ui.Row(c, func() {
			if ui.Button(c, "保存并发布").Primary().Loading(s.Loading()).Clicked() {
				s.Submit()
			}
			if ui.Button(c, "恢复初始值").Clicked() {
				s.Reset()
			}
		}, layout.Gap(8))
	}).Show()

	pf := ui.ProForm(c, fields).NoSubmitter().Show()
	if ui.Button(c, "外部提交").Primary().Loading(pf.Submitting()).Clicked() {
		pf.Submit()
	}
}
```

### 只读模式

`Readonly(true)` 把每个字段显示为按值类型格式化后的只读文本（金额、日期、选项标签、开关文字…），不做校验、不显示提交按钮；`ProField.Readonly` 只对单个字段生效。适合“查看 / 编辑”切换的详情页。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	readonly = true
	fields   = []ui.ProField{
		{Key: "name", Label: "规则名称", Readonly: true}, // 始终只读
		{Key: "amount", Label: "预算金额", ValueType: ui.ValueMoney},
		{Key: "notify", Label: "上线通知", ValueType: ui.ValueSwitch},
	}
	detail = ui.ProValues{"name": "TradeCode 99", "amount": 12800.0, "notify": true}
)

func page(c *ui.Context) {
	ui.Checkbox(c, "只读", &readonly)
	ui.ProForm(c, fields).Columns(2).Values(&detail).Readonly(readonly).Show()
}
```

### 动态列表与自定义控件

字段的 `ValueType` 设为 `ui.ValueFormList` 并通过 `List: &ui.ProFieldList{...}` 给出每行的字段，即可在 ProForm 中编辑多行数据（值为 `[]ui.ProValues`）。`Render` 用自定义控件替换内置控件，读写 `vals[Key]` 即可；`Children(func(f *ui.FormW))` 在字段之后追加任意 Form 表单项（例如 [FormList](/components/form#动态增减表单项-formlist)）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	roles  = []ui.ProEnum{{Value: "owner", Label: "负责人"}, {Value: "dev", Label: "开发"}}
	fields = []ui.ProField{
		{Key: "members", Label: "成员", ValueType: ui.ValueFormList, List: &ui.ProFieldList{
			Max: 5, AddText: "添加成员",
			Fields: []ui.ProField{
				{Key: "name", Label: "姓名", Required: true, Width: 160},
				{Key: "role", Label: "角色", ValueType: ui.ValueSelect, ValueEnum: roles, Initial: "dev", Width: 140},
			},
		}},
		{Key: "color", Label: "主题色", Render: func(c *ui.Context, vals ui.ProValues) {
			s := vals.String("color")
			if ui.Input(c, &s).Placeholder("#1677ff").Changed() {
				vals["color"] = s
			}
		}},
	}
	agree bool
)

func page(c *ui.Context) {
	ui.ProForm(c, fields).Children(func(f *ui.FormW) {
		ui.RulesOf(f.Item("协议").NoLabel(), &agree, ui.MustBeTrue("请先同意协议")).Body(func() {
			ui.Checkbox(c, "我已阅读并同意服务协议", &agree)
		})
	}).Show()
}
```

### 其他表单外壳

同一组 `[]ui.ProField` 还可以交给这些组件渲染，字段定义、校验与联动规则完全相同：

| 组件 | 用途 |
| --- | --- |
| `ui.ModalForm(c, &open, 标题, fields)` / `ui.DrawerForm(...)` | 弹窗 / 抽屉中的表单，提交成功后自动关闭 |
| `ui.StepsForm(c, steps)` | 分步表单，每步一组字段，最后一步提交全部值 |
| `ui.QueryFilter(c, fields)` | 列表页顶部的查询表单，自动折叠多余字段 |
| `ui.LightFilter(c, fields)` | 轻量筛选栏，每个字段是一个可展开的筛选按钮，多余的收进“更多筛选” |
| `ui.SchemaForm(c, schema)` | 统一入口，用 `LayoutType` 在以上外壳之间切换 |
| `ui.LoginForm(c)` | 登录表单 |

详见 [ModalForm](/components/catalog/modalform)、[StepsForm 分步表单](/components/catalog/stepsform)、[QueryFilter 查询表单](/components/catalog/queryfilter)、[LightFilter 轻量筛选](/components/catalog/lightfilter)、[LoginForm 登录表单](/components/catalog/loginform)。

## 主题与样式

- ProForm 内部使用 Form 与各输入组件，样式完全跟随 `theme.Components.Form` 与对应控件的令牌，三套风格切换无需改代码。
- `ConfigProvider.Defaults("ProForm", ui.Props{...})` 支持 `layout`（`"vertical"`、`"horizontal"`）、`labelWidth`、`columns`、`gutter`、`submitter`（`false` 隐藏按钮区）；`"ProFormList"` 支持 `min`、`max`、`addText`。

## 注意事项

- **状态放在哪**：不调用 `Values` 时，表单数据和“初始值快照”保存在组件内部（按调用位置）；需要在表单外读写数据时用 `Values(&vals)` 绑定。`fields` 建议放在包级变量或缓存中，避免每帧重建大量闭包。
- **Key 的作用**：在循环中渲染多个 ProForm（例如每个标签页一个），用 `ui.Key(c, id, ...)` 区分，否则它们会共享内部状态。
- **Show 之后再读状态**：`Finished()`、`Submitting()`、`Loading()`、`Err()`、`Form()` 反映最近一次 `Show()` 的结果，应在 `Show()` 之后调用；`Finished()` 读一次后复位。
- **异步回调**：`Request`、`OnFinish`、`AsyncValidate` 的 `done` 可以在任意 goroutine 调用，界面会自动刷新；不要在这些 goroutine 中直接修改绑定的 `ProValues`。
- **值的类型**：`ProValues` 中的值类型由 `ValueType` 决定（见上表）；用 `Initial` 或 `Request` 设置初始值时请使用对应的 Go 类型（例如金额用 `float64` 而不是 `int`）。
- **无障碍**：每个字段的标签自动成为控件的无障碍名称，必填、错误信息同样传给控件。

## 示例代码

<!--@include: ./_gen/proform-examples.md-->

## API

<!--@include: ./_gen/proform-api.md-->

## 相关链接

- [源码 ui/form_pro.go](https://github.com/tangoui/tango/blob/main/ui/form_pro.go)、[ui/form_pro_field.go](https://github.com/tangoui/tango/blob/main/ui/form_pro_field.go)
- [ui Go API](/reference/api/ui)
- [Form 表单](/components/form)、[ProTable](/components/protable)、[ProDescriptions](/components/prodescriptions)、[ProFormGroup 表单分组](/components/catalog/proformgroup)
