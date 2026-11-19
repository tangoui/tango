---
title: FormList 动态增减表单项
---
# FormList 动态增减表单项

在 `Form` 中管理绑定切片的动态行，提供添加、删除、上移下移和逐行校验。

## 何时使用

- 联系人、成员、地址或规则明细等数量不固定的重复字段。
- 每行字段需要独立校验，同时列表整体还有最小/最大数量约束时。
- 结构固定且只有一组字段时，直接使用 [Form 表单](/components/form)；只读重复数据用 [List 列表](/components/list)。
- 用配置描述的 ProForm 动态字段，也可使用 `ValueFormList` 和 `ProFieldList`。

## 在线体验

<GalleryPreview component="FormList" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

type Member struct {
    Name string
    Role string
}

var members = []Member{{Name: "曲丽丽", Role: "owner"}}

func page(c *ui.Context) {
    f := ui.Form(c)
    f.Body(func() {
        ui.FormList(f, "成员", &members).Min(1).Max(5).
            Creator(func() Member { return Member{Role: "member"} }).
            Body(func(row *ui.FormListRow[Member]) {
                value := row.Value
                f.Item("姓名").Name(row.Name("name")).Body(func() {
                    ui.Input(c, &value.Name).Placeholder("请输入姓名")
                })
            })
    })
}
```

`FormListRow.Value` 指向当前行，`Name` 为该行字段生成稳定的表单字段名；把切片放在页面外的状态中即可在即时模式下保留行数据。

## 常见用法

### 行级校验与列表规则

行内字段通过 `FormItem.Rules` 校验，`Rule` 接收当前行数并校验列表整体。`Min` 在构建时补足行数，并禁止删到下限以下。`Max` 限制继续添加，默认不限制行数。

```go
import "github.com/tangoui/tango/ui"

type Contact struct{ Name, Phone string }

var contacts = []Contact{{}}

func page(c *ui.Context) {
    f := ui.Form(c)
    f.Body(func() {
        ui.FormList(f, "联系人", &contacts).Min(1).Max(5).
            Rule(func(n int) string {
                if n < 2 {
                    return "至少需要 2 位联系人"
                }
                return ""
            }).
            Body(func(row *ui.FormListRow[Contact]) {
                value := row.Value
                f.Item("姓名").Name(row.Name("name")).
                    Rules(&value.Name, ui.Required("请输入姓名")).Body(func() {
                        ui.Input(c, &value.Name)
                    })
                f.Item("手机号").Name(row.Name("phone")).
                    Rules(&value.Phone, ui.Pattern(`^1\d{10}$`, "手机号格式不正确")).Body(func() {
                        ui.Input(c, &value.Phone)
                    })
            })
    })
}
```

### 自定义新增值与按钮文字

`Creator` 为新增行返回初始值，`AddText` 修改添加按钮文案。默认新增按钮会在达到 `Max` 后禁用。

```go
import "github.com/tangoui/tango/ui"

type Endpoint struct {
    URL    string
    Method string
}

var endpoints []Endpoint

func page(c *ui.Context) {
    f := ui.Form(c)
    f.Body(func() {
        ui.FormList(f, "接口", &endpoints).
            Creator(func() Endpoint { return Endpoint{Method: "GET"} }).
            AddText("添加接口").Body(func(row *ui.FormListRow[Endpoint]) {
                value := row.Value
                f.Item("地址").Name(row.Name("url")).Body(func() {
                    ui.Input(c, &value.URL).Placeholder("https://example.com")
                })
            })
    })
}
```

### 自定义行操作

默认会显示上移、下移和删除操作；`NoActions` 隐藏它们后，可在行内容里调用 `Remove`、`MoveUp` 或 `MoveDown`。

```go
import "github.com/tangoui/tango/ui"

type Tag struct{ Name string }

var tags = []Tag{{Name: "核心"}, {Name: "实验"}}

func page(c *ui.Context) {
    f := ui.Form(c)
    f.Body(func() {
        ui.FormList(f, "标签", &tags).NoActions().Body(func(row *ui.FormListRow[Tag]) {
            value := row.Value
            ui.Row(c, func() {
                ui.Input(c, &value.Name)
                if ui.Button(c, "上移").Clicked() {
                    row.MoveUp()
                }
                if ui.Button(c, "删除").Danger().Clicked() {
                    row.Remove()
                }
            })
        })
    })
}
```

### 读取列表变化

`Changed()` 在添加、删除或移动操作改变列表的这次 `Body` 构建中返回 `true`，可用来触发摘要刷新。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

type Item struct{ Value string }

var items []Item

func page(c *ui.Context) {
    f := ui.Form(c)
    f.Body(func() {
        list := ui.FormList(f, "条目", &items).Body(func(row *ui.FormListRow[Item]) {
            ui.Input(c, &row.Value.Value)
        })
        if list.Changed() {
            ui.Message(c, "info", fmt.Sprintf("当前 %d 行", len(items)))
        }
    })
}
```

## 主题与样式

- 行内字段沿用 `Form` 的标签、错误、间距和输入控件令牌；添加按钮使用 Button 的 Dashed 变体。
- `ConfigProvider.Defaults("ProFormList", ...)` 支持 `min`、`max` 和 `addText`；链式方法可覆盖这些默认值。
- 行操作图标、按钮和焦点状态会跟随 Ant Design、Element Plus、shadcn 主题。

## 注意事项

- `FormList` 必须在 `f.Body` 中调用，且最终调用 `Body`；不要在列表回调外缓存 `FormListRow` 指针。
- 每个字段使用 `row.Name("field")`，否则多行字段会共享校验名称和错误状态。
- `Row.Value` 在回调期间有效；将它保存到异步 goroutine 中是不安全的，应在页面状态中保存结果。
- 行的稳定 key 由组件管理；不要用数组下标自己拼接持久化身份。
- 默认行操作支持键盘聚焦；自定义操作也应提供文字或 `A11yLabel`，不要只依赖颜色。

## 示例代码

<!--@include: ./_gen/formlist-examples.md-->

## API

<!--@include: ./_gen/formlist-api.md-->

## 相关链接

- [源码 ui/form_pro_list.go](https://github.com/tangoui/tango/blob/main/ui/form_pro_list.go)
- [ui Go API](/reference/api/ui)
- [Form 表单](/components/form)、[ProForm 高级表单](/components/proform)、[List 列表](/components/list)
