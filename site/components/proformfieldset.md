---
title: ProFormFieldSet 组合字段
---
# ProFormFieldSet 组合字段

把多个相邻控件组合成一个带统一标签、校验和 `[]any` 值的 ProField，适合金额区间、区号加号码等成组输入。

## 何时使用

- 一个业务字段由两个或多个有顺序的部分组成，例如最低价与最高价、区号与号码。
- 这些部分需要共享一个标签、必填状态或跨部分校验时。
- 同一组字段需要放进 `ProForm`、`QueryFilter`、`ModalForm` 或 `StepsForm` 时。
- 各控件之间没有共同值或校验关系时，直接定义多个 `ProField` 更清晰。

## 在线体验

<GalleryPreview component="ProFormFieldSet" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
package main

import "github.com/tangoui/tango/ui"

var rangeValues ui.ProValues

func page(c *ui.Context) {
    fields := []ui.ProField{
        ui.ProFormFieldSet(c, ui.ProFieldSet{
            Key: "budget", Label: "预算区间", Separator: "~", Required: true,
            Parts: []ui.ProField{
                {ValueType: ui.ValueMoney, Placeholder: "最低"},
                {ValueType: ui.ValueMoney, Placeholder: "最高"},
            },
        }),
    }
    ui.ProForm(c, fields).Values(&rangeValues).Horizontal().Show()
}

func main() {}
```

组合字段在 `ProValues["budget"]` 中保存为 `[]any`，可以使用 `rangeValues.Parts("budget")` 读取；每个元素对应 `Parts` 中同位置的控件，空控件为 `nil`。

## 常见用法

### 数字区间与跨段校验

`Required` 要求每个部分都有值；`Validate(parts, all)` 在必填检查之后运行，可以检查最小值不大于最大值或其他业务关系。

```go
package main

import "github.com/tangoui/tango/ui"

var budgetValues ui.ProValues

func page(c *ui.Context) {
    fields := []ui.ProField{
        ui.ProFormFieldSet(c, ui.ProFieldSet{
            Key: "budget", Label: "预算区间", Separator: "~", Required: true,
            Parts: []ui.ProField{{ValueType: ui.ValueMoney}, {ValueType: ui.ValueMoney}},
            Validate: func(parts []any, all ui.ProValues) string {
                if len(parts) < 2 || parts[0] == nil || parts[1] == nil {
                    return ""
                }
                min, minOK := parts[0].(float64)
                max, maxOK := parts[1].(float64)
                if minOK && maxOK && min > max {
                    return "最低预算不能大于最高预算"
                }
                return ""
            },
        }),
    }
    ui.ProForm(c, fields).Values(&budgetValues).Show()
}

func main() {}
```

`all` 是表单的全部值，适合在组合字段校验时读取其他字段；示例只比较本组的 `parts`，避免依赖控件的文本格式。

### 区号、号码与每段宽度

每个 `ProField` 可以单独设置 `ValueType`、`ValueEnum`、`Placeholder` 和 `Width`。组合值仍然只占一个表单字段，便于提交时一起转换。

```go
package main

import "github.com/tangoui/tango/ui"

var phoneValues = ui.ProValues{"phone": []any{"86", ""}}

func page(c *ui.Context) {
    phone := ui.ProFormFieldSet(c, ui.ProFieldSet{
        Key: "phone", Label: "联系电话", Required: true, Help: "区号 + 号码",
        Parts: []ui.ProField{
            {ValueType: ui.ValueSelect, Width: 100,
                ValueEnum: []ui.ProEnum{{Value: "86", Label: "+86"}, {Value: "1", Label: "+1"}}},
            {Placeholder: "手机号码"},
        },
    })
    ui.ProForm(c, []ui.ProField{phone}).Values(&phoneValues).Horizontal().Show()
}

func main() {}
```

提交成功后可以把 `phoneValues.Parts("phone")` 转成后端需要的结构；不要假设空输入一定是字符串，统一按 `nil` 和具体值处理。

### 放进 QueryFilter 或 ModalForm

`ProFormFieldSet` 返回普通 `ProField`，所以可以与其他字段混用。查询表单会把组合字段作为一个条件，弹窗表单则复用同一份定义和校验。

```go
package main

import "github.com/tangoui/tango/ui"

var searchValues ui.ProValues

func page(c *ui.Context) {
    fields := []ui.ProField{
        {Key: "name", Label: "名称"},
        ui.ProFormFieldSet(c, ui.ProFieldSet{
            Key: "period", Label: "执行日期", Separator: "至",
            Parts: []ui.ProField{{ValueType: ui.ValueDate}, {ValueType: ui.ValueDate}},
        }),
    }
    q := ui.QueryFilter(c, fields).Values(&searchValues).DefaultExpanded().Show()
    if q.Searched() {
        search(searchValues.Parts("period"))
    }
}

func search(period []any) {}

func main() {}
```

### 在普通 Form 中使用 FieldSet

不需要 ProForm 的提交按钮或字段网格时，可以直接用 `FieldSet` 把组合字段放进 `Form`。`FieldSet` 和 `ProFormFieldSet` 使用同一份 `ProFieldSet` 定义。

```go
package main

import "github.com/tangoui/tango/ui"

var formValues = ui.ProValues{"phone": []any{"86", nil}}

func page(c *ui.Context) {
    f := ui.Form(c).LabelWidth(100)
    f.Body(func() {
        ui.FieldSet(c, f, ui.ProFieldSet{
            Key: "phone", Label: "联系电话", Required: true,
            Parts: []ui.ProField{
                {ValueType: ui.ValueSelect, Width: 90,
                    ValueEnum: []ui.ProEnum{{Value: "86", Label: "+86"}, {Value: "1", Label: "+1"}}},
                {},
            },
        }, formValues)
    })
}

func main() {}
```

## 主题与样式

组合字段沿用外层表单的标签、间距、控件令牌和尺寸；`Separator` 只绘制部分之间的文本，不改变值结构。若没有传 `Separator`，可以通过 `ConfigProvider.Defaults("ProFormFieldSet", ui.Props{"separator": "至"})` 设置作用域默认分隔符。Ant Design、Element Plus 和 shadcn 主题会分别应用自己的表单标签、输入框和焦点样式。

## 注意事项

- `ProValues.Parts(key)` 返回 `[]any`；读取时检查长度和 `nil`，不要直接断言每一段都为字符串或浮点数。
- `Required` 检查每个 `Parts` 项；只需要整体非空时，可省略 `Required` 并在 `Validate` 中定义自己的规则。
- `Initial` 可以设置组合字段初始值，长度应与 `Parts` 对应；表单重置会恢复该值。
- 每个部分都有可访问名称，通常继承组合字段的 `Label`；如果替换 `Render`，需要保留明确的 `A11yLabel`。

## 示例代码

<!--@include: ./_gen/proformfieldset-examples.md-->

## API

<!--@include: ./_gen/proformfieldset-api.md-->

## 相关链接

- [ProFormFieldSet gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_pro3_fieldset.go)
- [ProForm 高级表单](/components/proform)
- [QueryFilter 查询表单](/components/queryfilter)
- [Form 表单](/components/form)
- [源码](https://github.com/tangoui/tango/blob/main/ui/pro3_fieldset.go)
- [ui Go API](/reference/api/ui)
