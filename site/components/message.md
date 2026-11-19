---
title: Message 全局提示
---
# Message 全局提示

用几秒钟的轻量提示反馈保存成功、网络错误、正在加载等状态。Ant Design 和 Element Plus 默认在窗口顶部居中显示，shadcn 默认在底部结束角显示。

## 何时使用

- 对用户操作的即时、简短反馈：成功、失败、警告、普通信息，以及“正在处理”的加载提示。
- 内容需要标题加描述、带操作按钮或需要停留较久时，用 [Notification 通知提醒框](/components/notification)。
- 需要用户确认或作出选择时，用 [Confirm 确认框](/components/confirm) 或 [Modal 对话框](/components/modal)；需要常驻在页面内的提示用 [Alert](/components/alert)。

## 在线体验

<GalleryPreview component="Message" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Row(c, func() {
		if ui.Button(c, "成功").Clicked() {
			ui.Message(c, "success", "这是一条成功提示")
		}
		if ui.Button(c, "错误").Clicked() {
			ui.Message(c, "error", "这是一条错误提示")
		}
		if ui.Button(c, "警告").Clicked() {
			ui.Message(c, "warning", "这是一条警告提示")
		}
		if ui.Button(c, "普通").Clicked() {
			ui.Message(c, "info", "这是一条普通提示")
		}
	}, layout.Gap(8))
}
```

`ui.Message(c, status, content)` 是命令式调用：调用一次就弹出一条，3 秒后自动消失，不需要保存任何状态。`status` 取 `success`、`info`、`warning`、`error`、`loading`。多条提示会依次堆叠。

## 常见用法

### 加载中与原地更新

`Key` 给提示命名：屏幕上已有同名提示时，新调用会原地更新它的内容、状态并重新计时，而不是再叠一条。`Duration(0)` 让提示一直显示，直到被同名更新替换（新提示的时长为正）或调用 `ui.MessageDestroy(c, key)`。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Row(c, func() {
		if ui.Button(c, "开始保存").Clicked() {
			ui.Message(c, "loading", "正在保存…").Key("save").Duration(0)
		}
		if ui.Button(c, "保存完成").Clicked() {
			ui.Message(c, "success", "保存成功").Key("save") // 替换上一条，3 秒后消失
		}
		if ui.Button(c, "全部关闭").Clicked() {
			ui.MessageDestroy(c, "") // 空 key 关闭全部
		}
	}, layout.Gap(8))
}
```

### 异步任务的结果

后台任务完成后要弹提示，先把结果交回 UI 线程，再在页面函数里调用 `ui.Message`（`ui.*` 函数只能在 UI 线程、帧内调用）。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	syncing  bool
	syncDone = make(chan error, 1)
)

func page(c *ui.Context) {
	if ui.Button(c, "同步").Loading(syncing).Clicked() {
		syncing = true
		ui.Message(c, "loading", "正在同步…").Key("sync").Duration(0)
		wake := c.Invalidator()
		go func() {
			time.Sleep(time.Second) // 模拟耗时操作
			syncDone <- nil
			wake()
		}()
	}
	select {
	case err := <-syncDone:
		syncing = false
		if err != nil {
			ui.Message(c, "error", "同步失败").Key("sync")
		} else {
			ui.Message(c, "success", "同步完成").Key("sync")
		}
	default:
	}
}
```

### 时长与关闭按钮

`Duration(d)` 设置停留时间，`d <= 0` 表示不自动关闭；`ShowClose()` 加一个关闭按钮，常和 `Duration(0)` 搭配，用于需要用户看到再关闭的提示。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	if ui.Button(c, "长提示").Clicked() {
		ui.Message(c, "info", "这条提示停留 10 秒").Duration(10 * time.Second)
	}
	if ui.Button(c, "可关闭").Clicked() {
		ui.Message(c, "warning", "带关闭按钮，不会自动消失").ShowClose().Duration(0)
	}
}
```

### 位置、偏移与朴素样式

`Placement` 选择出现位置，可设为 `ui.MessageTop`、`ui.MessageTopStart`、`ui.MessageTopEnd`、`ui.MessageBottom`、`ui.MessageBottomStart` 或 `ui.MessageBottomEnd`。各位置分别堆叠，底部位置向上堆叠。Ant Design 和 Element Plus 中，最新一条提示的 `Offset(px)` 决定该堆栈到窗口边缘的距离。shadcn 的 Sonner 堆栈使用自己的边距。`Plain()` 使用中性底色，图标保留状态色。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	if ui.Button(c, "右下角").Clicked() {
		ui.Message(c, "info", "消息出现在右下角").Placement(ui.MessageBottomEnd)
	}
	if ui.Button(c, "偏移 80").Clicked() {
		ui.Message(c, "success", "离顶部 80 像素").Offset(80)
	}
	if ui.Button(c, "朴素样式").Clicked() {
		ui.Message(c, "success", "朴素样式的成功提示").Plain()
	}
}
```

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 默认位置 | 顶部居中 | 顶部居中 | 右下角（Sonner 堆叠卡片） |
| 外观 | 白底卡片 + 彩色图标 | 状态色浅底 + 状态色边框（`Plain()` 为中性底色） | 宽 356 的卡片，细边框，阴影 |

组件令牌在 `theme.Components.Message`（`theme.MessageToken`）：`ContentBg`、`Color`、`PaddingBlock`、`PaddingInline`、`Width`、`Radius`、`BorderColor`、`Shadow`、`Top`（默认离窗口边缘的距离）、`MarginInline`、`CloseColor`，以及 `Plain()` 使用的 `PlainBg`、`PlainColor`、`PlainBorderColor`、`PlainShadow`。

`ConfigProvider.Defaults("Message", ui.Props{...})` 支持 `duration`（秒数或 `time.Duration`，`<= 0` 持续显示）、`maxCount`（超过上限时移除最早的提示）和 `pauseOnHover`（默认 `true`）。包级变量 `ui.MessageDuration` 控制全局默认时长。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.ConfigProvider(c).Defaults("Message", ui.Props{"duration": 5, "maxCount": 3}).Body(func() {
		if ui.Button(c, "提示").Clicked() {
			ui.Message(c, "info", "停留 5 秒，最多同时 3 条")
		}
	})
}
```

## 注意事项

- **只在事件里调用**：`ui.Message` 每调用一次就新开一条提示。把它放在 `if ...Clicked()`、`Ok()`、channel 收到结果等只执行一次的分支里，不要放在每帧都会执行的位置。
- **返回值不用保存**：`*ui.MessageRef` 只用于同一条链式调用；以后要更新或关闭，用 `Key` 和 `ui.MessageDestroy`。
- **线程**：只能在 UI 线程的帧内调用；后台 goroutine 通过 channel 加 `c.Invalidator()` 把结果交回来，见上面的示例。
- **无障碍**：`error` / `warning` 以 alert 角色立即播报，其余以 status 角色礼貌播报。悬停默认暂停计时，单条提示可用 `PauseOnHover(false)` 关闭。
- **RTL**：`Start` / `End` 位置随方向镜像。

## 示例代码

<!--@include: ./_gen/message-examples.md-->

## API

<!--@include: ./_gen/message-api.md-->

## 相关链接

- [源码 ui/nav_notify.go](https://github.com/tangoui/tango/blob/main/ui/nav_notify.go)
- [Go API 参考](/reference/api/ui)
- [Notification 通知提醒框](/components/notification)、[Alert](/components/alert)、[Confirm 确认框](/components/confirm)
- [组件总览](/components/)
