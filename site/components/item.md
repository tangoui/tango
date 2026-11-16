---
title: Item 列表项
---
# Item 列表项

通用的“一行”展示块（对标 shadcn/ui 的 Item）：行首是图标块或头像等媒体，中间是标题加描述，行尾是操作；有 default、outline、muted 三种底色，default、sm 两种密度，整行可以当链接或按钮点击。`ItemGroup` 把多个 Item 叠成一组，可加分隔线和外框。

## 何时使用

- 设置页、详情页里的条目：“安全设置 / 通知偏好 / 存储空间”这类图标 + 标题 + 描述 + 操作的行，或者“个人资料 >”这种整行可点的入口。
- 数量不多、每项内容和样式各不相同，需要自己拼装媒体、附加内容和操作。
- 和 [List 列表](/components/list) 的区别：`List` 是数据驱动的列表容器，按条目数量逐项回调，自带页头页脚、分页、加载更多、栅格布局；`Item` 只是一行的外观，没有数据、分页和虚拟化，由你自己在 `for` 循环里创建。需要分页或大量数据用 `List`（海量数据用 `VirtualList`，Pro 规范的列表用 `ProList`）；只有几项、或需要 shadcn 风格的行用 `Item` / `ItemGroup`。
- 带选中状态的导航用 [Menu 导航菜单](/components/menu)；有统一列结构、需要排序筛选的数据用 [Table 表格](/components/table)。

## 在线体验

<GalleryPreview component="Item" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

func page(c *ui.Context) {
	ui.Item(c, "安全设置").
		Description("双重验证已开启，最近一次登录在 2 小时前。").
		Icon(icon.Lock).
		Actions(func() { ui.Button(c, "管理").Small() })
}
```

`ui.Item(c, title)` 创建一行；链式调用 `Description`（标题下的次要文字，最多两行）、`Icon`（行首的图标块）、`Actions`（行尾的控件）等。这一行在链的末尾（调用 `Clicked`，或者构建结束时）真正画出来。

## 常见用法

以下片段省略了 `ui`、`ui/icon`、`ui/layout` 等常用 `import`；用到额外的包时会写出。

### 变体和尺寸

`Variant` 选择底色：`ui.ItemVariantDefault` 无底色，`ui.ItemVariantOutline`（简写 `Outline()`）带边框，`ui.ItemVariantMuted`（简写 `Muted()`）带浅色底。`Size` 选择密度：`ui.ItemSizeDefault` 或 `ui.ItemSizeSmall`（简写 `Small()`，内边距和间距更小）。

```go
func variants(c *ui.Context) {
	ui.Column(c, func() {
		ui.Item(c, "基础项").Description("没有底色与边框的简单列表项。").Icon(icon.File)
		ui.Item(c, "安全设置").Description("已开启双重验证。").Icon(icon.Lock).Outline()
		ui.Item(c, "存储空间").Description("已使用 18.4 GB / 50 GB。").Icon(icon.Folder).Muted().
			Actions(func() { ui.Button(c, "升级").Small().Primary() })
		ui.Item(c, "紧凑的行").Description("sm 密度。").Icon(icon.Bell).Outline().Small()
	}, layout.Gap(12), layout.W(560))
}
```

### 头像等自定义媒体和附加内容

`Media` 用任意控件（头像、图片）代替图标块；`Content` 在描述下面追加内容，如标签、进度条。

```go
func rich(c *ui.Context) {
	ui.Item(c, "张三").Description("zhangsan@example.com").Outline().
		Media(func() { ui.Avatar(c, "张").Size(36) }).
		Content(func() { ui.Progress(c, 60) }).
		Actions(func() { ui.Tag(c, "管理员") })
}
```

### 整行可点击：按钮与链接

`Clickable()` 让整行成为按钮：可聚焦、悬停高亮，点击或 Enter / 空格时 `Clicked()` 返回 `true`；`Link()` 同样可点，但向辅助技术声明为链接，由你在 `Clicked()` 里导航。`Disabled(true)` 让行变暗并停止响应。

```go
var lastClicked string

func clickable(c *ui.Context) {
	ui.Column(c, func() {
		if ui.Item(c, "个人资料").Description("姓名、头像与联系方式").Outline().Link().
			Media(func() { ui.Avatar(c, "张").Size(36) }).
			Actions(func() { ui.Icon(c, icon.Right) }).Clicked() {
			lastClicked = "个人资料"
		}
		if ui.Item(c, "通知偏好").Icon(icon.Bell).Outline().Small().Clickable().
			Actions(func() { ui.Icon(c, icon.Right) }).Clicked() {
			lastClicked = "通知偏好"
		}
		ui.Item(c, "已停用的功能").Description("当前套餐不可用").Icon(icon.Star).Outline().Clickable().Disabled(true)
		ui.Text(c, "最近点击："+lastClicked).Tertiary()
	}, layout.Gap(12), layout.W(560))
}
```

不调用 `Clickable()` / `Link()` 的 Item 是静态行，`Clicked()` 永远返回 `false`。行内的按钮等控件仍可正常点击，但整行可点击时不建议再放可交互的操作，以免两者争抢点击。

### ItemGroup：成组、分隔线与外框

`ItemGroup` 把一组 Item 作为列表：`Separated()` 在相邻项之间画分隔线，`Bordered()` 给整组加圆角外框，`Gap(px)` 设置项间距，`A11yLabel` 给列表命名。用循环创建时给每项套 `ui.Key`。

```go
type member struct{ Name, Mail, Role string }

var members = []member{
	{"王小明", "xiaoming@example.com", "管理员"},
	{"李华", "lihua@example.com", "成员"},
	{"陈静", "chenjing@example.com", "访客"},
}

func group(c *ui.Context) {
	ui.ItemGroup(c).Separated().Bordered().A11yLabel("团队成员").Body(func() {
		for _, m := range members {
			ui.Key(c, m.Name, func() {
				ui.Item(c, m.Name).Description(m.Mail).Small().
					Media(func() { ui.Avatar(c, string([]rune(m.Name)[:1])).Size(32) }).
					Actions(func() { ui.Text(c, m.Role).Tertiary() })
			})
		}
	})
}
```

`ItemGroup` 的 `Body` 会把子元素标为列表项，所以组内的静态 Item 在无障碍树里是 `listitem`；组外的静态 Item 是 `group`。

### 用 ConfigProvider 设置默认值

`ConfigProvider.Defaults("Item", ...)` 的 `variant`（`"default"`、`"outline"`、`"muted"`）和 `size`（`"default"`、`"sm"`）作用于作用域内所有 Item；`Defaults("ItemGroup", ...)` 的 `split`、`bordered` 控制组的分隔线和外框。链式方法优先于默认值。

```go
func defaults(c *ui.Context) {
	ui.ConfigProvider(c).
		Defaults("Item", ui.Props{"variant": "outline", "size": "sm"}).
		Defaults("ItemGroup", ui.Props{"split": true, "bordered": true}).
		Body(func() {
			ui.ItemGroup(c).Body(func() {
				ui.Item(c, "通知").Description("接收站内通知")
				ui.Item(c, "隐私").Description("管理可见范围")
			})
		})
}
```

## 主题与样式

三套风格都有对应的外观：shadcn 主题对照 shadcn/ui 的 Item（图标块带边框和 muted 底色，muted 变体为半透明底，链接行悬停为半透明 accent）；Element Plus 主题使用较小的圆角和 500 的标题字重；Ant Design 主题的圆角取 `RadiusLG`，图标块用主色浅底。组件令牌是 `theme.Components.Item`（`ItemToken`）：`Radius`、`PaddingBlock` / `PaddingInline`（及 `SM` 变体）、`Gap`、`ContentGap`、`TitleFontSize`、`TitleColor`、`DescriptionColor`、`BorderColor`、`MutedBg`、`HoverBg`、`MediaIcon*`、`ActionsGap`、`GroupSeparator`、`GroupSeparatorInset` 等，可通过 `ui.ConfigProvider(c).Components(...)` 局部覆盖。

## 注意事项

- **即时模式与链式调用**：Item 在链的末尾才构建。需要 `Clicked()` 时把它接在链的最后；不关心点击时不必调用。
- **Key**：在循环里创建 Item / ItemGroup 的子项时，用 `ui.Key(c, 稳定的键, func(){...})` 包裹，行内状态（悬停动画、焦点）才不会在增删项后串位。
- **无障碍**：静态行的名称是标题、描述作为描述；可点击的行整体是一个按钮或链接，名称取标题（可用 `A11yLabel` 覆盖），描述同样挂在节点上；行内的标题和描述文字对辅助技术隐藏以免重复朗读，所以可点击行里不要再依赖文字节点。禁用的可点击行不可聚焦。
- **键盘**：Clickable / Link 行可 Tab 聚焦，聚焦时绘制焦点环，Enter / 空格触发。
- **布局**：Item 是横向的行，会铺满父容器的宽度；放在 `Column` 里并用 `layout.W` 或外层容器限制宽度。描述最多两行，超出被截断。

## 示例代码

<!--@include: ./_gen/item-examples.md-->

## API

<!--@include: ./_gen/item-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/item.go)
- [Go API 参考](/reference/api/ui)
- [List 列表](/components/list)、[Avatar 头像](/components/avatar)、[Menu 导航菜单](/components/menu)
