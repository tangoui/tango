---
title: Notification 通知提醒框
---
# Notification 通知提醒框

在窗口角落弹出的通知卡片，带标题、描述和可选的操作按钮，用于推送系统消息或较复杂的结果反馈。

## 何时使用

- 内容较多、需要标题加描述的通知，例如“版本更新”“任务完成”“同步失败及原因”。
- 需要在通知里放操作按钮（“立即重启”“查看详情”），或希望通知停留更久、悬停时暂停计时。
- 由系统主动推送、与用户当前操作无直接关联的消息。
- 一句话的操作反馈用 [Message 全局提示](/components/message)；需要用户必须处理的用 [Modal 对话框](/components/modal) 或 [Confirm 确认框](/components/confirm)；需要一个可回看的消息列表用 [NotificationCenter](/components/catalog/notificationcenter)。

## 在线体验

<GalleryPreview component="Notification" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	if ui.Button(c, "发布").Primary().Clicked() {
		ui.Notification(c, "success", "已发布").Description("版本 1.2 已上线，所有用户将在下次启动时更新。")
	}
}
```

`ui.Notification(c, status, title)` 是命令式调用：调用一次弹出一张卡片，默认出现在右上角（RTL 下左上角），4.5 秒后自动关闭，鼠标悬停时暂停计时。`status` 取 `success`、`info`、`warning`、`error`，传 `""` 不显示图标。卡片右上角有关闭按钮。

## 常见用法

### 位置

`Placement` 选择出现的角落：`ui.NotifyTopEnd`（默认）、`ui.NotifyTopStart`、`ui.NotifyBottomEnd`、`ui.NotifyBottomStart` 随 RTL 镜像；`ui.NotifyTopRight`、`ui.NotifyTopLeft`、`ui.NotifyBottomRight`、`ui.NotifyBottomLeft` 是固定的物理方位；`ui.NotifyTop` / `ui.NotifyBottom` 为顶部 / 底部居中。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	if ui.Button(c, "左下角").Clicked() {
		ui.Notification(c, "info", "通知标题").Description("这是通知的内容。").Placement(ui.NotifyBottomStart)
	}
}
```

### 时长、进度条与悬停暂停

`Duration(d)` 设置停留时间，`d <= 0` 时一直显示直到用户关闭或代码关闭；`ShowProgress()` 在卡片底部画倒计时进度条；默认悬停时暂停倒计时，`NoPause()` 取消这一行为。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	if ui.Button(c, "10 秒 + 进度条").Clicked() {
		ui.Notification(c, "warning", "磁盘空间不足").Description("剩余 2 GB，请及时清理。").
			Duration(10 * time.Second).ShowProgress()
	}
	if ui.Button(c, "不自动关闭").Clicked() {
		ui.Notification(c, "error", "同步失败").Description("网络连接已断开。").Duration(0)
	}
}
```

### 操作按钮

`Actions(fn)` 在描述下方放置按钮，按钮逻辑写在 `fn` 里；配合 `Key` 和 `ui.NotificationDestroy(c, key)` 在点击后关闭这张卡片。`OnClick(fn)` 在点击卡片本身时执行，例如跳转到详情页。

```go
import (
	"github.com/tangoui/tango/ui"
)

var restartRequested bool

func page(c *ui.Context) {
	if ui.Button(c, "检查更新").Clicked() {
		ui.Notification(c, "info", "版本更新").Description("新版本已下载完成，是否立即重启？").
			Key("upd").Duration(0).
			Actions(func() {
				if ui.Button(c, "稍后").Small().Clicked() {
					ui.NotificationDestroy(c, "upd")
				}
				if ui.Button(c, "立即重启").Small().Primary().Clicked() {
					restartRequested = true
					ui.NotificationDestroy(c, "upd")
				}
			})
	}
}
```

### 原地更新同一条

`Key` 给卡片命名，再次以同一个 key 打开时替换原卡片的内容并重新计时，适合“处理中 → 完成”这类进度通知。`ui.NotificationDestroy(c, "")` 关闭全部卡片。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	if ui.Button(c, "开始导出").Clicked() {
		ui.Notification(c, "info", "正在导出").Description("共 1,280 条记录…").Key("export").Duration(0)
	}
	if ui.Button(c, "导出完成").Clicked() {
		ui.Notification(c, "success", "导出完成").Description("文件已保存到下载目录。").Key("export")
	}
}
```

### 堆叠

同一角落的卡片超过 `ui.NotifyStackThreshold`（默认 3）张时折叠成层叠的卡片组，悬停展开（Element Plus 风格不折叠，依次排列；shadcn 风格使用 Sonner 的层叠卡片）。把 `ui.NotifyStackThreshold` 设为 0 可关闭折叠。

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 宽度 | 384 | 330 | 356（Sonner 卡片） |
| 标题 | 16px | 加粗（700），描述为次要文字色 | 14px 中等字重（500），描述为弱化文字色 |
| 边框 | 无，大阴影 | 细边框 + 普通阴影 | 细边框 |
| 堆叠 | 超过阈值折叠，悬停展开 | 不折叠 | Sonner 式层叠卡片，悬停展开 |

组件令牌在 `theme.Components.Notification`（`theme.NotificationToken`）：`Bg`、`TitleColor`、`DescriptionColor`、`TitleFontSize`、`TitleFontWeight`、`TitleLineHeight`、`Width`、`Padding`、`PaddingInlineStart`、`PaddingBlock`、`Radius`、`BorderColor`、`Shadow`。

`ConfigProvider.Defaults("Notification", ui.Props{...})` 支持：`placement`（`ui.NotifyPlacement`，或字符串 `"topLeft"`、`"bottomRight"`、`"bottomLeft"`、`"top"`、`"bottom"`，左右为逻辑方向）、`duration`（秒数或 `time.Duration`）、`showProgress`、`pauseOnHover`。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.ConfigProvider(c).Defaults("Notification", ui.Props{"placement": "bottomRight", "showProgress": true}).Body(func() {
		if ui.Button(c, "通知").Clicked() {
			ui.Notification(c, "info", "默认出现在右下角")
		}
	})
}
```

## 注意事项

- **只在事件里调用**：每调用一次就新开（或按 key 替换）一张卡片，放在只执行一次的分支里，不要每帧调用。
- **Actions 里的状态**：`Actions` 的函数在卡片显示期间每帧执行，里面引用的变量要放在包级或结构体里；关闭卡片用 `NotificationDestroy`。
- **线程**：只能在 UI 线程的帧内调用；后台任务的结果先通过 channel 交回 UI 线程（参见 [Message 的异步示例](/components/message#异步任务的结果)）。
- **无障碍**：键盘焦点可以进入卡片里的操作按钮；不自动关闭的通知要确保用户能通过关闭按钮或操作按钮关掉它。
- **RTL**：`Start` / `End` 方位随方向镜像，`Left` / `Right` 不镜像。

## 示例代码

<!--@include: ./_gen/notification-examples.md-->

## API

<!--@include: ./_gen/notification-api.md-->

## 相关链接

- [源码 ui/nav_notify.go](https://github.com/tangoui/tango/blob/main/ui/nav_notify.go)
- [Go API 参考](/reference/api/ui)
- [Message 全局提示](/components/message)、[NotificationCenter](/components/catalog/notificationcenter)
- [组件总览](/components/)
