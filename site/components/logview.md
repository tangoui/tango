---
title: LogView 日志查看器
---
# LogView 日志查看器

显示持续追加的日志或进程输出：百万行虚拟滚动，解析 ANSI 颜色与属性，按级别着色和过滤，带搜索、自动换行和跟随底部，可拖选复制。数据放在线程安全的 `LogBuffer` 里，后台 goroutine 可以直接写入。

## 何时使用

- 展示构建输出、服务日志、子进程的标准输出等会不断增长、可能很长的文本流，尤其是带 ANSI 转义序列的终端输出。
- 需要搜索、按级别过滤、跟随最新行，或在不卡顿的前提下保留几十万行历史时使用。
- 展示一段固定的代码或配置用 [CodeBlock 代码块](/components/codeblock)；展示结构化数据用 [JSONView 值树查看器](/components/jsonview)；需要键盘输入、完整终端行为的交互式会话用终端组件，不要用 LogView 模拟。

## 在线体验

<GalleryPreview component="LogView" />

## 基本用法

日志数据放在 `*ui.LogBuffer` 里，它要比页面函数活得更久（包级变量或应用结构体的字段），不要在页面函数里每帧 `NewLogBuffer`。

```go
import "github.com/tangoui/tango/ui"

var logs = ui.NewLogBuffer(1_000_000) // 最多保留最新的一百万行

func init() {
    logs.Append(
        "2026-11-14 09:00:00 INFO  server: listening on :8080",
        "2026-11-14 09:00:01 WARN  cache: miss ratio above threshold",
        "2026-11-14 09:00:02 ERROR handler: request failed",
    )
}

func page(c *ui.Context) {
    ui.LogView(c, logs).Height(400).LineNumbers(true)
}
```

`NewLogBuffer(maxLines)` 的参数是保留的行数上限，超出后丢弃最旧的行；`<= 0` 表示不限制。`Height` 固定整个查看器（含工具栏）的高度；不设置时它在父容器里伸展，因此父容器要有明确高度。查看器最矮 120 像素。

## 常见用法

### 在后台 goroutine 里追加

`LogBuffer` 内部有锁，`Append`、`AppendLevel`、`Write`、`Flush`、`Clear` 都可以在任意 goroutine 里调用，界面线程读取时拿的是一份快照，不会出现读写竞争。唯一要自己做的是唤醒界面：用 `OnChange` 挂上 `Context.Invalidator()` 返回的函数。

```go
import (
    "os/exec"

    "github.com/tangoui/tango/ui"
)

var (
    buildLogs   = ui.NewLogBuffer(200_000)
    buildHooked bool
)

func startBuild() {
    buildLogs.Clear()
    cmd := exec.Command("go", "build", "-v", "./...")
    cmd.Stdout = buildLogs // *LogBuffer 实现了 io.Writer
    cmd.Stderr = buildLogs
    go func() {
        _ = cmd.Run()
        buildLogs.Flush() // 把没有换行结尾的最后一段也作为一行加入
    }()
}

func buildPage(c *ui.Context) {
    if !buildHooked {
        // 在界面线程取一次 Invalidator；之后任何 goroutine 追加行都会触发重绘。
        buildLogs.OnChange(c.Invalidator())
        buildHooked = true
    }
    if ui.Button(c, "构建").Primary().Clicked() {
        startBuild()
    }
    ui.LogView(c, buildLogs).Height(360)
}
```

要点：

- `Write` 按 `\n` 切行，没有换行结尾的尾部会等待下一次 `Write`，进程结束时调用 `Flush` 补上；行尾的 `\r` 会被去掉。
- 从后台 goroutine 里不要调用 `c.Invalidate()`（它只能在界面线程的帧内调用），要用 `c.Invalidator()` 返回的函数；它可以从任何 goroutine、在帧结束之后调用。
- `OnChange` 的回调在写入的 goroutine 里、锁外执行，每次追加或清空后调用一次。回调里不要做耗时操作。
- 一个 `LogBuffer` 只有一个 `OnChange` 回调；多个窗口显示同一个缓冲区时，回调里自己依次触发各窗口的 Invalidator。
- 界面线程内追加（例如按钮点击时 `Append`）同样安全，此时回调可以不设置。

### ANSI 颜色与属性

`LogBuffer` 在加入一行时就解析好转义序列，绘制时不再重复解析。支持粗体、弱化、斜体、下划线、反显、删除线及对应的复位；8+8 色（30–37、90–97、40–47、100–107）、256 色（`38;5;n`）和 24 位真彩色（`38;2;r;g;b`，也支持 `:` 分隔）。其他 CSI 序列、OSC 字符串和控制字符会被去掉；行内的回车 `\r` 会丢弃它之前的内容（进度条类输出只留最后一次刷新）。样式会像终端一样延续到下一行，直到被复位。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

func ansiDemo() {
    plain, spans := ui.ParseANSI("\x1b[1;31mERROR\x1b[0m connection reset")
    for _, sp := range spans {
        fmt.Println(plain[sp.Start:sp.End], sp.Style.Bold, sp.Style.Fg.Kind == ui.ANSIIndexed)
    }
}
```

`ui.ParseANSI` 是独立的解析函数：返回去掉转义序列后的纯文本，以及按字节范围描述样式的 `ANSISpan` 列表（默认样式的文本没有 span）。传入含 `\n` 的多行文本时，各行共享同一个样式状态。需要逐行解析、并让样式延续的场景用零值可用的 `ui.ANSIParser`（`Line` 方法）。0–15 号颜色取自当前主题，所以浅色和深色主题下都清晰可读。

### 级别识别、着色与过滤

每一行在加入时检测级别：`DetectLogLevel` 在行首 120 字节内找 `TRACE`、`DEBUG`、`INFO`、`WARN`/`WARNING`、`ERROR`/`FATAL`/`PANIC`/`CRITICAL`/`SEVERE`（不区分大小写），以及大写缩写 `TRC`、`DBG`、`INF`、`WRN`、`ERR`、`CRIT`。没有级别的行（例如堆栈的续行）继承上一行的级别，所以过滤时整段堆栈会和它的错误行一起显示或隐藏。

```go
import (
    "strings"

    "github.com/tangoui/tango/ui"
)

var (
    appLogs = newAppLogs()
    minLv   = ui.LogWarn // 只显示 Warn 及以上；LogNone 显示全部
)

func newAppLogs() *ui.LogBuffer {
    b := ui.NewLogBuffer(0)
    // 自定义检测：以 "[E]" 开头的是错误行，其余沿用默认规则。
    b.SetDetector(func(line string) ui.LogLevel {
        if strings.HasPrefix(line, "[E]") {
            return ui.LogError
        }
        return ui.DetectLogLevel(line)
    })
    b.AppendLevel(ui.LogInfo, "已知级别的行不再检测")
    return b
}

func levelPage(c *ui.Context) {
    ui.LogView(c, appLogs).Height(300).MinLevel(&minLv)
}
```

`AppendLevel(level, lines...)` 用于已知级别的行（传 `LogNone` 等同于 `Append`，走检测）。`SetDetector` 只影响之后加入的行。`MinLevel(&lv)` 把级别过滤绑定到你的变量：低于它的行被隐藏，`LogNone`（零值）显示全部；没有级别可继承的行（比如缓冲区开头的无级别行）始终显示。错误、警告、调试行默认按级别着色（行里已有 ANSI 颜色的部分保持原色），`NoLevelColors()` 关闭。

### 搜索、换行与跟随底部

工具栏提供搜索框（不区分大小写，Enter / Shift+Enter 或箭头在命中之间跳转）、级别下拉、自动换行开关、跟随底部开关和清空按钮。默认这些状态由 LogView 自己保存；需要和界面其他部分联动，或持久化，就把它们绑定到你的变量，并可用 `NoToolbar()` 隐藏内置工具栏，自己画控件：

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var (
    logQuery  string
    logWrap   bool
    logFollow = true // 绑定后初始值由你决定
    logsDemo  = ui.NewLogBuffer(100_000)
)

func customToolbar(c *ui.Context) {
    ui.Row(c, func() {
        ui.Input(c, &logQuery).Placeholder("搜索日志")
        ui.Toggle(c, "自动换行", &logWrap)
        ui.Toggle(c, "跟随", &logFollow)
    })
    lv := ui.LogView(c, logsDemo).Height(320).NoToolbar().
        Query(&logQuery).Wrap(&logWrap).Follow(&logFollow)
    ui.Text(c, fmt.Sprintf("命中数：%d", lv.Matches()))
}
```

跟随的规则：追加新行时视图保持在底部；用户向上滚动即停止跟随（`*follow` 变成 `false`），滚回底部又恢复跟随。搜索命中后会自动滚动到当前命中，同时停止跟随。`Matches()` 返回目前已找到的命中数；搜索在每帧的 6 毫秒预算内进行，超大缓冲区会在后续帧继续，所以这个数字可能还在增长。单次查询最多保留一百万个命中。

### 行号与读取内容

`LineNumbers(true)` 显示行号，行号计入已被丢弃的行，所以一行在整个会话里编号不变。需要在代码里读取内容时：

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

func readLogs(c *ui.Context, buf *ui.LogBuffer) {
    lv := ui.LogView(c, buf).Height(240)
    ui.Text(c, fmt.Sprintf("共 %d 行，选中 %d 字节", buf.Len(), len(lv.Selection())))
    if buf.Len() > 0 {
        last := buf.Len() - 1
        ui.Text(c, fmt.Sprintf("最后一行（级别 %d）：%s", buf.Level(last), buf.Line(last)))
    }
}
```

`Len`、`Line(i)`、`Level(i)` 读取的是纯文本和级别（`i` 从最旧的保留行起算）；`Selection()` 返回视图里选中的文本。选区支持拖动、双击选词、`Cmd/Ctrl+A` 全选、`Cmd/Ctrl+C` 复制，视图是一个 Tab 停靠点，聚焦后方向键、`PageUp` / `PageDown`、`Home` / `End` 滚动。

## 主题与样式

LogView 的令牌是 `theme.Components.LogView`（`theme.LogViewToken`）：

- 外观：`Bg`、`BorderColor`（零值无边框）、`Radius`、`LineFontSize`、`LineHeight`、`PaddingInline`、`ToolbarGap`、`Text`、`GutterColor`、`SelectionBg`。
- 级别色：`ErrorColor`（error / fatal）、`WarnColor`、`DebugColor`（debug / trace）。
- 搜索高亮：`MatchBg`、`MatchCurrentBg`、`MatchCurrentBorder`。
- 16 个 ANSI 颜色：`Black`、`Red`、`Green`、`Yellow`、`Blue`、`Magenta`、`Cyan`、`White` 及 `Bright` 前缀的八个。256 色中的 16–231 为 6×6×6 色立方，232–255 为灰阶。

ANSI 颜色会按背景自动调整对比度，所以同一段输出在浅色、深色和高对比主题下都可读；三套风格（Ant Design / Element Plus / shadcn）各自派生边框、圆角和字号。用 `theme.WithComponents` 覆盖：

```go
import "github.com/tangoui/tango/ui/theme"

func logTheme() *theme.Theme {
    return theme.ElementPlus(theme.WithComponents(theme.Components{
        LogView: theme.LogViewToken{LineFontSize: 12, LineHeight: 18},
    }))
}
```

工具栏文字（搜索占位、匹配数、级别名、“N 行”等）来自本地化表 `Locale().LogView`（`ui.LogViewLocale`）。LogView 没有 `ConfigProvider.Defaults` 键；在 RTL 布局中日志仍然从左到右排版。

## 注意事项

- **缓冲区的生命周期**：`LogBuffer` 放在页面函数之外。LogView 的搜索结果、级别过滤索引、滚动位置等状态保存在节点 ID 对应的状态里；调用 `Clear` 后搜索结果和过滤索引会重置，选区也会清除。
- **工具栏的“清空”会清掉你的缓冲区**：它调用的是 `buf.Clear()`，和代码里直接清空等价；不希望用户清空时用 `NoToolbar()` 并自己画需要的控件。
- **并发规则**：写入端只能通过 `LogBuffer` 的方法（任何 goroutine 都可以）；界面端只在界面线程里构建 LogView。唤醒界面只用 `Context.Invalidator()`。
- **内存**：不设上限的缓冲区会一直增长；长时间运行的流用 `NewLogBuffer(n)` 限定行数。ANSI 解析在加入时完成一次，每行额外保存样式 span。
- **长行**：关闭自动换行时每行最多显示 10000 个字符；打开换行后按字符单元计算折行，不影响百万行的滚动。
- **无障碍**：日志区作为文本区域暴露给读屏，默认名称来自本地化文案，用 `A11yLabel` 指定更具体的名称（如“构建日志”）。工具栏中的开关、按钮都带标签，可用键盘操作。

## 示例代码

<!--@include: ./_gen/logview-examples.md-->

## API

<!--@include: ./_gen/logview-api.md-->

## 相关链接

- [源码：logview.go](https://github.com/tangoui/tango/blob/main/ui/logview.go)、[logview_buffer.go](https://github.com/tangoui/tango/blob/main/ui/logview_buffer.go)、[ansi.go](https://github.com/tangoui/tango/blob/main/ui/ansi.go)
- [ui Go API](/reference/api/ui)
- [CodeBlock 代码块](/components/codeblock)
- [JSONView 值树查看器](/components/jsonview)
