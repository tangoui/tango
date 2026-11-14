---
title: Modal 对话框
---
# Modal 对话框

模态对话框：在当前页面上方打开一个浮层并用遮罩挡住页面，要求用户先处理对话框里的内容（确认、填写、查看），处理完再回到原页面。

## 何时使用

- 需要用户处理事务，又不希望跳转页面打断工作流时，例如新建 / 编辑一条记录、查看详情、二次确认带说明的操作。
- 只需要一句话确认（“确定删除吗？”）时，用命令式的 [Confirm 确认框](/components/confirm) 更省事，不必自己维护打开状态；只需要告知结果时用 [MessageBox](/components/catalog/messagebox)。
- 内容较多、需要和页面对照着看（例如长表单、详情面板），用 [Drawer 抽屉](/components/drawer)。
- 轻量的、不打断操作的反馈用 [Message 全局提示](/components/message) 或 [Notification 通知提醒框](/components/notification)；就地的小确认用 [Popconfirm](/components/catalog/popconfirm)。

## 在线体验

<GalleryPreview component="Modal" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	open   bool
	result string
)

func page(c *ui.Context) {
	if ui.Button(c, "打开对话框").Primary().Clicked() {
		open = true
	}
	m := ui.Modal(c, &open, "基础对话框").Body(func() {
		ui.Text(c, "对话框的内容……")
		ui.Text(c, "支持任意组件。")
	})
	if m.Ok() {
		result = "点了确定"
		open = false // 确定不会自动关闭，由你决定何时关
	}
	if m.Cancelled() {
		result = "取消了" // 取消 / 关闭按钮 / 遮罩 / Esc 已经把 open 置为 false
	}
	ui.Text(c, result).Secondary()
}
```

`ui.Modal(c, &open, title)` 只在 `open` 为 `true` 时显示，先链式设置选项，最后调用 `Body` 构建内容。`Body` 之后读取本帧的结果：

- `Ok()`：本帧点击了“确定”。对话框**不会**自动关闭，校验、保存成功后把 `open` 设为 `false` 才关（对应 antd 的 `onOk` 里自己 `setOpen(false)`）。
- `Cancelled()`：本帧点击了“取消”、右上角关闭按钮、遮罩，或按了 Esc。这些情况下组件已经把 `open` 置为 `false`。

## 常见用法

### 打开与关闭状态

打开状态就是你传入的 `*bool`，组件只读写这一个变量：

- 打开：在事件分支里把它设为 `true`（按钮、菜单项、表格行操作……）。
- 关闭：取消类操作由组件自动写 `false`；确定、自定义按钮、异步完成等由你写 `false`。
- 关闭后有淡出动画，动画期间对话框仍挡住页面，但按钮、Esc 和遮罩不再响应，`Ok()` / `Cancelled()` 都返回 `false`，不会重复触发。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	editOpen bool
	editing  string
)

func page(c *ui.Context) {
	for _, name := range []string{"Alice", "Bob"} {
		ui.Key(c, name, func() {
			if ui.Button(c, "编辑 "+name).Clicked() {
				editing, editOpen = name, true
			}
		})
	}
	// 标题保持不变：对话框的内部状态（动画、拖动位置）按标题区分。
	m := ui.Modal(c, &editOpen, "编辑用户").Body(func() {
		ui.Text(c, "正在编辑："+editing)
	})
	if m.Ok() {
		editOpen = false
	}
}
```

如果不希望点遮罩就关闭（例如表单填了一半），用 `MaskClosable(false)`；不要右上角关闭按钮用 `Closable(false)`。Esc 和“取消”按钮仍然可以关闭。

### 确认与取消：按钮文字与危险操作

`OkText` / `CancelText` 改按钮文字（默认取当前语言包的“确定” / “取消”），`Danger()` 把确定按钮变成危险样式，用于删除、清空之类不可恢复的操作。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	delOpen bool
	items   = []string{"a.txt", "b.txt"}
)

func page(c *ui.Context) {
	if ui.Button(c, "清空列表").Danger().Clicked() {
		delOpen = true
	}
	m := ui.Modal(c, &delOpen, "清空列表").OkText("清空").CancelText("再想想").Danger().
		Body(func() {
			ui.Text(c, "将删除全部条目，此操作不可恢复。")
		})
	if m.Ok() {
		items = nil
		delOpen = false
	}
}
```

### 异步关闭

确定后要等网络请求或磁盘写入完成再关闭时，用 `ConfirmLoading(true)` 让确定按钮显示加载状态并阻止重复点击。耗时操作放到 goroutine 里，结果通过 channel 交回 UI 线程，用 `c.Invalidator()` 返回的函数请求下一帧（它可以在任意 goroutine 调用）。

```go
import (
	"errors"
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	submitOpen bool
	submitting bool
	submitDone = make(chan error, 1)
)

func submit() error {
	time.Sleep(time.Second) // 模拟网络请求
	return errors.New("服务器繁忙")
}

func page(c *ui.Context) {
	if ui.Button(c, "提交审核").Primary().Clicked() {
		submitOpen = true
	}
	m := ui.Modal(c, &submitOpen, "提交审核").
		ConfirmLoading(submitting).
		Closable(!submitting).MaskClosable(!submitting).
		Body(func() {
			ui.Text(c, "提交后两秒内关闭。")
		})
	if m.Ok() && !submitting {
		submitting = true
		wake := c.Invalidator()
		go func() {
			submitDone <- submit()
			wake()
		}()
	}
	select {
	case err := <-submitDone:
		submitting = false
		if err != nil {
			ui.Message(c, "error", "提交失败："+err.Error())
		} else {
			submitOpen = false
			ui.Message(c, "success", "已提交")
		}
	default:
	}
}
```

加载中确定按钮不会再触发 `Ok()`；示例同时关掉了右上角关闭和遮罩关闭，避免请求未完成就关闭。默认页脚的“取消”和 Esc 仍可关闭对话框，需要完全禁止时改用自定义页脚（见下一节）。命令式的 [Confirm](/components/confirm#异步关闭) 也支持同样的异步关闭。

### 自定义页脚

`Footer(fn)` 用你自己的按钮替换默认的“取消 / 确定”，此时 `Ok()` 不再触发（默认按钮不存在），按钮逻辑写在 `fn` 里；右上角关闭、遮罩和 Esc 仍会触发 `Cancelled()` 并关闭。`NoFooter()` 去掉整个页脚，适合纯展示或按钮放在内容里的场景。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	draftOpen bool
	status    string
)

func page(c *ui.Context) {
	if ui.Button(c, "离开编辑器").Clicked() {
		draftOpen = true
	}
	ui.Modal(c, &draftOpen, "有未保存的修改").Centered().
		Footer(func() {
			if ui.Button(c, "不保存").Danger().Clicked() {
				status, draftOpen = "已丢弃", false
			}
			if ui.Button(c, "取消").Clicked() {
				draftOpen = false
			}
			if ui.Button(c, "保存").Primary().Clicked() {
				status, draftOpen = "已保存", false
			}
		}).
		Body(func() {
			ui.Text(c, "离开前是否保存对文档的修改？")
		})
	ui.Text(c, status).Secondary()
}
```

页脚是右对齐、间距 8px 的一行，按钮顺序就是你写的顺序。

### 表单弹窗

对话框里放 [Form](/components/form) 时，在 `Body` 里构建表单并把 `*ui.FormW` 留到外面；`Body` 返回后在 `Ok()` 分支里调用 `Validate()`，校验通过才保存和关闭。打开时清空上一次的输入，并用 `ResetValidation()` 清掉上一次的错误提示。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	userOpen bool
	user     struct{ name, email string }
	users    []string
)

func page(c *ui.Context) {
	var form *ui.FormW
	if ui.Button(c, "新建用户").Primary().Clicked() {
		user.name, user.email = "", ""
		userOpen = true
	}
	m := ui.Modal(c, &userOpen, "新建用户").OkText("创建").MaskClosable(false).Body(func() {
		form = ui.Form(c).Vertical()
		form.Body(func() {
			form.Item("用户名").Rules(&user.name, ui.Required("请输入用户名"), ui.MinLen(3, "至少 3 个字符")).Body(func() {
				ui.Input(c, &user.name).Placeholder("请输入用户名")
			})
			form.Item("邮箱").Rules(&user.email, ui.Required("请输入邮箱"), ui.Email("邮箱格式不正确")).Body(func() {
				ui.Input(c, &user.email).Placeholder("name@example.com")
			})
		})
	})
	if m.Ok() && form != nil && form.Validate() {
		users = append(users, user.name)
		userOpen = false
	}
	if m.Cancelled() && form != nil {
		form.ResetValidation()
	}
}
```

字段多、需要 `OnFinish` 回调和提交中状态时，可直接使用 Pro 组件 [ModalForm](/components/catalog/modalform)（`ui.ModalForm(c, &open, title, fields)`），它把对话框、表单、校验和提交流程打包在一起；抽屉形式用 `ui.DrawerForm`。

### 位置、宽度、拖动与全屏

- `Width(px)`：对话框宽度（Ant Design 默认 520；Element Plus 默认窗口宽度的 50%，至少 320）。
- `Centered()`：垂直居中；`Top(px)`：离窗口顶部的距离（Ant Design 默认 100，Element Plus 默认窗口高度的 15%）。
- `Draggable()`：按住标题栏拖动；拖动范围限制在窗口内，重新打开时回到原位。
- `FullscreenToggle()` 在标题栏加最大化按钮，`Fullscreen(&full)` 绑定全屏状态（也可以只用它从代码控制全屏）。
- `Mask(false)`：不画暗色遮罩，但页面仍被挡住、不可操作。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	logOpen bool
	logFull bool
)

func page(c *ui.Context) {
	if ui.Button(c, "查看日志").Clicked() {
		logOpen = true
	}
	ui.Modal(c, &logOpen, "构建日志").Width(720).Draggable().
		FullscreenToggle().Fullscreen(&logFull).NoFooter().
		Body(func() {
			ui.Text(c, "按住标题栏拖动；右上角按钮切换全屏。")
		})
}
```

### 焦点与 Esc

对话框打开时是一个焦点陷阱：键盘焦点移入对话框，Tab / Shift+Tab 只在对话框内循环，读屏软件看不到下层页面；关闭后焦点回到打开前聚焦的控件（通常就是触发按钮）。Esc 等同于取消，嵌套浮层时只关闭最上层（例如对话框里展开的下拉框先收起）。这些都是内置行为，不需要额外代码。

对话框的无障碍角色是 `dialog`，名称取自标题，状态带 `modal`；读屏软件的“关闭”动作同样走取消逻辑。

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 默认宽度 | 520 | 窗口宽度 50%（最少 320） | 520 |
| 默认位置 | 距顶部 100 | 距顶部 15% 窗口高度 | 始终居中（设置了 `Top` 时除外） |
| 标题 | 16px 加粗 | 18px 常规字重 | 18px 半粗（600） |
| 内边距 | 左右 24、上下 20 | 16 | 24 |
| 其他 | 大圆角（RadiusLG） | 普通圆角（Radius） | 关闭按钮无悬停底色，图标 70% 不透明度 |

组件令牌在 `theme.Components.Modal`（`theme.ModalToken`），同时作用于 Confirm / MessageBox / Prompt：`ContentBg`、`HeaderBg`、`FooterBg`、`TitleColor`、`TitleFontSize`、`TitleFontWeight`、`Padding`、`PaddingBlock`、`HeaderMarginBottom`、`FooterMarginTop`、`CloseIconColor`、`Radius`、`BorderColor`、`MaskBg`、`Shadow`。用 `ui.ConfigProvider(c).Components(theme.Components{Modal: theme.ModalToken{Radius: 12}})` 在一个作用域内覆盖，零值字段继承主题。

`ConfigProvider.Defaults("Modal", ui.Props{...})` 支持的键：`width`、`centered`、`closable`、`mask`、`maskClosable`、`okText`、`cancelText`；链式方法优先。

```go
import (
	"github.com/tangoui/tango/ui"
)

var open bool

func page(c *ui.Context) {
	ui.ConfigProvider(c).Defaults("Modal", ui.Props{"width": 640, "centered": true, "okText": "好的"}).Body(func() {
		if ui.Button(c, "打开").Clicked() {
			open = true
		}
		ui.Modal(c, &open, "全局默认值").Body(func() { ui.Text(c, "宽 640、垂直居中。") })
	})
}
```

## 注意事项

- **状态放在哪**：打开状态和表单数据放在包级变量、结构体字段或闭包里，不能是页面函数内的局部变量（每帧都会重新执行页面函数）。
- **读取时机**：`Ok()` / `Cancelled()` 在 `Body` 执行时才计算，必须在 `Body` 之后读取。对话框关闭（且淡出动画结束）时 `Body` 不会构建内容，两者都是 `false`。
- **标题即身份**：对话框的内部状态（动画、拖动偏移、全屏）由“所在作用域 + 标题”标识。同一作用域里两个标题相同的对话框会共用状态，打开期间修改标题会让对话框重新播放打开动画；动态内容放进 `Body`，或用 `ui.Key(c, key, fn)` 包住对话框。
- **命令式反馈**：在 `Ok()` 分支里调用 `ui.Message` / `ui.Notification` 只会执行一次，不要在每帧都会执行的位置调用它们。
- **后台任务**：goroutine 里不要直接改界面状态，也不要调用 `ui.*` 函数；把结果交回 UI 线程（channel 加 `c.Invalidator()`），参见上面的异步关闭示例。
- **RTL**：对话框内的行布局随 `ConfigProvider.Direction` 镜像（标题在右、关闭按钮在左，页脚按钮靠左）。

## 示例代码

<!--@include: ./_gen/modal-examples.md-->

## API

<!--@include: ./_gen/modal-api.md-->

## 相关链接

- [源码 ui/feedback.go](https://github.com/tangoui/tango/blob/main/ui/feedback.go)
- [Go API 参考](/reference/api/ui)
- [Confirm 确认框](/components/confirm)、[Drawer 抽屉](/components/drawer)、[ModalForm](/components/catalog/modalform)、[Popconfirm](/components/catalog/popconfirm)
- [组件总览](/components/)
