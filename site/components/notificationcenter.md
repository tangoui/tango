---
title: NotificationCenter 通知中心
---
# NotificationCenter 通知中心

标题栏或状态栏里的铃铛按钮：徽标显示未读数，点开后在气泡面板里按分组列出通知，可标为已读、清空、逐条移除，也可以在通知上放操作按钮。

## 何时使用

- 需要一个可以回看的通知收件箱：构建结果、同步状态、更新提示、任务完成等，用户不必在弹出的瞬间处理，稍后打开铃铛再看。
- 通知需要“未读 / 已读”状态、按“今天 / 更早”分组、带操作按钮（“打开文件”“立即重启”）。
- 它和 [Notification 通知提醒框](/components/notification)、[Message 全局提示](/components/message) 是不同层次：Message 是一句话的瞬时反馈，Notification 是在窗口角落弹出、几秒后消失的卡片，二者都不保留历史；NotificationCenter 是常驻的列表，数据由应用自己持有。常见做法是同一个事件既弹出 Notification，又追加到 NotificationCenter 的列表里。
- 只是想提示一个状态点或数字，用 [Badge 徽章](/components/badge)；成块的、常驻页面内的提示用 [Alert](/components/alert)。

## 在线体验

<GalleryPreview component="NotificationCenter" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var notes = []ui.NotificationItem{
	{Key: "build", Group: "今天", Level: "error", Title: "构建失败", Body: "ui/titlebar.go:42 未定义的标识符 frame。", Time: time.Now().Add(-2 * time.Minute)},
	{Key: "update", Group: "今天", Level: "info", Title: "有可用更新", Body: "重启后安装。", Time: time.Now().Add(-40 * time.Minute)},
	{Key: "push", Group: "更早", Level: "success", Title: "推送完成", Time: time.Now().Add(-26 * time.Hour), Read: true},
}

func page(c *ui.Context) {
	ui.NotificationCenter(c, &notes)
}
```

`ui.NotificationCenter(c, &notes)` 接收切片的**指针**：铃铛上的徽标显示 `Read == false` 的条数；点开后面板按 `Group` 分节，节的先后取决于各分组在切片中第一次出现的位置，节内顺序就是切片顺序，所以要“最新在前”请自己把新通知插到切片开头。每条通知的 `Key` 必须稳定且唯一，面板用它保存每一行的状态。点击通知或其中的操作会把它标为已读并关闭面板；标题栏的按钮可以“全部标为已读”或“清空”，每行末尾的关闭按钮移除单条。这些修改都直接作用在你的切片上。

## 常见用法

以下片段沿用基本用法里的 `notes` 数据，省略了 `ui` 等常用 `import`；用到额外的包时会写出。

### 响应点击和操作按钮

`Clicked()` 返回被点击通知的 `Key`；通知上声明了 `Actions` 时，`Action()` 返回通知 `Key` 和被点的操作 `Key`。两者都只在发生的那一帧返回 `ok == true`，同一帧里读取。

```go
var lastOpened string

var actionNotes = []ui.NotificationItem{
	{
		Key: "build", Group: "今天", Level: "error", Title: "构建失败",
		Actions: []ui.NotificationAction{
			{Key: "open", Label: "打开文件", Primary: true},
			{Key: "log", Label: "查看日志"},
		},
	},
}

func withActions(c *ui.Context) {
	nc := ui.NotificationCenter(c, &actionNotes)
	if key, ok := nc.Clicked(); ok {
		lastOpened = key
	}
	if key, act, ok := nc.Action(); ok {
		lastOpened = key + " → " + act
	}
}
```

### 新通知：追加到列表并弹出提醒

面板的数据是应用自己的切片，往里追加就是“收到新通知”。需要即时提示时，可以同时调用 `ui.Notification` 弹出角落卡片；用户错过卡片也能在铃铛里找到。

```go
import "fmt"

var seq int

func notify(c *ui.Context, title string) {
	seq++
	n := ui.NotificationItem{
		Key: fmt.Sprintf("job-%d", seq), Group: "今天", Level: "success",
		Title: title, Time: c.Now(),
	}
	notes = append([]ui.NotificationItem{n}, notes...) // 最新在前
	ui.Notification(c, "success", title)
}

func pushDemo(c *ui.Context) {
	if ui.Button(c, "模拟一条通知").Clicked() {
		notify(c, "任务完成")
	}
	ui.NotificationCenter(c, &notes)
}
```

### 保存已读状态

标记已读、清空、移除都会让 `Changed()` 在当次帧返回 `true`，据此把切片写回磁盘或服务端。

```go
func persist(c *ui.Context, save func([]ui.NotificationItem)) {
	nc := ui.NotificationCenter(c, &notes)
	if nc.Changed() {
		save(notes)
	}
}
```

### 面板位置、宽度与标题

默认面板在铃铛下方、右对齐（`"bottomRight"`）。铃铛放在窗口底部状态栏时用 `Placement("topRight")` 让面板向上展开；`Width` 调整面板宽度，`Title` 替换面板标题（默认是本地化的“通知”）。

```go
func statusBar(c *ui.Context) {
	ui.NotificationCenter(c, &notes).
		Placement("topRight").
		Width(400).
		Title("消息")
}
```

### 受控的打开状态

`Open(&open)` 把面板的开合绑定到你的布尔变量，可以由快捷键或菜单打开面板；`Unread()` 返回未读数，`IsOpen()` 返回当前是否展开。

```go
var centerOpen bool

func controlled(c *ui.Context) {
	if ui.Button(c, "打开通知").Clicked() {
		centerOpen = true
	}
	nc := ui.NotificationCenter(c, &notes).Open(&centerOpen)
	if nc.Unread() > 0 && !nc.IsOpen() {
		ui.Text(c, "有未读通知").Tertiary()
	}
}
```

## 主题与样式

三套风格共用同一结构，差别在细节：Element Plus 在条目之间画分隔线，面板头的下边线用次级边框色；shadcn 的标题字重为 500，面板头下边线使用边框色，次要文字使用 muted 前景色。组件令牌是 `theme.Components.NotificationCenter`（`NotificationCenterToken`），包括 `PanelWidth`（默认 360）、`PanelMaxHeight`（默认 420，超出后列表滚动）、`TriggerSize`、`ItemHoverBg`、`ItemUnreadBg`、`UnreadDot`、`UnreadDotSize` 等，可通过 `ui.ConfigProvider(c).Components(...)` 局部覆盖。

`Level` 的图标取自主题的成功、警告、错误、信息色：`"info"`、`"success"`、`"warning"`、`"error"`，留空则不显示图标。未读条目标题加粗并在行首显示小圆点。

## 注意事项

- **数据归应用所有**：NotificationCenter 不会自动收集 `ui.Notification` 弹出的卡片，也不会持久化；它只读写你传入的切片。`Key` 缺失或重复会导致行状态串位。
- **指针与切片**：控件会直接修改切片（`Read`、删除、清空时把切片截为零长度）。从多个 goroutine 追加通知时，请先把事件交回 UI 线程再改切片。
- **相对时间**：`Time` 以“3 分钟前”这类相对时间显示，基准是帧时间 `c.Now()`，零值则不显示时间。
- **关闭方式**：面板是 Popover，按 Esc 或点击面板外部关闭；点击通知或操作后也会自动关闭。
- **无障碍**：铃铛按钮的名称是 `Locale.A11y.Notifications`；面板是列表，每条通知是一个按钮，描述中包含未读状态、正文和时间，关闭按钮的名称带有通知标题。面板文案（全部标为已读、清空、移除、空状态）取自当前 `Locale.NotificationCenter`。

## 示例代码

<!--@include: ./_gen/notificationcenter-examples.md-->

## API

<!--@include: ./_gen/notificationcenter-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/notification_center.go)
- [Go API 参考](/reference/api/ui)
- [Notification 通知提醒框](/components/notification)、[Message 全局提示](/components/message)、[Badge 徽章](/components/badge)、[Popover 气泡卡片](/components/popover)
