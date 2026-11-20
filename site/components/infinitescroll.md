---
title: InfiniteScroll 无限滚动
---
# InfiniteScroll 无限滚动

在滚动内容接近底部时通知应用加载更多数据，并提供加载中和没有更多内容的页脚状态。

## 何时使用

- 消息流、动态、通知等需要连续向下浏览的内容。
- 数据按页从服务端追加，并且用户不需要跳到某个固定页码时。
- 需要页码导航、回到某一页时用 [Pagination 分页](/components/pagination)；内容量巨大且行高固定时用 [VirtualList 虚拟列表](/components/virtuallist)。

## 在线体验

示例包含同步追加、加载中、没有更多内容以及接近底部的触发距离。

<GalleryPreview component="InfiniteScroll" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var shown = 12

func page(c *ui.Context) {
    inf := ui.InfiniteScroll(c, func() {
        for i := 0; i < shown; i++ {
            ui.Text(c, fmt.Sprintf("消息 %d", i+1))
        }
    }).Height(320).Distance(24).Done(shown >= 60)
    if inf.NeedMore() && shown < 60 {
        shown += 12
    }
}
```

`InfiniteScroll` 的回调负责构建整个滚动内容。给它一个有界高度后，`NeedMore()` 会在距离底部不超过 `Distance` 时返回一次 `true`；应用追加数据后，内容高度变化会重新允许下一次触发。

## 常见用法

以下片段省略了 `import`。

### 真实请求的加载状态

触发后先把 `loading` 设为 `true`，请求完成后追加数据并设回 `false`。`Loading(true)` 会显示状态页脚并抑制新的 `NeedMore`，避免同一批数据被重复请求。

```go
var (
    messages []string
    loading  bool
    finished  bool
)

func loadMessages() {
    // 在实际应用中发起请求，回到 UI 线程后更新 messages。
}

func asyncList(c *ui.Context) {
    inf := ui.InfiniteScroll(c, func() {
        for _, message := range messages {
            ui.Text(c, message)
        }
    }).Height(360).Loading(loading).Done(finished)
    if inf.NeedMore() && !loading && !finished {
        loading = true
        loadMessages()
    }
}
```

### 自定义触发距离和页脚文案

`Distance(px)` 让应用在离底部还有一段距离时就开始准备下一页。`LoadingText` 和 `DoneText` 接受空字符串以隐藏对应文字；状态仍保留无障碍语义。

```go
func labeledInfinite(c *ui.Context) {
    inf := ui.InfiniteScroll(c, func() {
        for i := 0; i < shown; i++ {
            ui.Text(c, fmt.Sprintf("项目 %d", i+1))
        }
    }).Height(300).Distance(80).
        Loading(loading).Done(finished).
        LoadingText("正在同步下一页").DoneText("已经到底了")
    if inf.NeedMore() && !loading && !finished {
        loading = true
    }
}
```

### 暂停加载与程序化滚动

`Disabled(true)` 暂停触发 `NeedMore()`。页脚仍由 `Loading` 和 `Done` 控制。`ScrollTo` 接受垂直像素偏移，`Offset` 读取当前偏移。

```go
var pauseLoading bool

func controlledInfinite(c *ui.Context) {
    inf := ui.InfiniteScroll(c, func() {
        for i := 0; i < shown; i++ {
            ui.Text(c, fmt.Sprintf("项目 %d", i+1))
        }
    }).Height(300).Disabled(pauseLoading)
    if ui.Button(c, "回到顶部").Clicked() {
        inf.ScrollTo(0)
    }
    ui.Text(c, fmt.Sprintf("偏移 %.0f px", inf.Offset())).Tertiary()
}
```

### 短内容与已结束状态

当首屏内容不足以填满视口时，组件会继续检查是否需要加载；全部数据返回后要把 `Done(true)` 设上，避免短列表持续触发。`Done` 和 `Loading` 都应来自页面真实状态，而不是在回调中临时声明。

```go
func finishedInfinite(c *ui.Context) {
    ui.InfiniteScroll(c, func() {
        ui.Text(c, "最后一条记录")
    }).Height(300).Done(true).DoneText("没有更多记录")
}
```

## 主题与样式

InfiniteScroll 使用滚动容器、文本和加载指示器的主题令牌；没有单独的组件令牌。`ConfigProvider.Defaults("InfiniteScroll", ...)` 支持 `"distance"`（像素）作为实例默认触发距离，单实例的 `Distance` 优先。

## 注意事项

- **触发语义**：停留在触发范围内时，同一内容高度只触发一次。内容高度变化，或滚离触发范围后重返底部，都可再次触发。数据已到末尾时设置 `Done(true)`。
- **状态放在哪**：`loading`、`finished`、已加载条目和请求游标放在页面变量或 `ui.State` 中，不能在渲染函数里重置。
- **并发请求**：先设置 `loading` 再发请求，回调结果在 UI 线程写回数据；不要在多个 `NeedMore` 分支中同时启动同一页请求。
- **性能**：组件会构建滚动区域内的全部条目；条目非常多且不需要连续追加时改用 `VirtualList` 或 `Listy.Virtual`。
- **无障碍**：加载中页脚是状态区域并标记忙碌；自定义文字应准确描述请求状态。条目内按钮、图片仍要提供自己的可访问名称。
- **RTL**：页脚和滚动内容遵循当前方向，文字和行布局自动镜像。

## 示例代码

<!--@include: ./_gen/infinitescroll-examples.md-->

## API

<!--@include: ./_gen/infinitescroll-api.md-->

## 相关链接

- 源码：[ui/more_infinite.go](https://github.com/tangoui/tango/blob/main/ui/more_infinite.go)
- [ui Go API 参考](/reference/api/ui)
- [VirtualList 虚拟列表](/components/virtuallist)、[List 列表](/components/list)、[Pagination 分页](/components/pagination)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
